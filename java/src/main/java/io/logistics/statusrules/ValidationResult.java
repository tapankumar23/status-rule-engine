package io.logistics.statusrules;

/**
 * Immutable result returned by every {@link Engine} validation call.
 *
 * @param valid           {@code true} if the transition is allowed
 * @param isOverride      {@code true} if the transition is an override edge
 *                        (e.g. a FORCE_* status); the operator ID was present
 * @param isInterSystem   {@code true} if the transition crossed system
 *                        ownership boundaries (source system ≠ owner of next)
 * @param errorCode       Machine-readable reason; {@link ErrorCode#NONE} when valid
 * @param anomalyReason   Human-readable description; empty string when valid
 */
public record ValidationResult(
        boolean valid,
        boolean isOverride,
        boolean isInterSystem,
        ErrorCode errorCode,
        String anomalyReason) {

    // ── Static factory helpers ─────────────────────────────────────────────────

    /** A clean, valid transition with no special flags. */
    public static ValidationResult ok() {
        return new ValidationResult(true, false, false, ErrorCode.NONE, "");
    }

    /** A valid override transition (FORCE_* edge; operatorId was present). */
    public static ValidationResult okOverride() {
        return new ValidationResult(true, true, false, ErrorCode.NONE, "");
    }

    /** A valid inter-system transition. */
    public static ValidationResult okInterSystem() {
        return new ValidationResult(true, false, true, ErrorCode.NONE, "");
    }

    /** A valid override transition that also crosses system boundaries. */
    public static ValidationResult okOverrideInterSystem() {
        return new ValidationResult(true, true, true, ErrorCode.NONE, "");
    }

    /**
     * An invalid result with the given {@link ErrorCode} and a human-readable
     * reason message.
     *
     * @param code   the machine-readable error code
     * @param reason the human-readable explanation
     */
    public static ValidationResult error(ErrorCode code, String reason) {
        return new ValidationResult(false, false, false, code, reason);
    }
}
