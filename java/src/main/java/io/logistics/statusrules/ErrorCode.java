package io.logistics.statusrules;

/**
 * Machine-readable reason codes returned inside a {@link ValidationResult}.
 *
 * <p>Callers should switch on this value rather than parsing the human-readable
 * {@code anomalyReason} string, which is subject to change.
 */
public enum ErrorCode {

    /** No error — the transition is allowed. */
    NONE,

    /** The {@code next} status is not a valid successor of {@code current}. */
    INVALID_TRANSITION,

    /** The {@code current} status is terminal; no further transitions are permitted. */
    TERMINAL_STATUS,

    /** One or both status codes were not found in the spec registry. */
    UNKNOWN_STATUS,

    /** The transition requires an {@code operatorId} in the context but none was provided. */
    MISSING_OPERATOR_ID,

    /**
     * The transition crosses system ownership boundaries (inter-system) and the
     * calling system does not own the {@code next} status.
     */
    FLOW_MISMATCH,

    /** The {@code sourceSystem} in the context is {@code null} or unrecognised. */
    INVALID_SOURCE_SYSTEM,

    /** A {@code null} or zero-value status was supplied as input. */
    ZERO_VALUE_STATUS,

    /** A first-mile status was proposed after a middle-mile or last-mile status. */
    FIRST_MILE_AFTER_LATER_MILE
}
