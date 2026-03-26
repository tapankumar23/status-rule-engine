'use strict';

/**
 * @fileoverview Engine integration tests.
 *
 * Run with:
 *   node --test test/engine.test.js
 *
 * Uses only Node.js built-in modules (node:test + node:assert) — no extra
 * test-runner dependencies are required.
 */

const { describe, it, before } = require('node:test');
const assert = require('node:assert/strict');

const {
  Engine,
  Status,
  Flow,
  SourceSystem,
  ErrorCode,
} = require('../src/index');

// ── Shared engine instance ────────────────────────────────────────────────────

let engine;

describe('Engine', () => {
  before(() => {
    engine = new Engine();
  });

  // ── Test 1: Valid forward transition ───────────────────────────────────────
  it('1. permits HUB_IN_SCANNED → HUB_OUTSCAN (FORWARD)', () => {
    const result = engine.validate(
      'HUB-IN-SCANNED',
      'HUB-OUTSCAN',
      Flow.FORWARD,
      { sourceSystem: SourceSystem.HUB },
    );

    assert.equal(result.valid, true,  'expected valid=true');
    assert.equal(result.errorCode, ErrorCode.NONE, 'expected ErrNone');
    assert.equal(result.isOverride, false, 'should not be an override');
  });

  // ── Test 2: Invalid transition ─────────────────────────────────────────────
  it('2. rejects HUB_IN_SCANNED → DELIVERED (FORWARD) with ErrInvalidTransition', () => {
    const result = engine.validate(
      'HUB-IN-SCANNED',
      'DELIVERED',
      Flow.FORWARD,
      { sourceSystem: SourceSystem.DELIVERY },
    );

    assert.equal(result.valid, false, 'expected valid=false');
    assert.equal(result.errorCode, ErrorCode.INVALID_TRANSITION);
  });

  // ── Test 3: Terminal status as source ──────────────────────────────────────
  it('3. rejects DELIVERED → anything with ErrTerminalStatus', () => {
    const result = engine.validate(
      'DELIVERED',
      'INSCANNED_AT_CP',
      Flow.FORWARD,
      {},
    );

    assert.equal(result.valid, false, 'expected valid=false');
    assert.equal(result.errorCode, ErrorCode.TERMINAL_STATUS);
  });

  // ── Test 4: FORCE_* transition without operatorId ─────────────────────────
  it('4. rejects HUB_IN_SCANNED → FORCE_HUB_OUTSCAN without operatorId', () => {
    const result = engine.validate(
      'HUB-IN-SCANNED',
      'FORCE_HUB_OUTSCAN',
      Flow.FORWARD,
      { sourceSystem: SourceSystem.HUB }, // no operatorId
    );

    assert.equal(result.valid, false, 'expected valid=false');
    assert.equal(result.errorCode, ErrorCode.MISSING_OPERATOR_ID);
  });

  // ── Test 5: FORCE_* transition with operatorId ────────────────────────────
  it('5. allows HUB_IN_SCANNED → FORCE_HUB_OUTSCAN with operatorId; isOverride=true', () => {
    const result = engine.validate(
      'HUB-IN-SCANNED',
      'FORCE_HUB_OUTSCAN',
      Flow.FORWARD,
      { sourceSystem: SourceSystem.HUB, operatorId: 'ops-user-42' },
    );

    assert.equal(result.valid, true, 'expected valid=true');
    assert.equal(result.isOverride, true, 'expected isOverride=true');
    assert.equal(result.errorCode, ErrorCode.NONE);
  });

  // ── Test 6: Wildcard target (any non-terminal → DAMAGED) ──────────────────
  it('6. permits OUT_FOR_DELIVERY → DAMAGED via wildcard expansion', () => {
    const result = engine.validate(
      'OUT_FOR_DELIVERY',
      'DAMAGED',
      Flow.FORWARD,
      { sourceSystem: SourceSystem.HUB },
    );

    assert.equal(result.valid, true, 'expected valid=true');
    assert.equal(result.errorCode, ErrorCode.NONE);
  });

  // ── Test 7: Zero-value status ─────────────────────────────────────────────
  it('7. rejects empty string current with ErrZeroValueStatus', () => {
    const result = engine.validate('', 'BOOKED', Flow.FORWARD, {});

    assert.equal(result.valid, false);
    assert.equal(result.errorCode, ErrorCode.ZERO_VALUE_STATUS);
  });

  it('7b. rejects empty string next with ErrZeroValueStatus', () => {
    const result = engine.validate('DRAFT', '', Flow.FORWARD, {});

    assert.equal(result.valid, false);
    assert.equal(result.errorCode, ErrorCode.ZERO_VALUE_STATUS);
  });

  // ── Test 8: validateInitial ────────────────────────────────────────────────
  it('8. validateInitial(DRAFT, FORWARD) returns valid=true', () => {
    const result = engine.validateInitial('DRAFT', Flow.FORWARD);

    assert.equal(result.valid, true);
    assert.equal(result.errorCode, ErrorCode.NONE);
  });

  it('8b. validateInitial(BOOKED, FORWARD) returns invalid (not an initial status)', () => {
    const result = engine.validateInitial('BOOKED', Flow.FORWARD);

    assert.equal(result.valid, false);
    assert.equal(result.errorCode, ErrorCode.INVALID_TRANSITION);
  });

  // ── Test 9: specVersion ────────────────────────────────────────────────────
  it("9. specVersion returns '1.0.0'", () => {
    assert.equal(engine.specVersion, '1.0.0');
  });

  // ── Test 10: permittedTransitions ─────────────────────────────────────────
  it('10. permittedTransitions returns non-empty array for a non-terminal status', () => {
    const transitions = engine.permittedTransitions('HUB-IN-SCANNED', Flow.FORWARD);

    assert.ok(Array.isArray(transitions), 'should return an array');
    assert.ok(transitions.length > 0,     'should have at least one permitted transition');

    // Sanity-check that each entry has the expected shape.
    for (const t of transitions) {
      assert.equal(typeof t.code,               'string');
      assert.equal(typeof t.isOverride,         'boolean');
      assert.equal(typeof t.isInterSystem,      'boolean');
      assert.equal(typeof t.requiresOperatorId, 'boolean');
    }

    // HUB-IN-SCANNED should be able to transition to BAGGED
    const codes = transitions.map(t => t.code);
    assert.ok(codes.includes('BAGGED'), `expected BAGGED in [${codes.join(', ')}]`);
  });

  it('10b. permittedTransitions returns [] for a terminal status', () => {
    const transitions = engine.permittedTransitions('DELIVERED', Flow.FORWARD);
    assert.deepEqual(transitions, []);
  });

  // ── Metrics counter sanity check ───────────────────────────────────────────
  it('snapshot() reflects accumulated counters', () => {
    const snap = engine.snapshot();

    assert.equal(typeof snap.cntTotal,       'number');
    assert.equal(typeof snap.cntInvalid,     'number');
    assert.equal(typeof snap.cntOverride,    'number');
    assert.equal(typeof snap.cntInterSystem, 'number');
    assert.equal(typeof snap.cntZeroValue,   'number');
    assert.equal(snap.specVersion, '1.0.0');

    // Some calls have been made by earlier tests.
    assert.ok(snap.cntTotal   > 0, 'cntTotal should be > 0');
    assert.ok(snap.cntInvalid > 0, 'cntInvalid should be > 0 (invalid tests ran)');
    assert.ok(snap.cntZeroValue > 0, 'cntZeroValue should be > 0 (zero-value tests ran)');
  });

  // ── Status registry sanity checks ─────────────────────────────────────────
  it('Status registry has correct ids for spot-checked entries', () => {
    assert.equal(Status.DRAFT,              1);
    assert.equal(Status.DELIVERED,         29);
    assert.equal(Status['HUB-IN-SCANNED'],  8);
    assert.equal(Status.HUB_IN_SCANNED,     8);
    assert.equal(Status['3PL_ITEM_BOOK'],  48);
    assert.equal(Status.TPL_ITEM_BOOK,     48);
    assert.equal(Status['3PL_BAG_OPEN'],   55);
    assert.equal(Status.TPL_BAG_OPEN,      55);
  });
});
