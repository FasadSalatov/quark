"""Python conformance runner. Reads cases.json, runs each test."""

import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent / "clients" / "python"))

from sael_client import SCT, apply_filter, new_trace_id, new_span_id, PROTOCOL_VERSION


def load_cases():
    p = Path(__file__).parent / "cases.json"
    return json.loads(p.read_text())


def run_qct(cases):
    passed = failed = 0
    for tc in cases["qct_tests"]:
        name = tc["name"]
        secret = tc["secret"]
        payload = tc["payload"]
        expect_valid = tc["expect_valid"]
        try:
            token = SCT.create(secret, payload)
        except Exception as e:
            if expect_valid:
                print(f"  ✗ qct/{name}: create failed: {e}")
                failed += 1
                continue
            else:
                passed += 1
                continue

        try:
            verified = SCT.verify(token, secret)
            if not expect_valid:
                print(f"  ✗ qct/{name}: expected error, got valid")
                failed += 1
                continue
            assert verified["sub"] == payload["sub"]
            print(f"  ✓ qct/{name}")
            passed += 1
        except Exception as e:
            if not expect_valid:
                expected = tc.get("expect_error_contains", "")
                if expected.lower() in str(e).lower():
                    print(f"  ✓ qct/{name}")
                    passed += 1
                else:
                    print(f"  ✗ qct/{name}: expected '{expected}', got '{e}'")
                    failed += 1
            else:
                print(f"  ✗ qct/{name}: verify failed: {e}")
                failed += 1
    return passed, failed


def run_filter(cases):
    passed = failed = 0
    for tc in cases["filter_tests"]:
        name = tc["name"]
        items = tc["items"]
        expr = tc["expr"]
        expected = tc["expected_count"]
        result = apply_filter(items, expr)
        if len(result) == expected:
            if "expected_first" in tc:
                first = result[0]
                ok = all(first.get(k) == v for k, v in tc["expected_first"].items())
                if ok:
                    print(f"  ✓ filter/{name}")
                    passed += 1
                else:
                    print(f"  ✗ filter/{name}: first item mismatch")
                    failed += 1
            else:
                print(f"  ✓ filter/{name}")
                passed += 1
        else:
            print(f"  ✗ filter/{name}: expected {expected}, got {len(result)} ({result})")
            failed += 1
    return passed, failed


def run_tracing(cases):
    passed = failed = 0
    for tc in cases["tracing_tests"]:
        name = tc["name"]
        expected = tc["expected_length"]
        got = new_trace_id() if name == "trace_id_length" else new_span_id()
        if len(got) == expected:
            print(f"  ✓ tracing/{name}")
            passed += 1
        else:
            print(f"  ✗ tracing/{name}: expected length {expected}, got {len(got)}")
            failed += 1
    return passed, failed


def run_protocol(cases):
    passed = failed = 0
    for tc in cases["protocol_tests"]:
        name = tc["name"]
        if name == "protocol_version":
            if PROTOCOL_VERSION == tc["expected"]:
                print(f"  ✓ protocol/{name}")
                passed += 1
            else:
                print(f"  ✗ protocol/{name}: expected {tc['expected']}, got {PROTOCOL_VERSION}")
                failed += 1
    return passed, failed


if __name__ == "__main__":
    cases = load_cases()
    print(f"=== Python conformance (cases.json v{cases['version']}) ===")
    p1, f1 = run_qct(cases)
    p2, f2 = run_filter(cases)
    p3, f3 = run_tracing(cases)
    p4, f4 = run_protocol(cases)
    total_p, total_f = p1+p2+p3+p4, f1+f2+f3+f4
    print()
    print(f"=== {total_p} passed, {total_f} failed ===")
    sys.exit(0 if total_f == 0 else 1)
