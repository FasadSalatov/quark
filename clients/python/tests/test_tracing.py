"""Tracing tests."""
from quark_client import new_trace_id, new_span_id, PROTOCOL_VERSION


def test_new_trace_id():
    id = new_trace_id()
    assert len(id) == 32
    assert all(c in "0123456789abcdef" for c in id)


def test_new_span_id():
    id = new_span_id()
    assert len(id) == 16
    assert all(c in "0123456789abcdef" for c in id)


def test_trace_ids_unique():
    a = new_trace_id()
    b = new_trace_id()
    assert a != b


def test_protocol_version():
    assert PROTOCOL_VERSION == 1
