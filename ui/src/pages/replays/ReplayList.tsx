import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useReplays, useReplayStats, useDeleteReplay } from '@/hooks/useReplayQueries'
import { useSelectedDomain } from '@/hooks/useSelectedDomain'
import { FeatureGate } from '@/components/FeatureGate'
import { toast } from 'sonner'
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

function RecordingRow({ recording, onDelete }: { recording: SessionRecording; onDelete: (id: string) => void }) {
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
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span>{formatTime(recording.start_time)}</span>
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {formatDuration(recording.duration)}
          </span>
          <span>{recording.pages} page{recording.pages !== 1 ? 's' : ''}</span>
          <span>{recording.browser_name}</span>
          {recording.screen_width > 0 && (
            <span>{recording.screen_width}x{recording.screen_height}</span>
          )}
          <span className="flex items-center gap-1">
            <HardDrive className="h-3 w-3" />
            {formatBytes(recording.size_bytes)}
          </span>
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Link
          to={`/replays/${recording.session_id}`}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          <Play className="h-3.5 w-3.5" />
          Play
        </Link>
        <button
          onClick={() => onDelete(recording.session_id)}
          className="p-1.5 text-muted-foreground hover:text-destructive transition-colors rounded-md hover:bg-destructive/10"
          title="Delete recording"
        >
          <Trash2 className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}

function ReplayListContent() {
  const { selectedDomain } = useSelectedDomain()
  const [page, setPage] = useState(0)
  const limit = 20

  const { data, isLoading } = useReplays({
    domain: selectedDomain?.domain,
    limit,
    offset: page * limit,
  })

  const { data: stats } = useReplayStats()
  const deleteMutation = useDeleteReplay()

  const handleDelete = (sessionId: string) => {
    if (!confirm('Delete this recording?')) return
    deleteMutation.mutate(sessionId, {
      onSuccess: () => toast.success('Recording deleted'),
      onError: () => toast.error('Failed to delete recording'),
    })
  }

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
        <Link
          to="/replays/settings"
          className="text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          Settings
        </Link>
      </div>

      {stats && (
        <div className="grid grid-cols-3 gap-4">
          <div className="rounded-lg border p-4">
            <div className="text-2xl font-bold">{stats.total_recordings.toLocaleString()}</div>
            <div className="text-sm text-muted-foreground">Total recordings</div>
          </div>
          <div className="rounded-lg border p-4">
            <div className="text-2xl font-bold">{formatBytes(stats.disk_usage_bytes)}</div>
            <div className="text-sm text-muted-foreground">Storage used</div>
          </div>
          <div className="rounded-lg border p-4">
            <div className="text-2xl font-bold">
              {stats.quota_bytes > 0
                ? `${Math.round((stats.disk_usage_bytes / stats.quota_bytes) * 100)}%`
                : 'Unlimited'}
            </div>
            <div className="text-sm text-muted-foreground">Quota usage</div>
          </div>
        </div>
      )}

      <div className="rounded-lg border">
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
          </div>
        ) : recordings.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
            <Video className="h-12 w-12 mb-3 opacity-30" />
            <p className="text-lg font-medium">No recordings yet</p>
            <p className="text-sm">Enable session replay in settings to start recording</p>
          </div>
        ) : (
          <>
            {recordings.map((rec) => (
              <RecordingRow
                key={rec.session_id}
                recording={rec}
                onDelete={handleDelete}
              />
            ))}
          </>
        )}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            Showing {page * limit + 1}-{Math.min((page + 1) * limit, total)} of {total}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage(p => Math.max(0, p - 1))}
              disabled={page === 0}
              className="p-1.5 rounded-md border hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <span className="text-sm">{page + 1} / {totalPages}</span>
            <button
              onClick={() => setPage(p => Math.min(totalPages - 1, p + 1))}
              disabled={page >= totalPages - 1}
              className="p-1.5 rounded-md border hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}
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
