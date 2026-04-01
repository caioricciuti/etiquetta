import { useState } from 'react'
import { useFunnels, useCreateFunnel, useDeleteFunnel, useFunnelMetrics } from '../hooks/useFunnels'
import { useDomainStore } from '../stores/useDomainStore'
import { useDateRangeStore } from '../stores/useDateRangeStore'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Skeleton } from '../components/ui/skeleton'
import { Filter, Plus, Trash2, Loader2, ArrowDown } from 'lucide-react'
import { toast } from 'sonner'
import type { Funnel, FunnelStep, FunnelStepMetric } from '../lib/types'

function FunnelVisualization({ steps }: { steps: FunnelStepMetric[] }) {
  if (!steps.length) return null

  const maxVisitors = steps[0].visitors || 1

  return (
    <div className="space-y-2">
      {steps.map((step, i) => {
        const width = Math.max(10, (step.visitors / maxVisitors) * 100)
        return (
          <div key={i}>
            <div className="flex items-center justify-between text-sm mb-1">
              <span className="font-medium">
                {step.step}. {step.name}
              </span>
              <span className="text-muted-foreground">
                {step.visitors.toLocaleString()} visitors ({step.rate.toFixed(1)}%)
              </span>
            </div>
            <div className="h-8 bg-muted rounded-md overflow-hidden">
              <div
                className="h-full bg-primary/80 rounded-md transition-all duration-500"
                style={{ width: `${width}%` }}
              />
            </div>
            {i < steps.length - 1 && step.drop_off > 0 && (
              <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1 ml-2">
                <ArrowDown className="h-3 w-3" />
                {step.drop_off.toLocaleString()} dropped off
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

function CreateFunnelForm({ onClose }: { onClose: () => void }) {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const createFunnel = useCreateFunnel()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [steps, setSteps] = useState<FunnelStep[]>([
    { event_type: 'pageview', page_path: '' },
    { event_type: 'pageview', page_path: '' },
  ])

  function addStep() {
    setSteps([...steps, { event_type: 'pageview', page_path: '' }])
  }

  function removeStep(i: number) {
    if (steps.length <= 2) return
    setSteps(steps.filter((_, idx) => idx !== i))
  }

  function updateStep(i: number, updates: Partial<FunnelStep>) {
    setSteps(steps.map((s, idx) => idx === i ? { ...s, ...updates } : s))
  }

  function handleSubmit() {
    if (!name.trim() || !selectedDomainId) return
    const validSteps = steps.every(s =>
      s.event_type === 'pageview' ? s.page_path?.trim() : s.event_name?.trim()
    )
    if (!validSteps) {
      toast.error('All steps must have a value')
      return
    }

    createFunnel.mutate(
      { domain_id: selectedDomainId, name, description, steps },
      {
        onSuccess: () => { toast.success('Funnel created'); onClose() },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to create funnel'),
      }
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>New Funnel</CardTitle>
        <CardDescription>Define the steps visitors should complete in order.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder="Signup funnel" />
          </div>
          <div className="space-y-2">
            <Label>Description</Label>
            <Input value={description} onChange={e => setDescription(e.target.value)} placeholder="Optional" />
          </div>
        </div>

        <div className="space-y-3">
          <Label>Steps</Label>
          {steps.map((step, i) => (
            <div key={i} className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground w-6">{i + 1}.</span>
              <Select
                value={step.event_type}
                onValueChange={v => updateStep(i, { event_type: v as 'pageview' | 'custom', page_path: '', event_name: '' })}
              >
                <SelectTrigger className="w-[130px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pageview">Page visit</SelectItem>
                  <SelectItem value="custom">Custom event</SelectItem>
                </SelectContent>
              </Select>
              {step.event_type === 'pageview' ? (
                <Input
                  className="flex-1"
                  value={step.page_path ?? ''}
                  onChange={e => updateStep(i, { page_path: e.target.value })}
                  placeholder="/pricing"
                />
              ) : (
                <Input
                  className="flex-1"
                  value={step.event_name ?? ''}
                  onChange={e => updateStep(i, { event_name: e.target.value })}
                  placeholder="signup_click"
                />
              )}
              <Button variant="ghost" size="icon" onClick={() => removeStep(i)} disabled={steps.length <= 2}>
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
          <Button variant="outline" size="sm" onClick={addStep}>
            <Plus className="h-4 w-4 mr-1" /> Add step
          </Button>
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={createFunnel.isPending}>
            {createFunnel.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Create Funnel
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function FunnelDetail({ funnel }: { funnel: Funnel }) {
  const { data, isLoading } = useFunnelMetrics(funnel.id)
  const deleteFunnel = useDeleteFunnel()

  function handleDelete() {
    if (!confirm(`Delete funnel "${funnel.name}"?`)) return
    deleteFunnel.mutate(funnel.id, {
      onSuccess: () => toast.success('Funnel deleted'),
      onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete'),
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>{funnel.name}</CardTitle>
            {funnel.description && <CardDescription>{funnel.description}</CardDescription>}
          </div>
          <Button variant="ghost" size="icon" onClick={handleDelete}>
            <Trash2 className="h-4 w-4 text-muted-foreground" />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-3">
            {funnel.steps.map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
          </div>
        ) : data?.steps?.length ? (
          <FunnelVisualization steps={data.steps} />
        ) : (
          <p className="text-sm text-muted-foreground text-center py-8">
            No data for this date range.
          </p>
        )}
      </CardContent>
    </Card>
  )
}

export function Funnels() {
  const [creating, setCreating] = useState(false)
  const { data: funnels, isLoading } = useFunnels()
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const { dateRange, setDateRange, selectedPreset, setPreset } = useDateRangeStore()

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Funnels</h1>
          <p className="text-sm text-muted-foreground">Track visitor conversion flows step by step.</p>
        </div>
        <div className="flex items-center gap-2">
          <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} selectedPreset={selectedPreset} onPresetChange={setPreset} />
          {selectedDomainId && (
            <Button onClick={() => setCreating(true)} disabled={creating}>
              <Plus className="h-4 w-4 mr-1" /> New Funnel
            </Button>
          )}
        </div>
      </div>

      {!selectedDomainId ? (
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">Select a property to manage funnels.</p>
          </CardContent>
        </Card>
      ) : (
        <>
          {creating && <CreateFunnelForm onClose={() => setCreating(false)} />}

          {isLoading ? (
            <div className="space-y-4">
              <Skeleton className="h-48 w-full" />
              <Skeleton className="h-48 w-full" />
            </div>
          ) : funnels?.length ? (
            funnels.map(f => <FunnelDetail key={f.id} funnel={f} />)
          ) : !creating ? (
            <Card>
              <CardContent className="py-12 text-center">
                <Filter className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
                <h3 className="text-lg font-medium mb-2">No funnels yet</h3>
                <p className="text-sm text-muted-foreground mb-4">
                  Create your first funnel to track how visitors move through your conversion steps.
                </p>
                <Button onClick={() => setCreating(true)}>
                  <Plus className="h-4 w-4 mr-1" /> Create Funnel
                </Button>
              </CardContent>
            </Card>
          ) : null}
        </>
      )}
    </div>
  )
}
