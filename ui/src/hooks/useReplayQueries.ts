import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchAPI } from '@/lib/api'
import type { ReplayListResponse, ReplayData, ReplayStats, ReplaySettings } from '@/lib/types'

export function useReplays(params: {
  domain?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}) {
  const searchParams = new URLSearchParams()
  if (params.domain) searchParams.set('domain', params.domain)
  if (params.from) searchParams.set('from', params.from)
  if (params.to) searchParams.set('to', params.to)
  if (params.limit) searchParams.set('limit', String(params.limit))
  if (params.offset) searchParams.set('offset', String(params.offset))

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
