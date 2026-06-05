"""
quark_client — Python SDK for Quark Protocol v1.0.

Quark replaces MCP for connecting AI agents to tools.
Spec: https://github.com/FasadSalatov/quark/blob/main/docs/spec.md

Quick start:

    import asyncio
    from quark_client import Quark, QCT

    async def main():
        token = QCT.create(
            secret="shared-secret",
            payload={
                "iss": "https://my-app.com",
                "sub": "user@example.com",
                "exp": int(time.time()) + 3600,
                "scope": ["echo:invoke"],
            },
        )

        async with Quark.connect(
            "wss://server/quark/ws",
            agent={"id": "my-bot", "kind": "llm", "name": "My Bot"},
            auth={"type": "bearer", "token": token},
        ) as ch:
            tools = await ch.list_tools()
            result = await ch.invoke("echo.upper", {"text": "hello"})
            async for chunk in ch.stream("logs.tail", {"file": "app.log"}):
                print(chunk)
            composed = await ch.pipeline([
                {"tool": "demo.fake_repos"},
                {"filter": "stars > 100"},
                {"map": ["name"]},
            ])

    asyncio.run(main())
"""

from .client import Quark, Channel, ConnectOptions
from .qct import QCT, QCTPayload
from .tracing import new_trace_id, new_span_id
from .filter import apply_filter, eval_expr

PROTOCOL_VERSION = 1

__all__ = [
    "Quark",
    "Channel",
    "ConnectOptions",
    "QCT",
    "QCTPayload",
    "new_trace_id",
    "new_span_id",
    "apply_filter",
    "eval_expr",
    "PROTOCOL_VERSION",
]
