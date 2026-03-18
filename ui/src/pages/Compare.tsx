import { useState, useMemo } from 'react'
import { subDays, subMonths, subYears, differenceInDays } from 'date-fns'
import type { DateRange } from 'react-day-picker'
import { useComparison } from '../hooks/useAnalyticsQueries'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { useDomainStore } from '../stores/useDomainStore'
import { useDomains } from '../hooks/useDomains'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { calcTrend } from '../components/dashboard/StatCard'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Skeleton } from '../components/ui/skeleton'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from '../components/ui/chart'
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
  Users,
  Eye,
  MousePointer,
  Timer,
  BarChart3,
  Activity,
  TrendingUp,
  TrendingDown,
  Minus,
  GitCompareArrows,
  ArrowUpRight,
  ArrowDownRight,
  type LucideIcon,
} from 'lucide-react'
import type {
  ComparisonResponse,
  ComparisonInsight,
  TopPage,
  Referrer,
  GeoData,
  DeviceData,
  BrowserData,
  Campaign,
  CustomEvent,
  OutboundLink,
} from '../lib/types'

function prevPeriodRange(range: DateRange | undefined): DateRange | undefined {
  if (!range?.from || !range?.to) return undefined
  const days = differenceInDays(range.to, range.from)
  return {
    from: subDays(range.from, days + 1),
    to: subDays(range.from, 1),
  }
}

export function Compare() {
  const { dateRange } = useDateRangeStore()
  const { selectedDomainId } = useDomainStore()
  const { data: domains } = useDomains()
  const selectedDomain = domains?.find((d) => d.id === selectedDomainId)

  const dateRangeKey = `${dateRange?.from?.getTime()}-${dateRange?.to?.getTime()}`
  const [currentRange, setCurrentRange] = useState<DateRange | undefined>(dateRange)
  const [compareRange, setCompareRange] = useState<DateRange | undefined>(() => prevPeriodRange(dateRange))
  const [activePreset, setActivePreset] = useState<string>('previous')
  const [lastSyncKey, setLastSyncKey] = useState(dateRangeKey)

  if (dateRangeKey !== lastSyncKey) {
    setLastSyncKey(dateRangeKey)
    setCurrentRange(dateRange)
    setCompareRange(prevPeriodRange(dateRange))
    setActivePreset('previous')
  }

  const handlePreset = (preset: string) => {
    if (!currentRange?.from || !currentRange?.to) return
    setActivePreset(preset)
    switch (preset) {
      case 'previous':
        setCompareRange(prevPeriodRange(currentRange))
        break
      case 'last_month':
        setCompareRange({
          from: subMonths(currentRange.from, 1),
          to: subMonths(currentRange.to, 1),
        })
        break
      case 'last_year':
        setCompareRange({
          from: subYears(currentRange.from, 1),
          to: subYears(currentRange.to, 1),
        })
        break
    }
  }

  const { data, isLoading } = useComparison(currentRange, compareRange, selectedDomain?.domain)

  return (
    <div className="p-4 md:p-6 space-y-6 overflow-y-auto h-full">
      <div className="flex flex-col gap-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <GitCompareArrows className="h-7 w-7" />
            Period Comparison
          </h1>
          <p className="text-muted-foreground mt-1">Compare metrics between two time periods</p>
        </div>
        <CompareHeader
          currentRange={currentRange}
          compareRange={compareRange}
          activePreset={activePreset}
          onCurrentChange={(r) => {
            setCurrentRange(r)
            if (activePreset === 'previous') setCompareRange(prevPeriodRange(r))
          }}
          onCompareChange={(r) => { setCompareRange(r); setActivePreset('') }}
          onPreset={handlePreset}
        />
      </div>

      {isLoading ? <CompareLoadingSkeleton /> : data ? (
        <>
          <InsightsBar insights={data.insights} />
          <CompareStatsGrid data={data} />
          <CompareTimeseries data={data} />
          <ComparisonTabs data={data} />
        </>
      ) : null}
    </div>
  )
}

