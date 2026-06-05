/**
 * Quark v1.0 extended filter expression language.
 *
 * Grammar:
 *   expression := orExpr
 *   orExpr     := andExpr ('||' andExpr)*
 *   andExpr    := notExpr ('&&' notExpr)*
 *   notExpr    := '!' atom | atom
 *   atom       := '(' expression ')' | comparison
 *   comparison := field op value
 *   op         := '>' | '<' | '>=' | '<=' | '==' | '!=' | 'contains' | 'startsWith' | 'endsWith' | 'matches' | 'in' | 'notIn'
 *
 * This is the client-side version of the same engine in clients/go/filter.go.
 */

export type Row = Record<string, unknown>

/** Apply a filter expression to an array of objects. */
export function applyFilter(items: Row[], expr: string): Row[] {
  return items.filter((item) => evalExpr(item, expr.trim()))
}

/** Evaluate a filter expression against a single object. */
export function evalExpr(obj: Row, expr: string): boolean {
  return evalOr(obj, expr.trim())
}

function evalOr(obj: Row, expr: string): boolean {
  const parts = splitTopLevel(expr, '||')
  if (parts.length === 1) return evalAnd(obj, expr)
  return parts.some((p) => evalAnd(obj, p.trim()))
}

function evalAnd(obj: Row, expr: string): boolean {
  const parts = splitTopLevel(expr, '&&')
  if (parts.length === 1) return evalNot(obj, expr)
  return parts.every((p) => evalNot(obj, p.trim()))
}

function evalNot(obj: Row, expr: string): boolean {
  if (expr.startsWith('!')) return !evalAtom(obj, expr.slice(1).trim())
  return evalAtom(obj, expr)
}

function evalAtom(obj: Row, expr: string): boolean {
  if (expr.startsWith('(') && expr.endsWith(')')) {
    return evalOr(obj, expr.slice(1, -1))
  }
  return evalComparison(obj, expr)
}

function splitTopLevel(expr: string, sep: string): string[] {
  const out: string[] = []
  let depth = 0
  let inQuote = ''
  let start = 0
  for (let i = 0; i < expr.length; i++) {
    const c = expr[i]
    if (inQuote) {
      if (c === inQuote) inQuote = ''
      continue
    }
    if (c === '"' || c === "'") {
      inQuote = c
    } else if (c === '(') {
      depth++
    } else if (c === ')') {
      depth--
    } else if (depth === 0 && expr.startsWith(sep, i)) {
      out.push(expr.slice(start, i))
      start = i + sep.length
      i += sep.length - 1
    }
  }
  out.push(expr.slice(start))
  return out
}

function evalComparison(obj: Row, expr: string): boolean {
  expr = expr.trim()
  if (!expr) return true

  const wordOps = [' contains ', ' startsWith ', ' endsWith ', ' matches ', ' in ', ' notIn ']
  const symbolOps = ['>=', '<=', '==', '!=']
  const singleOps = ['>', '<']

  for (const op of [...wordOps, ...symbolOps]) {
    const i = indexUnquoted(expr, op)
    if (i >= 0) {
      const field = expr.slice(0, i).trim()
      const valStr = expr.slice(i + op.length).trim()
      return doCompare(resolveField(obj, field), op.trim(), valStr, obj)
    }
  }
  for (const op of singleOps) {
    const i = indexUnquoted(expr, op)
    if (i >= 0) {
      const field = expr.slice(0, i).trim()
      const valStr = expr.slice(i + 1).trim()
      return doCompare(resolveField(obj, field), op, valStr, obj)
    }
  }
  // Bare field — coerce to bool
  return truthy(resolveField(obj, expr))
}

function indexUnquoted(s: string, sub: string): number {
  let inQuote = ''
  for (let i = 0; i + sub.length <= s.length; i++) {
    const c = s[i]
    if (inQuote) {
      if (c === inQuote) inQuote = ''
      continue
    }
    if (c === '"' || c === "'") inQuote = c
    else if (s.startsWith(sub, i)) return i
  }
  return -1
}

