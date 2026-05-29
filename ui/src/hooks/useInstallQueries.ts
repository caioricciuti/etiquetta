import { useQuery } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'

export interface InstallRecentEvent {
  timestamp: number
  event_type: string
  event_name: string
  path: string
  geo_country: string
  geo_city: string
  device_type: string
  browser_name: string
  os_name: string
  referrer_type: string
  bot_category: string
  bot_score: number
  is_bot: boolean
  has_click: boolean
  has_scroll: boolean
}

export interface InstallStatus {
  domain: string
  total_events: number
  events_last_30m: number
  installed: boolean
  receiving: boolean
  last_event_at: number | null
  recent: InstallRecentEvent[]
}

/**
 * Polls the install verification endpoint for the given domain.
 * Refetches every 3s so the page updates live as events arrive.
 * Disabled when no domain is provided.
 */
export function useInstallStatus(domain: string | undefined) {
  return useQuery({
    queryKey: ['install', 'status', domain ?? ''],
    queryFn: () =>
      fetchAPI<InstallStatus>(
        `/api/install/status?domain=${encodeURIComponent(domain!)}`,
      ),
    enabled: !!domain,
    refetchInterval: 3000,
  })
}
