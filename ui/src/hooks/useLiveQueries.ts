import { useQuery } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'

export interface LiveActivePage {
  path: string
  visitors: number
  views: number
}

export interface LiveSource {
  source: string
  visitors: number
}

export interface LiveCountry {
  country: string
  visitors: number
}

export interface LiveSparklinePoint {
  minute: string
  views: number
  visitors: number
}

export interface LiveRecentEvent {
  timestamp: number
  event_type: string
  event_name: string
  path: string
  geo_country: string
  geo_city: string
  device_type: string
  browser_name: string
  referrer_type: string
}

export interface LiveStats {
  current_visitors: number
  active_sessions: number
  pageviews_5m: number
  active_pages: LiveActivePage[]
  live_sources: LiveSource[]
  live_countries: LiveCountry[]
  sparkline: LiveSparklinePoint[]
  recent: LiveRecentEvent[]
  window_minutes: number
  server_time: number
}

/**
 * Polls the live stats endpoint for the given domain.
 * Refetches every 5s so the page reflects who is on the site right now.
 * Disabled when no domain is provided. The endpoint does not use a date
 * range — only the domain and optional bot filter.
 */
export function useLiveStats(domain: string | undefined, botFilter?: string) {
  return useQuery({
    queryKey: ['stats', 'live', domain ?? '', botFilter ?? ''],
    queryFn: () => {
      const params = new URLSearchParams()
      params.set('domain', domain!)
      if (botFilter) params.set('bot_filter', botFilter)
      return fetchAPI<LiveStats>(`/api/stats/live?${params.toString()}`)
    },
    enabled: !!domain,
    refetchInterval: 5000,
  })
}
