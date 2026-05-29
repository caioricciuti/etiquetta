import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { useDomainStore } from '../stores/useDomainStore'
import { useFilterStore } from '../stores/useFilterStore'
import { useDomains } from './useDomains'

// ============ Types ============

export interface HeatmapPage {
  path: string
  pageviews: number
  clicks: number
}

export interface HeatmapPoint {
  x: number
  y: number
  count: number
}

export interface ClickHeatmap {
  points: HeatmapPoint[]
  max_x: number
  max_y: number
  max_count: number
  total_clicks: number
  bucket: number
}

export interface ScrollMilestone {
  depth: 25 | 50 | 75 | 100
  reached: number
  pct: number
}

export interface ScrollHeatmap {
  pageviews: number
  avg_max_scroll: number
  milestones: ScrollMilestone[]
}

// ============ Query Params ============
// Mirrors useAnalyticsParams in useAnalyticsQueries.ts so that the heatmaps page
// respects the global domain selector, date range and filters (incl. bot_filter)
// exactly like every other stats page.

function useHeatmapParams() {
  const { dateRange } = useDateRangeStore()
  const { selectedDomainId } = useDomainStore()
  const { filters } = useFilterStore()
  const { data: domains, isLoading: domainsLoading } = useDomains()
  const selectedDomain = domains?.find((d) => d.id === selectedDomainId)

  const params = new URLSearchParams()
  if (dateRange?.from && dateRange?.to) {
    params.set('start', dateRange.from.toISOString())
    params.set('end', dateRange.to.toISOString())
  } else {
    params.set('days', '7')
  }
  if (selectedDomain) params.set('domain', selectedDomain.domain)
  if (filters.country) params.set('country', filters.country)
  if (filters.browser) params.set('browser', filters.browser)
  if (filters.device) params.set('device', filters.device)
  if (filters.referrer) params.set('referrer', filters.referrer)
  if (filters.bot_filter) params.set('bot_filter', filters.bot_filter)

  return { params, enabled: !domainsLoading && !!selectedDomain }
}

// ============ Hooks ============

/** Top pages with click/pageview counts, used to populate the page selector. */
export function useHeatmapPages() {
  const { params, enabled } = useHeatmapParams()
  const qs = params.toString()
  return useQuery({
    queryKey: ['heatmap', 'pages', qs],
    queryFn: () => fetchAPI<HeatmapPage[]>(`/api/stats/heatmap/pages?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

/** Bucketed click-density points for a given page. */
export function useClickHeatmap(page: string | null, bucket?: number) {
  const { params, enabled } = useHeatmapParams()
  const p = new URLSearchParams(params)
  if (page) p.set('page', page)
  if (bucket) p.set('bucket', String(bucket))
  const qs = p.toString()
  return useQuery({
    queryKey: ['heatmap', 'clicks', qs],
    queryFn: () => fetchAPI<ClickHeatmap>(`/api/stats/heatmap/clicks?${qs}`),
    enabled: enabled && !!page,
    placeholderData: keepPreviousData,
  })
}

/** Scroll-depth milestones for a given page. */
export function useScrollHeatmap(page: string | null) {
  const { params, enabled } = useHeatmapParams()
  const p = new URLSearchParams(params)
  if (page) p.set('page', page)
  const qs = p.toString()
  return useQuery({
    queryKey: ['heatmap', 'scroll', qs],
    queryFn: () => fetchAPI<ScrollHeatmap>(`/api/stats/heatmap/scroll?${qs}`),
    enabled: enabled && !!page,
    placeholderData: keepPreviousData,
  })
}
