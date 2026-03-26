"""
status.py — stable status-ID registry.

IDs are yaml_id + 1 so that Python int 0 remains the uninitialized/zero-value
sentinel (matching the Go implementation).  IDs MUST NOT be renumbered; they
are a stable wire contract shared with all consumers of the spec.

Naming rules applied to produce valid Python identifiers
---------------------------------------------------------
* Hyphens replaced with underscores  (HUB-IN-SCANNED → HUB_IN_SCANNED)
* Leading digits prefixed with THREE_PL_  (3PL_* → THREE_PL_*)
"""

from __future__ import annotations


class Status:
    """Integer constants for every shipment status defined in rules.yaml.

    Example::

        from statusrules.status import Status
        current = Status.HUB_IN_SCANNED   # 8
    """

    # ── ORDER_BOOKING / PRE_MILE ─────────────────────────────────────────────
    DRAFT                  = 1    # yaml id 0
    BOOKED                 = 2    # yaml id 1
    PART_PAYMENT_PENDING   = 3    # yaml id 2
    UPDATED_BOOKING        = 4    # yaml id 3
    MANIFESTED             = 5    # yaml id 4

    # ── FIRST_MILE ────────────────────────────────────────────────────────────
    READY_FOR_PICKUP       = 6    # yaml id 5
    INSCANNED              = 7    # yaml id 6

    # ── MIDDLE_MILE ───────────────────────────────────────────────────────────
    HUB_IN_SCANNED         = 8    # yaml id 7  (yaml code: HUB-IN-SCANNED)
    BAGGED                 = 9    # yaml id 8
    BAG_CREATED            = 10   # yaml id 9
    BAG_FINALISED          = 11   # yaml id 10
    BAG_DELETED            = 12   # yaml id 11
    INSCANNED_AT_TRANSIT   = 13   # yaml id 12
    HUB_OUTSCAN            = 14   # yaml id 13  (yaml code: HUB-OUTSCAN)
    REMOVED_FROM_BAG       = 15   # yaml id 14
    REMOVED_FROM_LCR       = 16   # yaml id 15
    IN_BAG_INSCAN          = 17   # yaml id 16  (yaml code: IN-BAG-INSCAN)
    IN_BAG_OUTSCAN         = 18   # yaml id 17  (yaml code: IN-BAG-OUTSCAN)
    IN_BAG_OUTSCAN_TO_CP   = 19   # yaml id 18
    OUT_SCAN_TO_CP         = 20   # yaml id 19
    OUT_SCAN_TO_3PL        = 21   # yaml id 20

    # ── LAST_MILE ─────────────────────────────────────────────────────────────
    INSCANNED_AT_CP        = 22   # yaml id 21
    SCHEDULED_FOR_TRIP     = 23   # yaml id 22
    OUT_FOR_DELIVERY       = 24   # yaml id 23
    ATTEMPTED              = 25   # yaml id 24
    UNDELIVERED            = 26   # yaml id 25
    RETURN_INITIATED       = 27   # yaml id 26
    RETURN_REVOKED         = 28   # yaml id 27

    # ── TERMINAL ─────────────────────────────────────────────────────────────
    DELIVERED              = 29   # yaml id 28
    RTO_DELIVERED          = 30   # yaml id 29
    CANCELLED              = 31   # yaml id 30
    REJECTED               = 32   # yaml id 31
    REJECTED_BY_HO         = 33   # yaml id 32

    # ── RTO / REVERSE ─────────────────────────────────────────────────────────
    RTO                    = 34   # yaml id 33
    RTO_OUT_FOR_DELIVERY   = 35   # yaml id 34
    RTO_UNDELIVERED        = 36   # yaml id 35

    # ── EXCEPTION WILDCARDS ───────────────────────────────────────────────────
    DAMAGED                = 37   # yaml id 36
    MIS_ROUTED             = 38   # yaml id 37
    REROUTED               = 39   # yaml id 38
    EXCESS_INSCAN          = 40   # yaml id 39
    PINCODE_AUDITED        = 41   # yaml id 40
    WEIGHT_AUDITED         = 42   # yaml id 41
    MODE_AUDITED           = 43   # yaml id 42

    # ── FORCE_* OVERRIDE ──────────────────────────────────────────────────────
    FORCE_HUB_OUTSCAN      = 44   # yaml id 43
    FORCE_BAG              = 45   # yaml id 44
    FORCE_BAG_ATTEMPTED    = 46   # yaml id 45
    FORCE_OUTSCAN_TO_CP    = 47   # yaml id 46

    # ── 3PL ───────────────────────────────────────────────────────────────────
    THREE_PL_ITEM_BOOK     = 48   # yaml id 47  (yaml code: 3PL_ITEM_BOOK)
    THREE_PL_ITEM_DELIVERY = 49   # yaml id 48  (yaml code: 3PL_ITEM_DELIVERY)
    THREE_PL_ITEM_ONHOLD   = 50   # yaml id 49  (yaml code: 3PL_ITEM_ONHOLD)
    THREE_PL_ITEM_REDIRECT = 51   # yaml id 50  (yaml code: 3PL_ITEM_REDIRECT)
    THREE_PL_ITEM_RETURN   = 52   # yaml id 51  (yaml code: 3PL_ITEM_RETURN)
    THREE_PL_BAG_CLOSE     = 53   # yaml id 52  (yaml code: 3PL_BAG_CLOSE)
    THREE_PL_BAG_DISPATCH  = 54   # yaml id 53  (yaml code: 3PL_BAG_DISPATCH)
    THREE_PL_BAG_OPEN      = 55   # yaml id 54  (yaml code: 3PL_BAG_OPEN)


