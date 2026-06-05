/**
 * @unyly/quark-client — TypeScript SDK for the Quark Protocol v0.1.
 *
 * Quark is a streaming-first AI tool protocol that replaces MCP.
 * Spec: https://unyly.org/quark — docs/quark/spec.md
 *
 * Usage:
 *
 *   import { Quark } from '@unyly/quark-client'
 *
 *   const ch = await Quark.connect('wss://server/quark', {
 *     agent: { id: 'my-agent', kind: 'llm', name: 'My Agent' },
 *     capabilities: ['github:read:*'],
 *   })
 *
 *   const tools = await ch.listTools()
 *
 *   // One-shot
 *   const repos = await ch.invoke('github.list_repos', { owner: 'anthropic' })
 *
 *   // Streaming
 *   for await (const chunk of ch.stream('logs.tail', { file: 'app.log' })) {
 *     console.log(chunk.line)
 *   }
 *
 *   // Pipeline (server-side composition)
 *   const filtered = await ch.pipeline([
 *     { tool: 'github.list_repos', input: { owner: 'anthropic' } },
 *     { filter: 'stars > 100' },
 *     { map: ['name'] },
 *   ])
 *
 *   // Subscriptions
 *   const sub = await ch.subscribe('github.pr_opened', { repo: 'foo/bar' })
 *   for await (const event of sub.events) {
 *     console.log('new PR:', event.pr)
 *   }
 *
 *   await ch.close()
 */

export type AgentInfo = {
  id: string
  kind?: 'llm' | 'agent' | 'human' | string
  name?: string
}

export type ConnectOptions = {
  agent: AgentInfo
  capabilities?: string[]
  supports?: string[]
}

export type ToolMeta = {
  name: string
  description?: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  effects?: string[]
  cost?: { estimate: number; currency: string }
  streaming?: boolean
  requires_capability?: string
}

export type PipelineStage =
  | { tool: string; input?: Record<string, unknown>; input_bind?: Record<string, unknown> }
  | { filter: string }
  | { map: string[] }

type Frame = Record<string, unknown> & { kind: string; seq?: number }
type Pending = {
  resolve: (data: unknown) => void
  reject: (err: Error) => void
  stream?: {
    push: (chunk: unknown) => void
    end: () => void
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
  private seq = 1
  private pending = new Map<number, Pending>()
  ready: Promise<void>
  private resolveReady!: () => void
  private rejectReady!: (e: Error) => void
  private closed = false

  constructor(url: string, opts: ConnectOptions) {
    this.ready = new Promise((res, rej) => {
      this.resolveReady = res
      this.rejectReady = rej
    })

    this.ws = new WebSocket(url)
    this.ws.onopen = () => {
      this.send({
        kind: 'HEY',
        v: 1,
        agent: opts.agent,
        capabilities: opts.capabilities ?? [],
        supports: opts.supports ?? ['streaming', 'subscribe', 'compose', 'capabilities'],
      })
    }
    this.ws.onmessage = (ev) => this.onFrame(ev.data)
    this.ws.onerror = (ev) => {
      const e = new Error('Quark websocket error')
      this.rejectReady(e)
      this.failAllPending(e)
    }
    this.ws.onclose = () => {
      this.closed = true
      this.failAllPending(new Error('Quark connection closed'))
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
    const p = this.pending.get(seq)

    switch (kind) {
      case 'HEY':
        this.resolveReady()
        return
      case 'LST':
      case 'RES':
        if (p) {
          // For LST: tools array; for RES: output
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
          p.stream.end()
          this.pending.delete(seq)
        }
        return
      case 'ERR':
        if (p) {
          const err = new Error(String(f.message ?? f.code ?? 'Quark error'))
          if (p.stream) p.stream.error(err)
          else p.reject(err)
          this.pending.delete(seq)
        }
        return
    }
  }

  private failAllPending(e: Error) {
    for (const p of this.pending.values()) {
      if (p.stream) p.stream.error(e)
      else p.reject(e)
    }
    this.pending.clear()
  }

  // ─────────────────────────────────────────────────────────────
  // Public API
  // ─────────────────────────────────────────────────────────────

  /** List all tools registered on the server. */
  async listTools(): Promise<ToolMeta[]> {
    const seq = this.next()
    return new Promise<ToolMeta[]>((resolve, reject) => {
      this.pending.set(seq, {
        resolve: (data) => resolve(data as ToolMeta[]),
        reject,
      })
      this.send({ kind: 'LST', seq })
    })
  }

  /** Invoke a tool (one-shot). */
  async invoke<T = unknown>(tool: string, input: Record<string, unknown> = {}): Promise<T> {
    const seq = this.next()
    return new Promise<T>((resolve, reject) => {
      this.pending.set(seq, {
        resolve: (data) => resolve(data as T),
        reject,
      })
      this.send({ kind: 'INV', seq, tool, input })
    })
  }

  /**
   * Invoke a tool in streaming mode. Returns an async iterable of chunks.
   * Cancel by `break`ing out of the loop or calling .cancel().
   */
  stream<T = unknown>(tool: string, input: Record<string, unknown> = {}): AsyncStream<T> {
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
    this.send({ kind: 'INV', seq, tool, input })
    return {
      [Symbol.asyncIterator]: () => sink.iterator,
      cancel: () => {
        this.send({ kind: 'CAN', seq })
        sink.end()
        this.pending.delete(seq)
      },
    }
  }

  /**
   * Run a server-side pipeline. Stages execute sequentially server-side; only
   * the final result returns to the client. This is Quark's killer feature.
   */
  async pipeline<T = unknown>(stages: PipelineStage[]): Promise<T> {
    const seq = this.next()
    return new Promise<T>((resolve, reject) => {
      this.pending.set(seq, {
        resolve: (data) => resolve(data as T),
        reject,
      })
      this.send({ kind: 'INV', seq, pipeline: stages })
    })
  }

  /**
   * Subscribe to a topic; events arrive as async iterable.
   * Unsubscribe by calling .close() or letting the channel close.
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
    this.send({ kind: 'SUB', seq, topic, filter })
    return {
      events: { [Symbol.asyncIterator]: () => sink.iterator } as AsyncIterable<T>,
      close: () => {
        this.send({ kind: 'UNS', seq })
        sink.end()
        this.pending.delete(seq)
      },
    }
  }

  /** Close the channel cleanly. */
  async close(): Promise<void> {
    if (this.closed) return
    try {
      this.send({ kind: 'BYE' })
    } catch {}
    this.ws.close()
  }
}

// ─────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────

export interface AsyncStream<T> extends AsyncIterable<T> {
  cancel(): void
}

export interface Subscription<T> {
  events: AsyncIterable<T>
  close(): void
}

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

function makeAsyncSink<T>() {
  const queue: T[] = []
  const waiters: Array<(r: IteratorResult<T>) => void> = []
  let ended = false
  let error: Error | null = null

  const iterator: AsyncIterator<T> = {
    next(): Promise<IteratorResult<T>> {
      if (error) return Promise.reject(error)
      if (queue.length > 0) {
        return Promise.resolve({ value: queue.shift() as T, done: false })
      }
      if (ended) return Promise.resolve({ value: undefined as unknown as T, done: true })
      return new Promise((resolve) => {
        waiters.push(resolve)
      })
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
      while (waiters.length) {
        const w = waiters.shift()!
        w({ value: undefined as unknown as T, done: true })
      }
    },
    error(err: Error) {
      error = err
      // Reject pending waiters by completing them, error will be thrown on next .next() call
      while (waiters.length) {
        const w = waiters.shift()!
        w({ value: undefined as unknown as T, done: true })
      }
    },
  }
}
