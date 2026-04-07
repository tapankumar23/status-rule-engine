package io.logistics.statusrules;

import org.yaml.snakeyaml.Yaml;

import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Core rule engine for shipment status transitions.
 *
 * <h2>Adjacency table layout</h2>
 * <p>The engine compiles the YAML spec into a flat {@code short[8192]} array.
 * Each entry is addressed by a packed integer key:
 * <pre>
 *   key = (fromId &lt;&lt; 7) | (toId &lt;&lt; 1) | flowBit
 *   flowBit: FORWARD = 0, REVERSE = 1
 * </pre>
 * Status IDs are the YAML 0-based {@code id} field plus one (matching the Go
 * convention where 0 is the uninitialised sentinel). With MAX_ID = 55, the
 * maximum key is (55 &lt;&lt; 7) | (55 &lt;&lt; 1) | 1 = 7040 + 110 + 1 = 7151, which
 * fits comfortably in the 8192-entry array.
 *
 * <h2>Edge flags</h2>
 * <pre>
 *   FLAG_PRESENT       = 0x01  — edge exists
 *   FLAG_OVERRIDE      = 0x02  — FORCE_* / override transition; requires operatorId
 *   FLAG_REQUIRES_OPID = 0x04  — alias for override requirement (same bit field)
 *   FLAG_INTER_SYSTEM  = 0x08  — from-owner ≠ to-owner
 * </pre>
 *
 * <h2>Thread safety</h2>
 * <p>After construction the compiled tables are immutable. Metric counters use
 * {@link AtomicLong} so the engine is safe to share across threads.
 */
public final class Engine {

    // ── Adjacency-table constants ──────────────────────────────────────────────

    /** Total adjacency-table entries: IDs 0-63 × IDs 0-63 × 2 flows = 8192. */
    private static final int ADJ_SIZE = 8192;

    /** Bit 0 — the edge exists in the spec. */
    static final short FLAG_PRESENT       = 0x01;

    /** Bit 1 — this is an override (FORCE_*) edge. */
    static final short FLAG_OVERRIDE      = 0x02;

    /** Bit 2 — the edge requires a non-blank operatorId in the context. */
    static final short FLAG_REQUIRES_OPID = 0x04;

    /** Bit 3 — the source system does not own the destination status. */
    static final short FLAG_INTER_SYSTEM  = 0x08;

    // ── Compiled spec tables ───────────────────────────────────────────────────

    /**
     * Packed adjacency table. Each element is a bitset of FLAG_* constants.
     * Java {@code short} is 16-bit signed; we read it as unsigned via
     * {@code & 0xFFFF} wherever comparison is needed.
     */
    private final short[] adj;

    /**
     * Per-status owning system (indexed by status ID, 1-based).
     * Element [0] is unused (zero-value sentinel).
     */
    private final SourceSystem[] owners;

    /**
     * Per-status terminal flag (indexed by status ID, 1-based).
     * Element [0] is unused.
     */
    private final boolean[] terminals;

    /**
     * Per-status flag: true if this status has category FIRST_MILE.
     * Element [0] is unused.
     */
    private final boolean[] firstMile;

    /**
     * Per-status flag: true if this status has category MIDDLE_MILE or LAST_MILE.
     * Element [0] is unused.
     */
    private final boolean[] postFirstMile;

    /**
     * Valid first statuses for new shipments.
     * Each element is a two-element array {@code [statusId, flowBit]}.
     */
    private final List<int[]> initials;

    /** Spec version string read from the YAML {@code spec_version} field. */
    private final String specVersion;

    // ── Metrics counters ───────────────────────────────────────────────────────

    private final AtomicLong validationsTotal      = new AtomicLong();
    private final AtomicLong invalidTransitions     = new AtomicLong();
    private final AtomicLong overrideTransitions    = new AtomicLong();
    private final AtomicLong interSystemTransitions = new AtomicLong();
    private final AtomicLong zeroValueRejections    = new AtomicLong();

