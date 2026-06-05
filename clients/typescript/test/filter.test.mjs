import { test } from 'node:test'
import assert from 'node:assert'
import { applyFilter, evalExpr } from '../src/index.ts'

test('filter: basic comparison', () => {
  const items = [
    { name: 'a', stars: 50 },
    { name: 'b', stars: 200 },
    { name: 'c', stars: 1000 },
  ]
  const result = applyFilter(items, 'stars > 100')
  assert.strictEqual(result.length, 2)
})

test('filter: and', () => {
  const items = [
    { name: 'a', stars: 50, owner: 'x' },
    { name: 'b', stars: 200, owner: 'x' },
    { name: 'c', stars: 200, owner: 'y' },
  ]
  const result = applyFilter(items, 'stars > 100 && owner == "x"')
  assert.strictEqual(result.length, 1)
  assert.strictEqual(result[0].name, 'b')
})

test('filter: or with parens', () => {
  const items = [
    { name: 'a', stars: 200, verified: true },
    { name: 'b', stars: 50, verified: false },
    { name: 'c', stars: 50, verified: true },
  ]
  const result = applyFilter(items, '(stars > 100 || verified == true)')
  assert.strictEqual(result.length, 2)
})

test('filter: not', () => {
  const items = [
    { name: 'a', archived: true },
    { name: 'b', archived: false },
  ]
  const result = applyFilter(items, '!archived')
  assert.strictEqual(result.length, 1)
})

test('filter: in', () => {
  const items = [
    { lang: 'go' },
    { lang: 'rust' },
    { lang: 'python' },
  ]
  const result = applyFilter(items, 'lang in ["go", "rust"]')
  assert.strictEqual(result.length, 2)
})

test('filter: notIn', () => {
  const items = [
    { status: 'active' },
    { status: 'archived' },
    { status: 'deleted' },
  ]
  const result = applyFilter(items, 'status notIn ["archived", "deleted"]')
  assert.strictEqual(result.length, 1)
})

test('filter: matches regex', () => {
  const items = [
    { email: 'a@example.com' },
    { email: 'b@other.com' },
  ]
  const result = applyFilter(items, 'email matches ".*@example.*"')
  assert.strictEqual(result.length, 1)
})

test('filter: nested field', () => {
  const items = [
    { name: 'a', meta: { score: 50 } },
    { name: 'b', meta: { score: 200 } },
  ]
  const result = applyFilter(items, 'meta.score > 100')
  assert.strictEqual(result.length, 1)
})

test('filter: arithmetic value', () => {
  const items = [
    { a: 10, b: 5 },
    { a: 3, b: 5 },
  ]
  const result = applyFilter(items, 'a > b * 1.5')
  assert.strictEqual(result.length, 1)
})

test('filter: contains', () => {
  const items = [{ name: 'claude-code' }, { name: 'mcp' }]
  const result = applyFilter(items, 'name contains "claude"')
  assert.strictEqual(result.length, 1)
})

test('filter: evalExpr direct', () => {
  assert.strictEqual(evalExpr({ stars: 100 }, 'stars > 50'), true)
  assert.strictEqual(evalExpr({ stars: 30 }, 'stars > 50'), false)
})
