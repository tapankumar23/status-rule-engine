package io.logistics.statusrules;

/**
 * Point-in-time snapshot of engine validation counters.
 *
 * <p>All counters are monotonically increasing from the moment the
 * {@link Engine} instance was created. Obtain a snapshot via
 * {@link Engine#snapshot()}.
 *
 * @param validationsTotal      Total number of {@code validate} / {@code validateInitial} calls
 * @param invalidTransitions    Calls that returned {@code valid=false}
 * @param overrideTransitions   Calls that returned {@code isOverride=true}
 * @param interSystemTransitions Calls that returned {@code isInterSystem=true}
 * @param zeroValueRejections   Calls rejected because a {@code null} status was supplied
 */
public record MetricsSnapshot(
        long validationsTotal,
        long invalidTransitions,
        long overrideTransitions,
        long interSystemTransitions,
        long zeroValueRejections) {
}
