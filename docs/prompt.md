# Architecture: Shipment Status Transition Rule Engine

**Role:** Principal Engineer — Logistics Platform
**Scope:** Shared library embedded in all source systems (Booking, Hub, Delivery, 3PL adapters)
**Hard constraint:** Validation response ≤ 1ms (sub-millisecond, in-process, zero network calls)

---

## 1. Why a Shared Library, Not a Service

The first instinct might be to build a centralized rule service — one API, one deployment, one truth. Reject it.

A network call to a validation service costs 1–5ms on a healthy LAN, 20–200ms under load, and ∞ when the service is down. The sub-millisecond requirement is not negotiable at 50 lakh events/day with hub scanner burst rates. A shared library is the only architecture that satisfies it.

| Option | Latency | Availability risk | Single source of truth |
|---|---|---|---|
| Centralized validation service | 1–200ms + tail latency | Yes — outage blocks all sources | Yes |
| Sidecar process (per pod) | ~0.5ms IPC | Moderate | Harder to sync |
| **Embedded in-process library** | **< 0.1ms (hash lookup)** | **None — no dependency** | **Yes — via versioned spec** |

The canonical artifact is the **rule specification** (a versioned YAML file). The library is just a compiled, in-memory execution engine for that spec. Every source system embeds the same spec version. Truth is in the spec, not in the runtime.

---

## 2. Core Design Principles

1. **Stateless validation** — the library holds no shipment state. The caller passes in the current shipment state and the proposed next status. The library does pure computation and returns a result. No locks, no shared mutable state, no goroutine contention.

2. **Immutable adjacency map** — at startup, the rule spec is compiled once into an immutable hash map of `(from_status, to_status) → TransitionRule`. All validations are O(1) lookups. The map is never mutated after initialization.

3. **Data-driven rules** — transitions are defined in a versioned YAML spec, not in code. Engineers never modify library source to change a rule. This makes rule changes auditable, diffable, and deployable without recompiling source systems.

4. **FORCE_\* statuses are first-class edges** — they are not exemptions from the graph. They are explicit override edges in the transition map, flagged with `is_override: true`. The library validates them like any other transition and tags the result accordingly.

5. **Exception statuses are wildcard edges** — `DAMAGED`, `MIS_ROUTED`, `REROUTED`, `PINCODE_AUDITED`, `WEIGHT_AUDITED`, `MODE_AUDITED`, `EXCESS_INSCAN` are valid *destinations* from any state in any flow. They are modeled as a wildcard target set in the spec, not as individual edges from every node.

6. **Flow-aware** — the graph is partitioned by flow (`FORWARD`, `REVERSE`). The same status code can have different valid successors depending on which flow the shipment is in. The caller declares the current flow.

---

## 3. Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Source System Process               │
│                                                     │
│  ┌──────────────┐       ┌──────────────────────┐   │
│  │  Operational │       │   Rule Engine Lib    │   │
│  │    Logic     │──────▶│  (embedded, in-proc) │   │
│  └──────────────┘       └──────────┬───────────┘   │
│                                    │               │
│                         ┌──────────▼───────────┐   │
│                         │  Immutable Transition │   │
│                         │    Adjacency Map      │   │
│                         │  (loaded at startup)  │   │
│                         └──────────┬────────────┘   │
│                                    │               │
│                         ┌──────────▼───────────┐   │
│                         │  Rule Spec (YAML)     │   │
│                         │  v1.0.0 — bundled     │   │
│                         │  in library artifact  │   │
│                         └───────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

The rule spec is **bundled inside the library artifact** at build time (not fetched at runtime). Rule updates require a new library version release and source system redeployment — this is intentional. It prevents silent drift and ensures that rule changes go through the same deployment pipeline as code changes.

### 3.1 Trade-off: Spec Bundled vs. Startup-Loaded

This is a deliberate choice, not the obvious default. The alternative — loading the spec from a config store (etcd, Consul, S3) at startup and compiling the adjacency map once — still satisfies the sub-millisecond constraint because the spec is fetched once at boot, not per-call. It would allow rule updates via a rolling restart instead of a full library release and deployment pipeline.

