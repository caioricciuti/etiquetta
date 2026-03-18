import { useEffect } from 'react'
import type { MigrateSource } from '../../../lib/types'
import { useStartMigration, useMigrateJobStatus, useCancelJob } from '../../../hooks/useMigrate'
import { Button } from '../../../components/ui/button'
import { Progress } from '../../../components/ui/progress'

interface Props {
  analysisId: string
  source: MigrateSource
  domain: string
  mapping: Record<string, string>
  onJobStarted: (jobId: string) => void
  onComplete: () => void
}

export function StepImport({ analysisId, source, domain, mapping, onJobStarted, onComplete }: Props) {
  const startMutation = useStartMigration()
  const cancelMutation = useCancelJob()

  const jobId = startMutation.data?.job_id ?? null
  const { data: job } = useMigrateJobStatus(jobId)

  // Start the job on mount
  useEffect(() => {
    if (!startMutation.data && !startMutation.isPending) {
      startMutation.mutate({
        analysis_id: analysisId,
        source,
        domain,
        column_mapping: mapping,
      }, {
        onSuccess: (data) => onJobStarted(data.job_id),
      })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const progress = job && job.rows_total > 0
    ? Math.round((job.rows_imported / job.rows_total) * 100)
    : job?.status === 'completed' ? 100 : 0

  const isFinished = job && ['completed', 'failed', 'cancelled'].includes(job.status)

  useEffect(() => {
    if (job?.status === 'completed') {
      const timer = setTimeout(onComplete, 1500)
      return () => clearTimeout(timer)
    }
  }, [job?.status, onComplete])

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Importing</h2>

      {startMutation.isPending && (
        <p className="text-sm text-muted-foreground">Starting import...</p>
      )}

      {startMutation.isError && (
        <p className="text-sm text-destructive">{startMutation.error.message}</p>
      )}

      {job && (
        <div className="mt-4 space-y-3">
          <Progress value={progress} className="h-2" />
          <div className="flex justify-between text-sm">
            <span className="capitalize">{job.status}</span>
            <span>{job.rows_imported.toLocaleString()} / {job.rows_total > 0 ? job.rows_total.toLocaleString() : '?'} rows</span>
          </div>

          {job.status === 'failed' && job.error_message && (
            <p className="text-sm text-destructive">{job.error_message}</p>
          )}

          {!isFinished && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => jobId && cancelMutation.mutate(jobId)}
              disabled={cancelMutation.isPending}
            >
              Cancel
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
