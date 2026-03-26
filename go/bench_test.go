package statusrules_test

import (
	"os"
	"testing"

	statusrules "github.com/logistics-oss/status-rules"
	"github.com/logistics-oss/status-rules/status"
)

// ── Zero-allocation assertion ─────────────────────────────────────────────────
//
// This test is the most critical CI gate in the library.
// Any change that introduces a heap allocation on the Validate() success path
// will fail this test and must be fixed before merge.

func TestValidate_NoAlloc_SuccessPath(t *testing.T) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{
		AWB:          "AWB123456",
		SourceSystem: statusrules.SystemHub,
	}

	allocs := testing.AllocsPerRun(1000, func() {
		e.Validate(status.HubInScanned, status.HubOutscan, statusrules.FlowForward, ctx)
	})

	if allocs > 0 {
		t.Fatalf("Validate() hot path must not allocate: got %.0f allocs/op", allocs)
	}
}

func TestValidate_NoAlloc_FailurePath(t *testing.T) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}

	allocs := testing.AllocsPerRun(1000, func() {
		e.Validate(status.HubInScanned, status.Delivered, statusrules.FlowForward, ctx)
	})

	if allocs > 0 {
		t.Fatalf("Validate() failure path must not allocate: got %.0f allocs/op", allocs)
	}
}

func TestValidate_NoAlloc_ForceOverridePath(t *testing.T) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{
		SourceSystem: statusrules.SystemHub,
		OperatorID:   "op-999",
	}

	allocs := testing.AllocsPerRun(1000, func() {
		e.Validate(status.HubInScanned, status.ForceHubOutscan, statusrules.FlowForward, ctx)
	})

	if allocs > 0 {
		t.Fatalf("Validate() FORCE_* path must not allocate: got %.0f allocs/op", allocs)
	}
}

// ── Benchmarks ────────────────────────────────────────────────────────────────
//
// Run with: go test -bench=. -benchmem -count=3
//
// Expected output (approximate):
//   BenchmarkValidate_HotPath-N         50000000    ~21 ns/op    0 B/op    0 allocs/op
//   BenchmarkValidate_InvalidTransition 50000000    ~19 ns/op    0 B/op    0 allocs/op
//   BenchmarkValidate_ForceOverride     50000000    ~22 ns/op    0 B/op    0 allocs/op
//   BenchmarkNewEngine-N                    1000   ~5000000 ns/op

func BenchmarkValidate_HotPath(b *testing.B) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{
		AWB:          "AWB123456",
		SourceSystem: statusrules.SystemHub,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Validate(status.HubInScanned, status.HubOutscan, statusrules.FlowForward, ctx)
	}
}

func BenchmarkValidate_InvalidTransition(b *testing.B) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Validate(status.Inscanned, status.Delivered, statusrules.FlowForward, ctx)
	}
}

func BenchmarkValidate_ForceOverride(b *testing.B) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{
		SourceSystem: statusrules.SystemHub,
		OperatorID:   "op-bench",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Validate(status.HubInScanned, status.ForceHubOutscan, statusrules.FlowForward, ctx)
	}
}

func BenchmarkValidate_InterSystemHandoff(b *testing.B) {
	e := statusrules.NewEngine()
	// MANIFESTED (Booking) → READY_FOR_PICKUP (Hub): inter-system handoff
	ctx := statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Validate(status.Manifested, status.ReadyForPickup, statusrules.FlowForward, ctx)
	}
}

func BenchmarkValidate_WildcardTarget(b *testing.B) {
	e := statusrules.NewEngine()
	ctx := statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Validate(status.OutForDelivery, status.Damaged, statusrules.FlowForward, ctx)
	}
}

func BenchmarkNewEngine(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = statusrules.NewEngine()
	}
}

func BenchmarkPermittedTransitions(b *testing.B) {
	e := statusrules.NewEngine()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = e.PermittedTransitions(status.HubInScanned, statusrules.FlowForward)
	}
}

// ── Spec loading helper (used by engine_test.go) ──────────────────────────────

// readSpecFile reads spec/rules.yaml for use in NewEngineFromSpec tests.
func readSpecFile(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("../spec/rules.yaml")
	if err != nil {
		t.Skipf("spec/rules.yaml not readable from test working directory: %v", err)
	}
	return data
}

func TestNewEngineFromSpec_BundledSpec(t *testing.T) {
	data := readSpecFile(t)
	e, err := statusrules.NewEngineFromSpec(data)
	if err != nil {
		t.Fatalf("NewEngineFromSpec with bundled spec: %v", err)
	}
	if e.SpecVersion() == "" {
		t.Error("SpecVersion should not be empty")
	}
}
