package statusrules

// Flow represents the operational flow of a shipment.
type Flow uint8

const (
	FlowForward Flow = 1
	FlowReverse Flow = 2
)

// SourceSystem identifies which operational system is emitting the next status.
// The library uses this to determine whether a transition crosses a system
// ownership boundary (inter-system) or stays within one (intra-system).
type SourceSystem uint8

const (
	SystemBooking  SourceSystem = 1
	SystemHub      SourceSystem = 2
	SystemDelivery SourceSystem = 3
	System3PL      SourceSystem = 4
)

// TransitionContext carries auxiliary metadata for a validation call.
// Flow is a first-class validation input passed as an explicit parameter to Validate().
// This struct carries only auxiliary context: source system identity, audit info,
// and operator identity for FORCE_* overrides.
type TransitionContext struct {
	AWB          string       // for audit logging only; not used in transition logic
	SourceSystem SourceSystem // which system is emitting nextStatus
	OperatorID   string       // required for FORCE_* transitions
}

// ValidationMode indicates whether the validated transition stays within
// one source system's ownership domain or crosses into another's.
type ValidationMode uint8

const (
	// ModeIntraSystem: both from and to statuses are owned by the same source system.
	// Full enforcement applies; the source system has full authority to block.
	ModeIntraSystem ValidationMode = 1

	// ModeInterSystem: the transition crosses a system ownership boundary
	// (e.g., MANIFESTED→READY_FOR_PICKUP crosses Booking→Hub).
	// The library validates structural correctness and that ctx.SourceSystem
	// matches the to-status owner. The tracking anomaly detector is the
	// authoritative validator for these transitions at runtime.
	ModeInterSystem ValidationMode = 2
)

// ErrorCode uses typed integer constants to ensure zero allocation on all
// paths including the failure path. Never use string comparisons on ErrorCode.
type ErrorCode uint8

const (
	ErrNone                ErrorCode = 0
	ErrInvalidTransition   ErrorCode = 1
	ErrTerminalStatus      ErrorCode = 2
	ErrUnknownStatus       ErrorCode = 3
	ErrMissingOperatorID   ErrorCode = 4
	ErrFlowMismatch        ErrorCode = 5
	ErrInvalidSourceSystem ErrorCode = 6 // ctx.SourceSystem does not own nextStatus
	ErrZeroValueStatus     ErrorCode = 7 // currentStatus or nextStatus is uninitialized
)

// ValidationResult is the return type of all validation calls.
// Returned by value. Zero allocation on the success path —
// AnomalyReason is only populated on the failure path.
type ValidationResult struct {
	Valid         bool
	IsOverride    bool           // true if transition is a FORCE_* override edge
	Mode          ValidationMode // intra-system or inter-system boundary crossing
	ErrorCode     ErrorCode
	AnomalyReason string // non-empty only if Valid=false; always a string literal (no heap alloc)
}

// MetricsSnapshot is a point-in-time copy of the engine's internal counters.
// Retrieve via Engine.Snapshot() from a background goroutine — never from the hot path.
type MetricsSnapshot struct {
	ValidationsTotal       uint64
	InvalidTransitions     uint64
	OverrideTransitions    uint64
	InterSystemTransitions uint64
	ZeroValueRejections    uint64
}
