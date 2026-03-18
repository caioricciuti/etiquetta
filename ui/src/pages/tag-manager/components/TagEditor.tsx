import { useState } from 'react'
import { useCreateTag, useUpdateTag, useVariables } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CodeEditor } from './CodeEditor'
import { Switch } from '@/components/ui/switch'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TAG_TEMPLATES, CONSENT_CATEGORIES, getTemplate } from './tag-templates'
import { Loader2, ShieldOff } from 'lucide-react'
import type { TMTag, TMTrigger, TagType } from '@/lib/types'

interface TagEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  containerId: string
  tag?: TMTag
  triggers: TMTrigger[]
}

interface TagFormState {
  name: string
  tag_type: TagType
  config: Record<string, string>
  consent_category: string
  priority: number
  is_enabled: boolean
  trigger_ids: string[]
  exception_trigger_ids: string[]
}

function getInitialState(tag?: TMTag): TagFormState {
  if (tag) {
    const config: Record<string, string> = {}
    for (const [k, v] of Object.entries(tag.config)) {
      config[k] = String(v ?? '')
    }
    return {
      name: tag.name,
      tag_type: tag.tag_type,
      config,
      consent_category: tag.consent_category,
      priority: tag.priority,
      is_enabled: tag.is_enabled,
      trigger_ids: [...tag.trigger_ids],
      exception_trigger_ids: [...tag.exception_trigger_ids],
    }
  }
  return {
    name: '',
    tag_type: 'custom_html',
    config: {},
    consent_category: 'analytics',
    priority: 0,
    is_enabled: true,
    trigger_ids: [],
    exception_trigger_ids: [],
  }
}

