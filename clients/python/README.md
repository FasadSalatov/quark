# sael-client (Python)

Python SDK for the **Sael Protocol v1.0** — streaming-first AI tool protocol replacing MCP.

## Install

```bash
pip install sael-client
```

For MessagePack support:
```bash
pip install "sael-client[msgpack]"
```

## Quick start

```python
import asyncio
import time
from sael_client import Sael, SCT, new_trace_id

async def main():
    # 1. Mint a signed capability token
    token = SCT.create(
        secret="shared-secret-with-server",
        payload={
            "iss": "https://my-app.com",
            "sub": "user@example.com",
            "exp": int(time.time()) + 3600,
            "scope": ["echo:invoke", "github:read:*"],
            "max_cost_usd": 5.00,
        },
    )

    # 2. Connect
    async with await Sael.connect(
        "wss://unyly.org/sael/ws",
        agent={"id": "my-bot", "kind": "llm", "name": "My Bot"},
        auth={"type": "bearer", "token": token},
    ) as ch:
        # 3. List tools
        tools = await ch.list_tools()
        print(f"{len(tools)} tools available")

        # 4. One-shot invocation
        result = await ch.invoke("echo.upper", {"text": "hello"})
        print(result)  # "HELLO"

        # 5. Streaming
        async for chunk in ch.stream("demo.counter", {"n": 5}):
            print(chunk)

        # 6. Server-side pipeline composition
        names = await ch.pipeline([
            {"tool": "demo.fake_repos"},
            {"filter": "stars > 100"},
            {"map": ["name"]},
        ])
        print(names)  # ['claude-code', 'mcp']

        # 7. Subscriptions
        async for event in ch.subscribe("demo.heartbeat"):
            print(event)
            break

        # 8. Cost tracking
        print("Total cost:", ch.get_cost())

asyncio.run(main())
```

## Distributed tracing

```python
from sael_client import new_trace_id, new_span_id

trace_id = new_trace_id()
await ch.invoke("a.tool", {}, trace_id=trace_id, span_id=new_span_id())
await ch.invoke("b.tool", {}, trace_id=trace_id, span_id=new_span_id())
```

## Filter expressions (local)

```python
from sael_client import apply_filter

items = [{"name": "x", "stars": 200}, {"name": "y", "stars": 50}]
result = apply_filter(items, "stars > 100 && name != 'z'")
```

## Spec

Full protocol spec: <https://github.com/FasadSalatov/sael/blob/main/docs/spec.md>

## License

MIT
