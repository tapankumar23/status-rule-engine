package io.logistics.statusrules;

/**
 * Immutable bag of caller-supplied context passed to every validation call.
 *
 * <p>At minimum a {@link SourceSystem} is required. The {@code awb} field is
 * carried through for tracing / logging but is not evaluated by the engine.
 * The {@code operatorId} must be non-blank for any transition whose edge has
 * {@code requires_context: [operator_id]} in the spec.
 *
 * @param awb          Air-waybill / tracking number (may be empty — not validated by engine)
 * @param sourceSystem The system reporting the transition; must not be {@code null}
 * @param operatorId   Operator identifier; required for override transitions
 */
public record TransitionContext(String awb, SourceSystem sourceSystem, String operatorId) {

    /**
     * Convenience factory: no AWB, no operatorId.
     *
     * @param sys the reporting system
     */
    public static TransitionContext of(SourceSystem sys) {
        return new TransitionContext("", sys, "");
    }

    /**
     * Convenience factory: no AWB, with operatorId.
     *
     * @param sys  the reporting system
     * @param opId the operator performing the override
     */
    public static TransitionContext withOperator(SourceSystem sys, String opId) {
        return new TransitionContext("", sys, opId);
    }
}
