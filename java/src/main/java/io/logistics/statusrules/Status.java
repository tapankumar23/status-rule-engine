package io.logistics.statusrules;

import java.util.HashMap;
import java.util.Map;

/**
 * Every shipment status known to the rule engine.
 *
 * <p><b>ID contract:</b> {@code id} is the YAML 0-based {@code id} field plus one,
 * matching the Go convention where Status(0) is the zero/uninitialised sentinel.
 * IDs are a stable wire contract and must never be renumbered.
 *
 * <p><b>Code contract:</b> {@code code} is the original YAML string, including
 * hyphens (e.g. {@code "HUB-IN-SCANNED"}) and numeric prefixes (e.g.
 * {@code "3PL_ITEM_BOOK"}). Enum constant names use Java-safe equivalents only
 * where the YAML string is not a legal identifier.
 */
public enum Status {

    // ── ORDER_BOOKING / PRE_MILE ───────────────────────────────────────────────
    DRAFT                  ("DRAFT",                  1),
    BOOKED                 ("BOOKED",                 2),
    PART_PAYMENT_PENDING   ("PART_PAYMENT_PENDING",   3),
    UPDATED_BOOKING        ("UPDATED_BOOKING",        4),
    MANIFESTED             ("MANIFESTED",             5),

    // ── FIRST_MILE ─────────────────────────────────────────────────────────────
    READY_FOR_PICKUP       ("READY_FOR_PICKUP",       6),
    INSCANNED              ("INSCANNED",              7),

    // ── MIDDLE_MILE ────────────────────────────────────────────────────────────
    /** YAML code: {@code HUB-IN-SCANNED} */
    HUB_IN_SCANNED         ("HUB-IN-SCANNED",         8),
    BAGGED                 ("BAGGED",                 9),
    BAG_CREATED            ("BAG_CREATED",           10),
    BAG_FINALISED          ("BAG_FINALISED",         11),
    BAG_DELETED            ("BAG_DELETED",           12),
    INSCANNED_AT_TRANSIT   ("INSCANNED_AT_TRANSIT",  13),
    /** YAML code: {@code HUB-OUTSCAN} */
    HUB_OUTSCAN            ("HUB-OUTSCAN",           14),
    REMOVED_FROM_BAG       ("REMOVED_FROM_BAG",      15),
    REMOVED_FROM_LCR       ("REMOVED_FROM_LCR",      16),
    /** YAML code: {@code IN-BAG-INSCAN} */
    IN_BAG_INSCAN          ("IN-BAG-INSCAN",         17),
    /** YAML code: {@code IN-BAG-OUTSCAN} */
    IN_BAG_OUTSCAN         ("IN-BAG-OUTSCAN",        18),
    IN_BAG_OUTSCAN_TO_CP   ("IN_BAG_OUTSCAN_TO_CP",  19),
    OUT_SCAN_TO_CP         ("OUT_SCAN_TO_CP",        20),
    OUT_SCAN_TO_3PL        ("OUT_SCAN_TO_3PL",       21),

    // ── LAST_MILE ──────────────────────────────────────────────────────────────
    INSCANNED_AT_CP        ("INSCANNED_AT_CP",       22),
    SCHEDULED_FOR_TRIP     ("SCHEDULED_FOR_TRIP",    23),
    OUT_FOR_DELIVERY       ("OUT_FOR_DELIVERY",      24),
    ATTEMPTED              ("ATTEMPTED",             25),
    UNDELIVERED            ("UNDELIVERED",           26),
    RETURN_INITIATED       ("RETURN_INITIATED",      27),
    RETURN_REVOKED         ("RETURN_REVOKED",        28),

    // ── TERMINAL ───────────────────────────────────────────────────────────────
    DELIVERED              ("DELIVERED",             29),
    RTO_DELIVERED          ("RTO_DELIVERED",         30),
    CANCELLED              ("CANCELLED",             31),
    REJECTED               ("REJECTED",              32),
    REJECTED_BY_HO         ("REJECTED_BY_HO",        33),

