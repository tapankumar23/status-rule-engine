package statusrules

import (
	"fmt"
	"strings"

	"github.com/logistics-oss/status-rules/status"
	"gopkg.in/yaml.v3"
)

// ── Adjacency map ─────────────────────────────────────────────────────────────
//
// The adjacency map is a fixed array indexed by a packed (from, to, flow) key.
//
// Key packing: uint16(from)<<7 | uint16(to)<<1 | flowBit
//   - from, to: status.Status values in range [1, 55]
//   - flowBit:  0 = FORWARD, 1 = REVERSE
//   - Max key:  55<<7 | 55<<1 | 1 = 7151 < 8192
//
// Array size [8192] (next power of two above 7151) — 8192 × 2 bytes = 16 KB.
// Fits in L1 cache (typically 32–64 KB per core). All hot-path lookups are
// served from L1 after warmup.
//
// A zero value means "no valid transition". The flagPresent bit distinguishes
// a valid entry from the zero default.

const adjMapSize = 8192

type adjacencyMap [adjMapSize]uint16

// Edge flag bits packed into each uint16 slot.
const (
	flagPresent      uint16 = 1 << 0 // transition is defined in the spec
	flagOverride     uint16 = 1 << 1 // FORCE_* override edge (is_override: true)
	flagRequiresOpID uint16 = 1 << 2 // requires operator_id in TransitionContext
	flagInterSystem  uint16 = 1 << 3 // crosses system ownership boundary
)

// packKey packs (from, to, flow) into a uint16 array index.
// Preconditions: from and to are in [1,55]; flow is FlowForward or FlowReverse.
func packKey(from, to status.Status, flow Flow) uint16 {
	flowBit := uint16(flow - 1) // Forward(1)→0, Reverse(2)→1
	return uint16(from)<<7 | uint16(to)<<1 | flowBit
}

// ── Internal engine state ─────────────────────────────────────────────────────

// ownerArray maps Status → SourceSystem. Index = status.Status value (1–55).
type ownerArray [56]SourceSystem

// terminalArray maps Status → bool. Index = status.Status value (1–55).
type terminalArray [56]bool

// initialEntry records a valid (status, flow) pair for ValidateInitial.
type initialEntry struct {
	statusID status.Status
	flow     Flow
}

// ── Spec YAML types ───────────────────────────────────────────────────────────

type specFile struct {
	SpecVersion     string           `yaml:"spec_version"`
	SpecDate        string           `yaml:"spec_date"`
	Statuses        []specStatus     `yaml:"statuses"`
	WildcardTargets []string         `yaml:"wildcard_targets"`
	InitialStatuses []specInitial    `yaml:"initial_statuses"`
	Transitions     []specTransition `yaml:"transitions"`
}

type specStatus struct {
	Code     string   `yaml:"code"`
	ID       int      `yaml:"id"`
	Category string   `yaml:"category"`
	Mile     string   `yaml:"mile"`
	Flows    []string `yaml:"flows"`
	OwnedBy  string   `yaml:"owned_by"`
	Terminal bool     `yaml:"terminal"`
}

type specInitial struct {
	Code string `yaml:"code"`
	Flow string `yaml:"flow"`
}

type specTransition struct {
	From            string        `yaml:"from"`
	To              toField       `yaml:"to"`
	Flow            string        `yaml:"flow"`
	IsOverride      bool          `yaml:"is_override"`
	RequiresContext []string      `yaml:"requires_context"`
	CrossesBoundary bool          `yaml:"crosses_boundary"`
}

// toField handles both scalar and sequence YAML values for the "to" field.
type toField []string

func (t *toField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*t = []string{value.Value}
	case yaml.SequenceNode:
		var ss []string
		if err := value.Decode(&ss); err != nil {
			return err
		}
		*t = ss
	default:
		return fmt.Errorf("expected string or sequence for 'to', got kind %v", value.Kind)
	}
	return nil
}

// ── Spec compilation ──────────────────────────────────────────────────────────

