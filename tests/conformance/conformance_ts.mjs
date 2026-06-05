// TypeScript/Node conformance runner.
// Reads cases.json, runs each test against @fasad_salatov/quark-client locals.

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { test } from 'node:test'
import assert from 'node:assert'

const __dirname = dirname(fileURLToPath(import.meta.url))
const sdkPath = join(__dirname, '..', '..', 'clients', 'typescript', 'src', 'index.ts')
const { QCT, applyFilter, newTraceId, newSpanId, ProtocolVersion } = await import(sdkPath)

const cases = JSON.parse(readFileSync(join(__dirname, 'cases.json'), 'utf-8'))

// ─── QCT ───
for (const tc of cases.qct_tests) {
  test(`qct: ${tc.name}`, async () => {
    let token, error
    try {
      token = await QCT.create({ secret: tc.secret, payload: tc.payload })
    } catch (e) {
      error = e
    }

    if (tc.expect_valid) {
      assert.ok(token, `expected create to succeed, got error: ${error?.message}`)
      const verified = await QCT.verify(token, tc.secret)
      assert.strictEqual(verified.sub, tc.payload.sub)
    } else {
      try {
        await QCT.verify(token, tc.secret)
        assert.fail(`expected verify to throw`)
      } catch (e) {
        if (tc.expect_error_contains) {
          assert.match(
            e.message.toLowerCase(),
            new RegExp(tc.expect_error_contains.toLowerCase()),
            `error should contain ${tc.expect_error_contains}`,
          )
        }
      }
    }
  })
}

// ─── Filter ───
for (const tc of cases.filter_tests) {
  test(`filter: ${tc.name}`, () => {
    const result = applyFilter(tc.items, tc.expr)
    assert.strictEqual(result.length, tc.expected_count,
      `expected ${tc.expected_count}, got ${result.length}: ${JSON.stringify(result)}`)
    if (tc.expected_first) {
      for (const [k, v] of Object.entries(tc.expected_first)) {
        assert.strictEqual(result[0][k], v)
      }
    }
  })
}

// ─── Tracing ───
for (const tc of cases.tracing_tests) {
  test(`tracing: ${tc.name}`, () => {
    let got
    if (tc.name === 'trace_id_length') got = newTraceId()
    else if (tc.name === 'span_id_length') got = newSpanId()
    assert.strictEqual(got.length, tc.expected_length)
  })
}

// ─── Protocol ───
for (const tc of cases.protocol_tests) {
  test(`protocol: ${tc.name}`, () => {
    if (tc.name === 'protocol_version') {
      assert.strictEqual(ProtocolVersion, tc.expected)
    }
  })
}
