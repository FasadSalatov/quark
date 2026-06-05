/**
 * @unyly/quark-client — TypeScript SDK for the Quark Protocol v0.2.
 *
 * Quark is a streaming-first AI tool protocol that replaces MCP.
 * Spec: https://github.com/FasadSalatov/quark/blob/main/docs/spec.md
 *
 * v0.2 features:
 *   - Signed capability tokens (QCT) via {@link QCT.create} / {@link QCT.verify}
 *   - Bearer authentication
 *   - Session resume after disconnect (auto-reconnect)
 *   - Heartbeat
 *   - Cost tracking
 *   - Tracing (trace_id / span_id)
 *
 * Quick start:
 * ```ts
 * import { Quark, QCT } from '@unyly/quark-client'
 *
 * const token = QCT.create({
 *   secret: 'shared-secret',
 *   payload: {
 *     iss: 'https://my-app.com',
 *     sub: 'user@example.com',
 *     exp: Math.floor(Date.now() / 1000) + 3600,
 *     scope: ['echo:invoke'],
 *   },
 * })
 *
 * const ch = await Quark.connect('wss://server/quark/ws', {
 *   agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
 *   auth: { type: 'bearer', token },
 * })
 *
 * const tools = await ch.listTools()
 * const result = await ch.invoke('echo.upper', { text: 'hello' })
 * for await (const chunk of ch.stream('logs.tail', { file: 'app.log' })) console.log(chunk)
 * const composed = await ch.pipeline([
 *   { tool: 'demo.fake_repos' },
 *   { filter: 'stars > 100' },
 *   { map: ['name'] },
 * ])
 * ```
 */

export const ProtocolVersion = 1

// ─────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────

export type AgentInfo = {
  id: string
  kind?: 'llm' | 'agent' | 'human' | string
  name?: string
}

export type AuthBearer = { type: 'bearer'; token: string }

export type ConnectOptions = {
  agent: AgentInfo
  auth?: AuthBearer
  capabilities?: string[]
  supports?: string[]
  /** Auto-reconnect with session resume. Default: true. */
  autoReconnect?: boolean
  /** Heartbeat interval in ms. Default: 30000. */
  heartbeatIntervalMs?: number
}

export type ToolMeta = {
  name: string
  version?: string
  description?: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  effects?: string[]
  cost?: { estimate: number; currency: string }
  streaming?: boolean
  requires_capability?: string
}

export type Cost = {
  compute_ms?: number
  api_calls?: number
  usd?: number
  tokens?: number
}

export type PipelineStage =
  | { tool: string; input?: Record<string, unknown>; input_bind?: Record<string, unknown> }
  | { filter: string }
  | { map: string[] }

export type QCTPayload = {
  iss: string
  sub: string
  iat?: number
  nbf?: number
  exp: number
  scope: string[]
  client_id?: string
  session_id?: string
  max_cost_usd?: number
}

// ─────────────────────────────────────────────────────────────
// QCT (Quark Capability Token)
// ─────────────────────────────────────────────────────────────

/**
 * QCT mint / verify utilities.
 *
 * Tokens look like:
 *   qct.v1.<base64url(payload)>.<base64url(hmac-sha256(secret, "v1." + payload))>
 */
export const QCT = {
  /**
   * Mint a signed token.
   */
  async create(opts: { secret: string | Uint8Array; payload: QCTPayload }): Promise<string> {
    const payload = { ...opts.payload }
    if (!payload.iat) payload.iat = Math.floor(Date.now() / 1000)
    if (!payload.exp) throw new Error('QCT payload.exp required')

    const bytes = new TextEncoder().encode(JSON.stringify(payload))
    const encoded = base64UrlEncode(bytes)
    const signing = 'v1.' + encoded
    const sig = await hmacSha256(opts.secret, signing)
    return 'qct.v1.' + encoded + '.' + base64UrlEncode(sig)
  },

  /**
   * Verify a token's signature and time bounds. Returns payload or throws.
   */
  async verify(token: string, secret: string | Uint8Array): Promise<QCTPayload> {
    const parts = token.split('.')
    if (parts.length !== 4 || parts[0] !== 'qct' || parts[1] !== 'v1') {
      throw new Error('malformed QCT')
    }
    const [, , encoded, sig] = parts
    const expected = await hmacSha256(secret, 'v1.' + encoded)
    if (base64UrlEncode(expected) !== sig) {
      throw new Error('signature mismatch')
    }
    const decoded = new TextDecoder().decode(base64UrlDecode(encoded))
    const payload = JSON.parse(decoded) as QCTPayload
    const now = Math.floor(Date.now() / 1000)
    if (payload.nbf && now < payload.nbf) throw new Error('token not yet valid (nbf)')
    if (payload.exp <= now) throw new Error('token expired')
    return payload
  },
}

