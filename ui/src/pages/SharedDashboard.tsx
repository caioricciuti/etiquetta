import { useParams } from 'react-router-dom'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card'
import { Skeleton } from '../components/ui/skeleton'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { BarChart3, Users, Eye, Clock, ArrowUpRight, Globe } from 'lucide-react'

const BASE = ''

async function sharedFetch<T>(token: string, stat: string, params?: URLSearchParams): Promise<T> {
  const qs = params ? `?${params}` : ''
  const res = await fetch(`${BASE}/api/shared/stats/${token}/${stat}${qs}`)
  if (!res.ok) throw new Error('Failed to fetch shared data')
  return res.json() as Promise<T>
}

interface OverviewData {
  total_events: number
  unique_visitors: number
  sessions: number
  pageviews: number
  bounce_rate: number
  avg_session_seconds: number
}

interface PageData {
  path: string
  views: number
  visitors: number
}

interface ReferrerData {
  source: string
  visits: number
}

interface GeoData {
  country: string
  visits: number
}

function useSharedParams() {
  const { dateRange } = useDateRangeStore()
  const params = new URLSearchParams()
  if (dateRange?.from) params.set('start', String(dateRange.from.getTime()))
  if (dateRange?.to) params.set('end', String(dateRange.to.getTime()))
  return params
}

function StatCard({ label, value, icon: Icon }: { label: string; value: string | number; icon: React.ElementType }) {
  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-muted-foreground">{label}</p>
            <p className="text-2xl font-bold">{typeof value === 'number' ? value.toLocaleString() : value}</p>
          </div>
          <Icon className="h-8 w-8 text-muted-foreground/30" />
        </div>
      </CardContent>
    </Card>
  )
}

export function SharedDashboard() {
  const { token } = useParams<{ token: string }>()
  const params = useSharedParams()
  const { dateRange, setDateRange, selectedPreset, setPreset } = useDateRangeStore()

  const { data: config, isLoading: configLoading, error: configError } = useQuery({
    queryKey: ['shared-config', token],
    queryFn: async () => {
      const res = await fetch(`${BASE}/api/shared/dashboard/${token}`)
      if (!res.ok) throw new Error('Invalid or expired share link')
      return res.json() as Promise<{ domain: string }>
    },
    enabled: !!token,
  })

  const { data: overview } = useQuery({
    queryKey: ['shared-overview', token, params.toString()],
    queryFn: () => sharedFetch<OverviewData>(token!, 'overview', params),
    enabled: !!token && !!config,
    placeholderData: keepPreviousData,
  })

  const { data: pages } = useQuery({
    queryKey: ['shared-pages', token, params.toString()],
    queryFn: () => sharedFetch<PageData[]>(token!, 'pages', params),
    enabled: !!token && !!config,
    placeholderData: keepPreviousData,
  })

  const { data: referrers } = useQuery({
    queryKey: ['shared-referrers', token, params.toString()],
    queryFn: () => sharedFetch<ReferrerData[]>(token!, 'referrers', params),
    enabled: !!token && !!config,
    placeholderData: keepPreviousData,
  })

  const { data: geo } = useQuery({
    queryKey: ['shared-geo', token, params.toString()],
    queryFn: () => sharedFetch<GeoData[]>(token!, 'geo', params),
    enabled: !!token && !!config,
    placeholderData: keepPreviousData,
  })

  if (configLoading) {
    return (
      <div className="min-h-screen bg-background p-6 space-y-6 max-w-6xl mx-auto">
        <Skeleton className="h-10 w-64" />
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[1, 2, 3, 4].map(i => <Skeleton key={i} className="h-24" />)}
        </div>
      </div>
    )
  }

  if (configError || !config) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <Card className="max-w-md">
          <CardContent className="py-12 text-center">
            <h2 className="text-xl font-bold mb-2">Link not found</h2>
            <p className="text-muted-foreground">This shared dashboard link is invalid or has expired.</p>
          </CardContent>
        </Card>
      </div>
    )
  }

  const avgDuration = overview ? `${Math.floor(overview.avg_session_seconds / 60)}m ${Math.round(overview.avg_session_seconds % 60)}s` : '--'

  return (
    <div className="min-h-screen bg-background p-6 space-y-6 max-w-6xl mx-auto">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{config.domain}</h1>
          <p className="text-sm text-muted-foreground">Public analytics dashboard</p>
        </div>
        <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} selectedPreset={selectedPreset} onPresetChange={setPreset} />
      </div>

      {/* Stats grid */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard label="Visitors" value={overview?.unique_visitors ?? 0} icon={Users} />
        <StatCard label="Pageviews" value={overview?.pageviews ?? 0} icon={Eye} />
        <StatCard label="Bounce Rate" value={overview ? `${overview.bounce_rate.toFixed(1)}%` : '--'} icon={ArrowUpRight} />
        <StatCard label="Avg Duration" value={avgDuration} icon={Clock} />
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        {/* Top Pages */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <BarChart3 className="h-4 w-4" /> Top Pages
            </CardTitle>
          </CardHeader>
          <CardContent>
            {pages?.length ? (
              <div className="space-y-2">
                {pages.slice(0, 10).map((p, i) => (
                  <div key={i} className="flex items-center justify-between text-sm">
                    <span className="truncate flex-1 mr-4 font-mono text-xs">{p.path}</span>
                    <span className="text-muted-foreground whitespace-nowrap">{p.views.toLocaleString()}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground text-center py-4">No data</p>
            )}
          </CardContent>
        </Card>

        {/* Top Referrers */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ArrowUpRight className="h-4 w-4" /> Top Referrers
            </CardTitle>
          </CardHeader>
          <CardContent>
            {referrers?.length ? (
              <div className="space-y-2">
                {referrers.slice(0, 10).map((r, i) => (
                  <div key={i} className="flex items-center justify-between text-sm">
                    <span className="truncate flex-1 mr-4">{r.source || 'Direct'}</span>
                    <span className="text-muted-foreground whitespace-nowrap">{r.visits.toLocaleString()}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground text-center py-4">No data</p>
            )}
          </CardContent>
        </Card>

        {/* Countries */}
        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Globe className="h-4 w-4" /> Countries
            </CardTitle>
          </CardHeader>
          <CardContent>
            {geo?.length ? (
              <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                {geo.slice(0, 12).map((g, i) => (
                  <div key={i} className="flex items-center justify-between text-sm p-2 bg-muted/50 rounded">
                    <span>{g.country || 'Unknown'}</span>
                    <span className="text-muted-foreground">{g.visits.toLocaleString()}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground text-center py-4">No data</p>
            )}
          </CardContent>
        </Card>
      </div>

      <p className="text-center text-xs text-muted-foreground pt-4">
        Powered by Etiquetta
      </p>
    </div>
  )
}
