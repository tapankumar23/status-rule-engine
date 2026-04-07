package statusrules_test

import (
	"testing"

	statusrules "github.com/logistics-oss/status-rules"
	"github.com/logistics-oss/status-rules/status"
)

// engine is shared across all tests — NewEngine() is called once.
var engine = statusrules.NewEngine()

// ── Spec version ──────────────────────────────────────────────────────────────

func TestSpecVersion(t *testing.T) {
	v := engine.SpecVersion()
	if v == "" {
		t.Fatal("SpecVersion() returned empty string")
	}
	t.Logf("spec version: %s", v)
}

// ── Contract tests: positive (valid transitions) ──────────────────────────────

type transitionCase struct {
	name    string
	current status.Status
	next    status.Status
	flow    statusrules.Flow
	ctx     statusrules.TransitionContext
}

var validTransitions = []transitionCase{
	// Pre-Mile / Booking
	{"DRAFT→BOOKED", status.Draft, status.Booked, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"DRAFT→CANCELLED", status.Draft, status.Cancelled, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"BOOKED→MANIFESTED", status.Booked, status.Manifested, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"BOOKED→PART_PAYMENT_PENDING", status.Booked, status.PartPaymentPending, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"PART_PAYMENT_PENDING→BOOKED", status.PartPaymentPending, status.Booked, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"UPDATED_BOOKING→MANIFESTED", status.UpdatedBooking, status.Manifested, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},

	// First Mile
	{"MANIFESTED→READY_FOR_PICKUP", status.Manifested, status.ReadyForPickup, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"READY_FOR_PICKUP→INSCANNED", status.ReadyForPickup, status.Inscanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"READY_FOR_PICKUP→HUB_IN_SCANNED", status.ReadyForPickup, status.HubInScanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},

	// Middle Mile
	{"INSCANNED→HUB_IN_SCANNED", status.Inscanned, status.HubInScanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"INSCANNED→BAGGED", status.Inscanned, status.Bagged, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"INSCANNED→OUT_SCAN_TO_3PL", status.Inscanned, status.OutScanTo3PL, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"HUB_IN_SCANNED→HUB_OUTSCAN", status.HubInScanned, status.HubOutscan, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"HUB_IN_SCANNED→BAGGED", status.HubInScanned, status.Bagged, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"HUB_OUTSCAN→INSCANNED_AT_TRANSIT", status.HubOutscan, status.InscannedAtTransit, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"HUB_OUTSCAN→INSCANNED_AT_CP", status.HubOutscan, status.InscannedAtCP, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"BAGGED→BAG_FINALISED", status.Bagged, status.BagFinalised, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"BAG_CREATED→BAG_FINALISED", status.BagCreated, status.BagFinalised, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"BAG_CREATED→BAG_DELETED", status.BagCreated, status.BagDeleted, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"INSCANNED_AT_TRANSIT→HUB_OUTSCAN", status.InscannedAtTransit, status.HubOutscan, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},

	// Last Mile
	{"INSCANNED_AT_CP→SCHEDULED_FOR_TRIP", status.InscannedAtCP, status.ScheduledForTrip, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"INSCANNED_AT_CP→OUT_FOR_DELIVERY", status.InscannedAtCP, status.OutForDelivery, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"SCHEDULED_FOR_TRIP→OUT_FOR_DELIVERY", status.ScheduledForTrip, status.OutForDelivery, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"OUT_FOR_DELIVERY→DELIVERED", status.OutForDelivery, status.Delivered, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"OUT_FOR_DELIVERY→ATTEMPTED", status.OutForDelivery, status.Attempted, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"OUT_FOR_DELIVERY→UNDELIVERED", status.OutForDelivery, status.Undelivered, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"ATTEMPTED→OUT_FOR_DELIVERY", status.Attempted, status.OutForDelivery, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"ATTEMPTED→RETURN_INITIATED", status.Attempted, status.ReturnInitiated, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"UNDELIVERED→RETURN_INITIATED", status.Undelivered, status.ReturnInitiated, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},

	// RTO / Reverse
	{"RETURN_INITIATED→RTO", status.ReturnInitiated, status.RTO, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"RETURN_INITIATED→RETURN_REVOKED", status.ReturnInitiated, status.ReturnRevoked, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"RETURN_REVOKED→OUT_FOR_DELIVERY", status.ReturnRevoked, status.OutForDelivery, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"RTO→RTO_OUT_FOR_DELIVERY", status.RTO, status.RTOOutForDelivery, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"RTO→HUB_IN_SCANNED", status.RTO, status.HubInScanned, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"RTO_OUT_FOR_DELIVERY→RTO_DELIVERED", status.RTOOutForDelivery, status.RTODelivered, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"RTO_UNDELIVERED→RTO_OUT_FOR_DELIVERY", status.RTOUndelivered, status.RTOOutForDelivery, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},

	// 3PL
	{"OUT_SCAN_TO_3PL→3PL_ITEM_BOOK", status.OutScanTo3PL, status.ThreePLItemBook, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.System3PL}},
	{"3PL_ITEM_BOOK→3PL_ITEM_DELIVERY", status.ThreePLItemBook, status.ThreePLItemDelivery, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.System3PL}},
	{"3PL_ITEM_BOOK→3PL_ITEM_ONHOLD", status.ThreePLItemBook, status.ThreePLItemOnhold, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.System3PL}},
	{"3PL_BAG_CLOSE→3PL_BAG_DISPATCH", status.ThreePLBagClose, status.ThreePLBagDispatch, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.System3PL}},
	{"3PL_BAG_DISPATCH→3PL_BAG_OPEN", status.ThreePLBagDispatch, status.ThreePLBagOpen, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.System3PL}},
	{"3PL_ITEM_RETURN→INSCANNED", status.ThreePLItemReturn, status.Inscanned, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
}

