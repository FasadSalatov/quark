"""Sael Capability Tokens (SCT) — HMAC-SHA256 signed tokens for v1.0+."""

import base64
import hashlib
import hmac
import json
import time
from dataclasses import dataclass, field, asdict
from typing import List, Optional


@dataclass
class QCTPayload:
    iss: str
    sub: str
    exp: int  # Unix seconds
    scope: List[str]
    iat: Optional[int] = None
    nbf: Optional[int] = None
    client_id: Optional[str] = None
    session_id: Optional[str] = None
    max_cost_usd: Optional[float] = None
    federation_allowed: Optional[List[str]] = None

    def to_dict(self) -> dict:
        d = {k: v for k, v in asdict(self).items() if v is not None}
        return d


class SCT:
    @staticmethod
    def create(secret, payload) -> str:
        """Mint a signed token.

        Args:
            secret: HMAC key (str or bytes)
            payload: QCTPayload or dict with at least iss/sub/exp/scope.

        Returns:
            Token string in format 'qct.v1.<base64url(payload)>.<base64url(signature)>'.
        """
        if isinstance(payload, QCTPayload):
            payload_dict = payload.to_dict()
        else:
            payload_dict = dict(payload)

        if "exp" not in payload_dict:
            raise ValueError("payload.exp required")
        if "iat" not in payload_dict:
            payload_dict["iat"] = int(time.time())

        if isinstance(secret, str):
            secret = secret.encode()

        body = json.dumps(payload_dict, separators=(",", ":"), sort_keys=False).encode()
        encoded = _b64url(body)
        signing = "v1." + encoded
        sig = hmac.new(secret, signing.encode(), hashlib.sha256).digest()
        return "qct.v1." + encoded + "." + _b64url(sig)

    @staticmethod
    def verify(token: str, secret) -> dict:
        """Verify signature + time bounds. Returns payload dict or raises ValueError."""
        if isinstance(secret, str):
            secret = secret.encode()

        parts = token.split(".")
        if len(parts) != 4 or parts[0] != "qct" or parts[1] != "v1":
            raise ValueError("malformed SCT")

        _, _, encoded, sig = parts
        signing = "v1." + encoded
        expected = _b64url(hmac.new(secret, signing.encode(), hashlib.sha256).digest())
        if not hmac.compare_digest(expected, sig):
            raise ValueError("signature mismatch")

        body = _b64url_decode(encoded)
        payload = json.loads(body)

        now = int(time.time())
        if "nbf" in payload and now < payload["nbf"]:
            raise ValueError("token not yet valid (nbf)")
        if payload["exp"] <= now:
            raise ValueError("token expired")
        return payload


def _b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def _b64url_decode(s: str) -> bytes:
    padding = "=" * (-len(s) % 4)
    return base64.urlsafe_b64decode(s + padding)
