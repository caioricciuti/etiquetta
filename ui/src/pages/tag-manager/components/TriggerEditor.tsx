import { useState } from 'react'
import { useCreateTrigger, useUpdateTrigger, useVariables } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
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
import {
  TRIGGER_TYPE_LABELS,
  TRIGGER_TYPE_DESCRIPTIONS,
  TRIGGER_CONFIG_FIELDS,
  CONDITION_OPERATORS,
  BUILT_IN_CONDITION_VARIABLES,
  SELECTOR_TRIGGER_TYPES,
} from './tag-templates'
import { SelectorBuilder } from './SelectorBuilder'
import type { SelectorState } from './SelectorBuilder'
import { ElementPicker } from './ElementPicker'
import type { PickerSuggestion } from './ElementPicker'
import { Loader2, Plus, X, Info } from 'lucide-react'
import type { TMTrigger, TriggerType, TriggerCondition, SelectorMatchType } from '@/lib/types'

interface TriggerEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  containerId: string
  domain: string
  trigger?: TMTrigger
}

interface TriggerFormState {
  name: string
  trigger_type: TriggerType
  config: Record<string, string>
  conditions: TriggerCondition[]
  selectorState: SelectorState
}

function buildFinalSelector(s: SelectorState): string {
  switch (s.match_type) {
    case 'id':
      return s.selector ? `#${s.selector.replace(/^#/, '')}` : ''
    case 'data_attr':
      if (!s.data_attr_name) return ''
      return s.data_attr_value ? `[data-${s.data_attr_name}="${s.data_attr_value}"]` : `[data-${s.data_attr_name}]`
    case 'link_url':
      return s.selector ? `a[href*="${s.selector}"]` : ''
    case 'text':
    case 'css':
    default:
      return s.selector
  }
}

const DEFAULT_SELECTOR_STATE: SelectorState = {
  selector: '',
  match_type: 'css',
  data_attr_name: '',
  data_attr_value: '',
  text_match_mode: 'contains',
}

function getInitialState(trigger?: TMTrigger): TriggerFormState {
  if (trigger) {
    const config: Record<string, string> = {}
    const conditions: TriggerCondition[] = []
    const selectorState: SelectorState = { ...DEFAULT_SELECTOR_STATE }

    for (const [k, v] of Object.entries(trigger.config)) {
      if (k === 'conditions' && Array.isArray(v)) {
        for (const c of v) {
          conditions.push({
            variable: String((c as Record<string, unknown>).variable ?? ''),
            operator: ((c as Record<string, unknown>).operator ?? 'equals') as TriggerCondition['operator'],
            value: String((c as Record<string, unknown>).value ?? ''),
          })
        }
      } else if (k === 'selector') {
        selectorState.selector = String(v ?? '')
      } else if (k === 'match_type') {
        selectorState.match_type = (v as SelectorMatchType) ?? 'css'
      } else if (k === 'data_attr_name') {
        selectorState.data_attr_name = String(v ?? '')
      } else if (k === 'data_attr_value') {
        selectorState.data_attr_value = String(v ?? '')
      } else if (k === 'text_match_mode') {
        selectorState.text_match_mode = String(v ?? 'contains')
      } else {
        config[k] = String(v ?? '')
      }
    }
    return { name: trigger.name, trigger_type: trigger.trigger_type, config, conditions, selectorState }
  }
  return {
    name: '',
    trigger_type: 'page_load',
    config: {},
    conditions: [],
    selectorState: { ...DEFAULT_SELECTOR_STATE },
  }
}

// Group trigger types for better organization
const TRIGGER_GROUPS = [
  {
    label: 'Page',
    types: ['page_load', 'dom_ready', 'history_change'] as TriggerType[],
  },
  {
    label: 'Interaction',
    types: ['click_all', 'click_specific', 'form_submit'] as TriggerType[],
  },
  {
    label: 'Engagement',
    types: ['scroll_depth', 'element_visibility', 'timer'] as TriggerType[],
  },
  {
    label: 'Custom',
    types: ['custom_event'] as TriggerType[],
  },
]