func TestValidate_ValidTransitions(t *testing.T) {
	for _, tc := range validTransitions {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.Validate(tc.current, tc.next, tc.flow, tc.ctx)
			if !result.Valid {
				t.Errorf("expected Valid=true, got ErrorCode=%d AnomalyReason=%q",
					result.ErrorCode, result.AnomalyReason)
			}
		})
	}
}

// ── Contract tests: negative (invalid transitions) ────────────────────────────

var invalidTransitions = []transitionCase{
	// Cannot skip steps
	{"DRAFT→MANIFESTED_skip", status.Draft, status.Manifested, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"BOOKED→DELIVERED_skip", status.Booked, status.Delivered, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},
	{"INSCANNED→DELIVERED_skip", status.Inscanned, status.Delivered, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},

	// Cannot go backwards
	{"MANIFESTED→DRAFT_back", status.Manifested, status.Draft, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"DELIVERED→INSCANNED_back", status.Delivered, status.Inscanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},

	// Wrong flow
	{"DRAFT→BOOKED_wrong_flow", status.Draft, status.Booked, statusrules.FlowReverse,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking}},
	{"RTO→RTO_OUT_WRONG_FLOW", status.RTO, status.RTOOutForDelivery, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemDelivery}},

	// FORCE_* without operator_id
	{"HUB_IN_SCANNED→FORCE_HUB_OUTSCAN_no_op", status.HubInScanned, status.ForceHubOutscan, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub, OperatorID: ""}},

	// First-mile status after middle-mile or last-mile
	{"HUB_IN_SCANNED→INSCANNED_first_mile_regress", status.HubInScanned, status.Inscanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"OUT_FOR_DELIVERY→READY_FOR_PICKUP_first_mile_regress", status.OutForDelivery, status.ReadyForPickup, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
	{"HUB_OUTSCAN→INSCANNED_first_mile_regress", status.HubOutscan, status.Inscanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub}},
}

func TestValidate_InvalidTransitions(t *testing.T) {
	for _, tc := range invalidTransitions {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.Validate(tc.current, tc.next, tc.flow, tc.ctx)
			if result.Valid {
				t.Errorf("expected Valid=false for %s→%s (flow=%v), got Valid=true",
					tc.current, tc.next, tc.flow)
			}
		})
	}
}

// ── Property tests ────────────────────────────────────────────────────────────

