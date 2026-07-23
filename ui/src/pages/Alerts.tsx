import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Skeleton } from '../components/ui/skeleton'
import { Switch } from '../components/ui/switch'
import { Bell, Plus, Trash2, Loader2, Webhook, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { toast } from 'sonner'
import {
  useAlerts,
  useAlertFires,
  useCreateAlert,
  useUpdateAlert,
  useDeleteAlert,
  useWebhooks,
  useCreateWebhook,
  useDeleteWebhook,
  useTestWebhook,
  type Alert,
} from '../hooks/useAlerts'
import { useDomainStore } from '../stores/useDomainStore'

const METRICS = [
  { value: 'pageviews', label: 'Pageviews' },
  { value: 'visitors', label: 'Unique visitors' },
  { value: 'errors_count', label: 'Error count' },
  { value: 'error_rate', label: 'Error rate (%)' },
  { value: 'bot_rate', label: 'Bot rate (%)' },
]

function CreateAlertForm({ onClose }: { onClose: () => void }) {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const createAlert = useCreateAlert()
  const [name, setName] = useState('')
  const [metric, setMetric] = useState('pageviews')
  const [comparator, setComparator] = useState<'gt' | 'lt'>('gt')
  const [threshold, setThreshold] = useState('100')
  const [windowMinutes, setWindowMinutes] = useState('60')
  const [cooldownMinutes, setCooldownMinutes] = useState('60')
  const [emailTo, setEmailTo] = useState('')
  const [slackUrl, setSlackUrl] = useState('')
  const [enableWebhooks, setEnableWebhooks] = useState(false)

  function handleSubmit() {
    if (!name.trim() || !selectedDomainId) return
    const channels: string[] = []
    const recipients: string[] = []
    if (emailTo.trim()) {
      channels.push('email')
      emailTo.split(',').map(s => s.trim()).filter(Boolean).forEach(e => recipients.push(e))
    }
    if (slackUrl.trim()) {
      channels.push('slack')
      recipients.push('slack:' + slackUrl.trim())
    }
    if (enableWebhooks) channels.push('webhook')
    if (channels.length === 0) {
      toast.error('Configure at least one channel')
      return
    }

    createAlert.mutate({
      domain_id: selectedDomainId,
      name,
      metric,
      comparator,
      threshold: Number(threshold),
      window_minutes: Number(windowMinutes),
      cooldown_minutes: Number(cooldownMinutes),
      channels,
      recipients,
      enabled: true,
    }, {
      onSuccess: () => { toast.success('Alert created'); onClose() },
      onError: err => toast.error(err instanceof Error ? err.message : 'Failed'),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>New Alert</CardTitle>
        <CardDescription>Fire when a metric crosses a threshold within a window.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder="Traffic spike" />
          </div>
          <div className="space-y-2">
            <Label>Metric</Label>
            <Select value={metric} onValueChange={setMetric}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {METRICS.map(m => <SelectItem key={m.value} value={m.value}>{m.label}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-3">
          <div className="space-y-2">
            <Label>Comparator</Label>
            <Select value={comparator} onValueChange={v => setComparator(v as 'gt' | 'lt')}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="gt">Greater than</SelectItem>
                <SelectItem value="lt">Less than</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Threshold</Label>
            <Input type="number" value={threshold} onChange={e => setThreshold(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>Window (minutes)</Label>
            <Input type="number" value={windowMinutes} onChange={e => setWindowMinutes(e.target.value)} />
          </div>
        </div>

        <div className="space-y-2">
          <Label>Cooldown (minutes between fires)</Label>
          <Input type="number" value={cooldownMinutes} onChange={e => setCooldownMinutes(e.target.value)} />
        </div>

        <div className="pt-2 border-t space-y-3">
          <Label className="text-sm font-semibold">Channels</Label>

          <div className="space-y-2">
            <Label className="text-xs font-normal text-muted-foreground">Email recipients (comma separated)</Label>
            <Input value={emailTo} onChange={e => setEmailTo(e.target.value)} placeholder="ops@example.com" />
          </div>

          <div className="space-y-2">
            <Label className="text-xs font-normal text-muted-foreground">Slack webhook URL</Label>
            <Input value={slackUrl} onChange={e => setSlackUrl(e.target.value)} placeholder="https://hooks.slack.com/services/..." />
          </div>

          <div className="flex items-center gap-2">
            <Switch checked={enableWebhooks} onCheckedChange={setEnableWebhooks} id="enable-wh" />
            <Label htmlFor="enable-wh" className="text-sm font-normal">
              Also deliver to configured webhooks
            </Label>
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={createAlert.isPending}>
            {createAlert.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Create Alert
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function AlertRow({ alert }: { alert: Alert }) {
  const update = useUpdateAlert()
  const del = useDeleteAlert()

  function toggle() {
    update.mutate({ ...alert, id: alert.id, enabled: !alert.enabled })
  }
  function handleDelete() {
    if (!confirm(`Delete alert "${alert.name}"?`)) return
    del.mutate(alert.id, { onSuccess: () => toast.success('Alert deleted') })
  }

  const metricLabel = METRICS.find(m => m.value === alert.metric)?.label ?? alert.metric
  const comp = alert.comparator === 'lt' ? '<' : '>'

  return (
    <div className="flex items-center justify-between p-4 border rounded-lg">
      <div className="flex items-center gap-3">
        <Bell className={`h-5 w-5 ${alert.enabled ? 'text-primary' : 'text-muted-foreground/50'}`} />
        <div>
          <div className="font-medium">{alert.name}</div>
          <div className="text-xs text-muted-foreground">
            {metricLabel} {comp} {alert.threshold} over {alert.window_minutes} min · {alert.channels.join(', ')}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Switch checked={alert.enabled} onCheckedChange={toggle} />
        <Button variant="ghost" size="icon" onClick={handleDelete}>
          <Trash2 className="h-4 w-4 text-muted-foreground" />
        </Button>
      </div>
    </div>
  )
}

function AlertFiresList() {
  const { data: fires, isLoading } = useAlertFires()
  if (isLoading) return <Skeleton className="h-24 w-full" />
  if (!fires?.length) {
    return <p className="text-sm text-muted-foreground text-center py-6">No alerts have fired yet.</p>
  }
  return (
    <div className="space-y-2">
      {fires.slice(0, 20).map(f => (
        <div key={f.id} className="flex items-center gap-3 text-sm p-2 rounded bg-muted/30">
          {f.error ? (
            <AlertTriangle className="h-4 w-4 text-destructive shrink-0" />
          ) : (
            <CheckCircle2 className="h-4 w-4 text-primary shrink-0" />
          )}
          <div className="flex-1 min-w-0">
            <div className="truncate">{new Date(f.fired_at).toLocaleString()}</div>
            <div className="text-xs text-muted-foreground truncate">
              value {f.metric_value.toFixed(1)} · threshold {f.threshold} · {f.channels_sent.join(', ') || 'no channels'}
              {f.error ? ` · ${f.error}` : ''}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function WebhookSection() {
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const { data: webhooks, isLoading } = useWebhooks()
  const createHook = useCreateWebhook()
  const delHook = useDeleteWebhook()
  const testHook = useTestWebhook()
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [newUrl, setNewUrl] = useState('')
  const [newSecret, setNewSecret] = useState<string | null>(null)

  function handleCreate() {
    if (!newName.trim() || !newUrl.trim() || !selectedDomainId) return
    createHook.mutate({
      domain_id: selectedDomainId,
      name: newName,
      url: newUrl,
      events: ['alert.fired'],
      enabled: true,
    }, {
      onSuccess: wh => {
        setNewSecret(wh.secret)
        setNewName('')
        setNewUrl('')
        toast.success('Webhook created')
      },
      onError: err => toast.error(err instanceof Error ? err.message : 'Failed'),
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Webhook className="h-5 w-5" /> Webhooks
            </CardTitle>
            <CardDescription>Receive signed JSON payloads when alerts fire.</CardDescription>
          </div>
          {!creating && (
            <Button variant="outline" size="sm" onClick={() => { setCreating(true); setNewSecret(null) }}>
              <Plus className="h-4 w-4 mr-1" /> Add webhook
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {creating && (
          <div className="space-y-3 p-3 border rounded-lg">
            <div className="grid gap-3 sm:grid-cols-2">
              <Input placeholder="Name" value={newName} onChange={e => setNewName(e.target.value)} />
              <Input placeholder="https://example.com/hook" value={newUrl} onChange={e => setNewUrl(e.target.value)} />
            </div>
            {newSecret && (
              <div className="p-3 rounded bg-primary/10 text-sm space-y-1">
                <div className="font-medium">Signing secret — copy now, you won't see it again:</div>
                <code className="text-xs break-all">{newSecret}</code>
              </div>
            )}
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={() => { setCreating(false); setNewSecret(null) }}>Done</Button>
              <Button size="sm" onClick={handleCreate} disabled={createHook.isPending}>
                {createHook.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
                Create
              </Button>
            </div>
          </div>
        )}

        {isLoading ? <Skeleton className="h-16 w-full" /> : !webhooks?.length ? (
          <p className="text-sm text-muted-foreground text-center py-4">No webhooks configured.</p>
        ) : (
          webhooks.map(wh => (
            <div key={wh.id} className="flex items-center justify-between p-3 border rounded-lg">
              <div className="min-w-0">
                <div className="font-medium truncate">{wh.name}</div>
                <div className="text-xs text-muted-foreground truncate">{wh.url}</div>
                {wh.last_fired_at && (
                  <div className="text-xs text-muted-foreground">
                    Last: {new Date(wh.last_fired_at).toLocaleString()}
                    {wh.last_status ? ` · HTTP ${wh.last_status}` : ''}
                    {wh.last_error ? ` · ${wh.last_error}` : ''}
                  </div>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Button variant="outline" size="sm" onClick={() => {
                  testHook.mutate(wh.id, {
                    onSuccess: r => r.success ? toast.success(r.message) : toast.error(r.message),
                  })
                }}>Test</Button>
                <Button variant="ghost" size="icon" onClick={() => {
                  if (!confirm('Delete this webhook?')) return
                  delHook.mutate(wh.id)
                }}>
                  <Trash2 className="h-4 w-4 text-muted-foreground" />
                </Button>
              </div>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  )
}

export function Alerts() {
  const [creating, setCreating] = useState(false)
  const { data: alerts, isLoading } = useAlerts()
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Alerts</h1>
          <p className="text-sm text-muted-foreground">
            Notify your team when traffic, errors, or bot rates cross a threshold.
          </p>
        </div>
        {selectedDomainId && !creating && (
          <Button onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4 mr-1" /> New Alert
          </Button>
        )}
      </div>

      {!selectedDomainId ? (
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">Select a property to configure alerts.</p>
          </CardContent>
        </Card>
      ) : (
        <>
          {creating && <CreateAlertForm onClose={() => setCreating(false)} />}

          <Card>
            <CardHeader>
              <CardTitle>Active rules</CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? <Skeleton className="h-24 w-full" /> : !alerts?.length ? (
                <p className="text-sm text-muted-foreground text-center py-4">
                  No alerts yet. Create one to start monitoring thresholds.
                </p>
              ) : (
                <div className="space-y-2">
                  {alerts.map(a => <AlertRow key={a.id} alert={a} />)}
                </div>
              )}
            </CardContent>
          </Card>

          <WebhookSection />

          <Card>
            <CardHeader>
              <CardTitle>Recent fires</CardTitle>
              <CardDescription>Last 20 alert triggers for this property.</CardDescription>
            </CardHeader>
            <CardContent>
              <AlertFiresList />
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
