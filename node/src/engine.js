'use strict';

/**
 * @fileoverview Status rule engine.
 *
 * Compiles the YAML spec into a flat Uint16Array adjacency table and exposes
 * validate / validateInitial / permittedTransitions helpers.  The design
 * mirrors the Go implementation exactly so that both can be used as reference
 * implementations for compatibility testing.
 *
 * Key packing (matches Go):
 *   key = (fromId << 7) | (toId << 1) | flowBit
 *   flowBit: FORWARD=0, REVERSE=1
 *   Array size: 8192 (covers ids 0-63 × 0-63 × 2 flows)
 *
 * Edge flags (uint16 bit positions):
 *   bit 0 (1):  flagPresent        — transition exists
 *   bit 1 (2):  flagOverride       — FORCE_* edge
 *   bit 2 (4):  flagRequiresOpID   — needs operatorId in context
 *   bit 3 (8):  flagInterSystem    — crosses system ownership boundary
 */

const path = require('path');
const fs   = require('fs');
const yaml = require('js-yaml');

// ── Bit-flag constants ────────────────────────────────────────────────────────

const FLAG_PRESENT         = 1;  // bit 0
const FLAG_OVERRIDE        = 2;  // bit 1
const FLAG_REQUIRES_OP_ID  = 4;  // bit 2
const FLAG_INTER_SYSTEM    = 8;  // bit 3

// ── Adjacency table size ──────────────────────────────────────────────────────

const ADJ_SIZE = 8192; // 64 × 64 × 2

// ── Error codes ───────────────────────────────────────────────────────────────

const ERR_NONE                       = 'ErrNone';
const ERR_ZERO_VALUE                 = 'ErrZeroValueStatus';
const ERR_UNKNOWN_STATUS             = 'ErrUnknownStatus';
const ERR_TERMINAL_STATUS            = 'ErrTerminalStatus';
const ERR_FLOW_MISMATCH              = 'ErrFlowMismatch';
const ERR_INVALID_TRANSITION         = 'ErrInvalidTransition';
const ERR_MISSING_OP_ID              = 'ErrMissingOperatorID';
const ERR_INVALID_SRC_SYS            = 'ErrInvalidSourceSystem';
const ERR_FIRST_MILE_AFTER_LATER_MILE = 'ErrFirstMileAfterLaterMile';

// ── Internal: spec compiler ───────────────────────────────────────────────────

/**
 * Compiled spec data produced by compileSpec().
 *
 * @typedef {Object} CompiledSpec
 * @property {Uint16Array}        adj         - Adjacency table (8192 slots).
 * @property {Map<string,number>} codeToId    - YAML code → numeric id.
 * @property {string[]}           owners      - owners[id] = owned_by string.
 * @property {boolean[]}          terminals   - terminals[id] = true if terminal.
 * @property {Set<string>}        initials    - "code:FLOW" strings for valid first statuses.
 * @property {string}             specVersion - Parsed spec_version field.
 */

/**
 * Compile a YAML spec string into an adjacency table and supporting lookup
 * structures.  This is a pure function — it does not read files or produce
 * side-effects.
 *
 * @param {string} specText - Raw YAML content.
 * @returns {CompiledSpec}
 */
