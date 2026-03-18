import { useState } from 'react'
import type { MigrateJob } from '../../../lib/types'
import { useRollbackJob, useCancelJob } from '../../../hooks/useMigrate'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '../../../components/ui/button'
import { Badge } from '../../../components/ui/badge'
import { Progress } from '../../../components/ui/progress'
import { toast } from 'sonner'

interface Props {
  job: MigrateJob
}

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  running: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
  completed: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  failed: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
  cancelled: 'bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200',
  rolled_back: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
}

const sourceLabels: Record<string, string> = {
  ga4_bigquery: 'GA4 BigQuery',
  ga4_csv: 'GA4 CSV',
  plausible: 'Plausible',
  matomo: 'Matomo',
  umami: 'Umami',
  csv: 'Generic CSV',
  gtm: 'GTM',
}

export function JobCard({ job }: Props) {
  const queryClient = useQueryClient()
  const rollback = useRollbackJob()
  const cancel = useCancelJob()
  const [confirmRollback, setConfirmRollback] = useState(false)

  const progress = job.rows_total > 0
    ? Math.round((job.rows_imported / job.rows_total) * 100)
    : 0

  const handleRollback = () => {
    if (!confirmRollback) {
      setConfirmRollback(true)
      return
    }
    rollback.mutate(job.id, {
      onSuccess: () => {
        toast.success('Import rolled back')
        queryClient.invalidateQueries({ queryKey: ['migrate', 'jobs'] })
        setConfirmRollback(false)
      },
      onError: (err) => toast.error(err.message),
    })
  }

  const handleCancel = () => {
    cancel.mutate(job.id, {
      onSuccess: () => {
        toast.success('Import cancelled')
        queryClient.invalidateQueries({ queryKey: ['migrate', 'jobs'] })
      },
      onError: (err) => toast.error(err.message),
    })
  }

  return (
    <div className="border rounded-lg p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className="font-medium text-sm">{sourceLabels[job.source] || job.source}</span>
            <Badge variant="outline" className={statusColors[job.status]}>
              {job.status.replace('_', ' ')}
            </Badge>
          </div>
          <p className="text-xs text-muted-foreground truncate">{job.domain} — {job.file_name}</p>
          <p className="text-xs text-muted-foreground mt-1">
            {job.rows_imported.toLocaleString()} imported
            {job.rows_skipped > 0 && `, ${job.rows_skipped.toLocaleString()} skipped`}
            {job.rows_total > 0 && ` of ${job.rows_total.toLocaleString()}`}
          </p>
          {job.error_message && (
            <p className="text-xs text-destructive mt-1">{job.error_message}</p>
          )}
        </div>
        <div className="flex gap-1 shrink-0">
          {job.status === 'running' && (
            <Button variant="outline" size="sm" onClick={handleCancel} disabled={cancel.isPending}>
              Cancel
            </Button>
          )}
          {job.status === 'completed' && (
            <Button
              variant={confirmRollback ? 'destructive' : 'outline'}
              size="sm"
              onClick={handleRollback}
              disabled={rollback.isPending}
            >
              {confirmRollback ? 'Confirm Rollback' : 'Rollback'}
            </Button>
          )}
        </div>
      </div>

      {job.status === 'running' && (
        <Progress value={progress} className="h-1 mt-3" />
      )}
    </div>
  )
}
