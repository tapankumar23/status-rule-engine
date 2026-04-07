# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Repo Is

A polyglot, spec-driven shipment status transition rule engine. The same logical engine is implemented in Go, Java, Python, and Node.js, all compiled from a single YAML rule spec at `spec/rules.yaml`. The spec is the source of truth — do not hard-code transitions in library code.

## Commands

### Go
```bash
cd go
go test ./...                         # run all tests
go test -run TestName ./...           # run a single test
go test -bench=. ./...                # run benchmarks
go test -run TestValidate_NoAlloc ./... # zero-alloc CI gate
```

### Java
```bash
cd java
mvn test                              # run all tests
mvn test -Dtest=EngineTest#methodName # run a single test
mvn package                           # build JAR
```

### Python
```bash
cd python
pytest tests/                         # run all tests
pytest tests/test_engine.py::TestClass::test_name  # single test
```

### Node.js
```bash
cd node
node --test test/engine.test.js       # run all tests
```

## Architecture

### Spec → Engine compilation

At startup, each language's `Engine` / `NewEngine()` reads `spec/rules.yaml` (path relative to the source file, bundled in the artifact) and compiles it into an **8192-slot flat array** (`adjacencyMap` / `adj`). This happens once; the resulting engine is immutable and safe for concurrent use.

Key compilation steps:
1. Build a `code → id` map. YAML uses 0-indexed IDs; all runtime IDs are `yaml_id + 1` so that `id=0` is the uninitialized sentinel.
2. Expand `wildcard_targets` — each wildcard status becomes a valid destination from every non-terminal status in both flows.
3. Apply explicit `transitions` entries, which may overwrite wildcard slots (explicit wins).
4. Index `initial_statuses` for `ValidateInitial` / `validate_initial`.

### Adjacency map key packing

```
key = (from_id << 7) | (to_id << 1) | flow_bit
flow_bit: FORWARD=0, REVERSE=1
array size: 8192 (fits all id pairs 1..55 × 1..55 × 2 flows)
```

A zero slot means no valid transition. Each non-zero slot is a `uint16` bitmask:
- `bit 0` — edge present
- `bit 1` — override edge (`FORCE_*`)
- `bit 2` — requires `operator_id` in context
- `bit 3` — crosses system ownership boundary (inter-system)

### Validation logic (all languages implement this identically)

`Validate(current, next, flow, ctx)` guards in order:
1. Zero-value sentinel check
2. Out-of-range status ID check
3. Flow validity check
4. Terminal status check (no outbound edges from terminal)
5. First-mile regression check (FIRST_MILE cannot follow MIDDLE_MILE or LAST_MILE)
6. Adjacency lookup — single array read
7. `operator_id` required check (for `FORCE_*` edges)
8. Inter-system ownership check (`ctx.SourceSystem` must own `nextStatus`)

### Public API surface

| Method | Purpose |
|---|---|
| `Validate` / `validate` | In-flight shipment: check current→next transition |
| `ValidateInitial` / `validate_initial` | New shipment: check first status (no prior state) |
| `PermittedTransitions` / `permitted_transitions` | UI helpers: list valid next statuses from current |
| `SpecVersion` / `spec_version` | Returns spec semver; log on startup, expose in /health |
| `Snapshot` / `snapshot` | Returns atomic counter snapshot for metrics export |

### Generated code — do not edit

`go/status/status.go` is generated from `spec/rules.yaml`. The comment at the top says `DO NOT EDIT`. To regenerate, update `spec/rules.yaml` and run `go generate` in `go/`.

### Key constraints

- **Zero allocation on the `Validate()` success path** — enforced by `TestValidate_NoAlloc_SuccessPath` in Go. Never change this method in a way that causes heap escapes.
- **No network calls on the hot path** — the spec is bundled, not fetched at runtime.
- **Rule changes go in `spec/rules.yaml` only** — never add transition logic to engine source code.
- **Status IDs are `yaml_id + 1`** — the zero value is an intentional uninitialized sentinel across all language implementations.

## Spec schema quick reference

```yaml
spec_version: "1.0.0"
statuses:
  - code: DRAFT
    id: 0              # yaml_id; runtime id = id+1
    category: ORDER_BOOKING
    mile: PRE_MILE
    flows: [FORWARD]
    owned_by: BOOKING_SYSTEM   # BOOKING_SYSTEM | HUB_SYSTEM | DELIVERY_SYSTEM | SYSTEM_3PL
    terminal: false

wildcard_targets: [DAMAGED, MIS_ROUTED, ...]   # valid destination from any non-terminal

initial_statuses:
  - code: DRAFT
    flow: FORWARD

transitions:
  - from: DRAFT
    to: [BOOKED, CANCELLED]    # scalar or list
    flow: FORWARD
    is_override: false
    requires_context: []       # [operator_id] for FORCE_* edges
    crosses_boundary: false    # informational; engine derives this from owned_by
```