    // ── Constructor ────────────────────────────────────────────────────────────

    /** All construction goes through the static factories. */
    private Engine(short[] adj,
                   SourceSystem[] owners,
                   boolean[] terminals,
                   boolean[] firstMile,
                   boolean[] postFirstMile,
                   List<int[]> initials,
                   String specVersion) {
        this.adj           = adj;
        this.owners        = owners;
        this.terminals     = terminals;
        this.firstMile     = firstMile;
        this.postFirstMile = postFirstMile;
        this.initials      = Collections.unmodifiableList(initials);
        this.specVersion   = specVersion;
    }

    // ── Static factories ───────────────────────────────────────────────────────

    /**
     * Creates an engine by reading {@code ../spec/rules.yaml} relative to the
     * {@code java/} working directory.  This is the default production factory.
     *
     * @return a compiled, ready-to-use engine
     * @throws IOException if the spec file cannot be read
     */
    public static Engine newEngine() throws IOException {
        return newEngineFromPath(Path.of("../spec/rules.yaml"));
    }

    /**
     * Creates an engine from an explicit file path.  Useful for testing or
     * deployments where the spec lives at a non-standard location.
     *
     * @param specPath absolute or relative path to the YAML spec file
     * @return a compiled engine
     * @throws IOException if the file cannot be read
     */
    public static Engine newEngineFromPath(Path specPath) throws IOException {
        byte[] bytes = Files.readAllBytes(specPath);
        return newEngineFromSpec(bytes);
    }

    /**
     * Creates an engine from raw YAML bytes.  Useful for tests that supply the
     * spec as an in-memory resource rather than a file on disk.
     *
     * @param yaml raw UTF-8 bytes of the rules YAML
     * @return a compiled engine
     */
    public static Engine newEngineFromSpec(byte[] yaml) {
        return compileSpec(yaml);
    }

    // ── Validation API ─────────────────────────────────────────────────────────

    /**
     * Validates whether transitioning from {@code current} to {@code next}
     * under the given {@code flow} is permitted for the supplied context.
     *
     * <p>Call order:
     * <ol>
     *   <li>Null / zero-value guard
     *   <li>Terminal check on {@code current}
     *   <li>Adjacency lookup
     *   <li>Override / operatorId check
     *   <li>Inter-system detection
     * </ol>
     *
     * @param current the shipment's present status
     * @param next    the desired next status
     * @param flow    direction of the lifecycle
     * @param ctx     caller context (source system, operator)
     * @return a {@link ValidationResult} describing the outcome
     */
    public ValidationResult validate(Status current,
                                     Status next,
                                     Flow flow,
                                     TransitionContext ctx) {
        validationsTotal.incrementAndGet();

        // 1. Zero-value / null guard
        if (current == null || next == null) {
            zeroValueRejections.incrementAndGet();
            invalidTransitions.incrementAndGet();
            return ValidationResult.error(
                    ErrorCode.ZERO_VALUE_STATUS,
                    "current and next status must both be non-null");
        }

        // 2. Terminal check — no transitions out of a terminal status
        if (terminals[current.id]) {
            invalidTransitions.incrementAndGet();
            return ValidationResult.error(
                    ErrorCode.TERMINAL_STATUS,
                    current.code + " is a terminal status; no further transitions are allowed");
        }

        // 2b. First-mile regression check
        if (postFirstMile[current.id] && firstMile[next.id]) {
            invalidTransitions.incrementAndGet();
            return ValidationResult.error(
                    ErrorCode.FIRST_MILE_AFTER_LATER_MILE,
                    "first-mile status " + next.code + " cannot follow middle-mile or last-mile status " + current.code);
        }

        // 3. Adjacency lookup
        int key       = packKey(current.id, next.id, flow);
        int flags     = adj[key] & 0xFFFF;

        if ((flags & FLAG_PRESENT) == 0) {
            invalidTransitions.incrementAndGet();
            return ValidationResult.error(
                    ErrorCode.INVALID_TRANSITION,
                    current.code + " → " + next.code + " (" + flow + ") is not a permitted transition");
        }

        // 4. Override / operatorId requirement
        boolean requiresOpId = (flags & FLAG_REQUIRES_OPID) != 0;
        boolean isOverride   = (flags & FLAG_OVERRIDE)      != 0;

        if (requiresOpId && (ctx.operatorId() == null || ctx.operatorId().isBlank())) {
            invalidTransitions.incrementAndGet();
            return ValidationResult.error(
                    ErrorCode.MISSING_OPERATOR_ID,
                    "Transition " + current.code + " → " + next.code
                            + " requires a non-blank operatorId in the context");
        }

        // 5. Inter-system detection
        boolean isInterSystem = (flags & FLAG_INTER_SYSTEM) != 0;

        // Update metrics
        if (isOverride)     overrideTransitions.incrementAndGet();
        if (isInterSystem)  interSystemTransitions.incrementAndGet();

        // Build result
        if (isOverride && isInterSystem) return ValidationResult.okOverrideInterSystem();
        if (isOverride)                  return ValidationResult.okOverride();
        if (isInterSystem)               return ValidationResult.okInterSystem();
        return ValidationResult.ok();
    }

