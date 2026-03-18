import { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useReplay } from '@/hooks/useReplayQueries'
import 'rrweb-player/dist/style.css'
import { FeatureGate } from '@/components/FeatureGate'
import {
  ArrowLeft,
  Clock,
  Loader2,
} from 'lucide-react'
import type { eventWithTime } from 'rrweb'

function formatDuration(ms: number): string {
  const sec = Math.floor(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  const remSec = sec % 60
  return `${min}m ${remSec}s`
}

function ReplayPlayerContent() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const { data, isLoading, error } = useReplay(sessionId ?? null)
  const containerRef = useRef<HTMLDivElement>(null)
  const playerRef = useRef<{ setSpeed: (speed: number) => void; $destroy?: () => void } | null>(null)
  const [speed, setSpeed] = useState(1)
  const [playerReady, setPlayerReady] = useState(false)

  useEffect(() => {
    if (!data?.events?.length || !containerRef.current) return

    // Clear previous player
    if (containerRef.current.firstChild) {
      containerRef.current.innerHTML = ''
    }

    // Dynamically import rrweb-player to avoid SSR issues
    import('rrweb-player').then(({ default: RRWebPlayer }) => {
      if (!containerRef.current) return

      const player = new RRWebPlayer({
        target: containerRef.current,
        props: {
          events: data.events as eventWithTime[],
          width: containerRef.current.clientWidth,
          height: containerRef.current.clientHeight - 80,
          autoPlay: true,
          showController: true,
          skipInactive: true,
          speed,
        },
      })

      playerRef.current = player
      setPlayerReady(true)
    }).catch((err) => {
      console.error('Failed to load rrweb-player:', err)
    })

    return () => {
      playerRef.current = null
      setPlayerReady(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  useEffect(() => {
    if (playerRef.current && playerReady) {
      playerRef.current.setSpeed(speed)
    }
  }, [speed, playerReady])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !data) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <p className="text-muted-foreground">Recording not found</p>
        <Link
          to="/replays"
          className="text-sm text-primary hover:underline flex items-center gap-1"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to recordings
        </Link>
      </div>
    )
  }

  const events = data.events as eventWithTime[]
  const duration = events.length > 1
    ? events[events.length - 1].timestamp - events[0].timestamp
    : 0

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b shrink-0">
        <div className="flex items-center gap-3">
          <Link
            to="/replays"
            className="p-1.5 rounded-md hover:bg-muted transition-colors"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1 className="text-lg font-semibold">Session Replay</h1>
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span className="font-mono">{sessionId?.slice(0, 12)}...</span>
              <span className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                {formatDuration(duration)}
              </span>
              <span>{events.length} events</span>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">Speed:</span>
          {[1, 2, 4, 8].map((s) => (
            <button
              key={s}
              onClick={() => setSpeed(s)}
              className={`px-2 py-1 text-xs rounded-md border transition-colors ${
                speed === s
                  ? 'bg-primary text-primary-foreground border-primary'
                  : 'hover:bg-muted border-border'
              }`}
            >
              {s}x
            </button>
          ))}
        </div>
      </div>

      {/* Player */}
      <div
        ref={containerRef}
        className="flex-1 bg-zinc-950 overflow-hidden"
      />
    </div>
  )
}

export function ReplayPlayer() {
  return (
    <FeatureGate feature="session_replay">
      <ReplayPlayerContent />
    </FeatureGate>
  )
}
