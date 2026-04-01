import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import { useDomainStore } from '../stores/useDomainStore'

export interface ShareLink {
  id: string
  token: string
  domain_id: string
  name: string
  created_by: string
  created_at: number
  expires_at: number | null
}

export function useShareLinks() {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  return useQuery({
    queryKey: ['share-links', selectedDomainId],
    queryFn: () => fetchAPI<ShareLink[]>(`/api/share-links?domain_id=${selectedDomainId}`),
    enabled: !!selectedDomainId,
  })
}

export function useCreateShareLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { domain_id: string; name?: string; expires_in_days?: number }) =>
      fetchAPI<ShareLink>('/api/share-links', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['share-links'] }),
  })
}

export function useDeleteShareLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI<void>(`/api/share-links/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['share-links'] }),
  })
}