// Terminal statuses must never be valid source nodes.
func TestProperty_TerminalStatusNeverValidFrom(t *testing.T) {
	terminals := []status.Status{
		status.Delivered,
		status.RTODelivered,
		status.Cancelled,
		status.Rejected,
		status.RejectedByHO,
		status.ThreePLItemDelivery,
	}
	for _, term := range terminals {
		for _, next := range []status.Status{status.Inscanned, status.Booked, status.HubOutscan} {
			for _, flow := range []statusrules.Flow{statusrules.FlowForward, statusrules.FlowReverse} {
				result := engine.Validate(term, next, flow, statusrules.TransitionContext{
					SourceSystem: statusrules.SystemHub,
				})
				if result.Valid {
					t.Errorf("terminal status %v should never be a valid source, but %v→%v (flow=%v) returned Valid=true",
						term, term, next, flow)
				}
				if result.ErrorCode != statusrules.ErrTerminalStatus {
					t.Errorf("expected ErrTerminalStatus for terminal %v, got %v", term, result.ErrorCode)
				}
			}
		}
	}
}

// Wildcard exception targets must be valid destinations from every non-terminal status.
func TestProperty_WildcardTargetsReachableFromAny(t *testing.T) {
	wildcards := []status.Status{
		status.Damaged,
		status.MisRouted,
		status.Rerouted,
		status.ExcessInscan,
		status.PincodeAudit,
		status.WeightAudit,
		status.ModeAudit,
	}
	// Sample of non-terminal statuses across the pipeline
	nonTerminals := []struct {
		s    status.Status
		flow statusrules.Flow
		sys  statusrules.SourceSystem
	}{
		{status.Draft, statusrules.FlowForward, statusrules.SystemBooking},
		{status.Inscanned, statusrules.FlowForward, statusrules.SystemHub},
		{status.HubInScanned, statusrules.FlowForward, statusrules.SystemHub},
		{status.OutForDelivery, statusrules.FlowForward, statusrules.SystemDelivery},
		{status.Attempted, statusrules.FlowForward, statusrules.SystemDelivery},
		{status.RTO, statusrules.FlowReverse, statusrules.SystemHub},
	}
	for _, from := range nonTerminals {
		for _, wc := range wildcards {
			result := engine.Validate(from.s, wc, from.flow, statusrules.TransitionContext{
				SourceSystem: statusrules.SystemHub, // Hub owns all exception statuses
			})
			if !result.Valid {
				t.Errorf("wildcard %v should be reachable from %v (flow=%v), got ErrorCode=%d reason=%q",
					wc, from.s, from.flow, result.ErrorCode, result.AnomalyReason)
			}
		}
	}
}

// FORCE_* transitions must return IsOverride=true.
func TestProperty_ForceEdgesReturnIsOverride(t *testing.T) {
	forceEdges := []struct {
		from status.Status
		to   status.Status
	}{
		{status.HubInScanned, status.ForceHubOutscan},
		{status.Bagged, status.ForceBag},
		{status.ForceBag, status.ForceBagAttempted},
		{status.HubOutscan, status.ForceOutscanToCP},
	}
	for _, e := range forceEdges {
		result := engine.Validate(e.from, e.to, statusrules.FlowForward, statusrules.TransitionContext{
			SourceSystem: statusrules.SystemHub,
			OperatorID:   "op-123",
		})
		if !result.Valid {
			t.Errorf("FORCE edge %v→%v should be valid with operator_id, got ErrorCode=%d reason=%q",
				e.from, e.to, result.ErrorCode, result.AnomalyReason)
		}
		if !result.IsOverride {
			t.Errorf("FORCE edge %v→%v must return IsOverride=true", e.from, e.to)
		}
	}
}

// FORCE_* transitions without operator_id must return ErrMissingOperatorID.
func TestProperty_ForceEdgesRequireOperatorID(t *testing.T) {
	forceEdges := []struct {
		from status.Status
		to   status.Status
	}{
		{status.HubInScanned, status.ForceHubOutscan},
		{status.Bagged, status.ForceBag},
		{status.HubOutscan, status.ForceOutscanToCP},
	}
	for _, e := range forceEdges {
		result := engine.Validate(e.from, e.to, statusrules.FlowForward, statusrules.TransitionContext{
			SourceSystem: statusrules.SystemHub,
			OperatorID:   "", // missing
		})
		if result.Valid {
			t.Errorf("FORCE edge %v→%v without operator_id should be invalid", e.from, e.to)
		}
		if result.ErrorCode != statusrules.ErrMissingOperatorID {
			t.Errorf("expected ErrMissingOperatorID for %v→%v, got %d", e.from, e.to, result.ErrorCode)
		}
	}
}