    /**
     * Validates that {@code first} is a permissible initial status for a new
     * shipment (i.e. no previous status exists).
     *
     * @param first the proposed first status
     * @param flow  direction of the lifecycle
     * @param ctx   caller context
     * @return a {@link ValidationResult} describing the outcome
     */
    public ValidationResult validateInitial(Status first, Flow flow, TransitionContext ctx) {
        validationsTotal.incrementAndGet();

        if (first == null) {
            zeroValueRejections.incrementAndGet();
            invalidTransitions.incrementAndGet();
            return ValidationResult.error(
                    ErrorCode.ZERO_VALUE_STATUS,
                    "first status must be non-null");
        }

        int flowBit = (flow == Flow.REVERSE) ? 1 : 0;

        for (int[] entry : initials) {
            if (entry[0] == first.id && entry[1] == flowBit) {
                return ValidationResult.ok();
            }
        }

        invalidTransitions.incrementAndGet();
        return ValidationResult.error(
                ErrorCode.INVALID_TRANSITION,
                first.code + " (" + flow + ") is not a valid initial status");
    }

    /**
     * Returns all statuses that are reachable from {@code current} under
     * {@code flow} according to the compiled spec.
     *
     * @param current the shipment's present status
     * @param flow    direction of the lifecycle
     * @return an immutable list of permitted next statuses (may be empty)
     */
    public List<Status> permittedTransitions(Status current, Flow flow) {
        if (current == null || terminals[current.id]) {
            return Collections.emptyList();
        }

        List<Status> result = new ArrayList<>();
        int flowBit = (flow == Flow.REVERSE) ? 1 : 0;

        for (Status next : Status.values()) {
            int key   = packKey(current.id, next.id, flow);
            int flags = adj[key] & 0xFFFF;
            if ((flags & FLAG_PRESENT) != 0) {
                result.add(next);
            }
        }

        return Collections.unmodifiableList(result);
    }

    // ── Metadata & metrics ────────────────────────────────────────────────────

    /**
     * Returns the {@code spec_version} string from the compiled YAML spec.
     *
     * @return spec version, e.g. {@code "1.0.0"}
     */
    public String specVersion() {
        return specVersion;
    }

    /**
     * Returns a point-in-time snapshot of all validation counters.
     *
     * @return an immutable {@link MetricsSnapshot}
     */
    public MetricsSnapshot snapshot() {
        return new MetricsSnapshot(
                validationsTotal.get(),
                invalidTransitions.get(),
                overrideTransitions.get(),
                interSystemTransitions.get(),
                zeroValueRejections.get());
    }

