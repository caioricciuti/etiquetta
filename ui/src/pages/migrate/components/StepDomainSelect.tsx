import { useState } from 'react'
import { useDomains } from '../../../hooks/useDomains'
import { Button } from '../../../components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../components/ui/select'

interface Props {
  onSelect: (domain: string) => void
  onBack: () => void
}

export function StepDomainSelect({ onSelect, onBack }: Props) {
  const { data: domains, isLoading } = useDomains()
  const [selected, setSelected] = useState<string>('')

  return (
    <div>
      <h2 className="text-lg font-semibold mb-1">Select Domain</h2>
      <p className="text-sm text-muted-foreground mb-4">Choose which domain to import data into</p>

      {isLoading ? (
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
      ) : (
        <Select value={selected} onValueChange={setSelected}>
          <SelectTrigger className="w-full max-w-sm">
            <SelectValue placeholder="Select a domain" />
          </SelectTrigger>
          <SelectContent>
            {domains?.map((d) => (
              <SelectItem key={d.id} value={d.domain}>{d.domain}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <div className="flex gap-2 mt-6">
        <Button variant="outline" onClick={onBack}>Back</Button>
        <Button disabled={!selected} onClick={() => onSelect(selected)}>Next</Button>
      </div>
    </div>
  )
}
