import { useState } from 'react'
import { usePublishContainer, usePreviewToken } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { VersionHistory } from './VersionHistory'
import { Rocket, History, Loader2, Eye } from 'lucide-react'
import type { TMContainer } from '@/lib/types'

interface PublishBarProps {
  container: TMContainer
}

export function PublishBar({ container }: PublishBarProps) {
  const publishContainer = usePublishContainer(container.id)
  const previewToken = usePreviewToken(container.id)
  const [historyOpen, setHistoryOpen] = useState(false)

  const hasUnpublishedChanges = container.draft_version > container.published_version
  const changeCount = container.draft_version - container.published_version

  function handlePublish() {
    if (!confirm('Publish all changes? This will make the current draft live.')) return
    publishContainer.mutate()
  }

  function handlePreview() {
    previewToken.mutate(undefined, {
      onSuccess: (data) => {
        const domain = data.domain || 'localhost'
        const protocol = domain.includes('localhost') ? 'http' : 'https'
        const url = `${protocol}://${domain}?etq_debug=${data.token}&etq_site=${data.site_id}`
        window.open(url, '_blank')
      },
    })
  }

  return (
    <>
      <div className="sticky top-0 z-30 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center justify-between px-6 py-2.5 max-w-6xl mx-auto w-full">
          <div className="flex items-center gap-3">
            {hasUnpublishedChanges ? (
              <Badge variant="secondary" className="font-normal">
                {changeCount} unpublished change{changeCount !== 1 ? 's' : ''}
              </Badge>
            ) : (
              <span className="text-sm text-muted-foreground">All changes published</span>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handlePreview}
              disabled={previewToken.isPending}
            >
              {previewToken.isPending ? (
                <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
              ) : (
                <Eye className="h-4 w-4 mr-1.5" />
              )}
              Preview
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setHistoryOpen(true)}
            >
              <History className="h-4 w-4 mr-1.5" />
              Version History
            </Button>
            <Button
              size="sm"
              onClick={handlePublish}
              disabled={!hasUnpublishedChanges || publishContainer.isPending}
            >
              {publishContainer.isPending ? (
                <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
              ) : (
                <Rocket className="h-4 w-4 mr-1.5" />
              )}
              Publish
            </Button>
          </div>
        </div>
      </div>

      <VersionHistory
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        containerId={container.id}
      />
    </>
  )
}
