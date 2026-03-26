'use strict';

/**
 * @fileoverview Public API surface for @logistics-oss/status-rules.
 *
 * Everything a caller needs is re-exported from this single entry point.
 *
 * @example
 * const {
 *   Engine, Status, Flow, SourceSystem, ErrorCode,
 *   MAX_ID, STATUS_NAMES,
 * } = require('@logistics-oss/status-rules');
 *
 * const engine = new Engine();
 * const result = engine.validate(
 *   'HUB-IN-SCANNED', 'HUB-OUTSCAN',
 *   Flow.FORWARD,
 *   { sourceSystem: SourceSystem.HUB },
 * );
 * if (!result.valid) throw new Error(result.errorCode);
 */

const { Engine }                       = require('./engine');
const { Status, MAX_ID, STATUS_NAMES } = require('./status');

// ── Flow direction constants ───────────────────────────────────────────────────

/**
 * Permitted values for the `flow` parameter.
 *
 * @type {{ FORWARD: 'FORWARD', REVERSE: 'REVERSE' }}
 */
const Flow = Object.freeze({
  FORWARD: 'FORWARD',
  REVERSE: 'REVERSE',
});

// ── Source system identifiers ─────────────────────────────────────────────────

/**
 * Canonical source-system identifiers used in the spec's `owned_by` field and
 * in ValidationContext.sourceSystem.
 *
 * @type {{ BOOKING: string, HUB: string, DELIVERY: string, TPL: string }}
 */
const SourceSystem = Object.freeze({
  BOOKING:  'BOOKING_SYSTEM',
  HUB:      'HUB_SYSTEM',
  DELIVERY: 'DELIVERY_SYSTEM',
  TPL:      'SYSTEM_3PL',
});

// ── Error code constants ──────────────────────────────────────────────────────

/**
 * All error codes that can appear in ValidationResult.errorCode.
 *
 * @type {Object}
 */
const ErrorCode = Object.freeze({
  /** Transition is permitted. */
  NONE:                 'ErrNone',
  /** current or next was empty / zero-value. */
  ZERO_VALUE_STATUS:    'ErrZeroValueStatus',
  /** current or next is not a recognised status code. */
  UNKNOWN_STATUS:       'ErrUnknownStatus',
  /** current is a terminal status; no transitions are allowed. */
  TERMINAL_STATUS:      'ErrTerminalStatus',
  /** The edge exists but only in the opposite flow direction. */
  FLOW_MISMATCH:        'ErrFlowMismatch',
  /** No edge exists between current and next in any flow. */
  INVALID_TRANSITION:   'ErrInvalidTransition',
  /** The transition requires an operatorId but none was supplied. */
  MISSING_OPERATOR_ID:  'ErrMissingOperatorID',
  /** The destination is owned by a different system than the caller. */
  INVALID_SOURCE_SYSTEM:'ErrInvalidSourceSystem',
});

module.exports = {
  Engine,
  Status,
  MAX_ID,
  STATUS_NAMES,
  Flow,
  SourceSystem,
  ErrorCode,
};
