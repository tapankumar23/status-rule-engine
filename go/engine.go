// Package statusrules is the shipment status transition rule engine.
//
// It answers one question: given a shipment's current status and a proposed
// next status, is this transition allowed by the spec?
//
// All validation is done in-process via an immutable adjacency map compiled
// from the shared rule spec at startup. There are no network calls on the
// hot path. Validate() completes in < 100µs p99 and allocates nothing on
// the success path.
//
// Usage:
//
//	engine := statusrules.NewEngine()
//	result := engine.Validate(current, next, statusrules.FlowForward, ctx)
//	if !result.Valid { ... }
package statusrules

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/logistics-oss/status-rules/status"
)

// Pre-defined AnomalyReason strings. String literals live in the data segment —
// assigning them to the result struct never allocates on the heap.
const (
	reasonZeroValue       = "zero-value status: use ValidateInitial for new shipments"
	reasonUnknownStatus   = "unknown status code"
	reasonTerminal        = "transition from a terminal status is never valid"
	reasonInvalidFlow     = "flow must be FlowForward or FlowReverse"
	reasonNotInSpec       = "transition not defined in rule spec"
	reasonMissingOperator = "operator_id required for FORCE_* override transition"
	reasonWrongOwner      = "ctx.SourceSystem does not own the next status"
)

// Engine is the compiled rule engine. It is immutable after construction and
// safe for concurrent use by any number of goroutines without locking.
type Engine struct {
	adj         adjacencyMap
	owners      ownerArray
	terminals   terminalArray
	initials    []initialEntry
	specVersion string

	// Atomic counters — LOCK XADD, no allocation, no contention.
	// Read via Snapshot() from a background goroutine; never on the hot path.
	cntTotal       atomic.Uint64
	cntInvalid     atomic.Uint64
	cntOverride    atomic.Uint64
	cntInterSystem atomic.Uint64
	cntZeroValue   atomic.Uint64
}

// NewEngine initializes the rule engine from the shared spec/rules.yaml.
// Called once at application startup. Panics if the spec cannot be read or
// is structurally invalid.
//
// Initialization time: ~5ms (adjacency map compile + wildcard expansion).
// The returned *Engine is immutable and safe for concurrent use.
func NewEngine() *Engine {
	_, file, _, _ := runtime.Caller(0)
	specPath := filepath.Join(filepath.Dir(file), "..", "spec", "rules.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		panic("status-rules: cannot read spec/rules.yaml: " + err.Error())
	}
	e, err := compileSpec(data)
	if err != nil {
		panic("status-rules: spec is invalid: " + err.Error())
	}
	return e
}

// NewEngineFromSpec initializes from an externally provided spec YAML.
// Used in tests and for canary validation of new rule spec versions before release.
// Not for production initialization.
func NewEngineFromSpec(specYAML []byte) (*Engine, error) {
	return compileSpec(specYAML)
}

// Validate checks whether transitioning from currentStatus to nextStatus
// is permitted in the given flow and context.
//
// This is the hot path. It completes in < 100µs p99.
// It is safe to call from multiple goroutines concurrently.
// It allocates nothing on the heap for the success path.
//
// currentStatus: the shipment's last confirmed status known to the caller.
//
//	The caller is responsible for providing this correctly.
//	The library does not store or retrieve shipment state.
//	Passing a zero-value Status returns ErrZeroValueStatus immediately.
//
// nextStatus: the status the source system intends to emit.
//
//	Passing a zero-value Status returns ErrZeroValueStatus immediately.
//
// flow: the current shipment flow (FlowForward or FlowReverse).
// ctx:  auxiliary metadata — source system identity, operator ID for overrides.
func (e *Engine) Validate(
	currentStatus status.Status,
	nextStatus status.Status,
	flow Flow,
	ctx TransitionContext,
) ValidationResult {
	e.cntTotal.Add(1)

	// ── Guard: zero-value inputs ──────────────────────────────────────────────
	if currentStatus == 0 || nextStatus == 0 {
		e.cntZeroValue.Add(1)
		return ValidationResult{
			ErrorCode:     ErrZeroValueStatus,
			AnomalyReason: reasonZeroValue,
		}
	}

	// ── Guard: unknown status IDs ─────────────────────────────────────────────
	if currentStatus > status.MaxID || nextStatus > status.MaxID {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrUnknownStatus,
			AnomalyReason: reasonUnknownStatus,
		}
	}

	// ── Guard: valid flow ─────────────────────────────────────────────────────
	if flow != FlowForward && flow != FlowReverse {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrFlowMismatch,
			AnomalyReason: reasonInvalidFlow,
		}
	}

	// ── Guard: terminal source status ─────────────────────────────────────────
	if e.terminals[currentStatus] {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrTerminalStatus,
			AnomalyReason: reasonTerminal,
		}
	}

	// ── Hot path: single array lookup ────────────────────────────────────────
	edge := e.adj[packKey(currentStatus, nextStatus, flow)]

	if edge&flagPresent == 0 {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrInvalidTransition,
			AnomalyReason: reasonNotInSpec,
		}
	}

	// ── Override: operator_id required ────────────────────────────────────────
	if edge&flagRequiresOpID != 0 && ctx.OperatorID == "" {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrMissingOperatorID,
			AnomalyReason: reasonMissingOperator,
		}
	}

	// ── Inter-system boundary check ───────────────────────────────────────────
	// The emitting system must own the next status.
	isInterSystem := edge&flagInterSystem != 0
	if isInterSystem {
		if e.owners[nextStatus] != ctx.SourceSystem {
			e.cntInvalid.Add(1)
			return ValidationResult{
				ErrorCode:     ErrInvalidSourceSystem,
				AnomalyReason: reasonWrongOwner,
			}
		}
		e.cntInterSystem.Add(1)
	}

	// ── Success path ──────────────────────────────────────────────────────────
	isOverride := edge&flagOverride != 0
	if isOverride {
		e.cntOverride.Add(1)
	}

	mode := ModeIntraSystem
	if isInterSystem {
		mode = ModeInterSystem
	}

	return ValidationResult{
		Valid:      true,
		IsOverride: isOverride,
		Mode:       mode,
		ErrorCode:  ErrNone,
		// AnomalyReason is "" (zero value) — no allocation.
	}
}