function compileSpec(specText) {
  /** @type {any} */
  const doc = yaml.load(specText);

  const specVersion = String(doc.spec_version || '');

  // ── Step 1: build codeToId (YAML code → yamlId+1) ────────────────────────
  // Slot count: YAML ids are 0-54 → JS ids 1-55.  We allocate 56 slots so
  // that index 55 is valid and index 0 stays the zero sentinel.
  const SLOT_COUNT = 56;

  /** @type {Map<string, number>} */
  const codeToId = new Map();

  /** @type {string[]}  owners[id] = owned_by */
  const owners = new Array(SLOT_COUNT).fill('');

  /** @type {boolean[]} terminals[id] */
  const terminals = new Array(SLOT_COUNT).fill(false);

  /** @type {boolean[]} firstMile[id] — true for FIRST_MILE category statuses */
  const firstMile = new Array(SLOT_COUNT).fill(false);

  /** @type {boolean[]} postFirstMile[id] — true for MIDDLE_MILE or LAST_MILE category statuses */
  const postFirstMile = new Array(SLOT_COUNT).fill(false);

  for (const s of doc.statuses) {
    const id = s.id + 1; // shift: YAML 0-indexed → JS 1-indexed
    codeToId.set(s.code, id);
    owners[id]    = s.owned_by || '';
    terminals[id] = s.terminal === true;
    const cat = (s.category || '').toUpperCase();
    if (cat === 'FIRST_MILE') {
      firstMile[id] = true;
    } else if (cat === 'MIDDLE_MILE' || cat === 'LAST_MILE') {
      postFirstMile[id] = true;
    }
  }

  // ── Step 2: adjacency table ───────────────────────────────────────────────
  const adj = new Uint16Array(ADJ_SIZE);

  /**
   * Pack fromId, toId and flowBit into a table index.
   * @param {number} from
   * @param {number} to
   * @param {number} flowBit - 0=FORWARD, 1=REVERSE
   * @returns {number}
   */
  function key(from, to, flowBit) {
    return (from << 7) | (to << 1) | flowBit;
  }

  /**
   * Return true when fromId and toId belong to different owning systems.
   * @param {number} fromId
   * @param {number} toId
   * @returns {boolean}
   */
  function isInterSystem(fromId, toId) {
    return owners[fromId] !== owners[toId];
  }

  // ── Step 3: expand wildcard targets ──────────────────────────────────────
  // For every non-terminal from-status and every wildcard to-status, in both
  // flows, write the base edge (flagPresent | optionally flagInterSystem).
  // Explicit transitions written in Step 4 will overwrite these entries.

  const wildcardTargets = /** @type {string[]} */ (doc.wildcard_targets || []);

  for (const s of doc.statuses) {
    const fromId = s.id + 1;
    if (terminals[fromId]) continue; // terminal statuses cannot be a source

    for (const wCode of wildcardTargets) {
      const toId = codeToId.get(wCode);
      if (toId === undefined) continue;

      for (const flowBit of [0, 1]) {
        const flags = FLAG_PRESENT | (isInterSystem(fromId, toId) ? FLAG_INTER_SYSTEM : 0);
        adj[key(fromId, toId, flowBit)] = flags;
      }
    }
  }

  // ── Step 4: apply explicit transitions (overwrites wildcard entries) ──────

  for (const t of doc.transitions) {
    const fromId = codeToId.get(t.from);
    if (fromId === undefined) continue;

    const flowBit = t.flow === 'REVERSE' ? 1 : 0;
    const isOverride    = t.is_override === true;
    const requiresOpId  = Array.isArray(t.requires_context) &&
                          t.requires_context.includes('operator_id');

    // `to` may be a single string or an array
    const targets = Array.isArray(t.to) ? t.to : [t.to];

    for (const toCode of targets) {
      const toId = codeToId.get(toCode);
      if (toId === undefined) continue;

      const flags =
        FLAG_PRESENT |
        (isOverride   ? FLAG_OVERRIDE       : 0) |
        (requiresOpId ? FLAG_REQUIRES_OP_ID : 0) |
        (isInterSystem(fromId, toId) ? FLAG_INTER_SYSTEM : 0);

      adj[key(fromId, toId, flowBit)] = flags;
    }
  }

  // ── Step 5: build initials set ────────────────────────────────────────────
  /** @type {Set<string>} */
  const initials = new Set();
  for (const entry of (doc.initial_statuses || [])) {
    initials.add(`${entry.code}:${entry.flow}`);
  }

  return { adj, codeToId, owners, terminals, firstMile, postFirstMile, initials, specVersion };
}

// ── Engine class ──────────────────────────────────────────────────────────────

/**
 * Validation context passed to validate().
 *
 * @typedef {Object} ValidationContext
 * @property {string} [operatorId]   - Required for FORCE_* transitions.
 * @property {string} [sourceSystem] - Caller's system identity; validated on
 *                                     inter-system edges.
 */

