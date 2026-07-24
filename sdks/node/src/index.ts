/**
 * Etiquetta server-side SDK.
 *
 * Batches events in memory and flushes to the `/i` endpoint on a timer or when
 * the batch fills. Safe to call `track()` thousands of times per second — the
 * SDK will coalesce into bulk NDJSON requests.
 *
 * Usage:
 *   const client = new Etiquetta({
 *     endpoint: 'https://analytics.example.com',
 *     siteId: 'site_abc123',
 *     domain: 'example.com',
 *   })
 *   client.track({ path: '/signup', visitorId: 'user_123' })
 *   await client.close() // call on server shutdown (SIGTERM/SIGINT)
 *
 * Note on enrichment: the server derives client IP, user agent, geo, and bot
 * score from the HTTP request that delivers the batch — not from event fields.
 * Server-side events therefore carry the SDK host's request context. Pass a
 * stable `visitorId` per user so sessions and unique-visitor counts stay
 * meaningful.
 */

import { createHash } from 'node:crypto'

export interface EtiquettaOptions {
  /** Full origin of the Etiquetta server, e.g. https://analytics.example.com */
  endpoint: string
  /** Site ID configured in Etiquetta settings (same as the browser tracker). */
  siteId: string
  /**
   * Domain registered for the site in Etiquetta, e.g. "example.com". Used to
   * build the event URL when `track()` is called with only a `path`.
   */
  domain?: string
  /** Max events per batch. Default 50. */
  batchSize?: number
  /** Max ms between flushes. Default 5000. */
  flushIntervalMs?: number
  /** Max events held in memory across retries. Default 5000; overflow is dropped oldest-first. */
  maxQueueSize?: number
  /** Optional custom fetch implementation (for testing, or environments without global fetch). */
  fetch?: typeof fetch
  /** Called when a flush fails. Default: console.error. Set to noop to silence. */
  onError?: (err: unknown) => void
}

export type EventType = 'pageview' | 'custom'

export interface TrackEventInput {
  /** Event type. Defaults to 'custom' when `event` is set, 'pageview' otherwise. */
  type?: EventType
  /** Event name — required for custom events. */
  event?: string
  /** Page path, e.g. "/signup". Combined with the configured `domain` when `url` is absent. */
  path?: string
  /** Full page URL. Takes precedence over `path`; the server derives domain and path from it. */
  url?: string
  /** Stable per-user identifier. Hashed client-side before sending. */
  visitorId?: string
  /** HTTP referrer URL. */
  referrer?: string
  /** Arbitrary event properties. */
  properties?: Record<string, unknown>
  /** Override the domain for this event when building the URL from `path`. */
  domain?: string
  /** UTM parameters. */
  utmSource?: string
  utmMedium?: string
  utmCampaign?: string
}

type WireEvent = Record<string, unknown>

const HEX_FINGERPRINT = /^[0-9a-fA-F]{16,64}$/

/**
 * The server only accepts visitor hashes that look like hex fingerprints
 * (16-64 hex chars); anything else is discarded and replaced. Hash arbitrary
 * IDs so any stable string works.
 */
function normalizeVisitorId(id?: string): string | undefined {
  if (!id) return undefined
  if (HEX_FINGERPRINT.test(id)) return id.toLowerCase()
  return createHash('sha256').update(id).digest('hex')
}

export class Etiquetta {
  private endpoint: string
  private siteId: string
  private domain?: string
  private batchSize: number
  private flushIntervalMs: number
  private maxQueueSize: number
  private fetchImpl: typeof fetch
  private onError: (err: unknown) => void

  private queue: WireEvent[] = []
  private timer: ReturnType<typeof setTimeout> | null = null
  private closed = false

  constructor(opts: EtiquettaOptions) {
    if (!opts.endpoint) throw new Error('Etiquetta: endpoint is required')
    if (!opts.siteId) throw new Error('Etiquetta: siteId is required')
    this.endpoint = opts.endpoint.replace(/\/+$/, '')
    this.siteId = opts.siteId
    this.domain = opts.domain
    this.batchSize = opts.batchSize ?? 50
    this.flushIntervalMs = opts.flushIntervalMs ?? 5000
    this.maxQueueSize = opts.maxQueueSize ?? 5000
    this.fetchImpl = opts.fetch ?? fetch
    this.onError = opts.onError ?? ((e) => console.error('[etiquetta]', e))
  }

  /** Queue an event. Non-blocking; returns immediately. */
  track(input: TrackEventInput): void {
    if (this.closed) return
    const eventType = input.type ?? (input.event ? 'custom' : 'pageview')
    const domain = input.domain ?? this.domain
    const url =
      input.url ?? (input.path && domain ? `https://${domain}${input.path}` : undefined)
    const wire: WireEvent = {
      site_id: this.siteId,
      type: 'event',
      event_type: eventType,
      event_name: input.event,
      url,
      referrer_url: input.referrer,
      visitor_hash: normalizeVisitorId(input.visitorId),
      utm_source: input.utmSource,
      utm_medium: input.utmMedium,
      utm_campaign: input.utmCampaign,
      props: input.properties ? JSON.stringify(input.properties) : undefined,
    }
    // Drop undefined fields — keeps the wire format tidy.
    for (const k of Object.keys(wire)) {
      if (wire[k] === undefined) delete wire[k]
    }
    this.queue.push(wire)
    if (this.queue.length > this.maxQueueSize) {
      this.queue.splice(0, this.queue.length - this.maxQueueSize)
    }
    if (this.queue.length >= this.batchSize) {
      void this.flush()
    } else {
      this.scheduleFlush()
    }
  }

  /**
   * Flush all queued events immediately. Resolves when the request completes.
   * On failure the batch is re-queued (up to `maxQueueSize`) and retried on
   * the next flush.
   */
  async flush(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer)
      this.timer = null
    }
    if (this.queue.length === 0) return
    const batch = this.queue
    this.queue = []
    const ndjson = batch.map((e) => JSON.stringify(e)).join('\n')
    try {
      const res = await this.fetchImpl(`${this.endpoint}/i`, {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: ndjson,
      })
      if (!res.ok && res.status !== 204) {
        const text = await res.text().catch(() => '')
        throw new Error(`ingest failed (${res.status}): ${text}`)
      }
    } catch (err) {
      this.requeue(batch)
      this.onError(err)
    }
  }

  /** Stop the flush timer and flush any remaining events. */
  async close(): Promise<void> {
    this.closed = true
    if (this.timer) {
      clearTimeout(this.timer)
      this.timer = null
    }
    await this.flush()
  }

  private requeue(batch: WireEvent[]) {
    if (this.closed) return
    this.queue = batch.concat(this.queue)
    if (this.queue.length > this.maxQueueSize) {
      this.queue.splice(0, this.queue.length - this.maxQueueSize)
    }
    this.scheduleFlush()
  }

  private scheduleFlush() {
    if (this.timer) return
    this.timer = setTimeout(() => {
      this.timer = null
      void this.flush()
    }, this.flushIntervalMs)
    // Don't keep short-lived processes (scripts, drained Lambdas) alive.
    this.timer.unref?.()
  }
}

/** Convenience singleton constructor for environments where a single client suffices. */
let singleton: Etiquetta | null = null

export function init(opts: EtiquettaOptions): Etiquetta {
  singleton = new Etiquetta(opts)
  return singleton
}

export function track(input: TrackEventInput): void {
  if (!singleton) throw new Error('Etiquetta: call init() first')
  singleton.track(input)
}

export function flush(): Promise<void> {
  if (!singleton) return Promise.resolve()
  return singleton.flush()
}