async function hmacSha256(
  secret: string | Uint8Array,
  message: string,
): Promise<Uint8Array> {
  const keyData = (typeof secret === 'string' ? new TextEncoder().encode(secret) : secret) as ArrayBuffer | ArrayBufferView
  const key = await crypto.subtle.importKey(
    'raw',
    keyData as BufferSource,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign'],
  )
  const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(message))
  return new Uint8Array(sig)
}

function base64UrlEncode(bytes: Uint8Array): string {
  let s = ''
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i])
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function base64UrlDecode(s: string): Uint8Array {
  const padded = s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

// ─────────────────────────────────────────────────────────────
// Tracing
// ─────────────────────────────────────────────────────────────

/** Generates a fresh W3C trace_id (32 hex chars). */
export function newTraceId(): string {
  return hex(16)
}

/** Generates a fresh W3C span_id (16 hex chars). */
export function newSpanId(): string {
  return hex(8)
}

function hex(byteLen: number): string {
  const bytes = new Uint8Array(byteLen)
  crypto.getRandomValues(bytes)
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('')
}

// ─────────────────────────────────────────────────────────────
// Channel + Quark
// ─────────────────────────────────────────────────────────────

type Frame = Record<string, unknown> & { kind: string; seq?: number }

type Pending = {
  resolve: (data: unknown) => void
  reject: (err: Error) => void
  stream?: {
    push: (chunk: unknown) => void
    end: (final?: { cost?: Cost }) => void
    error: (err: Error) => void
  }
}

export class Quark {
  static async connect(url: string, opts: ConnectOptions): Promise<Channel> {
    const ch = new Channel(url, opts)
    await ch.ready
    return ch
  }
}

export class Channel {
  private ws!: WebSocket
  private url: string
  private opts: ConnectOptions
  private seq = 1
  private pending = new Map<number, Pending>()
  ready: Promise<void>
  private resolveReady!: () => void
  private rejectReady!: (e: Error) => void
  private closed = false
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  sessionId: string | null = null
  /** Running cost accumulator from RES/END frames. */
  costAccum: Cost = { compute_ms: 0, api_calls: 0, usd: 0, tokens: 0 }
  private lastSeqReceived = 0

  constructor(url: string, opts: ConnectOptions) {
    this.url = url
    this.opts = opts
    this.ready = new Promise((res, rej) => {
      this.resolveReady = res
      this.rejectReady = rej
    })
    this.open()
  }

  private open() {
    this.ws = new WebSocket(this.url)
    this.ws.onopen = () => {
      this.send({
        v: ProtocolVersion,
        kind: 'HEY',
        agent: this.opts.agent,
        auth: this.opts.auth,
        capabilities: this.opts.capabilities ?? [],
        supports: this.opts.supports ?? [
          'streaming', 'subscribe', 'compose', 'capabilities',
          'resume', 'tracing', 'heartbeat', 'validation',
        ],
      })
    }
    this.ws.onmessage = (ev) => this.onFrame(ev.data)
    this.ws.onerror = () => {
      const e = new Error('Quark websocket error')
      this.rejectReady(e)
    }
    this.ws.onclose = () => {
      this.stopHeartbeat()
      if (this.closed) {
        this.failAllPending(new Error('Quark connection closed'))
        return
      }
      // Auto-resume
      if (this.opts.autoReconnect !== false && this.sessionId) {
        setTimeout(() => this.resume(), 1000)
      } else {
        this.failAllPending(new Error('Quark connection closed'))
      }
    }
  }

  private resume() {
    this.ws = new WebSocket(this.url)
    this.ws.onopen = () => {
      this.send({
        v: ProtocolVersion,
        kind: 'RSM',
        session_id: this.sessionId!,
        last_seq_received: this.lastSeqReceived,
      })
    }
    this.ws.onmessage = (ev) => this.onFrame(ev.data)
    this.ws.onclose = () => {
      this.stopHeartbeat()
      if (this.closed) return
      // Try again
      if (this.opts.autoReconnect !== false) {
        setTimeout(() => this.resume(), 2000)
      }
    }
  }

  private startHeartbeat() {
    const interval = this.opts.heartbeatIntervalMs ?? 30000
    this.stopHeartbeat()
    this.heartbeatTimer = setInterval(() => {
      if (this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ v: ProtocolVersion, kind: 'HBT', ts: Math.floor(Date.now() / 1000) }))
      }
    }, interval)
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }

  private send(payload: Frame) {
    if (this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('Quark channel not open')
    }
    this.ws.send(JSON.stringify(payload))
  }

  private next(): number {
    return this.seq++
  }

  private onFrame(raw: string | ArrayBuffer | Blob) {
    if (typeof raw !== 'string') return
    let f: Frame
    try {
      f = JSON.parse(raw)
    } catch {
      return
    }
    const kind = f.kind
    const seq = (f.seq as number) ?? 0
    if (seq > this.lastSeqReceived) this.lastSeqReceived = seq
    const p = this.pending.get(seq)

    switch (kind) {
      case 'HEY':
        if (f.session_id) this.sessionId = String(f.session_id)
        this.resolveReady()
        this.startHeartbeat()
        return
      case 'HBA':
        return
      case 'LST':
      case 'RES':
        if (p) {
          if (f.cost) this.accumulateCost(f.cost as Cost)
          const data = (f.tools as unknown) ?? (f.output as unknown) ?? f
          p.resolve(data)
          this.pending.delete(seq)
        }
        return
      case 'STR':
        if (p?.stream) p.stream.push(f.data)
        return
      case 'EVT':
        if (p?.stream) p.stream.push(f.data)
        return
      case 'END':
        if (p?.stream) {
          if (f.cost) this.accumulateCost(f.cost as Cost)
          p.stream.end({ cost: f.cost as Cost | undefined })
          this.pending.delete(seq)
        }
        return
      case 'ERR':
        if (p) {
          const err = new Error(String(f.message ?? f.code ?? 'Quark error'))
          ;(err as Error & { code?: string }).code = String(f.code ?? '')
          if (p.stream) p.stream.error(err)
          else p.reject(err)
          this.pending.delete(seq)
        }
        return
      case 'WIN':
        // backpressure — could implement queue throttle here
        return
    }
  }

  private accumulateCost(c: Cost) {
    if (!c) return
    this.costAccum.compute_ms = (this.costAccum.compute_ms ?? 0) + (c.compute_ms ?? 0)
    this.costAccum.api_calls = (this.costAccum.api_calls ?? 0) + (c.api_calls ?? 0)
    this.costAccum.usd = (this.costAccum.usd ?? 0) + (c.usd ?? 0)
    this.costAccum.tokens = (this.costAccum.tokens ?? 0) + (c.tokens ?? 0)
  }

  private failAllPending(e: Error) {
    for (const p of this.pending.values()) {
      if (p.stream) p.stream.error(e)
      else p.reject(e)
    }
    this.pending.clear()
  }

  // ───────────────────────────────────────────────────────────
  // Public API
  // ───────────────────────────────────────────────────────────

  /** List all tools registered on the server. */
  async listTools(): Promise<ToolMeta[]> {
    const seq = this.next()
    return new Promise<ToolMeta[]>((resolve, reject) => {
      this.pending.set(seq, { resolve: (data) => resolve(data as ToolMeta[]), reject })
      this.send({ v: ProtocolVersion, kind: 'LST', seq })
    })
  }

  /**
   * Invoke a tool (one-shot).
   * @param tool tool name (optionally with @version suffix)
   * @param input input object
   * @param tracing optional trace_id/span_id for distributed tracing
   */
  async invoke<T = unknown>(
    tool: string,
    input: Record<string, unknown> = {},
    tracing?: { trace_id?: string; span_id?: string; parent_span_id?: string },
  ): Promise<T> {
    const seq = this.next()
    return new Promise<T>((resolve, reject) => {
      this.pending.set(seq, { resolve: (data) => resolve(data as T), reject })
      this.send({ v: ProtocolVersion, kind: 'INV', seq, tool, input, ...tracing })
    })
  }

  /**
   * Invoke a tool in streaming mode. Returns an async iterable of chunks.
   */
  stream<T = unknown>(
    tool: string,
    input: Record<string, unknown> = {},
    tracing?: { trace_id?: string; span_id?: string },
  ): AsyncStream<T> {
    const seq = this.next()
    const sink = makeAsyncSink<T>()
    this.pending.set(seq, {
      resolve: () => {},
      reject: () => {},
      stream: {
        push: (chunk) => sink.push(chunk as T),
        end: () => sink.end(),
        error: (err) => sink.error(err),
      },
    })
    this.send({ v: ProtocolVersion, kind: 'INV', seq, tool, input, ...tracing })
    return {
      [Symbol.asyncIterator]: () => sink.iterator,
      cancel: () => {
        this.send({ v: ProtocolVersion, kind: 'CAN', seq })
        sink.end()
        this.pending.delete(seq)
      },
    }
  }

  /**
   * Run a server-side pipeline. Stages execute sequentially server-side; only
   * the final result returns to the client. This is Quark's killer feature.
   */
  async pipeline<T = unknown>(
    stages: PipelineStage[],
    tracing?: { trace_id?: string; span_id?: string },
  ): Promise<T> {
    const seq = this.next()
    return new Promise<T>((resolve, reject) => {
      this.pending.set(seq, { resolve: (data) => resolve(data as T), reject })
      this.send({ v: ProtocolVersion, kind: 'INV', seq, pipeline: stages, ...tracing })
    })
  }

  /**
   * Subscribe to a topic. Returns async iterable of events.
   */
  subscribe<T = unknown>(
    topic: string,
    filter: Record<string, unknown> = {},
  ): Subscription<T> {
    const seq = this.next()
    const sink = makeAsyncSink<T>()
    this.pending.set(seq, {
      resolve: () => {},
      reject: () => {},
      stream: {
        push: (chunk) => sink.push(chunk as T),
        end: () => sink.end(),
        error: (err) => sink.error(err),
      },
    })
    this.send({ v: ProtocolVersion, kind: 'SUB', seq, topic, filter })
    return {
      events: { [Symbol.asyncIterator]: () => sink.iterator } as AsyncIterable<T>,
      close: () => {
        this.send({ v: ProtocolVersion, kind: 'UNS', seq })
        sink.end()
        this.pending.delete(seq)
      },
    }
  }

  /** Close the channel cleanly. */
  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    this.stopHeartbeat()
    try {
      this.send({ v: ProtocolVersion, kind: 'BYE' })
    } catch {}
    this.ws.close()
  }

  /** Current cumulative cost since channel opened. */
  getCost(): Cost {
    return { ...this.costAccum }
  }
}

