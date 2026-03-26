'use strict';

/**
 * @fileoverview Shipment status registry.
 *
 * IDs are YAML id + 1 so that JS id 0 is always the zero/uninitialised
 * sentinel — the same convention used by the Go implementation.
 *
 * Rules:
 *  - IDs must never be renumbered; they are a stable wire contract.
 *  - YAML codes that contain hyphens or start with a digit are mapped to
 *    safe JS identifiers (e.g. HUB-IN-SCANNED → HUB_IN_SCANNED).
 *    The original YAML codes are preserved in STATUS_NAMES.
 */

/**
 * Maximum status id (inclusive).  There are 55 statuses (YAML ids 0-54).
 * @type {number}
 */
const MAX_ID = 55;

/**
 * STATUS_NAMES[id] → original YAML code string.
 * Index 0 is the zero sentinel (empty string).
 *
 * @type {string[]}
 */
const STATUS_NAMES = [
  '',                    // 0  — zero sentinel (never a real status)
  'DRAFT',               // 1
  'BOOKED',              // 2
  'PART_PAYMENT_PENDING',// 3
  'UPDATED_BOOKING',     // 4
  'MANIFESTED',          // 5
  'READY_FOR_PICKUP',    // 6
  'INSCANNED',           // 7
  'HUB-IN-SCANNED',      // 8
  'BAGGED',              // 9
  'BAG_CREATED',         // 10
  'BAG_FINALISED',       // 11
  'BAG_DELETED',         // 12
  'INSCANNED_AT_TRANSIT',// 13
  'HUB-OUTSCAN',         // 14
  'REMOVED_FROM_BAG',    // 15
  'REMOVED_FROM_LCR',    // 16
  'IN-BAG-INSCAN',       // 17
  'IN-BAG-OUTSCAN',      // 18
  'IN_BAG_OUTSCAN_TO_CP',// 19
  'OUT_SCAN_TO_CP',      // 20
  'OUT_SCAN_TO_3PL',     // 21
  'INSCANNED_AT_CP',     // 22
  'SCHEDULED_FOR_TRIP',  // 23
  'OUT_FOR_DELIVERY',    // 24
  'ATTEMPTED',           // 25
  'UNDELIVERED',         // 26
  'RETURN_INITIATED',    // 27
  'RETURN_REVOKED',      // 28
  'DELIVERED',           // 29
  'RTO_DELIVERED',       // 30
  'CANCELLED',           // 31
  'REJECTED',            // 32
  'REJECTED_BY_HO',      // 33
  'RTO',                 // 34
  'RTO_OUT_FOR_DELIVERY',// 35
  'RTO_UNDELIVERED',     // 36
  'DAMAGED',             // 37
  'MIS_ROUTED',          // 38
  'REROUTED',            // 39
  'EXCESS_INSCAN',       // 40
  'PINCODE_AUDITED',     // 41
  'WEIGHT_AUDITED',      // 42
  'MODE_AUDITED',        // 43
  'FORCE_HUB_OUTSCAN',   // 44
  'FORCE_BAG',           // 45
  'FORCE_BAG_ATTEMPTED', // 46
  'FORCE_OUTSCAN_TO_CP', // 47
  '3PL_ITEM_BOOK',       // 48
  '3PL_ITEM_DELIVERY',   // 49
  '3PL_ITEM_ONHOLD',     // 50
  '3PL_ITEM_REDIRECT',   // 51
  '3PL_ITEM_RETURN',     // 52
  '3PL_BAG_CLOSE',       // 53
  '3PL_BAG_DISPATCH',    // 54
  '3PL_BAG_OPEN',        // 55
];

/**
 * Frozen map of JS-safe name → numeric id.
 *
 * Keys that could not be valid JS identifiers (hyphens, leading digit) are
 * aliased to underscore-safe names.  Both the aliased key and the original
 * string key are present so callers can use either form.
 *
 * @type {Readonly<Record<string, number>>}
 */
