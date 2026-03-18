import { useState } from 'react'
import { useEventsSummary, useEventsTimeseries, useEventsProps } from '../hooks/useAnalyticsQueries'
import { formatNumber } from '@/lib/utils'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { Skeleton } from '../components/ui/skeleton'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '../components/ui/chart'
import { Zap, ArrowUpRight, ArrowDownRight, X, Search, TrendingUp } from 'lucide-react'
import { Line, LineChart, XAxis, YAxis, CartesianGrid, BarChart, Bar } from 'recharts'
import type { EventSummaryItem } from '../lib/types'

const EVENT_TYPE_LABELS: Record<string, string> = {
  pageview: 'Page View',
  custom: 'Custom',
  click: 'Click',
  scroll: 'Scroll',
  engagement: 'Engagement',
}

const EVENT_TYPE_COLORS: Record<string, string> = {
  pageview: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
  custom: 'bg-cyan-500/15 text-cyan-600 dark:text-cyan-400',
  click: 'bg-green-500/15 text-green-600 dark:text-green-400',
  scroll: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  engagement: 'bg-purple-500/15 text-purple-600 dark:text-purple-400',
}

const timeseriesConfig = {
  count: { label: 'Events', color: 'hsl(200, 95%, 48%)' },
  visitors: { label: 'Visitors', color: 'hsl(262, 83%, 58%)' },
} satisfies ChartConfig

const propsBarConfig = {
  count: { label: 'Count', color: 'hsl(200, 95%, 48%)' },
} satisfies ChartConfig

