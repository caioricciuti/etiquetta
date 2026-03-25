import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { useDomainStore } from '../stores/useDomainStore'
import { useFilterStore } from '../stores/useFilterStore'
import { useDomains } from './useDomains'
import type { DateRange } from 'react-day-picker'
import type {
  OverviewStats,
  TimeseriesPoint,
  TopPage,
  Referrer,
  GeoData,
  MapPoint,
  DeviceData,
  BrowserData,
  WebVitals,
  ErrorSummary,
  Campaign,
  CustomEvent,
  OutboundLink,
  BotData,
  FraudSummary,
  SourceQuality,
  AdFraudCampaign,
  ComparisonResponse,
  AdConnection,
  AdProvider,
  AdSpendPoint,
  AdAttributionRow,
  EventsSummaryResponse,
  EventTimeseriesPoint,
  EventPropsResponse,
} from '../lib/types'

function useAnalyticsParams() {
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
  if (filters.page) params.set('page', filters.page)
  if (filters.referrer) params.set('referrer', filters.referrer)
  if (filters.bot_filter) params.set('bot_filter', filters.bot_filter)

  return { qs: params.toString(), enabled: !domainsLoading }
}

export function useOverview() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'overview', qs],
    queryFn: () => fetchAPI<OverviewStats>(`/api/stats/overview?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useTimeseries() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'timeseries', qs],
    queryFn: () => fetchAPI<TimeseriesPoint[]>(`/api/stats/timeseries?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useTopPages() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'pages', qs],
    queryFn: () => fetchAPI<TopPage[]>(`/api/stats/pages?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useReferrers() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'referrers', qs],
    queryFn: () => fetchAPI<Referrer[]>(`/api/stats/referrers?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useGeo() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'geo', qs],
    queryFn: () => fetchAPI<GeoData[]>(`/api/stats/geo?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useMapData() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'map', qs],
    queryFn: () => fetchAPI<MapPoint[]>(`/api/stats/map?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
    meta: { silent: true },
  })
}

export function useDevices() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'devices', qs],
    queryFn: () => fetchAPI<DeviceData[]>(`/api/stats/devices?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useBrowsers() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'browsers', qs],
    queryFn: () => fetchAPI<BrowserData[]>(`/api/stats/browsers?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useVitals(featureEnabled = true) {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'vitals', qs],
    queryFn: () => fetchAPI<WebVitals>(`/api/stats/vitals?${qs}`),
    enabled: enabled && featureEnabled,
    retry: false,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}

export function useErrors(featureEnabled = true) {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'errors', qs],
    queryFn: () => fetchAPI<ErrorSummary[]>(`/api/stats/errors?${qs}`),
    enabled: enabled && featureEnabled,
    retry: false,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}

export function useCampaigns() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'campaigns', qs],
    queryFn: () => fetchAPI<Campaign[]>(`/api/stats/campaigns?${qs}`),
    enabled,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}

export function useCustomEvents() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'events', qs],
    queryFn: () => fetchAPI<CustomEvent[]>(`/api/stats/events?${qs}`),
    enabled,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}

export function useOutboundLinks() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'outbound', qs],
    queryFn: () => fetchAPI<OutboundLink[]>(`/api/stats/outbound?${qs}`),
    enabled,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}

// --- Events Page ---

