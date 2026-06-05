"""Quark protocol client (asyncio-based WebSocket).

Usage:
    async with Quark.connect(url, agent={...}, auth={...}) as ch:
        tools = await ch.list_tools()
        result = await ch.invoke("echo.upper", {"text": "hi"})
        async for chunk in ch.stream("logs.tail", {"file": "x.log"}):
            print(chunk)
        composed = await ch.pipeline([...])
"""

import asyncio
import json
import os
import time
from dataclasses import dataclass, field
from typing import Optional, Dict, Any, List, AsyncIterator

try:
    import websockets
    from websockets.client import WebSocketClientProtocol
except ImportError as e:
    raise ImportError("websockets>=12.0 required: pip install websockets") from e


PROTOCOL_VERSION = 1


@dataclass
class ConnectOptions:
    agent: Dict[str, Any]
    auth: Optional[Dict[str, str]] = None
    capabilities: List[str] = field(default_factory=list)
    supports: List[str] = field(default_factory=lambda: [
        "streaming", "subscribe", "compose", "capabilities",
        "resume", "tracing", "heartbeat", "validation"
    ])
    auto_reconnect: bool = True
    heartbeat_interval_s: int = 30


class Quark:
    @staticmethod
    async def connect(url: str, **kwargs) -> "Channel":
        """Open a Quark channel.

        Args:
            url: WebSocket URL like 'wss://server/quark/ws'
            agent: dict like {"id": "my-bot", "kind": "llm", "name": "..."}
            auth: optional {"type": "bearer", "token": "qct.v1..."}
            capabilities: list of capability strings (optional)
            auto_reconnect: bool (default True)
        """
        opts = ConnectOptions(**kwargs)
        ch = Channel(url, opts)
        await ch._open()
        await ch.ready
        return ch