function EventTypeBadge({ type }: { type: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${EVENT_TYPE_COLORS[type] || 'bg-muted text-muted-foreground'}`}>
      {EVENT_TYPE_LABELS[type] || type}
    </span>
  )
}

function EventDetailPanel({ event, onClose }: { event: EventSummaryItem; onClose: () => void }) {
  const { data: tsData, isLoading: tsLoading } = useEventsTimeseries(event.event_name, event.event_type)
  const { data: propsData, isLoading: propsLoading } = useEventsProps(event.event_name)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold flex items-center gap-2">
            {event.event_name}
            <EventTypeBadge type={event.event_type} />
          </h3>
          <p className="text-sm text-muted-foreground">
            {formatNumber(event.count)} events from {formatNumber(event.unique_visitors)} visitors
          </p>
        </div>
        <button
          onClick={onClose}
          className="h-8 w-8 rounded-md flex items-center justify-center hover:bg-muted transition-colors"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Timeseries */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium">Events Over Time</CardTitle>
        </CardHeader>
        <CardContent>
          {tsLoading ? (
            <Skeleton className="h-[200px] w-full" />
          ) : tsData && tsData.length > 0 ? (
            <ChartContainer config={timeseriesConfig} className="h-[200px] w-full">
              <LineChart data={tsData}>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                <XAxis dataKey="period" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={35} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Line type="monotone" dataKey="count" stroke="var(--color-count)" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="visitors" stroke="var(--color-visitors)" strokeWidth={2} dot={false} strokeDasharray="5 5" />
              </LineChart>
            </ChartContainer>
          ) : (
            <div className="h-[200px] flex items-center justify-center text-sm text-muted-foreground">No timeseries data</div>
          )}
        </CardContent>
      </Card>

      {/* Properties */}
      {propsLoading ? (
        <Card>
          <CardContent className="p-6">
            <Skeleton className="h-4 w-32 mb-4" />
            <Skeleton className="h-[150px] w-full" />
          </CardContent>
        </Card>
      ) : propsData?.properties && propsData.properties.length > 0 ? (
        <div className="space-y-3">
          {propsData.properties.map((prop) => (
            <Card key={prop.key}>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{prop.key}</code>
                  <span className="text-xs text-muted-foreground font-normal">{formatNumber(prop.count)} events</span>
                </CardTitle>
              </CardHeader>
              <CardContent>
                {prop.values.length > 0 ? (
                  <ChartContainer config={propsBarConfig} className="h-[120px] w-full">
                    <BarChart data={prop.values.slice(0, 8)} layout="vertical">
                      <XAxis type="number" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                      <YAxis type="category" dataKey="value" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={100} />
                      <ChartTooltip content={<ChartTooltipContent />} />
                      <Bar dataKey="count" fill="var(--color-count)" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ChartContainer>
                ) : (
                  <p className="text-sm text-muted-foreground">No values</p>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}
    </div>
  )
}

export function Events() {
  const { dateRange, setDateRange } = useDateRangeStore()
  const [selectedType, setSelectedType] = useState<string>('')
  const [search, setSearch] = useState('')
  const [selectedEvent, setSelectedEvent] = useState<EventSummaryItem | null>(null)

  const { data, isLoading, isPlaceholderData } = useEventsSummary(selectedType || undefined)

  const filteredEvents = (data?.events ?? []).filter((ev) =>
    !search || ev.event_name.toLowerCase().includes(search.toLowerCase()),
  )

  const trendPct = data && data.prev_total > 0
    ? ((data.total - data.prev_total) / data.prev_total * 100)
    : 0
  const trendUp = trendPct >= 0

  return (
    <div className="p-6 space-y-6 overflow-y-auto h-full" style={{ opacity: isPlaceholderData ? 0.6 : 1, transition: 'opacity 150ms' }}>
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Zap className="h-7 w-7" />
            Events
          </h1>
          <p className="text-muted-foreground">All tracked events across your sites</p>
        </div>
        <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} />
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardContent className="p-6">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Total Events</p>
            {isLoading ? (
              <Skeleton className="h-8 w-20 mt-1" />
            ) : (
              <div className="flex items-center gap-2 mt-1">
                <p className="text-2xl font-bold">{formatNumber(data?.total ?? 0)}</p>
                {data?.prev_total !== undefined && data.prev_total > 0 && (
                  <span className={`flex items-center text-xs font-medium ${trendUp ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                    {trendUp ? <ArrowUpRight className="h-3 w-3" /> : <ArrowDownRight className="h-3 w-3" />}
                    {Math.abs(trendPct).toFixed(1)}%
                  </span>
                )}
              </div>
            )}
          </CardContent>
        </Card>

        {isLoading ? (
          <>
            <Card><CardContent className="p-6"><Skeleton className="h-4 w-20 mb-2" /><Skeleton className="h-8 w-16" /></CardContent></Card>
            <Card><CardContent className="p-6"><Skeleton className="h-4 w-20 mb-2" /><Skeleton className="h-8 w-16" /></CardContent></Card>
            <Card><CardContent className="p-6"><Skeleton className="h-4 w-20 mb-2" /><Skeleton className="h-8 w-16" /></CardContent></Card>
          </>
        ) : (
          (data?.type_counts ?? []).slice(0, 3).map((tc) => (
            <Card key={tc.event_type} className="cursor-pointer hover:border-primary/30 transition-colors" onClick={() => setSelectedType(selectedType === tc.event_type ? '' : tc.event_type)}>
              <CardContent className="p-6">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {EVENT_TYPE_LABELS[tc.event_type] || tc.event_type}
                  </p>
                  {selectedType === tc.event_type && (
                    <span className="text-xs text-primary font-medium">filtered</span>
                  )}
                </div>
                <p className="text-2xl font-bold mt-1">{formatNumber(tc.count)}</p>
              </CardContent>
            </Card>
          ))
        )}
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 flex-wrap">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input
            type="text"
            placeholder="Search events..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-ring"
          />
        </div>
        <div className="flex gap-1.5">
          <button
            onClick={() => setSelectedType('')}
            className={`px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${!selectedType ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground'}`}
          >
            All
          </button>
          {['pageview', 'custom', 'click', 'scroll', 'engagement'].map((t) => (
            <button
              key={t}
              onClick={() => setSelectedType(selectedType === t ? '' : t)}
              className={`px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${selectedType === t ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground'}`}
            >
              {EVENT_TYPE_LABELS[t]}
            </button>
          ))}
        </div>
      </div>

      {/* Main content */}
      <div className={`grid gap-6 ${selectedEvent ? 'grid-cols-1 lg:grid-cols-2' : 'grid-cols-1'}`}>
        {/* Events table */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg font-semibold flex items-center gap-2">
              <TrendingUp className="h-5 w-5 text-muted-foreground" />
              Event Breakdown
            </CardTitle>
            <CardDescription>
              {filteredEvents.length} event{filteredEvents.length !== 1 ? 's' : ''} found
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Event</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Type</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Count</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Visitors</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Sessions</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoading ? (
                    Array.from({ length: 8 }).map((_, i) => (
                      <tr key={i} className="border-b border-border">
                        <td className="py-3 px-4"><Skeleton className="h-4 w-32" /></td>
                        <td className="py-3 px-4"><Skeleton className="h-4 w-16" /></td>
                        <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                        <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                        <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                      </tr>
                    ))
                  ) : filteredEvents.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="text-center py-8 text-sm text-muted-foreground">
                        No events found
                      </td>
                    </tr>
                  ) : (
                    filteredEvents.map((ev, idx) => (
                      <tr
                        key={`${ev.event_type}-${ev.event_name}-${idx}`}
                        className={`border-b border-border last:border-0 cursor-pointer transition-colors ${
                          selectedEvent?.event_name === ev.event_name && selectedEvent?.event_type === ev.event_type
                            ? 'bg-primary/5'
                            : 'hover:bg-muted/50'
                        }`}
                        onClick={() => setSelectedEvent(
                          selectedEvent?.event_name === ev.event_name && selectedEvent?.event_type === ev.event_type
                            ? null
                            : ev,
                        )}
                      >
                        <td className="py-3 px-4">
                          <span className="font-medium">{ev.event_name}</span>
                        </td>
                        <td className="py-3 px-4">
                          <EventTypeBadge type={ev.event_type} />
                        </td>
                        <td className="text-right py-3 px-4 tabular-nums">{formatNumber(ev.count)}</td>
                        <td className="text-right py-3 px-4 tabular-nums">{formatNumber(ev.unique_visitors)}</td>
                        <td className="text-right py-3 px-4 tabular-nums">{formatNumber(ev.sessions)}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>

        {/* Detail panel */}
        {selectedEvent && (
          <EventDetailPanel event={selectedEvent} onClose={() => setSelectedEvent(null)} />
        )}
      </div>
    </div>
  )
}
