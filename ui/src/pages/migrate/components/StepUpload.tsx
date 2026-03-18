import { useCallback, useState } from 'react'
import type { MigrateSource, MigrateAnalysis } from '../../../lib/types'
import { sourceConfigs } from './source-config'
import { useAnalyzeMigration } from '../../../hooks/useMigrate'
import { Button } from '../../../components/ui/button'
import { Upload } from 'lucide-react'

interface Props {
  source: MigrateSource
  onAnalyzed: (file: File, analysis: MigrateAnalysis) => void
  onBack: () => void
}

export function StepUpload({ source, onAnalyzed, onBack }: Props) {
  const cfg = sourceConfigs.find((c) => c.id === source)!
  const [dragOver, setDragOver] = useState(false)
  const analyze = useAnalyzeMigration()

  const handleFile = useCallback((file: File) => {
    analyze.mutate({ file, source }, {
      onSuccess: (data) => onAnalyzed(file, data),
    })
  }, [analyze, source, onAnalyzed])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }, [handleFile])

  const handleInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) handleFile(file)
  }, [handleFile])

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Upload File</h2>
      <p className="text-sm text-muted-foreground mb-4">Upload your {cfg.name} export file</p>

      <div
        className={`border-2 border-dashed rounded-lg p-12 text-center transition-colors ${
          dragOver ? 'border-primary bg-primary/5' : 'border-muted-foreground/25'
        }`}
        onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
      >
        <Upload className="h-8 w-8 mx-auto mb-3 text-muted-foreground" />
        <p className="text-sm text-muted-foreground mb-2">
          Drag and drop your file here, or
        </p>
        <label className="cursor-pointer">
          <span className="text-sm text-primary hover:underline">browse files</span>
          <input
            type="file"
            className="hidden"
            accept={cfg.accept}
            onChange={handleInputChange}
          />
        </label>
        <p className="text-xs text-muted-foreground mt-2">Accepted: {cfg.accept}</p>
      </div>

      {analyze.isPending && (
        <div className="flex items-center gap-2 mt-4 text-sm text-muted-foreground">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary" />
          Analyzing file...
        </div>
      )}

      {analyze.isError && (
        <p className="text-sm text-destructive mt-4">{analyze.error.message}</p>
      )}

      <div className="flex gap-2 mt-4">
        <Button variant="outline" onClick={onBack}>Back</Button>
      </div>
    </div>
  )
}
