import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { useDomainStore } from '../stores/useDomainStore'

export interface Goal {
  id: string
  domain_id: string
  name: string
  kind: 'pageview' | 'custom' | 'outbound'
  match_value: string
  value_cents: number
  currency: string
  created_by: string
  created_at: number
  updated_at: number
}

export interface GoalStat {
  goal_id: string
  name: string
  kind: string
  completions: number
  unique_visitors: number
  conversion_rate: number
  revenue_cents: number
  currency: string
}

export interface CustomEventName {
  name: string
  count: number
}

export function useGoals() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['goals', domainId],
    queryFn: () => fetchAPI<Goal[]>(`/api/goals?domain_id=${domainId}`),
    enabled: !!domainId,
    placeholderData: keepPreviousData,
  })
}

export function useGoalStats() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  const { dateRange } = useDateRangeStore()
  const params = new URLSearchParams()
  if (domainId) params.set('domain_id', domainId)
  if (dateRange?.from) params.set('start', dateRange.from.toISOString())
  if (dateRange?.to) params.set('end', dateRange.to.toISOString())
  return useQuery({
    queryKey: ['goals', 'stats', domainId, params.toString()],
    queryFn: () => fetchAPI<GoalStat[]>(`/api/stats/goals?${params}`),
    enabled: !!domainId && !!dateRange?.from && !!dateRange?.to,
    placeholderData: keepPreviousData,
  })
}

export function useCreateGoal() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Goal>) =>
      fetchAPI<Goal>('/api/goals', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['goals'] }),
  })
}

export function useDeleteGoal() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/goals/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['goals'] }),
  })
}

export function useCustomEventNames() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['custom-events', domainId],
    queryFn: () => fetchAPI<CustomEventName[]>(`/api/stats/custom-events?domain_id=${domainId}`),
    enabled: !!domainId,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60 * 1000,
  })
}
