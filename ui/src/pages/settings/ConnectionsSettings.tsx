import { useState, useEffect, useCallback, useMemo } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { fetchAPI } from '@/lib/api'
import { Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Cable, ChevronRight, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { SettingsLayout } from './SettingsLayout'

// --- Types ---

interface GoogleAdsSettingsData {
  google_ads_client_id: string
  google_ads_client_secret: string
  google_ads_developer_token: string
}

interface MetaAdsSettingsData {
  meta_ads_app_id: string
  meta_ads_app_secret: string
}

interface MicrosoftAdsSettingsData {
  microsoft_ads_client_id: string
  microsoft_ads_client_secret: string
  microsoft_ads_developer_token: string
}

// --- Shared components ---

function StatusBanner({ result, testResult }: {
  result: string | null
  testResult: { success: boolean; message: string } | null
}) {
  return (
    <>
      {result && (
        <div
          className={`p-3 rounded text-sm ${
            result.startsWith('Error')
              ? 'bg-destructive/10 text-destructive'
              : 'bg-green-500/10 text-green-600'
          }`}
        >
          {result}
        </div>
      )}
      {testResult && (
        <div
          className={`p-3 rounded text-sm ${
            testResult.success
              ? 'bg-green-500/10 text-green-600'
              : 'bg-destructive/10 text-destructive'
          }`}
        >
          {testResult.message}
        </div>
      )}
    </>
  )
}

function ActionButtons({ onTest, onSave, testing, saving, hasChanges }: {
  onTest: () => void
  onSave: () => void
  testing: boolean
  saving: boolean
  hasChanges: boolean
}) {
  return (
    <div className="flex items-center justify-between pt-4 border-t">
      <div className="flex flex-col gap-1">
        <Button variant="outline" onClick={onTest} disabled={testing}>
          {testing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Test Credentials
        </Button>
        <span className="text-xs text-muted-foreground">
          Validates that credentials are configured.
        </span>
      </div>
      <Button onClick={onSave} disabled={saving || !hasChanges}>
        {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        Save Settings
      </Button>
    </div>
  )
}

// --- Provider hook ---

function useProviderSettings<T extends { [K in keyof T]: string }>(endpoint: string, label: string) {
  const [settings, setSettings] = useState<T | null>(null)
  const [edited, setEdited] = useState<Partial<T>>({})
  const [saving, setSaving] = useState(false)
  const [result, setResult] = useState<string | null>(null)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)

  const hasChanges = useMemo(() => Object.keys(edited).length > 0, [edited])

  const fetchSettings = useCallback(async () => {
    try {
      const data = await fetchAPI<T>(endpoint)
      setSettings(data)
    } catch {
      toast.error(`Failed to load ${label} settings`)
    }
  }, [endpoint, label])

  useEffect(() => {
    fetchSettings()
  }, [fetchSettings])

  async function handleSave() {
    if (!hasChanges) return
    setSaving(true)
    setResult(null)
    try {
      await fetchAPI(endpoint, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(edited),
      })
      setEdited({})
      setResult('Settings saved successfully')
      fetchSettings()
    } catch (err) {
      setResult(`Error: ${err instanceof Error ? err.message : 'Failed to save settings'}`)
    } finally {
      setSaving(false)
    }
  }

  async function handleTest() {
    setTesting(true)
    setTestResult(null)
    try {
      if (hasChanges) {
        await fetchAPI(endpoint, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(edited),
        })
        setEdited({})
        fetchSettings()
      }
      const res = await fetchAPI<{ success: boolean; message: string }>(`${endpoint}/test`, {
        method: 'POST',
      })
      setTestResult(res)
    } catch (err) {
      setTestResult({
        success: false,
        message: err instanceof Error ? err.message : 'Test failed',
      })
    } finally {
      setTesting(false)
    }
  }

  function updateField(key: keyof T, value: string) {
    setEdited((prev) => ({ ...prev, [key]: value }))
  }

  return { settings, edited, hasChanges, saving, result, testing, testResult, handleSave, handleTest, updateField }
}

// --- Provider Sections ---

