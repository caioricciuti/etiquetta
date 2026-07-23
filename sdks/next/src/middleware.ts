/**
 * Next.js middleware helper that tracks a pageview for every matched request.
 *
 * Usage in `middleware.ts`:
 *
 *   import { NextResponse } from 'next/server'
 *   import type { NextRequest, NextFetchEvent } from 'next/server'
 *   import { etiquettaMiddleware, configure } from '@etiquetta/next/middleware'
 *
 *   configure({ endpoint: '...', siteId: '...' })
 *
 *   export function middleware(req: NextRequest, event: NextFetchEvent) {
 *     etiquettaMiddleware(req, event)
 *     return NextResponse.next()
 *   }
 *
 *   export const config = { matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'] }
 *
 * The tracking fetch never blocks the response: with a NextFetchEvent it runs
 * via `event.waitUntil`, otherwise it is fired and forgotten. Middleware runs
 * on the Edge runtime, so we use `fetch` directly rather than the batching
 * client, since Edge functions are short-lived.
 */

import type { NextRequest } from 'next/server'

let ENDPOINT = ''
let SITE_ID = ''

interface WaitUntilEvent {
  waitUntil(promise: Promise<unknown>): void
}

export function configure(opts: { endpoint: string; siteId: string }) {
  ENDPOINT = opts.endpoint.replace(/\/+$/, '')
  SITE_ID = opts.siteId
}

export function etiquettaMiddleware(req: NextRequest, event?: WaitUntilEvent): void {
  if (!ENDPOINT || !SITE_ID) return
  const wire = {
    site_id: SITE_ID,
    type: 'event',
    event_type: 'pageview',
    url: req.url,
    referrer_url: req.headers.get('referer') ?? undefined,
  }
  const send = fetch(`${ENDPOINT}/i`, {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: JSON.stringify(wire),
  }).then(
    () => undefined,
    () => undefined, // tracking failures must never break the app
  )
  if (event) {
    event.waitUntil(send)
  } else {
    void send
  }
}
