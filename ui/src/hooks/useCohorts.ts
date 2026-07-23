import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { useDomainStore } from '../stores/useDomainStore'

export interface CohortDefinition {
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  referrer_contains?: string
  path_contains?: string
  country?: string
}

export interface Cohort {
  id: string
  domain_id: string
  name: string
  description: string
  definition: CohortDefinition
  created_by: string
  created_at: number
  updated_at: number
}

export interface RetentionCohort {
  date: string
  size: number
  returns: number[]
  rates: number[]
}

export interface RetentionResponse {
  days: number
  cohorts: RetentionCohort[]
}

export function useCohorts() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['cohorts', domainId],
    queryFn: () => fetchAPI<Cohort[]>(`/api/cohorts?domain_id=${domainId}`),
    enabled: !!domainId,
    placeholderData: keepPreviousData,
  })
}

export function useCreateCohort() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Cohort>) =>
      fetchAPI<Cohort>('/api/cohorts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cohorts'] }),
  })
}

export function useUpdateCohort() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: Partial<Cohort> & { id: string }) =>
      fetchAPI<Cohort>(`/api/cohorts/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cohorts'] }),
  })
}

export function useDeleteCohort() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/cohorts/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cohorts'] }),
  })
}

export function useRetention(opts?: { days?: number; cohortId?: string }) {
  const domainId = useDomainStore(s => s.selectedDomainId)
  const { dateRange } = useDateRangeStore()
  const params = new URLSearchParams()
  if (domainId) params.set('domain_id', domainId)
  if (dateRange?.from) params.set('start', dateRange.from.toISOString())
  if (dateRange?.to) params.set('end', dateRange.to.toISOString())
  if (opts?.days) params.set('days', String(opts.days))
  if (opts?.cohortId) params.set('cohort_id', opts.cohortId)
  return useQuery({
    queryKey: ['retention', domainId, params.toString()],
    queryFn: () => fetchAPI<RetentionResponse>(`/api/stats/retention?${params}`),
    enabled: !!domainId && !!dateRange?.from && !!dateRange?.to,
    placeholderData: keepPreviousData,
  })
}
