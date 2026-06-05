package quark

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ApplyFilter is the public export of the filter engine. Used by clients and
// the pipeline runtime.
func ApplyFilter(items []any, expr string) []any {
	out, _ := applyFilterExtended(items, expr).([]any)
	return out
}

// EvalFilterExpr evaluates a single boolean expression against an object.
func EvalFilterExpr(obj map[string]any, expr string) bool {
	return evalOr(obj, strings.TrimSpace(expr))
}

func applyFilterExtended(input any, expr string) any {
	arr, ok := input.([]any)
	if !ok {
		return input
	}
	out := []any{}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if EvalFilterExpr(obj, expr) {
			out = append(out, obj)
		}
	}
	return out
}

// evalOr handles top-level `||` operators (lowest precedence).
func evalOr(obj map[string]any, expr string) bool {
	parts := splitTopLevel(expr, "||")
	if len(parts) == 1 {
		return evalAnd(obj, expr)
	}
	for _, p := range parts {
		if evalAnd(obj, strings.TrimSpace(p)) {
			return true
		}
	}
	return false
}

// evalAnd handles `&&` (higher precedence than ||).
func evalAnd(obj map[string]any, expr string) bool {
	parts := splitTopLevel(expr, "&&")
	if len(parts) == 1 {
		return evalNot(obj, expr)
	}
	for _, p := range parts {
		if !evalNot(obj, strings.TrimSpace(p)) {
			return false
		}
	}
	return true
}

// evalNot handles unary `!`.
func evalNot(obj map[string]any, expr string) bool {
	if strings.HasPrefix(expr, "!") {
		return !evalAtom(obj, strings.TrimSpace(expr[1:]))
	}
	return evalAtom(obj, expr)
}

// evalAtom handles `(...)` parens or a single comparison.
func evalAtom(obj map[string]any, expr string) bool {
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evalOr(obj, expr[1:len(expr)-1])
	}
	return evalComparison(obj, expr)
}

// splitTopLevel splits expr by sep, ignoring sep inside parens or quotes.
func splitTopLevel(expr, sep string) []string {
	out := []string{}
	depth := 0
	inQuote := byte(0)
	start := 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && i+len(sep) <= len(expr) && expr[i:i+len(sep)] == sep {
				out = append(out, expr[start:i])
				start = i + len(sep)
				i += len(sep) - 1
			}
		}
	}
	out = append(out, expr[start:])
	return out
}

// evalComparison parses "field op value" with extended ops.
func evalComparison(obj map[string]any, expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}

	// Multi-char + word ops
	ops := []string{">=", "<=", "==", "!=", " contains ", " startsWith ", " endsWith ", " matches ", " in ", " notIn "}
	for _, op := range ops {
		if i := indexUnquoted(expr, op); i >= 0 {
			field := strings.TrimSpace(expr[:i])
			valStr := strings.TrimSpace(expr[i+len(op):])
			return compareValue(resolveField(obj, field), strings.TrimSpace(op), valStr, obj)
		}
	}
	// Single-char
	for _, op := range []string{">", "<"} {
		if i := indexUnquoted(expr, op); i >= 0 {
			field := strings.TrimSpace(expr[:i])
			valStr := strings.TrimSpace(expr[i+1:])
			return compareValue(resolveField(obj, field), op, valStr, obj)
		}
	}
	// Bare field — coerce to bool
	val := resolveField(obj, expr)
	return truthyValue(val)
}

func indexUnquoted(s, sub string) int {
	inQuote := byte(0)
	for i := 0; i+len(sub) <= len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			continue
		}
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// resolveField supports nested.field and field[idx] access.
func resolveField(obj map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = obj
	for _, p := range parts {
		// Handle field[idx]
		if i := strings.Index(p, "["); i >= 0 && strings.HasSuffix(p, "]") {
			fieldName := p[:i]
			idxStr := p[i+1 : len(p)-1]
			if m, ok := current.(map[string]any); ok {
				current = m[fieldName]
			}
			if arr, ok := current.([]any); ok {
				var idx int
				fmt.Sscanf(idxStr, "%d", &idx)
				if idx >= 0 && idx < len(arr) {
					current = arr[idx]
				} else {
					return nil
				}
			}
			continue
		}
		if m, ok := current.(map[string]any); ok {
			current = m[p]
		} else {
			return nil
		}
	}
	return current
}

