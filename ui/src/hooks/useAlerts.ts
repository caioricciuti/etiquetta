import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDomainStore } from '../stores/useDomainStore'

export interface Alert {
  id: string
  domain_id: string
  name: string
  metric: string
  comparator: 'gt' | 'lt'
  threshold: number
  window_minutes: number
  cooldown_minutes: number
  channels: string[]
  recipients: string[]
  enabled: boolean
  created_by: string
  created_at: number
  updated_at: number
}

export interface AlertFire {
  id: string
  alert_id: string
  domain_id: string
  fired_at: number
  metric_value: number
  threshold: number
  channels_sent: string[]
  error?: string
}

export interface Webhook {
  id: string
  domain_id: string
  name: string
  url: string
  secret: string
  events: string[]
  enabled: boolean
  created_by: string
  created_at: number
  updated_at: number
  last_fired_at?: number
  last_status?: number
  last_error?: string
}

export function useAlerts() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['alerts', domainId],
    queryFn: () => fetchAPI<Alert[]>(`/api/alerts?domain_id=${domainId}`),
    enabled: !!domainId,
    placeholderData: keepPreviousData,
  })
}

export function useAlertFires() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['alerts', 'fires', domainId],
    queryFn: () => fetchAPI<AlertFire[]>(`/api/alerts/fires?domain_id=${domainId}`),
    enabled: !!domainId,
    placeholderData: keepPreviousData,
  })
}

export function useCreateAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Alert>) =>
      fetchAPI<Alert>('/api/alerts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
}

export function useUpdateAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: Partial<Alert> & { id: string }) =>
      fetchAPI<void>(`/api/alerts/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
}

export function useDeleteAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/alerts/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
}

export function useWebhooks() {
  const domainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['webhooks', domainId],
    queryFn: () => fetchAPI<Webhook[]>(`/api/webhooks?domain_id=${domainId}`),
    enabled: !!domainId,
    placeholderData: keepPreviousData,
  })
}

export function useCreateWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Webhook>) =>
      fetchAPI<Webhook>('/api/webhooks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
}

export function useUpdateWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: Partial<Webhook> & { id: string }) =>
      fetchAPI<void>(`/api/webhooks/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
}

export function useDeleteWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/webhooks/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
}

export function useTestWebhook() {
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<{ success: boolean; message: string }>(`/api/webhooks/${id}/test`, { method: 'POST' }),
  })
}
