import { useEffect, useMemo, useRef, useState } from 'react'
import { useDomainStore } from '../stores/useDomainStore'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import {
  useHeatmapPages,
  useClickHeatmap,
  useScrollHeatmap,
  type HeatmapPoint,
} from '../hooks/useHeatmap'
import { Card } from '../components/ui/card'
import { Skeleton } from '../components/ui/skeleton'
import { DateRangePicker } from '../components/ui/date-range-picker'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../components/ui/select'
import {
  Flame,
  MousePointerClick,
  AlertCircle,
  ArrowDownWideNarrow,
} from 'lucide-react'

// ---- formatting helpers ----------------------------------------------------

function formatNumber(n: number): string {
  return new Intl.NumberFormat().format(Math.round(n))
}

function formatPct(n: number): string {
  return `${Math.round(n)}%`
}

// ---- canvas click-density heatmap -----------------------------------------

interface ClickHeatmapCanvasProps {
  points: HeatmapPoint[]
  maxX: number
  maxY: number
  maxCount: number
  bucket: number
}

/**
 * Renders a viewport-relative click-density heatmap onto an HTML canvas.
 * Each click bucket is drawn as a radial gradient whose alpha scales with
 * count/maxCount; overlapping gradients composite to build up "heat", then a
 * blue -> green -> yellow -> red gradient ramp is applied via the alpha channel.
 */
function ClickHeatmapCanvas({
  points,
  maxX,
  maxY,
  maxCount,
  bucket,
}: ClickHeatmapCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [containerWidth, setContainerWidth] = useState(0)

  // Track container width so the canvas fits responsively.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const update = () => setContainerWidth(el.clientWidth)
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Logical (source) dimensions of the captured viewport space.
  const sourceWidth = Math.max(maxX + bucket, 320)
  const sourceHeight = Math.max(maxY + bucket, 240)

  // Scale to fit the container width (never upscale beyond 1x).
  const scale =
    containerWidth > 0 ? Math.min(containerWidth / sourceWidth, 1) : 1
  const canvasWidth = Math.round(sourceWidth * scale)
  const canvasHeight = Math.round(sourceHeight * scale)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || canvasWidth === 0 || canvasHeight === 0) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    ctx.clearRect(0, 0, canvasWidth, canvasHeight)
    if (points.length === 0 || maxCount <= 0) return

    // 1) Draw greyscale alpha "heat" from radial gradients.
    const radius = Math.max(bucket * scale * 1.6, 14)
    for (const p of points) {
      const cx = p.x * scale
      const cy = p.y * scale
      const intensity = Math.min(p.count / maxCount, 1)
      // Boost low-intensity points so single clicks remain visible.
      const alpha = 0.15 + Math.pow(intensity, 0.6) * 0.85
      const grad = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius)
      grad.addColorStop(0, `rgba(0,0,0,${alpha})`)
      grad.addColorStop(1, 'rgba(0,0,0,0)')
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.arc(cx, cy, radius, 0, Math.PI * 2)
      ctx.fill()
    }

    // 2) Colorize: map accumulated alpha through a blue->green->yellow->red ramp.
    const img = ctx.getImageData(0, 0, canvasWidth, canvasHeight)
    const data = img.data
    for (let i = 0; i < data.length; i += 4) {
      const a = data[i + 3]
      if (a === 0) continue
      const t = Math.min(a / 255, 1)
      const [r, g, b] = heatColor(t)
      data[i] = r
      data[i + 1] = g
      data[i + 2] = b
      // Keep a visible but translucent overlay.
      data[i + 3] = Math.round(Math.min(t * 1.1, 1) * 220)
    }
    ctx.putImageData(img, 0, 0)
  }, [points, maxCount, bucket, scale, canvasWidth, canvasHeight])

  return (
    <div ref={containerRef} className="w-full">
      <div
        className="relative mx-auto overflow-hidden rounded-lg border border-border bg-muted/30"
        style={{ width: canvasWidth || '100%', height: canvasHeight || undefined }}
      >
        {/* Neutral framed "viewport" surface (no screenshot available). */}
        <div className="pointer-events-none absolute inset-0 bg-[repeating-linear-gradient(45deg,transparent,transparent_10px,rgba(127,127,127,0.04)_10px,rgba(127,127,127,0.04)_20px)]" />
        <canvas
          ref={canvasRef}
          width={canvasWidth}
          height={canvasHeight}
          className="relative block"
        />
      </div>
    </div>
  )
}