| Dimension | Bundled in artifact (chosen) | Loaded from config store at startup |
|---|---|---|
| Rule update cost | Library release + redeploy all 4 systems (multi-day coordination) | Config push + rolling restart per system |
| Drift risk | Zero — build and runtime are always in sync | Low but possible — config store lag, partial rollouts |
| Build-time guarantees | Typos and invalid specs fail the build | Failures surface at service startup, not at build |
| Rollback | Redeploy previous library version | Revert config entry |
| Operational complexity | Higher for updates | Higher for config store ops (HA, auth, versioning) |

**Why bundled is the right call here:** Logistics' status transition rules are operational business rules, not feature flags. They change infrequently — a few times per quarter — and each change must be deliberately coordinated across source system owners and domain experts. The deployment overhead is acceptable given the change frequency. The stronger safety guarantee (invalid spec = build failure, not runtime crash) is worth it. If the business cadence of rule changes increases significantly, migrating to startup-loaded config is a well-understood refactor.

---

## 4. Transition Graph Model

### 4.1 Node

Each node in the graph is a status code from the canonical 55-status schema:

```yaml
status: INSCANNED
category: MIDDLE_MILE
mile: MIDDLE
flow: [FORWARD, REVERSE]
owned_by: HUB_SYSTEM
```

### 4.2 Edge

An edge represents a valid transition:

```yaml
from: MANIFESTED
to: READY_FOR_PICKUP
flow: FORWARD
is_override: false
requires_context: []
```

An override edge:

```yaml
from: INSCANNED
to: HUB-OUTSCAN
flow: FORWARD
is_override: true        # FORCE_HUB_OUTSCAN edge
requires_context:
  - operator_id          # must be present in TransitionContext
```

A cross-system handoff edge — the `from` status is owned by a different system than the `to` status. These edges are marked `crosses_boundary: true` in the spec. The engine uses `owned_by` from the status registry (not this flag directly) to compute `ValidationMode` at runtime; the flag exists for human readability in the spec.

```yaml
from: MANIFESTED          # owned_by: BOOKING_SYSTEM
to: READY_FOR_PICKUP      # owned_by: HUB_SYSTEM
flow: FORWARD
crosses_boundary: true    # inter-system handoff: Booking → Hub
is_override: false
requires_context: []
```

### 4.3 Wildcard Exception Edges

Exception statuses are valid *destinations* from any non-terminal node in any flow — any status can transition *to* them. They are declared as a wildcard target set rather than enumerating N×7 individual edges:

```yaml
wildcard_targets:
  - DAMAGED
  - MIS_ROUTED
  - REROUTED
  - EXCESS_INSCAN
  - PINCODE_AUDITED
  - WEIGHT_AUDITED
  - MODE_AUDITED
```

At adjacency map compile time, each wildcard target is expanded into edges from every non-terminal node. The expansion is done once at startup — it does not affect lookup latency.

### 4.4 Terminal Nodes

Certain statuses are terminal — no forward transition is valid:

```yaml
terminal_statuses:
  - DELIVERED
  - RTO_DELIVERED
  - CANCELLED
  - REJECTED
  - REJECTED_BY_HO
```

A transition *from* a terminal status is always invalid, regardless of the destination.

---

## 5. Library Interface

The public interface is intentionally narrow. Everything else is internal.

### 5.1 Core Types

```go
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
// Flow and SourceSystem are first-class validation inputs and are passed
// as explicit parameters to Validate(), not here.
// This struct carries only auxiliary context: audit info and operator identity.
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
    // The library validates that the edge exists in the spec and that
    // ctx.SourceSystem matches the to-status owner. The tracking anomaly
    // detector is the authoritative validator for these transitions at runtime,
    // since no single source system holds the prior system's confirmed state.
    ModeInterSystem ValidationMode = 2
)

// ValidationResult is the return type of all validation calls.
// Zero allocation on the success path — AnomalyReason is only populated
// on the failure path and is the only field that may escape to the heap.
type ValidationResult struct {
    Valid          bool
    IsOverride     bool           // true if transition is a FORCE_* override edge
    Mode           ValidationMode // intra-system or inter-system boundary crossing
    ErrorCode      ErrorCode
    AnomalyReason  string // non-empty only if Valid=false
}

// ErrorCode uses typed integer constants, not strings, to ensure zero
// allocation on all paths including the failure path.
type ErrorCode uint8
const (
    ErrNone                 ErrorCode = 0
    ErrInvalidTransition    ErrorCode = 1
    ErrTerminalStatus       ErrorCode = 2
    ErrUnknownStatus        ErrorCode = 3
    ErrMissingOperatorID    ErrorCode = 4
    ErrFlowMismatch         ErrorCode = 5
    ErrInvalidSourceSystem  ErrorCode = 6 // ctx.SourceSystem does not own nextStatus
    ErrZeroValueStatus      ErrorCode = 7 // currentStatus or nextStatus is uninitialized
)
```

