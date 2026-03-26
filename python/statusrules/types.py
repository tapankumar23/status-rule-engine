"""
types.py — public data types for the status rule engine.

All wire-level identifiers (ErrorCode strings, SourceSystem strings) match the
Go implementation exactly so that JSON payloads are interchangeable between the
two runtimes.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import IntEnum


# ---------------------------------------------------------------------------
# Flow
# ---------------------------------------------------------------------------

class Flow(IntEnum):
    """Direction of a shipment lifecycle.

    FORWARD = normal delivery direction (booking → delivery).
    REVERSE = return / RTO direction (delivery attempt → origin hub).
    """

    FORWARD = 1
    REVERSE = 2


# ---------------------------------------------------------------------------
# SourceSystem
# ---------------------------------------------------------------------------

class SourceSystem(str):
    """String constants that identify which back-end system is reporting a
    status update.  These values are compared against the ``owned_by`` field
    in the spec to detect inter-system transitions.

    Usage::

        ctx = TransitionContext(source_system=SourceSystem.HUB)
    """

    BOOKING  = 'BOOKING_SYSTEM'
    HUB      = 'HUB_SYSTEM'
    DELIVERY = 'DELIVERY_SYSTEM'
    TPL      = 'SYSTEM_3PL'


# ---------------------------------------------------------------------------
# ErrorCode
# ---------------------------------------------------------------------------

class ErrorCode(str):
    """String sentinel values returned in ValidationResult.error_code.

    ``NONE`` means the transition is valid; all other values describe why it
    was rejected.  These strings match the Go ``Err*`` constant names.
    """

    NONE                  = 'ErrNone'
    INVALID_TRANSITION    = 'ErrInvalidTransition'
    TERMINAL_STATUS       = 'ErrTerminalStatus'
    UNKNOWN_STATUS        = 'ErrUnknownStatus'
    MISSING_OPERATOR_ID   = 'ErrMissingOperatorID'
    FLOW_MISMATCH         = 'ErrFlowMismatch'
    INVALID_SOURCE_SYSTEM = 'ErrInvalidSourceSystem'
    ZERO_VALUE_STATUS     = 'ErrZeroValueStatus'


# ---------------------------------------------------------------------------
# TransitionContext
# ---------------------------------------------------------------------------

@dataclass
class TransitionContext:
    """Caller-supplied metadata that accompanies a status-transition request.

    Attributes:
        source_system: Identifies the reporting system (use ``SourceSystem``
                       constants or any ``owned_by`` string from the spec).
        operator_id:   Required for FORCE_* override transitions.
        awb:           Airway-bill / shipment identifier (used for logging /
                       anomaly attribution only; not part of rule logic).
    """

    source_system: str = ''
    operator_id: str = ''
    awb: str = ''


# ---------------------------------------------------------------------------
# ValidationResult
# ---------------------------------------------------------------------------

@dataclass
class ValidationResult:
    """Result returned by ``Engine.validate`` and ``Engine.validate_initial``.

    Attributes:
        valid:              ``True`` when the transition is permitted.
        is_override:        ``True`` when the edge carries the ``is_override``
                            flag (FORCE_* targets).
        is_inter_system:    ``True`` when the source system differs from the
                            ``owned_by`` system of *next_status*.
        error_code:         One of the ``ErrorCode`` string constants.
        anomaly_reason:     Human-readable explanation when ``valid`` is
                            ``False`` or when the transition is unusual.
    """

    valid: bool = False
    is_override: bool = False
    is_inter_system: bool = False
    error_code: str = ErrorCode.NONE
    anomaly_reason: str = ''


# ---------------------------------------------------------------------------
# MetricsSnapshot
# ---------------------------------------------------------------------------

@dataclass
class MetricsSnapshot:
    """Point-in-time copy of the engine's internal counters.

    All counters are cumulative since the ``Engine`` was instantiated.
    Thread safety relies on the GIL; see ``Engine.snapshot`` for details.
    """

    validations_total: int = 0
    invalid_transitions: int = 0
    override_transitions: int = 0
    inter_system_transitions: int = 0
    zero_value_rejections: int = 0
