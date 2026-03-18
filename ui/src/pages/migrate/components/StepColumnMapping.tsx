import { useState } from 'react'
import type { MigrateAnalysis } from '../../../lib/types'
import { Button } from '../../../components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../components/ui/select'

const TARGET_FIELDS = [
  { value: '', label: '-- Skip --' },
  { value: 'path', label: 'Page Path' },
  { value: 'timestamp', label: 'Date / Timestamp' },
  { value: 'page_title', label: 'Page Title' },
  { value: 'country', label: 'Country' },
  { value: 'browser', label: 'Browser' },
  { value: 'os', label: 'OS' },
  { value: 'device', label: 'Device Type' },
  { value: 'referrer', label: 'Referrer' },
  { value: 'utm_source', label: 'UTM Source' },
  { value: 'utm_medium', label: 'UTM Medium' },
  { value: 'utm_campaign', label: 'UTM Campaign' },
  { value: 'event_name', label: 'Event Name' },
  { value: 'count', label: 'Event Count' },
]

interface Props {
  analysis: MigrateAnalysis
  onConfirm: (mapping: Record<string, string>) => void
  onBack: () => void
}

export function StepColumnMapping({ analysis, onConfirm, onBack }: Props) {
  const [mapping, setMapping] = useState<Record<string, string>>(() => {
    // Pre-fill from suggested mapping
    const m: Record<string, string> = {}
    if (analysis.suggested_mapping) {
      for (const [src, tgt] of Object.entries(analysis.suggested_mapping)) {
        m[src] = tgt
      }
    }
    return m
  })

  const updateMapping = (col: string, target: string) => {
    setMapping((prev) => ({ ...prev, [col]: target }))
  }

  const hasPath = Object.values(mapping).includes('path')
  const hasTimestamp = Object.values(mapping).includes('timestamp')
  const canProceed = hasPath && hasTimestamp

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Column Mapping</h2>
      <p className="text-sm text-muted-foreground mb-4">
        Map your CSV columns to Etiquetta fields. At minimum, map a page path and date/timestamp.
      </p>

      <div className="space-y-3 max-w-xl">
        {analysis.columns.map((col) => (
          <div key={col} className="flex items-center gap-3">
            <span className="text-sm font-mono w-40 truncate shrink-0" title={col}>{col}</span>
            <span className="text-muted-foreground text-sm shrink-0">&rarr;</span>
            <Select value={mapping[col] || ''} onValueChange={(v) => updateMapping(col, v)}>
              <SelectTrigger className="w-48">
                <SelectValue placeholder="Skip" />
              </SelectTrigger>
              <SelectContent>
                {TARGET_FIELDS.map((f) => (
                  <SelectItem key={f.value} value={f.value || 'skip'}>{f.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {analysis.sample_rows[0] && (
              <span className="text-xs text-muted-foreground truncate">
                e.g. {analysis.sample_rows[0][analysis.columns.indexOf(col)]}
              </span>
            )}
          </div>
        ))}
      </div>

      {!canProceed && (
        <p className="text-sm text-amber-500 mt-3">Map at least a page path and date column to proceed.</p>
      )}

      <div className="flex gap-2 mt-6">
        <Button variant="outline" onClick={onBack}>Back</Button>
        <Button disabled={!canProceed} onClick={() => {
          // Filter out empty/skip mappings
          const clean: Record<string, string> = {}
          for (const [k, v] of Object.entries(mapping)) {
            if (v && v !== 'skip') clean[k] = v
          }
          onConfirm(clean)
        }}>Next</Button>
      </div>
    </div>
  )
}
