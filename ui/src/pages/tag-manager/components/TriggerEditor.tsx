import { useState } from 'react'
import { useCreateTrigger, useUpdateTrigger } from '@/hooks/useTagManager'
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
import { TRIGGER_TYPE_LABELS, TRIGGER_CONFIG_FIELDS } from './tag-templates'
import { Loader2 } from 'lucide-react'
import type { TMTrigger, TriggerType } from '@/lib/types'

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
}

function getInitialState(trigger?: TMTrigger): TriggerFormState {
  if (trigger) {
    const config: Record<string, string> = {}
    for (const [k, v] of Object.entries(trigger.config)) {
      config[k] = String(v ?? '')
    }
    return {
      name: trigger.name,
      trigger_type: trigger.trigger_type,
      config,
    }
  }
  return {
    name: '',
    trigger_type: 'page_load',
    config: {},
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

  const isEditing = !!trigger
  const isPending = createTrigger.isPending || updateTrigger.isPending
  const configFields = TRIGGER_CONFIG_FIELDS[form.trigger_type] ?? []

  function handleConfigChange(key: string, value: string) {
    setForm((prev) => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }))
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.name.trim()) return

    const payload = {
      name: form.name.trim(),
      trigger_type: form.trigger_type,
      config: form.config as Record<string, unknown>,
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