    // ── REVERSE / RTO ──────────────────────────────────────────────────────────
    RTO                    ("RTO",                   34),
    RTO_OUT_FOR_DELIVERY   ("RTO_OUT_FOR_DELIVERY",  35),
    RTO_UNDELIVERED        ("RTO_UNDELIVERED",       36),

    // ── EXCEPTION WILDCARDS ────────────────────────────────────────────────────
    DAMAGED                ("DAMAGED",               37),
    MIS_ROUTED             ("MIS_ROUTED",            38),
    REROUTED               ("REROUTED",              39),
    EXCESS_INSCAN          ("EXCESS_INSCAN",         40),
    PINCODE_AUDITED        ("PINCODE_AUDITED",       41),
    WEIGHT_AUDITED         ("WEIGHT_AUDITED",        42),
    MODE_AUDITED           ("MODE_AUDITED",          43),

    // ── FORCE / OVERRIDE ───────────────────────────────────────────────────────
    FORCE_HUB_OUTSCAN      ("FORCE_HUB_OUTSCAN",     44),
    FORCE_BAG              ("FORCE_BAG",             45),
    FORCE_BAG_ATTEMPTED    ("FORCE_BAG_ATTEMPTED",   46),
    FORCE_OUTSCAN_TO_CP    ("FORCE_OUTSCAN_TO_CP",   47),

    // ── 3PL ────────────────────────────────────────────────────────────────────
    /** YAML code: {@code 3PL_ITEM_BOOK} */
    TPL_ITEM_BOOK          ("3PL_ITEM_BOOK",         48),
    /** YAML code: {@code 3PL_ITEM_DELIVERY} */
    TPL_ITEM_DELIVERY      ("3PL_ITEM_DELIVERY",     49),
    /** YAML code: {@code 3PL_ITEM_ONHOLD} */
    TPL_ITEM_ONHOLD        ("3PL_ITEM_ONHOLD",       50),
    /** YAML code: {@code 3PL_ITEM_REDIRECT} */
    TPL_ITEM_REDIRECT      ("3PL_ITEM_REDIRECT",     51),
    /** YAML code: {@code 3PL_ITEM_RETURN} */
    TPL_ITEM_RETURN        ("3PL_ITEM_RETURN",       52),
    /** YAML code: {@code 3PL_BAG_CLOSE} */
    TPL_BAG_CLOSE          ("3PL_BAG_CLOSE",         53),
    /** YAML code: {@code 3PL_BAG_DISPATCH} */
    TPL_BAG_DISPATCH       ("3PL_BAG_DISPATCH",      54),
    /** YAML code: {@code 3PL_BAG_OPEN} */
    TPL_BAG_OPEN           ("3PL_BAG_OPEN",          55);

    // ── Fields ─────────────────────────────────────────────────────────────────

    /** Stable numeric ID (YAML id + 1). Used as the adjacency-table coordinate. */
    public final int id;

    /** Original YAML code string, used for serialisation and lookup. */
    public final String code;

    /**
     * Maximum valid status ID (= number of statuses in the spec).
     * Used to size arrays and validate bounds.
     */
    public static final int MAX_ID = 55;

    Status(String code, int id) {
        this.code = code;
        this.id   = id;
    }

    // ── Static lookup ──────────────────────────────────────────────────────────

    private static final Map<String, Status> BY_CODE = new HashMap<>();

    static {
        for (Status s : values()) {
            BY_CODE.put(s.code, s);
        }
    }

    /**
     * Returns the {@link Status} whose {@code code} equals the supplied string.
     *
     * @param code the YAML code string (e.g. {@code "HUB-IN-SCANNED"})
     * @return the matching constant, or {@code null} if not found
     */
    public static Status fromCode(String code) {
        return BY_CODE.get(code);
    }
}
