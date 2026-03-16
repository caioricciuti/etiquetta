import { useContainerVersions, useRollbackContainer } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Loader2, RotateCcw, Clock } from 'lucide-react'

interface VersionHistoryProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  containerId: string
}

export function VersionHistory({ open, onOpenChange, containerId }: VersionHistoryProps) {
  const { data: versions, isLoading } = useContainerVersions(containerId)
  const rollback = useRollbackContainer(containerId)

  function handleRollback(version: number) {
    if (!confirm(`Rollback to version ${version}? This will overwrite the current draft.`)) return
    rollback.mutate(version)
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="sm:max-w-md w-full overflow-y-auto px-4">
        <SheetHeader>
          <SheetTitle>Version History</SheetTitle>
          <SheetDescription>
            Published versions of this container. You can rollback to any previous version.
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto -mx-6 px-6">
          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : !versions || versions.length === 0 ? (
            <div className="text-center py-8">
              <Clock className="h-10 w-10 text-muted-foreground/30 mx-auto mb-3" />
              <p className="text-sm text-muted-foreground">
                No published versions yet.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {versions.map((snapshot, index) => (
                <div
                  key={snapshot.id}
                  className="flex items-center justify-between p-3 rounded-lg border border-border"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-sm">
                        Version {snapshot.version}
                      </span>
                      {index === 0 && (
                        <Badge variant="default" className="text-xs">
                          Latest
                        </Badge>
                      )}
                    </div>
                    <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                      <span>
                        {new Date(snapshot.published_at).toLocaleString()}
                      </span>
                      {snapshot.published_by && (
                        <>
                          <span>by</span>
                          <span>{snapshot.published_by}</span>
                        </>
                      )}
                    </div>
                  </div>
                  {index > 0 && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleRollback(snapshot.version)}
                      disabled={rollback.isPending}
                    >
                      {rollback.isPending ? (
                        <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" />
                      ) : (
                        <RotateCcw className="h-3.5 w-3.5 mr-1" />
                      )}
                      Rollback
                    </Button>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