    // ── Spec compilation ───────────────────────────────────────────────────────

    /**
     * Compiles the YAML spec into the in-memory adjacency table.
     *
     * <p><b>Algorithm (mirrors Go implementation):</b>
     * <ol>
     *   <li>Parse YAML into a raw {@code Map<String, Object>}.
     *   <li>Build a {@code code → Status} lookup and populate {@code owners[]}
     *       and {@code terminals[]}.
     *   <li>Collect wildcard-target codes and initial-status entries.
     *   <li>For each transition entry, resolve the {@code from} code, normalise
     *       the {@code to} field (String or List<String>), and write edge flags.
     *   <li>For every non-terminal status and every wildcard target, write a
     *       wildcard edge in both flows.
     * </ol>
     */
    @SuppressWarnings("unchecked")
    private static Engine compileSpec(byte[] yaml) {

        // ── 1. Parse YAML ──────────────────────────────────────────────────────
        Yaml parser = new Yaml();
        Map<String, Object> root = parser.load(new String(yaml));

        String specVersion = (String) root.get("spec_version");
        if (specVersion == null) specVersion = "";

        // ── 2. Build status registry ───────────────────────────────────────────
        // code→id map (YAML ids are 0-based; we store them as id+1)
        Map<String, Integer> codeToId = new HashMap<>();
        // Indexed by Java id (1-based)
        SourceSystem[] owners       = new SourceSystem[Status.MAX_ID + 1];
        boolean[]      terminals    = new boolean[Status.MAX_ID + 1];
        boolean[]      firstMile    = new boolean[Status.MAX_ID + 1];
        boolean[]      postFirstMile = new boolean[Status.MAX_ID + 1];

        List<Map<String, Object>> statuses =
                (List<Map<String, Object>>) root.get("statuses");

        for (Map<String, Object> entry : statuses) {
            String code   = (String) entry.get("code");
            int    yamlId = (Integer) entry.get("id");
            int    javaId = yamlId + 1;           // +1: 0 is the zero-value sentinel

            codeToId.put(code, javaId);

            String ownedBy = (String) entry.get("owned_by");
            owners[javaId] = SourceSystem.fromYaml(ownedBy);

            Object terminalObj = entry.get("terminal");
            terminals[javaId]  = Boolean.TRUE.equals(terminalObj);

            String category = entry.containsKey("category") ? ((String) entry.get("category")).toUpperCase() : "";
            if ("FIRST_MILE".equals(category)) {
                firstMile[javaId] = true;
            } else if ("MIDDLE_MILE".equals(category) || "LAST_MILE".equals(category)) {
                postFirstMile[javaId] = true;
            }
        }

        // ── 3. Wildcard targets ────────────────────────────────────────────────
        List<String> wildcardCodes = new ArrayList<>();
        Object wildcardRaw = root.get("wildcard_targets");
        if (wildcardRaw instanceof List<?> wcList) {
            for (Object wc : wcList) {
                wildcardCodes.add((String) wc);
            }
        }

        // ── 4. Initial statuses ────────────────────────────────────────────────
        List<int[]> initials = new ArrayList<>();
        Object initialRaw = root.get("initial_statuses");
        if (initialRaw instanceof List<?> initList) {
            for (Object item : initList) {
                Map<String, Object> initEntry = (Map<String, Object>) item;
                String code = (String) initEntry.get("code");
                String flowStr = (String) initEntry.get("flow");
                Integer id = codeToId.get(code);
                if (id == null) continue;
                int flowBit = "REVERSE".equals(flowStr) ? 1 : 0;
                initials.add(new int[]{id, flowBit});
            }
        }

        // ── 5. Build adjacency table ───────────────────────────────────────────
        short[] adj = new short[ADJ_SIZE];

        List<Map<String, Object>> transitions =
                (List<Map<String, Object>>) root.get("transitions");

        for (Map<String, Object> tx : transitions) {
            String fromCode = (String) tx.get("from");
            Integer fromId  = codeToId.get(fromCode);
            if (fromId == null) continue;                // defensive: unknown status

            // 'flow' field maps to a Flow enum
            String flowStr = (String) tx.get("flow");
            Flow flow      = "REVERSE".equals(flowStr) ? Flow.REVERSE : Flow.FORWARD;

            // Flags on this edge
            boolean isOverride   = Boolean.TRUE.equals(tx.get("is_override"));
            boolean requiresOpId = false;

            Object ctxRaw = tx.get("requires_context");
            if (ctxRaw instanceof List<?> ctxList) {
                for (Object c : ctxList) {
                    if ("operator_id".equals(c)) {
                        requiresOpId = true;
                    }
                }
            }

            // 'to' may be a single String or a List<String>
            List<String> toCodes = new ArrayList<>();
            Object toRaw = tx.get("to");
            if (toRaw instanceof String singleTo) {
                toCodes.add(singleTo);
            } else if (toRaw instanceof List<?> toList) {
                for (Object t : toList) {
                    toCodes.add((String) t);
                }
            }

            for (String toCode : toCodes) {
                Integer toId = codeToId.get(toCode);
                if (toId == null) continue;              // defensive: unknown status

                // Determine inter-system flag
                boolean interSystem = !owners[fromId].equals(owners[toId]);

                // Compute flags
                short flags = FLAG_PRESENT;
                if (isOverride)   flags |= FLAG_OVERRIDE;
                if (requiresOpId) flags |= FLAG_REQUIRES_OPID;
                if (interSystem)  flags |= FLAG_INTER_SYSTEM;

                int key = packKey(fromId, toId, flow);
                adj[key] |= flags;
            }
        }

        // ── 6. Expand wildcards ────────────────────────────────────────────────
        // Every non-terminal status may transition to any wildcard target in
        // either flow direction.
        for (int fromId = 1; fromId <= Status.MAX_ID; fromId++) {
            if (terminals[fromId]) continue;

            for (String wcCode : wildcardCodes) {
                Integer toId = codeToId.get(wcCode);
                if (toId == null) continue;

                boolean interSystem = !owners[fromId].equals(owners[toId]);
                short   flags       = FLAG_PRESENT;
                if (interSystem) flags |= FLAG_INTER_SYSTEM;

                // Apply in both directions
                for (Flow f : Flow.values()) {
                    int key = packKey(fromId, toId, f);
                    adj[key] |= flags;
                }
            }
        }

        return new Engine(adj, owners, terminals, firstMile, postFirstMile, initials, specVersion);
    }

    // ── Key packing ────────────────────────────────────────────────────────────

    /**
     * Packs a (from, to, flow) triple into a single integer adjacency-table key.
     *
     * <pre>
     *   key = (fromId &lt;&lt; 7) | (toId &lt;&lt; 1) | flowBit
     * </pre>
     *
     * With IDs in the range [1, 55]:
     * <ul>
     *   <li>{@code fromId << 7} uses bits 7–12 (max = 55 × 128 = 7040)
     *   <li>{@code toId   << 1} uses bits 1–6  (max = 55 × 2   =  110)
     *   <li>{@code flowBit}     uses bit  0    (0 = FORWARD, 1 = REVERSE)
     * </ul>
     * Maximum key = 7040 + 110 + 1 = 7151 &lt; 8192 — always in bounds.
     *
     * @param fromId  source status ID (1-based)
     * @param toId    destination status ID (1-based)
     * @param flow    transition direction
     * @return packed key suitable as an index into {@link #adj}
     */
    private static int packKey(int fromId, int toId, Flow flow) {
        int flowBit = (flow == Flow.REVERSE) ? 1 : 0;
        return (fromId << 7) | (toId << 1) | flowBit;
    }
}
