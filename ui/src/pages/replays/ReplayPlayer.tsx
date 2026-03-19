import { useEffect, useRef, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { useReplay, useDeleteReplay, useSessionEvents } from '@/hooks/useReplayQueries'
import { toast } from 'sonner'
import 'rrweb-player/dist/style.css'
import { FeatureGate } from '@/components/FeatureGate'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  AlertTriangle,
  ArrowLeft,
  ChevronDown,
  Clock,
  Copy,
  Loader2,
  MoreHorizontal,
  ListTree,
  Trash2,
} from 'lucide-react'
import type { eventWithTime } from 'rrweb'

const RRWEB_FULL_SNAPSHOT = 2

function formatDuration(ms: number): string {
  const sec = Math.floor(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  const remSec = sec % 60
  return `${min}m ${remSec}s`
}

function formatRelativeTime(ts: number, baseTs: number): string {
  const diff = Math.floor((ts - baseTs) / 1000)
  const min = Math.floor(diff / 60)
  const sec = diff % 60
  return `${min}:${sec.toString().padStart(2, '0')}`
}

function ReplayPlayerContent() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const navigate = useNavigate()
  const { data, isLoading, error } = useReplay(sessionId ?? null)
  const containerRef = useRef<HTMLDivElement>(null)
  const playerRef = useRef<{ setSpeed: (speed: number) => void; $destroy?: () => void } | null>(null)
  const [speed, setSpeed] = useState(1)
  const [playerReady, setPlayerReady] = useState(false)
  const [playerError, setPlayerError] = useState<string | null>(null)
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [eventsOpen, setEventsOpen] = useState(false)

  const deleteMutation = useDeleteReplay()
  const { data: sessionEvents, isLoading: eventsLoading } = useSessionEvents(
    eventsOpen ? (sessionId ?? null) : null
  )

  useEffect(() => {
    if (!data?.events?.length || !containerRef.current) return

    const events = data.events as eventWithTime[]
    const hasFullSnapshot = events.some((e) => e.type === RRWEB_FULL_SNAPSHOT)
    if (!hasFullSnapshot) {
      setPlayerError('This recording is incomplete — no full DOM snapshot was captured. The initial page state may have been too large to transmit.')
      return
    }

    if (containerRef.current.firstChild) {
      containerRef.current.innerHTML = ''
    }
    setPlayerError(null)

    const container = containerRef.current
    requestAnimationFrame(() => {
      if (!container) return

      import('rrweb-player').then(({ default: RRWebPlayer }) => {
        if (!container) return

        try {
          const width = container.clientWidth || 800
          const height = Math.max(container.clientHeight - 80, 400)

          const player = new RRWebPlayer({
            target: container,
            props: {
              events,
              width,
              height,
              autoPlay: true,
              showController: true,
              skipInactive: true,
              speed,
            },
          })

          playerRef.current = player
          setPlayerReady(true)
        } catch (err) {
          console.error('Failed to initialize rrweb-player:', err)
          setPlayerError('Failed to initialize the replay player. The recording data may be corrupt.')
        }
      }).catch((err) => {
        console.error('Failed to load rrweb-player:', err)
        setPlayerError('Failed to load the replay player module.')
      })
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

  const handleDelete = () => {
    if (!sessionId) return
    deleteMutation.mutate(sessionId, {
      onSuccess: () => {
        toast.success('Recording deleted')
        navigate('/replays')
      },
      onError: () => toast.error('Failed to delete recording'),
    })
  }

  const copySessionId = () => {
    if (sessionId) {
      navigator.clipboard.writeText(sessionId)
      toast.success('Session ID copied')
    }
  }

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
        <Button variant="ghost" size="sm" asChild>
          <Link to="/replays">
            <ArrowLeft className="h-4 w-4 mr-1" />
            Back to recordings
          </Link>
        </Button>
      </div>
    )
  }

  const events = data.events as eventWithTime[]
  const duration = events.length > 1
    ? events[events.length - 1].timestamp - events[0].timestamp
    : 0
  const meta = data.metadata
  const baseTimestamp = sessionEvents?.[0]?.timestamp ?? 0

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b shrink-0">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" asChild>
            <Link to="/replays">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <h1 className="text-lg font-semibold">Session Replay</h1>
            <div className="flex items-center gap-2 mt-0.5">
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      onClick={copySessionId}
                      className="font-mono text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
                    >
                      {sessionId?.slice(0, 16)}...
                      <Copy className="h-3 w-3" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>Click to copy full session ID</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
            {meta && (
              <div className="flex items-center gap-1.5 mt-1.5 flex-wrap">
                {meta.device_type && (
                  <Badge variant="outline" className="text-xs">{meta.device_type}</Badge>
                )}
                {meta.browser_name && (
                  <Badge variant="secondary" className="text-xs">{meta.browser_name}</Badge>
                )}
                {meta.os_name && (
                  <Badge variant="secondary" className="text-xs">{meta.os_name}</Badge>
                )}
                {meta.screen_width > 0 && (
                  <Badge variant="secondary" className="text-xs">
                    {meta.screen_width}x{meta.screen_height}
                  </Badge>
                )}
                {meta.geo_country && (
                  <Badge variant="secondary" className="text-xs">{meta.geo_country}</Badge>
                )}
                <span className="text-xs text-muted-foreground ml-1">
                  {meta.first_url && <>{meta.first_url} &middot; </>}
                  {meta.pages > 0 && <>{meta.pages} page{meta.pages !== 1 ? 's' : ''} &middot; </>}
                  <Clock className="h-3 w-3 inline mr-0.5" />
                  {formatDuration(duration)} &middot; {events.length} events
                </span>
              </div>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground mr-1">Speed:</span>
          {[1, 2, 4, 8].map((s) => (
            <Button
              key={s}
              size="sm"
              variant={speed === s ? 'default' : 'outline'}
              onClick={() => setSpeed(s)}
              className="h-7 px-2 text-xs"
            >
              {s}x
            </Button>
          ))}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="ml-1">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setEventsOpen((o) => !o)}>
                <ListTree className="h-4 w-4 mr-2" />
                {eventsOpen ? 'Hide' : 'Show'} session events
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onClick={() => setShowDeleteDialog(true)}
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete recording
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Session events panel */}
      <Collapsible open={eventsOpen} onOpenChange={setEventsOpen}>
        <CollapsibleContent>
          <Card className="rounded-none border-x-0 border-t-0">
            <CollapsibleTrigger asChild>
              <button className="flex items-center gap-2 px-4 py-2 w-full text-left text-sm font-medium hover:bg-muted/50 transition-colors">
                <ChevronDown className="h-4 w-4" />
                Session Events ({sessionEvents?.length ?? 0})
              </button>
            </CollapsibleTrigger>
            <CardContent className="p-0">
              <div className="max-h-[250px] overflow-y-auto">
                {eventsLoading ? (
                  <div className="flex items-center justify-center py-6">
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                  </div>
                ) : !sessionEvents?.length ? (
                  <p className="text-sm text-muted-foreground px-4 py-4">No analytics events for this session</p>
                ) : (
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-xs text-muted-foreground">
                        <th className="px-4 py-2 w-16">Time</th>
                        <th className="px-4 py-2 w-28">Type</th>
                        <th className="px-4 py-2">Name / Path</th>
                        <th className="px-4 py-2 hidden lg:table-cell">URL</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sessionEvents.map((ev) => (
                        <tr key={ev.id} className="border-b border-border/50 hover:bg-muted/30">
                          <td className="px-4 py-1.5 font-mono text-xs text-muted-foreground">
                            {formatRelativeTime(ev.timestamp, baseTimestamp)}
                          </td>
                          <td className="px-4 py-1.5">
                            <Badge variant="outline" className="text-xs">
                              {ev.event_type}
                            </Badge>
                          </td>
                          <td className="px-4 py-1.5 truncate max-w-[250px]">
                            {ev.event_name || ev.path}
                          </td>
                          <td className="px-4 py-1.5 truncate max-w-[300px] text-xs text-muted-foreground hidden lg:table-cell">
                            {ev.url}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </CardContent>
          </Card>
        </CollapsibleContent>
      </Collapsible>

      {/* Player */}
      {playerError ? (
        <div className="flex-1 bg-muted/50 dark:bg-zinc-950 flex items-center justify-center">
          <div className="flex flex-col items-center gap-3 text-center max-w-md px-4">
            <AlertTriangle className="h-8 w-8 text-yellow-500" />
            <p className="text-sm text-muted-foreground">{playerError}</p>
          </div>
        </div>
      ) : (
        <div
          ref={containerRef}
          className="flex-1 bg-muted/50 dark:bg-zinc-950 overflow-hidden"
        />
      )}

      {/* Delete dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete recording</DialogTitle>
            <DialogDescription>
              This will permanently delete the session recording and its data. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteDialog(false)}>
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

export function ReplayPlayer() {
  return (
    <FeatureGate feature="session_replay">
      <ReplayPlayerContent />
    </FeatureGate>
  )
}
