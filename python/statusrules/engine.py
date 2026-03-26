"""
engine.py — shipment status transition rule engine.

Algorithm overview
------------------
The adjacency table is a flat array of unsigned-short (uint16) flags with
8 192 entries, matching the Go implementation exactly.

Key packing
~~~~~~~~~~~
    key = (from_id << 7) | (to_id << 1) | flow_bit

where:
  * from_id / to_id are Python IDs (yaml_id + 1), each fitting in 6 bits
    (values 1..55 ≤ 63).
  * flow_bit: FORWARD → 0, REVERSE → 1
  * The table therefore needs  64 × 64 × 2 = 8 192 slots.

Edge flags (uint16 bitmask)
~~~~~~~~~~~~~~~~~~~~~~~~~~~
  0x01  FLAG_PRESENT        — edge exists
  0x02  FLAG_OVERRIDE       — requires operator_id (FORCE_*)
  0x04  FLAG_REQUIRES_OP_ID — alias for FLAG_OVERRIDE (same bit)
  0x08  FLAG_INTER_SYSTEM   — (not set at compile time; derived at runtime)

Thread safety
~~~~~~~~~~~~~
The plain int counters (``_cnt_*``) are updated without locks.  CPython's GIL
makes individual integer increments atomic, so reads via ``snapshot()`` are
safe but may race on rapid-fire concurrent calls.  If you need hard guarantees
under free-threaded Python (PEP 703 / Python 3.13t), replace the counters with
``threading.Lock``-guarded fields.
"""

from __future__ import annotations

import array
from pathlib import Path
from typing import List, Optional

import yaml

from .status import CODE_TO_ID, ID_TO_CODE, MAX_ID
from .types import (
    ErrorCode,
    Flow,
    MetricsSnapshot,
    TransitionContext,
    ValidationResult,
)

# ---------------------------------------------------------------------------
# Edge-flag constants
# ---------------------------------------------------------------------------

FLAG_PRESENT        = 0x01
FLAG_OVERRIDE       = 0x02  # edge carries is_override; operator_id required
FLAG_REQUIRES_OP_ID = 0x04  # same semantic — set alongside FLAG_OVERRIDE
FLAG_INTER_SYSTEM   = 0x08  # derived at validate() time, not stored in table

# ---------------------------------------------------------------------------
# Default spec path (relative to this file so the package is relocatable)
# ---------------------------------------------------------------------------

_DEFAULT_SPEC: Path = (
    Path(__file__).parent.parent.parent / "spec" / "rules.yaml"
)


