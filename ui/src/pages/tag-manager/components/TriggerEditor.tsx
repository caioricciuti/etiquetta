import { useState } from 'react'
import { useCreateTrigger, useUpdateTrigger, useVariables } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
  TRIGGER_CONFIG_FIELDS,
  CONDITION_OPERATORS,
  BUILT_IN_CONDITION_VARIABLES,
} from './tag-templates'
import { Loader2, Plus, X } from 'lucide-react'
import type { TMTrigger, TriggerType, TriggerCondition } from '@/lib/types'

interface TriggerEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  containerId: string
  trigger?: TMTrigger
}

interface TriggerFormState {
  name: string
  trigger_type: TriggerType
  config: Record<string, string>
  conditions: TriggerCondition[]
}

function getInitialState(trigger?: TMTrigger): TriggerFormState {
  if (trigger) {
    const config: Record<string, string> = {}
    const conditions: TriggerCondition[] = []
    for (const [k, v] of Object.entries(trigger.config)) {
      if (k === 'conditions' && Array.isArray(v)) {
        for (const c of v) {
          conditions.push({
            variable: String((c as Record<string, unknown>).variable ?? ''),
            operator: ((c as Record<string, unknown>).operator ?? 'equals') as TriggerCondition['operator'],
            value: String((c as Record<string, unknown>).value ?? ''),
          })
        }
      } else {
        config[k] = String(v ?? '')
      }
    }
    return {
      name: trigger.name,
      trigger_type: trigger.trigger_type,
      config,
      conditions,
    }
  }
  return {
    name: '',
    trigger_type: 'page_load',
    config: {},
    conditions: [],
  }
}

const TRIGGER_TYPES = Object.entries(TRIGGER_TYPE_LABELS) as [TriggerType, string][]

function TriggerEditorForm({
  trigger,
  containerId,
  onClose,
}: {
  trigger?: TMTrigger
  containerId: string
  onClose: () => void
}) {
  const [form, setForm] = useState<TriggerFormState>(() => getInitialState(trigger))
  const createTrigger = useCreateTrigger(containerId)
  const updateTrigger = useUpdateTrigger(containerId)
  const { data: variables } = useVariables(containerId)

  const isEditing = !!trigger
  const isPending = createTrigger.isPending || updateTrigger.isPending
  const configFields = TRIGGER_CONFIG_FIELDS[form.trigger_type] ?? []

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

    // Merge conditions into config
    const config: Record<string, unknown> = { ...form.config }
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

      <form onSubmit={handleSubmit} className="space-y-4 px-4">
        <div className="space-y-2">
          <Label htmlFor="trigger-name">Name</Label>
          <Input
            id="trigger-name"
            value={form.name}
            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            placeholder="e.g., All Pages"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="trigger-type">Trigger Type</Label>
          <Select
            value={form.trigger_type}
            onValueChange={(value) =>
              setForm((prev) => ({
                ...prev,
                trigger_type: value as TriggerType,
                config: {},
              }))
            }
          >
            <SelectTrigger id="trigger-type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TRIGGER_TYPES.map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

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
                <div key={i} className="flex items-center gap-2">
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
    </>
  )
}

export function TriggerEditor({ open, onOpenChange, containerId, trigger }: TriggerEditorProps) {
  const formKey = trigger?.id ?? 'new'

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="sm:max-w-md w-full overflow-y-auto">
        <TriggerEditorForm
          key={formKey}
          trigger={trigger}
          containerId={containerId}
          onClose={() => onOpenChange(false)}
        />
      </SheetContent>
    </Sheet>
  )
}
