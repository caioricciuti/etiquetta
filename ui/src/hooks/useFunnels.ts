import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { useDomainStore } from '../stores/useDomainStore'
import type { Funnel, FunnelStep, FunnelMetricsResponse } from '../lib/types'

export function useFunnels() {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['funnels', selectedDomainId],
    queryFn: () => fetchAPI<Funnel[]>(`/api/funnels?domain_id=${selectedDomainId}`),
    enabled: !!selectedDomainId,
    placeholderData: keepPreviousData,
  })
}

export function useCreateFunnel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { domain_id: string; name: string; description?: string; steps: FunnelStep[] }) =>
      fetchAPI<Funnel>('/api/funnels', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['funnels'] }),
  })
}

export function useUpdateFunnel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string; name: string; description?: string; steps: FunnelStep[] }) =>
      fetchAPI<void>(`/api/funnels/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['funnels'] }),
  })
}

export function useDeleteFunnel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/funnels/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['funnels'] }),
  })
}

export function useFunnelMetrics(funnelId: string | null) {
  const { dateRange } = useDateRangeStore()
  const params = new URLSearchParams()
  if (dateRange?.from) params.set('start', dateRange.from.toISOString())
  if (dateRange?.to) params.set('end', dateRange.to.toISOString())

  return useQuery({
    queryKey: ['funnels', 'metrics', funnelId, params.toString()],
    queryFn: () => fetchAPI<FunnelMetricsResponse>(`/api/funnels/${funnelId}/metrics?${params}`),
    enabled: !!funnelId && !!dateRange?.from && !!dateRange?.to,
    placeholderData: keepPreviousData,
  })
}
