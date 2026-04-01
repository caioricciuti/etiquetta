import { useState, useMemo, useCallback } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { fetchAPI } from '@/lib/api'
import { Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Activity, Database, Loader2, Brain, Info } from 'lucide-react'
import { toast } from 'sonner'
import { useLicense } from '@/hooks/useLicenseQuery'
import { useDomainStore } from '@/stores/useDomainStore'
import { useDomainSettings, useUpdateDomainSettings } from '@/hooks/useDomainSettings'
import { SettingsLayout } from './SettingsLayout'
import type { AICrawlerSettings } from '@/lib/types'

function ScopeIndicator({ scope }: { scope: string | undefined }) {
  if (scope !== 'global') return null
  return (
    <span className="inline-flex items-center gap-1 text-xs text-muted-foreground ml-2">
      <Info className="h-3 w-3" />
      Using default
    </span>
  )
}

export function TrackingSettings() {
  const { isAdmin } = useAuth()
  const { license } = useLicense()
  const selectedDomainId = useDomainStore(s => s.selectedDomainId)
  const { data: settings, isLoading } = useDomainSettings(selectedDomainId)
  const updateSettings = useUpdateDomainSettings(selectedDomainId)
  const [edited, setEdited] = useState<Record<string, string>>({})

  const hasChanges = useMemo(() => Object.keys(edited).length > 0, [edited])

  if (!isAdmin) {
    return <Navigate to="/settings/properties" replace />
  }

  function getBool(key: string, fallback: boolean): boolean {
    const val = edited[key] ?? settings?.[key]
    if (val === undefined) return fallback
    return val === 'true' || val === '1'
  }

  function getNumber(key: string, fallback: number): string {
    return edited[key] ?? settings?.[key] ?? String(fallback)
  }

  function setBool(key: string, value: boolean) {
    setEdited(prev => ({ ...prev, [key]: value ? 'true' : 'false' }))
  }

  function setNumber(key: string, value: string) {
    setEdited(prev => ({ ...prev, [key]: value }))
  }

  function getString(key: string, fallback: string): string {
    return edited[key] ?? settings?.[key] ?? fallback
  }

  function setString(key: string, value: string) {
    setEdited(prev => ({ ...prev, [key]: value }))
  }

  function getScope(key: string): string | undefined {
    return settings?.['scope:' + key]
  }

  async function handleSave() {
    if (!hasChanges) return
    updateSettings.mutate(edited, {
      onSuccess: () => setEdited({}),
    })
  }

  if (!selectedDomainId) {
    return (
      <SettingsLayout title="Tracking" description="Configure data collection and privacy settings">
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">
              Select a property from the sidebar to configure tracking settings.
            </p>
          </CardContent>
        </Card>
      </SettingsLayout>
    )
  }

  if (isLoading) {
    return (
      <SettingsLayout title="Tracking" description="Configure data collection and privacy settings">
        <Card>
          <CardContent className="py-8 flex items-center justify-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading settings...
          </CardContent>
        </Card>
      </SettingsLayout>
    )
  }

  return (
    <SettingsLayout title="Tracking" description="Configure data collection and privacy settings for this property">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Activity className="h-5 w-5" />
            Data Collection
          </CardTitle>
          <CardDescription>
            Control what data the tracker collects from visitors.
            Changes take effect within 24 hours (tracker script is cached by browsers).
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Track Performance */}
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label>
                Core Web Vitals
                <ScopeIndicator scope={getScope('track_performance')} />
              </Label>
              <p className="text-xs text-muted-foreground">
                Collect LCP, FCP, CLS, INP, TTFB and page load times
              </p>
            </div>
            <Switch
              checked={getBool('track_performance', true)}
              onCheckedChange={v => setBool('track_performance', v)}
            />
          </div>

          {/* Track Errors */}
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label>
                JavaScript Errors
                <ScopeIndicator scope={getScope('track_errors')} />
              </Label>
              <p className="text-xs text-muted-foreground">
                Capture JavaScript errors from tracked pages
              </p>
            </div>
            <Switch
              checked={getBool('track_errors', true)}
              onCheckedChange={v => setBool('track_errors', v)}
            />
          </div>

          {/* Respect DNT */}
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label>
                Honor Do Not Track
                <ScopeIndicator scope={getScope('respect_dnt')} />
              </Label>
              <p className="text-xs text-muted-foreground">
                Respect the DNT browser header. Many browsers enable this by default,
                which may cause significant data loss. Not legally required.
              </p>
            </div>
            <Switch
              checked={getBool('respect_dnt', true)}
              onCheckedChange={v => setBool('respect_dnt', v)}
            />
          </div>

          {/* Tracking Mode */}
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label>
                Visitor Identification
                <ScopeIndicator scope={getScope('tracking_mode')} />
              </Label>
              <p className="text-xs text-muted-foreground">
                How visitors are identified across page views.
                Cookieless uses no storage and is fully GDPR-compliant without consent.
                Cookie and Local Storage enable cross-session identification but may require consent.
              </p>
            </div>
            <Select
              value={getString('tracking_mode', 'cookieless')}
              onValueChange={(value) => setString('tracking_mode', value)}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="cookieless">Cookieless</SelectItem>
                <SelectItem value="cookie">First-party Cookie</SelectItem>
                <SelectItem value="localStorage">Local Storage</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Session Timeout */}
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="session_timeout">
                Session Timeout (minutes)
                <ScopeIndicator scope={getScope('session_timeout_minutes')} />
              </Label>
              <p className="text-xs text-muted-foreground">
                Minutes of inactivity before a visitor session expires (default: 30)
              </p>
            </div>
            <Input
              id="session_timeout"
              type="number"
              min={1}
              max={1440}
              className="w-24"
              value={getNumber('session_timeout_minutes', 30)}
              onChange={e => setNumber('session_timeout_minutes', e.target.value)}
            />
          </div>

          {/* Save */}
          <div className="flex justify-end pt-4 border-t">
            <Button onClick={handleSave} disabled={updateSettings.isPending || !hasChanges}>
              {updateSettings.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save Settings
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="h-5 w-5" />
            Data Retention
          </CardTitle>
          <CardDescription>
            Configure how long analytics data is kept before automatic deletion.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {(() => {
            const isPro = license.tier === 'pro' || license.tier === 'enterprise'
            const currentValue = getNumber('data_retention_days', 180)

            return (
              <>
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label>
                      Delete data older than
                      <ScopeIndicator scope={getScope('data_retention_days')} />
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      {isPro
                        ? 'Choose how long to keep analytics data'
                        : 'Community plan: up to 180 days'}
                    </p>
                  </div>
                  <Select
                    value={currentValue}
                    onValueChange={(value) => setNumber('data_retention_days', value)}
                  >
                    <SelectTrigger className="w-[160px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="90">90 days</SelectItem>
                      <SelectItem value="180">180 days</SelectItem>
                      {isPro && <SelectItem value="365">1 year</SelectItem>}
                      {isPro && <SelectItem value="-1">Forever</SelectItem>}
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex justify-end pt-4 border-t">
                  <Button onClick={handleSave} disabled={updateSettings.isPending || !hasChanges}>
                    {updateSettings.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Save Settings
                  </Button>
                </div>
              </>
            )
          })()}
        </CardContent>
      </Card>

      <AICrawlerCard />
    </SettingsLayout>
  )
}

