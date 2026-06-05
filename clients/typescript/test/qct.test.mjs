// Smoke tests for @unyly/quark-client.
// Run with: node --experimental-vm-modules test/qct.test.mjs
// (No test framework — using node:test for zero deps.)

import { test } from 'node:test'
import assert from 'node:assert'
import { QCT } from '../src/index.ts'

test('QCT round-trip', async () => {
  const secret = 'test-secret-32bytes-min-recommend'
  const payload = {
    iss: 'https://test.example',
    sub: 'user@example.com',
    exp: Math.floor(Date.now() / 1000) + 3600,
    scope: ['github:read:*', 'echo:invoke'],
    client_id: 'my-client',
    max_cost_usd: 1.50,
  }

  const token = await QCT.create({ secret, payload })
  assert.ok(token.startsWith('qct.v1.'), 'token should start with qct.v1.')

  const verified = await QCT.verify(token, secret)
  assert.strictEqual(verified.sub, 'user@example.com')
  assert.strictEqual(verified.scope.length, 2)
  assert.strictEqual(verified.max_cost_usd, 1.50)
})

test('QCT signature mismatch', async () => {
  const payload = {
    iss: 'test',
    sub: 'u',
    exp: Math.floor(Date.now() / 1000) + 3600,
    scope: ['x'],
  }
  const token = await QCT.create({ secret: 'real-secret', payload })

  await assert.rejects(
    async () => await QCT.verify(token, 'wrong-secret'),
    /signature mismatch/,
  )
})

test('QCT expired', async () => {
  const payload = {
    iss: 'test',
    sub: 'u',
    exp: Math.floor(Date.now() / 1000) - 3600, // already expired
    scope: ['x'],
  }
  const token = await QCT.create({ secret: 'test', payload })

  await assert.rejects(
    async () => await QCT.verify(token, 'test'),
    /expired/,
  )
})

test('QCT not-before', async () => {
  const payload = {
    iss: 'test',
    sub: 'u',
    nbf: Math.floor(Date.now() / 1000) + 3600, // future
    exp: Math.floor(Date.now() / 1000) + 7200,
    scope: ['x'],
  }
  const token = await QCT.create({ secret: 'test', payload })

  await assert.rejects(
    async () => await QCT.verify(token, 'test'),
    /not yet valid/,
  )
})

test('QCT requires exp', async () => {
  await assert.rejects(
    async () => await QCT.create({
      secret: 't',
      payload: { iss: 't', sub: 'u', scope: ['x'] },
    }),
    /exp required/,
  )
})

test('QCT preserves all payload fields', async () => {
  const exp = Math.floor(Date.now() / 1000) + 3600
  const iat = Math.floor(Date.now() / 1000) - 100
  const nbf = Math.floor(Date.now() / 1000) - 50

  const token = await QCT.create({
    secret: 'test',
    payload: {
      iss: 'iss-value',
      sub: 'sub-value',
      iat, nbf, exp,
      scope: ['a:b', 'c:d:*'],
      client_id: 'cid',
      session_id: 'sid',
      max_cost_usd: 2.5,
    },
  })

  const verified = await QCT.verify(token, 'test')
  assert.strictEqual(verified.iss, 'iss-value')
  assert.strictEqual(verified.sub, 'sub-value')
  assert.strictEqual(verified.iat, iat)
  assert.strictEqual(verified.nbf, nbf)
  assert.strictEqual(verified.exp, exp)
  assert.deepStrictEqual(verified.scope, ['a:b', 'c:d:*'])
  assert.strictEqual(verified.client_id, 'cid')
  assert.strictEqual(verified.session_id, 'sid')
  assert.strictEqual(verified.max_cost_usd, 2.5)
})
