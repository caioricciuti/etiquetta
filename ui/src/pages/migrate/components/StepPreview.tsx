import type { MigrateAnalysis } from '../../../lib/types'
import { Button } from '../../../components/ui/button'

interface Props {
  analysis: MigrateAnalysis
  mapping: Record<string, string>
  onConfirm: () => void
  onBack: () => void
}

export function StepPreview({ analysis, mapping, onConfirm, onBack }: Props) {
  const previewRows = analysis.sample_rows.slice(0, 10)

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Preview</h2>
      <p className="text-sm text-muted-foreground mb-4">
        Review the data before importing. {analysis.row_estimate > 0 && `Estimated ${analysis.row_estimate.toLocaleString()} rows.`}
      </p>

      {analysis.date_range && (
        <p className="text-sm mb-3">
          Date range: <strong>{analysis.date_range.start}</strong> to <strong>{analysis.date_range.end}</strong>
        </p>
      )}

      {analysis.warnings.length > 0 && (
        <div className="bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 rounded p-3 mb-4">
          <p className="text-sm font-medium text-amber-800 dark:text-amber-200 mb-1">Warnings</p>
          {analysis.warnings.map((w, i) => (
            <p key={i} className="text-sm text-amber-700 dark:text-amber-300">{w}</p>
          ))}
        </div>
      )}

      <div className="border rounded-lg overflow-x-auto max-h-72">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 sticky top-0">
            <tr>
              {analysis.columns.map((col) => (
                <th key={col} className="text-xs whitespace-nowrap text-left font-medium px-3 py-2">
                  {col}
                  {(analysis.suggested_mapping?.[col] || mapping[col]) && (
                    <span className="block text-xs text-primary font-normal">
                      &rarr; {mapping[col] || analysis.suggested_mapping?.[col]}
                    </span>
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {previewRows.map((row, i) => (
              <tr key={i} className="border-t border-border">
                {row.map((cell, j) => (
                  <td key={j} className="text-xs whitespace-nowrap px-3 py-1.5">{cell}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="flex gap-2 mt-6">
        <Button variant="outline" onClick={onBack}>Back</Button>
        <Button onClick={onConfirm}>Start Import</Button>
      </div>
    </div>
  )
}
