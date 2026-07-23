import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { Skeleton } from '../components/ui/skeleton'
import { Users, Plus, Trash2, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { useCohorts, useCreateCohort, useDeleteCohort } from '../hooks/useCohorts'
import type { Cohort, CohortDefinition } from '../hooks/useCohorts'
import { useDomainStore } from '../stores/useDomainStore'

function CreateCohortForm({ onClose }: { onClose: () => void }) {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const createCohort = useCreateCohort()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [def, setDef] = useState<CohortDefinition>({})

  function update<K extends keyof CohortDefinition>(key: K, value: string) {
    setDef(prev => ({ ...prev, [key]: value || undefined }))
  }

  function handleSubmit() {
    if (!name.trim() || !selectedDomainId) return
    if (Object.values(def).every(v => !v)) {
      toast.error('Add at least one filter')
      return
    }
    createCohort.mutate(
      { domain_id: selectedDomainId, name, description, definition: def },
      {
        onSuccess: () => {
          toast.success('Cohort created')
          onClose()
        },
        onError: err => toast.error(err instanceof Error ? err.message : 'Failed'),
      }
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>New Cohort</CardTitle>
        <CardDescription>
          Group visitors by source, content, or geography. Filters are AND-ed together.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder="Paid search" />
          </div>
          <div className="space-y-2">
            <Label>Description</Label>
            <Input
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="Optional"
            />
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>UTM Source</Label>
            <Input
              value={def.utm_source ?? ''}
              onChange={e => update('utm_source', e.target.value)}
              placeholder="google"
            />
          </div>
          <div className="space-y-2">
            <Label>UTM Medium</Label>
            <Input
              value={def.utm_medium ?? ''}
              onChange={e => update('utm_medium', e.target.value)}
              placeholder="cpc"
            />
          </div>
          <div className="space-y-2">
            <Label>UTM Campaign</Label>
            <Input
              value={def.utm_campaign ?? ''}
              onChange={e => update('utm_campaign', e.target.value)}
              placeholder="spring_sale"
            />
          </div>
          <div className="space-y-2">
            <Label>Country (ISO code)</Label>
            <Input
              value={def.country ?? ''}
              onChange={e => update('country', e.target.value.toUpperCase())}
              placeholder="US"
            />
          </div>
          <div className="space-y-2">
            <Label>Referrer contains</Label>
            <Input
              value={def.referrer_contains ?? ''}
              onChange={e => update('referrer_contains', e.target.value)}
              placeholder="producthunt"
            />
          </div>
          <div className="space-y-2">
            <Label>Path contains</Label>
            <Input
              value={def.path_contains ?? ''}
              onChange={e => update('path_contains', e.target.value)}
              placeholder="/blog"
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={createCohort.isPending}>
            {createCohort.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Create Cohort
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function summarize(def: CohortDefinition): string {
  const parts: string[] = []
  if (def.utm_source) parts.push(`source=${def.utm_source}`)
  if (def.utm_medium) parts.push(`medium=${def.utm_medium}`)
  if (def.utm_campaign) parts.push(`campaign=${def.utm_campaign}`)
  if (def.referrer_contains) parts.push(`ref~${def.referrer_contains}`)
  if (def.path_contains) parts.push(`path~${def.path_contains}`)
  if (def.country) parts.push(`country=${def.country}`)
  return parts.join(' AND ') || '—'
}

function CohortRow({ cohort }: { cohort: Cohort }) {
  const deleteCohort = useDeleteCohort()
  function handleDelete() {
    if (!confirm(`Delete cohort "${cohort.name}"?`)) return
    deleteCohort.mutate(cohort.id, {
      onSuccess: () => toast.success('Cohort deleted'),
      onError: err => toast.error(err instanceof Error ? err.message : 'Failed'),
    })
  }
  return (
    <tr className="border-t">
      <td className="p-3">
        <div className="font-medium">{cohort.name}</div>
        {cohort.description && (
          <div className="text-xs text-muted-foreground">{cohort.description}</div>
        )}
      </td>
      <td className="p-3 text-sm text-muted-foreground font-mono">{summarize(cohort.definition)}</td>
      <td className="p-3">
        <Button variant="ghost" size="icon" onClick={handleDelete}>
          <Trash2 className="h-4 w-4 text-muted-foreground" />
        </Button>
      </td>
    </tr>
  )
}

export function Cohorts() {
  const [creating, setCreating] = useState(false)
  const { data: cohorts, isLoading } = useCohorts()
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Cohorts</h1>
          <p className="text-sm text-muted-foreground">
            Named visitor segments you can reuse in retention and other reports.
          </p>
        </div>
        {selectedDomainId && !creating && (
          <Button onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4 mr-1" /> New Cohort
          </Button>
        )}
      </div>

      {!selectedDomainId ? (
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">
              Select a property to manage cohorts.
            </p>
          </CardContent>
        </Card>
      ) : (
        <>
          {creating && <CreateCohortForm onClose={() => setCreating(false)} />}

          {isLoading ? (
            <Skeleton className="h-48 w-full" />
          ) : !cohorts?.length && !creating ? (
            <Card>
              <CardContent className="py-12 text-center">
                <Users className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
                <h3 className="text-lg font-medium mb-2">No cohorts yet</h3>
                <p className="text-sm text-muted-foreground mb-4">
                  Create a cohort to group visitors by acquisition channel, content, or country.
                </p>
                <Button onClick={() => setCreating(true)}>
                  <Plus className="h-4 w-4 mr-1" /> Create Cohort
                </Button>
              </CardContent>
            </Card>
          ) : cohorts?.length ? (
            <Card>
              <CardContent className="p-0">
                <table className="w-full text-sm">
                  <thead className="bg-muted/50 text-xs uppercase">
                    <tr>
                      <th className="text-left p-3">Name</th>
                      <th className="text-left p-3">Definition</th>
                      <th className="p-3 w-8" />
                    </tr>
                  </thead>
                  <tbody>
                    {cohorts.map(c => (
                      <CohortRow key={c.id} cohort={c} />
                    ))}
                  </tbody>
                </table>
              </CardContent>
            </Card>
          ) : null}
        </>
      )}
    </div>
  )
}