/** Blue -> cyan -> green -> yellow -> red ramp for t in [0,1]. */
function heatColor(t: number): [number, number, number] {
  const stops: Array<[number, [number, number, number]]> = [
    [0.0, [0, 0, 255]],
    [0.25, [0, 255, 255]],
    [0.5, [0, 255, 0]],
    [0.75, [255, 255, 0]],
    [1.0, [255, 0, 0]],
  ]
  for (let i = 0; i < stops.length - 1; i++) {
    const [t0, c0] = stops[i]
    const [t1, c1] = stops[i + 1]
    if (t >= t0 && t <= t1) {
      const f = (t - t0) / (t1 - t0)
      return [
        Math.round(c0[0] + (c1[0] - c0[0]) * f),
        Math.round(c0[1] + (c1[1] - c0[1]) * f),
        Math.round(c0[2] + (c1[2] - c0[2]) * f),
      ]
    }
  }
  return stops[stops.length - 1][1]
}

// ---- scroll milestone bars -------------------------------------------------

function depthColor(depth: number): string {
  switch (depth) {
    case 25:
      return 'bg-emerald-500'
    case 50:
      return 'bg-lime-500'
    case 75:
      return 'bg-amber-500'
    default:
      return 'bg-rose-500'
  }
}

// ---- page ------------------------------------------------------------------

