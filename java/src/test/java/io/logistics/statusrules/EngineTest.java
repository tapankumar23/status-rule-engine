package io.logistics.statusrules;

import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import java.nio.file.Path;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Integration tests for {@link Engine}.
 *
 * <p>The spec is loaded once before all tests from {@code ../spec/rules.yaml}
 * (relative to the Maven {@code java/} working directory), so these tests
 * exercise the real production spec rather than a synthetic fixture.
 */
class EngineTest {

    private static Engine engine;

    @BeforeAll
    static void loadEngine() throws Exception {
        // Load from the canonical spec location used in production.
        engine = Engine.newEngineFromPath(Path.of("../spec/rules.yaml"));
    }

    // ── Test 1: Valid forward transition ──────────────────────────────────────

    /**
     * A standard middle-mile FORWARD hop: HUB_IN_SCANNED → HUB_OUTSCAN.
     * The spec defines this edge explicitly in the transitions block.
     */
    @Test
    void validForwardTransition() {
        ValidationResult result = engine.validate(
                Status.HUB_IN_SCANNED,
                Status.HUB_OUTSCAN,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.HUB_SYSTEM));

        assertTrue(result.valid(),     "HUB_IN_SCANNED → HUB_OUTSCAN FORWARD should be valid");
        assertEquals(ErrorCode.NONE,   result.errorCode());
        assertFalse(result.isOverride(), "should not be an override transition");
    }

    // ── Test 2: Invalid transition ────────────────────────────────────────────

    /**
     * DRAFT → DELIVERED is not an edge in the spec; the engine must reject it.
     */
    @Test
    void invalidTransition() {
        ValidationResult result = engine.validate(
                Status.DRAFT,
                Status.DELIVERED,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.BOOKING_SYSTEM));

        assertFalse(result.valid(),                   "DRAFT → DELIVERED should be invalid");
        assertEquals(ErrorCode.INVALID_TRANSITION,    result.errorCode());
    }

    // ── Test 3: Terminal status ───────────────────────────────────────────────

    /**
     * No transition is allowed out of DELIVERED, which is marked terminal
     * in the spec.
     */
    @Test
    void terminalStatusIsRejected() {
        ValidationResult result = engine.validate(
                Status.DELIVERED,
                Status.INSCANNED_AT_CP,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.DELIVERY_SYSTEM));

        assertFalse(result.valid(),                  "Transition from DELIVERED should be invalid");
        assertEquals(ErrorCode.TERMINAL_STATUS,      result.errorCode());
    }

    // ── Test 4: Override without operatorId ───────────────────────────────────

    /**
     * HUB_IN_SCANNED → FORCE_HUB_OUTSCAN is an override edge that requires an
     * operatorId; submitting without one must return MISSING_OPERATOR_ID.
     */
    @Test
    void overrideWithoutOperatorIdIsRejected() {
        ValidationResult result = engine.validate(
                Status.HUB_IN_SCANNED,
                Status.FORCE_HUB_OUTSCAN,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.HUB_SYSTEM));   // no operatorId

        assertFalse(result.valid(),                       "Override without operatorId should fail");
        assertEquals(ErrorCode.MISSING_OPERATOR_ID,       result.errorCode());
    }

    // ── Test 5: Override with operatorId ─────────────────────────────────────

    /**
     * The same override edge succeeds when an operatorId is present, and the
     * result must have {@code isOverride=true}.
     */
    @Test
    void overrideWithOperatorIdIsValid() {
        ValidationResult result = engine.validate(
                Status.HUB_IN_SCANNED,
                Status.FORCE_HUB_OUTSCAN,
                Flow.FORWARD,
                TransitionContext.withOperator(SourceSystem.HUB_SYSTEM, "ops-user-42"));

        assertTrue(result.valid(),         "Override with operatorId should succeed");
        assertTrue(result.isOverride(),    "isOverride must be true for a FORCE_* edge");
        assertEquals(ErrorCode.NONE,       result.errorCode());
    }

    // ── Test 6: Wildcard DAMAGED from OUT_FOR_DELIVERY ────────────────────────

    /**
     * DAMAGED is a wildcard target reachable from every non-terminal status in
     * any flow.  Verify it works from OUT_FOR_DELIVERY FORWARD.
     */
    @Test
    void wildcardDamagedFromOutForDelivery() {
        ValidationResult result = engine.validate(
                Status.OUT_FOR_DELIVERY,
                Status.DAMAGED,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.HUB_SYSTEM));

        assertTrue(result.valid(),
                "DAMAGED should be reachable from OUT_FOR_DELIVERY as a wildcard target");
        assertEquals(ErrorCode.NONE, result.errorCode());
    }

    // ── Test 7: Null status → ZERO_VALUE_STATUS ───────────────────────────────

    /**
     * Passing {@code null} as the current status is a caller error; the engine
     * must return ZERO_VALUE_STATUS rather than throwing.
     */
    @Test
    void nullStatusReturnsZeroValueStatus() {
        ValidationResult result = engine.validate(
                null,
                Status.BOOKED,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.BOOKING_SYSTEM));

        assertFalse(result.valid(),                  "null current status should be invalid");
        assertEquals(ErrorCode.ZERO_VALUE_STATUS,    result.errorCode());
    }

    // ── Test 8: validateInitial DRAFT FORWARD ─────────────────────────────────

    /**
     * DRAFT / FORWARD is the only entry in {@code initial_statuses}; it must
     * be accepted by {@link Engine#validateInitial}.
     */
    @Test
    void validateInitialDraftForward() {
        ValidationResult result = engine.validateInitial(
                Status.DRAFT,
                Flow.FORWARD,
                TransitionContext.of(SourceSystem.BOOKING_SYSTEM));

        assertTrue(result.valid(),     "DRAFT FORWARD should be a valid initial status");
        assertEquals(ErrorCode.NONE,   result.errorCode());
    }

    // ── Test 9: specVersion ───────────────────────────────────────────────────

    /**
     * The YAML spec declares {@code spec_version: "1.0.0"}; the engine must
     * surface this exact string.
     */
    @Test
    void specVersionIsCorrect() {
        assertEquals("1.0.0", engine.specVersion(),
                "specVersion() should return the value from the YAML spec_version field");
    }

    // ── Test 10: permittedTransitions is non-empty ────────────────────────────

    /**
     * DRAFT has several outbound edges in the spec; {@link Engine#permittedTransitions}
     * must return a non-empty list containing at least BOOKED.
     */
    @Test
    void permittedTransitionsNonEmpty() {
        List<Status> permitted = engine.permittedTransitions(Status.DRAFT, Flow.FORWARD);

        assertFalse(permitted.isEmpty(),
                "DRAFT should have at least one permitted forward transition");
        assertTrue(permitted.contains(Status.BOOKED),
                "BOOKED should be among the permitted transitions from DRAFT");
    }
}
