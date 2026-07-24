# @etiquetta/node

Server-side tracking SDK for [Etiquetta](https://etiquetta.com) analytics.

## Install

```bash
bun add @etiquetta/node
# or
npm install @etiquetta/node
```

## Usage

```ts
import { Etiquetta } from '@etiquetta/node'

const analytics = new Etiquetta({
  endpoint: 'https://analytics.example.com',
  siteId: 'site_abc123',
  domain: 'example.com', // the domain registered in Etiquetta
})

analytics.track({
  type: 'pageview',
  path: '/pricing',
  visitorId: 'user_42',
  referrer: 'https://google.com/',
})

analytics.track({
  type: 'custom',
  event: 'signup_completed',
  visitorId: 'user_42',
  properties: { plan: 'pro' },
})

// Flush before the process exits (e.g. in Lambda, serverless).
await analytics.close()
```

### Singleton API

For apps where a single client suffices:

```ts
import { init, track, flush } from '@etiquetta/node'

init({ endpoint: '...', siteId: '...', domain: 'example.com' })

track({ type: 'pageview', path: '/home' })

// `beforeExit` does NOT fire on SIGTERM/SIGINT — hook those explicitly so
// containers and Kubernetes shutdowns don't drop buffered events.
for (const sig of ['SIGTERM', 'SIGINT'] as const) {
  process.on(sig, () => {
    void flush().finally(() => process.exit(0))
  })
}
```

## Options

| Option            | Default | Description |
| ----------------- | ------- | ----------- |
| `endpoint`        | —       | Required. Full origin of your Etiquetta server. |
| `siteId`          | —       | Required. The site ID from Etiquetta settings. |
| `domain`          | —       | Domain registered for the site; used to build event URLs from `path`. |
| `batchSize`       | 50      | Events per flush batch. |
| `flushIntervalMs` | 5000    | Max ms between flushes. |
| `maxQueueSize`    | 5000    | Max events buffered across retries; overflow drops oldest first. |
| `fetch`           | global  | Custom fetch (Node 18+ has it built in). |
| `onError`         | console | Called on ingest errors. |

## Notes

- The SDK batches events in memory and flushes async. Always call `close()` or `flush()` on shutdown.
- Every event needs a `url`, or a `path` plus a `domain` (per event or via options) — the server derives the event's domain and path from the URL, and the dashboard filters by domain.
- `visitorId` should be stable per user (e.g. a user ID). It is hashed client-side before sending. If omitted, all events from one app server collapse into a single visitor, since the server otherwise buckets by IP and user agent of the SDK host.
- Enrichment (geo, device, bot scoring) comes from the HTTP request the SDK makes, not from the tracked user. Server-side events are best suited for custom events tied to a `visitorId`; use the browser tracker for audience analytics.
- If a flush fails, the batch is re-queued (bounded by `maxQueueSize`) and retried on the next flush.
- For privacy, do **not** send PII in `properties`.
