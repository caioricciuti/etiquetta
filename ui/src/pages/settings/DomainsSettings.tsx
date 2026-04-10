import { useState } from 'react'
import { toast } from 'sonner'
import { useDomains, useCreateDomain, useDeleteDomain } from '@/hooks/useDomains'
import { useShareLinks, useCreateShareLink, useDeleteShareLink } from '@/hooks/useShareLinks'
import { useDomainStore } from '@/stores/useDomainStore'
import { fetchAPI } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Globe, Copy, Trash2, Plus, Check, Link2, ExternalLink } from 'lucide-react'
import { SettingsLayout } from './SettingsLayout'

export function DomainsSettings() {
  const { data: domains, isLoading } = useDomains()
  const createDomain = useCreateDomain()
  const deleteDomain = useDeleteDomain()
  const [newDomain, setNewDomain] = useState({ name: '', domain: '' })
  const [copiedId, setCopiedId] = useState<string | null>(null)

  async function handleAddDomain(e: React.FormEvent) {
    e.preventDefault()
    if (!newDomain.name || !newDomain.domain) return
    createDomain.mutate(newDomain, {
      onSuccess: () => setNewDomain({ name: '', domain: '' }),
    })
  }

  async function handleDeleteDomain(id: string) {
    if (!confirm('Are you sure you want to delete this domain?')) return
    deleteDomain.mutate(id)
  }

  async function copySnippet(id: string) {
    try {
      const data = await fetchAPI<{ snippet: string }>(`/api/domains/${id}/snippet`)
      await navigator.clipboard.writeText(data.snippet)
      setCopiedId(id)
      toast.success('Snippet copied to clipboard')
      setTimeout(() => setCopiedId(null), 2000)
    } catch {
      toast.error('Failed to copy snippet')
    }
  }

  return (
    <SettingsLayout title="Properties" description="Manage your tracked properties">
      <Card>
        <CardHeader>
          <CardTitle>Add Property</CardTitle>
          <CardDescription>Register a domain to start tracking analytics</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleAddDomain} className="flex gap-4">
            <div className="flex-1">
              <Input
                placeholder="Site name (e.g., My Blog)"
                value={newDomain.name}
                onChange={(e) => setNewDomain({ ...newDomain, name: e.target.value })}
              />
            </div>
            <div className="flex-1">
              <Input
                placeholder="Domain (e.g., blog.example.com)"
                value={newDomain.domain}
                onChange={(e) => setNewDomain({ ...newDomain, domain: e.target.value })}
              />
            </div>
            <Button type="submit" disabled={createDomain.isPending}>
              <Plus className="h-4 w-4 mr-2" />
              Add
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Registered Properties</CardTitle>
          <CardDescription>Click the copy button to get the tracking snippet for each property</CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-muted-foreground">Loading...</p>
          ) : !domains || domains.length === 0 ? (
            <p className="text-muted-foreground">No properties registered yet.</p>
          ) : (
            <div className="space-y-3">
              {domains.map((domain) => (
                <div key={domain.id} className="flex items-center justify-between p-4 rounded-lg border border-border">
                  <div className="flex items-center gap-3">
                    <Globe className="h-5 w-5 text-muted-foreground" />
                    <div>
                      <p className="font-medium">{domain.name}</p>
                      <p className="text-sm text-muted-foreground">{domain.domain}</p>
                      <p className="text-xs text-muted-foreground font-mono">{domain.site_id}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="outline" size="sm" onClick={() => copySnippet(domain.id)}>
                      {copiedId === domain.id ? (
                        <><Check className="h-4 w-4 mr-1" />Copied</>
                      ) : (
                        <><Copy className="h-4 w-4 mr-1" />Copy Snippet</>
                      )}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => handleDeleteDomain(domain.id)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Tracking Snippet</CardTitle>
          <CardDescription>
            Add this script to your website. Each domain has a unique <code className="text-xs bg-muted px-1 rounded">data-site</code> ID that ensures only your registered domains can send analytics data.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="p-4 bg-zinc-100 dark:bg-zinc-800 rounded-lg text-sm overflow-x-auto">
            <code>{`<!-- Etiquetta Analytics -->
<script defer data-site="YOUR_SITE_ID" src="${window.location.origin}/s.js?id=YOUR_SITE_ID"></script>`}</code>
          </pre>
          <p className="text-xs text-muted-foreground mt-3">
            Click "Copy Snippet" on a domain above to get the snippet with the correct site ID.
          </p>
        </CardContent>
      </Card>

      <ShareLinksCard />
    </SettingsLayout>
  )
}

function ShareLinksCard() {
  const { data: domains } = useDomains()
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const setSelectedDomainId = useDomainStore(s => s.setSelectedDomainId)
  const { data: links } = useShareLinks()
  const createLink = useCreateShareLink()
  const deleteLink = useDeleteShareLink()
  const [copiedToken, setCopiedToken] = useState<string | null>(null)

  function handleCreate() {
    if (!selectedDomainId) return
    createLink.mutate({ domain_id: selectedDomainId }, {
      onSuccess: () => toast.success('Share link created'),
      onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to create'),
    })
  }

  function copyLink(token: string) {
    const url = `${window.location.origin}/shared/${token}`
    navigator.clipboard.writeText(url)
    setCopiedToken(token)
    toast.success('Link copied')
    setTimeout(() => setCopiedToken(null), 2000)
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Link2 className="h-5 w-5" />
              Public Dashboard Links
            </CardTitle>
            <CardDescription>
              Share a read-only view of your analytics with anyone.
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Select value={selectedDomainId ?? ''} onValueChange={setSelectedDomainId}>
              <SelectTrigger className="w-[200px]">
                <SelectValue placeholder="Select property" />
              </SelectTrigger>
              <SelectContent>
                {domains?.map((d) => (
                  <SelectItem key={d.id} value={d.id}>{d.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {selectedDomainId && (
              <Button size="sm" onClick={handleCreate} disabled={createLink.isPending}>
                <Plus className="h-4 w-4 mr-1" /> New Link
              </Button>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {!selectedDomainId ? (
          <p className="text-sm text-muted-foreground">Select a property to manage its share links.</p>
        ) : !links?.length ? (
          <p className="text-sm text-muted-foreground">No share links yet. Create one to share your dashboard publicly.</p>
        ) : (
          <div className="space-y-3">
            {links.map((link) => (
              <div key={link.id} className="flex items-center justify-between p-3 rounded-lg border">
                <div className="flex-1 mr-4">
                  <p className="text-sm font-medium">{link.name}</p>
                  <p className="text-xs text-muted-foreground font-mono truncate">
                    {window.location.origin}/shared/{link.token}
                  </p>
                  {link.expires_at && (
                    <p className="text-xs text-muted-foreground">
                      Expires: {new Date(link.expires_at).toLocaleDateString()}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="icon" onClick={() => copyLink(link.token)}>
                    {copiedToken === link.token ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                  <Button variant="ghost" size="icon" asChild>
                    <a href={`/shared/${link.token}`} target="_blank" rel="noopener noreferrer">
                      <ExternalLink className="h-4 w-4" />
                    </a>
                  </Button>
                  <Button variant="ghost" size="icon" onClick={() => deleteLink.mutate(link.id)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
