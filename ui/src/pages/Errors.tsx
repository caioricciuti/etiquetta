import { useState } from 'react'
import { useErrors, useErrorDetail, useErrorTimeseries } from '../hooks/useAnalyticsQueries'
import { useLicense } from '../hooks/useLicenseQuery'
import { formatNumber } from '@/lib/utils'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { Skeleton } from '../components/ui/skeleton'
import { FeatureGate, FeatureBadge } from '../components/FeatureGate'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '../components/ui/chart'
import {
  AlertTriangle,
  Search,
  X,
  Code,
  Globe,
  Monitor,
  MapPin,
  Clock,
  Users,
  Hash,
  ExternalLink,
  ChevronDown,
  ChevronRight,
} from 'lucide-react'
import { Area, AreaChart, XAxis, YAxis, CartesianGrid } from 'recharts'
import type { ErrorSummary, ErrorDetail } from '../lib/types'

const timeseriesConfig = {
  count: { label: 'Errors', color: 'hsl(0, 72%, 51%)' },
} satisfies ChartConfig

function ErrorDetailPanel({ error, onClose }: { error: ErrorSummary; onClose: () => void }) {
  const { hasFeature } = useLicense()
  const enabled = hasFeature('error_tracking')
  const { data: details, isLoading: detailsLoading } = useErrorDetail(error.error_hash, enabled)
  const { data: tsData, isLoading: tsLoading } = useErrorTimeseries(error.error_hash, enabled)
  const [expandedStack, setExpandedStack] = useState<string | null>(null)

  const occurrences = details ?? []

  // Aggregate browsers
  const browsers = occurrences.reduce<Record<string, number>>((acc, d) => {
    const name = d.browser_name ?? 'Unknown'
    acc[name] = (acc[name] ?? 0) + 1
    return acc
  }, {})
  const browserList = Object.entries(browsers).sort((a, b) => b[1] - a[1])

  // Aggregate pages
  const pages = occurrences.reduce<Record<string, number>>((acc, d) => {
    acc[d.path] = (acc[d.path] ?? 0) + 1
    return acc
  }, {})
  const pageList = Object.entries(pages).sort((a, b) => b[1] - a[1])

  // Aggregate countries
  const countries = occurrences.reduce<Record<string, number>>((acc, d) => {
    const name = d.geo_country ?? 'Unknown'
    acc[name] = (acc[name] ?? 0) + 1
    return acc
  }, {})
  const countryList = Object.entries(countries).sort((a, b) => b[1] - a[1])

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            Error Details
          </h3>
          <p className="text-sm text-muted-foreground mt-1 break-all">
            {error.error_message || 'Unknown error'}
          </p>
          <div className="flex items-center gap-3 mt-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-destructive/10 text-destructive font-medium">
              {error.error_type || 'Error'}
            </span>
            <span>{formatNumber(error.occurrences)} occurrences</span>
            <span>{formatNumber(error.affected_sessions)} sessions</span>
          </div>
        </div>
        <button
          onClick={onClose}
          className="h-8 w-8 rounded-md flex items-center justify-center hover:bg-muted transition-colors shrink-0"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Timeseries */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Clock className="h-4 w-4 text-muted-foreground" />
            Occurrences Over Time
          </CardTitle>
        </CardHeader>
        <CardContent>
          {tsLoading ? (
            <Skeleton className="h-[180px] w-full" />
          ) : tsData && tsData.length > 0 ? (
            <ChartContainer config={timeseriesConfig} className="h-[180px] w-full">
              <AreaChart data={tsData}>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                <XAxis dataKey="period" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={35} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Area type="monotone" dataKey="count" stroke="var(--color-count)" fill="var(--color-count)" fillOpacity={0.15} strokeWidth={2} />
              </AreaChart>
            </ChartContainer>
          ) : (
            <div className="h-[180px] flex items-center justify-center text-sm text-muted-foreground">No timeseries data</div>
          )}
        </CardContent>
      </Card>

      {/* Recent occurrences with stack traces */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Code className="h-4 w-4 text-muted-foreground" />
            Recent Occurrences
          </CardTitle>
          <CardDescription>{occurrences.length} most recent</CardDescription>
        </CardHeader>
        <CardContent>
          {detailsLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full" />
              ))}
            </div>
          ) : occurrences.length > 0 ? (
            <div className="space-y-2 max-h-[400px] overflow-y-auto">
              {occurrences.slice(0, 20).map((occ) => (
                <OccurrenceRow
                  key={occ.id}
                  occurrence={occ}
                  expanded={expandedStack === occ.id}
                  onToggle={() => setExpandedStack(expandedStack === occ.id ? null : occ.id)}
                />
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground text-center py-4">No occurrences found</p>
          )}
        </CardContent>
      </Card>

      {/* Breakdown cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Affected pages */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Globe className="h-4 w-4 text-muted-foreground" />
              Affected Pages
            </CardTitle>
          </CardHeader>
          <CardContent>
            {detailsLoading ? (
              <Skeleton className="h-20 w-full" />
            ) : pageList.length > 0 ? (
              <div className="space-y-2">
                {pageList.slice(0, 5).map(([path, count]) => (
                  <div key={path} className="flex items-center justify-between gap-2">
                    <span className="text-sm truncate min-w-0 flex-1 font-mono text-xs">{path}</span>
                    <span className="text-xs text-muted-foreground tabular-nums shrink-0">{count}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground text-center py-2">No data</p>
            )}
          </CardContent>
        </Card>

        {/* Browsers */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Monitor className="h-4 w-4 text-muted-foreground" />
              Browsers
            </CardTitle>
          </CardHeader>
          <CardContent>
            {detailsLoading ? (
              <Skeleton className="h-20 w-full" />
            ) : browserList.length > 0 ? (
              <div className="space-y-2">
                {browserList.slice(0, 5).map(([name, count]) => (
                  <div key={name} className="flex items-center justify-between gap-2">
                    <span className="text-sm truncate">{name}</span>
                    <span className="text-xs text-muted-foreground tabular-nums shrink-0">{count}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground text-center py-2">No data</p>
            )}
          </CardContent>
        </Card>

        {/* Countries */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <MapPin className="h-4 w-4 text-muted-foreground" />
              Countries
            </CardTitle>
          </CardHeader>
          <CardContent>
            {detailsLoading ? (
              <Skeleton className="h-20 w-full" />
            ) : countryList.length > 0 ? (
              <div className="space-y-2">
                {countryList.slice(0, 5).map(([name, count]) => (
                  <div key={name} className="flex items-center justify-between gap-2">
                    <span className="text-sm truncate">{name}</span>
                    <span className="text-xs text-muted-foreground tabular-nums shrink-0">{count}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground text-center py-2">No data</p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function OccurrenceRow({ occurrence, expanded, onToggle }: { occurrence: ErrorDetail; expanded: boolean; onToggle: () => void }) {
  const ts = new Date(occurrence.timestamp)
  const hasStack = !!occurrence.error_stack

  return (
    <div className="rounded-lg border border-border">
      <button
        onClick={onToggle}
        className="w-full text-left p-3 flex items-start gap-3 hover:bg-muted/50 transition-colors"
      >
        <div className="mt-0.5 shrink-0">
          {hasStack ? (
            expanded ? <ChevronDown className="h-4 w-4 text-muted-foreground" /> : <ChevronRight className="h-4 w-4 text-muted-foreground" />
          ) : (
            <div className="h-4 w-4" />
          )}
        </div>
        <div className="flex-1 min-w-0 space-y-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-xs text-muted-foreground">
              {ts.toLocaleDateString()} {ts.toLocaleTimeString()}
            </span>
            {occurrence.browser_name && (
              <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                {occurrence.browser_name}
              </span>
            )}
            {occurrence.geo_country && (
              <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                {occurrence.geo_country}
              </span>
            )}
          </div>
          <p className="text-sm truncate font-mono">{occurrence.path}</p>
          {occurrence.script_url && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <ExternalLink className="h-3 w-3 shrink-0" />
              <span className="truncate">{occurrence.script_url}</span>
              {occurrence.line_number != null && (
                <span className="shrink-0">:{occurrence.line_number}{occurrence.column_number != null ? `:${occurrence.column_number}` : ''}</span>
              )}
            </div>
          )}
        </div>
      </button>
      {expanded && hasStack && (
        <div className="px-3 pb-3 pl-10">
          <pre className="text-xs bg-muted/50 rounded-md p-3 overflow-x-auto whitespace-pre-wrap break-all font-mono leading-relaxed max-h-[300px] overflow-y-auto">
            {occurrence.error_stack}
          </pre>
        </div>
      )}
    </div>
  )
}

export function Errors() {
  const { dateRange, setDateRange } = useDateRangeStore()
  const { hasFeature } = useLicense()
  const enabled = hasFeature('error_tracking')
  const [search, setSearch] = useState('')
  const [selectedError, setSelectedError] = useState<ErrorSummary | null>(null)

  const { data, isLoading, isPlaceholderData } = useErrors(enabled)
  const { data: tsData, isLoading: tsLoading } = useErrorTimeseries(undefined, enabled)

  const errors = data ?? []
  const filtered = errors.filter((err) =>
    !search || err.error_message.toLowerCase().includes(search.toLowerCase()) || err.error_type.toLowerCase().includes(search.toLowerCase()),
  )

  const totalOccurrences = errors.reduce((sum, e) => sum + e.occurrences, 0)
  const totalSessions = errors.reduce((sum, e) => sum + e.affected_sessions, 0)
  const uniqueTypes = new Set(errors.map((e) => e.error_type)).size

  return (
    <div className="p-6 space-y-6 overflow-y-auto h-full" style={{ opacity: isPlaceholderData ? 0.6 : 1, transition: 'opacity 150ms' }}>
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <AlertTriangle className="h-7 w-7" />
            Errors
            <FeatureBadge feature="error_tracking" />
          </h1>
          <p className="text-muted-foreground">JavaScript errors from your users</p>
        </div>
        <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} />
      </div>

      <FeatureGate feature="error_tracking">
        {/* Summary cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card>
            <CardContent className="p-6">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Total Errors</p>
              {isLoading ? (
                <Skeleton className="h-8 w-20 mt-1" />
              ) : (
                <p className="text-2xl font-bold mt-1 text-destructive">{formatNumber(totalOccurrences)}</p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-6">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Affected Sessions</p>
              {isLoading ? (
                <Skeleton className="h-8 w-20 mt-1" />
              ) : (
                <div className="flex items-center gap-2 mt-1">
                  <Users className="h-5 w-5 text-muted-foreground" />
                  <p className="text-2xl font-bold">{formatNumber(totalSessions)}</p>
                </div>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-6">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Unique Errors</p>
              {isLoading ? (
                <Skeleton className="h-8 w-20 mt-1" />
              ) : (
                <div className="flex items-center gap-2 mt-1">
                  <Hash className="h-5 w-5 text-muted-foreground" />
                  <p className="text-2xl font-bold">{formatNumber(errors.length)}</p>
                </div>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-6">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Error Types</p>
              {isLoading ? (
                <Skeleton className="h-8 w-20 mt-1" />
              ) : (
                <p className="text-2xl font-bold mt-1">{formatNumber(uniqueTypes)}</p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Overall timeseries */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Error Trend</CardTitle>
            <CardDescription>All errors over time</CardDescription>
          </CardHeader>
          <CardContent>
            {tsLoading ? (
              <Skeleton className="h-[200px] w-full" />
            ) : tsData && tsData.length > 0 ? (
              <ChartContainer config={timeseriesConfig} className="h-[200px] w-full">
                <AreaChart data={tsData}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis dataKey="period" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={35} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Area type="monotone" dataKey="count" stroke="var(--color-count)" fill="var(--color-count)" fillOpacity={0.15} strokeWidth={2} />
                </AreaChart>
              </ChartContainer>
            ) : (
              <div className="h-[200px] flex items-center justify-center text-sm text-muted-foreground">No error data in this period</div>
            )}
          </CardContent>
        </Card>

        {/* Search */}
        <div className="flex items-center gap-3">
          <div className="relative flex-1 max-w-sm">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <input
              type="text"
              placeholder="Search errors..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full pl-9 pr-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <span className="text-sm text-muted-foreground">
            {filtered.length} error{filtered.length !== 1 ? 's' : ''}
          </span>
        </div>

        {/* Main content */}
        <div className={`grid gap-6 ${selectedError ? 'grid-cols-1 lg:grid-cols-2' : 'grid-cols-1'}`}>
          {/* Errors table */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg font-semibold flex items-center gap-2">
                <AlertTriangle className="h-5 w-5 text-muted-foreground" />
                Error Breakdown
              </CardTitle>
              <CardDescription>Click an error to see details</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Error</th>
                      <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Type</th>
                      <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Count</th>
                      <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Sessions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {isLoading ? (
                      Array.from({ length: 8 }).map((_, i) => (
                        <tr key={i} className="border-b border-border">
                          <td className="py-3 px-4"><Skeleton className="h-4 w-48" /></td>
                          <td className="py-3 px-4"><Skeleton className="h-4 w-16" /></td>
                          <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                          <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                        </tr>
                      ))
                    ) : filtered.length === 0 ? (
                      <tr>
                        <td colSpan={4} className="text-center py-8 text-sm text-muted-foreground">
                          {errors.length === 0 ? 'No errors recorded' : 'No errors match your search'}
                        </td>
                      </tr>
                    ) : (
                      filtered.map((err, idx) => (
                        <tr
                          key={`${err.error_hash}-${idx}`}
                          className={`border-b border-border last:border-0 cursor-pointer transition-colors ${
                            selectedError?.error_hash === err.error_hash
                              ? 'bg-destructive/5'
                              : 'hover:bg-muted/50'
                          }`}
                          onClick={() => setSelectedError(
                            selectedError?.error_hash === err.error_hash ? null : err,
                          )}
                        >
                          <td className="py-3 px-4 max-w-[300px]">
                            <p className="font-medium text-sm truncate">{err.error_message || 'Unknown error'}</p>
                            <p className="text-xs text-muted-foreground font-mono truncate mt-0.5">{err.error_hash.slice(0, 12)}</p>
                          </td>
                          <td className="py-3 px-4">
                            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-destructive/10 text-destructive">
                              {err.error_type || 'Error'}
                            </span>
                          </td>
                          <td className="text-right py-3 px-4 tabular-nums text-sm">{formatNumber(err.occurrences)}</td>
                          <td className="text-right py-3 px-4 tabular-nums text-sm">{formatNumber(err.affected_sessions)}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          {/* Detail panel */}
          {selectedError && (
            <ErrorDetailPanel error={selectedError} onClose={() => setSelectedError(null)} />
          )}
        </div>
      </FeatureGate>
    </div>
  )
}
