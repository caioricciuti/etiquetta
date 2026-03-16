import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { fetchAPI } from '../lib/api'
import type { TMContainer, TMTag, TMTrigger, TMVariable, TMSnapshot } from '../lib/types'

// Containers
export function useContainers() {
  return useQuery({
    queryKey: ['tm', 'containers'],
    queryFn: () => fetchAPI<TMContainer[]>('/api/tagmanager/containers'),
    staleTime: 60_000,
  })
}

export function useContainer(id: string | undefined) {
  return useQuery({
    queryKey: ['tm', 'containers', id],
    queryFn: () => fetchAPI<TMContainer>(`/api/tagmanager/containers/${id}`),
    enabled: !!id,
  })
}

export function useCreateContainer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { domain_id: string; name: string }) =>
      fetchAPI<TMContainer>('/api/tagmanager/containers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'containers'] })
      toast.success('Container created')
    },
    onError: (err) => toast.error('Failed to create container', { description: err.message }),
  })
}

export function useDeleteContainer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI(`/api/tagmanager/containers/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'containers'] })
      toast.success('Container deleted')
    },
    onError: (err) => toast.error('Failed to delete container', { description: err.message }),
  })
}

export function usePublishContainer(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      fetchAPI(`/api/tagmanager/containers/${containerId}/publish`, { method: 'POST' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm'] })
      toast.success('Container published')
    },
    onError: (err) => toast.error('Failed to publish', { description: err.message }),
  })
}

export function useContainerVersions(containerId: string | undefined) {
  return useQuery({
    queryKey: ['tm', 'versions', containerId],
    queryFn: () => fetchAPI<TMSnapshot[]>(`/api/tagmanager/containers/${containerId}/versions`),
    enabled: !!containerId,
  })
}

export function useRollbackContainer(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (version: number) =>
      fetchAPI(`/api/tagmanager/containers/${containerId}/rollback/${version}`, { method: 'POST' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm'] })
      toast.success('Rolled back successfully')
    },
    onError: (err) => toast.error('Failed to rollback', { description: err.message }),
  })
}

// Tags
export function useTags(containerId: string | undefined) {
  return useQuery({
    queryKey: ['tm', 'tags', containerId],
    queryFn: () => fetchAPI<TMTag[]>(`/api/tagmanager/containers/${containerId}/tags`),
    enabled: !!containerId,
  })
}

export function useCreateTag(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Omit<TMTag, 'id' | 'container_id' | 'version' | 'created_at' | 'updated_at'>) =>
      fetchAPI<TMTag>(`/api/tagmanager/containers/${containerId}/tags`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'tags', containerId] })
      toast.success('Tag created')
    },
    onError: (err) => toast.error('Failed to create tag', { description: err.message }),
  })
}

export function useUpdateTag(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & Partial<TMTag>) =>
      fetchAPI<TMTag>(`/api/tagmanager/containers/${containerId}/tags/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'tags', containerId] })
      toast.success('Tag updated')
    },
    onError: (err) => toast.error('Failed to update tag', { description: err.message }),
  })
}

export function useDeleteTag(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI(`/api/tagmanager/containers/${containerId}/tags/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'tags', containerId] })
      toast.success('Tag deleted')
    },
    onError: (err) => toast.error('Failed to delete tag', { description: err.message }),
  })
}

// Triggers
export function useTriggers(containerId: string | undefined) {
  return useQuery({
    queryKey: ['tm', 'triggers', containerId],
    queryFn: () => fetchAPI<TMTrigger[]>(`/api/tagmanager/containers/${containerId}/triggers`),
    enabled: !!containerId,
  })
}

export function useCreateTrigger(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Omit<TMTrigger, 'id' | 'container_id' | 'created_at' | 'updated_at'>) =>
      fetchAPI<TMTrigger>(`/api/tagmanager/containers/${containerId}/triggers`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'triggers', containerId] })
      toast.success('Trigger created')
    },
    onError: (err) => toast.error('Failed to create trigger', { description: err.message }),
  })
}

export function useUpdateTrigger(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & Partial<TMTrigger>) =>
      fetchAPI<TMTrigger>(`/api/tagmanager/containers/${containerId}/triggers/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'triggers', containerId] })
      toast.success('Trigger updated')
    },
    onError: (err) => toast.error('Failed to update trigger', { description: err.message }),
  })
}

export function useDeleteTrigger(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI(`/api/tagmanager/containers/${containerId}/triggers/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'triggers', containerId] })
      toast.success('Trigger deleted')
    },
    onError: (err) => toast.error('Failed to delete trigger', { description: err.message }),
  })
}

// Variables
export function useVariables(containerId: string | undefined) {
  return useQuery({
    queryKey: ['tm', 'variables', containerId],
    queryFn: () => fetchAPI<TMVariable[]>(`/api/tagmanager/containers/${containerId}/variables`),
    enabled: !!containerId,
  })
}

export function useCreateVariable(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Omit<TMVariable, 'id' | 'container_id' | 'created_at' | 'updated_at'>) =>
      fetchAPI<TMVariable>(`/api/tagmanager/containers/${containerId}/variables`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'variables', containerId] })
      toast.success('Variable created')
    },
    onError: (err) => toast.error('Failed to create variable', { description: err.message }),
  })
}

export function useUpdateVariable(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & Partial<TMVariable>) =>
      fetchAPI<TMVariable>(`/api/tagmanager/containers/${containerId}/variables/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'variables', containerId] })
      toast.success('Variable updated')
    },
    onError: (err) => toast.error('Failed to update variable', { description: err.message }),
  })
}

export function useDeleteVariable(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      fetchAPI(`/api/tagmanager/containers/${containerId}/variables/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm', 'variables', containerId] })
      toast.success('Variable deleted')
    },
    onError: (err) => toast.error('Failed to delete variable', { description: err.message }),
  })
}

// Container Import/Export
export function useExportContainer(containerId: string | undefined) {
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(`/api/tagmanager/containers/${containerId}/export`, {
        credentials: 'include',
      })
      if (!res.ok) throw new Error('Export failed')
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `container-${containerId}-export.json`
      a.click()
      URL.revokeObjectURL(url)
    },
    onSuccess: () => toast.success('Container exported'),
    onError: (err) => toast.error('Export failed', { description: err.message }),
  })
}

export function useImportContainer(containerId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: unknown) =>
      fetchAPI(`/api/tagmanager/containers/${containerId}/import`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tm'] })
      toast.success('Container imported successfully')
    },
    onError: (err) => toast.error('Import failed', { description: err.message }),
  })
}

// Preview/Debug
export function usePreviewToken(containerId: string | undefined) {
  return useMutation({
    mutationFn: () =>
      fetchAPI<{ token: string; site_id: string; domain: string }>(
        `/api/tagmanager/containers/${containerId}/preview-token`,
        { method: 'POST' }
      ),
    onError: (err) => toast.error('Failed to generate preview', { description: err.message }),
  })
}