function CompareHeader({
  currentRange,
  compareRange,
  activePreset,
  onCurrentChange,
  onCompareChange,
  onPreset,
}: {
  currentRange: DateRange | undefined
  compareRange: DateRange | undefined
  activePreset: string
  onCurrentChange: (r: DateRange | undefined) => void
  onCompareChange: (r: DateRange | undefined) => void
  onPreset: (preset: string) => void
}) {
  return (
    <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 flex-wrap">
      <DateRangePicker dateRange={currentRange} onDateRangeChange={onCurrentChange} />
      <span className="text-sm font-medium text-muted-foreground">vs</span>
      <DateRangePicker dateRange={compareRange} onDateRangeChange={onCompareChange} />
      <div className="flex gap-1.5">
        {([
          ['previous', 'Previous Period'],
          ['last_month', 'Last Month'],
          ['last_year', 'Last Year'],
        ] as const).map(([key, label]) => (
          <Button
            key={key}
            variant={activePreset === key ? 'default' : 'outline'}
            size="sm"
            onClick={() => onPreset(key)}
          >
            {label}
          </Button>
        ))}
      </div>
    </div>
  )
}

/* ---------- Insights ---------- */

function InsightsBar({ insights }: { insights: ComparisonInsight[] }) {
  if (!insights?.length) return null

  const styles: Record<string, {
    bg: string
    icon: React.ReactNode
    text: string
  }> = {
    positive: {
      bg: 'bg-emerald-500/10 border-emerald-500/20',
      icon: <TrendingUp className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />,
      text: 'text-emerald-600 dark:text-emerald-400',
    },
    negative: {
      bg: 'bg-red-500/10 border-red-500/20',
      icon: <TrendingDown className="h-3.5 w-3.5 text-red-600 dark:text-red-400 shrink-0" />,
      text: 'text-red-600 dark:text-red-400',
    },
    neutral: {
      bg: 'bg-blue-500/10 border-blue-500/20',
      icon: <Minus className="h-3.5 w-3.5 text-blue-600 dark:text-blue-400 shrink-0" />,
      text: 'text-blue-600 dark:text-blue-400',
    },
  }

  return (
    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
      {insights.map((insight, i) => {
        const s = styles[insight.type] ?? styles.neutral
        return (
          <div key={i} className={`rounded-lg p-3 border ${s.bg}`}>
            <div className="flex items-center gap-2 mb-1">
              {s.icon}
              <span className={`text-[10px] font-semibold uppercase tracking-wider ${s.text}`}>
                {insight.metric}
              </span>
            </div>
            <p className="text-sm text-foreground leading-snug">{insight.text}</p>
          </div>
        )
      })}
    </div>
  )
}

/* ---------- KPI Cards ---------- */