// First-mile statuses must be rejected after middle-mile or last-mile statuses.
func TestProperty_FirstMileCannotFollowLaterMile(t *testing.T) {
	cases := []struct {
		from status.Status
		to   status.Status
	}{
		{status.HubInScanned, status.Inscanned},
		{status.HubInScanned, status.ReadyForPickup},
		{status.Bagged, status.Inscanned},
		{status.InscannedAtTransit, status.ReadyForPickup},
		{status.OutForDelivery, status.Inscanned},
		{status.OutForDelivery, status.ReadyForPickup},
		{status.Attempted, status.Inscanned},
		{status.Undelivered, status.ReadyForPickup},
	}
	for _, tc := range cases {
		result := engine.Validate(tc.from, tc.to, statusrules.FlowForward, statusrules.TransitionContext{
			SourceSystem: statusrules.SystemHub,
		})
		if result.Valid {
			t.Errorf("first-mile %v should not follow later-mile %v, but got Valid=true", tc.to, tc.from)
		}
		if result.ErrorCode != statusrules.ErrFirstMileAfterLaterMile {
			t.Errorf("expected ErrFirstMileAfterLaterMile for %v→%v, got %d", tc.from, tc.to, result.ErrorCode)
		}
	}
}

// Zero-value inputs must return ErrZeroValueStatus.
func TestProperty_ZeroValueStatus(t *testing.T) {
	cases := []struct {
		current status.Status
		next    status.Status
	}{
		{0, status.Booked},
		{status.Draft, 0},
		{0, 0},
	}
	for _, tc := range cases {
		result := engine.Validate(tc.current, tc.next, statusrules.FlowForward,
			statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking})
		if result.Valid {
			t.Errorf("zero-value input (%v→%v) should be invalid", tc.current, tc.next)
		}
		if result.ErrorCode != statusrules.ErrZeroValueStatus {
			t.Errorf("expected ErrZeroValueStatus, got %d", result.ErrorCode)
		}
	}
}

// ── ValidateInitial tests ─────────────────────────────────────────────────────

func TestValidateInitial_DraftForward(t *testing.T) {
	result := engine.ValidateInitial(status.Draft, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking})
	if !result.Valid {
		t.Errorf("DRAFT should be valid initial status for FORWARD flow, got ErrorCode=%d", result.ErrorCode)
	}
}

func TestValidateInitial_InvalidInitial(t *testing.T) {
	result := engine.ValidateInitial(status.Inscanned, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemHub})
	if result.Valid {
		t.Error("INSCANNED should not be a valid initial status")
	}
}

// ── PermittedTransitions tests ────────────────────────────────────────────────

func TestPermittedTransitions_NonEmpty(t *testing.T) {
	nexts := engine.PermittedTransitions(status.HubInScanned, statusrules.FlowForward)
	if len(nexts) == 0 {
		t.Error("HUB-IN-SCANNED should have permitted transitions in FORWARD flow")
	}
}

func TestPermittedTransitions_Terminal(t *testing.T) {
	nexts := engine.PermittedTransitions(status.Delivered, statusrules.FlowForward)
	if len(nexts) != 0 {
		t.Errorf("DELIVERED is terminal — expected 0 transitions, got %d", len(nexts))
	}
}

// ── Metrics snapshot ──────────────────────────────────────────────────────────

func TestSnapshot_CountsInvalidTransitions(t *testing.T) {
	// NewEngineFromSpec(nil) must return an error, not panic.
	_, err := statusrules.NewEngineFromSpec(nil)
	if err == nil {
		t.Error("NewEngineFromSpec(nil) should return an error")
	}

	// Use a fresh engine (not the package-level one) so counters start at zero.
	fresh := statusrules.NewEngine()
	before := fresh.Snapshot()

	fresh.Validate(status.Draft, status.Delivered, statusrules.FlowForward,
		statusrules.TransitionContext{SourceSystem: statusrules.SystemBooking})

	after := fresh.Snapshot()
	if after.InvalidTransitions <= before.InvalidTransitions {
		t.Error("invalid transition should increment InvalidTransitions counter")
	}
	if after.ValidationsTotal <= before.ValidationsTotal {
		t.Error("any Validate() call should increment ValidationsTotal counter")
	}
}

// ── NewEngineFromSpec validation ──────────────────────────────────────────────

func TestNewEngineFromSpec_InvalidYAML(t *testing.T) {
	_, err := statusrules.NewEngineFromSpec([]byte("not: valid: yaml: ["))
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

