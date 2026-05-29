import { useDomainStore } from '@/stores/useDomainStore'
import { useDomains } from '@/hooks/useDomains'
import { useInstallStatus, type InstallRecentEvent } from '@/hooks/useInstallQueries'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  CheckCircle2,
  Loader2,
  AlertTriangle,
  ShieldCheck,
  MousePointerClick,
  MoveVertical,
  Bot,
  UserRound,
  PlugZap,
} from 'lucide-react'

// Format an epoch-ms timestamp as a short relative time ("12s ago", "5m ago").
function relativeTime(ms: number | null | undefined): string {
  if (!ms) return '—'
  const diff = Date.now() - ms
  if (diff < 0) return 'just now'
  const sec = Math.floor(diff / 1000)
  if (sec < 5) return 'just now'
  if (sec < 60) return `${sec}s ago`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h ago`
  const days = Math.floor(hr / 24)
  return `${days}d ago`
}

function BotBadge({ event }: { event: InstallRecentEvent }) {
  const score = typeof event.bot_score === 'number' ? Math.round(event.bot_score) : null
  if (event.is_bot) {
    return (
      <Badge variant="destructive" className="gap-1">
        <Bot className="h-3 w-3" />
        {event.bot_category || 'Bot'}
        {score !== null ? ` · ${score}` : ''}
      </Badge>
    )
  }
  return (
    <Badge
      variant="outline"
      className="gap-1 border-green-600/40 text-green-700 dark:text-green-400"
    >
      <UserRound className="h-3 w-3" />
      Human
      {score !== null ? ` · ${score}` : ''}
    </Badge>
  )
}

function StatusBanner({
  domain,
  installed,
  receiving,
  eventsLast30m,
  totalEvents,
  lastEventAt,
}: {
  domain: string
  installed: boolean
  receiving: boolean
  eventsLast30m: number
  totalEvents: number
  lastEventAt: number | null
}) {
  // waiting: never seen an event
  if (!installed) {
    return (
      <Card className="border-blue-500/40 bg-blue-500/5">
        <CardContent className="flex items-start gap-4 p-6">
          <Loader2 className="mt-0.5 h-6 w-6 shrink-0 animate-spin text-blue-600 dark:text-blue-400" />
          <div>
            <p className="text-lg font-semibold">Waiting for first event from {domain}…</p>
            <p className="text-sm text-muted-foreground">
              Open your site in a new tab to generate a pageview. This page updates
              automatically.
            </p>
          </div>
        </CardContent>
      </Card>
    )
  }

  // receiving: events in the last 30 min
  if (receiving) {
    return (
      <Card className="border-green-500/40 bg-green-500/5">
        <CardContent className="flex items-start gap-4 p-6">
          <CheckCircle2 className="mt-0.5 h-6 w-6 shrink-0 text-green-600 dark:text-green-400" />
          <div>
            <p className="text-lg font-semibold text-green-700 dark:text-green-400">
              Receiving data — {eventsLast30m.toLocaleString()}{' '}
              {eventsLast30m === 1 ? 'event' : 'events'} in the last 30 min
            </p>
            <p className="text-sm text-muted-foreground">
              Last event {relativeTime(lastEventAt)} · {totalEvents.toLocaleString()} total
              events recorded for {domain}.
            </p>
          </div>
        </CardContent>
      </Card>
    )
  }

  // installed but quiet
  return (
    <Card className="border-amber-500/40 bg-amber-500/5">
      <CardContent className="flex items-start gap-4 p-6">
        <AlertTriangle className="mt-0.5 h-6 w-6 shrink-0 text-amber-600 dark:text-amber-400" />
        <div>
          <p className="text-lg font-semibold text-amber-700 dark:text-amber-500">
            Installed ({totalEvents.toLocaleString()} total events) but nothing in the last 30
            min
          </p>
          <p className="text-sm text-muted-foreground">
            The snippet has worked before. Last event {relativeTime(lastEventAt)}. Open your site
            to confirm it is still sending data.
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

export function InstallCheck() {
  const selectedDomainId = useDomainStore((s) => s.selectedDomainId)
  const { data: domains } = useDomains()
  const domain = domains?.find((d) => d.id === selectedDomainId)?.domain

  const { data, isLoading, isError, error, isFetching } = useInstallStatus(domain)

  return (
    <div className="h-full overflow-y-auto">
      <div className="space-y-6 p-6 max-w-6xl mx-auto">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <PlugZap className="mt-1 h-6 w-6 text-muted-foreground" />
            <div>
              <h1 className="text-2xl font-bold text-foreground">Install Check</h1>
              <p className="text-muted-foreground">
                Confirm your tracking snippet is working and inspect what Etiquetta collects.
              </p>
            </div>
          </div>
          {isFetching && domain ? (
            <span className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="h-3 w-3 animate-spin" />
              Live
            </span>
          ) : null}
        </div>

        {!domain ? (
          <Card>
            <CardContent className="py-8">
              <p className="text-center text-muted-foreground">
                Select a property from the sidebar to verify its installation.
              </p>
            </CardContent>
          </Card>
        ) : isLoading ? (
          <Card>
            <CardContent className="py-8 flex items-center justify-center gap-2 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Checking installation…
            </CardContent>
          </Card>
        ) : isError ? (
          <Card className="border-destructive/40">
            <CardContent className="py-8">
              <p className="text-center text-destructive">
                Error: {error instanceof Error ? error.message : 'Request failed'}
              </p>
            </CardContent>
          </Card>
        ) : data ? (
          <>
            <StatusBanner
              domain={data.domain || domain}
              installed={data.installed}
              receiving={data.receiving}
              eventsLast30m={data.events_last_30m}
              totalEvents={data.total_events}
              lastEventAt={data.last_event_at}
            />

            <Card>
              <CardHeader>
                <CardTitle>Live inspector</CardTitle>
                <CardDescription>
                  The {Math.min(data.recent?.length ?? 0, 25)} most recent events, newest first.
                  Updates every few seconds.
                </CardDescription>
              </CardHeader>
              <CardContent>
                {!data.recent || data.recent.length === 0 ? (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    No events captured yet.
                  </p>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                          <th className="px-3 py-2 font-medium">Time</th>
                          <th className="px-3 py-2 font-medium">Event</th>
                          <th className="px-3 py-2 font-medium">Path</th>
                          <th className="px-3 py-2 font-medium">Location</th>
                          <th className="px-3 py-2 font-medium">Device</th>
                          <th className="px-3 py-2 font-medium">Referrer</th>
                          <th className="px-3 py-2 font-medium">Engagement</th>
                          <th className="px-3 py-2 font-medium">Bot</th>
                        </tr>
                      </thead>
                      <tbody>
                        {data.recent.map((ev, i) => {
                          const location = [ev.geo_city, ev.geo_country]
                            .filter(Boolean)
                            .join(', ')
                          const device = [ev.device_type, ev.browser_name, ev.os_name]
                            .filter(Boolean)
                            .join(' · ')
                          return (
                            <tr
                              key={`${ev.timestamp}-${i}`}
                              className="border-b last:border-0 hover:bg-muted/40"
                            >
                              <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">
                                {relativeTime(ev.timestamp)}
                              </td>
                              <td className="px-3 py-2">
                                <div className="flex flex-col">
                                  <span className="font-medium">{ev.event_type || '—'}</span>
                                  {ev.event_name ? (
                                    <span className="text-xs text-muted-foreground">
                                      {ev.event_name}
                                    </span>
                                  ) : null}
                                </div>
                              </td>
                              <td
                                className="max-w-[16rem] truncate px-3 py-2"
                                title={ev.path}
                              >
                                {ev.path || '—'}
                              </td>
                              <td className="whitespace-nowrap px-3 py-2">{location || '—'}</td>
                              <td className="px-3 py-2 text-muted-foreground">{device || '—'}</td>
                              <td className="px-3 py-2">{ev.referrer_type || '—'}</td>
                              <td className="px-3 py-2">
                                <div className="flex items-center gap-1.5">
                                  {ev.has_click ? (
                                    <Badge
                                      variant="secondary"
                                      className="gap-1"
                                      title="Click recorded"
                                    >
                                      <MousePointerClick className="h-3 w-3" />
                                      Click
                                    </Badge>
                                  ) : null}
                                  {ev.has_scroll ? (
                                    <Badge
                                      variant="secondary"
                                      className="gap-1"
                                      title="Scroll recorded"
                                    >
                                      <MoveVertical className="h-3 w-3" />
                                      Scroll
                                    </Badge>
                                  ) : null}
                                  {!ev.has_click && !ev.has_scroll ? (
                                    <span className="text-muted-foreground">—</span>
                                  ) : null}
                                </div>
                              </td>
                              <td className="px-3 py-2">
                                <BotBadge event={ev} />
                              </td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </CardContent>
            </Card>

            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              <ShieldCheck className="h-3.5 w-3.5" />
              Etiquetta is cookieless by default; IPs are hashed and not stored raw.
            </p>
          </>
        ) : null}
      </div>
    </div>
  )
}