function AICrawlerCard() {
  const [crawlerSettings, setCrawlerSettings] = useState<AICrawlerSettings | null>(null)
  const [editedRules, setEditedRules] = useState<Record<string, 'allow' | 'block'> | null>(null)
  const [saving, setSaving] = useState(false)
  const [loaded, setLoaded] = useState(false)

  if (!loaded) {
    setLoaded(true)
    fetchAPI<AICrawlerSettings>('/api/settings/ai-crawlers')
      .then((data) => setCrawlerSettings(data))
      .catch(() => toast.error('Failed to load AI crawler settings'))
  }

  const currentRules = editedRules ?? crawlerSettings?.rules ?? {}
  const hasChanges = editedRules !== null

  const toggleCrawler = useCallback((name: string) => {
    setEditedRules((prev) => {
      const base = prev ?? crawlerSettings?.rules ?? {}
      const current = base[name] ?? 'block'
      return { ...base, [name]: current === 'allow' ? 'block' : 'allow' }
    })
  }, [crawlerSettings])

  async function handleSave() {
    if (!hasChanges) return
    setSaving(true)
    try {
      await fetchAPI('/api/settings/ai-crawlers', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ rules: editedRules }),
      })
      setCrawlerSettings((prev) => prev ? { ...prev, rules: editedRules! } : prev)
      setEditedRules(null)
      toast.success('AI crawler settings saved')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Brain className="h-5 w-5" />
          AI Crawler Controls
        </CardTitle>
        <CardDescription>
          Control which AI crawlers can access your site via <code className="text-xs bg-muted px-1 rounded">/robots.txt</code>.
          Blocked crawlers are instructed not to crawl your content for AI training.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!crawlerSettings ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading crawlers...
          </div>
        ) : (
          <>
            <div className="space-y-3">
              {crawlerSettings.known_crawlers.map((name) => (
                <div key={name} className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label>{name}</Label>
                  </div>
                  <Switch
                    checked={(currentRules[name] ?? 'block') === 'allow'}
                    onCheckedChange={() => toggleCrawler(name)}
                  />
                </div>
              ))}
            </div>

            <p className="text-xs text-muted-foreground pt-2">
              Toggling a crawler to &quot;on&quot; allows it to crawl your site. Off (default) adds a Disallow rule to robots.txt.
              Note: robots.txt is advisory — well-behaved crawlers respect it, but it is not enforced.
            </p>

            <div className="flex justify-end pt-4 border-t">
              <Button onClick={handleSave} disabled={saving || !hasChanges}>
                {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Save AI Crawler Settings
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