function TriggerEditorForm({
  trigger,
  containerId,
  domain,
  onClose,
}: {
  trigger?: TMTrigger
  containerId: string
  domain: string
  onClose: () => void
}) {
  const [form, setForm] = useState<TriggerFormState>(() => getInitialState(trigger))
  const [pickerOpen, setPickerOpen] = useState(false)
  const createTrigger = useCreateTrigger(containerId)
  const updateTrigger = useUpdateTrigger(containerId)
  const { data: variables } = useVariables(containerId)

  const isEditing = !!trigger
  const isPending = createTrigger.isPending || updateTrigger.isPending
  const configFields = TRIGGER_CONFIG_FIELDS[form.trigger_type] ?? []
  const needsSelector = SELECTOR_TRIGGER_TYPES.has(form.trigger_type)

  // Build variable options: built-in + user-defined
  const variableOptions = [
    ...BUILT_IN_CONDITION_VARIABLES,
    ...(variables ?? []).map((v) => ({ label: v.name, value: v.name })),
  ]

  function handleConfigChange(key: string, value: string) {
    setForm((prev) => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }))
  }

  function handleSelectorChange(newState: SelectorState) {
    setForm((prev) => ({ ...prev, selectorState: newState }))
  }

  function handlePickerSelect(suggestion: PickerSuggestion) {
    setForm((prev) => ({
      ...prev,
      selectorState: {
        selector: suggestion.selector,
        match_type: suggestion.type,
        data_attr_name: suggestion.data_attr_name ?? '',
        data_attr_value: suggestion.data_attr_value ?? '',
        text_match_mode: prev.selectorState.text_match_mode,
      },
    }))
  }

  function addCondition() {
    setForm((prev) => ({
      ...prev,
      conditions: [...prev.conditions, { variable: 'page_path', operator: 'equals', value: '' }],
    }))
  }

  function removeCondition(index: number) {
    setForm((prev) => ({
      ...prev,
      conditions: prev.conditions.filter((_, i) => i !== index),
    }))
  }

  function updateCondition(index: number, field: keyof TriggerCondition, value: string) {
    setForm((prev) => ({
      ...prev,
      conditions: prev.conditions.map((c, i) =>
        i === index ? { ...c, [field]: value } : c
      ),
    }))
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.name.trim()) return

    // Build config
    const config: Record<string, unknown> = { ...form.config }

    // Add selector config for selector-based triggers
    if (needsSelector) {
      const ss = form.selectorState
      const finalSel = buildFinalSelector(ss)
      if (finalSel) {
        config.selector = finalSel
        config.match_type = ss.match_type
        if (ss.match_type === 'data_attr') {
          config.data_attr_name = ss.data_attr_name
          config.data_attr_value = ss.data_attr_value
        }
        if (ss.match_type === 'text') {
          config.text_match_mode = ss.text_match_mode
        }
      }
    }

    if (form.conditions.length > 0) {
      config.conditions = form.conditions
    }

    const payload = {
      name: form.name.trim(),
      trigger_type: form.trigger_type,
      config,
    }

    if (isEditing && trigger) {
      updateTrigger.mutate(
        { id: trigger.id, ...payload },
        { onSuccess: onClose }
      )
    } else {
      createTrigger.mutate(payload, { onSuccess: onClose })
    }
  }

  return (
    <>
      <SheetHeader>
        <SheetTitle>{isEditing ? 'Edit Trigger' : 'Add Trigger'}</SheetTitle>
        <SheetDescription>
          {isEditing
            ? 'Update this trigger configuration.'
            : 'Define when tags should fire.'}
        </SheetDescription>
      </SheetHeader>

      <form onSubmit={handleSubmit} className="space-y-5 px-4">
        {/* Name */}
        <div className="space-y-2">
          <Label htmlFor="trigger-name">Name</Label>
          <Input
            id="trigger-name"
            value={form.name}
            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            placeholder="e.g., CTA Button Click"
            required
          />
        </div>

        {/* Trigger Type - Grouped */}
        <div className="space-y-2">
          <Label>Trigger Type</Label>
          <div className="space-y-2">
            {TRIGGER_GROUPS.map((group) => (
              <div key={group.label}>
                <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider mb-1.5 px-1">
                  {group.label}
                </p>
                <div className="grid grid-cols-2 gap-1.5">
                  {group.types.map((type) => {
                    const isActive = form.trigger_type === type
                    return (
                      <button
                        key={type}
                        type="button"
                        onClick={() =>
                          setForm((prev) => ({
                            ...prev,
                            trigger_type: type,
                            config: {},
                            selectorState: { ...DEFAULT_SELECTOR_STATE },
                          }))
                        }
                        className={`flex items-center gap-2 rounded-md border px-3 py-2 text-xs text-left transition-colors ${
                          isActive
                            ? 'border-primary bg-primary/10 text-primary font-medium'
                            : 'border-border hover:bg-muted/50 text-foreground'
                        }`}
                      >
                        <span className="truncate">{TRIGGER_TYPE_LABELS[type]}</span>
                      </button>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
          {/* Type Description */}
          {TRIGGER_TYPE_DESCRIPTIONS[form.trigger_type] && (
            <div className="flex items-start gap-2 rounded-md bg-muted/50 px-3 py-2">
              <Info className="h-3.5 w-3.5 text-muted-foreground mt-0.5 shrink-0" />
              <p className="text-xs text-muted-foreground">
                {TRIGGER_TYPE_DESCRIPTIONS[form.trigger_type]}
              </p>
            </div>
          )}
        </div>

        {/* Selector Builder (for click_specific, form_submit, element_visibility) */}
        {needsSelector && (
          <SelectorBuilder
            state={form.selectorState}
            onChange={handleSelectorChange}
            onPickElement={() => setPickerOpen(true)}
            triggerType={form.trigger_type}
            isPickerAvailable
          />
        )}

        {/* Standard Config Fields (scroll percentage, timer, custom event, etc.) */}
        {configFields.map((field) => (
          <div key={field.key} className="space-y-2">
            <Label htmlFor={`trigger-config-${field.key}`}>{field.label}</Label>
            <Input
              id={`trigger-config-${field.key}`}
              type={field.type}
              value={form.config[field.key] ?? ''}
              onChange={(e) => handleConfigChange(field.key, e.target.value)}
              placeholder={field.placeholder}
              required={field.required}
            />
          </div>
        ))}

        {/* Conditions */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <Label>Conditions</Label>
            <Button type="button" variant="outline" size="sm" onClick={addCondition}>
              <Plus className="h-3.5 w-3.5 mr-1" />
              Add
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            All conditions must match for this trigger to fire.
          </p>
          {form.conditions.length > 0 && (
            <div className="space-y-2 rounded-md border p-3">
              {form.conditions.map((cond, i) => (
                <div key={i} className="space-y-1.5">
                  {i > 0 && (
                    <div className="flex items-center gap-2 px-1">
                      <div className="h-px flex-1 bg-border" />
                      <Badge variant="outline" className="text-[10px]">AND</Badge>
                      <div className="h-px flex-1 bg-border" />
                    </div>
                  )}
                  <div className="flex items-center gap-2">
                    <Select
                      value={cond.variable}
                      onValueChange={(v) => updateCondition(i, 'variable', v)}
                    >
                      <SelectTrigger className="w-[140px] h-8 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {variableOptions.map((v) => (
                          <SelectItem key={v.value} value={v.value}>
                            {v.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Select
                      value={cond.operator}
                      onValueChange={(v) => updateCondition(i, 'operator', v)}
                    >
                      <SelectTrigger className="w-[130px] h-8 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {CONDITION_OPERATORS.map((op) => (
                          <SelectItem key={op.value} value={op.value}>
                            {op.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Input
                      value={cond.value}
                      onChange={(e) => updateCondition(i, 'value', e.target.value)}
                      placeholder="value"
                      className="h-8 text-xs flex-1"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-8 w-8 p-0"
                      onClick={() => removeCondition(i)}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <SheetFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={isPending || !form.name.trim()}>
            {isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            {isEditing ? 'Save Changes' : 'Create Trigger'}
          </Button>
        </SheetFooter>
      </form>

      <ElementPicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        containerId={containerId}
        domain={domain}
        onSelect={handlePickerSelect}
      />
    </>
  )
}

export function TriggerEditor({ open, onOpenChange, containerId, domain, trigger }: TriggerEditorProps) {
  const formKey = trigger?.id ?? 'new'

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="sm:max-w-lg w-full overflow-y-auto">
        <TriggerEditorForm
          key={formKey}
          trigger={trigger}
          containerId={containerId}
          domain={domain}
          onClose={() => onOpenChange(false)}
        />
      </SheetContent>
    </Sheet>
  )
}
