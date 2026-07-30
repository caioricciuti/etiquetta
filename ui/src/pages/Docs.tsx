import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../components/ui/tabs'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import {
  BookOpen,
  Copy,
  Check,
  Rocket,
  MousePointerClick,
  Server,
  Terminal,
  KeyRound,
  Zap,
} from 'lucide-react'

// The server host is this app's own origin, so snippets point at the right
// place out of the box. Domain-specific values stay as placeholders.
const HOST = typeof window !== 'undefined' ? window.location.origin : 'https://your-server.com'
const SITE_ID = 'YOUR_SITE_ID'

interface Section {
  id: string
  label: string
  icon: typeof BookOpen
}

const SECTIONS: Section[] = [
  { id: 'overview', label: 'Overview', icon: BookOpen },
  { id: 'quickstart', label: 'Quick start', icon: Rocket },
  { id: 'custom-events', label: 'Custom events', icon: MousePointerClick },
  { id: 'backend', label: 'Server-side data', icon: Server },
  { id: 'ingest-api', label: 'Ingest API', icon: Terminal },
  { id: 'read-api', label: 'Reading your data', icon: KeyRound },
]

function CodeBlock({ code, label }: { code: string; label?: string }) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="group relative">
      {label && (
        <div className="absolute left-3 top-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
          {label}
        </div>
      )}
      <pre
        className={`overflow-x-auto rounded-lg border bg-muted/40 p-4 text-sm leading-relaxed ${
          label ? 'pt-8' : ''
        }`}
      >
        <code className="font-mono">{code}</code>
      </pre>
      <Button
        variant="outline"
        size="xs"
        onClick={copy}
        className="absolute right-2 top-2 opacity-0 transition-opacity group-hover:opacity-100"
        aria-label="Copy code"
      >
        {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
      </Button>
    </div>
  )
}

function Field({ name, type, required, children }: {
  name: string
  type: string
  required?: boolean
  children: React.ReactNode
}) {
  return (
    <tr className="border-b last:border-0">
      <td className="py-2 pr-4 align-top font-mono text-sm whitespace-nowrap">
        {name}
        {required && <span className="ml-1 text-destructive">*</span>}
      </td>
      <td className="py-2 pr-4 align-top font-mono text-xs text-muted-foreground whitespace-nowrap">
        {type}
      </td>
      <td className="py-2 align-top text-sm text-muted-foreground">{children}</td>
    </tr>
  )
}

function SectionHeading({ id, icon: Icon, title, desc }: {
  id: string
  icon: typeof BookOpen
  title: string
  desc?: string
}) {
  return (
    <div className="mb-4" style={{ scrollMarginTop: '1.5rem' }} id={id}>
      <h2 className="flex items-center gap-2 text-xl font-bold">
        <Icon className="h-5 w-5 text-primary" />
        {title}
      </h2>
      {desc && <p className="mt-1 text-muted-foreground">{desc}</p>}
    </div>
  )
}

