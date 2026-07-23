import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@/hooks/useAuth'
import { Navigate } from 'react-router-dom'
import { fetchAPI } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Shield, Plus, Trash2, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { SettingsLayout } from './SettingsLayout'

interface SSOProvider {
  id: string
  name: string
  kind: string
  issuer: string
  client_id: string
  client_secret?: string
  scopes: string
  allowed_domains: string
  default_role: string
  enabled: boolean
  has_secret: boolean
}

function useSSOProviders() {
  return useQuery({
    queryKey: ['sso', 'providers'],
    queryFn: () => fetchAPI<SSOProvider[]>('/api/settings/sso'),
  })
}

function ProviderForm({
  initial,
  onClose,
}: {
  initial?: SSOProvider
  onClose: () => void
}) {
  const qc = useQueryClient()
  const [p, setP] = useState<Partial<SSOProvider>>(
    initial ?? {
      name: '',
      issuer: '',
      client_id: '',
      client_secret: '',
      scopes: 'openid email profile',
      allowed_domains: '',
      default_role: 'viewer',
      enabled: true,
    }
  )

  const save = useMutation({
    mutationFn: () =>
      initial
        ? fetchAPI(`/api/settings/sso/${initial.id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(p),
          })
        : fetchAPI('/api/settings/sso', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(p),
          }),
    onSuccess: () => {
      toast.success(initial ? 'Provider updated' : 'Provider created')
      qc.invalidateQueries({ queryKey: ['sso'] })
      onClose()
    },
    onError: (e: unknown) => toast.error(e instanceof Error ? e.message : 'Failed'),
  })

  function up<K extends keyof SSOProvider>(key: K, v: SSOProvider[K]) {
    setP(prev => ({ ...prev, [key]: v }))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{initial ? 'Edit provider' : 'New OIDC provider'}</CardTitle>
        <CardDescription>
          Works with any OpenID Connect compliant IdP (Google, Microsoft Entra, Okta, Auth0, Keycloak, GitHub via OIDC proxy).
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Display name</Label>
            <Input value={p.name ?? ''} onChange={e => up('name', e.target.value)} placeholder="Google Workspace" />
          </div>
          <div className="space-y-2">
            <Label>Default role for new users</Label>
            <Input value={p.default_role ?? 'viewer'} onChange={e => up('default_role', e.target.value)} />
          </div>
        </div>

        <div className="space-y-2">
          <Label>Issuer URL</Label>
          <Input
            value={p.issuer ?? ''}
            onChange={e => up('issuer', e.target.value)}
            placeholder="https://accounts.google.com"
          />
          <p className="text-xs text-muted-foreground">
            The discovery doc is fetched from <code>{'{issuer}'}/.well-known/openid-configuration</code>.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Client ID</Label>
            <Input value={p.client_id ?? ''} onChange={e => up('client_id', e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>Client secret {initial?.has_secret && <span className="text-xs text-muted-foreground">(leave blank to keep)</span>}</Label>
            <Input
              type="password"
              value={p.client_secret ?? ''}
              onChange={e => up('client_secret', e.target.value)}
              placeholder={initial?.has_secret ? '••••••••' : ''}
            />
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Scopes</Label>
            <Input value={p.scopes ?? ''} onChange={e => up('scopes', e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>Allowed email domains</Label>
            <Input
              value={p.allowed_domains ?? ''}
              onChange={e => up('allowed_domains', e.target.value)}
              placeholder="example.com,partner.com"
            />
            <p className="text-xs text-muted-foreground">Comma separated. Blank = any.</p>
          </div>
        </div>

        <div className="flex items-center justify-between pt-2">
          <div className="flex items-center gap-2">
            <Switch checked={p.enabled ?? true} onCheckedChange={v => up('enabled', v)} />
            <Label>Enabled</Label>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={onClose}>Cancel</Button>
            <Button onClick={() => save.mutate()} disabled={save.isPending}>
              {save.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              Save
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function RedirectURIHint() {
  const url = `${window.location.origin}/api/auth/sso/{provider_id}/callback`
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Redirect URI</CardTitle>
        <CardDescription>
          Register this callback URL with your identity provider.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <code className="text-xs bg-muted p-2 rounded block break-all">{url}</code>
        <p className="text-xs text-muted-foreground mt-2">
          Replace <code>{'{provider_id}'}</code> with the ID shown on each provider row after saving.
        </p>
      </CardContent>
    </Card>
  )
}

export function SSOSettings() {
  const { isAdmin } = useAuth()
  const [editing, setEditing] = useState<SSOProvider | null>(null)
  const [creating, setCreating] = useState(false)
  const qc = useQueryClient()
  const { data, isLoading } = useSSOProviders()

  const del = useMutation({
    mutationFn: (id: string) => fetchAPI(`/api/settings/sso/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('Provider deleted')
      qc.invalidateQueries({ queryKey: ['sso'] })
    },
    onError: (e: unknown) => toast.error(e instanceof Error ? e.message : 'Failed'),
  })

  if (!isAdmin) return <Navigate to="/" replace />

  return (
    <SettingsLayout
      title="Single Sign-On"
      description="Let users log in via your identity provider. OpenID Connect only."
    >
      <RedirectURIHint />

      {creating && <ProviderForm onClose={() => setCreating(false)} />}
      {editing && <ProviderForm initial={editing} onClose={() => setEditing(null)} />}

      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : !data?.length && !creating ? (
        <Card>
          <CardContent className="py-12 text-center">
            <Shield className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
            <h3 className="text-lg font-medium mb-2">No SSO providers</h3>
            <p className="text-sm text-muted-foreground mb-4">
              Add an OIDC provider to let users log in with their corporate identity.
            </p>
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4 mr-1" /> Add provider
            </Button>
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="flex justify-end">
            {!creating && (
              <Button onClick={() => setCreating(true)} size="sm">
                <Plus className="h-4 w-4 mr-1" /> Add provider
              </Button>
            )}
          </div>
          <div className="space-y-3">
            {data?.map(p => (
              <Card key={p.id}>
                <CardContent className="p-4">
                  <div className="flex items-center justify-between gap-4">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <h3 className="font-medium">{p.name}</h3>
                        {p.enabled ? (
                          <span className="text-xs px-2 py-0.5 rounded bg-primary/10 text-primary">Enabled</span>
                        ) : (
                          <span className="text-xs px-2 py-0.5 rounded bg-muted text-muted-foreground">Disabled</span>
                        )}
                      </div>
                      <div className="text-xs text-muted-foreground mt-1 truncate">
                        <span className="font-mono">{p.id}</span> · {p.issuer}
                      </div>
                      {p.allowed_domains && (
                        <div className="text-xs text-muted-foreground mt-0.5">
                          Domains: {p.allowed_domains}
                        </div>
                      )}
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <Button variant="outline" size="sm" onClick={() => setEditing(p)}>
                        Edit
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => {
                          if (confirm(`Delete provider "${p.name}"?`)) del.mutate(p.id)
                        }}
                      >
                        <Trash2 className="h-4 w-4 text-muted-foreground" />
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </>
      )}
    </SettingsLayout>
  )
}
