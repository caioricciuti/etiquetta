import { useState, useCallback } from 'react'
import { useGtmConvert } from '../../../hooks/useMigrate'
import { Button } from '../../../components/ui/button'
import { Upload, CheckCircle } from 'lucide-react'
import type { GtmConvertResult } from '../../../lib/types'

interface Props {
  onBack: () => void
}

export function GtmConverter({ onBack }: Props) {
  const convert = useGtmConvert()
  const [result, setResult] = useState<GtmConvertResult | null>(null)
  const [dragOver, setDragOver] = useState(false)

  const handleFile = useCallback((file: File) => {
    convert.mutate(file, {
      onSuccess: (data) => setResult(data),
    })
  }, [convert])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }, [handleFile])

  if (result) {
    return (
      <div>
        <h2 className="text-lg font-semibold mb-4">GTM Conversion Result</h2>

        <div className="flex items-center gap-2 mb-4">
          <CheckCircle className="h-5 w-5 text-green-500" />
          <span className="text-sm">Container converted successfully</span>
        </div>

        <div className="grid grid-cols-3 gap-4 mb-4">
          <div className="border rounded-lg p-3 text-center">
            <div className="text-2xl font-bold">{result.container.tags}</div>
            <div className="text-xs text-muted-foreground">Tags</div>
          </div>
          <div className="border rounded-lg p-3 text-center">
            <div className="text-2xl font-bold">{result.container.triggers}</div>
            <div className="text-xs text-muted-foreground">Triggers</div>
          </div>
          <div className="border rounded-lg p-3 text-center">
            <div className="text-2xl font-bold">{result.container.variables}</div>
            <div className="text-xs text-muted-foreground">Variables</div>
          </div>
        </div>

        {result.warnings.length > 0 && (
          <div className="bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 rounded p-3 mb-4">
            <p className="text-sm font-medium text-amber-800 dark:text-amber-200 mb-1">Warnings ({result.warnings.length})</p>
            <ul className="text-sm text-amber-700 dark:text-amber-300 space-y-1 max-h-40 overflow-y-auto">
              {result.warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        )}

        <p className="text-sm text-muted-foreground mb-4">
          To import this container, go to Tag Manager, create a new container, and use the Import feature with the converted data.
        </p>

        <div className="flex gap-2">
          <Button variant="outline" onClick={onBack}>Back</Button>
          <Button onClick={() => {
            const blob = new Blob([JSON.stringify(result.container_data, null, 2)], { type: 'application/json' })
            const url = URL.createObjectURL(blob)
            const a = document.createElement('a')
            a.href = url
            a.download = 'etiquetta-container.json'
            a.click()
            URL.revokeObjectURL(url)
          }}>
            Download Converted Container
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Convert GTM Container</h2>
      <p className="text-sm text-muted-foreground mb-4">
        Upload your Google Tag Manager container export JSON to convert it to Etiquetta format
      </p>

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
          Drag and drop your GTM container JSON here, or
        </p>
        <label className="cursor-pointer">
          <span className="text-sm text-primary hover:underline">browse files</span>
          <input
            type="file"
            className="hidden"
            accept=".json"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) handleFile(file)
            }}
          />
        </label>
      </div>

      {convert.isPending && (
        <div className="flex items-center gap-2 mt-4 text-sm text-muted-foreground">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary" />
          Converting...
        </div>
      )}

      {convert.isError && (
        <p className="text-sm text-destructive mt-4">{convert.error.message}</p>
      )}

      <div className="flex gap-2 mt-4">
        <Button variant="outline" onClick={onBack}>Back</Button>
      </div>
    </div>
  )
}
