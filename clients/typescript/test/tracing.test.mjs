import { test } from 'node:test'
import assert from 'node:assert'
import { newTraceId, newSpanId, ProtocolVersion } from '../src/index.ts'

test('newTraceId is 32 hex chars', () => {
  const id = newTraceId()
  assert.strictEqual(id.length, 32)
  assert.match(id, /^[0-9a-f]{32}$/)
})

test('newSpanId is 16 hex chars', () => {
  const id = newSpanId()
  assert.strictEqual(id.length, 16)
  assert.match(id, /^[0-9a-f]{16}$/)
})

test('trace IDs are unique', () => {
  const a = newTraceId()
  const b = newTraceId()
  assert.notStrictEqual(a, b)
})

test('ProtocolVersion is 2', () => {
  assert.strictEqual(ProtocolVersion, 2)
})
