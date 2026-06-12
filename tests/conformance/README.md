# Sael Conformance Tests

Cross-implementation conformance suite. **All three reference implementations MUST produce identical output** for every test case in `cases.json`.

## Layout

```
tests/conformance/
├── cases.json              # language-agnostic test spec
├── conformance_go_test.go  # Go runner (build tag: conformance)
├── conformance_ts.mjs      # TypeScript runner (Node test runner)
├── conformance_py.py       # Python runner (stdlib only)
└── run.sh                  # orchestrator — runs all three
```

## Run all

```bash
./run.sh
```

## Run individually

### Go
```bash
cd ../../clients/go
go test -tags conformance ../../tests/conformance/... -v
```

### TypeScript
```bash
node --import tsx --test conformance_ts.mjs
```

### Python
```bash
python3 conformance_py.py
```

## What's tested

- **SCT** — round-trip, expired tokens, nbf-future tokens
- **Filter language** — all operators (>, <, ==, !=, contains, startsWith, in, notIn, matches, !, &&, ||), nested fields, arithmetic
- **Tracing** — trace_id (32 hex), span_id (16 hex)
- **Protocol** — version field stable

## Adding new cases

Edit `cases.json`. Re-run `./run.sh`. All three runners will pick up the new case automatically.

## Used by CI

The conformance suite is run on every commit. If any implementation diverges from the spec, the build fails.
