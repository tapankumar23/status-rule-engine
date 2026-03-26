"""
tests/test_engine.py — pytest suite for the status rule engine.

Run with:
    cd python
    pytest tests/
"""

import pytest

from statusrules import (
    Engine,
    ErrorCode,
    Flow,
    SourceSystem,
    Status,
    TransitionContext,
    ValidationResult,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(scope="module")
def engine() -> Engine:
    """A single Engine instance shared across all tests in this module.
    Engine compilation is deterministic and read-only after __init__, so
    sharing is safe.
    """
    return Engine()


# ---------------------------------------------------------------------------
# Helper
# ---------------------------------------------------------------------------

def _ctx(**kwargs) -> TransitionContext:
    """Convenience constructor for TransitionContext."""
    return TransitionContext(**kwargs)


# ---------------------------------------------------------------------------
# Test 1 — Valid forward transition
# ---------------------------------------------------------------------------

def test_valid_forward_transition(engine: Engine) -> None:
    """HUB_IN_SCANNED → HUB_OUTSCAN in FORWARD flow must be permitted."""
    result = engine.validate(
        current=Status.HUB_IN_SCANNED,
        next_status=Status.HUB_OUTSCAN,
        flow=Flow.FORWARD,
        ctx=_ctx(source_system=SourceSystem.HUB),
    )
    assert result.valid, f"Expected valid, got error_code={result.error_code}"
    assert result.error_code == ErrorCode.NONE


# ---------------------------------------------------------------------------
# Test 2 — Invalid transition
# ---------------------------------------------------------------------------

def test_invalid_transition(engine: Engine) -> None:
    """DRAFT → DELIVERED is not an edge in the spec; must be rejected."""
    result = engine.validate(
        current=Status.DRAFT,
        next_status=Status.DELIVERED,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    assert not result.valid
    assert result.error_code == ErrorCode.INVALID_TRANSITION


# ---------------------------------------------------------------------------
# Test 3 — Terminal status
# ---------------------------------------------------------------------------

def test_terminal_status_rejected(engine: Engine) -> None:
    """No transition is allowed out of a terminal status (DELIVERED)."""
    result = engine.validate(
        current=Status.DELIVERED,
        next_status=Status.INSCANNED_AT_CP,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    assert not result.valid
    assert result.error_code == ErrorCode.TERMINAL_STATUS


# ---------------------------------------------------------------------------
# Test 4 — FORCE_* without operator_id
# ---------------------------------------------------------------------------

def test_force_override_missing_operator_id(engine: Engine) -> None:
    """HUB_IN_SCANNED → FORCE_HUB_OUTSCAN without operator_id must fail."""
    result = engine.validate(
        current=Status.HUB_IN_SCANNED,
        next_status=Status.FORCE_HUB_OUTSCAN,
        flow=Flow.FORWARD,
        ctx=_ctx(operator_id=''),          # deliberately empty
    )
    assert not result.valid
    assert result.error_code == ErrorCode.MISSING_OPERATOR_ID
    # The engine should still tell callers this is an override edge.
    assert result.is_override


# ---------------------------------------------------------------------------
# Test 5 — FORCE_* with operator_id present
# ---------------------------------------------------------------------------

def test_force_override_with_operator_id(engine: Engine) -> None:
    """HUB_IN_SCANNED → FORCE_HUB_OUTSCAN with operator_id must succeed and
    set is_override=True."""
    result = engine.validate(
        current=Status.HUB_IN_SCANNED,
        next_status=Status.FORCE_HUB_OUTSCAN,
        flow=Flow.FORWARD,
        ctx=_ctx(
            source_system=SourceSystem.HUB,
            operator_id='ops-agent-42',
        ),
    )
    assert result.valid, f"Expected valid, got error_code={result.error_code}"
    assert result.is_override
    assert result.error_code == ErrorCode.NONE


# ---------------------------------------------------------------------------
# Test 6 — Wildcard exception target
# ---------------------------------------------------------------------------

def test_wildcard_exception_target(engine: Engine) -> None:
    """DAMAGED is a wildcard target; OUT_FOR_DELIVERY → DAMAGED must be valid
    in FORWARD flow."""
    result = engine.validate(
        current=Status.OUT_FOR_DELIVERY,
        next_status=Status.DAMAGED,
        flow=Flow.FORWARD,
        ctx=_ctx(source_system=SourceSystem.HUB),
    )
    assert result.valid, f"Expected valid, got error_code={result.error_code}"
    assert result.error_code == ErrorCode.NONE


# ---------------------------------------------------------------------------
# Test 7 — Zero-value status
# ---------------------------------------------------------------------------

def test_zero_value_status_rejected(engine: Engine) -> None:
    """Status ID 0 is the uninitialized sentinel and must always be rejected."""
    result_current = engine.validate(
        current=0,
        next_status=Status.BOOKED,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    assert not result_current.valid
    assert result_current.error_code == ErrorCode.ZERO_VALUE_STATUS

    result_next = engine.validate(
        current=Status.DRAFT,
        next_status=0,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    assert not result_next.valid
    assert result_next.error_code == ErrorCode.ZERO_VALUE_STATUS


# ---------------------------------------------------------------------------
# Test 8 — validate_initial DRAFT FORWARD
# ---------------------------------------------------------------------------

def test_validate_initial_draft_forward(engine: Engine) -> None:
    """DRAFT in FORWARD flow is the only declared initial status; it must pass
    validate_initial."""
    result = engine.validate_initial(
        first=Status.DRAFT,
        flow=Flow.FORWARD,
        ctx=_ctx(source_system=SourceSystem.BOOKING),
    )
    assert result.valid, f"Expected valid, got error_code={result.error_code}"
    assert result.error_code == ErrorCode.NONE


def test_validate_initial_non_initial_rejected(engine: Engine) -> None:
    """BOOKED is not declared as an initial status; validate_initial must
    reject it."""
    result = engine.validate_initial(
        first=Status.BOOKED,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    assert not result.valid
    assert result.error_code == ErrorCode.INVALID_TRANSITION


# ---------------------------------------------------------------------------
# Test 9 — spec_version
# ---------------------------------------------------------------------------

def test_spec_version(engine: Engine) -> None:
    """Engine must report the spec version string from the YAML."""
    assert engine.spec_version() == '1.0.0'


# ---------------------------------------------------------------------------
# Test 10 — permitted_transitions non-empty
# ---------------------------------------------------------------------------

def test_permitted_transitions_non_empty(engine: Engine) -> None:
    """HUB_IN_SCANNED has several outbound FORWARD edges; the list must be
    non-empty and contain at least HUB_OUTSCAN."""
    transitions = engine.permitted_transitions(
        current=Status.HUB_IN_SCANNED,
        flow=Flow.FORWARD,
    )
    assert len(transitions) > 0, "Expected at least one permitted transition"
    assert Status.HUB_OUTSCAN in transitions


def test_permitted_transitions_terminal_empty(engine: Engine) -> None:
    """Terminal statuses must return an empty list."""
    transitions = engine.permitted_transitions(
        current=Status.DELIVERED,
        flow=Flow.FORWARD,
    )
    assert transitions == []


# ---------------------------------------------------------------------------
# Test 11 — metrics counters accumulate correctly
# ---------------------------------------------------------------------------

def test_metrics_accumulate(engine: Engine) -> None:
    """After a fresh engine is created, totals must track valid and invalid
    calls correctly."""
    e = Engine()
    snap0 = e.snapshot()

    # One valid call.
    e.validate(
        current=Status.DRAFT,
        next_status=Status.BOOKED,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    snap1 = e.snapshot()
    assert snap1.validations_total == snap0.validations_total + 1
    assert snap1.invalid_transitions == snap0.invalid_transitions

    # One invalid call.
    e.validate(
        current=Status.DRAFT,
        next_status=Status.DELIVERED,
        flow=Flow.FORWARD,
        ctx=_ctx(),
    )
    snap2 = e.snapshot()
    assert snap2.validations_total   == snap0.validations_total + 2
    assert snap2.invalid_transitions == snap0.invalid_transitions + 1


# ---------------------------------------------------------------------------
# Test 12 — inter-system detection
# ---------------------------------------------------------------------------

def test_inter_system_flag(engine: Engine) -> None:
    """A HUB_SYSTEM status reported by DELIVERY_SYSTEM must set
    is_inter_system=True while still being valid."""
    # HUB_OUTSCAN is owned by HUB_SYSTEM; reporting via DELIVERY_SYSTEM.
    result = engine.validate(
        current=Status.HUB_IN_SCANNED,
        next_status=Status.HUB_OUTSCAN,
        flow=Flow.FORWARD,
        ctx=_ctx(source_system=SourceSystem.DELIVERY),
    )
    assert result.valid
    assert result.is_inter_system


def test_no_inter_system_flag_same_system(engine: Engine) -> None:
    """Transition within the same system must NOT set is_inter_system."""
    result = engine.validate(
        current=Status.HUB_IN_SCANNED,
        next_status=Status.HUB_OUTSCAN,
        flow=Flow.FORWARD,
        ctx=_ctx(source_system=SourceSystem.HUB),
    )
    assert result.valid
    assert not result.is_inter_system