### 5.2 Validation API

```go
// Validate checks whether transitioning from currentStatus to nextStatus
// is permitted in the given flow and context.
//
// This is the hot path. It must complete in < 1ms.
// It is safe to call from multiple goroutines concurrently.
// It allocates nothing on the heap for the success path.
//
// currentStatus: the shipment's last confirmed status known to the caller.
//                The caller is responsible for providing this correctly.
//                The library does not store or retrieve shipment state.
//                Passing a zero-value Status returns ErrZeroValueStatus immediately —
//                callers that have no prior status must use ValidateInitial instead.
// nextStatus:    the status the source system intends to emit.
//                Passing a zero-value Status returns ErrZeroValueStatus immediately.
// flow:          the current shipment flow (FORWARD or REVERSE). This is a
//                first-class validation input, not auxiliary context.
// ctx:           auxiliary metadata — source system identity, operator ID for overrides.
//
// The result's Mode field indicates whether the transition is intra-system
// (both statuses owned by ctx.SourceSystem) or inter-system (crosses ownership
// boundary). Inter-system transitions are validated for structural correctness
// only; the tracking anomaly detector holds runtime authority for these.
func (e *Engine) Validate(
    currentStatus Status,
    nextStatus    Status,
    flow          Flow,
    ctx           TransitionContext,
) ValidationResult

// ValidateInitial checks whether a status is a valid first event for a new
// shipment in the given flow. Used by the Booking System on shipment creation,
// where there is no prior status and Validate() must not be called.
func (e *Engine) ValidateInitial(
    firstStatus Status,
    flow        Flow,
    ctx         TransitionContext,
) ValidationResult

// PermittedTransitions returns all valid next statuses reachable from
// currentStatus in the given flow. Used by source system UIs to constrain
// operator-facing action menus. Not on the hot path.
func (e *Engine) PermittedTransitions(
    currentStatus Status,
    flow          Flow,
) []Status

// SpecVersion returns the semantic version of the rule spec compiled
// into this engine instance. Source systems must log this on startup
// and expose it via their /health endpoint.
func (e *Engine) SpecVersion() string
```

### 5.3 Intra-System vs. Inter-System Validation

The library distinguishes two validation modes based on the `owned_by` field of the `from` and `to` statuses in the spec:

**Intra-system transition** (`ModeIntraSystem`): both `currentStatus` and `nextStatus` are owned by `ctx.SourceSystem`. The source system has full operational context and full authority to block the action. Example: Hub emitting `HUB-OUTSCAN` after `HUB-IN-SCANNED` — both are Hub System statuses.

**Inter-system transition** (`ModeInterSystem`): `currentStatus` is owned by a different system than `ctx.SourceSystem`. This is a system handoff. Example: Hub emitting `READY_FOR_PICKUP` after `MANIFESTED` — `MANIFESTED` is a Booking System status. The Hub does not hold the Booking System's confirmed current state; it passes in the status it expects the shipment to be in based on the operational handoff protocol.

For inter-system transitions, the library:
1. Validates the edge exists in the spec (the transition is structurally defined)
2. Validates that `ctx.SourceSystem` matches the `to` status's `owned_by` (the emitting system must own the status it is emitting)
3. Returns `Mode: ModeInterSystem` so the caller can treat this as a best-effort check