// compareValue does the actual op.
func compareValue(got any, op, valStr string, obj map[string]any) bool {
	val := parseValueOrArithmetic(valStr, obj)
	switch op {
	case ">":
		return numVal(got) > numVal(val)
	case "<":
		return numVal(got) < numVal(val)
	case ">=":
		return numVal(got) >= numVal(val)
	case "<=":
		return numVal(got) <= numVal(val)
	case "==":
		return equalValue(got, val)
	case "!=":
		return !equalValue(got, val)
	case "contains":
		s, _ := got.(string)
		v, _ := val.(string)
		return strings.Contains(s, v)
	case "startsWith":
		s, _ := got.(string)
		v, _ := val.(string)
		return strings.HasPrefix(s, v)
	case "endsWith":
		s, _ := got.(string)
		v, _ := val.(string)
		return strings.HasSuffix(s, v)
	case "matches":
		s, _ := got.(string)
		pattern, _ := val.(string)
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(s)
	case "in":
		arr, ok := val.([]any)
		if !ok {
			return false
		}
		for _, v := range arr {
			if equalValue(got, v) {
				return true
			}
		}
		return false
	case "notIn":
		arr, ok := val.([]any)
		if !ok {
			return true
		}
		for _, v := range arr {
			if equalValue(got, v) {
				return false
			}
		}
		return true
	}
	return false
}

func equalValue(a, b any) bool {
	switch ta := a.(type) {
	case float64:
		return ta == numVal(b)
	case int:
		return float64(ta) == numVal(b)
	case int64:
		return float64(ta) == numVal(b)
	case string:
		if tb, ok := b.(string); ok {
			return ta == tb
		}
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	case bool:
		if tb, ok := b.(bool); ok {
			return ta == tb
		}
	case nil:
		return b == nil
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func truthyValue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t != "" && t != "false" && t != "0"
	case float64:
		return t != 0
	case int:
		return t != 0
	case nil:
		return false
	}
	return true
}

// parseValueOrArithmetic supports literal values OR simple arithmetic.
// Examples: 100, "hello", true, [1, 2], followers * 0.1, now() - 86400
func parseValueOrArithmetic(s string, obj map[string]any) any {
	s = strings.TrimSpace(s)
	// Array literal: [a, b, c]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []any{}
		}
		parts := splitTopLevel(inner, ",")
		out := []any{}
		for _, p := range parts {
			out = append(out, parseValueOrArithmetic(strings.TrimSpace(p), obj))
		}
		return out
	}
	// Quoted string
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}
	// Boolean/null literal
	switch s {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	// now() function
	if s == "now()" {
		return float64(time.Now().Unix())
	}
	// Arithmetic? Try + - * / at top level (parens-aware via splitTopLevel)
	for _, op := range []string{"+", "-", "*", "/"} {
		parts := splitTopLevel(s, op)
		if len(parts) >= 2 {
			left := parseValueOrArithmetic(strings.TrimSpace(parts[0]), obj)
			rest := strings.Join(parts[1:], op)
			right := parseValueOrArithmetic(strings.TrimSpace(rest), obj)
			ln := numVal(left)
			rn := numVal(right)
			switch op {
			case "+":
				return ln + rn
			case "-":
				return ln - rn
			case "*":
				return ln * rn
			case "/":
				if rn == 0 {
					return 0.0
				}
				return ln / rn
			}
		}
	}
	// Field reference (resolve in obj if it's not a number literal)
	if obj != nil {
		if v := resolveField(obj, s); v != nil {
			return v
		}
	}
	// Number?
	var f float64
	n, _ := fmt.Sscanf(s, "%f", &f)
	if n == 1 {
		return f
	}
	// Default: string
	return s
}
