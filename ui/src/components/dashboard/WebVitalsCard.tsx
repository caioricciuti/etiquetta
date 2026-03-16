import { CircleHelp, Gauge } from 'lucide-react'
import { useVitals } from '../../hooks/useAnalyticsQueries'
import { formatDuration } from '../../lib/utils'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { FeatureGate, FeatureBadge } from '../FeatureGate'
import { useLicense } from '../../hooks/useLicenseQuery'
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip'

// Google CWV thresholds: [good, needs-improvement] — above the second value is poor
const thresholds: Record<string, [number, number]> = {
  LCP:  [2500, 4000],
  FCP:  [1800, 3000],
  CLS:  [0.1, 0.25],
  TTFB: [800, 1800],
  INP:  [200, 500],
}

const metricInfo: Record<string, { fullName: string; description: string; unit: string }> = {
  LCP:  { fullName: 'Largest Contentful Paint', description: 'Time until the largest content element is visible.', unit: 'ms' },
  FCP:  { fullName: 'First Contentful Paint', description: 'Time until the first text or image is painted.', unit: 'ms' },
  CLS:  { fullName: 'Cumulative Layout Shift', description: 'Measures visual stability — how much the page layout shifts unexpectedly.', unit: '' },
  TTFB: { fullName: 'Time to First Byte', description: 'Time from navigation start until the first byte of the response is received.', unit: 'ms' },
  INP:  { fullName: 'Interaction to Next Paint', description: 'Responsiveness — the latency of the worst user interaction.', unit: 'ms' },
}

function formatThreshold(value: number, unit: string): string {
  if (!unit) return value.toString()
  if (value >= 1000) return `${(value / 1000).toFixed(1)}s`
  return `${value}ms`
}

type Rating = 'good' | 'needs-improvement' | 'poor'

function rateMetric(label: string, raw: number): Rating {
  const t = thresholds[label]
  if (!t) return 'good'
  if (raw <= t[0]) return 'good'
  if (raw <= t[1]) return 'needs-improvement'
  return 'poor'
}

const ratingConfig: Record<Rating, { text: string; className: string }> = {
  'good':              { text: 'Good',              className: 'text-emerald-600 dark:text-emerald-400' },
  'needs-improvement': { text: 'Needs improvement', className: 'text-amber-600 dark:text-amber-400' },
  'poor':              { text: 'Poor',              className: 'text-red-600 dark:text-red-400' },
}

export function WebVitalsCard() {
  const { hasFeature } = useLicense()
  const { data } = useVitals(hasFeature('performance'))

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Gauge className="h-5 w-5 text-muted-foreground" />
          <CardTitle className="text-lg font-semibold">Web Vitals</CardTitle>
          <FeatureBadge feature="performance" />
        </div>
        <CardDescription>Core Web Vitals performance metrics</CardDescription>
      </CardHeader>
      <CardContent>
        <FeatureGate feature="performance">
          {data ? (
            <div className="grid grid-cols-5 gap-4">
              {[
                { label: 'LCP', raw: data.lcp, value: formatDuration(data.lcp), desc: 'Loading' },
                { label: 'FCP', raw: data.fcp, value: formatDuration(data.fcp), desc: 'First Paint' },
                { label: 'CLS', raw: data.cls, value: data.cls.toFixed(3), desc: 'Layout Shift' },
                { label: 'TTFB', raw: data.ttfb, value: formatDuration(data.ttfb), desc: 'Server' },
                { label: 'INP', raw: data.inp, value: formatDuration(data.inp), desc: 'Interaction' },
              ].map((metric) => {
                const rating = rateMetric(metric.label, metric.raw)
                const config = ratingConfig[rating]
                const info = metricInfo[metric.label]
                const t = thresholds[metric.label]
                return (
                  <div key={metric.label} className="relative text-center p-3 rounded-lg bg-muted/50">
                    {info && t && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button className="absolute top-2 right-2 text-muted-foreground hover:text-foreground transition-colors">
                            <CircleHelp className="h-3.5 w-3.5" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-xs text-left">
                          <p className="font-semibold">{info.fullName}</p>
                          <p className="mt-1">{info.description}</p>
                          <div className="mt-1.5 space-y-0.5">
                            <p className="text-emerald-400">Good: ≤ {formatThreshold(t[0], info.unit)}</p>
                            <p className="text-amber-400">Needs improvement: ≤ {formatThreshold(t[1], info.unit)}</p>
                            <p className="text-red-400">Poor: &gt; {formatThreshold(t[1], info.unit)}</p>
                          </div>
                          <p className="mt-1.5 text-muted-foreground italic">Source: Google Web Vitals</p>
                        </TooltipContent>
                      </Tooltip>
                    )}
                    <p className={`text-xl font-bold tracking-tight ${config.className}`}>{metric.value}</p>
                    <p className="text-sm font-semibold text-muted-foreground">{metric.label}</p>
                    <p className="text-xs text-muted-foreground">{metric.desc}</p>
                    <p className={`text-xs font-medium mt-1 ${config.className}`}>{config.text}</p>
                  </div>
                )
              })}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm text-center py-4">No performance data yet</p>
          )}
        </FeatureGate>
      </CardContent>
    </Card>
  )
}
