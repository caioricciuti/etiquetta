import { useMigrateJobStatus } from '../../../hooks/useMigrate'
import { Button } from '../../../components/ui/button'
import { CheckCircle } from 'lucide-react'

interface Props {
  jobId: string | null
  onNewImport: () => void
  onViewHistory: () => void
}

export function StepDone({ jobId, onNewImport, onViewHistory }: Props) {
  const { data: job } = useMigrateJobStatus(jobId)

  return (
    <div className="text-center py-8">
      <CheckCircle className="h-12 w-12 mx-auto mb-4 text-green-500" />
      <h2 className="text-lg font-semibold mb-2">Import Complete</h2>
      {job && (
        <p className="text-sm text-muted-foreground mb-6">
          Successfully imported {job.rows_imported.toLocaleString()} events
          {job.rows_skipped > 0 && ` (${job.rows_skipped.toLocaleString()} skipped)`}
        </p>
      )}
      <div className="flex gap-2 justify-center">
        <Button variant="outline" onClick={onNewImport}>New Import</Button>
        <Button variant="outline" onClick={onViewHistory}>View History</Button>
        <Button onClick={() => window.location.href = '/'}>Go to Dashboard</Button>
      </div>
    </div>
  )
}