class Engine:
    """Compiled rule engine for validating shipment status transitions.

    Parameters
    ----------
    spec_path:
        Override the location of ``rules.yaml``.  Defaults to the canonical
        path three directory levels above this file (repo root/spec/rules.yaml).

    Example
    -------
    ::

        engine = Engine()
        result = engine.validate(
            current=Status.HUB_IN_SCANNED,
            next_status=Status.HUB_OUTSCAN,
            flow=Flow.FORWARD,
            ctx=TransitionContext(source_system=SourceSystem.HUB),
        )
        assert result.valid
    """

    # ------------------------------------------------------------------
    # Construction / compilation
    # ------------------------------------------------------------------

    def __init__(self, spec_path: Optional[Path] = None) -> None:
        path = Path(spec_path) if spec_path is not None else _DEFAULT_SPEC
        raw = path.read_text(encoding="utf-8")
        spec = yaml.safe_load(raw)
        self._compile(spec)

    def _compile(self, spec: dict) -> None:
        """Build the adjacency array and supporting metadata from the parsed
        YAML spec dict.  Called exactly once from ``__init__``."""

        # ── 1. Build code → python-id mapping ────────────────────────────────
        # yaml_id is 0-indexed; python id = yaml_id + 1.
        code_to_id: dict[str, int] = {}
        for entry in spec["statuses"]:
            code_to_id[entry["code"]] = entry["id"] + 1

        # Sanity: must agree with the static CODE_TO_ID table.
        assert code_to_id == CODE_TO_ID, (
            "Loaded spec does not match the static CODE_TO_ID table in "
            "status.py — the spec may have been renumbered."
        )

        # ── 2. owners[id] = owned_by string  (index 0 unused) ────────────────
        owners: List[str] = [''] * (MAX_ID + 1)
        terminals: List[bool] = [False] * (MAX_ID + 1)
        for entry in spec["statuses"]:
            pid = entry["id"] + 1
            owners[pid] = entry.get("owned_by", '')
            terminals[pid] = bool(entry.get("terminal", False))

        # ── 3. Adjacency array (8 192 unsigned-short slots) ───────────────────
        # Initialised to all-zeros; a zero entry means "no edge".
        adj: array.array = array.array('H', bytes(8192 * 2))

        def _set(from_id: int, to_id: int, flow: Flow, flags: int) -> None:
            """OR *flags* into the adjacency slot for (from_id, to_id, flow)."""
            flow_bit = 0 if flow == Flow.FORWARD else 1
            key = (from_id << 7) | (to_id << 1) | flow_bit
            adj[key] = adj[key] | flags

        # ── 4. Expand wildcard targets ────────────────────────────────────────
        # A wildcard target is reachable from every non-terminal status in
        # BOTH flows.
        wildcard_ids = {
            code_to_id[code]
            for code in spec.get("wildcard_targets", [])
        }
        for entry in spec["statuses"]:
            from_id = entry["id"] + 1
            if terminals[from_id]:
                continue  # terminal statuses have no outbound edges
            for to_id in wildcard_ids:
                for flow in (Flow.FORWARD, Flow.REVERSE):
                    _set(from_id, to_id, flow, FLAG_PRESENT)

        # ── 5. Apply explicit transitions ─────────────────────────────────────
        def _resolve(code_or_list) -> List[int]:
            """Normalise a ``to`` value (string or list of strings) to a list
            of Python status IDs."""
            if isinstance(code_or_list, list):
                return [code_to_id[c] for c in code_or_list]
            return [code_to_id[code_or_list]]

        for txn in spec.get("transitions", []):
            from_id = code_to_id[txn["from"]]
            flow    = Flow.FORWARD if txn["flow"] == "FORWARD" else Flow.REVERSE
            flags   = FLAG_PRESENT

            if txn.get("is_override"):
                flags |= FLAG_OVERRIDE | FLAG_REQUIRES_OP_ID

            for to_id in _resolve(txn["to"]):
                _set(from_id, to_id, flow, flags)

        # ── 6. Build initial-statuses set ────────────────────────────────────
        initials: set[tuple[int, Flow]] = set()
        for entry in spec.get("initial_statuses", []):
            sid   = code_to_id[entry["code"]]
            fflow = Flow.FORWARD if entry["flow"] == "FORWARD" else Flow.REVERSE
            initials.add((sid, fflow))

        # ── Store compiled data ───────────────────────────────────────────────
        self._adj      = adj
        self._owners   = owners
        self._terminals = terminals
        self._initials = initials
        self._version  = spec.get("spec_version", "")

        # Metrics counters (GIL-atomic for CPython; see module docstring).
        self._cnt_total       = 0
        self._cnt_invalid     = 0
        self._cnt_override    = 0
        self._cnt_inter       = 0
        self._cnt_zero        = 0

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def validate(
        self,
        current: int,
        next_status: int,
        flow: Flow,
        ctx: TransitionContext,
    ) -> ValidationResult:
        """Validate a status transition for an in-flight shipment.

        Parameters
        ----------
        current:     Python status ID of the *current* status.
        next_status: Python status ID of the *proposed next* status.
        flow:        ``Flow.FORWARD`` or ``Flow.REVERSE``.
        ctx:         Caller context (source system, operator_id, AWB).

        Returns
        -------
        ValidationResult with ``valid=True`` when the transition is allowed.
        """
        self._cnt_total += 1

        # ── Zero-value guard ──────────────────────────────────────────────────
        if current == 0 or next_status == 0:
            self._cnt_zero    += 1
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.ZERO_VALUE_STATUS,
                anomaly_reason="current or next_status is the zero sentinel",
            )

        # ── Unknown-status guard ──────────────────────────────────────────────
        if current > MAX_ID or next_status > MAX_ID:
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.UNKNOWN_STATUS,
                anomaly_reason=(
                    f"status id out of range: current={current} "
                    f"next={next_status} max={MAX_ID}"
                ),
            )

        # ── Terminal guard ────────────────────────────────────────────────────
        if self._terminals[current]:
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.TERMINAL_STATUS,
                anomaly_reason=(
                    f"{ID_TO_CODE.get(current, current)} is a terminal status"
                ),
            )

        # ── Adjacency lookup ──────────────────────────────────────────────────
        flow_bit = 0 if flow == Flow.FORWARD else 1
        key   = (current << 7) | (next_status << 1) | flow_bit
        flags = self._adj[key]

        if not (flags & FLAG_PRESENT):
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.INVALID_TRANSITION,
                anomaly_reason=(
                    f"no edge {ID_TO_CODE.get(current, current)} → "
                    f"{ID_TO_CODE.get(next_status, next_status)} "
                    f"in flow {flow.name}"
                ),
            )

        # ── Override / operator_id check ─────────────────────────────────────
        is_override = bool(flags & FLAG_OVERRIDE)
        if is_override and not ctx.operator_id:
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                is_override=True,
                error_code=ErrorCode.MISSING_OPERATOR_ID,
                anomaly_reason=(
                    f"transition to {ID_TO_CODE.get(next_status, next_status)} "
                    "requires operator_id in context"
                ),
            )

        # ── Inter-system detection ────────────────────────────────────────────
        owner = self._owners[next_status]
        is_inter = bool(
            ctx.source_system
            and owner
            and ctx.source_system != owner
        )

        # ── Metrics ───────────────────────────────────────────────────────────
        if is_override:
            self._cnt_override += 1
        if is_inter:
            self._cnt_inter += 1

        return ValidationResult(
            valid=True,
            is_override=is_override,
            is_inter_system=is_inter,
            error_code=ErrorCode.NONE,
        )

    def validate_initial(
        self,
        first: int,
        flow: Flow,
        ctx: TransitionContext,
    ) -> ValidationResult:
        """Validate the *first* status for a brand-new shipment.

        Unlike ``validate``, there is no *current* status — only a proposed
        *first* status.  The engine checks the ``initial_statuses`` list from
        the spec rather than the adjacency table.

        Parameters
        ----------
        first: Python status ID of the proposed initial status.
        flow:  ``Flow.FORWARD`` or ``Flow.REVERSE``.
        ctx:   Caller context (used for inter-system and zero-value checks).
        """
        self._cnt_total += 1

        if first == 0:
            self._cnt_zero    += 1
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.ZERO_VALUE_STATUS,
                anomaly_reason="first status is the zero sentinel",
            )

        if first > MAX_ID:
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.UNKNOWN_STATUS,
                anomaly_reason=f"status id out of range: first={first} max={MAX_ID}",
            )

        if (first, flow) not in self._initials:
            self._cnt_invalid += 1
            return ValidationResult(
                valid=False,
                error_code=ErrorCode.INVALID_TRANSITION,
                anomaly_reason=(
                    f"{ID_TO_CODE.get(first, first)} / {flow.name} "
                    "is not a valid initial status"
                ),
            )

        owner    = self._owners[first]
        is_inter = bool(
            ctx.source_system
            and owner
            and ctx.source_system != owner
        )
        if is_inter:
            self._cnt_inter += 1

        return ValidationResult(
            valid=True,
            is_inter_system=is_inter,
            error_code=ErrorCode.NONE,
        )

    def permitted_transitions(self, current: int, flow: Flow) -> List[int]:
        """Return a sorted list of Python status IDs reachable from *current*
        in the given *flow*.

        Parameters
        ----------
        current: Python status ID of the source status.
        flow:    ``Flow.FORWARD`` or ``Flow.REVERSE``.

        Returns
        -------
        List of integer status IDs (may be empty if *current* is terminal or
        has no outbound edges for this flow).
        """
        if current <= 0 or current > MAX_ID:
            return []
        if self._terminals[current]:
            return []

        flow_bit = 0 if flow == Flow.FORWARD else 1
        result: List[int] = []
        for to_id in range(1, MAX_ID + 1):
            key = (current << 7) | (to_id << 1) | flow_bit
            if self._adj[key] & FLAG_PRESENT:
                result.append(to_id)
        return result

    def spec_version(self) -> str:
        """Return the ``spec_version`` string from the loaded YAML (e.g. ``'1.0.0'``)."""
        return self._version

    def snapshot(self) -> MetricsSnapshot:
        """Return a point-in-time copy of the engine's cumulative counters.

        Thread safety: each attribute read is GIL-atomic in CPython, but two
        successive reads are not.  Under concurrent load the snapshot may be
        slightly inconsistent across fields.  This is acceptable for
        monitoring/telemetry use cases.
        """
        return MetricsSnapshot(
            validations_total          = self._cnt_total,
            invalid_transitions        = self._cnt_invalid,
            override_transitions       = self._cnt_override,
            inter_system_transitions   = self._cnt_inter,
            zero_value_rejections      = self._cnt_zero,
        )