// compileSpec parses the YAML spec and compiles it into an Engine.
// Returns an error if the spec is structurally invalid.
// Called once at startup — not on the hot path.
func compileSpec(specYAML []byte) (*Engine, error) {
	var spec specFile
	if err := yaml.Unmarshal(specYAML, &spec); err != nil {
		return nil, fmt.Errorf("status-rules: parse spec YAML: %w", err)
	}
	if spec.SpecVersion == "" {
		return nil, fmt.Errorf("status-rules: spec_version is missing")
	}

	// ── Build status registry ─────────────────────────────────────────────────
	// YAML IDs are 0-indexed. Go Status values are YAML_id + 1 so that Status(0)
	// remains the zero/uninitialized sentinel.

	codeToStatus := make(map[string]status.Status, len(spec.Statuses))
	var owners ownerArray
	var terminals terminalArray
	var firstMile [56]bool
	var postFirstMile [56]bool

	for _, s := range spec.Statuses {
		if s.ID < 0 || s.ID > int(status.MaxID)-1 {
			return nil, fmt.Errorf("status-rules: status %q has out-of-range id %d", s.Code, s.ID)
		}
		goID := status.Status(s.ID + 1)

		if _, exists := codeToStatus[s.Code]; exists {
			return nil, fmt.Errorf("status-rules: duplicate status code %q", s.Code)
		}
		codeToStatus[s.Code] = goID

		sys, err := parseSourceSystem(s.OwnedBy)
		if err != nil {
			return nil, fmt.Errorf("status-rules: status %q: %w", s.Code, err)
		}
		owners[goID] = sys

		if s.Terminal {
			terminals[goID] = true
		}

		switch strings.ToUpper(s.Category) {
		case "FIRST_MILE":
			firstMile[goID] = true
		case "MIDDLE_MILE", "LAST_MILE":
			postFirstMile[goID] = true
		}
	}

	// ── Validate and index wildcard targets ───────────────────────────────────

	wildcardSet := make(map[status.Status]bool, len(spec.WildcardTargets))
	for _, code := range spec.WildcardTargets {
		id, ok := codeToStatus[code]
		if !ok {
			return nil, fmt.Errorf("status-rules: wildcard_target %q not in status registry", code)
		}
		if terminals[id] {
			return nil, fmt.Errorf("status-rules: wildcard_target %q is terminal", code)
		}
		wildcardSet[id] = true
	}

	// ── Compile adjacency map ─────────────────────────────────────────────────

	var adj adjacencyMap

	// Expand wildcard targets: every non-terminal status can transition to every
	// wildcard target in both flows. Done once at compile time.
	for _, s := range spec.Statuses {
		fromID := codeToStatus[s.Code]
		if terminals[fromID] {
			continue
		}
		for toID := range wildcardSet {
			for _, flow := range []Flow{FlowForward, FlowReverse} {
				flags := flagPresent
				if owners[fromID] != owners[toID] {
					flags |= flagInterSystem
				}
				key := packKey(fromID, toID, flow)
				adj[key] = flags
			}
		}
	}

	// Explicit transitions from the spec (may overwrite wildcard entries if a
	// wildcard target also appears as an explicit 'to' — explicit wins).
	for _, t := range spec.Transitions {
		fromID, ok := codeToStatus[t.From]
		if !ok {
			return nil, fmt.Errorf("status-rules: transition from unknown status %q", t.From)
		}
		if terminals[fromID] {
			return nil, fmt.Errorf("status-rules: transition from terminal status %q", t.From)
		}

		flow, err := parseFlow(t.Flow)
		if err != nil {
			return nil, fmt.Errorf("status-rules: transition from %q: %w", t.From, err)
		}
		flowBit := uint16(flow - 1)

		requiresOpID := false
		for _, ctx := range t.RequiresContext {
			if ctx == "operator_id" {
				requiresOpID = true
			}
		}

		for _, toCode := range t.To {
			toID, ok := codeToStatus[toCode]
			if !ok {
				return nil, fmt.Errorf("status-rules: transition from %q to unknown status %q", t.From, toCode)
			}

			flags := flagPresent
			if t.IsOverride {
				flags |= flagOverride
			}
			if requiresOpID {
				flags |= flagRequiresOpID
			}
			if owners[fromID] != owners[toID] {
				flags |= flagInterSystem
			}

			key := uint16(fromID)<<7 | uint16(toID)<<1 | flowBit
			adj[key] = flags
		}
	}

	// ── Initial statuses ──────────────────────────────────────────────────────

	initials := make([]initialEntry, 0, len(spec.InitialStatuses))
	for _, init := range spec.InitialStatuses {
		id, ok := codeToStatus[init.Code]
		if !ok {
			return nil, fmt.Errorf("status-rules: initial_status %q not in registry", init.Code)
		}
		flow, err := parseFlow(init.Flow)
		if err != nil {
			return nil, fmt.Errorf("status-rules: initial_status %q: %w", init.Code, err)
		}
		initials = append(initials, initialEntry{statusID: id, flow: flow})
	}

	return &Engine{
		adj:           adj,
		owners:        owners,
		terminals:     terminals,
		firstMile:     firstMile,
		postFirstMile: postFirstMile,
		initials:      initials,
		specVersion:   spec.SpecVersion,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseFlow(s string) (Flow, error) {
	switch strings.ToUpper(s) {
	case "FORWARD":
		return FlowForward, nil
	case "REVERSE":
		return FlowReverse, nil
	default:
		return 0, fmt.Errorf("unknown flow %q (expected FORWARD or REVERSE)", s)
	}
}

func parseSourceSystem(s string) (SourceSystem, error) {
	switch strings.ToUpper(s) {
	case "BOOKING_SYSTEM":
		return SystemBooking, nil
	case "HUB_SYSTEM":
		return SystemHub, nil
	case "DELIVERY_SYSTEM":
		return SystemDelivery, nil
	case "SYSTEM_3PL":
		return System3PL, nil
	default:
		return 0, fmt.Errorf("unknown owned_by %q", s)
	}
}
