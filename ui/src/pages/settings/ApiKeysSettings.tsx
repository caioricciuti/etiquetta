import { useState } from 'react'
import { toast } from 'sonner'
import { useApiKeys, useCreateApiKey, useRevokeApiKey } from '@/hooks/useApiKeys'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { KeyRound, Plus, Copy, Check, Trash2 } from 'lucide-react'
import { SettingsLayout } from './SettingsLayout'

export function ApiKeysSettings() {
  const { data: keys, isLoading } = useApiKeys()
  const createKey = useCreateApiKey()
  const revokeKey = useRevokeApiKey()
  const [name, setName] = useState('')
  const [newKey, setNewKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    createKey.mutate(name, {
      onSuccess: (data) => {
        setNewKey(data.key)
        setName('')
        toast.success('API key created')
      },
    })
  }

  async function handleCopy() {
    if (!newKey) return
    await navigator.clipboard.writeText(newKey)
    setCopied(true)
    toast.success('Copied to clipboard')
    setTimeout(() => setCopied(false), 2000)
  }

  function handleRevoke(id: string) {
    if (!confirm('Are you sure? This key will stop working immediately.')) return
    revokeKey.mutate(id)
  }

  function formatDate(ms: number) {
    return new Date(ms).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  }

  return (
    <SettingsLayout title="API Keys" description="Manage API keys for external integrations like WordPress">
      {/* Create key */}
      <Card>
        <CardHeader>
          <CardTitle>Create API Key</CardTitle>
          <CardDescription>
            API keys allow external services to read your analytics data. The key is shown once after creation.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleCreate} className="flex gap-4">
            <div className="flex-1">
              <Input
                placeholder="Key name (e.g., WordPress site)"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <Button type="submit" disabled={createKey.isPending || !name.trim()}>
              <Plus className="h-4 w-4 mr-2" />
              Create Key
            </Button>
          </form>

          {newKey && (
            <div className="mt-4 p-4 bg-emerald-50 dark:bg-emerald-950 border border-emerald-200 dark:border-emerald-800 rounded-lg">
              <p className="text-sm font-medium text-emerald-800 dark:text-emerald-200 mb-2">
                Copy this key now — it won't be shown again.
              </p>
              <div className="flex items-center gap-2">
                <code className="flex-1 p-2 bg-white dark:bg-zinc-900 rounded text-sm font-mono break-all border">
                  {newKey}
                </code>
                <Button variant="outline" size="sm" onClick={handleCopy}>
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                </Button>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="mt-2 text-xs text-muted-foreground"
                onClick={() => setNewKey(null)}
              >
                Dismiss
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Keys list */}
      <Card>
        <CardHeader>
          <CardTitle>Your API Keys</CardTitle>
          <CardDescription>Active and revoked keys for your account</CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-muted-foreground">Loading...</p>
          ) : !keys || keys.length === 0 ? (
            <p className="text-muted-foreground">No API keys created yet.</p>
          ) : (
            <div className="space-y-3">
              {keys.map((key) => {
                const isRevoked = key.revoked_at !== null && key.revoked_at !== undefined
                return (
                  <div
                    key={key.id}
                    className={`flex items-center justify-between p-4 rounded-lg border ${
                      isRevoked ? 'opacity-50 border-border' : 'border-border'
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      <KeyRound className="h-5 w-5 text-muted-foreground" />
                      <div>
                        <p className="font-medium">
                          {key.name}
                          {isRevoked && (
                            <span className="ml-2 text-xs text-red-500 font-normal">Revoked</span>
                          )}
                        </p>
                        <p className="text-sm text-muted-foreground font-mono">{key.key_prefix}</p>
                        <p className="text-xs text-muted-foreground">
                          Created {formatDate(key.created_at)}
                          {key.last_used_at && ` · Last used ${formatDate(key.last_used_at)}`}
                        </p>
                      </div>
                    </div>
                    {!isRevoked && (
                      <Button variant="outline" size="sm" onClick={() => handleRevoke(key.id)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Info */}
      <Card>
        <CardHeader>
          <CardTitle>About API Keys</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground space-y-2">
          <p>
            API keys provide read-only access to your analytics data via the Etiquetta API.
            Use them to connect external services like the WordPress plugin dashboard widget.
          </p>
          <p>
            Pass the key as a <code className="text-xs bg-muted px-1 rounded">Bearer</code> token
            in the <code className="text-xs bg-muted px-1 rounded">Authorization</code> header.
          </p>
        </CardContent>
      </Card>
    </SettingsLayout>
  )
}
