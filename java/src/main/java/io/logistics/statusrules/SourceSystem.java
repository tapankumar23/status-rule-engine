package io.logistics.statusrules;

import java.util.HashMap;
import java.util.Map;

/**
 * Represents the upstream system that is reporting a status change.
 *
 * <p>The {@code yamlKey} must match the {@code owned_by} strings used in the
 * YAML spec exactly so that cross-system transitions can be detected at
 * validation time.
 */
public enum SourceSystem {
    BOOKING_SYSTEM("BOOKING_SYSTEM"),
    HUB_SYSTEM("HUB_SYSTEM"),
    DELIVERY_SYSTEM("DELIVERY_SYSTEM"),
    SYSTEM_3PL("SYSTEM_3PL");

    /** The literal string that appears in the YAML spec's {@code owned_by} field. */
    public final String yamlKey;

    SourceSystem(String k) {
        this.yamlKey = k;
    }

    // ── Static lookup ──────────────────────────────────────────────────────────

    private static final Map<String, SourceSystem> BY_YAML_KEY = new HashMap<>();

    static {
        for (SourceSystem s : values()) {
            BY_YAML_KEY.put(s.yamlKey, s);
        }
    }

    /**
     * Returns the {@link SourceSystem} whose {@code yamlKey} equals {@code s}.
     *
     * @param s the YAML key string (e.g. {@code "HUB_SYSTEM"})
     * @return the matching enum constant
     * @throws IllegalArgumentException if {@code s} does not match any constant
     */
    public static SourceSystem fromYaml(String s) {
        SourceSystem result = BY_YAML_KEY.get(s);
        if (result == null) {
            throw new IllegalArgumentException("Unknown source system YAML key: " + s);
        }
        return result;
    }
}
