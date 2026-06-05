#!/usr/bin/env bash
# Quark Protocol Cross-language Conformance Suite v1.0
# All three reference implementations MUST produce identical output.

set +e
cd "$(dirname "$0")"
ROOT="$(cd ../.. && pwd)"

echo "═══════════════════════════════════════════════════════════"
echo "  Quark Protocol Conformance Suite v1.0"
echo "  Spec: cases.json"
echo "═══════════════════════════════════════════════════════════"
echo

# 1. Go
echo "[1/3] Go implementation"
cd "$ROOT/clients/go"
go test -tags conformance ./conformance/... 2>&1 | tail -3
GO_EXIT=${PIPESTATUS[0]}
cd "$ROOT/tests/conformance"

echo
# 2. TypeScript
echo "[2/3] TypeScript implementation"
if [ ! -d node_modules ]; then
  pnpm install --silent
fi
pnpm test 2>&1 | tail -4
TS_EXIT=${PIPESTATUS[0]}

echo
# 3. Python
echo "[3/3] Python implementation"
if [ -d "$ROOT/clients/python/.venv" ]; then
  PYTHON="$ROOT/clients/python/.venv/bin/python"
else
  PYTHON="python3"
fi
$PYTHON conformance_py.py 2>&1 | tail -3
PY_EXIT=${PIPESTATUS[0]}

echo
echo "═══════════════════════════════════════════════════════════"
if [ "$GO_EXIT" = "0" ] && [ "$TS_EXIT" = "0" ] && [ "$PY_EXIT" = "0" ]; then
  echo "  ✓ ALL THREE IMPLEMENTATIONS CONFORM"
  exit 0
else
  echo "  ✗ CONFORMANCE FAILED — divergence between implementations"
  echo "    go=$GO_EXIT, ts=$TS_EXIT, py=$PY_EXIT"
  exit 1
fi