function resolveField(obj: Row, path: string): unknown {
  const parts = path.split('.')
  let cur: unknown = obj
  for (const p of parts) {
    const m = /^([^\[]+)\[(\d+)\]$/.exec(p)
    if (m) {
      const [, fieldName, idxStr] = m
      if (cur && typeof cur === 'object' && fieldName in (cur as Row)) {
        cur = (cur as Row)[fieldName]
      }
      if (Array.isArray(cur)) {
        const idx = parseInt(idxStr, 10)
        cur = idx < cur.length ? cur[idx] : undefined
      }
      continue
    }
    if (cur && typeof cur === 'object') {
      cur = (cur as Row)[p]
    } else {
      return undefined
    }
  }
  return cur
}

function doCompare(got: unknown, op: string, valStr: string, obj: Row): boolean {
  const val = parseValueOrArith(valStr, obj)
  switch (op) {
    case '>': return numOf(got) > numOf(val)
    case '<': return numOf(got) < numOf(val)
    case '>=': return numOf(got) >= numOf(val)
    case '<=': return numOf(got) <= numOf(val)
    case '==': return equal(got, val)
    case '!=': return !equal(got, val)
    case 'contains':
      return typeof got === 'string' && typeof val === 'string' && got.includes(val)
    case 'startsWith':
      return typeof got === 'string' && typeof val === 'string' && got.startsWith(val)
    case 'endsWith':
      return typeof got === 'string' && typeof val === 'string' && got.endsWith(val)
    case 'matches':
      if (typeof got !== 'string' || typeof val !== 'string') return false
      try {
        return new RegExp(val).test(got)
      } catch {
        return false
      }
    case 'in':
      return Array.isArray(val) && val.some((v) => equal(got, v))
    case 'notIn':
      return Array.isArray(val) && !val.some((v) => equal(got, v))
  }
  return false
}

function equal(a: unknown, b: unknown): boolean {
  if (typeof a === 'number' || typeof b === 'number') return numOf(a) === numOf(b)
  if (typeof a === 'string' && typeof b === 'string') return a === b
  if (typeof a === 'boolean' && typeof b === 'boolean') return a === b
  return JSON.stringify(a) === JSON.stringify(b)
}

function truthy(v: unknown): boolean {
  if (typeof v === 'boolean') return v
  if (typeof v === 'string') return v !== '' && v !== 'false' && v !== '0'
  if (typeof v === 'number') return v !== 0
  if (v == null) return false
  return true
}

function numOf(v: unknown): number {
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = parseFloat(v)
    return isNaN(n) ? 0 : n
  }
  if (typeof v === 'boolean') return v ? 1 : 0
  return 0
}

function parseValueOrArith(s: string, obj: Row): unknown {
  s = s.trim()
  if (s.startsWith('[') && s.endsWith(']')) {
    const inner = s.slice(1, -1).trim()
    if (!inner) return []
    return splitTopLevel(inner, ',').map((p) => parseValueOrArith(p.trim(), obj))
  }
  if ((s.startsWith('"') && s.endsWith('"')) || (s.startsWith("'") && s.endsWith("'"))) {
    return s.slice(1, -1)
  }
  if (s === 'true') return true
  if (s === 'false') return false
  if (s === 'null') return null
  if (s === 'now()') return Math.floor(Date.now() / 1000)

  for (const op of ['+', '-', '*', '/']) {
    const parts = splitTopLevel(s, op)
    if (parts.length >= 2) {
      const left = parseValueOrArith(parts[0].trim(), obj)
      const right = parseValueOrArith(parts.slice(1).join(op).trim(), obj)
      const ln = numOf(left)
      const rn = numOf(right)
      switch (op) {
        case '+': return ln + rn
        case '-': return ln - rn
        case '*': return ln * rn
        case '/': return rn === 0 ? 0 : ln / rn
      }
    }
  }
  if (obj) {
    const v = resolveField(obj, s)
    if (v !== undefined) return v
  }
  const n = parseFloat(s)
  if (!isNaN(n)) return n
  return s
}
