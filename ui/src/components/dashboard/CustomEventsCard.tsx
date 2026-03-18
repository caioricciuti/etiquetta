import { Link } from 'react-router-dom'
import { Zap, ArrowRight } from 'lucide-react'
import { useEventsSummary } from '../../hooks/useAnalyticsQueries'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { ProgressList } from './ProgressList'
import { ProgressListSkeleton } from './skeletons'

const EVENT_TYPE_LABELS: Record<string, string> = {
  pageview: 'Page View',
  custom: 'Custom',
  click: 'Click',
  scroll: 'Scroll',
  engagement: 'Engagement',
}

export function CustomEventsCard() {
  const { data, isLoading, isPlaceholderData } = useEventsSummary()

  // Show top 5 events across all types (excluding pageview to keep it interesting)
  const events = (data?.events ?? [])
    .filter((ev) => ev.event_type !== 'pageview')
    .slice(0, 5)

  const maxCount = events[0]?.count || 1
  const items = events.map((event) => ({
    label: `${event.event_name} (${EVENT_TYPE_LABELS[event.event_type] || event.event_type})`,
    value: event.count,
    percentage: (event.count / maxCount) * 100,
  }))

  return (
    <Card className={`transition-opacity ${isPlaceholderData ? 'opacity-60' : ''}`}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Zap className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-lg font-semibold">Events</CardTitle>
          </div>
          <Link
            to="/events"
            className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors"
          >
            View all <ArrowRight className="h-3 w-3" />
          </Link>
        </div>
        <CardDescription>Top events (excl. page views)</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading && !data ? <ProgressListSkeleton count={3} /> : <ProgressList items={items} colorClass="bg-cyan-500" />}
      </CardContent>
    </Card>
  )
}
