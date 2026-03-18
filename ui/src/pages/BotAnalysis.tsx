import { useBotAnalysis } from '../hooks/useAnalyticsQueries'
import { formatNumber } from '@/lib/utils'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { Skeleton } from '../components/ui/skeleton'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from '../components/ui/chart'
import { Bot, ShieldAlert, ShieldCheck, AlertTriangle, Users, Brain } from 'lucide-react'
import { Line, LineChart, XAxis, YAxis, CartesianGrid, PieChart, Pie, Cell, BarChart, Bar } from 'recharts'

const CATEGORY_LABELS: Record<string, string> = {
  human: 'Humans',
  good_bot: 'Good Bots',
  suspicious: 'Suspicious',
  bad_bot: 'Bad Bots',
  ai_crawler: 'AI Crawlers',
}

const SIGNAL_LABELS: Record<string, string> = {
  headless_browser: 'Headless',
  webdriver: 'Webdriver',
  headless: 'Headless UA',
  screen_anomaly: 'Screen 0x0',
  no_plugins: 'No Plugins',
  datacenter_ip: 'Datacenter IP',
  zero_interaction: 'No Interaction',
  impossible_speed: 'Impossible Speed',
  perfect_timing: 'Robotic Timing',
  known_good_bot: 'Known Bot',
  automation_ua: 'Automation UA',
  short_ua: 'Short UA',
  empty_ua: 'Empty UA',
  missing_accept_language: 'No Accept-Language',
  suspicious_path: 'Suspicious Path',
  phantom: 'PhantomJS',
  selenium: 'Selenium',
  no_languages: 'No Languages',
  known_ai_crawler: 'AI Crawler',
}

const CATEGORY_BADGE_STYLES: Record<string, string> = {
  good_bot: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
  bad_bot: 'bg-red-500/15 text-red-600 dark:text-red-400',
  suspicious: 'bg-orange-500/15 text-orange-600 dark:text-orange-400',
  ai_crawler: 'bg-purple-500/15 text-purple-600 dark:text-purple-400',
}

const pieChartConfig = {
  events: { label: 'Events' },
  human: { label: 'Humans', color: 'hsl(142, 71%, 45%)' },
  good_bot: { label: 'Good Bots', color: 'hsl(217, 91%, 60%)' },
  suspicious: { label: 'Suspicious', color: 'hsl(38, 92%, 50%)' },
  bad_bot: { label: 'Bad Bots', color: 'hsl(0, 84%, 60%)' },
  ai_crawler: { label: 'AI Crawlers', color: 'hsl(262, 83%, 58%)' },
} satisfies ChartConfig

const areaChartConfig = {
  humans: { label: 'Humans', color: 'hsl(142, 71%, 45%)' },
  good_bots: { label: 'Good Bots', color: 'hsl(217, 91%, 60%)' },
  suspicious: { label: 'Suspicious', color: 'hsl(38, 92%, 50%)' },
  bad_bots: { label: 'Bad Bots', color: 'hsl(0, 84%, 60%)' },
  ai_crawlers: { label: 'AI Crawlers', color: 'hsl(262, 83%, 58%)' },
} satisfies ChartConfig

const barChartConfig = {
  count: { label: 'Visitors', color: 'hsl(262, 83%, 58%)' },
} satisfies ChartConfig

function StatCardSkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <Skeleton className="h-4 w-24 mb-2" />
        <Skeleton className="h-8 w-16" />
      </CardContent>
    </Card>
  )
}