The tracking service's anomaly detector is the authoritative validator for inter-system transitions at runtime — it holds the complete timeline and can detect if the actual prior state conflicts with what the source system assumed. Source systems do not need to query tracking before calling `Validate()`; that would re-introduce the network latency the library design eliminates.

### 5.4 Engine Initialization

```go
// NewEngine initializes the rule engine from the spec bundled in the
// library. Called once at application startup. Panics if the bundled
// spec fails to parse — this is a build-time invariant violation.
//
// Initialization time: ~5ms (adjacency map compile + wildcard expansion).
// The returned *Engine is immutable and safe for concurrent use.
func NewEngine() *Engine

// NewEngineFromSpec initializes from an externally provided spec.
// Used in tests and for canary validation of new rule spec versions
// before release. Not for production initialization.
func NewEngineFromSpec(specYAML []byte) (*Engine, error)
```

---

## 6. Internal: Adjacency Map

The adjacency map is the sole data structure used on the hot path.

```
Map key:   (from_status_id, to_status_id, flow)  — packed into a uint16 at compile time
Map value: TransitionEdge { is_override bool, mode ValidationMode, required_context []ContextKey }

Lookup:    adjacencyMap[pack(from, to, flow)]  →  O(1), single array index
Miss:      zero value at index → invalid transition
Hit:       non-zero value → valid; inspect edge for override/mode/context requirements
```

**Why fixed array with packed key?** Status codes are assigned sequential integer IDs (0–54) at spec compile time. Flow is a 1-bit flag. The triple `(from_id, to_id, flow)` is packed into `from_id<<7 | to_id<<1 | flow_bit`. With 55 statuses (IDs 0–54, requiring 6 bits each), the maximum packed value is `54<<7 | 54<<1 | 1 = 6912 + 108 + 1 = 7021` — a **13-bit value**. The array is therefore `[8192]TransitionEdge`, sized to the next power of two above 7021. Array indexing is a single memory read with no hashing, no collision resolution, no pointer chasing.

**Cache footprint:** `8192 × 2 bytes = 16KB`. This fits comfortably in L1 cache (typically 32–64KB per core) on any modern CPU. After the first access, all lookups are served from L1. This is the primary reason for the fixed array design over a hash map — cache line predictability, not just O(1) complexity.

This is the reason validation is sub-millisecond even under contention: a single L1-cache array read, with no allocation, no locking, no pointer chasing.

---

## 7. Rule Specification Schema

The canonical YAML spec that all library versions are compiled from:

