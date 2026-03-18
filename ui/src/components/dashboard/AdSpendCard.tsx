import { useAdSpend } from '@/hooks/useAnalyticsQueries'
import { useConnections } from '@/hooks/useAnalyticsQueries'
import { FeatureGate } from '@/components/FeatureGate'
import { DollarSign, MousePointerClick, Eye } from 'lucide-react'

function AdSpendContent() {
  const { data: connections } = useConnections()
  const { data: spendData } = useAdSpend()

  // Don't render if no active connections
  const hasActive = connections?.some((c) => c.status === 'active')
  if (!hasActive) return null

  // Aggregate totals
  const totals = (spendData ?? []).reduce(
    (acc, row) => ({
      cost: acc.cost + row.cost,
      impressions: acc.impressions + row.impressions,
      clicks: acc.clicks + row.clicks,
    }),
    { cost: 0, impressions: 0, clicks: 0 },
  )

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center gap-2 mb-4">
        <DollarSign className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-sm font-medium text-foreground">Ad Spend</h3>
      </div>
      <div className="grid grid-cols-3 gap-4">
        <div>
          <p className="text-2xl font-bold text-foreground">
            ${totals.cost.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </p>
          <p className="text-xs text-muted-foreground flex items-center gap-1 mt-1">
            <DollarSign className="h-3 w-3" />
            Total Spend
          </p>
        </div>
        <div>
          <p className="text-2xl font-bold text-foreground">
            {totals.clicks.toLocaleString()}
          </p>
          <p className="text-xs text-muted-foreground flex items-center gap-1 mt-1">
            <MousePointerClick className="h-3 w-3" />
            Clicks
          </p>
        </div>
        <div>
          <p className="text-2xl font-bold text-foreground">
            {totals.impressions.toLocaleString()}
          </p>
          <p className="text-xs text-muted-foreground flex items-center gap-1 mt-1">
            <Eye className="h-3 w-3" />
            Impressions
          </p>
        </div>
      </div>
    </div>
  )
}

export function AdSpendCard() {
  return (
    <FeatureGate feature="connections" fallback={null}>
      <AdSpendContent />
    </FeatureGate>
  )
}
