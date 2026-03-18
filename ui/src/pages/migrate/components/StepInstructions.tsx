import type { MigrateSource } from '../../../lib/types'
import { sourceConfigs } from './source-config'
import { Button } from '../../../components/ui/button'

interface Props {
  source: MigrateSource
  onNext: () => void
  onBack: () => void
}

export function StepInstructions({ source, onNext, onBack }: Props) {
  const cfg = sourceConfigs.find((c) => c.id === source)!

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Export from {cfg.name}</h2>
      <p className="text-sm text-muted-foreground mb-4">Follow these steps to export your data</p>

      <ol className="list-decimal list-inside space-y-2 mb-6 text-sm">
        {cfg.instructions.map((step, i) => (
          <li key={i} className="text-foreground">{step}</li>
        ))}
      </ol>

      <div className="flex gap-2">
        <Button variant="outline" onClick={onBack}>Back</Button>
        <Button onClick={onNext}>I have my file ready</Button>
      </div>
    </div>
  )
}