```yaml
spec_version: "1.0.0"
spec_date: "2026-03-24"

# ── Status registry ──────────────────────────────────────────────────────────
statuses:
  - code: DRAFT
    id: 0
    category: ORDER_BOOKING
    mile: PRE_MILE
    flows: [FORWARD]
    owned_by: BOOKING_SYSTEM
    terminal: false

  - code: BOOKED
    id: 1
    category: ORDER_BOOKING
    mile: PRE_MILE
    flows: [FORWARD]
    owned_by: BOOKING_SYSTEM
    terminal: false

  # ... all 55 statuses

  - code: DELIVERED
    id: 30
    category: LAST_MILE
    mile: LAST_MILE
    flows: [FORWARD]
    owned_by: DELIVERY_SYSTEM
    terminal: true

# ── Exception wildcards ───────────────────────────────────────────────────────
# These statuses are valid as a next status from ANY non-terminal status.
wildcard_targets:
  - DAMAGED
  - MIS_ROUTED
  - REROUTED
  - EXCESS_INSCAN
  - PINCODE_AUDITED
  - WEIGHT_AUDITED
  - MODE_AUDITED

# ── Transitions ───────────────────────────────────────────────────────────────
transitions:
  # Pre-Mile / Booking
  - from: DRAFT
    to: [BOOKED, CANCELLED, REJECTED, REJECTED_BY_HO]
    flow: FORWARD

  - from: BOOKED
    to: [PART_PAYMENT_PENDING, UPDATED_BOOKING, MANIFESTED, CANCELLED]
    flow: FORWARD

  - from: PART_PAYMENT_PENDING
    to: [BOOKED, CANCELLED]
    flow: FORWARD

  - from: UPDATED_BOOKING
    to: [MANIFESTED, CANCELLED]
    flow: FORWARD

  - from: MANIFESTED
    to: [READY_FOR_PICKUP, CANCELLED]
    flow: FORWARD

  # First Mile
  - from: READY_FOR_PICKUP
    to: [INSCANNED, HUB-IN-SCANNED]
    flow: FORWARD

  # Middle Mile — normal
  - from: INSCANNED
    to: [HUB-IN-SCANNED, BAGGED, BAG_CREATED, INSCANNED_AT_TRANSIT, OUT_SCAN_TO_CP, OUT_SCAN_TO_3PL]
    flow: FORWARD

  - from: HUB-IN-SCANNED
    to: [BAGGED, BAG_CREATED, HUB-OUTSCAN, REMOVED_FROM_BAG, REMOVED_FROM_LCR]
    flow: FORWARD

  - from: HUB-OUTSCAN
    to: [INSCANNED_AT_TRANSIT, INSCANNED_AT_CP, IN-BAG-INSCAN, OUT_SCAN_TO_CP, OUT_SCAN_TO_3PL]
    flow: FORWARD

  - from: BAGGED
    to: [BAG_FINALISED, REMOVED_FROM_BAG, IN-BAG-OUTSCAN, IN_BAG_OUTSCAN_TO_CP]
    flow: FORWARD

  - from: BAG_CREATED
    to: [BAG_FINALISED, BAG_DELETED]
    flow: FORWARD

  - from: BAG_FINALISED
    to: [IN-BAG-OUTSCAN, IN_BAG_OUTSCAN_TO_CP]
    flow: FORWARD

  - from: INSCANNED_AT_TRANSIT
    to: [HUB-OUTSCAN, HUB-IN-SCANNED, BAGGED]
    flow: FORWARD

  # Middle Mile — FORCE_* override edges
  - from: HUB-IN-SCANNED
    to: FORCE_HUB_OUTSCAN
    flow: FORWARD
    is_override: true
    requires_context: [operator_id]

  - from: BAGGED
    to: FORCE_BAG
    flow: FORWARD
    is_override: true
    requires_context: [operator_id]

  - from: FORCE_BAG
    to: FORCE_BAG_ATTEMPTED
    flow: FORWARD
    is_override: true
    requires_context: [operator_id]

  - from: HUB-OUTSCAN
    to: FORCE_OUTSCAN_TO_CP
    flow: FORWARD
    is_override: true
    requires_context: [operator_id]

  # Last Mile
  - from: INSCANNED_AT_CP
    to: [SCHEDULED_FOR_TRIP, OUT_FOR_DELIVERY]
    flow: FORWARD

  - from: SCHEDULED_FOR_TRIP
    to: [OUT_FOR_DELIVERY, REMOVED_FROM_LCR]
    flow: FORWARD

  - from: OUT_FOR_DELIVERY
    to: [DELIVERED, ATTEMPTED, UNDELIVERED]
    flow: FORWARD

  - from: ATTEMPTED
    to: [OUT_FOR_DELIVERY, UNDELIVERED, RETURN_INITIATED]
    flow: FORWARD

  - from: UNDELIVERED
    to: [OUT_FOR_DELIVERY, RETURN_INITIATED, INSCANNED_AT_CP]
    flow: FORWARD

  # RTO / Reverse flow
  - from: RETURN_INITIATED
    to: [RTO, RETURN_REVOKED]
    flow: REVERSE

  - from: RETURN_REVOKED
    to: [OUT_FOR_DELIVERY]
    flow: FORWARD

  - from: RTO
    to: [RTO_OUT_FOR_DELIVERY, HUB-IN-SCANNED, INSCANNED]
    flow: REVERSE

  - from: RTO_OUT_FOR_DELIVERY
    to: [RTO_DELIVERED, RTO_UNDELIVERED]
    flow: REVERSE

  - from: RTO_UNDELIVERED
    to: [RTO_OUT_FOR_DELIVERY, HUB-IN-SCANNED]
    flow: REVERSE

  # 3PL
  - from: OUT_SCAN_TO_3PL
    to: [3PL_ITEM_BOOK, 3PL_BAG_CLOSE]
    flow: FORWARD

  - from: 3PL_ITEM_BOOK
    to: [3PL_ITEM_DELIVERY, 3PL_ITEM_ONHOLD, 3PL_ITEM_REDIRECT, 3PL_ITEM_RETURN]
    flow: FORWARD

  - from: 3PL_BAG_CLOSE
    to: [3PL_BAG_DISPATCH, 3PL_BAG_OPEN]
    flow: FORWARD

  # 3PL_BAG_DISPATCH → 3PL_BAG_OPEN models a dispatched bag arriving at a
  # downstream 3PL hub and being opened there for item processing. This is
  # NOT a loop — it represents inbound receipt at the next facility, not a
  # re-open of the same bag at the origin. Reviewers: this edge is intentional.
  - from: 3PL_BAG_DISPATCH
    to: [3PL_BAG_OPEN]
    flow: FORWARD

  - from: 3PL_BAG_OPEN
    to: [3PL_ITEM_BOOK]
    flow: FORWARD

  - from: 3PL_ITEM_ONHOLD
    to: [3PL_ITEM_BOOK, 3PL_ITEM_RETURN]
    flow: FORWARD

  - from: 3PL_ITEM_REDIRECT
    to: [3PL_ITEM_BOOK, 3PL_ITEM_DELIVERY]
    flow: FORWARD

  - from: 3PL_ITEM_RETURN
    to: [INSCANNED, HUB-IN-SCANNED]
    flow: REVERSE
```