export function BotAnalysis() {
  const { dateRange, setDateRange } = useDateRangeStore()
  const { data, isLoading, isPlaceholderData, isError, error } = useBotAnalysis()

  if (isError) {
    return (
      <div className="p-6">
        <Card>
          <CardContent className="p-6 text-center">
            <AlertTriangle className="h-8 w-8 mx-auto text-yellow-500 mb-2" />
            <p className="text-muted-foreground">{error?.message || 'Failed to load bot data'}</p>
          </CardContent>
        </Card>
      </div>
    )
  }

  const totalEvents = data?.categories?.reduce((sum, c) => sum + c.events, 0) || 0
  const humanEvents = data?.categories?.find(c => c.category === 'human')?.events || 0
  const goodBotEvents = data?.categories?.find(c => c.category === 'good_bot')?.events || 0
  const badBotEvents = data?.categories?.find(c => c.category === 'bad_bot')?.events || 0
  const suspiciousEvents = data?.categories?.find(c => c.category === 'suspicious')?.events || 0
  const aiCrawlerEvents = data?.categories?.find(c => c.category === 'ai_crawler')?.events || 0
  const botPercentage = totalEvents > 0 ? ((totalEvents - humanEvents) / totalEvents * 100).toFixed(1) : '0'

  const pieData = data?.categories?.map(c => ({
    name: c.category,
    value: c.events,
    fill: `var(--color-${c.category})`,
  })) || []

  return (
    <div className="p-6 space-y-6" style={{ opacity: isPlaceholderData ? 0.6 : 1, transition: 'opacity 150ms' }}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Bot className="h-7 w-7" />
            Bot Analysis
          </h1>
          <p className="text-muted-foreground">Monitor and analyze bot traffic on your site</p>
        </div>
        <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} />
      </div>

      {/* Stats Cards */}
      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          <StatCardSkeleton /><StatCardSkeleton /><StatCardSkeleton /><StatCardSkeleton /><StatCardSkeleton />
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          <Card className="transition-all hover:shadow-md hover:border-primary/20">
            <CardContent className="p-6">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Total Traffic</p>
                  <p className="text-2xl font-bold mt-1">{formatNumber(totalEvents)}</p>
                </div>
                <div className="h-10 w-10 rounded-xl bg-muted flex items-center justify-center">
                  <Users className="h-5 w-5 text-muted-foreground" />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="transition-all hover:shadow-md hover:border-primary/20">
            <CardContent className="p-6">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Bot Traffic</p>
                  <p className="text-2xl font-bold mt-1">{botPercentage}%</p>
                </div>
                <div className="h-10 w-10 rounded-xl bg-orange-500/10 flex items-center justify-center">
                  <Bot className="h-5 w-5 text-orange-500" />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="transition-all hover:shadow-md hover:border-primary/20">
            <CardContent className="p-6">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Good Bots</p>
                  <p className="text-2xl font-bold mt-1">{formatNumber(goodBotEvents)}</p>
                </div>
                <div className="h-10 w-10 rounded-xl bg-blue-500/10 flex items-center justify-center">
                  <ShieldCheck className="h-5 w-5 text-blue-500" />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="transition-all hover:shadow-md hover:border-primary/20">
            <CardContent className="p-6">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">AI Crawlers</p>
                  <p className="text-2xl font-bold mt-1">{formatNumber(aiCrawlerEvents)}</p>
                </div>
                <div className="h-10 w-10 rounded-xl bg-purple-500/10 flex items-center justify-center">
                  <Brain className="h-5 w-5 text-purple-500" />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="transition-all hover:shadow-md hover:border-primary/20">
            <CardContent className="p-6">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Bad Bots Blocked</p>
                  <p className="text-2xl font-bold mt-1">{formatNumber(badBotEvents + suspiciousEvents)}</p>
                </div>
                <div className="h-10 w-10 rounded-xl bg-red-500/10 flex items-center justify-center">
                  <ShieldAlert className="h-5 w-5 text-red-500" />
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Charts Row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Category Breakdown Pie */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg font-semibold">Traffic Breakdown</CardTitle>
            <CardDescription>Distribution of traffic by category</CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <Skeleton className="h-[300px] w-full" />
            ) : pieData.length > 0 ? (
              <ChartContainer config={pieChartConfig} className="h-[300px] w-full">
                <PieChart>
                  <ChartTooltip content={<ChartTooltipContent nameKey="name" hideLabel />} />
                  <ChartLegend content={<ChartLegendContent nameKey="name" />} />
                  <Pie data={pieData} cx="50%" cy="50%" innerRadius={60} outerRadius={100} paddingAngle={2} dataKey="value" nameKey="name">
                    {pieData.map((entry, index) => (
                      <Cell key={`cell-${index}`} fill={entry.fill} />
                    ))}
                  </Pie>
                </PieChart>
              </ChartContainer>
            ) : (
              <div className="h-[300px] flex items-center justify-center text-muted-foreground">No data available</div>
            )}
          </CardContent>
        </Card>

        {/* Score Distribution */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg font-semibold">Bot Score Distribution</CardTitle>
            <CardDescription>Traffic segmented by bot detection score (0-100)</CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <Skeleton className="h-[300px] w-full" />
            ) : data?.score_distribution && data.score_distribution.length > 0 ? (
              <ChartContainer config={barChartConfig} className="h-[300px] w-full">
                <BarChart data={data.score_distribution}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis dataKey="range" tick={{ fontSize: 12 }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 12 }} axisLine={false} tickLine={false} width={40} />
                  <ChartTooltip cursor={{ fill: 'hsl(var(--muted))' }} content={<ChartTooltipContent indicator="line" />} />
                  <Bar dataKey="count" fill="var(--color-count)" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ChartContainer>
            ) : (
              <div className="h-[300px] flex items-center justify-center text-muted-foreground">No score distribution data</div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Traffic Over Time */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg font-semibold">Traffic Over Time</CardTitle>
          <CardDescription>Bot vs human traffic trends</CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-[300px] w-full" />
          ) : data?.timeseries && data.timeseries.length > 0 ? (
            <ChartContainer config={areaChartConfig} className="h-[300px] w-full">
              <LineChart data={data.timeseries}>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                <XAxis dataKey="period" tick={{ fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 12 }} axisLine={false} tickLine={false} width={40} />
                <ChartTooltip content={<ChartTooltipContent indicator="dot" />} />
                <ChartLegend content={<ChartLegendContent />} />
                <Line type="monotone" dataKey="humans" stroke="var(--color-humans)" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="good_bots" stroke="var(--color-good_bots)" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="suspicious" stroke="var(--color-suspicious)" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="bad_bots" stroke="var(--color-bad_bots)" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="ai_crawlers" stroke="var(--color-ai_crawlers)" strokeWidth={2} dot={false} />
              </LineChart>
            </ChartContainer>
          ) : (
            <div className="h-[300px] flex items-center justify-center text-muted-foreground">No timeseries data available</div>
          )}
        </CardContent>
      </Card>

      {/* Category Details Table */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg font-semibold">Category Details</CardTitle>
          <CardDescription>Breakdown by traffic category</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Category</th>
                  <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Events</th>
                  <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Visitors</th>
                  <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">% of Total</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  Array.from({ length: 4 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td className="py-3 px-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                      <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                      <td className="py-3 px-4"><Skeleton className="h-4 w-12 ml-auto" /></td>
                    </tr>
                  ))
                ) : (
                  data?.categories?.map((cat) => (
                    <tr key={cat.category} className="border-b border-border last:border-0 hover:bg-muted/50 transition-colors">
                      <td className="py-3 px-4">
                        <div className="flex items-center gap-2">
                          <div className="w-3 h-3 rounded-full" style={{ backgroundColor: `var(--color-${cat.category})` }} />
                          <span className="font-medium">{CATEGORY_LABELS[cat.category] || cat.category}</span>
                        </div>
                      </td>
                      <td className="text-right py-3 px-4 tabular-nums">{formatNumber(cat.events)}</td>
                      <td className="text-right py-3 px-4 tabular-nums">{formatNumber(cat.visitors)}</td>
                      <td className="text-right py-3 px-4 tabular-nums">
                        {totalEvents > 0 ? ((cat.events / totalEvents) * 100).toFixed(1) : '0'}%
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Detected Bots Table */}
      {data?.top_bots && data.top_bots.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg font-semibold">Detected Bots</CardTitle>
            <CardDescription>Individual bot visitors detected on your site</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Bot / User Agent</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Category</th>
                    <th className="text-center py-3 px-4 text-sm font-medium text-muted-foreground">Score</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Detection Signals</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Hits</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Visitors</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Last Seen</th>
                  </tr>
                </thead>
                <tbody>
                  {data.top_bots.map((bot, idx) => (
                    <tr key={`${bot.browser_name}-${bot.score}-${idx}`} className="border-b border-border last:border-0 hover:bg-muted/50 transition-colors">
                      <td className="py-3 px-4">
                        <div className="flex items-center gap-2">
                          <Bot className="h-4 w-4 text-muted-foreground shrink-0" />
                          <span className="font-medium">{bot.browser_name || 'Unknown'}</span>
                        </div>
                      </td>
                      <td className="py-3 px-4">
                        <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${CATEGORY_BADGE_STYLES[bot.category] || 'bg-muted text-muted-foreground'}`}>
                          {CATEGORY_LABELS[bot.category] || bot.category}
                        </span>
                      </td>
                      <td className="text-center py-3 px-4">
                        <span className={`inline-flex items-center justify-center w-10 h-6 rounded text-xs font-bold ${
                          bot.score >= 75 ? 'bg-red-500/15 text-red-600 dark:text-red-400' :
                          bot.score >= 50 ? 'bg-orange-500/15 text-orange-600 dark:text-orange-400' :
                          'bg-yellow-500/15 text-yellow-600 dark:text-yellow-400'
                        }`}>
                          {bot.score}
                        </span>
                      </td>
                      <td className="py-3 px-4">
                        <div className="flex flex-wrap gap-1">
                          {bot.signals.map((signal) => (
                            <span key={signal} className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-muted text-muted-foreground">
                              {SIGNAL_LABELS[signal] || signal}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td className="text-right py-3 px-4 tabular-nums">{formatNumber(bot.hits)}</td>
                      <td className="text-right py-3 px-4 tabular-nums">{formatNumber(bot.visitors)}</td>
                      <td className="text-right py-3 px-4 text-xs text-muted-foreground whitespace-nowrap">
                        {new Date(bot.last_seen).toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