/**
 * Result returned by validate() and validateInitial().
 *
 * @typedef {Object} ValidationResult
 * @property {boolean} valid          - Whether the transition is permitted.
 * @property {boolean} isOverride     - True for FORCE_* override edges.
 * @property {boolean} isInterSystem  - True when the edge crosses an ownership
 *                                      boundary between source systems.
 * @property {string}  errorCode      - One of the ErrXxx constants; 'ErrNone'
 *                                      on success.
 */

/**
 * @typedef {Object} PermittedTransition
 * @property {string}  code         - Destination status code (YAML name).
 * @property {boolean} isOverride
 * @property {boolean} isInterSystem
 * @property {boolean} requiresOperatorId
 */

/**
 * Snapshot of engine metrics returned by snapshot().
 *
 * @typedef {Object} EngineSnapshot
 * @property {string} specVersion
 * @property {number} cntTotal
 * @property {number} cntInvalid
 * @property {number} cntOverride
 * @property {number} cntInterSystem
 * @property {number} cntZeroValue
 */

/**
 * Shipment status rule engine.
 *
 * Reads the YAML spec at construction time and compiles it into a dense
 * adjacency table for O(1) lookups.
 *
 * @example
 * const { Engine, Flow, SourceSystem } = require('@logistics-oss/status-rules');
 * const engine = new Engine();
 * const result = engine.validate('HUB-IN-SCANNED', 'HUB-OUTSCAN', Flow.FORWARD, {});
 */
class Engine {
  constructor() {
    // Resolve the spec path relative to this source file so the package works
    // regardless of the working directory from which it is imported.
    const specPath = path.resolve(__dirname, '../../spec/rules.yaml');
    const specText = fs.readFileSync(specPath, 'utf8');

    /** @type {CompiledSpec} */
    this._spec = compileSpec(specText);

    // ── Metrics counters ──────────────────────────────────────────────────
    /** @type {number} Total calls to validate() */
    this.cntTotal        = 0;
    /** @type {number} Calls that returned valid=false */
    this.cntInvalid      = 0;
    /** @type {number} Successful calls where isOverride=true */
    this.cntOverride     = 0;
    /** @type {number} Successful calls where isInterSystem=true */
    this.cntInterSystem  = 0;
    /** @type {number} Calls that returned ErrZeroValueStatus */
    this.cntZeroValue    = 0;
  }

  // ── Public API ────────────────────────────────────────────────────────────

  /**
   * Spec version string from the YAML (e.g. "1.0.0").
   * @returns {string}
   */
  get specVersion() {
    return this._spec.specVersion;
  }

  /**
   * Validate a status transition.
   *
   * Follows the algorithm specified in the architecture document, matching the
   * Go implementation step-for-step.
   *
   * @param {string}            current - Current status code (YAML name).
   * @param {string}            next    - Desired next status code (YAML name).
   * @param {string}            flow    - 'FORWARD' or 'REVERSE'.
   * @param {ValidationContext} ctx     - Caller-supplied context.
   * @returns {ValidationResult}
   */
  validate(current, next, flow, ctx) {
    this.cntTotal++;

    // ── Guard: zero-value inputs ──────────────────────────────────────────
    if (!current || !next) {
      this.cntInvalid++;
      this.cntZeroValue++;
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_ZERO_VALUE };
    }

    const { adj, codeToId, owners, terminals, firstMile, postFirstMile } = this._spec;

    // ── Guard: unknown status codes ───────────────────────────────────────
    const currentId = codeToId.get(current);
    const nextId    = codeToId.get(next);

