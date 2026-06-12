"""SCT tests."""
import time
import pytest
from sael_client import SCT


def test_qct_round_trip():
    payload = {
        "iss": "https://test.example",
        "sub": "user@example.com",
        "exp": int(time.time()) + 3600,
        "scope": ["echo:invoke", "github:read:*"],
        "max_cost_usd": 1.50,
    }
    token = SCT.create("secret", payload)
    assert token.startswith("qct.v1.")
    verified = SCT.verify(token, "secret")
    assert verified["sub"] == "user@example.com"
    assert verified["scope"] == ["echo:invoke", "github:read:*"]
    assert verified["max_cost_usd"] == 1.50


def test_qct_signature_mismatch():
    token = SCT.create("real", {
        "iss": "x", "sub": "u",
        "exp": int(time.time()) + 3600,
        "scope": ["x"],
    })
    with pytest.raises(ValueError, match="signature"):
        SCT.verify(token, "wrong")


def test_qct_expired():
    token = SCT.create("s", {
        "iss": "x", "sub": "u",
        "exp": int(time.time()) - 3600,
        "scope": ["x"],
    })
    with pytest.raises(ValueError, match="expired"):
        SCT.verify(token, "s")


def test_qct_nbf():
    token = SCT.create("s", {
        "iss": "x", "sub": "u",
        "nbf": int(time.time()) + 3600,
        "exp": int(time.time()) + 7200,
        "scope": ["x"],
    })
    with pytest.raises(ValueError, match="not yet"):
        SCT.verify(token, "s")


def test_qct_requires_exp():
    with pytest.raises(ValueError, match="exp"):
        SCT.create("s", {"iss": "x", "sub": "u", "scope": ["x"]})


def test_qct_preserves_fields():
    exp = int(time.time()) + 3600
    nbf = int(time.time()) - 50
    token = SCT.create("s", {
        "iss": "issuer", "sub": "subject",
        "nbf": nbf, "exp": exp,
        "scope": ["a:b"],
        "client_id": "cid",
        "session_id": "sid",
        "max_cost_usd": 2.5,
        "federation_allowed": ["host.example.com"],
    })
    payload = SCT.verify(token, "s")
    assert payload["iss"] == "issuer"
    assert payload["sub"] == "subject"
    assert payload["nbf"] == nbf
    assert payload["exp"] == exp
    assert payload["client_id"] == "cid"
    assert payload["session_id"] == "sid"
    assert payload["max_cost_usd"] == 2.5
    assert payload["federation_allowed"] == ["host.example.com"]