export function useEventsSummary(eventType?: string) {
  const { qs, enabled } = useAnalyticsParams()
  const params = new URLSearchParams(qs)
  if (eventType) params.set('event_type', eventType)
  const fullQs = params.toString()
  return useQuery({
    queryKey: ['stats', 'events', 'summary', fullQs],
    queryFn: () => fetchAPI<EventsSummaryResponse>(`/api/stats/events/summary?${fullQs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useEventsTimeseries(eventName?: string, eventType?: string) {
  const { qs, enabled } = useAnalyticsParams()
  const params = new URLSearchParams(qs)
  if (eventName) params.set('event_name', eventName)
  if (eventType) params.set('event_type', eventType)
  const fullQs = params.toString()
  return useQuery({
    queryKey: ['stats', 'events', 'timeseries', fullQs],
    queryFn: () => fetchAPI<EventTimeseriesPoint[]>(`/api/stats/events/timeseries?${fullQs}`),
    enabled: enabled && !!eventName,
    placeholderData: keepPreviousData,
  })
}

export function useEventsProps(eventName?: string) {
  const { qs, enabled } = useAnalyticsParams()
  const params = new URLSearchParams(qs)
  if (eventName) params.set('event_name', eventName)
  const fullQs = params.toString()
  return useQuery({
    queryKey: ['stats', 'events', 'props', fullQs],
    queryFn: () => fetchAPI<EventPropsResponse>(`/api/stats/events/props?${fullQs}`),
    enabled: enabled && !!eventName,
    placeholderData: keepPreviousData,
  })
}

// --- Period Comparison ---

export function useComparison(
  currentRange: DateRange | undefined,
  compareRange: DateRange | undefined,
  domain: string | undefined,
) {
  const params = new URLSearchParams()
  if (currentRange?.from && currentRange?.to) {
    params.set('start', currentRange.from.toISOString())
    params.set('end', currentRange.to.toISOString())
  }
  if (compareRange?.from && compareRange?.to) {
    params.set('compare_start', compareRange.from.toISOString())
    params.set('compare_end', compareRange.to.toISOString())
  }
  if (domain) params.set('domain', domain)
  const qs = params.toString()

  return useQuery({
    queryKey: ['stats', 'compare', qs],
    queryFn: () => fetchAPI<ComparisonResponse>(`/api/stats/compare?${qs}`),
    enabled: !!currentRange?.from && !!currentRange?.to,
    placeholderData: keepPreviousData,
  })
}

// --- Bot Analysis ---

function useBotParams() {
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
  if (filters.bot_filter) params.set('bot_filter', filters.bot_filter)

  return { qs: params.toString(), enabled: !domainsLoading }
}

export function useBotAnalysis() {
  const { qs, enabled } = useBotParams()
  return useQuery({
    queryKey: ['stats', 'bots', qs],
    queryFn: () => fetchAPI<BotData>(`/api/stats/bots?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

// --- Ad Fraud ---

export function useFraudSummary() {
  const { qs, enabled } = useBotParams()
  return useQuery({
    queryKey: ['stats', 'fraud', qs],
    queryFn: () => fetchAPI<FraudSummary>(`/api/stats/fraud?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useAvailableEventNames() {
  const { qs, enabled } = useBotParams()
  return useQuery({
    queryKey: ['fraud', 'event-names', qs],
    queryFn: () => fetchAPI<string[]>(`/api/stats/fraud/event-names?${qs}`),
    enabled,
    staleTime: 60_000,
  })
}

export function useSourceQuality() {
  const { qs, enabled } = useBotParams()
  return useQuery({
    queryKey: ['sources', 'quality', qs],
    queryFn: () => fetchAPI<SourceQuality[]>(`/api/sources/quality?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useAdFraudCampaigns() {
  return useQuery({
    queryKey: ['campaigns'],
    queryFn: () => fetchAPI<AdFraudCampaign[]>('/api/campaigns'),
  })
}

export function useCreateCampaign() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { name: string; cpc: number; cpm: number; budget: number }) =>
      fetchAPI<AdFraudCampaign>('/api/campaigns', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['campaigns'] }),
  })
}

export function useDeleteCampaign() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/campaigns/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['campaigns'] }),
  })
}

// --- Ad Platform Connections ---

export function useConnections(domainId?: string | null) {
  const params = domainId ? `?domain_id=${domainId}` : ''
  return useQuery({
    queryKey: ['connections', domainId ?? 'all'],
    queryFn: () => fetchAPI<AdConnection[]>(`/api/connections${params}`),
    retry: false,
    meta: { silent: true },
  })
}

export function useProviders() {
  return useQuery({
    queryKey: ['connections', 'providers'],
    queryFn: () => fetchAPI<AdProvider[]>('/api/connections/providers'),
    staleTime: 60 * 60 * 1000,
  })
}

export function useCreateConnection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { provider: string; name: string; account_id?: string; domain_id?: string; refresh_token?: string; config?: Record<string, string> }) =>
      fetchAPI<AdConnection>('/api/connections', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }),
  })
}

export function useDeleteConnection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/connections/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }),
  })
}

export function useSyncConnection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/connections/${id}/sync`, { method: 'POST' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connections'] })
      qc.invalidateQueries({ queryKey: ['stats', 'ad-spend'] })
      qc.invalidateQueries({ queryKey: ['stats', 'ad-attribution'] })
    },
  })
}

export function useUpdateConnectionToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, refresh_token }: { id: string; refresh_token: string }) =>
      fetchAPI(`/api/connections/${id}/tokens`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }),
  })
}

export function useAdSpend() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'ad-spend', qs],
    queryFn: () => fetchAPI<AdSpendPoint[]>(`/api/stats/ad-spend?${qs}`),
    enabled,
    retry: false,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}

export function useAdAttribution() {
  const { qs, enabled } = useAnalyticsParams()
  return useQuery({
    queryKey: ['stats', 'ad-attribution', qs],
    queryFn: () => fetchAPI<AdAttributionRow[]>(`/api/stats/ad-attribution?${qs}`),
    enabled,
    retry: false,
    meta: { silent: true },
    placeholderData: keepPreviousData,
  })
}