class Channel:
    def __init__(self, url: str, opts: ConnectOptions):
        self.url = url
        self.opts = opts
        self.ws: Optional[WebSocketClientProtocol] = None
        self._seq = 1
        self._pending: Dict[int, asyncio.Future] = {}
        self._streams: Dict[int, asyncio.Queue] = {}
        self.session_id: Optional[str] = None
        self.server_version: Optional[int] = None
        self.cost_accum = {"compute_ms": 0, "api_calls": 0, "usd": 0.0, "tokens": 0}
        self._reader_task: Optional[asyncio.Task] = None
        self._hb_task: Optional[asyncio.Task] = None
        self._closed = False
        self._ready_event = asyncio.Event()
        self._last_seq_received = 0

    @property
    def ready(self):
        return self._ready_event.wait()

    async def _open(self):
        self.ws = await websockets.connect(self.url)
        await self._send({
            "v": PROTOCOL_VERSION,
            "kind": "HEY",
            "agent": self.opts.agent,
            "auth": self.opts.auth,
            "capabilities": self.opts.capabilities,
            "supports": self.opts.supports,
        })
        self._reader_task = asyncio.create_task(self._reader())

    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        await self.close()

    def _next_seq(self) -> int:
        s = self._seq
        self._seq += 1
        return s

    async def _send(self, frame: Dict[str, Any]):
        frame = {k: v for k, v in frame.items() if v is not None}
        await self.ws.send(json.dumps(frame))

    async def _reader(self):
        try:
            async for raw in self.ws:
                if isinstance(raw, bytes):
                    raw = raw.decode()
                try:
                    f = json.loads(raw)
                except json.JSONDecodeError:
                    continue
                await self._dispatch(f)
        except websockets.ConnectionClosed:
            pass

    async def _dispatch(self, f: Dict[str, Any]):
        kind = f.get("kind", "")
        seq = f.get("seq", 0)
        if seq > self._last_seq_received:
            self._last_seq_received = seq

        if kind == "HEY":
            if "session_id" in f:
                self.session_id = f["session_id"]
            self.server_version = f.get("v")
            self._ready_event.set()
            if "heartbeat" in (f.get("supports") or []):
                self._hb_task = asyncio.create_task(self._heartbeat_loop())
        elif kind == "HBA":
            pass
        elif kind == "LST":
            fut = self._pending.pop(seq, None)
            if fut and not fut.done():
                fut.set_result(f.get("tools", []))
        elif kind == "RES":
            if "cost" in f:
                self._accumulate_cost(f["cost"])
            fut = self._pending.pop(seq, None)
            if fut and not fut.done():
                fut.set_result(f.get("output"))
        elif kind == "STR" or kind == "EVT":
            q = self._streams.get(seq)
            if q:
                await q.put(f.get("data"))
        elif kind == "END":
            if "cost" in f:
                self._accumulate_cost(f["cost"])
            q = self._streams.pop(seq, None)
            if q:
                await q.put(None)
        elif kind == "ERR":
            err = QuarkError(
                code=f.get("code", "UNKNOWN"),
                message=f.get("message", ""),
                stage=f.get("stage"),
                trace_id=f.get("trace_id"),
            )
            fut = self._pending.pop(seq, None)
            if fut and not fut.done():
                fut.set_exception(err)
            q = self._streams.pop(seq, None)
            if q:
                await q.put(err)

    def _accumulate_cost(self, cost: Dict[str, Any]):
        for k in ("compute_ms", "api_calls", "tokens"):
            if k in cost:
                self.cost_accum[k] = self.cost_accum.get(k, 0) + cost[k]
        if "usd" in cost:
            self.cost_accum["usd"] = self.cost_accum.get("usd", 0.0) + cost["usd"]

    async def _heartbeat_loop(self):
        try:
            while not self._closed:
                await asyncio.sleep(self.opts.heartbeat_interval_s)
                if self.ws and self.ws.state.name == "OPEN":
                    await self._send({"v": PROTOCOL_VERSION, "kind": "HBT", "ts": int(time.time())})
        except asyncio.CancelledError:
            pass

    async def list_tools(self) -> List[Dict[str, Any]]:
        seq = self._next_seq()
        fut: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[seq] = fut
        await self._send({"v": PROTOCOL_VERSION, "kind": "LST", "seq": seq})
        return await fut

    async def invoke(self, tool: str, input: Optional[Dict] = None,
                     trace_id: Optional[str] = None, span_id: Optional[str] = None):
        seq = self._next_seq()
        fut: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[seq] = fut
        frame = {
            "v": PROTOCOL_VERSION,
            "kind": "INV",
            "seq": seq,
            "tool": tool,
            "input": input or {},
        }
        if trace_id:
            frame["trace_id"] = trace_id
        if span_id:
            frame["span_id"] = span_id
        await self._send(frame)
        return await fut

    async def stream(self, tool: str, input: Optional[Dict] = None) -> AsyncIterator[Any]:
        seq = self._next_seq()
        q: asyncio.Queue = asyncio.Queue()
        self._streams[seq] = q
        await self._send({
            "v": PROTOCOL_VERSION, "kind": "INV", "seq": seq,
            "tool": tool, "input": input or {},
        })

        while True:
            chunk = await q.get()
            if chunk is None:
                return
            if isinstance(chunk, QuarkError):
                raise chunk
            yield chunk

    async def pipeline(self, stages: List[Dict[str, Any]], trace_id: Optional[str] = None) -> Any:
        seq = self._next_seq()
        fut: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[seq] = fut
        frame = {
            "v": PROTOCOL_VERSION,
            "kind": "INV",
            "seq": seq,
            "pipeline": stages,
        }
        if trace_id:
            frame["trace_id"] = trace_id
        await self._send(frame)
        return await fut

    async def subscribe(self, topic: str, filter: Optional[Dict] = None) -> AsyncIterator[Any]:
        seq = self._next_seq()
        q: asyncio.Queue = asyncio.Queue()
        self._streams[seq] = q
        await self._send({
            "v": PROTOCOL_VERSION, "kind": "SUB", "seq": seq,
            "topic": topic, "filter": filter or {},
        })

        try:
            while True:
                ev = await q.get()
                if ev is None:
                    return
                if isinstance(ev, QuarkError):
                    raise ev
                yield ev
        finally:
            try:
                await self._send({"v": PROTOCOL_VERSION, "kind": "UNS", "seq": seq})
            except Exception:
                pass

    def get_cost(self) -> Dict[str, Any]:
        return dict(self.cost_accum)

    async def close(self):
        if self._closed:
            return
        self._closed = True
        if self._hb_task:
            self._hb_task.cancel()
        try:
            await self._send({"v": PROTOCOL_VERSION, "kind": "BYE"})
        except Exception:
            pass
        if self.ws:
            await self.ws.close()
        if self._reader_task:
            self._reader_task.cancel()


class QuarkError(Exception):
    def __init__(self, code: str, message: str, stage=None, trace_id=None):
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
        self.stage = stage
        self.trace_id = trace_id
