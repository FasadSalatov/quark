"""
Sael v1.0 extended filter expression language (client-side).

Mirrors the Go/TS implementations. Use for local pre-filtering before
sending data to AI agents, or for testing filter expressions.

Grammar:
    expression := orExpr
    orExpr     := andExpr ('||' andExpr)*
    andExpr    := notExpr ('&&' notExpr)*
    notExpr    := '!' atom | atom
    atom       := '(' expression ')' | comparison
    comparison := field op value
    op         := > | < | >= | <= | == | != | contains | startsWith | endsWith
                | matches | in | notIn
"""

import re
import time as _time
from typing import List, Any


def apply_filter(items: List[dict], expr: str) -> List[dict]:
    """Filter a list of dicts by expression."""
    return [item for item in items if eval_expr(item, expr.strip())]


def eval_expr(obj: dict, expr: str) -> bool:
    """Evaluate expression against a single dict."""
    return _eval_or(obj, expr.strip())


def _eval_or(obj, expr):
    parts = _split_top_level(expr, "||")
    if len(parts) == 1:
        return _eval_and(obj, expr)
    return any(_eval_and(obj, p.strip()) for p in parts)


def _eval_and(obj, expr):
    parts = _split_top_level(expr, "&&")
    if len(parts) == 1:
        return _eval_not(obj, expr)
    return all(_eval_not(obj, p.strip()) for p in parts)


def _eval_not(obj, expr):
    if expr.startswith("!"):
        return not _eval_atom(obj, expr[1:].strip())
    return _eval_atom(obj, expr)


def _eval_atom(obj, expr):
    if expr.startswith("(") and expr.endswith(")"):
        return _eval_or(obj, expr[1:-1])
    return _eval_comparison(obj, expr)


def _split_top_level(expr, sep):
    out = []
    depth = 0
    in_quote = ""
    start = 0
    i = 0
    while i < len(expr):
        c = expr[i]
        if in_quote:
            if c == in_quote:
                in_quote = ""
            i += 1
            continue
        if c in '"\'':
            in_quote = c
        elif c == "(":
            depth += 1
        elif c == ")":
            depth -= 1
        elif depth == 0 and expr[i:i+len(sep)] == sep:
            out.append(expr[start:i])
            start = i + len(sep)
            i += len(sep)
            continue
        i += 1
    out.append(expr[start:])
    return out


def _eval_comparison(obj, expr):
    expr = expr.strip()
    if not expr:
        return True

    word_ops = [" contains ", " startsWith ", " endsWith ", " matches ", " in ", " notIn "]
    sym_ops = [">=", "<=", "==", "!="]
    single_ops = [">", "<"]

    for op in word_ops + sym_ops:
        i = _index_unquoted(expr, op)
        if i >= 0:
            field = expr[:i].strip()
            val_str = expr[i+len(op):].strip()
            return _do_compare(_resolve_field(obj, field), op.strip(), val_str, obj)
    for op in single_ops:
        i = _index_unquoted(expr, op)
        if i >= 0:
            field = expr[:i].strip()
            val_str = expr[i+1:].strip()
            return _do_compare(_resolve_field(obj, field), op, val_str, obj)
    return _truthy(_resolve_field(obj, expr))


def _index_unquoted(s, sub):
    in_quote = ""
    for i in range(len(s) - len(sub) + 1):
        c = s[i]
        if in_quote:
            if c == in_quote:
                in_quote = ""
            continue
        if c in '"\'':
            in_quote = c
            continue
        if s[i:i+len(sub)] == sub:
            return i
    return -1


def _resolve_field(obj, path):
    parts = path.split(".")
    cur = obj
    for p in parts:
        m = re.match(r"^([^\[]+)\[(\d+)\]$", p)
        if m:
            field, idx = m.group(1), int(m.group(2))
            if isinstance(cur, dict):
                cur = cur.get(field)
            if isinstance(cur, list):
                cur = cur[idx] if 0 <= idx < len(cur) else None
            continue
        if isinstance(cur, dict):
            cur = cur.get(p)
        else:
            return None
    return cur


def _do_compare(got, op, val_str, obj):
    val = _parse_value_or_arith(val_str, obj)
    if op == ">": return _num(got) > _num(val)
    if op == "<": return _num(got) < _num(val)
    if op == ">=": return _num(got) >= _num(val)
    if op == "<=": return _num(got) <= _num(val)
    if op == "==": return _equal(got, val)
    if op == "!=": return not _equal(got, val)
    if op == "contains":
        return isinstance(got, str) and isinstance(val, str) and val in got
    if op == "startsWith":
        return isinstance(got, str) and isinstance(val, str) and got.startswith(val)
    if op == "endsWith":
        return isinstance(got, str) and isinstance(val, str) and got.endswith(val)
    if op == "matches":
        if not isinstance(got, str) or not isinstance(val, str):
            return False
        try:
            return bool(re.search(val, got))
        except re.error:
            return False
    if op == "in":
        return isinstance(val, list) and any(_equal(got, v) for v in val)
    if op == "notIn":
        return isinstance(val, list) and not any(_equal(got, v) for v in val)
    return False


def _equal(a, b):
    if isinstance(a, (int, float)) or isinstance(b, (int, float)):
        return _num(a) == _num(b)
    return a == b


def _truthy(v):
    if isinstance(v, bool): return v
    if isinstance(v, str): return v not in ("", "false", "0")
    if isinstance(v, (int, float)): return v != 0
    if v is None: return False
    return True


def _num(v):
    if isinstance(v, (int, float)): return float(v)
    if isinstance(v, str):
        try: return float(v)
        except ValueError: return 0
    if isinstance(v, bool): return 1.0 if v else 0.0
    return 0.0


def _parse_value_or_arith(s, obj):
    s = s.strip()
    if s.startswith("[") and s.endswith("]"):
        inner = s[1:-1].strip()
        if not inner: return []
        return [_parse_value_or_arith(p.strip(), obj) for p in _split_top_level(inner, ",")]
    if (s.startswith('"') and s.endswith('"')) or (s.startswith("'") and s.endswith("'")):
        return s[1:-1]
    if s == "true": return True
    if s == "false": return False
    if s == "null": return None
    if s == "now()": return _time.time()

    for op in ["+", "-", "*", "/"]:
        parts = _split_top_level(s, op)
        if len(parts) >= 2:
            left = _parse_value_or_arith(parts[0].strip(), obj)
            right = _parse_value_or_arith(op.join(parts[1:]).strip(), obj)
            ln, rn = _num(left), _num(right)
            if op == "+": return ln + rn
            if op == "-": return ln - rn
            if op == "*": return ln * rn
            if op == "/": return 0 if rn == 0 else ln / rn

    if obj:
        v = _resolve_field(obj, s)
        if v is not None:
            return v
    try:
        return float(s)
    except ValueError:
        return s
