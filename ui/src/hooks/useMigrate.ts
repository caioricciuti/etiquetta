import { useQuery, useMutation } from '@tanstack/react-query'
import { fetchAPI } from '../lib/api'
import type { MigrateAnalysis, MigrateJob, GtmConvertResult } from '../lib/types'

export function useAnalyzeMigration() {
  return useMutation({
    mutationFn: async ({ file, source }: { file: File; source?: string }) => {
      const formData = new FormData()
      formData.append('file', file)
      if (source) formData.append('source', source)

      const res = await fetch('/api/migrate/analyze', {
        method: 'POST',
        body: formData,
        credentials: 'include',
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Upload failed' }))
        throw new Error(err.error || 'Upload failed')
      }
      return res.json() as Promise<MigrateAnalysis>
    },
  })
}

export function useStartMigration() {
  return useMutation({
    mutationFn: async (params: {
      analysis_id: string
      source: string
      domain: string
      column_mapping: Record<string, string>
    }) => {
      return fetchAPI<{ job_id: string }>('/api/migrate/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(params),
      })
    },
  })
}

export function useMigrateJobs() {
  return useQuery({
    queryKey: ['migrate', 'jobs'],
    queryFn: () => fetchAPI<MigrateJob[]>('/api/migrate/jobs'),
  })
}

export function useMigrateJobStatus(jobId: string | null) {
  return useQuery({
    queryKey: ['migrate', 'jobs', jobId],
    queryFn: () => fetchAPI<MigrateJob>(`/api/migrate/jobs/${jobId}`),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (status === 'pending' || status === 'running') return 2000
      return false
    },
  })
}

export function useRollbackJob() {
  return useMutation({
    mutationFn: async (jobId: string) => {
      return fetchAPI<{ status: string }>(`/api/migrate/jobs/${jobId}`, {
        method: 'DELETE',
      })
    },
  })
}

export function useCancelJob() {
  return useMutation({
    mutationFn: async (jobId: string) => {
      return fetchAPI<{ status: string }>(`/api/migrate/jobs/${jobId}/cancel`, {
        method: 'POST',
      })
    },
  })
}

export function useGtmConvert() {
  return useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData()
      formData.append('file', file)

      const res = await fetch('/api/migrate/gtm/convert', {
        method: 'POST',
        body: formData,
        credentials: 'include',
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Conversion failed' }))
        throw new Error(err.error || 'Conversion failed')
      }
      return res.json() as Promise<GtmConvertResult>
    },
  })
}
