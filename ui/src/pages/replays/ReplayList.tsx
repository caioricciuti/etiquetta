import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useReplays, useReplayStats, useDeleteReplay } from '@/hooks/useReplayQueries'
import { useSelectedDomain } from '@/hooks/useSelectedDomain'
import { FeatureGate } from '@/components/FeatureGate'
import { toast } from 'sonner'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Play,
  Trash2,
  Monitor,
  Smartphone,
  Tablet,
  Globe,
  Clock,
  HardDrive,
  ChevronLeft,
  ChevronRight,
  Video,
  Settings,
} from 'lucide-react'
import type { SessionRecording } from '@/lib/types'

function formatDuration(ms: number): string {
  const sec = Math.floor(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  const remSec = sec % 60
  return `${min}m ${remSec}s`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleString()
}

function DeviceIcon({ type }: { type: string }) {
  switch (type) {
    case 'mobile':
      return <Smartphone className="h-4 w-4" />
    case 'tablet':
      return <Tablet className="h-4 w-4" />
    default:
      return <Monitor className="h-4 w-4" />
  }
}

const DURATION_FILTERS: { label: string; min?: string; max?: string }[] = [
  { label: 'Any' },
  { label: '< 30s', max: '30000' },
  { label: '30s – 2m', min: '30000', max: '120000' },
  { label: '2m – 5m', min: '120000', max: '300000' },
  { label: '5m – 15m', min: '300000', max: '900000' },
  { label: '> 15m', min: '900000' },
]

function RecordingRow({
  recording,
  onDelete,
}: {
  recording: SessionRecording
  onDelete: (id: string) => void
}) {
  return (
    <div className="flex items-center gap-4 p-4 border-b border-border hover:bg-muted/50 transition-colors">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <DeviceIcon type={recording.device_type} />
          <span className="text-sm font-medium truncate">
            {recording.first_url || 'Unknown page'}
          </span>
          {recording.geo_country && (
            <span className="text-xs text-muted-foreground flex items-center gap-1">
              <Globe className="h-3 w-3" />
              {recording.geo_country}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <Badge variant="outline" className="text-xs">
            {recording.device_type || 'desktop'}
          </Badge>
          {recording.browser_name && (
            <Badge variant="secondary" className="text-xs">
              {recording.browser_name}
            </Badge>
          )}
          {recording.os_name && (
            <Badge variant="secondary" className="text-xs">
              {recording.os_name}
            </Badge>
          )}
          <span className="text-xs text-muted-foreground">{formatTime(recording.start_time)}</span>
          <span className="text-xs text-muted-foreground flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {formatDuration(recording.duration)}
          </span>
          <span className="text-xs text-muted-foreground">
            {recording.pages} page{recording.pages !== 1 ? 's' : ''}
          </span>
          {recording.screen_width > 0 && (
            <span className="text-xs text-muted-foreground">
              {recording.screen_width}x{recording.screen_height}
            </span>
          )}
          <span className="text-xs text-muted-foreground flex items-center gap-1">
            <HardDrive className="h-3 w-3" />
            {formatBytes(recording.size_bytes)}
          </span>
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Button asChild size="sm">
          <Link to={`/replays/${recording.session_id}`}>
            <Play className="h-3.5 w-3.5 mr-1" />
            Play
          </Link>
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-destructive"
          onClick={() => onDelete(recording.session_id)}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}

function ReplayListContent() {
  const { selectedDomain } = useSelectedDomain()
  const [page, setPage] = useState(0)
  const [deviceType, setDeviceType] = useState('')
  const [browserName, setBrowserName] = useState('')
  const [osName, setOsName] = useState('')
  const [durationIdx, setDurationIdx] = useState(0)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const limit = 20

  const durationFilter = DURATION_FILTERS[durationIdx]

  const { data, isLoading } = useReplays({
    domain: selectedDomain?.domain,
    limit,
    offset: page * limit,
    device_type: deviceType || undefined,
    browser_name: browserName || undefined,
    os_name: osName || undefined,
    min_duration: durationFilter?.min,
    max_duration: durationFilter?.max,
  })

  const { data: stats } = useReplayStats()
  const deleteMutation = useDeleteReplay()

  const handleDelete = () => {
    if (!deleteTarget) return
    deleteMutation.mutate(deleteTarget, {
      onSuccess: () => {
        toast.success('Recording deleted')
        setDeleteTarget(null)
      },
      onError: () => toast.error('Failed to delete recording'),
    })
  }

  const resetFilters = () => {
    setDeviceType('')
    setBrowserName('')
    setOsName('')
    setDurationIdx(0)
    setPage(0)
  }

  const hasFilters = deviceType || browserName || osName || durationIdx > 0

  const recordings = data?.recordings ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / limit)

  return (
    <div className="p-6 space-y-6 overflow-auto h-full">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Session Replay</h1>
          <p className="text-muted-foreground">Watch real user sessions to understand behavior</p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <Link to="/replays/settings">
            <Settings className="h-4 w-4 mr-1.5" />
            Settings
          </Link>
        </Button>
      </div>

      {stats && (
        <div className="grid grid-cols-3 gap-4">
          <Card>
            <CardContent className="pt-4 pb-3">
              <p className="text-sm text-muted-foreground">Total recordings</p>
              <p className="text-2xl font-bold">{stats.total_recordings.toLocaleString()}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-4 pb-3">
              <p className="text-sm text-muted-foreground">Storage used</p>
              <p className="text-2xl font-bold">{formatBytes(stats.disk_usage_bytes)}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-4 pb-3">
              <p className="text-sm text-muted-foreground">Quota usage</p>
              <p className="text-2xl font-bold">
                {stats.quota_bytes > 0
                  ? `${Math.round((stats.disk_usage_bytes / stats.quota_bytes) * 100)}%`
                  : 'Unlimited'}
              </p>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Filters */}
      <div className="flex items-center gap-3 flex-wrap">
        <Select value={deviceType} onValueChange={(v) => { setDeviceType(v === 'any' ? '' : v); setPage(0) }}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Device type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">Any device</SelectItem>
            <SelectItem value="desktop">Desktop</SelectItem>
            <SelectItem value="mobile">Mobile</SelectItem>
            <SelectItem value="tablet">Tablet</SelectItem>
          </SelectContent>
        </Select>

        <Select value={browserName} onValueChange={(v) => { setBrowserName(v === 'any' ? '' : v); setPage(0) }}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Browser" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">Any browser</SelectItem>
            <SelectItem value="Chrome">Chrome</SelectItem>
            <SelectItem value="Firefox">Firefox</SelectItem>
            <SelectItem value="Safari">Safari</SelectItem>
            <SelectItem value="Edge">Edge</SelectItem>
            <SelectItem value="Opera">Opera</SelectItem>
          </SelectContent>
        </Select>

        <Select value={osName} onValueChange={(v) => { setOsName(v === 'any' ? '' : v); setPage(0) }}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="OS" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">Any OS</SelectItem>
            <SelectItem value="Windows">Windows</SelectItem>
            <SelectItem value="macOS">macOS</SelectItem>
            <SelectItem value="Linux">Linux</SelectItem>
            <SelectItem value="Android">Android</SelectItem>
            <SelectItem value="iOS">iOS</SelectItem>
          </SelectContent>
        </Select>

        <Select value={String(durationIdx)} onValueChange={(v) => { setDurationIdx(Number(v)); setPage(0) }}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Duration" />
          </SelectTrigger>
          <SelectContent>
            {DURATION_FILTERS.map((d, i) => (
              <SelectItem key={i} value={String(i)}>{d.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={resetFilters}>
            Reset
          </Button>
        )}
      </div>

      <div className="rounded-lg border">
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
          </div>
        ) : recordings.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
            <Video className="h-12 w-12 mb-3 opacity-30" />
            <p className="text-lg font-medium">No recordings found</p>
            <p className="text-sm">
              {hasFilters ? 'Try adjusting your filters' : 'Enable session replay in settings to start recording'}
            </p>
          </div>
        ) : (
          recordings.map((rec) => (
            <RecordingRow
              key={rec.session_id}
              recording={rec}
              onDelete={setDeleteTarget}
            />
          ))
        )}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            Showing {page * limit + 1}–{Math.min((page + 1) * limit, total)} of {total}
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="icon"
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={page === 0}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="text-sm">
              {page + 1} / {totalPages}
            </span>
            <Button
              variant="outline"
              size="icon"
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={page >= totalPages - 1}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}

      {/* Delete confirmation dialog */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete recording</DialogTitle>
            <DialogDescription>
              This will permanently delete the session recording and its data. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export function ReplayList() {
  return (
    <FeatureGate feature="session_replay">
      <ReplayListContent />
    </FeatureGate>
  )
}