# Highest valid status ID (yaml has 55 entries, 0-indexed → python IDs 1..55).
MAX_ID: int = 55

# ---------------------------------------------------------------------------
# Lookup tables — built once at module import time.
# ---------------------------------------------------------------------------

# Maps the exact YAML ``code`` string (e.g. "HUB-IN-SCANNED", "3PL_BAG_OPEN")
# to the Python integer ID used throughout the engine.
CODE_TO_ID: dict[str, int] = {
    'DRAFT':                1,
    'BOOKED':               2,
    'PART_PAYMENT_PENDING': 3,
    'UPDATED_BOOKING':      4,
    'MANIFESTED':           5,
    'READY_FOR_PICKUP':     6,
    'INSCANNED':            7,
    'HUB-IN-SCANNED':       8,
    'BAGGED':               9,
    'BAG_CREATED':          10,
    'BAG_FINALISED':        11,
    'BAG_DELETED':          12,
    'INSCANNED_AT_TRANSIT': 13,
    'HUB-OUTSCAN':          14,
    'REMOVED_FROM_BAG':     15,
    'REMOVED_FROM_LCR':     16,
    'IN-BAG-INSCAN':        17,
    'IN-BAG-OUTSCAN':       18,
    'IN_BAG_OUTSCAN_TO_CP': 19,
    'OUT_SCAN_TO_CP':       20,
    'OUT_SCAN_TO_3PL':      21,
    'INSCANNED_AT_CP':      22,
    'SCHEDULED_FOR_TRIP':   23,
    'OUT_FOR_DELIVERY':     24,
    'ATTEMPTED':            25,
    'UNDELIVERED':          26,
    'RETURN_INITIATED':     27,
    'RETURN_REVOKED':       28,
    'DELIVERED':            29,
    'RTO_DELIVERED':        30,
    'CANCELLED':            31,
    'REJECTED':             32,
    'REJECTED_BY_HO':       33,
    'RTO':                  34,
    'RTO_OUT_FOR_DELIVERY': 35,
    'RTO_UNDELIVERED':      36,
    'DAMAGED':              37,
    'MIS_ROUTED':           38,
    'REROUTED':             39,
    'EXCESS_INSCAN':        40,
    'PINCODE_AUDITED':      41,
    'WEIGHT_AUDITED':       42,
    'MODE_AUDITED':         43,
    'FORCE_HUB_OUTSCAN':    44,
    'FORCE_BAG':            45,
    'FORCE_BAG_ATTEMPTED':  46,
    'FORCE_OUTSCAN_TO_CP':  47,
    '3PL_ITEM_BOOK':        48,
    '3PL_ITEM_DELIVERY':    49,
    '3PL_ITEM_ONHOLD':      50,
    '3PL_ITEM_REDIRECT':    51,
    '3PL_ITEM_RETURN':      52,
    '3PL_BAG_CLOSE':        53,
    '3PL_BAG_DISPATCH':     54,
    '3PL_BAG_OPEN':         55,
}

# Inverse map: Python integer ID → YAML code string.
ID_TO_CODE: dict[int, str] = {v: k for k, v in CODE_TO_ID.items()}
