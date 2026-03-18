import { useReplaySettings, useUpdateReplaySettings, useReplayStats } from '@/hooks/useReplayQueries'
import { FeatureGate } from '@/components/FeatureGate'
import { toast } from 'sonner'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, HardDrive, Save } from 'lucide-react'
import type { ReplaySettings as ReplaySettingsType } from '@/lib/types'

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function ReplaySettingsForm({ initial }: { initial: ReplaySettingsType }) {
  const { data: stats } = useReplayStats()
  const updateMutation = useUpdateReplaySettings()
  const [form, setForm] = useState(initial)

  const handleSave = () => {
    updateMutation.mutate(form, {
      onSuccess: () => toast.success('Replay settings updated'),
      onError: () => toast.error('Failed to update settings'),
    })
  }

  return (
    <div className="p-6 space-y-6 max-w-2xl mx-auto overflow-auto h-full">
      <div className="flex items-center gap-3">
        <Link to="/replays" className="p-1.5 rounded-md hover:bg-muted transition-colors">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1 className="text-2xl font-bold">Session Replay Settings</h1>
          <p className="text-muted-foreground">Configure recording behavior and privacy</p>
        </div>
      </div>

      {stats && (
        <div className="rounded-lg border p-4 flex items-center gap-4">
          <HardDrive className="h-5 w-5 text-muted-foreground" />
          <div className="flex-1">
            <div className="text-sm font-medium">
              Storage: {formatBytes(stats.disk_usage_bytes)} / {formatBytes(stats.quota_bytes)}
            </div>
            <div className="mt-1 h-2 bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-primary rounded-full transition-all"
                style={{
                  width: `${Math.min(100, (stats.disk_usage_bytes / stats.quota_bytes) * 100)}%`,
                }}
              />
            </div>
          </div>
          <span className="text-sm text-muted-foreground">
            {stats.total_recordings} recordings
          </span>
        </div>
      )}

      <div className="space-y-6">
        {/* Enable/Disable */}
        <div className="flex items-center justify-between rounded-lg border p-4">
          <div>
            <div className="font-medium">Enable Session Replay</div>
            <div className="text-sm text-muted-foreground">Record user sessions for playback</div>
          </div>
          <button
            onClick={() => setForm(f => ({ ...f, enabled: !f.enabled }))}
            className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
              form.enabled ? 'bg-primary' : 'bg-muted'
            }`}
          >
            <span
              className={`inline-block h-4 w-4 rounded-full bg-white transition-transform ${
                form.enabled ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
        </div>

        {/* Sample Rate */}
        <div className="rounded-lg border p-4 space-y-2">
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">Sample Rate</div>
              <div className="text-sm text-muted-foreground">Percentage of sessions to record</div>
            </div>
            <span className="text-lg font-bold">{form.sample_rate}%</span>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            value={form.sample_rate}
            onChange={e => setForm(f => ({ ...f, sample_rate: Number(e.target.value) }))}
            className="w-full"
          />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>0% (off)</span>
            <span>50%</span>
            <span>100% (all)</span>
          </div>
        </div>

        {/* Privacy */}
        <div className="rounded-lg border p-4 space-y-4">
          <div className="font-medium">Privacy</div>
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm font-medium">Mask all text</div>
              <div className="text-xs text-muted-foreground">Replace visible text with asterisks</div>
            </div>
            <button
              onClick={() => setForm(f => ({ ...f, mask_text: !f.mask_text }))}
              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                form.mask_text ? 'bg-primary' : 'bg-muted'
              }`}
            >
              <span
                className={`inline-block h-4 w-4 rounded-full bg-white transition-transform ${
                  form.mask_text ? 'translate-x-6' : 'translate-x-1'
                }`}
              />
            </button>
          </div>
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm font-medium">Mask all inputs</div>
              <div className="text-xs text-muted-foreground">Hide form field values in recordings</div>
            </div>
            <button
              onClick={() => setForm(f => ({ ...f, mask_inputs: !f.mask_inputs }))}
              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                form.mask_inputs ? 'bg-primary' : 'bg-muted'
              }`}
            >
              <span
                className={`inline-block h-4 w-4 rounded-full bg-white transition-transform ${
                  form.mask_inputs ? 'translate-x-6' : 'translate-x-1'
                }`}
              />
            </button>
          </div>
          <p className="text-xs text-muted-foreground border-t pt-3">
            Add <code className="bg-muted px-1 rounded">class="etq-no-record"</code> to any element to exclude it from recordings entirely.
          </p>
        </div>

        {/* Limits */}
        <div className="rounded-lg border p-4 space-y-4">
          <div className="font-medium">Limits</div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm font-medium">Max recording duration</label>
              <select
                value={form.max_duration_sec}
                onChange={e => setForm(f => ({ ...f, max_duration_sec: Number(e.target.value) }))}
                className="mt-1 w-full rounded-md border bg-background px-3 py-2 text-sm"
              >
                <option value={300}>5 minutes</option>
                <option value={600}>10 minutes</option>
                <option value={1800}>30 minutes</option>
                <option value={3600}>1 hour</option>
                <option value={7200}>2 hours</option>
              </select>
            </div>
            <div>
              <label className="text-sm font-medium">Storage quota</label>
              <select
                value={form.storage_quota_mb}
                onChange={e => setForm(f => ({ ...f, storage_quota_mb: Number(e.target.value) }))}
                className="mt-1 w-full rounded-md border bg-background px-3 py-2 text-sm"
              >
                <option value={1024}>1 GB</option>
                <option value={2048}>2 GB</option>
                <option value={5120}>5 GB</option>
                <option value={10240}>10 GB</option>
                <option value={20480}>20 GB</option>
                <option value={51200}>50 GB</option>
              </select>
            </div>
          </div>
        </div>

        <button
          onClick={handleSave}
          disabled={updateMutation.isPending}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
        >
          <Save className="h-4 w-4" />
          {updateMutation.isPending ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  )
}

function ReplaySettingsContent() {
  const { data: settings, isLoading } = useReplaySettings()

  if (isLoading || !settings) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
      </div>
    )
  }

  return <ReplaySettingsForm initial={settings} />
}

export function ReplaySettings() {
  return (
    <FeatureGate feature="session_replay">
      <ReplaySettingsContent />
    </FeatureGate>
  )
}