export function Docs() {
  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="flex items-center gap-2 text-2xl font-bold">
          <BookOpen className="h-7 w-7" />
          Documentation
        </h1>
        <p className="text-muted-foreground">
          How to send data to Etiquetta from the browser, your backend, or any language — and how to read it back.
        </p>
      </div>

      <div className="flex flex-col gap-8 lg:flex-row">
        {/* On this page */}
        <nav className="lg:sticky lg:top-6 lg:h-fit lg:w-56 lg:shrink-0">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            On this page
          </div>
          <ul className="space-y-1">
            {SECTIONS.map((s) => (
              <li key={s.id}>
                <a
                  href={`#${s.id}`}
                  className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <s.icon className="h-4 w-4" />
                  {s.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>

        {/* Content */}
        <div className="min-w-0 flex-1 space-y-10">
          {/* Overview */}
          <section>
            <SectionHeading
              id="overview"
              icon={BookOpen}
              title="Overview"
              desc="How data flows through Etiquetta."
            />
            <Card>
              <CardContent className="space-y-4 pt-6 text-sm leading-relaxed">
                <p>
                  Every event — a pageview, a custom event, an error — is sent to the ingest endpoint
                  <code className="mx-1 rounded bg-muted px-1.5 py-0.5 font-mono">POST /i</code>
                  as newline-delimited JSON. The server enriches it (geo, device, bot score), buffers it,
                  and writes it to DuckDB. Your dashboard reads from there.
                </p>
                <p>You have three ways to send events:</p>
                <ul className="ml-5 list-disc space-y-1">
                  <li>
                    <strong>Browser tracker</strong> — a one-line script tag that auto-tracks pageviews and
                    exposes a <code className="rounded bg-muted px-1 font-mono">track()</code> API.
                  </li>
                  <li>
                    <strong>Server-side SDKs</strong> — <code className="rounded bg-muted px-1 font-mono">@etiquetta/node</code>{' '}
                    and <code className="rounded bg-muted px-1 font-mono">@etiquetta/next</code> for backend and Next.js apps.
                  </li>
                  <li>
                    <strong>Raw HTTP</strong> — post NDJSON to <code className="rounded bg-muted px-1 font-mono">/i</code> from any language.
                  </li>
                </ul>
                <p className="text-muted-foreground">
                  Etiquetta is cookieless by default and does not require consent banners in most jurisdictions.
                  Visitors are identified by a rotating server-side hash, never a persistent cookie unless you opt in.
                </p>
              </CardContent>
            </Card>
          </section>

          {/* Quick start */}
          <section>
            <SectionHeading
              id="quickstart"
              icon={Rocket}
              title="Quick start: browser tracking"
              desc="Track pageviews on any website in under a minute."
            />
            <Card>
              <CardHeader>
                <CardTitle className="text-base">1. Add the script</CardTitle>
                <CardDescription>
                  Paste this into your site's <code className="font-mono">&lt;head&gt;</code>. Find your site ID in{' '}
                  <strong>Settings → Domains</strong>.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <CodeBlock
                  label="html"
                  code={`<script src="${HOST}/s.js" data-site="${SITE_ID}" defer></script>`}
                />
                <div className="rounded-md border border-primary/20 bg-primary/5 p-3 text-sm text-muted-foreground">
                  That's it. Pageviews, SPA route changes, scroll depth, and outbound clicks are tracked automatically.
                  Verify it's working on the <strong>Install Check</strong> page.
                </div>
              </CardContent>
            </Card>
          </section>

          {/* Custom events (client) */}
          <section>
            <SectionHeading
              id="custom-events"
              icon={MousePointerClick}
              title="Custom events (browser)"
              desc="Track signups, purchases, and any interaction from the client."
            />
            <Card>
              <CardContent className="space-y-4 pt-6">
                <p className="text-sm text-muted-foreground">
                  Once the script is loaded, a global <code className="rounded bg-muted px-1 font-mono">window.etiquetta</code>{' '}
                  object is available.
                </p>
                <CodeBlock
                  label="javascript"
                  code={`// Track a named event with optional properties
window.etiquetta.track('signup_completed', { plan: 'pro' })

// Send a manual pageview (useful for SPA route changes)
window.etiquetta.pageview()

// Force-send any queued events immediately
window.etiquetta.flush()

// Read the current visitor hash (empty string if not yet assigned)
const id = window.etiquetta.getVisitorHash()`}
                />
                <div className="flex items-start gap-2 text-sm text-muted-foreground">
                  <Zap className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                  <span>
                    Keep property values non-personal. Custom events require the{' '}
                    <Badge variant="secondary" className="font-mono">custom_events</Badge> feature.
                  </span>
                </div>
              </CardContent>
            </Card>
          </section>

          {/* Server-side */}
          <section>
            <SectionHeading
              id="backend"
              icon={Server}
              title="Server-side data"
              desc="Send events from your backend when there is no browser — signups, payments, jobs."
            />
            <Tabs defaultValue="node">
              <TabsList>
                <TabsTrigger value="node">Node.js</TabsTrigger>
                <TabsTrigger value="next">Next.js</TabsTrigger>
              </TabsList>

              <TabsContent value="node" className="space-y-4">
                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">@etiquetta/node</CardTitle>
                    <CardDescription>Batching server-side SDK for any Node runtime.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <CodeBlock label="bash" code={`npm install @etiquetta/node`} />
                    <CodeBlock
                      label="typescript"
                      code={`import { Etiquetta } from '@etiquetta/node'

const analytics = new Etiquetta({
  endpoint: '${HOST}',
  siteId: '${SITE_ID}',
  domain: 'example.com', // the domain registered in Etiquetta
})

// A pageview tied to a stable user
analytics.track({ type: 'pageview', path: '/pricing', visitorId: 'user_42' })

// A custom event with properties
analytics.track({
  type: 'custom',
  event: 'signup_completed',
  visitorId: 'user_42',
  properties: { plan: 'pro' },
})

// Flush on shutdown (SIGTERM/SIGINT) so nothing is dropped
await analytics.close()`}
                    />
                    <div className="rounded-md border border-yellow-500/20 bg-yellow-500/5 p-3 text-sm text-muted-foreground">
                      <strong className="text-foreground">Heads up:</strong> the server derives geo, device, and
                      bot score from the SDK's own HTTP request, not from the tracked user. Always pass a stable{' '}
                      <code className="font-mono">visitorId</code> per user, otherwise every event collapses into a
                      single visitor. Server-side tracking is best for custom events; use the browser tracker for
                      audience analytics.
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="next" className="space-y-4">
                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">@etiquetta/next</CardTitle>
                    <CardDescription>
                      Browser script, server-side helpers, and edge-middleware pageviews for Next.js.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <CodeBlock label="bash" code={`npm install @etiquetta/next`} />
                    <div className="text-sm font-medium">Browser tracking (App Router)</div>
                    <CodeBlock
                      label="tsx — app/layout.tsx"
                      code={`import { EtiquettaScript } from '@etiquetta/next/script'

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <head>
        <EtiquettaScript endpoint="${HOST}" siteId="${SITE_ID}" />
      </head>
      <body>{children}</body>
    </html>
  )
}`}
                    />
                    <div className="text-sm font-medium">Server actions &amp; route handlers</div>
                    <CodeBlock
                      label="ts — app/actions.ts"
                      code={`'use server'
import { configure, trackServer } from '@etiquetta/next'

configure({ endpoint: '${HOST}', siteId: '${SITE_ID}', domain: 'example.com' })

export async function signupAction() {
  // ...create the user...
  trackServer({ type: 'custom', event: 'signup_completed', visitorId: user.id })
}`}
                    />
                    <div className="text-sm font-medium">Edge middleware (script-less pageviews)</div>
                    <CodeBlock
                      label="ts — middleware.ts"
                      code={`import { NextResponse } from 'next/server'
import type { NextRequest, NextFetchEvent } from 'next/server'
import { configure, etiquettaMiddleware } from '@etiquetta/next/middleware'

configure({ endpoint: '${HOST}', siteId: '${SITE_ID}' })

export function middleware(req: NextRequest, event: NextFetchEvent) {
  etiquettaMiddleware(req, event) // never blocks the response
  return NextResponse.next()
}

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
}`}
                    />
                  </CardContent>
                </Card>
              </TabsContent>
            </Tabs>
          </section>

          {/* Ingest API */}
          <section>
            <SectionHeading
              id="ingest-api"
              icon={Terminal}
              title="Raw ingest API"
              desc="No SDK for your language? Post NDJSON directly."
            />
            <Card>
              <CardContent className="space-y-4 pt-6">
                <p className="text-sm text-muted-foreground">
                  <code className="rounded bg-muted px-1.5 py-0.5 font-mono">POST {HOST}/i</code> with a{' '}
                  <code className="rounded bg-muted px-1 font-mono">text/plain</code> body of newline-delimited JSON
                  (one event object per line). Server-side requests are accepted without a browser{' '}
                  <code className="font-mono">Origin</code> as long as the <code className="font-mono">site_id</code> is valid.
                </p>
                <CodeBlock
                  label="bash"
                  code={`curl -X POST ${HOST}/i \\
  -H 'Content-Type: text/plain' \\
  --data-binary '{"site_id":"${SITE_ID}","type":"event","event_type":"pageview","url":"https://example.com/pricing","visitor_hash":"a1b2c3d4e5f60718"}'`}
                />
                <div>
                  <div className="mb-2 text-sm font-medium">Event fields</div>
                  <div className="overflow-x-auto">
                    <table className="w-full text-left">
                      <thead>
                        <tr className="border-b text-xs uppercase tracking-wide text-muted-foreground">
                          <th className="py-2 pr-4 font-medium">Field</th>
                          <th className="py-2 pr-4 font-medium">Type</th>
                          <th className="py-2 font-medium">Notes</th>
                        </tr>
                      </thead>
                      <tbody>
                        <Field name="site_id" type="string" required>
                          Your site ID (Settings → Domains). Events are dropped without a valid one.
                        </Field>
                        <Field name="type" type="string">
                          <code className="font-mono">event</code> (default path), <code className="font-mono">performance</code>,
                          or <code className="font-mono">error</code>.
                        </Field>
                        <Field name="event_type" type="string">
                          <code className="font-mono">pageview</code> (default) or <code className="font-mono">custom</code>.
                        </Field>
                        <Field name="event_name" type="string">
                          Name of the custom event (when <code className="font-mono">event_type</code> is{' '}
                          <code className="font-mono">custom</code>).
                        </Field>
                        <Field name="url" type="string">
                          Full page URL. The domain and path are derived from it.
                        </Field>
                        <Field name="referrer_url" type="string">
                          The referring URL, if any.
                        </Field>
                        <Field name="visitor_hash" type="string">
                          Stable per-user identifier, 16–64 hex chars. If missing or invalid, the server assigns one.
                        </Field>
                        <Field name="props" type="string">
                          JSON-encoded string of custom properties.
                        </Field>
                        <Field name="utm_source" type="string">
                          Also <code className="font-mono">utm_medium</code>, <code className="font-mono">utm_campaign</code>.
                        </Field>
                      </tbody>
                    </table>
                  </div>
                </div>
                <div className="rounded-md border border-primary/20 bg-primary/5 p-3 text-sm text-muted-foreground">
                  The endpoint is rate-limited to 100 requests/minute per IP. Batch multiple events into one request by
                  sending several JSON lines separated by <code className="font-mono">\n</code>.
                </div>
              </CardContent>
            </Card>
          </section>

          {/* Reading data */}
          <section>
            <SectionHeading
              id="read-api"
              icon={KeyRound}
              title="Reading your data"
              desc="Query stats programmatically with an API token."
            />
            <Card>
              <CardContent className="space-y-4 pt-6">
                <p className="text-sm text-muted-foreground">
                  Create a token in <strong>Settings → API Keys</strong>, then send it as a Bearer token. Tokens carry
                  the role of the user who created them.
                </p>
                <CodeBlock
                  label="bash"
                  code={`curl '${HOST}/api/stats/overview?domain=example.com&start=2026-01-01T00:00:00Z&end=2026-02-01T00:00:00Z' \\
  -H 'Authorization: Bearer etq_your_token_here'`}
                />
                <p className="text-sm text-muted-foreground">
                  Most read endpoints live under <code className="rounded bg-muted px-1 font-mono">/api/stats/*</code> and
                  accept a <code className="font-mono">domain</code> (hostname) plus <code className="font-mono">start</code>{' '}
                  and <code className="font-mono">end</code> as RFC 3339 timestamps.
                </p>
              </CardContent>
            </Card>
          </section>
        </div>
      </div>
    </div>
  )
}
