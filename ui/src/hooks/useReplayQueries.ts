import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchAPI } from '@/lib/api'
import type { ReplayListResponse, ReplayData, ReplayStats, ReplaySettings, SessionEvent } from '@/lib/types'

export function useReplays(params: {
  domain?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
  device_type?: string
  browser_name?: string
  os_name?: string
  min_duration?: string
  max_duration?: string
}) {
  const searchParams = new URLSearchParams()
  if (params.domain) searchParams.set('domain', params.domain)
  if (params.from) searchParams.set('from', params.from)
  if (params.to) searchParams.set('to', params.to)
  if (params.limit) searchParams.set('limit', String(params.limit))
  if (params.offset) searchParams.set('offset', String(params.offset))
  if (params.device_type) searchParams.set('device_type', params.device_type)
  if (params.browser_name) searchParams.set('browser_name', params.browser_name)
  if (params.os_name) searchParams.set('os_name', params.os_name)
  if (params.min_duration) searchParams.set('min_duration', params.min_duration)
  if (params.max_duration) searchParams.set('max_duration', params.max_duration)

  return useQuery({
    queryKey: ['replays', params],
    queryFn: () => fetchAPI<ReplayListResponse>(`/api/replays?${searchParams.toString()}`),
  })
}

export function useReplay(sessionId: string | null) {
  return useQuery({
    queryKey: ['replay', sessionId],
    queryFn: () => fetchAPI<ReplayData>(`/api/replays/${sessionId}`),
    enabled: !!sessionId,
  })
}

export function useReplayStats() {
  return useQuery({
    queryKey: ['replay-stats'],
    queryFn: () => fetchAPI<ReplayStats>('/api/replays/stats'),
  })
}

export function useReplaySettings() {
  return useQuery({
    queryKey: ['replay-settings'],
    queryFn: () => fetchAPI<ReplaySettings>('/api/replays/settings'),
  })
}

export function useUpdateReplaySettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (settings: Partial<ReplaySettings>) =>
      fetchAPI<ReplaySettings>('/api/replays/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['replay-settings'] })
    },
  })
}

export function useSessionEvents(sessionId: string | null) {
  return useQuery({
    queryKey: ['session-events', sessionId],
    queryFn: () => fetchAPI<SessionEvent[]>(`/api/replays/${sessionId}/events`),
    enabled: !!sessionId,
  })
}

export function useDeleteReplay() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (sessionId: string) =>
      fetchAPI(`/api/replays/${sessionId}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['replays'] })
      queryClient.invalidateQueries({ queryKey: ['replay-stats'] })
    },
  })
}
