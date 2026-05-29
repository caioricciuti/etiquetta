import { useDomainStore } from '@/stores/useDomainStore'
import { useFilterStore } from '@/stores/useFilterStore'
import { useDomains } from '@/hooks/useDomains'
import {
  useLiveStats,
  type LiveRecentEvent,
  type LiveActivePage,
  type LiveSource,
  type LiveCountry,
} from '@/hooks/useLiveQueries'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { cn, formatNumber } from '@/lib/utils'
import {
  Radio,
  Loader2,
  FileText,
  Globe2,
  Link2,
  Users,
  Eye,
  MonitorSmartphone,
} from 'lucide-react'
import { Area, AreaChart, ResponsiveContainer, XAxis, Tooltip } from 'recharts'

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

function LiveDot() {
  return (
    <span className="relative flex h-3 w-3 shrink-0">
      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-75" />
      <span className="relative inline-flex h-3 w-3 rounded-full bg-green-500" />
    </span>
  )
}

function HeroStats({
  currentVisitors,
  activeSessions,
  pageviews5m,
}: {
  currentVisitors: number
  activeSessions: number
  pageviews5m: number
}) {
  return (
    <Card className="border-green-500/30 bg-green-500/5">
      <CardContent className="flex flex-col gap-6 p-6 md:flex-row md:items-center md:justify-between">
        <div className="flex items-center gap-4">
          <LiveDot />
          <div>
            <p className="text-5xl font-bold leading-none tabular-nums text-foreground">
              {formatNumber(currentVisitors)}
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              {currentVisitors === 1 ? 'visitor' : 'visitors'} in the last 5 minutes
            </p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-6">
          <div className="flex flex-col">
            <span className="flex items-center gap-1.5 text-xs uppercase tracking-wider text-muted-foreground">
              <Users className="h-3.5 w-3.5" />
              Active sessions
            </span>
            <span className="mt-1 text-2xl font-semibold tabular-nums">
              {formatNumber(activeSessions)}
            </span>
          </div>
          <div className="flex flex-col">
            <span className="flex items-center gap-1.5 text-xs uppercase tracking-wider text-muted-foreground">
              <Eye className="h-3.5 w-3.5" />
              Pageviews (5m)
            </span>
            <span className="mt-1 text-2xl font-semibold tabular-nums">
              {formatNumber(pageviews5m)}
            </span>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function ListCard({
  title,
  icon: Icon,
  rows,
}: {
  title: string
  icon: typeof FileText
  rows: { key: string; label: string; primary: number; secondary?: number; title?: string }[]
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Icon className="h-4 w-4 text-muted-foreground" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No activity in the last 5 minutes
          </p>
        ) : (
          <div className="space-y-1">
            {rows.map((row) => (
              <div
                key={row.key}
                className="flex items-center justify-between gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-muted/40"
              >
                <span className="min-w-0 truncate text-muted-foreground" title={row.title ?? row.label}>
                  {row.label}
                </span>
                <span className="flex shrink-0 items-center gap-2 tabular-nums">
                  <span className="font-medium text-foreground">{formatNumber(row.primary)}</span>
                  {typeof row.secondary === 'number' ? (
                    <span className="text-xs text-muted-foreground">
                      {formatNumber(row.secondary)} views
                    </span>
                  ) : null}
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function FeedRow({ event }: { event: LiveRecentEvent }) {
  const location = [event.geo_city, event.geo_country].filter(Boolean).join(', ')
  const device = [event.device_type, event.browser_name].filter(Boolean).join(' · ')
  return (
    <div className="flex items-start gap-3 border-b py-2.5 text-sm last:border-0">
      <span className="w-16 shrink-0 whitespace-nowrap pt-0.5 text-xs text-muted-foreground">
        {relativeTime(event.timestamp)}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="font-medium">
            {event.event_type || 'event'}
            {event.event_name ? (
              <span className="text-muted-foreground"> · {event.event_name}</span>
            ) : null}
          </span>
          <span className="min-w-0 truncate text-muted-foreground" title={event.path}>
            {event.path || '—'}
          </span>
        </div>
        <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          {location ? (
            <span className="flex items-center gap-1">
              <Globe2 className="h-3 w-3" />
              {location}
            </span>
          ) : null}
          {device ? (
            <span className="flex items-center gap-1">
              <MonitorSmartphone className="h-3 w-3" />
              {device}
            </span>
          ) : null}
          {event.referrer_type ? (
            <span className="flex items-center gap-1">
              <Link2 className="h-3 w-3" />
              {event.referrer_type}
            </span>
          ) : null}
        </div>
      </div>
    </div>
  )
}

export function Live() {
  const selectedDomainId = useDomainStore((s) => s.selectedDomainId)
  const { data: domains } = useDomains()
  const domain = domains?.find((d) => d.id === selectedDomainId)?.domain
  const botFilter = useFilterStore((s) => s.filters.bot_filter)

  const { data, isLoading, isError, error, isFetching } = useLiveStats(domain, botFilter)

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-6xl space-y-6 p-6">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <Radio className="mt-1 h-6 w-6 text-muted-foreground" />
            <div>
              <h1 className="text-2xl font-bold text-foreground">Live</h1>
              <p className="text-muted-foreground">
                See who is on your site right now. Updates every few seconds.
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
                Select a property from the sidebar to see live activity.
              </p>
            </CardContent>
          </Card>
        ) : isLoading ? (
          <Card>
            <CardContent className="flex items-center justify-center gap-2 py-8 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading live activity…
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
            <HeroStats
              currentVisitors={data.current_visitors}
              activeSessions={data.active_sessions}
              pageviews5m={data.pageviews_5m}
            />

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Last 30 minutes</CardTitle>
                <CardDescription>Pageviews per minute</CardDescription>
              </CardHeader>
              <CardContent>
                {!data.sparkline || data.sparkline.length === 0 ? (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    No activity in the last 30 minutes
                  </p>
                ) : (
                  <div className="h-32">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={data.sparkline}>
                        <defs>
                          <linearGradient id="liveViews" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--chart-1)" stopOpacity={0.3} />
                            <stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0.05} />
                          </linearGradient>
                        </defs>
                        <XAxis dataKey="minute" hide />
                        <Tooltip
                          contentStyle={{
                            background: 'var(--popover)',
                            color: 'var(--popover-foreground)',
                            border: '1px solid var(--border)',
                            borderRadius: '8px',
                            fontSize: '12px',
                          }}
                          labelFormatter={(label) => `Minute: ${label}`}
                        />
                        <Area
                          type="monotone"
                          dataKey="views"
                          name="Views"
                          stroke="var(--chart-1)"
                          fill="url(#liveViews)"
                          strokeWidth={2}
                        />
                        <Area
                          type="monotone"
                          dataKey="visitors"
                          name="Visitors"
                          stroke="var(--chart-2)"
                          fill="transparent"
                          strokeWidth={2}
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                )}
              </CardContent>
            </Card>

            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <ListCard
                title="Active pages"
                icon={FileText}
                rows={(data.active_pages ?? []).map((p: LiveActivePage, i) => ({
                  key: `${p.path}-${i}`,
                  label: p.path || '—',
                  title: p.path,
                  primary: p.visitors,
                  secondary: p.views,
                }))}
              />
              <ListCard
                title="Live sources"
                icon={Link2}
                rows={(data.live_sources ?? []).map((s: LiveSource, i) => ({
                  key: `${s.source}-${i}`,
                  label: s.source || 'Direct',
                  primary: s.visitors,
                }))}
              />
              <ListCard
                title="Live countries"
                icon={Globe2}
                rows={(data.live_countries ?? []).map((c: LiveCountry, i) => ({
                  key: `${c.country}-${i}`,
                  label: c.country || 'Unknown',
                  primary: c.visitors,
                }))}
              />
            </div>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Live feed</CardTitle>
                <CardDescription>
                  The {Math.min(data.recent?.length ?? 0, 30)} most recent events, newest first.
                </CardDescription>
              </CardHeader>
              <CardContent>
                {!data.recent || data.recent.length === 0 ? (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    No activity in the last 5 minutes
                  </p>
                ) : (
                  <div className={cn('flex flex-col')}>
                    {data.recent.map((ev, i) => (
                      <FeedRow key={`${ev.timestamp}-${i}`} event={ev} />
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </>
        ) : null}
      </div>
    </div>
  )
}