function GoogleAdsSection() {
  const { settings, edited, hasChanges, saving, result, testing, testResult, handleSave, handleTest, updateField } =
    useProviderSettings<GoogleAdsSettingsData>('/api/settings/google-ads', 'Google Ads')

  return (
    <div className="space-y-6">
      <StatusBanner result={result} testResult={testResult} />

      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2 md:col-span-2">
          <Label htmlFor="google_ads_client_id">OAuth Client ID</Label>
          <Input
            id="google_ads_client_id"
            type="text"
            placeholder="123456789-abc.apps.googleusercontent.com"
            value={edited.google_ads_client_id ?? settings?.google_ads_client_id ?? ''}
            onChange={(e) => updateField('google_ads_client_id', e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            From Google Cloud Console &rarr; APIs &amp; Services &rarr; Credentials
          </p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="google_ads_client_secret">OAuth Client Secret</Label>
          <Input
            id="google_ads_client_secret"
            type="password"
            placeholder={settings?.google_ads_client_secret ? '••••••••' : 'Enter client secret'}
            value={edited.google_ads_client_secret ?? ''}
            onChange={(e) => updateField('google_ads_client_secret', e.target.value)}
          />
          {settings?.google_ads_client_secret && (
            <p className="text-xs text-muted-foreground">Secret is set. Enter a new value to replace it.</p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="google_ads_developer_token">Developer Token</Label>
          <Input
            id="google_ads_developer_token"
            type="password"
            placeholder={settings?.google_ads_developer_token ? '••••••••' : 'Enter developer token'}
            value={edited.google_ads_developer_token ?? ''}
            onChange={(e) => updateField('google_ads_developer_token', e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            From Google Ads &rarr; Tools &amp; Settings &rarr; API Center
          </p>
        </div>
      </div>

      <div className="rounded-md bg-muted p-4 text-sm text-muted-foreground space-y-2">
        <p className="font-medium text-foreground">Setup instructions:</p>
        <ol className="list-decimal list-inside space-y-1">
          <li>Create a Google Cloud project and enable the Google Ads API</li>
          <li>Configure an OAuth consent screen (External or Internal)</li>
          <li>Create OAuth 2.0 credentials (<strong>Desktop application</strong> type — no redirect URI needed)</li>
          <li>Get your developer token from ads.google.com/aw/apicenter</li>
          <li>
            Obtain a refresh token using one of these methods:
            <ul className="list-disc list-inside ml-4 mt-1 space-y-0.5">
              <li>
                Google OAuth Playground — set scope to{' '}
                <code className="text-xs bg-background rounded px-1 py-0.5">https://www.googleapis.com/auth/adwords</code>
              </li>
              <li>
                <code className="text-xs bg-background rounded px-1 py-0.5">gcloud auth application-default login --scopes=https://www.googleapis.com/auth/adwords</code>
              </li>
            </ul>
          </li>
          <li>Paste the refresh token when creating a connection in the Connections page</li>
        </ol>
      </div>

      <ActionButtons onTest={handleTest} onSave={handleSave} testing={testing} saving={saving} hasChanges={hasChanges} />
    </div>
  )
}

function MetaAdsSection() {
  const { settings, edited, hasChanges, saving, result, testing, testResult, handleSave, handleTest, updateField } =
    useProviderSettings<MetaAdsSettingsData>('/api/settings/meta-ads', 'Meta Ads')

  return (
    <div className="space-y-6">
      <StatusBanner result={result} testResult={testResult} />

      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2 md:col-span-2">
          <Label htmlFor="meta_ads_app_id">App ID</Label>
          <Input
            id="meta_ads_app_id"
            type="text"
            placeholder="123456789012345"
            value={edited.meta_ads_app_id ?? settings?.meta_ads_app_id ?? ''}
            onChange={(e) => updateField('meta_ads_app_id', e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            From Meta for Developers &rarr; Your App &rarr; Settings &rarr; Basic
          </p>
        </div>

        <div className="space-y-2 md:col-span-2">
          <Label htmlFor="meta_ads_app_secret">App Secret</Label>
          <Input
            id="meta_ads_app_secret"
            type="password"
            placeholder={settings?.meta_ads_app_secret ? '••••••••' : 'Enter app secret'}
            value={edited.meta_ads_app_secret ?? ''}
            onChange={(e) => updateField('meta_ads_app_secret', e.target.value)}
          />
          {settings?.meta_ads_app_secret && (
            <p className="text-xs text-muted-foreground">Secret is set. Enter a new value to replace it.</p>
          )}
        </div>
      </div>

      <div className="rounded-md bg-muted p-4 text-sm text-muted-foreground space-y-2">
        <p className="font-medium text-foreground">Setup instructions:</p>
        <ol className="list-decimal list-inside space-y-1">
          <li>Create a Meta App at <strong>developers.facebook.com</strong></li>
          <li>Add the <strong>Marketing API</strong> product to your app</li>
          <li>Create a System User in Business Manager (recommended — token never expires)</li>
          <li>
            Generate a token with <code className="text-xs bg-background rounded px-1 py-0.5">ads_management</code> and{' '}
            <code className="text-xs bg-background rounded px-1 py-0.5">ads_read</code> permissions
          </li>
          <li>Or exchange a user token for a long-lived token via the Access Token Debugger</li>
          <li>Paste the token when creating a connection in the Connections page</li>
        </ol>
      </div>

      <ActionButtons onTest={handleTest} onSave={handleSave} testing={testing} saving={saving} hasChanges={hasChanges} />
    </div>
  )
}

function MicrosoftAdsSection() {
  const { settings, edited, hasChanges, saving, result, testing, testResult, handleSave, handleTest, updateField } =
    useProviderSettings<MicrosoftAdsSettingsData>('/api/settings/microsoft-ads', 'Microsoft Ads')

  return (
    <div className="space-y-6">
      <StatusBanner result={result} testResult={testResult} />

      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2 md:col-span-2">
          <Label htmlFor="microsoft_ads_client_id">OAuth Client ID</Label>
          <Input
            id="microsoft_ads_client_id"
            type="text"
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            value={edited.microsoft_ads_client_id ?? settings?.microsoft_ads_client_id ?? ''}
            onChange={(e) => updateField('microsoft_ads_client_id', e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            From Azure Portal &rarr; App registrations &rarr; Application (client) ID
          </p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="microsoft_ads_client_secret">OAuth Client Secret</Label>
          <Input
            id="microsoft_ads_client_secret"
            type="password"
            placeholder={settings?.microsoft_ads_client_secret ? '••••••••' : 'Enter client secret'}
            value={edited.microsoft_ads_client_secret ?? ''}
            onChange={(e) => updateField('microsoft_ads_client_secret', e.target.value)}
          />
          {settings?.microsoft_ads_client_secret && (
            <p className="text-xs text-muted-foreground">Secret is set. Enter a new value to replace it.</p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="microsoft_ads_developer_token">Developer Token</Label>
          <Input
            id="microsoft_ads_developer_token"
            type="password"
            placeholder={settings?.microsoft_ads_developer_token ? '••••••••' : 'Enter developer token'}
            value={edited.microsoft_ads_developer_token ?? ''}
            onChange={(e) => updateField('microsoft_ads_developer_token', e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            From Microsoft Advertising &rarr; Tools &rarr; Developer Portal
          </p>
        </div>
      </div>

      <div className="rounded-md bg-muted p-4 text-sm text-muted-foreground space-y-2">
        <p className="font-medium text-foreground">Setup instructions:</p>
        <ol className="list-decimal list-inside space-y-1">
          <li>Register an app in <strong>Azure AD</strong> (portal.azure.com)</li>
          <li>
            Set redirect URI to{' '}
            <code className="text-xs bg-background rounded px-1 py-0.5">
              https://login.microsoftonline.com/common/oauth2/nativeclient
            </code>{' '}
            (desktop flow)
          </li>
          <li>Grant Microsoft Ads API permissions to the app</li>
          <li>Get your Developer Token from <strong>apps.bingads.microsoft.com</strong></li>
          <li>
            Use OAuth sandbox or MSAL CLI to get a refresh token with scope{' '}
            <code className="text-xs bg-background rounded px-1 py-0.5">
              https://ads.microsoft.com/.default offline_access
            </code>
          </li>
          <li>Paste the refresh token when creating a connection in the Connections page</li>
        </ol>
      </div>

      <ActionButtons onTest={handleTest} onSave={handleSave} testing={testing} saving={saving} hasChanges={hasChanges} />
    </div>
  )
}

// --- Collapsible Section wrapper ---

function ProviderSection({ title, description, defaultOpen, children }: {
  title: string
  description: string
  defaultOpen?: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen ?? false)

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card>
        <CollapsibleTrigger asChild>
          <CardHeader className="cursor-pointer select-none hover:bg-muted/50 transition-colors">
            <CardTitle className="flex items-center gap-2">
              <Cable className="h-5 w-5" />
              <span className="flex-1">{title}</span>
              <ChevronRight className={`h-4 w-4 transition-transform duration-200 ${open ? 'rotate-90' : ''}`} />
            </CardTitle>
            <CardDescription>{description}</CardDescription>
          </CardHeader>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent>{children}</CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  )
}

// --- Main page ---

export function ConnectionsSettings() {
  const { isAdmin } = useAuth()

  if (!isAdmin) {
    return <Navigate to="/settings/domains" replace />
  }

  return (
    <SettingsLayout title="Connections" description="Configure ad platform API credentials for spend tracking">
      <div className="space-y-4">
        <ProviderSection
          title="Google Ads"
          description="Import campaign spend data from Google Ads"
          defaultOpen
        >
          <GoogleAdsSection />
        </ProviderSection>

        <ProviderSection
          title="Meta Ads"
          description="Import campaign spend data from Facebook and Instagram Ads"
        >
          <MetaAdsSection />
        </ProviderSection>

        <ProviderSection
          title="Microsoft Ads"
          description="Import campaign spend data from Microsoft Advertising (Bing Ads)"
        >
          <MicrosoftAdsSection />
        </ProviderSection>
      </div>
    </SettingsLayout>
  )
}