    if (currentId === undefined || nextId === undefined) {
      this.cntInvalid++;
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_UNKNOWN_STATUS };
    }

    // ── Guard: current status is terminal ────────────────────────────────
    if (terminals[currentId]) {
      this.cntInvalid++;
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_TERMINAL_STATUS };
    }

    // ── Guard: first-mile status cannot follow middle-mile or last-mile ───
    if (postFirstMile[currentId] && firstMile[nextId]) {
      this.cntInvalid++;
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_FIRST_MILE_AFTER_LATER_MILE };
    }

    // ── Look up edge in adjacency table ───────────────────────────────────
    const flowBit    = flow === 'FORWARD' ? 0 : 1;
    const edgeKey    = (currentId << 7) | (nextId << 1) | flowBit;
    const edge       = adj[edgeKey];

    if (!(edge & FLAG_PRESENT)) {
      // Edge not found in the requested flow — check if it exists in the
      // opposite flow to distinguish ErrFlowMismatch from ErrInvalidTransition.
      const altFlowBit = flowBit ^ 1;
      const altKey     = (currentId << 7) | (nextId << 1) | altFlowBit;
      const altEdge    = adj[altKey];

      this.cntInvalid++;
      if (altEdge & FLAG_PRESENT) {
        return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_FLOW_MISMATCH };
      }
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_INVALID_TRANSITION };
    }

    // ── Guard: operator id required ───────────────────────────────────────
    if ((edge & FLAG_REQUIRES_OP_ID) && !ctx.operatorId) {
      this.cntInvalid++;
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_MISSING_OP_ID };
    }

    // ── Guard: inter-system ownership check ───────────────────────────────
    const interSystem = !!(edge & FLAG_INTER_SYSTEM);
    if (interSystem && owners[nextId] !== ctx.sourceSystem) {
      this.cntInvalid++;
      return { valid: false, isOverride: false, isInterSystem: true, errorCode: ERR_INVALID_SRC_SYS };
    }

    // ── Success ───────────────────────────────────────────────────────────
    const override = !!(edge & FLAG_OVERRIDE);
    if (override)     this.cntOverride++;
    if (interSystem)  this.cntInterSystem++;

    return { valid: true, isOverride: override, isInterSystem: interSystem, errorCode: ERR_NONE };
  }

  /**
   * Validate the very first status of a new shipment.
   *
   * Unlike validate(), there is no prior status to check against — this method
   * consults the `initial_statuses` list in the spec.
   *
   * @param {string} first - The first status code.
   * @param {string} flow  - 'FORWARD' or 'REVERSE'.
   * @returns {ValidationResult}
   */
  validateInitial(first, flow) {
    if (!first) {
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_ZERO_VALUE };
    }

    if (!this._spec.codeToId.has(first)) {
      return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_UNKNOWN_STATUS };
    }

    const key = `${first}:${flow}`;
    if (this._spec.initials.has(key)) {
      return { valid: true, isOverride: false, isInterSystem: false, errorCode: ERR_NONE };
    }

    return { valid: false, isOverride: false, isInterSystem: false, errorCode: ERR_INVALID_TRANSITION };
  }

  /**
   * Return all permitted next statuses reachable from `current` in `flow`.
   *
   * @param {string} current - Current status code (YAML name).
   * @param {string} flow    - 'FORWARD' or 'REVERSE'.
   * @returns {PermittedTransition[]}
   */
  permittedTransitions(current, flow) {
    const { adj, codeToId, terminals } = this._spec;
    const { STATUS_NAMES } = require('./status');

    const currentId = codeToId.get(current);
    if (currentId === undefined || terminals[currentId]) return [];

    const flowBit = flow === 'FORWARD' ? 0 : 1;
    const results = [];

    // Iterate over all possible destination ids (1 … MAX_ID).
    const maxId = STATUS_NAMES.length - 1; // STATUS_NAMES[0] is the sentinel
    for (let toId = 1; toId <= maxId; toId++) {
      const edgeKey = (currentId << 7) | (toId << 1) | flowBit;
      const edge    = adj[edgeKey];
      if (!(edge & FLAG_PRESENT)) continue;

      results.push({
        code:               STATUS_NAMES[toId],
        isOverride:         !!(edge & FLAG_OVERRIDE),
        isInterSystem:      !!(edge & FLAG_INTER_SYSTEM),
        requiresOperatorId: !!(edge & FLAG_REQUIRES_OP_ID),
      });
    }

    return results;
  }

  /**
   * Return a point-in-time snapshot of engine metrics and spec metadata.
   *
   * @returns {EngineSnapshot}
   */
  snapshot() {
    return {
      specVersion:    this._spec.specVersion,
      cntTotal:       this.cntTotal,
      cntInvalid:     this.cntInvalid,
      cntOverride:    this.cntOverride,
      cntInterSystem: this.cntInterSystem,
      cntZeroValue:   this.cntZeroValue,
    };
  }
}

module.exports = { Engine };