export interface AsyncStream<T> extends AsyncIterable<T> {
  cancel(): void
}

export interface Subscription<T> {
  events: AsyncIterable<T>
  close(): void
}

function makeAsyncSink<T>() {
  const queue: T[] = []
  const waiters: Array<(r: IteratorResult<T>) => void> = []
  let ended = false
  let error: Error | null = null

  const iterator: AsyncIterator<T> = {
    next(): Promise<IteratorResult<T>> {
      if (error) return Promise.reject(error)
      if (queue.length > 0) return Promise.resolve({ value: queue.shift() as T, done: false })
      if (ended) return Promise.resolve({ value: undefined as unknown as T, done: true })
      return new Promise((resolve) => waiters.push(resolve))
    },
  }

  return {
    iterator,
    push(value: T) {
      const w = waiters.shift()
      if (w) w({ value, done: false })
      else queue.push(value)
    },
    end() {
      ended = true
      while (waiters.length) waiters.shift()!({ value: undefined as unknown as T, done: true })
    },
    error(err: Error) {
      error = err
      while (waiters.length) waiters.shift()!({ value: undefined as unknown as T, done: true })
    },
  }
}


// ─── v1.0 additions ───
export { applyFilter, evalExpr } from './filter'
export type { Row as FilterRow } from './filter'
