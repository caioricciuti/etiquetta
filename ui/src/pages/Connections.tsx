import { useState } from 'react'
import { toast } from 'sonner'
import {
  useConnections,
  useProviders,
  useCreateConnection,
  useDeleteConnection,
  useSyncConnection,
  useUpdateConnectionToken,
  useAdAttribution,
} from '../hooks/useAnalyticsQueries'
import { FeatureGate } from '../components/FeatureGate'
import { useLicense } from '../hooks/useLicenseQuery'
import type { AdConnection, AdProvider } from '../lib/types'
import {
  Plus,
  Trash2,
  RefreshCw,
  ExternalLink,
  AlertCircle,
  CheckCircle,
  Clock,
  Unplug,
  Cable,
  KeyRound,
} from 'lucide-react'

const PROVIDER_META: Record<string, { color: string; logo: string }> = {
  google_ads: { color: 'bg-blue-500', logo: 'G' },
  meta_ads: { color: 'bg-indigo-500', logo: 'M' },
  microsoft_ads: { color: 'bg-cyan-500', logo: 'MS' },
}

function StatusBadge({ status }: { status: AdConnection['status'] }) {
  const config = {
    active: { icon: CheckCircle, label: 'Active', classes: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' },
    pending: { icon: Clock, label: 'Pending', classes: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' },
    error: { icon: AlertCircle, label: 'Error', classes: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' },
    disconnected: { icon: Unplug, label: 'Disconnected', classes: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-400' },
  }

  const { icon: Icon, label, classes } = config[status] ?? config.disconnected
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${classes}`}>
      <Icon className="h-3 w-3" />
      {label}
    </span>
  )
}

function TokenForm({ connId, onDone }: { connId: string; onDone: () => void }) {
  const [token, setToken] = useState('')
  const updateToken = useUpdateConnectionToken()

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!token.trim()) return
    updateToken.mutate(
      { id: connId, refresh_token: token.trim() },
      {
        onSuccess: () => {
          toast.success('Token validated — connection is now active')
          onDone()
        },
        onError: (err) => toast.error(`Invalid token: ${err.message}`),
      },
    )
  }

  return (
    <form onSubmit={handleSubmit} className="mt-3 space-y-2">
      <textarea
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="Paste your OAuth refresh/access token here"
        rows={2}
        className="w-full rounded-md border border-border bg-background px-3 py-2 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
      />
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={!token.trim() || updateToken.isPending}
          className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {updateToken.isPending ? 'Validating...' : 'Save Token'}
        </button>
        <button
          type="button"
          onClick={onDone}
          className="rounded-md px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}

function ConnectionCard({
  conn,
  onDelete,
  onSync,
}: {
  conn: AdConnection
  onDelete: (id: string) => void
  onSync: (id: string) => void
}) {
  const [showTokenForm, setShowTokenForm] = useState(false)
  const meta = PROVIDER_META[conn.provider] ?? { color: 'bg-zinc-500', logo: '?' }

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-3">
          <div className={`h-10 w-10 rounded-lg ${meta.color} flex items-center justify-center text-white font-bold text-sm`}>
            {meta.logo}
          </div>
          <div>
            <h3 className="font-medium text-foreground">{conn.name}</h3>
            <p className="text-xs text-muted-foreground capitalize">{conn.provider.replace('_', ' ')}</p>
          </div>
        </div>
        <StatusBadge status={conn.status} />
      </div>

      {conn.account_id && (
        <p className="mt-2 text-xs text-muted-foreground">
          Account: {conn.account_id}
        </p>
      )}

      {conn.last_error && (
        <div className="mt-2 rounded bg-red-50 dark:bg-red-900/20 p-2 text-xs text-red-700 dark:text-red-400">
          {conn.last_error}
        </div>
      )}

      {(conn.status === 'pending' || conn.status === 'error') && !showTokenForm && (
        <button
          onClick={() => setShowTokenForm(true)}
          className="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 transition-colors"
        >
          <KeyRound className="h-4 w-4" />
          Add Token
        </button>
      )}

      {showTokenForm && (
        <TokenForm connId={conn.id} onDone={() => setShowTokenForm(false)} />
      )}

      <div className="mt-3 flex items-center justify-between">
        <p className="text-xs text-muted-foreground">
          {conn.last_sync_at
            ? `Last synced: ${new Date(conn.last_sync_at).toLocaleString()}`
            : 'Never synced'}
        </p>
        <div className="flex gap-1">
          {conn.status === 'active' && (
            <button
              onClick={() => setShowTokenForm(true)}
              className="rounded p-1.5 hover:bg-muted transition-colors"
              title="Update token"
            >
              <KeyRound className="h-4 w-4 text-muted-foreground" />
            </button>
          )}
          <button
            onClick={() => onSync(conn.id)}
            className="rounded p-1.5 hover:bg-muted transition-colors"
            title="Sync now"
          >
            <RefreshCw className="h-4 w-4 text-muted-foreground" />
          </button>
          <button
            onClick={() => onDelete(conn.id)}
            className="rounded p-1.5 hover:bg-red-100 dark:hover:bg-red-900/30 transition-colors"
            title="Delete connection"
          >
            <Trash2 className="h-4 w-4 text-red-500" />
          </button>
        </div>
      </div>
    </div>
  )
}

function AddConnectionDialog({
  providers,
  maxReached,
  onAdd,
  onClose,
}: {
  providers: AdProvider[]
  maxReached: boolean
  onAdd: (provider: string, name: string, accountId: string, refreshToken: string) => void
  onClose: () => void
}) {
  const [selectedProvider, setSelectedProvider] = useState('')
  const [name, setName] = useState('')
  const [accountId, setAccountId] = useState('')
  const [refreshToken, setRefreshToken] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedProvider || !name) return
    onAdd(selectedProvider, name, accountId, refreshToken)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-card rounded-lg border border-border shadow-lg w-full max-w-md p-6">
        <h2 className="text-lg font-semibold text-foreground mb-4">Add Connection</h2>

        {maxReached ? (
          <div className="text-center py-4">
            <AlertCircle className="h-8 w-8 text-amber-500 mx-auto mb-2" />
            <p className="text-sm text-muted-foreground">
              You've reached the connection limit for your plan.
              Upgrade to Pro for unlimited connections.
            </p>
            <a
              href="https://etiquetta.com/pricing"
              target="_blank"
              rel="noopener noreferrer"
              className="mt-3 inline-flex items-center gap-1 text-sm text-blue-600 dark:text-blue-400 hover:underline"
            >
              View plans <ExternalLink className="h-3 w-3" />
            </a>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="text-sm font-medium text-foreground mb-1.5 block">Provider</label>
              <div className="grid grid-cols-1 gap-2">
                {providers.map((p) => {
                  const meta = PROVIDER_META[p.name] ?? { color: 'bg-zinc-500', logo: '?' }
                  return (
                    <button
                      key={p.name}
                      type="button"
                      disabled={!p.available}
                      onClick={() => setSelectedProvider(p.name)}
                      className={`flex items-center gap-3 rounded-lg border p-3 text-left transition-colors ${
                        selectedProvider === p.name
                          ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                          : 'border-border hover:bg-muted'
                      } ${!p.available ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
                    >
                      <div className={`h-8 w-8 rounded ${meta.color} flex items-center justify-center text-white font-bold text-xs`}>
                        {meta.logo}
                      </div>
                      <div className="flex-1">
                        <span className="text-sm font-medium">{p.display_name}</span>
                        {!p.available && (
                          <span className="ml-2 text-xs text-muted-foreground">Coming soon</span>
                        )}
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>

            {selectedProvider && (
              <>
                <div>
                  <label className="text-sm font-medium text-foreground mb-1.5 block">
                    Connection Name
                  </label>
                  <input
                    type="text"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. My Google Ads Account"
                    className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    required
                  />
                </div>
                <div>
                  <label className="text-sm font-medium text-foreground mb-1.5 block">
                    Account ID <span className="text-muted-foreground">(optional)</span>
                  </label>
                  <input
                    type="text"
                    value={accountId}
                    onChange={(e) => setAccountId(e.target.value)}
                    placeholder="e.g. 123-456-7890"
                    className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                  />
                </div>
                <div>
                  <label className="text-sm font-medium text-foreground mb-1.5 block">
                    Refresh Token <span className="text-muted-foreground">(optional — can add later)</span>
                  </label>
                  <textarea
                    value={refreshToken}
                    onChange={(e) => setRefreshToken(e.target.value)}
                    placeholder={
                      selectedProvider === 'meta_ads'
                        ? 'Paste your Meta access token (System User or long-lived)'
                        : selectedProvider === 'microsoft_ads'
                          ? 'Paste your Microsoft OAuth refresh token'
                          : 'Paste your Google OAuth refresh token'
                    }
                    rows={2}
                    className="w-full rounded-md border border-border bg-background px-3 py-2 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                  />
                  <p className="text-xs text-muted-foreground mt-1">
                    See Settings &rarr;{' '}
                    {selectedProvider === 'meta_ads'
                      ? 'Meta Ads'
                      : selectedProvider === 'microsoft_ads'
                        ? 'Microsoft Ads'
                        : 'Google Ads'}{' '}
                    for instructions on obtaining a token.
                  </p>
                </div>
              </>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={onClose}
                className="rounded-md px-4 py-2 text-sm font-medium text-muted-foreground hover:bg-muted transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={!selectedProvider || !name}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              >
                Add Connection
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}

function AttributionTable() {
  const { data: attribution, isLoading } = useAdAttribution()

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-2">
        {[1, 2, 3].map((i) => (
          <div key={i} className="h-10 rounded bg-muted" />
        ))}
      </div>
    )
  }

  if (!attribution || attribution.length === 0) {
    return (
      <p className="text-sm text-muted-foreground text-center py-8">
        No attribution data yet. Connect an ad platform and ensure your campaigns use UTM parameters.
      </p>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border">
            <th className="text-left py-2 px-3 font-medium text-muted-foreground">Campaign</th>
            <th className="text-left py-2 px-3 font-medium text-muted-foreground">Source</th>
            <th className="text-right py-2 px-3 font-medium text-muted-foreground">Spend</th>
            <th className="text-right py-2 px-3 font-medium text-muted-foreground">Clicks</th>
            <th className="text-right py-2 px-3 font-medium text-muted-foreground">Visits</th>
            <th className="text-right py-2 px-3 font-medium text-muted-foreground">Visitors</th>
            <th className="text-right py-2 px-3 font-medium text-muted-foreground">CPC</th>
            <th className="text-right py-2 px-3 font-medium text-muted-foreground">CPA</th>
          </tr>
        </thead>
        <tbody>
          {attribution.map((row, i) => (
            <tr key={`${row.campaign}-${row.source}-${i}`} className="border-b border-border/50 hover:bg-muted/50">
              <td className="py-2 px-3 font-medium truncate max-w-[200px]">{row.campaign}</td>
              <td className="py-2 px-3 text-muted-foreground">{row.source}</td>
              <td className="py-2 px-3 text-right">{row.cost > 0 ? `$${row.cost.toFixed(2)}` : '-'}</td>
              <td className="py-2 px-3 text-right">{row.ad_clicks > 0 ? row.ad_clicks.toLocaleString() : '-'}</td>
              <td className="py-2 px-3 text-right">{row.visits.toLocaleString()}</td>
              <td className="py-2 px-3 text-right">{row.visitors.toLocaleString()}</td>
              <td className="py-2 px-3 text-right">{row.cpc > 0 ? `$${row.cpc.toFixed(2)}` : '-'}</td>
              <td className="py-2 px-3 text-right">{row.cpa > 0 ? `$${row.cpa.toFixed(2)}` : '-'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ConnectionsContent() {
  const [showAdd, setShowAdd] = useState(false)
  const { data: connectionsList, isLoading: connectionsLoading } = useConnections()
  const { data: providersList } = useProviders()
  const createConnection = useCreateConnection()
  const deleteConnection = useDeleteConnection()
  const syncConnection = useSyncConnection()
  const { license } = useLicense()

  const maxConnections = license?.limits?.max_connections ?? 1
  const currentCount = connectionsList?.length ?? 0
  const maxReached = maxConnections >= 0 && currentCount >= maxConnections

  const handleAdd = (provider: string, name: string, accountId: string, refreshToken: string) => {
    createConnection.mutate(
      {
        provider,
        name,
        account_id: accountId,
        refresh_token: refreshToken || undefined,
      },
      {
        onSuccess: () => {
          toast.success(refreshToken ? 'Connection created and activated' : 'Connection created — add a token to activate it')
          setShowAdd(false)
        },
        onError: (err) => toast.error(`Failed to create connection: ${err.message}`),
      },
    )
  }

  const handleDelete = (id: string) => {
    if (!confirm('Delete this connection and all its synced data?')) return
    deleteConnection.mutate(id, {
      onSuccess: () => toast.success('Connection deleted'),
      onError: (err) => toast.error(`Failed to delete: ${err.message}`),
    })
  }

  const handleSync = (id: string) => {
    syncConnection.mutate(id, {
      onSuccess: () => toast.success('Sync complete'),
      onError: (err) => toast.error(`Sync failed: ${err.message}`),
    })
  }

  return (
    <div className="p-6 space-y-8 max-w-6xl mx-auto overflow-y-auto h-full">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Connections</h1>
          <p className="text-muted-foreground text-sm">
            Connect ad platforms to track spend, ROI, and campaign attribution.
          </p>
        </div>
        <button
          onClick={() => setShowAdd(true)}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 transition-colors"
        >
          <Plus className="h-4 w-4" />
          Add Connection
        </button>
      </div>

      {/* Connections grid */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground mb-3">Active Connections</h2>
        {connectionsLoading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {[1, 2].map((i) => (
              <div key={i} className="h-32 rounded-lg border border-border animate-pulse bg-muted" />
            ))}
          </div>
        ) : !connectionsList || connectionsList.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border p-8 text-center">
            <Cable className="h-10 w-10 text-muted-foreground mx-auto mb-3" />
            <h3 className="font-medium text-foreground">No connections yet</h3>
            <p className="text-sm text-muted-foreground mt-1">
              Connect your Google Ads, Meta, or Microsoft Ads account to see campaign performance here.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {connectionsList.map((conn) => (
              <ConnectionCard
                key={conn.id}
                conn={conn}
                onDelete={handleDelete}
                onSync={handleSync}
              />
            ))}
          </div>
        )}
      </section>

      {/* Attribution table */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground mb-3">Campaign Attribution</h2>
        <div className="rounded-lg border border-border bg-card p-4">
          <AttributionTable />
        </div>
      </section>

      {/* Add connection dialog */}
      {showAdd && providersList && (
        <AddConnectionDialog
          providers={providersList}
          maxReached={maxReached}
          onAdd={handleAdd}
          onClose={() => setShowAdd(false)}
        />
      )}
    </div>
  )
}

export function Connections() {
  return (
    <FeatureGate feature="connections">
      <ConnectionsContent />
    </FeatureGate>
  )
}