export function Heatmaps() {
  const { selectedDomainId } = useDomainStore()
  const { dateRange, setDateRange } = useDateRangeStore()
  const [selectedPage, setSelectedPage] = useState<string | null>(null)

  const {
    data: pages,
    isLoading: pagesLoading,
    error: pagesError,
  } = useHeatmapPages()

  // Default to the busiest page (the endpoint returns them busiest-first).
  useEffect(() => {
    if (!selectedPage && pages && pages.length > 0) {
      setSelectedPage(pages[0].path)
    }
  }, [pages, selectedPage])

  // Reset the selection when the domain changes so we don't keep a stale page.
  useEffect(() => {
    setSelectedPage(null)
  }, [selectedDomainId])

  const {
    data: clicks,
    isLoading: clicksLoading,
    error: clicksError,
  } = useClickHeatmap(selectedPage)

  const {
    data: scroll,
    isLoading: scrollLoading,
    error: scrollError,
  } = useScrollHeatmap(selectedPage)

  const sortedMilestones = useMemo(
    () =>
      (scroll?.milestones ?? [])
        .slice()
        .sort((a, b) => a.depth - b.depth),
    [scroll]
  )

  if (!selectedDomainId) {
    return (
      <div className="p-6">
        <div className="text-center py-12 text-muted-foreground">
          Select a property to view heatmaps
        </div>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6 overflow-y-auto h-full">
      {/* Header */}
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Flame className="h-6 w-6" />
            Heatmaps
          </h1>
          <p className="text-muted-foreground text-sm">
            Where visitors click and how far they scroll
          </p>
        </div>
        <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} />
      </div>

      {/* Page selector */}
      <div className="flex items-center gap-3 flex-wrap">
        <span className="text-sm font-medium text-muted-foreground">Page</span>
        {pagesLoading ? (
          <Skeleton className="h-9 w-72" />
        ) : (
          <Select
            value={selectedPage ?? undefined}
            onValueChange={setSelectedPage}
            disabled={!pages || pages.length === 0}
          >
            <SelectTrigger className="w-full max-w-md">
              <SelectValue placeholder="Select a page" />
            </SelectTrigger>
            <SelectContent>
              {(pages ?? []).map((p) => (
                <SelectItem key={p.path} value={p.path}>
                  <span className="inline-flex w-full items-center justify-between gap-4">
                    <span className="truncate">{p.path}</span>
                    <span className="text-xs text-muted-foreground shrink-0">
                      {formatNumber(p.pageviews)} views
                    </span>
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {pagesError ? (
        <Card className="p-6">
          <div className="flex items-center gap-2 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load pages</span>
          </div>
        </Card>
      ) : !pagesLoading && (!pages || pages.length === 0) ? (
        <Card className="p-6">
          <div className="text-center py-8 text-muted-foreground">
            No page data available for this range yet.
          </div>
        </Card>
      ) : (
        <div className="grid gap-6 lg:grid-cols-3">
          {/* Click map */}
          <Card className="p-6 lg:col-span-2">
            <div className="flex items-center justify-between gap-4 flex-wrap mb-4">
              <div className="flex items-center gap-2">
                <MousePointerClick className="h-5 w-5 text-muted-foreground" />
                <h2 className="text-lg font-semibold">Click Map</h2>
              </div>
              {clicks && (
                <div className="text-sm text-muted-foreground">
                  {formatNumber(clicks.total_clicks)} clicks
                </div>
              )}
            </div>

            {clicksError ? (
              <div className="flex items-center gap-2 text-destructive py-8 justify-center">
                <AlertCircle className="h-5 w-5" />
                <span>Failed to load click data</span>
              </div>
            ) : clicksLoading ? (
              <Skeleton className="h-80 w-full" />
            ) : !clicks || clicks.points.length === 0 ? (
              <div className="text-center py-16 text-muted-foreground">
                No clicks recorded for this page in the selected range.
              </div>
            ) : (
              <>
                <ClickHeatmapCanvas
                  points={clicks.points}
                  maxX={clicks.max_x}
                  maxY={clicks.max_y}
                  maxCount={clicks.max_count}
                  bucket={clicks.bucket}
                />
                <div className="mt-3 flex items-center justify-between gap-4 flex-wrap">
                  <p className="text-xs text-muted-foreground max-w-prose">
                    Viewport-relative click-density map. Positions are based on
                    where visitors clicked within the browser viewport (no page
                    screenshot is captured). Brighter, warmer areas received more
                    clicks.
                  </p>
                  <div className="flex items-center gap-2 shrink-0">
                    <span className="text-xs text-muted-foreground">Less</span>
                    <div className="h-2 w-28 rounded-full bg-gradient-to-r from-blue-500 via-green-500 via-yellow-400 to-red-500" />
                    <span className="text-xs text-muted-foreground">More</span>
                  </div>
                </div>
              </>
            )}
          </Card>

          {/* Scroll map */}
          <Card className="p-6">
            <div className="flex items-center gap-2 mb-4">
              <ArrowDownWideNarrow className="h-5 w-5 text-muted-foreground" />
              <h2 className="text-lg font-semibold">Scroll Depth</h2>
            </div>

            {scrollError ? (
              <div className="flex items-center gap-2 text-destructive py-8 justify-center">
                <AlertCircle className="h-5 w-5" />
                <span>Failed to load scroll data</span>
              </div>
            ) : scrollLoading ? (
              <div className="space-y-4">
                <Skeleton className="h-16 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            ) : !scroll || scroll.pageviews === 0 ? (
              <div className="text-center py-16 text-muted-foreground">
                No scroll data recorded for this page.
              </div>
            ) : (
              <div className="space-y-5">
                <div className="rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">
                    Average max scroll depth
                  </p>
                  <p className="text-3xl font-bold">
                    {formatPct(scroll.avg_max_scroll)}
                  </p>
                  <p className="text-xs text-muted-foreground mt-1">
                    Across {formatNumber(scroll.pageviews)} pageviews
                  </p>
                </div>

                <div className="space-y-3">
                  {sortedMilestones.map((m) => (
                    <div key={m.depth}>
                      <div className="flex items-center justify-between text-sm mb-1">
                        <span className="font-medium">{m.depth}%</span>
                        <span className="text-muted-foreground">
                          {formatPct(m.pct)} ({formatNumber(m.reached)})
                        </span>
                      </div>
                      <div className="h-3 w-full rounded-full bg-muted overflow-hidden">
                        <div
                          className={`h-full rounded-full transition-all ${depthColor(
                            m.depth
                          )}`}
                          style={{ width: `${Math.min(Math.max(m.pct, 0), 100)}%` }}
                        />
                      </div>
                    </div>
                  ))}
                </div>

                <p className="text-xs text-muted-foreground">
                  Percentage of pageviews that scrolled at least this far down
                  the page.
                </p>
              </div>
            )}
          </Card>
        </div>
      )}
    </div>
  )
}
