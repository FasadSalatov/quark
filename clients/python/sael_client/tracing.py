"""W3C trace_id / span_id helpers."""

import secrets


def new_trace_id() -> str:
    """Generate a fresh 32-hex-char trace_id."""
    return secrets.token_hex(16)


def new_span_id() -> str:
    """Generate a fresh 16-hex-char span_id."""
    return secrets.token_hex(8)
