import { useNavigate } from 'react-router-dom'
import { useContainers, useCreateContainer } from '@/hooks/useTagManager'
import { useSelectedDomain } from '@/hooks/useSelectedDomain'
import { FeatureGate } from '@/components/FeatureGate'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tags, Plus, ExternalLink, Loader2, Globe } from 'lucide-react'
import type { TMContainer, Domain } from '@/lib/types'

function ContainerCard({
  domain,
  container,
  onOpen,
  onCreate,
  isCreating,
}: {
  domain: Domain
  container: TMContainer | undefined
  onOpen: (id: string) => void
  onCreate: (domainId: string) => void
  isCreating: boolean
}) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2">
            <Globe className="h-5 w-5 text-muted-foreground" />
            <div>
              <CardTitle className="text-base">{domain.name}</CardTitle>
              <CardDescription className="text-xs">{domain.domain}</CardDescription>
            </div>
          </div>
          {container && (
            <Badge variant={container.published_version > 0 ? 'default' : 'secondary'}>
              {container.published_version > 0
                ? `v${container.published_version}`
                : 'Unpublished'}
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {container ? (
          <div className="space-y-3">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Container</span>
              <span className="font-medium">{container.name}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Draft version</span>
              <span className="font-mono text-xs">v{container.draft_version}</span>
            </div>
            {container.published_at && (
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Last published</span>
                <span className="text-xs">
                  {new Date(container.published_at).toLocaleDateString()}
                </span>
              </div>
            )}
            <Button
              className="w-full"
              onClick={() => onOpen(container.id)}
            >
              <ExternalLink className="h-4 w-4 mr-2" />
              Open Container
            </Button>
          </div>
        ) : (
          <div className="text-center py-4">
            <p className="text-sm text-muted-foreground mb-3">No container configured</p>
            <Button
              variant="outline"
              onClick={() => onCreate(domain.id)}
              disabled={isCreating}
            >
              {isCreating ? (
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              ) : (
                <Plus className="h-4 w-4 mr-2" />
              )}
              Create Container
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function TagManagerContent() {
  const navigate = useNavigate()
  const { data: containers, isLoading: containersLoading } = useContainers()
  const { selectedDomain, domains, isLoading: domainsLoading } = useSelectedDomain()
  const createContainer = useCreateContainer()

  const isLoading = containersLoading || domainsLoading

  function getContainerForDomain(domainId: string): TMContainer | undefined {
    return containers?.find((c) => c.domain_id === domainId)
  }

  function handleOpen(containerId: string) {
    navigate(`/tag-manager/${containerId}`)
  }

  function handleCreate(domainId: string) {
    if (!selectedDomain) return
    createContainer.mutate(
      { domain_id: domainId, name: `${selectedDomain.name} Container` },
      {
        onSuccess: (container) => {
          navigate(`/tag-manager/${container.id}`)
        },
      }
    )
  }

  return (
    <div className="p-6 max-w-6xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
            <Tags className="h-6 w-6" />
            Tag Manager
          </h1>
          <p className="text-muted-foreground mt-1">
            Manage tags, triggers, and variables for your domain.
          </p>
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : !domains || domains.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center">
            <Globe className="h-10 w-10 text-muted-foreground/30 mx-auto mb-3" />
            <p className="text-muted-foreground">
              No domains registered. Add a domain in Settings first.
            </p>
          </CardContent>
        </Card>
      ) : !selectedDomain ? (
        <Card>
          <CardContent className="py-12 text-center">
            <Globe className="h-10 w-10 text-muted-foreground/30 mx-auto mb-3" />
            <p className="text-muted-foreground">
              Select a domain from the sidebar to manage its container.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="max-w-md">
          <ContainerCard
            domain={selectedDomain}
            container={getContainerForDomain(selectedDomain.id)}
            onOpen={handleOpen}
            onCreate={handleCreate}
            isCreating={createContainer.isPending}
          />
        </div>
      )}
    </div>
  )
}

export function TagManager() {
  return (
    <FeatureGate feature="tag_manager">
      <TagManagerContent />
    </FeatureGate>
  )
}
