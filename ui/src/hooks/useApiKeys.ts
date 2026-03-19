import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { fetchAPI } from '../lib/api'
import type { ApiKey } from '../lib/types'

interface CreateApiKeyResponse {
  id: string
  name: string
  key: string
  key_prefix: string
  created_at: number
}

export function useApiKeys() {
  return useQuery({
    queryKey: ['api-keys'],
    queryFn: async () => {
      const data = await fetchAPI<{ keys: ApiKey[] }>('/api/tokens')
      return data.keys
    },
    staleTime: 60_000,
  })
}

export function useCreateApiKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      fetchAPI<CreateApiKeyResponse>('/api/tokens', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: (err) => toast.error('Failed to create API key', { description: err.message }),
  })
}

export function useRevokeApiKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI(`/api/tokens/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
      toast.success('API key revoked')
    },
    onError: (err) => toast.error('Failed to revoke API key', { description: err.message }),
  })
}
