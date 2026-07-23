import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { DateRangePicker } from '../components/ui/date-range-picker'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Skeleton } from '../components/ui/skeleton'
import { Users } from 'lucide-react'
import { useRetention } from '../hooks/useCohorts'
import { useCohorts } from '../hooks/useCohorts'
import { useDomainStore } from '../stores/useDomainStore'
import { useDateRangeStore } from '../stores/useDateRangeStore'

function cellColor(rate: number): string {
  if (rate <= 0) return 'bg-muted/40'
  if (rate < 5) return 'bg-primary/10'
  if (rate < 15) return 'bg-primary/25'
  if (rate < 30) return 'bg-primary/45'
  if (rate < 50) return 'bg-primary/65'
  if (rate < 75) return 'bg-primary/80'
  return 'bg-primary'
}

export function Retention() {
  const [days, setDays] = useState(14)
  const [cohortId, setCohortId] = useState<string>('all')
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const { dateRange, setDateRange, selectedPreset, setPreset } = useDateRangeStore()
  const { data: cohorts } = useCohorts()
  const { data, isLoading } = useRetention({
    days,
    cohortId: cohortId === 'all' ? undefined : cohortId,
  })

  // Use the server-returned day count so headers stay aligned with the grid
  // while keepPreviousData shows a previous response during transitions
  const gridDays = data?.days ?? days
  const columnLabels = Array.from({ length: gridDays + 1 }, (_, i) => (i === 0 ? 'D0' : `+${i}`))

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div>
          <h1 className="text-2xl font-bold">Retention</h1>
          <p className="text-sm text-muted-foreground">
            % of new visitors who return on each subsequent day.
          </p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <DateRangePicker
            dateRange={dateRange}
            onDateRangeChange={setDateRange}
            selectedPreset={selectedPreset}
            onPresetChange={setPreset}
          />
          <Select value={String(days)} onValueChange={v => setDays(Number(v))}>
            <SelectTrigger className="w-[120px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="7">7 days</SelectItem>
              <SelectItem value="14">14 days</SelectItem>
              <SelectItem value="30">30 days</SelectItem>
            </SelectContent>
          </Select>
          <Select value={cohortId} onValueChange={setCohortId}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Cohort filter" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All visitors</SelectItem>
              {cohorts?.map(c => (
                <SelectItem key={c.id} value={c.id}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {!selectedDomainId ? (
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">Select a property to view retention.</p>
          </CardContent>
        </Card>
      ) : isLoading ? (
        <Skeleton className="h-96 w-full" />
      ) : !data?.cohorts?.length ? (
        <Card>
          <CardContent className="py-12 text-center">
            <Users className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
            <p className="text-muted-foreground">No data for this date range.</p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Retention grid</CardTitle>
            <CardDescription>
              Rows = the day visitors first appeared. Columns = days since first seen.
            </CardDescription>
          </CardHeader>
          <CardContent className="overflow-x-auto">
            <table className="text-xs border-separate border-spacing-0.5">
              <thead>
                <tr>
                  <th className="p-2 text-left sticky left-0 bg-background">Cohort</th>
                  <th className="p-2 text-right">Size</th>
                  {columnLabels.map(lbl => (
                    <th key={lbl} className="p-2 text-center w-12 text-muted-foreground">
                      {lbl}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.cohorts.map(c => (
                  <tr key={c.date}>
                    <td className="p-2 font-medium sticky left-0 bg-background whitespace-nowrap">
                      {c.date}
                    </td>
                    <td className="p-2 text-right text-muted-foreground">
                      {c.size.toLocaleString()}
                    </td>
                    {c.rates.map((rate, i) => (
                      <td
                        key={i}
                        className={`p-2 text-center rounded ${cellColor(rate)} ${
                          rate > 50 ? 'text-primary-foreground' : ''
                        }`}
                        title={`${c.returns[i]} of ${c.size}`}
                      >
                        {c.size > 0 ? `${rate.toFixed(0)}%` : '—'}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
