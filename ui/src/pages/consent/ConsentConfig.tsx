import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useConsentConfig, useSaveConsentConfig, useConsentDomainId } from '@/hooks/useConsent'
import { FeatureGate } from '@/components/FeatureGate'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Plus, Trash2, Save, ArrowLeft } from 'lucide-react'
import type { ConsentCategory, ConsentAppearance } from '@/lib/types'

const DEFAULT_CATEGORIES: ConsentCategory[] = [
  { id: 'necessary', label: 'Necessary', description: 'Essential cookies for site functionality', required: true, default_enabled: true },
  { id: 'analytics', label: 'Analytics', description: 'Help us understand how visitors use the site', required: false, default_enabled: false },
  { id: 'marketing', label: 'Marketing', description: 'Used to deliver relevant advertisements', required: false, default_enabled: false },
  { id: 'preferences', label: 'Preferences', description: 'Remember user preferences and settings', required: false, default_enabled: false },
]

const DEFAULT_APPEARANCE: ConsentAppearance = {
  style: 'bar',
  position: 'bottom',
  bg_color: '#ffffff',
  text_color: '#1a1a1a',
  btn_bg_color: '#18181b',
  btn_text_color: '#ffffff',
  show_reject_all: true,
}

function ConsentConfigContent() {
  const domainId = useConsentDomainId()
  const { data: config, isLoading } = useConsentConfig(domainId)
  const saveConfig = useSaveConsentConfig(domainId)

  const [categories, setCategories] = useState<ConsentCategory[]>(DEFAULT_CATEGORIES)
  const [appearance, setAppearance] = useState<ConsentAppearance>(DEFAULT_APPEARANCE)
  const [cookieName, setCookieName] = useState('etiquetta_consent')
  const [cookieExpiryDays, setCookieExpiryDays] = useState(365)
  const [autoLanguage, setAutoLanguage] = useState(true)
  const [newCategoryId, setNewCategoryId] = useState('')
  const [newCategoryLabel, setNewCategoryLabel] = useState('')
  const [newCategoryDescription, setNewCategoryDescription] = useState('')

  useEffect(() => {
    if (config) {
      setCategories(config.categories?.length ? config.categories : DEFAULT_CATEGORIES)
      setAppearance(config.appearance ?? DEFAULT_APPEARANCE)
      setCookieName(config.cookie_name || 'etiquetta_consent')
      setCookieExpiryDays(config.cookie_expiry_days || 365)
      setAutoLanguage(config.auto_language ?? true)
    }
  }, [config])

  function handleAddCategory() {
    if (!newCategoryId || !newCategoryLabel) return
    if (categories.some(c => c.id === newCategoryId)) return
    setCategories([
      ...categories,
      {
        id: newCategoryId,
        label: newCategoryLabel,
        description: newCategoryDescription,
        required: false,
        default_enabled: false,
      },
    ])
    setNewCategoryId('')
    setNewCategoryLabel('')
    setNewCategoryDescription('')
  }

  function handleRemoveCategory(id: string) {
    setCategories(categories.filter(c => c.id !== id))
  }

  function handleToggleCategoryDefault(id: string) {
    setCategories(categories.map(c =>
      c.id === id ? { ...c, default_enabled: !c.default_enabled } : c
    ))
  }

  function handleSave() {
    saveConfig.mutate({
      domain_id: domainId,
      categories,
      appearance,
      cookie_name: cookieName,
      cookie_expiry_days: cookieExpiryDays,
      auto_language: autoLanguage,
    })
  }

  if (!domainId) {
    return (
      <Card>
        <CardContent className="py-8">
          <p className="text-center text-muted-foreground">Select a domain to configure consent settings.</p>
        </CardContent>
      </Card>
    )
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="py-8">
          <p className="text-center text-muted-foreground">Loading consent configuration...</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      {/* Categories */}
      <Card>
        <CardHeader>
          <CardTitle>Consent Categories</CardTitle>
          <CardDescription>Define the consent categories visitors can opt in or out of</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {categories.map((category) => (
            <div key={category.id} className="flex items-center justify-between p-4 rounded-lg border border-border">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <p className="font-medium">{category.label}</p>
                  <span className="text-xs font-mono text-muted-foreground">({category.id})</span>
                  {category.required && (
                    <span className="text-xs bg-zinc-100 dark:bg-zinc-800 px-2 py-0.5 rounded">Required</span>
                  )}
                </div>
                <p className="text-sm text-muted-foreground mt-1">{category.description}</p>
              </div>
              <div className="flex items-center gap-3 ml-4">
                {!category.required && (
                  <>
                    <div className="flex items-center gap-2">
                      <Label htmlFor={`default-${category.id}`} className="text-xs text-muted-foreground">
                        Default on
                      </Label>
                      <Switch
                        id={`default-${category.id}`}
                        checked={category.default_enabled}
                        onCheckedChange={() => handleToggleCategoryDefault(category.id)}
                      />
                    </div>
                    <Button variant="outline" size="sm" onClick={() => handleRemoveCategory(category.id)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </>
                )}
              </div>
            </div>
          ))}

          <div className="border-t border-border pt-4">
            <p className="text-sm font-medium mb-3">Add Custom Category</p>
            <div className="flex gap-3">
              <Input
                placeholder="ID (e.g., social)"
                value={newCategoryId}
                onChange={(e) => setNewCategoryId(e.target.value.toLowerCase().replace(/\s+/g, '_'))}
                className="w-36"
              />
              <Input
                placeholder="Label (e.g., Social Media)"
                value={newCategoryLabel}
                onChange={(e) => setNewCategoryLabel(e.target.value)}
                className="w-48"
              />
              <Input
                placeholder="Description"
                value={newCategoryDescription}
                onChange={(e) => setNewCategoryDescription(e.target.value)}
                className="flex-1"
              />
              <Button variant="outline" onClick={handleAddCategory} disabled={!newCategoryId || !newCategoryLabel}>
                <Plus className="h-4 w-4 mr-1" />
                Add
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Appearance */}
      <Card>
        <CardHeader>
          <CardTitle>Appearance</CardTitle>
          <CardDescription>Customize how the consent banner looks on your site</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Banner Style</Label>
              <Select
                value={appearance.style}
                onValueChange={(value: 'bar' | 'popup' | 'modal') =>
                  setAppearance({ ...appearance, style: value })
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bar">Bar</SelectItem>
                  <SelectItem value="popup">Popup</SelectItem>
                  <SelectItem value="modal">Modal</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Position</Label>
              <Select
                value={appearance.position}
                onValueChange={(value: 'top' | 'bottom' | 'bottom-left' | 'bottom-right' | 'center') =>
                  setAppearance({ ...appearance, position: value })
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="top">Top</SelectItem>
                  <SelectItem value="bottom">Bottom</SelectItem>
                  <SelectItem value="bottom-left">Bottom Left</SelectItem>
                  <SelectItem value="bottom-right">Bottom Right</SelectItem>
                  <SelectItem value="center">Center</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="bg-color">Background Color</Label>
              <div className="flex gap-2">
                <input
                  type="color"
                  id="bg-color"
                  value={appearance.bg_color}
                  onChange={(e) => setAppearance({ ...appearance, bg_color: e.target.value })}
                  className="h-9 w-12 rounded border border-border cursor-pointer"
                />
                <Input
                  value={appearance.bg_color}
                  onChange={(e) => setAppearance({ ...appearance, bg_color: e.target.value })}
                  className="flex-1"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="text-color">Text Color</Label>
              <div className="flex gap-2">
                <input
                  type="color"
                  id="text-color"
                  value={appearance.text_color}
                  onChange={(e) => setAppearance({ ...appearance, text_color: e.target.value })}
                  className="h-9 w-12 rounded border border-border cursor-pointer"
                />
                <Input
                  value={appearance.text_color}
                  onChange={(e) => setAppearance({ ...appearance, text_color: e.target.value })}
                  className="flex-1"
                />
              </div>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="btn-bg-color">Button Background</Label>
              <div className="flex gap-2">
                <input
                  type="color"
                  id="btn-bg-color"
                  value={appearance.btn_bg_color}
                  onChange={(e) => setAppearance({ ...appearance, btn_bg_color: e.target.value })}
                  className="h-9 w-12 rounded border border-border cursor-pointer"
                />
                <Input
                  value={appearance.btn_bg_color}
                  onChange={(e) => setAppearance({ ...appearance, btn_bg_color: e.target.value })}
                  className="flex-1"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="btn-text-color">Button Text</Label>
              <div className="flex gap-2">
                <input
                  type="color"
                  id="btn-text-color"
                  value={appearance.btn_text_color}
                  onChange={(e) => setAppearance({ ...appearance, btn_text_color: e.target.value })}
                  className="h-9 w-12 rounded border border-border cursor-pointer"
                />
                <Input
                  value={appearance.btn_text_color}
                  onChange={(e) => setAppearance({ ...appearance, btn_text_color: e.target.value })}
                  className="flex-1"
                />
              </div>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <Switch
              id="show-reject-all"
              checked={appearance.show_reject_all}
              onCheckedChange={(checked) =>
                setAppearance({ ...appearance, show_reject_all: checked })
              }
            />
            <Label htmlFor="show-reject-all">Show "Reject All" button</Label>
          </div>
        </CardContent>
      </Card>

      {/* Advanced */}
      <Card>
        <CardHeader>
          <CardTitle>Advanced Settings</CardTitle>
          <CardDescription>Cookie storage and language settings</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="cookie-name">Cookie Name</Label>
              <Input
                id="cookie-name"
                value={cookieName}
                onChange={(e) => setCookieName(e.target.value)}
                placeholder="etiquetta_consent"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="cookie-expiry">Cookie Expiry (days)</Label>
              <Input
                id="cookie-expiry"
                type="number"
                min={1}
                max={730}
                value={cookieExpiryDays}
                onChange={(e) => setCookieExpiryDays(Number(e.target.value))}
              />
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Switch
              id="auto-language"
              checked={autoLanguage}
              onCheckedChange={setAutoLanguage}
            />
            <Label htmlFor="auto-language">Auto-detect visitor language</Label>
          </div>
        </CardContent>
      </Card>

      {/* Save */}
      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={saveConfig.isPending}>
          <Save className="h-4 w-4 mr-2" />
          {saveConfig.isPending ? 'Saving...' : 'Save Configuration'}
        </Button>
      </div>
    </div>
  )
}

export function ConsentConfig() {
  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="mb-6">
        <div className="flex items-center gap-3 mb-2">
          <Button variant="ghost" size="sm" asChild>
            <Link to="/consent">
              <ArrowLeft className="h-4 w-4 mr-1" />
              Back
            </Link>
          </Button>
        </div>
        <h1 className="text-2xl font-bold text-foreground">Consent Configuration</h1>
        <p className="text-muted-foreground">Configure cookie consent banners for your domains</p>
      </div>
      <FeatureGate feature="consent">
        <ConsentConfigContent />
      </FeatureGate>
    </div>
  )
}
