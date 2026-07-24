/**
 * Server-side hooks for tracking from Next.js App Router server components,
 * Route Handlers, and Server Actions.
 */

export { Etiquetta, init, track, flush } from '@etiquetta/node'
export type { EtiquettaOptions, TrackEventInput, EventType } from '@etiquetta/node'

import { Etiquetta, type EtiquettaOptions, type TrackEventInput } from '@etiquetta/node'

let server: Etiquetta | null = null

/** Initialise the Etiquetta server SDK. Call once at module load. */
export function configure(opts: EtiquettaOptions): Etiquetta {
  server = new Etiquetta(opts)
  return server
}

/**
 * Track an event. Safe to call from Server Components, Route Handlers, and
 * Server Actions. If `configure()` hasn't been called, this is a no-op.
 */
export function trackServer(input: TrackEventInput): void {
  if (!server) return
  server.track(input)
}

/** Flush queued events — call during graceful shutdown (rare in Next.js). */
export function flushServer(): Promise<void> {
  if (!server) return Promise.resolve()
  return server.flush()
}