---

## 8. Performance Design

### 8.1 Benchmark targets

| Operation | Target | Mechanism |
|---|---|---|
| `Validate()` hot path | **< 100µs p99** | Fixed array lookup, zero allocation |
| `Validate()` cold start (first call after init) | < 500µs | Array fits in L2 cache after first access |
| `NewEngine()` startup | < 10ms | One-time map compile; never on hot path |
| `PermittedTransitions()` | < 200µs | Iterate array slice; not on hot path |

### 8.2 Zero-allocation guarantee

The `Validate()` call must not allocate on the heap on the success path. This is enforced in the CI pipeline via `go test -run TestValidate_NoAlloc` with `testing.AllocsPerRun`. Any change that introduces an allocation on the success path fails the build.

### 8.3 Cache locality

The adjacency array is 8,192 × 2 bytes = 16KB. This fits in L1 cache (typically 32–64KB per core) on any modern CPU — not just L2. After warmup (first ~10 calls), all lookups are served from L1. This is the primary reason for the fixed array design over a hash map — cache line predictability and L1 residency, not just O(1) complexity.

### 8.4 Concurrency

`*Engine` is fully immutable after `NewEngine()` returns. No locks. No atomics. No synchronization overhead. Thousands of goroutines can call `Validate()` concurrently with zero contention.

---

## 9. Polyglot Distribution Strategy

Logistics source systems are written in multiple languages (Go for Hub, Java for Booking, Kotlin/Java for Delivery apps, Python for 3PL adapters). The rule engine must be available in all of them.

### 9.1 The canonical artifact

The **rule spec YAML** is the single source of truth. It lives in a dedicated repository: `logistics-oss/status-rule-spec`. All language libraries are generated from — or validated against — this spec at build time.

### 9.2 Language support matrix

| Source System | Language | Library form | Distribution |
|---|---|---|---|
| Hub System | Go | Go module | `github.com/logistics-oss/status-rules` |
| Booking System | Java | JAR | Maven internal registry |
| Delivery System | Kotlin/Java | JAR | Maven internal registry |
| 3PL Adapters | Python | Package | PyPI internal registry |

Each library bundles the spec YAML as an embedded resource. The spec YAML is the same file across all libraries — it is a build-time dependency of each, pulled from the spec repository at library build time, not at source system build time.

### 9.3 Code generation

The Go, Java, and Python libraries all use a shared **code generator** (`logistics-oss/status-rules-codegen`) that:
1. Reads the spec YAML
2. Generates strongly-typed status enums in each target language (no stringly-typed status codes in source systems)
3. Generates the adjacency map initialization code
4. Runs as part of the spec repository's CI — any spec change triggers regeneration and a PR to each library repository