function CompareKPICard({
  title,
  current,
  previous,
  icon: Icon,
  fmt,
  invert,
}: {
  title: string
  current: number
  previous: number
  icon: LucideIcon
  fmt?: (v: number) => string
  invert?: boolean
}) {
  const { trend, trendUp } = calcTrend(current, previous)
  const adjustedTrendUp = invert ? !trendUp : trendUp
  const display = fmt ? fmt(current) : current.toLocaleString()
  const prevDisplay = fmt ? fmt(previous) : previous.toLocaleString()
  const maxVal = Math.max(current, previous, 1)
  const curPct = (current / maxVal) * 100
  const prevPct = (previous / maxVal) * 100

  return (
    <Card className="relative overflow-hidden transition-all hover:shadow-md hover:border-primary/20">
      <CardContent className="p-4">
        <div className="flex items-start justify-between">
          <div className="space-y-1 min-w-0 flex-1">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{title}</p>
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold tracking-tight truncate">{display}</p>
              {trend && (
                <span className={`flex items-center text-xs font-medium ${adjustedTrendUp ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                  {adjustedTrendUp
                    ? <ArrowUpRight className="h-3 w-3" />
                    : <ArrowDownRight className="h-3 w-3" />
                  }
                  {trend}
                </span>
              )}
            </div>
          </div>
          <div className="h-10 w-10 rounded-xl bg-primary/10 flex items-center justify-center shrink-0">
            <Icon className="h-5 w-5 text-primary" />
          </div>
        </div>
        <div className="mt-3 space-y-1.5">
          <div className="flex items-center gap-2">
            <div className="h-1.5 flex-1 bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-[var(--chart-1)] rounded-full transition-all duration-500"
                style={{ width: `${curPct}%` }}
              />
            </div>
            <span className="text-[11px] font-mono tabular-nums text-muted-foreground w-14 text-right">{display}</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="h-1.5 flex-1 bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-muted-foreground/30 rounded-full transition-all duration-500"
                style={{ width: `${prevPct}%` }}
              />
            </div>
            <span className="text-[11px] font-mono tabular-nums text-muted-foreground w-14 text-right">{prevDisplay}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function CompareStatsGrid({ data }: { data: ComparisonResponse }) {
  const cur = data.overview.current
  const prev = data.overview.previous

  const cards = [
    { title: 'Visitors', cur: cur.unique_visitors, prev: prev.unique_visitors, icon: Users },
    { title: 'Pageviews', cur: cur.pageviews, prev: prev.pageviews, icon: Eye },
    { title: 'Sessions', cur: cur.sessions, prev: prev.sessions, icon: Activity },
    { title: 'Total Events', cur: cur.total_events, prev: prev.total_events, icon: MousePointer },
    { title: 'Bounce Rate', cur: cur.bounce_rate, prev: prev.bounce_rate, icon: BarChart3, fmt: (v: number) => `${v.toFixed(1)}%`, invert: true },
    { title: 'Avg Duration', cur: cur.avg_session_seconds, prev: prev.avg_session_seconds, icon: Timer, fmt: (v: number) => `${v.toFixed(0)}s` },
  ]

  return (
    <div className="grid gap-4 grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
      {cards.map((c) => (
        <CompareKPICard
          key={c.title}
          title={c.title}
          current={c.cur}
          previous={c.prev}
          icon={c.icon}
          fmt={c.fmt}
          invert={c.invert}
        />
      ))}
    </div>
  )
}

/* ---------- Timeseries Chart ---------- */

const chartConfig = {
  current_pageviews: { label: 'Pageviews', color: 'var(--chart-1)' },
  previous_pageviews: { label: 'Pageviews (prev)', color: 'var(--chart-3)' },
  current_visitors: { label: 'Visitors', color: 'var(--chart-2)' },
  previous_visitors: { label: 'Visitors (prev)', color: 'var(--chart-4)' },
} satisfies ChartConfig

function CompareTimeseries({ data }: { data: ComparisonResponse }) {
  const { current: curSeries, previous: prevSeries } = data.timeseries
  const merged = useMemo(() => {
    const maxLen = Math.max(curSeries?.length ?? 0, prevSeries?.length ?? 0)
    const result = []
    for (let i = 0; i < maxLen; i++) {
      const cur = curSeries?.[i]
      const prev = prevSeries?.[i]
      result.push({
        label: `Day ${i + 1}`,
        current_pageviews: cur?.pageviews ?? 0,
        current_visitors: cur?.visitors ?? 0,
        previous_pageviews: prev?.pageviews ?? 0,
        previous_visitors: prev?.visitors ?? 0,
        current_date: cur?.date ?? '',
        previous_date: prev?.date ?? '',
      })
    }
    return result
  }, [curSeries, prevSeries])

  if (!merged.length) return null

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-lg font-semibold">Traffic Over Time</CardTitle>
        <CardDescription>Pageviews and visitors compared across periods</CardDescription>
      </CardHeader>
      <CardContent>
        <ChartContainer config={chartConfig} className="h-[300px] w-full">
          <AreaChart data={merged} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="fillCurPV" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-current_pageviews)" stopOpacity={0.3} />
                <stop offset="95%" stopColor="var(--color-current_pageviews)" stopOpacity={0.05} />
              </linearGradient>
              <linearGradient id="fillPrevPV" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-previous_pageviews)" stopOpacity={0.15} />
                <stop offset="95%" stopColor="var(--color-previous_pageviews)" stopOpacity={0.02} />
              </linearGradient>
              <linearGradient id="fillCurV" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-current_visitors)" stopOpacity={0.3} />
                <stop offset="95%" stopColor="var(--color-current_visitors)" stopOpacity={0.05} />
              </linearGradient>
              <linearGradient id="fillPrevV" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-previous_visitors)" stopOpacity={0.15} />
                <stop offset="95%" stopColor="var(--color-previous_visitors)" stopOpacity={0.02} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
            <XAxis
              dataKey="label"
              tick={{ fontSize: 12 }}
              axisLine={false}
              tickLine={false}
              className="text-muted-foreground"
            />
            <YAxis
              tick={{ fontSize: 12 }}
              axisLine={false}
              tickLine={false}
              className="text-muted-foreground"
              width={40}
            />
            <ChartTooltip
              content={
                <ChartTooltipContent
                  indicator="dot"
                  labelFormatter={(_, payload) => {
                    const p = payload?.[0]?.payload as Record<string, string> | undefined
                    if (!p) return ''
                    const parts = []
                    if (p.current_date) parts.push(`Current: ${p.current_date}`)
                    if (p.previous_date) parts.push(`Previous: ${p.previous_date}`)
                    return parts.join(' | ')
                  }}
                />
              }
            />
            <ChartLegend content={<ChartLegendContent />} />
            <Area
              type="monotone"
              dataKey="current_pageviews"
              stroke="var(--color-current_pageviews)"
              strokeWidth={2}
              fill="url(#fillCurPV)"
            />
            <Area
              type="monotone"
              dataKey="previous_pageviews"
              stroke="var(--color-previous_pageviews)"
              strokeWidth={2}
              fill="url(#fillPrevPV)"
              strokeDasharray="5 5"
            />
            <Area
              type="monotone"
              dataKey="current_visitors"
              stroke="var(--color-current_visitors)"
              strokeWidth={2}
              fill="url(#fillCurV)"
            />
            <Area
              type="monotone"
              dataKey="previous_visitors"
              stroke="var(--color-previous_visitors)"
              strokeWidth={2}
              fill="url(#fillPrevV)"
              strokeDasharray="5 5"
            />
          </AreaChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
}

/* ---------- Dimension Tabs ---------- */

const TAB_CONFIG = [
  { value: 'pages', title: 'Top Pages', description: 'Page performance comparison between periods' },
  { value: 'referrers', title: 'Top Referrers', description: 'Traffic sources compared across periods' },
  { value: 'countries', title: 'Top Countries', description: 'Geographic distribution comparison' },
  { value: 'devices', title: 'Devices', description: 'Device type breakdown comparison' },
  { value: 'browsers', title: 'Browsers', description: 'Browser usage comparison' },
  { value: 'campaigns', title: 'Campaigns', description: 'Campaign performance comparison' },
  { value: 'events', title: 'Custom Events', description: 'Event tracking comparison' },
  { value: 'outbound', title: 'Outbound Links', description: 'Outbound click comparison' },
] as const

function ComparisonTabs({ data }: { data: ComparisonResponse }) {
  return (
    <Tabs defaultValue="pages">
      <TabsList className="flex-wrap h-auto">
        <TabsTrigger value="pages">Pages</TabsTrigger>
        <TabsTrigger value="referrers">Referrers</TabsTrigger>
        <TabsTrigger value="countries">Countries</TabsTrigger>
        <TabsTrigger value="devices">Devices</TabsTrigger>
        <TabsTrigger value="browsers">Browsers</TabsTrigger>
        <TabsTrigger value="campaigns">Campaigns</TabsTrigger>
        <TabsTrigger value="events">Events</TabsTrigger>
        <TabsTrigger value="outbound">Outbound</TabsTrigger>
      </TabsList>

      <TabsContent value="pages">
        <DimensionCard tab="pages">
          <ComparisonTable<TopPage>
            current={data.pages.current ?? []}
            previous={data.pages.previous ?? []}
            keyFn={(d) => d.path}
            labelFn={(d) => d.path}
            valueFn={(d) => d.views}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="referrers">
        <DimensionCard tab="referrers">
          <ComparisonTable<Referrer>
            current={data.referrers.current ?? []}
            previous={data.referrers.previous ?? []}
            keyFn={(d) => d.source}
            labelFn={(d) => d.source}
            valueFn={(d) => d.visits}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="countries">
        <DimensionCard tab="countries">
          <ComparisonTable<GeoData>
            current={data.geo.current ?? []}
            previous={data.geo.previous ?? []}
            keyFn={(d) => d.country}
            labelFn={(d) => d.country}
            valueFn={(d) => d.visitors}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="devices">
        <DimensionCard tab="devices">
          <ComparisonTable<DeviceData>
            current={data.devices.current ?? []}
            previous={data.devices.previous ?? []}
            keyFn={(d) => d.device}
            labelFn={(d) => d.device}
            valueFn={(d) => d.visitors}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="browsers">
        <DimensionCard tab="browsers">
          <ComparisonTable<BrowserData>
            current={data.browsers.current ?? []}
            previous={data.browsers.previous ?? []}
            keyFn={(d) => d.browser}
            labelFn={(d) => d.browser}
            valueFn={(d) => d.visitors}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="campaigns">
        <DimensionCard tab="campaigns">
          <ComparisonTable<Campaign>
            current={data.campaigns.current ?? []}
            previous={data.campaigns.previous ?? []}
            keyFn={(d) => `${d.utm_source}/${d.utm_medium}/${d.utm_campaign}`}
            labelFn={(d) => `${d.utm_source} / ${d.utm_medium} / ${d.utm_campaign}`}
            valueFn={(d) => d.sessions}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="events">
        <DimensionCard tab="events">
          <ComparisonTable<CustomEvent>
            current={data.events.current ?? []}
            previous={data.events.previous ?? []}
            keyFn={(d) => d.event_name}
            labelFn={(d) => d.event_name}
            valueFn={(d) => d.count}
          />
        </DimensionCard>
      </TabsContent>

      <TabsContent value="outbound">
        <DimensionCard tab="outbound">
          <ComparisonTable<OutboundLink>
            current={data.outbound.current ?? []}
            previous={data.outbound.previous ?? []}
            keyFn={(d) => d.url}
            labelFn={(d) => d.url}
            valueFn={(d) => d.clicks}
          />
        </DimensionCard>
      </TabsContent>
    </Tabs>
  )
}

function DimensionCard({
  tab,
  children,
}: {
  tab: (typeof TAB_CONFIG)[number]['value']
  children: React.ReactNode
}) {
  const cfg = TAB_CONFIG.find((t) => t.value === tab)!
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{cfg.title}</CardTitle>
        <CardDescription>{cfg.description}</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        {children}
      </CardContent>
    </Card>
  )
}

/* ---------- Comparison Table with bars ---------- */

function ComparisonTable<T>({
  current,
  previous,
  keyFn,
  labelFn,
  valueFn,
}: {
  current: T[]
  previous: T[]
  keyFn: (item: T) => string
  labelFn: (item: T) => string
  valueFn: (item: T) => number
}) {
  const rows = useMemo(() => {
    const currentMap = new Map(current.map((item) => [keyFn(item), item]))
    const previousMap = new Map(previous.map((item) => [keyFn(item), item]))
    const allKeys = new Set([...currentMap.keys(), ...previousMap.keys()])

    const result: {
      name: string
      current: number
      previous: number
      change: number
      changePct: number
      status: 'new' | 'gone' | 'changed'
    }[] = []

    for (const key of allKeys) {
      const cur = currentMap.get(key)
      const prev = previousMap.get(key)
      const curVal = cur ? valueFn(cur) : 0
      const prevVal = prev ? valueFn(prev) : 0
      const change = curVal - prevVal
      const changePct = prevVal > 0 ? (change / prevVal) * 100 : curVal > 0 ? 100 : 0

      result.push({
        name: cur ? labelFn(cur) : prev ? labelFn(prev!) : key,
        current: curVal,
        previous: prevVal,
        change,
        changePct,
        status: !prev ? 'new' : !cur ? 'gone' : 'changed',
      })
    }

    result.sort((a, b) => Math.abs(b.change) - Math.abs(a.change))
    return result
  }, [current, previous, keyFn, labelFn, valueFn])

  const globalMax = useMemo(
    () => Math.max(...rows.map((r) => Math.max(r.current, r.previous)), 1),
    [rows],
  )

  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-muted-foreground">
            <th className="text-left py-2.5 px-4 font-medium">Name</th>
            <th className="text-right py-2.5 px-4 font-medium">Current</th>
            <th className="text-right py-2.5 px-4 font-medium">Previous</th>
            <th className="text-right py-2.5 px-4 font-medium">Change</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const curBarPct = (row.current / globalMax) * 100
            const prevBarPct = (row.previous / globalMax) * 100
            return (
              <tr key={row.name} className="border-b last:border-0 hover:bg-muted/50 transition-colors">
                <td className="py-3 px-4">
                  <div className="flex items-center gap-2">
                    <span className={`font-medium truncate ${row.status === 'gone' ? 'line-through text-muted-foreground' : ''}`}>
                      {row.name}
                    </span>
                    {row.status === 'new' && (
                      <Badge className="bg-blue-500/15 text-blue-600 dark:text-blue-400 border-0 text-[10px] px-1.5 py-0">NEW</Badge>
                    )}
                    {row.status === 'gone' && (
                      <Badge variant="outline" className="text-[10px] px-1.5 py-0">GONE</Badge>
                    )}
                  </div>
                  <div className="flex flex-col gap-0.5 mt-1.5">
                    <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                      <div
                        className="h-full bg-[var(--chart-1)] rounded-full transition-all duration-500"
                        style={{ width: `${curBarPct}%` }}
                      />
                    </div>
                    <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                      <div
                        className="h-full bg-muted-foreground/30 rounded-full transition-all duration-500"
                        style={{ width: `${prevBarPct}%` }}
                      />
                    </div>
                  </div>
                </td>
                <td className="text-right py-3 px-4 font-mono tabular-nums">
                  {row.current.toLocaleString()}
                </td>
                <td className="text-right py-3 px-4 font-mono tabular-nums text-muted-foreground">
                  {row.previous.toLocaleString()}
                </td>
                <td className="text-right py-3 px-4">
                  <div className={`font-mono tabular-nums ${
                    row.change > 0
                      ? 'text-green-600 dark:text-green-400'
                      : row.change < 0
                        ? 'text-red-600 dark:text-red-400'
                        : ''
                  }`}>
                    {row.change > 0 ? '+' : ''}{row.change.toLocaleString()}
                  </div>
                  <div className={`text-xs font-mono tabular-nums ${
                    row.changePct > 0
                      ? 'text-green-600 dark:text-green-400'
                      : row.changePct < 0
                        ? 'text-red-600 dark:text-red-400'
                        : 'text-muted-foreground'
                  }`}>
                    {row.status === 'new' || row.status === 'gone'
                      ? '\u2014'
                      : `${row.changePct > 0 ? '+' : ''}${row.changePct.toFixed(1)}%`}
                  </div>
                </td>
              </tr>
            )
          })}
          {rows.length === 0 && (
            <tr>
              <td colSpan={4} className="text-center py-8 text-muted-foreground">
                No data for this period
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

/* ---------- Loading Skeleton ---------- */

function CompareLoadingSkeleton() {
  return (
    <div className="space-y-6">
      {/* Insight skeletons */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-16 rounded-lg" />
        ))}
      </div>
      {/* KPI card skeletons */}
      <div className="grid gap-4 grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        {Array.from({ length: 6 }).map((_, i) => (
          <Card key={i}>
            <CardContent className="p-4">
              <div className="flex items-start justify-between">
                <div className="space-y-2 flex-1">
                  <Skeleton className="h-3 w-20" />
                  <Skeleton className="h-7 w-16" />
                  <Skeleton className="h-3 w-full" />
                </div>
                <Skeleton className="h-10 w-10 rounded-xl" />
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
      {/* Chart skeleton */}
      <Card>
        <CardHeader className="pb-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-64 mt-1" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[300px] w-full rounded-lg" />
        </CardContent>
      </Card>
      {/* Table skeleton */}
      <Skeleton className="h-[400px] rounded-lg" />
    </div>
  )
}
