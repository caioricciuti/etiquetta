import { useState } from 'react'
import { useConsentAnalytics, useConsentRecords, useConsentDomainId } from '@/hooks/useConsent'
import { FeatureGate } from '@/components/FeatureGate'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from '@/components/ui/chart'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  PieChart,
  Pie,
  Cell,
  BarChart,
  Bar,
} from 'recharts'
import { Eye, CheckCircle, XCircle, Settings2, ChevronLeft, ChevronRight } from 'lucide-react'
import { Link } from 'react-router-dom'

const timeseriesConfig = {
  shows: { label: 'Shows', color: 'var(--chart-1)' },
  accept_all: { label: 'Accept All', color: 'var(--chart-2)' },
  reject_all: { label: 'Reject All', color: 'var(--chart-3)' },
  custom: { label: 'Custom', color: 'var(--chart-4)' },
} satisfies ChartConfig

const pieConfig = {
  accept_all: { label: 'Accept All', color: 'var(--chart-2)' },
  reject_all: { label: 'Reject All', color: 'var(--chart-3)' },
  custom: { label: 'Custom', color: 'var(--chart-4)' },
} satisfies ChartConfig

const PIE_COLORS = ['var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)']

function StatCard({ label, value, icon: Icon, subtitle }: {
  label: string
  value: string
  icon: React.ElementType
  subtitle?: string
}) {
  return (
    <Card>
      <CardContent className="pt-6">
        <div className="flex items-center gap-3">
          <div className="rounded-lg bg-muted p-2">
            <Icon className="h-5 w-5 text-muted-foreground" />
          </div>
          <div>
            <p className="text-2xl font-bold">{value}</p>
            <p className="text-sm text-muted-foreground">{label}</p>
            {subtitle && <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function ConsentDashboardContent() {
  const domainId = useConsentDomainId()
  const { data: analytics, isLoading } = useConsentAnalytics(domainId)
  const [recordsPage, setRecordsPage] = useState(1)
  const { data: recordsData } = useConsentRecords(domainId, recordsPage)

  if (!domainId) {
    return (
      <Card>
        <CardContent className="py-8">
          <p className="text-center text-muted-foreground">Select a domain to view consent analytics.</p>
        </CardContent>
      </Card>
    )
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="py-8">
          <p className="text-center text-muted-foreground">Loading consent analytics...</p>
        </CardContent>
      </Card>
    )
  }

  if (!analytics || (analytics.shows === 0 && analytics.total_responses === 0)) {
    return (
      <Card>
        <CardContent className="py-12">
          <div className="text-center space-y-3">
            <Eye className="h-10 w-10 text-muted-foreground mx-auto" />
            <p className="text-lg font-medium">No consent data yet</p>
            <p className="text-sm text-muted-foreground">
              Configure your consent banner in{' '}
              <Link to="/consent/settings" className="underline">settings</Link>{' '}
              to start collecting data.
            </p>
          </div>
        </CardContent>
      </Card>
    )
  }

  const pieData = [
    { name: 'Accept All', value: analytics.accept_all_count },
    { name: 'Reject All', value: analytics.reject_all_count },
    { name: 'Custom', value: analytics.custom_count },
  ].filter(d => d.value > 0)

  const records = recordsData?.records ?? []
  const totalRecords = recordsData?.total ?? 0
  const totalPages = Math.ceil(totalRecords / 50)

  return (
    <div className="space-y-6">
      {/* KPI Cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          icon={Eye}
          label="Banner Shows"
          value={analytics.shows.toLocaleString()}
        />
        <StatCard
          icon={CheckCircle}
          label="Responses"
          value={analytics.total_responses.toLocaleString()}
          subtitle={`${(analytics.response_rate * 100).toFixed(1)}% response rate`}
        />
        <StatCard
          icon={CheckCircle}
          label="Consent Rate"
          value={`${(analytics.consent_rate * 100).toFixed(1)}%`}
          subtitle="Accept + Custom"
        />
        <StatCard
          icon={XCircle}
          label="Reject Rate"
          value={analytics.total_responses > 0
            ? `${((analytics.reject_all_count / analytics.total_responses) * 100).toFixed(1)}%`
            : '0%'}
          subtitle={`${analytics.reject_all_count.toLocaleString()} rejections`}
        />
      </div>

      {/* Charts row */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Timeseries — spans 2 columns */}
        <Card className="lg:col-span-2">
          <CardHeader className="pb-2">
            <CardTitle className="text-lg">Consent Activity</CardTitle>
            <CardDescription>Banner shows and responses over time</CardDescription>
          </CardHeader>
          <CardContent>
            {analytics.timeseries.length > 0 ? (
              <ChartContainer config={timeseriesConfig} className="h-[300px] w-full">
                <AreaChart data={analytics.timeseries} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="fillShows" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="var(--color-shows)" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="var(--color-shows)" stopOpacity={0.05} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis
                    dataKey="period"
                    tick={{ fontSize: 12 }}
                    tickFormatter={(v) => v?.slice(5) || v}
                    axisLine={false}
                    tickLine={false}
                    className="text-muted-foreground"
                  />
                  <YAxis tick={{ fontSize: 12 }} axisLine={false} tickLine={false} width={40} className="text-muted-foreground" />
                  <ChartTooltip content={<ChartTooltipContent indicator="dot" />} />
                  <ChartLegend content={<ChartLegendContent />} />
                  <Area type="monotone" dataKey="shows" stroke="var(--color-shows)" strokeWidth={2} fill="url(#fillShows)" />
                  <Area type="monotone" dataKey="accept_all" stroke="var(--color-accept_all)" strokeWidth={2} fill="none" />
                  <Area type="monotone" dataKey="reject_all" stroke="var(--color-reject_all)" strokeWidth={2} fill="none" />
                  <Area type="monotone" dataKey="custom" stroke="var(--color-custom)" strokeWidth={2} fill="none" />
                </AreaChart>
              </ChartContainer>
            ) : (
              <div className="h-[300px] flex items-center justify-center text-muted-foreground">
                No data for the selected period
              </div>
            )}
          </CardContent>
        </Card>

        {/* Pie chart — action breakdown */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-lg">Response Breakdown</CardTitle>
            <CardDescription>How visitors respond</CardDescription>
          </CardHeader>
          <CardContent>
            {pieData.length > 0 ? (
              <ChartContainer config={pieConfig} className="h-[300px] w-full">
                <PieChart>
                  <Pie
                    data={pieData}
                    cx="50%"
                    cy="50%"
                    innerRadius={60}
                    outerRadius={100}
                    paddingAngle={3}
                    dataKey="value"
                    nameKey="name"
                  >
                    {pieData.map((_, i) => (
                      <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                    ))}
                  </Pie>
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <ChartLegend content={<ChartLegendContent />} />
                </PieChart>
              </ChartContainer>
            ) : (
              <div className="h-[300px] flex items-center justify-center text-muted-foreground">
                No responses yet
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Geo breakdown */}
      {analytics.geo_breakdown.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-lg">Consent by Country</CardTitle>
            <CardDescription>Top countries by banner impressions</CardDescription>
          </CardHeader>
          <CardContent>
            <ChartContainer
              config={{
                accept_all: { label: 'Accept All', color: 'var(--chart-2)' },
                reject_all: { label: 'Reject All', color: 'var(--chart-3)' },
                custom: { label: 'Custom', color: 'var(--chart-4)' },
              }}
              className="h-[300px] w-full"
            >
              <BarChart
                data={analytics.geo_breakdown.slice(0, 10)}
                layout="vertical"
                margin={{ top: 5, right: 10, left: 50, bottom: 5 }}
              >
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" horizontal={false} />
                <XAxis type="number" tick={{ fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis type="category" dataKey="country" tick={{ fontSize: 12 }} axisLine={false} tickLine={false} width={50} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <ChartLegend content={<ChartLegendContent />} />
                <Bar dataKey="accept_all" stackId="a" fill="var(--chart-2)" radius={[0, 0, 0, 0]} />
                <Bar dataKey="reject_all" stackId="a" fill="var(--chart-3)" />
                <Bar dataKey="custom" stackId="a" fill="var(--chart-4)" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ChartContainer>
          </CardContent>
        </Card>
      )}

      {/* Recent Records */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-lg">Recent Consent Records</CardTitle>
          <CardDescription>Individual consent decisions (excluding banner shows)</CardDescription>
        </CardHeader>
        <CardContent>
          {records.length > 0 ? (
            <>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b text-left text-muted-foreground">
                      <th className="pb-2 pr-4 font-medium">Time</th>
                      <th className="pb-2 pr-4 font-medium">Action</th>
                      <th className="pb-2 pr-4 font-medium">Country</th>
                      <th className="pb-2 font-medium">Visitor</th>
                    </tr>
                  </thead>
                  <tbody>
                    {records.filter(r => r.action !== 'show').map((record) => (
                      <tr key={record.id} className="border-b border-border/50 last:border-0">
                        <td className="py-2 pr-4 text-muted-foreground">
                          {new Date(record.timestamp).toLocaleString()}
                        </td>
                        <td className="py-2 pr-4">
                          <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${
                            record.action === 'accept_all'
                              ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                              : record.action === 'reject_all'
                              ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
                              : 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
                          }`}>
                            {record.action.replace('_', ' ')}
                          </span>
                        </td>
                        <td className="py-2 pr-4">{record.geo_country || '—'}</td>
                        <td className="py-2 font-mono text-xs text-muted-foreground">
                          {record.visitor_hash?.slice(0, 8)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {totalPages > 1 && (
                <div className="flex items-center justify-between mt-4 pt-4 border-t">
                  <p className="text-sm text-muted-foreground">
                    Page {recordsPage} of {totalPages} ({totalRecords} records)
                  </p>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={recordsPage <= 1}
                      onClick={() => setRecordsPage(p => p - 1)}
                    >
                      <ChevronLeft className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={recordsPage >= totalPages}
                      onClick={() => setRecordsPage(p => p + 1)}
                    >
                      <ChevronRight className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              )}
            </>
          ) : (
            <p className="text-sm text-muted-foreground py-4 text-center">No records found.</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export function ConsentDashboard() {
  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Consent Analytics</h1>
          <p className="text-muted-foreground">Track how visitors interact with your consent banner</p>
        </div>
        <Button variant="outline" asChild>
          <Link to="/consent/settings">
            <Settings2 className="h-4 w-4 mr-2" />
            Configure
          </Link>
        </Button>
      </div>
      <FeatureGate feature="consent">
        <ConsentDashboardContent />
      </FeatureGate>
    </div>
  )
}