const Status = Object.freeze({
  // ── ORDER_BOOKING / PRE_MILE ─────────────────────────────────────────────
  DRAFT:                 1,
  BOOKED:                2,
  PART_PAYMENT_PENDING:  3,
  UPDATED_BOOKING:       4,
  MANIFESTED:            5,

  // ── FIRST_MILE ───────────────────────────────────────────────────────────
  READY_FOR_PICKUP:      6,
  INSCANNED:             7,

  // ── MIDDLE_MILE ──────────────────────────────────────────────────────────
  HUB_IN_SCANNED:        8,   // JS-safe alias for YAML 'HUB-IN-SCANNED'
  'HUB-IN-SCANNED':      8,   // original YAML key — kept for engine lookups
  BAGGED:                9,
  BAG_CREATED:           10,
  BAG_FINALISED:         11,
  BAG_DELETED:           12,
  INSCANNED_AT_TRANSIT:  13,
  HUB_OUTSCAN:           14,  // JS-safe alias for YAML 'HUB-OUTSCAN'
  'HUB-OUTSCAN':         14,  // original YAML key
  REMOVED_FROM_BAG:      15,
  REMOVED_FROM_LCR:      16,
  IN_BAG_INSCAN:         17,  // JS-safe alias for YAML 'IN-BAG-INSCAN'
  'IN-BAG-INSCAN':       17,  // original YAML key
  IN_BAG_OUTSCAN:        18,  // JS-safe alias for YAML 'IN-BAG-OUTSCAN'
  'IN-BAG-OUTSCAN':      18,  // original YAML key
  IN_BAG_OUTSCAN_TO_CP:  19,
  OUT_SCAN_TO_CP:        20,
  OUT_SCAN_TO_3PL:       21,

  // ── LAST_MILE ─────────────────────────────────────────────────────────────
  INSCANNED_AT_CP:       22,
  SCHEDULED_FOR_TRIP:    23,
  OUT_FOR_DELIVERY:      24,
  ATTEMPTED:             25,
  UNDELIVERED:           26,
  RETURN_INITIATED:      27,
  RETURN_REVOKED:        28,

  // ── TERMINAL ──────────────────────────────────────────────────────────────
  DELIVERED:             29,
  RTO_DELIVERED:         30,
  CANCELLED:             31,
  REJECTED:              32,
  REJECTED_BY_HO:        33,

  // ── RTO / REVERSE ─────────────────────────────────────────────────────────
  RTO:                   34,
  RTO_OUT_FOR_DELIVERY:  35,
  RTO_UNDELIVERED:       36,

  // ── EXCEPTION WILDCARDS ───────────────────────────────────────────────────
  DAMAGED:               37,
  MIS_ROUTED:            38,
  REROUTED:              39,
  EXCESS_INSCAN:         40,
  PINCODE_AUDITED:       41,
  WEIGHT_AUDITED:        42,
  MODE_AUDITED:          43,

  // ── FORCE_* OVERRIDES ─────────────────────────────────────────────────────
  FORCE_HUB_OUTSCAN:     44,
  FORCE_BAG:             45,
  FORCE_BAG_ATTEMPTED:   46,
  FORCE_OUTSCAN_TO_CP:   47,

  // ── 3PL ───────────────────────────────────────────────────────────────────
  TPL_ITEM_BOOK:         48,  // JS-safe alias for YAML '3PL_ITEM_BOOK'
  '3PL_ITEM_BOOK':       48,  // original YAML key
  TPL_ITEM_DELIVERY:     49,
  '3PL_ITEM_DELIVERY':   49,
  TPL_ITEM_ONHOLD:       50,
  '3PL_ITEM_ONHOLD':     50,
  TPL_ITEM_REDIRECT:     51,
  '3PL_ITEM_REDIRECT':   51,
  TPL_ITEM_RETURN:       52,
  '3PL_ITEM_RETURN':     52,
  TPL_BAG_CLOSE:         53,
  '3PL_BAG_CLOSE':       53,
  TPL_BAG_DISPATCH:      54,
  '3PL_BAG_DISPATCH':    54,
  TPL_BAG_OPEN:          55,
  '3PL_BAG_OPEN':        55,
});

module.exports = { Status, MAX_ID, STATUS_NAMES };
