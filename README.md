# status-rules

A zero-allocation, in-process shipment status transition rule engine for logistics platforms — available in **Go, Node.js, Java, and Python** from a single shared rule spec.

It answers one question: **given a shipment's current status and a proposed next status, is this transition allowed?**

---

## Repository layout

```
status-rule-engine/
├── spec/
│   └── rules.yaml        ← single source of truth (shared by all languages)
│
├── go/                   ← Go package  (github.com/logistics-oss/status-rules)
│   ├── go.mod
│   ├── engine.go
│   ├── engine_internal.go
│   ├── types.go
│   └── status/
│       └── status.go
│
├── node/                 ← Node.js package  (@logistics-oss/status-rules)
│   ├── package.json
│   └── src/
│       ├── index.js
│       ├── engine.js
│       └── status.js
│
├── java/                 ← Java package  (io.logistics:status-rules)
│   ├── pom.xml
│   └── src/main/java/io/logistics/statusrules/
│       ├── Engine.java
│       ├── Status.java
│       └── ...
│
├── python/               ← Python package  (logistics-status-rules)
│   ├── pyproject.toml
│   └── statusrules/
│       ├── engine.py
│       ├── status.py
│       └── ...
│
└── docs/
    ├── demo.html         ← interactive browser demo
    └── statuses.md       ← human-readable status registry
```

Every language package reads `spec/rules.yaml` at runtime — the spec is never copied or duplicated.

---

## Language support

| Language | Package | Import |
|----------|---------|--------|
| Go | `github.com/logistics-oss/status-rules` | `import statusrules "github.com/logistics-oss/status-rules"` |
| Node.js | `@logistics-oss/status-rules` | `const { Engine, Status, Flow } = require('@logistics-oss/status-rules')` |
| Java | `io.logistics:status-rules` | `import io.logistics.statusrules.Engine;` |
| Python | `logistics-status-rules` | `from statusrules import Engine, Status, Flow` |

---

## Quick start

### Go

```go
import (
    statusrules "github.com/logistics-oss/status-rules"
    "github.com/logistics-oss/status-rules/status"
)

engine := statusrules.NewEngine()

result := engine.Validate(
    status.HubInScanned,
    status.HubOutscan,
    statusrules.FlowForward,
    statusrules.TransitionContext{SourceSystem: statusrules.SystemHub},
)
// result.Valid == true
```

### Node.js

```js
const { Engine, Status, Flow, SourceSystem } = require('@logistics-oss/status-rules');

const engine = new Engine();

const result = engine.validate(
    Status.HUB_IN_SCANNED,
    Status.HUB_OUTSCAN,
    Flow.FORWARD,
    { sourceSystem: SourceSystem.HUB }
);
// result.valid === true
```

### Java

```java
import io.logistics.statusrules.*;

Engine engine = Engine.newEngine();

ValidationResult result = engine.validate(
    Status.HUB_IN_SCANNED,
    Status.HUB_OUTSCAN,
    Flow.FORWARD,
    TransitionContext.of(SourceSystem.HUB_SYSTEM)
);
// result.valid() == true
```

### Python

```python
from statusrules import Engine, Status, Flow, SourceSystem, TransitionContext

engine = Engine()

result = engine.validate(
    Status.HUB_IN_SCANNED,
    Status.HUB_OUTSCAN,
    Flow.FORWARD,
    TransitionContext(source_system=SourceSystem.HUB)
)
# result.valid == True
```

---

## API (consistent across all languages)

### `validate(current, next, flow, ctx)`

Checks whether a transition is permitted. The hot path — completes in < 1ms, zero heap allocations on the Go success path.

| Return field | Type | Description |
|---|---|---|
| `valid` | bool | `true` if the transition is permitted |
| `isOverride` | bool | `true` if this is a `FORCE_*` operator override edge |
| `isInterSystem` | bool | `true` if the transition crosses a system ownership boundary |
| `errorCode` | enum/string | `ErrNone` on success; typed error code on failure |
| `anomalyReason` | string | Human-readable reason on failure; empty on success |

#### Error codes

| Code | Meaning |
|------|---------|
| `ErrNone` | Transition is valid |
| `ErrInvalidTransition` | Transition not defined in the spec |
| `ErrTerminalStatus` | Source status is terminal — no outgoing transitions |
| `ErrUnknownStatus` | Status not found in the spec |
| `ErrMissingOperatorID` | `FORCE_*` transition requires `operatorId` |
| `ErrFlowMismatch` | Transition exists but in the opposite flow |
| `ErrInvalidSourceSystem` | Caller's source system doesn't own the next status |
| `ErrZeroValueStatus` | Either status is null / zero / empty |

### `validateInitial(first, flow, ctx)`

For new shipments with no prior status. Use instead of `validate()` on shipment creation.

### `permittedTransitions(current, flow)`

Returns all valid next statuses from `current`. Allocates — not for the hot path. Use to populate operator UI menus.

### `specVersion()`

Returns the spec version string (e.g. `"1.0.0"`). Log on startup and expose via `/health`.

### `snapshot()`

Returns a point-in-time copy of internal counters (`validationsTotal`, `invalidTransitions`, `overrideTransitions`, `interSystemTransitions`, `zeroValueRejections`).

---

## Flows

| Constant | Description |
|----------|-------------|
| `FORWARD` | Normal delivery direction |
| `REVERSE` | Return-to-origin (RTO) direction |

---

## FORCE_* overrides

Operator-gated transitions for manually resolving stuck shipments. They are explicit edges in the spec — not bypasses.

```js
// Node.js example
const result = engine.validate(
    Status.HUB_IN_SCANNED,
    Status.FORCE_HUB_OUTSCAN,
    Flow.FORWARD,
    { sourceSystem: SourceSystem.HUB, operatorId: 'OPS-7842' }
);
// result.isOverride === true
```

Without `operatorId`, the engine returns `ErrMissingOperatorID`.

---

## Exception wildcards

Seven statuses (`DAMAGED`, `MIS_ROUTED`, `REROUTED`, `EXCESS_INSCAN`, `PINCODE_AUDITED`, `WEIGHT_AUDITED`, `MODE_AUDITED`) are reachable from **any** non-terminal status in any flow. They model operational disruptions that can occur at any stage.

---

## Running the tests

### Go
```bash
cd go
go test ./...

# Zero-allocation gate (must pass before merge)
go test -run TestValidate_NoAlloc ./...

# Benchmarks
go test -bench=. -benchmem -count=3
```

### Node.js
```bash
cd node
npm install
npm test
```

### Java
```bash
cd java
mvn test
```

### Python
```bash
cd python
pip install -e ".[dev]"
pytest tests/ -v
```

---

## Updating the rules

All rules live in `spec/rules.yaml`. Every language reads this file at runtime — edit it once and all packages see the change immediately (no codegen step for most changes).

If you add or rename a status, also update the typed status constants in each language:
- Go: `go/status/status.go`
- Node.js: `node/src/status.js`
- Java: `java/src/main/java/io/logistics/statusrules/Status.java`
- Python: `python/statusrules/status.py`

---

## Demo

```bash
python3 -m http.server 8080
# open http://localhost:8080/docs/demo.html
```

---

## Spec versioning

| Bump | When |
|------|------|
| **Patch** | Metadata / description changes only — no graph changes |
| **Minor** | New transitions added — backward compatible |
| **Major** | Existing transitions removed or changed — requires coordinated rollout |

Current spec version: **v1.0.0**

---

## License

MIT