This means source systems never import string literals like `"DELIVERED"` — they import `status.Delivered`, which is a generated typed constant. Typos are compile errors.

---

## 10. Rule Versioning and Governance

### 10.1 Versioning

The spec follows **semantic versioning**:

- **Patch** (2.4.x): Fixes to descriptions, metadata corrections — no transition graph changes
- **Minor** (2.x.0): New valid transitions added (backward compatible — source systems gain new capabilities)
- **Major** (x.0.0): Transitions removed or changed (breaking — requires coordinated source system update)

### 10.2 Change process

1. Engineer opens a PR against `logistics-oss/status-rule-spec`
2. PR must include: the spec diff, a justification comment, and a test case proving the new/changed transition
3. Required approvers: **domain owner of the affected source system** + **one Platform team member**
4. Merge triggers: spec validation CI → code generation → library PR creation for all language targets
5. Library PRs are auto-merged if all tests pass (patch/minor); manual approval required (major)

### 10.3 Spec version enforcement in tracking

The tracking ingestion layer reads the `spec_version` field from every incoming canonical event. Events carrying a spec version older than `(current - 2 minor versions)` are flagged with `stale_spec_version: true` in the event record and trigger an alert to the source system owner's on-call. They are **never rejected** — stale spec events still contain valid data.

---

## 11. Integration Pattern for Source Systems

### 11.1 Standard integration

```go
// At application startup — once
engine := statusrules.NewEngine()
log.Infof("Status rule engine initialized. Spec version: %s", engine.SpecVersion())

// On every status-emitting action — intra-system example (Hub emitting HUB-OUTSCAN)
func (s *HubService) EmitOutscan(shipment *Shipment, operatorID string) error {
    result := engine.Validate(
        shipment.CurrentStatus,  // last Hub-confirmed status for this shipment
        status.HubOutscan,
        statusrules.FlowForward, // flow is a first-class param, not in context
        statusrules.TransitionContext{
            AWB:          shipment.AWB,
            SourceSystem: statusrules.SystemHub,
            OperatorID:   operatorID,
        },
    )

    if !result.Valid {
        return fmt.Errorf("invalid transition %s → %s: %s",
            shipment.CurrentStatus, status.HubOutscan, result.AnomalyReason)
    }

    if result.IsOverride {
        s.auditLog.RecordOverride(shipment.AWB, status.HubOutscan, operatorID)
    }

    // result.Mode == ModeIntraSystem here; no special handling needed.
    return s.eventBus.Emit(shipment.AWB, status.HubOutscan)
}

// Inter-system handoff example: Hub emitting READY_FOR_PICKUP after receiving
// a shipment from the Booking System (MANIFESTED is the expected prior state).
func (s *HubService) ConfirmPickup(shipment *Shipment) error {
    result := engine.Validate(
        status.Manifested,        // the expected handoff state from Booking System
        status.ReadyForPickup,
        statusrules.FlowForward,
        statusrules.TransitionContext{
            AWB:          shipment.AWB,
            SourceSystem: statusrules.SystemHub,
        },
    )

    if !result.Valid {
        return fmt.Errorf("invalid handoff transition: %s", result.AnomalyReason)
    }

    // result.Mode == ModeInterSystem: Hub has validated structural correctness.
    // The tracking anomaly detector will cross-check against the actual prior
    // state on the timeline. Log the mode so ops can correlate if anomalies fire.
    if result.Mode == statusrules.ModeInterSystem {
        s.logger.Info("inter-system handoff validated",
            "awb", shipment.AWB, "from", status.Manifested, "to", status.ReadyForPickup)
    }

    return s.eventBus.Emit(shipment.AWB, status.ReadyForPickup)
}
```

### 11.2 What source systems do with an invalid result

