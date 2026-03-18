import type { MigrateSource } from '../../../lib/types'
import { sourceConfigs } from './source-config'
import { Card, CardHeader, CardTitle, CardDescription } from '../../../components/ui/card'

interface Props {
  onSelect: (source: MigrateSource) => void
}

export function StepSourceSelect({ onSelect }: Props) {
  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Select Source</h2>
      <p className="text-sm text-muted-foreground mb-4">Choose where you're importing data from</p>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {sourceConfigs.map((cfg) => (
          <Card
            key={cfg.id}
            className="cursor-pointer hover:border-primary transition-colors"
            onClick={() => onSelect(cfg.id)}
          >
            <CardHeader className="p-4">
              <CardTitle className="text-base">{cfg.name}</CardTitle>
              <CardDescription className="text-sm">{cfg.description}</CardDescription>
            </CardHeader>
          </Card>
        ))}
      </div>
    </div>
  )
}
