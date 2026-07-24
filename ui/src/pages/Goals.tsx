import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Skeleton } from '../components/ui/skeleton'
import { Target, Plus, Trash2, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { useGoals, useGoalStats, useCreateGoal, useDeleteGoal, useCustomEventNames } from '../hooks/useGoals'
import { useDomainStore } from '../stores/useDomainStore'
import { useDateRangeStore } from '../stores/useDateRangeStore'

function CreateGoalForm({ onClose }: { onClose: () => void }) {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const createGoal = useCreateGoal()
  const { data: eventNames } = useCustomEventNames()
  const [name, setName] = useState('')
  const [kind, setKind] = useState<'pageview' | 'custom' | 'outbound'>('pageview')
  const [matchValue, setMatchValue] = useState('')
  const [valueCents, setValueCents] = useState('0')
  const [currency, setCurrency] = useState('USD')

  function handleSubmit() {
    if (!name.trim() || !matchValue.trim() || !selectedDomainId) return
    createGoal.mutate({
      domain_id: selectedDomainId,
      name,
      kind,
      match_value: matchValue,
      value_cents: Number(valueCents),
      currency,
    }, {
      onSuccess: () => { toast.success('Goal created'); onClose() },
      onError: err => toast.error(err instanceof Error ? err.message : 'Failed'),
    })
  }

  const placeholder =
    kind === 'pageview' ? '/thank-you'
      : kind === 'outbound' ? 'partner.com/ref'
      : 'signup_completed'

  return (
    <Card>
      <CardHeader>
        <CardTitle>New Goal</CardTitle>
        <CardDescription>Track a single conversion event and (optionally) its revenue value.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder="Signup completed" />
          </div>
          <div className="space-y-2">
            <Label>Kind</Label>
            <Select value={kind} onValueChange={v => { setKind(v as typeof kind); setMatchValue('') }}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="pageview">Page visit</SelectItem>
                <SelectItem value="custom">Custom event</SelectItem>
                <SelectItem value="outbound">Outbound link</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="space-y-2">
          <Label>
            {kind === 'pageview' ? 'Path' : kind === 'outbound' ? 'URL contains' : 'Event name'}
          </Label>
          {kind === 'custom' && eventNames && eventNames.length > 0 ? (
            <>
              <Input
                value={matchValue}
                onChange={e => setMatchValue(e.target.value)}
                placeholder={placeholder}
                list="custom-event-suggestions"
              />
              <datalist id="custom-event-suggestions">
                {eventNames.map(e => <option key={e.name} value={e.name}>{`${e.count} events`}</option>)}
              </datalist>
            </>
          ) : (
            <Input value={matchValue} onChange={e => setMatchValue(e.target.value)} placeholder={placeholder} />
          )}
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Value per conversion (cents)</Label>
            <Input type="number" value={valueCents} onChange={e => setValueCents(e.target.value)} placeholder="0" />
          </div>
          <div className="space-y-2">
            <Label>Currency</Label>
            <Input value={currency} onChange={e => setCurrency(e.target.value.toUpperCase())} placeholder="USD" />
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={createGoal.isPending}>
            {createGoal.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Create Goal
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function formatMoney(cents: number, currency: string) {
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(cents / 100)
  } catch {
    return `${(cents / 100).toFixed(2)} ${currency}`
  }
}

export function Goals() {
  const [creating, setCreating] = useState(false)
  const { data: goals, isLoading: goalsLoading } = useGoals()
  const { data: stats, isLoading: statsLoading } = useGoalStats()
  const deleteGoal = useDeleteGoal()
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const { dateRange, setDateRange, selectedPreset, setPreset } = useDateRangeStore()

  function handleDelete(id: string, name: string) {
    if (!confirm(`Delete goal "${name}"?`)) return
    deleteGoal.mutate(id, { onSuccess: () => toast.success('Goal deleted') })
  }

  const statsByGoal = new Map(stats?.map(s => [s.goal_id, s]) ?? [])

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Goals</h1>
          <p className="text-sm text-muted-foreground">Track conversions and revenue for key actions.</p>
        </div>
        <div className="flex items-center gap-2">
          <DateRangePicker dateRange={dateRange} onDateRangeChange={setDateRange} selectedPreset={selectedPreset} onPresetChange={setPreset} />
          {selectedDomainId && !creating && (
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4 mr-1" /> New Goal
            </Button>
          )}
        </div>
      </div>

      {!selectedDomainId ? (
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">Select a property to manage goals.</p>
          </CardContent>
        </Card>
      ) : (
        <>
          {creating && <CreateGoalForm onClose={() => setCreating(false)} />}

          {goalsLoading ? (
            <Skeleton className="h-48 w-full" />
          ) : !goals?.length && !creating ? (
            <Card>
              <CardContent className="py-12 text-center">
                <Target className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
                <h3 className="text-lg font-medium mb-2">No goals yet</h3>
                <p className="text-sm text-muted-foreground mb-4">
                  Define what counts as a conversion and we'll track completions and revenue.
                </p>
                <Button onClick={() => setCreating(true)}>
                  <Plus className="h-4 w-4 mr-1" /> Create Goal
                </Button>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="p-0">
                <table className="w-full text-sm">
                  <thead className="bg-muted/50 text-xs uppercase">
                    <tr>
                      <th className="text-left p-3">Goal</th>
                      <th className="text-left p-3">Match</th>
                      <th className="text-right p-3">Completions</th>
                      <th className="text-right p-3">Unique</th>
                      <th className="text-right p-3">Conversion</th>
                      <th className="text-right p-3">Revenue</th>
                      <th className="p-3 w-8" />
                    </tr>
                  </thead>
                  <tbody>
                    {goals?.map(g => {
                      const s = statsByGoal.get(g.id)
                      return (
                        <tr key={g.id} className="border-t">
                          <td className="p-3">
                            <div className="font-medium">{g.name}</div>
                            <div className="text-xs text-muted-foreground">{g.kind}</div>
                          </td>
                          <td className="p-3 text-muted-foreground truncate max-w-xs">{g.match_value}</td>
                          <td className="p-3 text-right">
                            {statsLoading ? <Skeleton className="h-4 w-12 inline-block" /> : (s?.completions ?? 0).toLocaleString()}
                          </td>
                          <td className="p-3 text-right">
                            {statsLoading ? <Skeleton className="h-4 w-12 inline-block" /> : (s?.unique_visitors ?? 0).toLocaleString()}
                          </td>
                          <td className="p-3 text-right">
                            {statsLoading ? <Skeleton className="h-4 w-12 inline-block" /> : `${(s?.conversion_rate ?? 0).toFixed(1)}%`}
                          </td>
                          <td className="p-3 text-right">
                            {g.value_cents > 0 ? formatMoney(s?.revenue_cents ?? 0, g.currency) : '—'}
                          </td>
                          <td className="p-3">
                            <Button variant="ghost" size="icon" onClick={() => handleDelete(g.id, g.name)}>
                              <Trash2 className="h-4 w-4 text-muted-foreground" />
                            </Button>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  )
}