| Source System | On invalid transition | Rationale |
|---|---|---|
| Booking System | Block action, return error to caller | Synchronous, transactional — user can correct |
| Hub System | Block scan, surface error on scanner UI | Operator-driven; error is actionable in real time |
| Delivery System (online) | Block action, surface error in agent app | Agent can radio in for manual override |
| Delivery System (offline) | **Accept locally, flag for review on sync** | Cannot reject — agent may be unreachable; tracking logs anomaly on receipt |
| 3PL Adapter | Reject with 422, log partner-specific error | Partner has time to resend; we do not buffer invalid partner events |

The Delivery System offline exception is the only case where a source system cannot enforce the rule at emit time. This is why the tracking processing layer's anomaly detection exists — it catches what source systems couldn't.

---

## 12. Testing Strategy

### 12.1 Spec contract tests

Every transition listed in the spec has a generated contract test:
- **Positive test**: `Validate(from, to, flow)` returns `Valid: true`
- **Negative test**: `Validate(from, invalid_to, flow)` returns `Valid: false`

These are generated automatically from the spec YAML. They run in < 50ms total (pure in-memory).

### 12.2 Property tests

Fuzz-tested invariants, run in CI:
- Terminal statuses are never valid `from` nodes
- Exception wildcard statuses are valid `to` from every non-terminal node
- `FORCE_*` transitions always return `IsOverride: true`
- `FORCE_*` transitions without `operator_id` in context always return `ErrMissingOperatorID`
- No status valid in `REVERSE` flow appears as a transition in `FORWARD` flow (and vice versa, for flow-exclusive statuses)

### 12.3 Benchmark tests

```
BenchmarkValidate_HotPath-16         50000000     21 ns/op    0 B/op    0 allocs/op
BenchmarkValidate_InvalidTransition  50000000     19 ns/op    0 B/op    0 allocs/op
BenchmarkValidate_ForceOverride      50000000     22 ns/op    0 B/op    0 allocs/op
BenchmarkNewEngine                       1000   4800000 ns/op
```

The `0 allocs/op` on all `Validate` benchmarks is a CI-enforced invariant. Any PR that regresses this fails the build.

### 12.4 Cross-language parity tests

A test suite in the spec repository runs the same 500+ transition test cases against all language implementations using a shared JSON test fixture. All must produce identical results. A divergence in any language is a build failure.

---

## 13. Observability

### 13.1 Why not a MetricsReporter callback

The obvious design — inject a `MetricsReporter` interface and call it on every `Validate()` invocation — violates the zero-allocation guarantee. An interface call passes `ValidationResult` into an `interface{}` parameter, which can cause the struct to escape to the heap depending on the implementation. Even a no-op reporter adds a vtable dispatch on every hot-path call. At 50 lakh events/day, this is not acceptable.

### 13.2 Atomic counter approach

The library maintains internal atomic counters. Source systems read and export them on their own schedule via a `Snapshot()` call — entirely off the hot path.

```go
// Snapshot returns a point-in-time copy of all internal counters.
// Call this from your metrics export goroutine (e.g., every 15 seconds).
// This is the only metrics interaction — there are no callbacks on Validate().
func (e *Engine) Snapshot() MetricsSnapshot

type MetricsSnapshot struct {
    ValidationsTotal        uint64
    InvalidTransitions      uint64
    OverrideTransitions     uint64
    InterSystemTransitions  uint64
    ZeroValueRejections     uint64
}
```

Internal counters use `sync/atomic` adds — a single `LOCK XADD` instruction, no heap allocation, no function call overhead on the hot path.

Source systems export the snapshot delta to their own metrics backend (Prometheus, StatsD, etc.) in a background goroutine. The library has no dependency on any metrics system.

### 13.3 Recommended dashboards

| Metric | Alert threshold |
|---|---|
| `validation_invalid_rate` by source system | > 0.1% of calls → page source system owner |
| `validation_override_rate` by source system | > 5% of calls → page ops lead |
| `inter_system_transition_rate` | Sudden spike → investigate cross-system handoff failures |
| `spec_version_skew` (sources on old spec) | Any source > 2 minor versions behind → warn |
| `validation_p99_latency` | > 500µs → investigate CPU contention |

---

*This library is load-bearing infrastructure. Every status event emitted by every source system passes through it. Changes to the rule spec require the same rigour as schema migrations: backward compatibility analysis, coordinated rollout, and rollback planning.*