// ValidateInitial checks whether a status is a valid first event for a new
// shipment in the given flow. Used by the Booking System on shipment creation,
// where there is no prior status and Validate() must not be called.
func (e *Engine) ValidateInitial(
	firstStatus status.Status,
	flow Flow,
	ctx TransitionContext,
) ValidationResult {
	e.cntTotal.Add(1)

	if firstStatus == 0 {
		e.cntZeroValue.Add(1)
		return ValidationResult{
			ErrorCode:     ErrZeroValueStatus,
			AnomalyReason: reasonZeroValue,
		}
	}
	if firstStatus > status.MaxID {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrUnknownStatus,
			AnomalyReason: reasonUnknownStatus,
		}
	}
	if flow != FlowForward && flow != FlowReverse {
		e.cntInvalid.Add(1)
		return ValidationResult{
			ErrorCode:     ErrFlowMismatch,
			AnomalyReason: reasonInvalidFlow,
		}
	}

	for _, init := range e.initials {
		if init.statusID == firstStatus && init.flow == flow {
			return ValidationResult{
				Valid:      true,
				Mode:       ModeIntraSystem,
				ErrorCode:  ErrNone,
			}
		}
	}

	e.cntInvalid.Add(1)
	return ValidationResult{
		ErrorCode:     ErrInvalidTransition,
		AnomalyReason: reasonNotInSpec,
	}
}

// PermittedTransitions returns all valid next statuses reachable from
// currentStatus in the given flow. Used by source system UIs to constrain
// operator-facing action menus. Not on the hot path — allocates a slice.
func (e *Engine) PermittedTransitions(
	currentStatus status.Status,
	flow Flow,
) []status.Status {
	if currentStatus == 0 || currentStatus > status.MaxID {
		return nil
	}
	if e.terminals[currentStatus] {
		return nil
	}
	if flow != FlowForward && flow != FlowReverse {
		return nil
	}

	flowBit := uint16(flow - 1)
	var result []status.Status
	for next := status.Status(1); next <= status.MaxID; next++ {
		key := uint16(currentStatus)<<7 | uint16(next)<<1 | flowBit
		if e.adj[key]&flagPresent != 0 {
			result = append(result, next)
		}
	}
	return result
}

// SpecVersion returns the semantic version of the rule spec compiled into this
// engine instance. Source systems must log this on startup and expose it via
// their /health endpoint to enable spec version skew detection.
func (e *Engine) SpecVersion() string {
	return e.specVersion
}

// Snapshot returns a point-in-time copy of all internal counters.
// Call this from your metrics export goroutine (e.g., every 15 seconds).
// This is the only metrics interaction — there are no callbacks on Validate().
func (e *Engine) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ValidationsTotal:       e.cntTotal.Load(),
		InvalidTransitions:     e.cntInvalid.Load(),
		OverrideTransitions:    e.cntOverride.Load(),
		InterSystemTransitions: e.cntInterSystem.Load(),
		ZeroValueRejections:    e.cntZeroValue.Load(),
	}
}
