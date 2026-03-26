"""
statusrules — shipment status transition rule engine for logistics platforms.

Public surface
--------------
    Engine            — compiled rule engine (instantiate once, reuse)
    Flow              — FORWARD / REVERSE direction enum
    SourceSystem      — string constants for ``owned_by`` system names
    Status            — integer constants for every shipment status
    TransitionContext — caller-supplied context for a transition request
    ValidationResult  — result returned by Engine.validate / validate_initial
    ErrorCode         — string sentinel error codes
    MetricsSnapshot   — point-in-time counter snapshot from Engine.snapshot()

Quickstart
----------
::

    from statusrules import Engine, Flow, Status, SourceSystem, TransitionContext

    engine = Engine()
    result = engine.validate(
        current=Status.HUB_IN_SCANNED,
        next_status=Status.HUB_OUTSCAN,
        flow=Flow.FORWARD,
        ctx=TransitionContext(source_system=SourceSystem.HUB),
    )
    assert result.valid
"""

from .engine import Engine
from .types import (
    ErrorCode,
    Flow,
    MetricsSnapshot,
    SourceSystem,
    TransitionContext,
    ValidationResult,
)
from .status import Status

__all__ = [
    "Engine",
    "ErrorCode",
    "Flow",
    "MetricsSnapshot",
    "SourceSystem",
    "Status",
    "TransitionContext",
    "ValidationResult",
]