function TagEditorForm({
  tag,
  containerId,
  triggers,
  onClose,
}: {
  tag?: TMTag
  containerId: string
  triggers: TMTrigger[]
  onClose: () => void
}) {
  const [form, setForm] = useState<TagFormState>(() => getInitialState(tag))
  const createTag = useCreateTag(containerId)
  const updateTag = useUpdateTag(containerId)
  const { data: variables = [] } = useVariables(containerId)
  const variableNames = variables.map((v) => v.name)

  const isEditing = !!tag
  const isPending = createTag.isPending || updateTag.isPending
  const template = getTemplate(form.tag_type)

  function handleConfigChange(key: string, value: string) {
    setForm((prev) => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }))
  }

  function handleFiringTriggerToggle(triggerId: string) {
    setForm((prev) => {
      const ids = prev.trigger_ids.includes(triggerId)
        ? prev.trigger_ids.filter((id) => id !== triggerId)
        : [...prev.trigger_ids, triggerId]
      // Remove from exception if adding to firing
      const excIds = ids.includes(triggerId)
        ? prev.exception_trigger_ids.filter((id) => id !== triggerId)
        : prev.exception_trigger_ids
      return { ...prev, trigger_ids: ids, exception_trigger_ids: excIds }
    })
  }

  function handleExceptionTriggerToggle(triggerId: string) {
    setForm((prev) => {
      const ids = prev.exception_trigger_ids.includes(triggerId)
        ? prev.exception_trigger_ids.filter((id) => id !== triggerId)
        : [...prev.exception_trigger_ids, triggerId]
      // Remove from firing if adding to exception
      const fireIds = ids.includes(triggerId)
        ? prev.trigger_ids.filter((id) => id !== triggerId)
        : prev.trigger_ids
      return { ...prev, exception_trigger_ids: ids, trigger_ids: fireIds }
    })
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.name.trim()) return

    const payload = {
      name: form.name.trim(),
      tag_type: form.tag_type,
      config: form.config as Record<string, unknown>,
      consent_category: form.consent_category,
      priority: form.priority,
      is_enabled: form.is_enabled,
      trigger_ids: form.trigger_ids,
      exception_trigger_ids: form.exception_trigger_ids,
    }

    if (isEditing && tag) {
      updateTag.mutate(
        { id: tag.id, ...payload },
        { onSuccess: onClose }
      )
    } else {
      createTag.mutate(payload, { onSuccess: onClose })
    }
  }

  return (
    <>
      <SheetHeader>
        <SheetTitle>{isEditing ? 'Edit Tag' : 'Add Tag'}</SheetTitle>
        <SheetDescription>
          {isEditing
            ? 'Update this tag configuration.'
            : 'Configure a new tag for your container.'}
        </SheetDescription>
      </SheetHeader>

      <form onSubmit={handleSubmit} className="space-y-4 px-4">
        <div className="space-y-2">
          <Label htmlFor="tag-name">Name</Label>
          <Input
            id="tag-name"
            value={form.name}
            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            placeholder="e.g., GA4 Pageview"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="tag-type">Tag Type</Label>
          <Select
            value={form.tag_type}
            onValueChange={(value) =>
              setForm((prev) => ({ ...prev, tag_type: value as TagType, config: {} }))
            }
          >
            <SelectTrigger id="tag-type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TAG_TEMPLATES.map((t) => (
                <SelectItem key={t.type} value={t.type}>
                  <span className="flex flex-col">
                    <span>{t.name}</span>
                    <span className="text-xs text-muted-foreground">{t.description}</span>
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {template?.configFields.map((field) => (
          <div key={field.key} className="space-y-2">
            <Label htmlFor={`config-${field.key}`}>{field.label}</Label>
            {field.type === 'textarea' ? (
              <CodeEditor
                value={form.config[field.key] ?? ''}
                onChange={(v) => handleConfigChange(field.key, v)}
                language={field.key === 'event_props' ? 'json' : 'html'}
                height="200px"
                placeholder={field.placeholder}
                variables={variableNames}
              />
            ) : field.type === 'select' && field.options ? (
              <Select
                value={form.config[field.key] ?? ''}
                onValueChange={(v) => handleConfigChange(field.key, v)}
              >
                <SelectTrigger id={`config-${field.key}`}>
                  <SelectValue placeholder={field.placeholder} />
                </SelectTrigger>
                <SelectContent>
                  {field.options.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id={`config-${field.key}`}
                value={form.config[field.key] ?? ''}
                onChange={(e) => handleConfigChange(field.key, e.target.value)}
                placeholder={field.placeholder}
                required={field.required}
              />
            )}
            <p className="text-xs text-muted-foreground">
              {'Use {{Variable Name}} for dynamic values'}
            </p>
          </div>
        ))}

        <div className="space-y-2">
          <Label htmlFor="consent-category">Consent Category</Label>
          <Select
            value={form.consent_category}
            onValueChange={(v) => setForm((prev) => ({ ...prev, consent_category: v }))}
          >
            <SelectTrigger id="consent-category">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CONSENT_CATEGORIES.map((cat) => (
                <SelectItem key={cat.value} value={cat.value}>
                  {cat.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label htmlFor="priority">Priority</Label>
          <Input
            id="priority"
            type="number"
            min={0}
            max={1000}
            value={form.priority}
            onChange={(e) =>
              setForm((prev) => ({ ...prev, priority: parseInt(e.target.value, 10) || 0 }))
            }
            placeholder="0"
          />
          <p className="text-xs text-muted-foreground">
            Higher priority tags fire first. Default is 0.
          </p>
        </div>

        <div className="flex items-center justify-between">
          <Label htmlFor="is-enabled">Enabled</Label>
          <Switch
            id="is-enabled"
            checked={form.is_enabled}
            onCheckedChange={(checked) =>
              setForm((prev) => ({ ...prev, is_enabled: checked }))
            }
          />
        </div>

        {triggers.length > 0 && (
          <>
            <div className="space-y-2">
              <Label>Firing Triggers</Label>
              <p className="text-xs text-muted-foreground">Tag fires when any of these triggers match.</p>
              <div className="space-y-1.5 max-h-[140px] overflow-y-auto rounded-md border p-3">
                {triggers.map((trigger) => (
                  <label
                    key={trigger.id}
                    className="flex items-center gap-2 text-sm cursor-pointer hover:bg-muted/50 rounded px-1 py-0.5"
                  >
                    <input
                      type="checkbox"
                      checked={form.trigger_ids.includes(trigger.id)}
                      onChange={() => handleFiringTriggerToggle(trigger.id)}
                      className="rounded border-input"
                    />
                    <span>{trigger.name}</span>
                    <span className="text-xs text-muted-foreground ml-auto">
                      {trigger.trigger_type}
                    </span>
                  </label>
                ))}
              </div>
            </div>

            <div className="space-y-2">
              <Label className="flex items-center gap-1.5">
                <ShieldOff className="h-3.5 w-3.5" />
                Blocking (Exception) Triggers
              </Label>
              <p className="text-xs text-muted-foreground">
                If any of these triggers match, the tag is blocked even if a firing trigger matches.
              </p>
              <div className="space-y-1.5 max-h-[140px] overflow-y-auto rounded-md border p-3">
                {triggers.map((trigger) => (
                  <label
                    key={trigger.id}
                    className="flex items-center gap-2 text-sm cursor-pointer hover:bg-muted/50 rounded px-1 py-0.5"
                  >
                    <input
                      type="checkbox"
                      checked={form.exception_trigger_ids.includes(trigger.id)}
                      onChange={() => handleExceptionTriggerToggle(trigger.id)}
                      className="rounded border-input"
                    />
                    <span>{trigger.name}</span>
                    <span className="text-xs text-muted-foreground ml-auto">
                      {trigger.trigger_type}
                    </span>
                  </label>
                ))}
              </div>
            </div>
          </>
        )}

        <SheetFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={isPending || !form.name.trim()}>
            {isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            {isEditing ? 'Save Changes' : 'Create Tag'}
          </Button>
        </SheetFooter>
      </form>
    </>
  )
}

export function TagEditor({ open, onOpenChange, containerId, tag, triggers }: TagEditorProps) {
  // Key forces form remount when switching between tags or opening fresh
  const formKey = tag?.id ?? 'new'

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="sm:max-w-lg w-full overflow-y-auto">
        <TagEditorForm
          key={formKey}
          tag={tag}
          containerId={containerId}
          triggers={triggers}
          onClose={() => onOpenChange(false)}
        />
      </SheetContent>
    </Sheet>
  )
}
