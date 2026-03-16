import { useState } from 'react'
import { useCreateVariable, useUpdateVariable } from '@/hooks/useTagManager'
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
import { VARIABLE_TYPE_LABELS, VARIABLE_CONFIG_FIELDS } from './tag-templates'
import { Loader2 } from 'lucide-react'
import type { TMVariable, VariableType } from '@/lib/types'

interface VariableEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  containerId: string
  variable?: TMVariable
}

interface VariableFormState {
  name: string
  variable_type: VariableType
  config: Record<string, string>
}

function getInitialState(variable?: TMVariable): VariableFormState {
  if (variable) {
    const config: Record<string, string> = {}
    for (const [k, v] of Object.entries(variable.config)) {
      config[k] = String(v ?? '')
    }
    return {
      name: variable.name,
      variable_type: variable.variable_type,
      config,
    }
  }
  return {
    name: '',
    variable_type: 'data_layer',
    config: {},
  }
}

const VARIABLE_TYPES = Object.entries(VARIABLE_TYPE_LABELS) as [VariableType, string][]

function VariableEditorForm({
  variable,
  containerId,
  onClose,
}: {
  variable?: TMVariable
  containerId: string
  onClose: () => void
}) {
  const [form, setForm] = useState<VariableFormState>(() => getInitialState(variable))
  const createVariable = useCreateVariable(containerId)
  const updateVariable = useUpdateVariable(containerId)

  const isEditing = !!variable
  const isPending = createVariable.isPending || updateVariable.isPending
  const configFields = VARIABLE_CONFIG_FIELDS[form.variable_type] ?? []

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
      variable_type: form.variable_type,
      config: form.config as Record<string, unknown>,
    }

    if (isEditing && variable) {
      updateVariable.mutate(
        { id: variable.id, ...payload },
        { onSuccess: onClose }
      )
    } else {
      createVariable.mutate(payload, { onSuccess: onClose })
    }
  }

  return (
    <>
      <SheetHeader>
        <SheetTitle>{isEditing ? 'Edit Variable' : 'Add Variable'}</SheetTitle>
        <SheetDescription>
          {isEditing
            ? 'Update this variable configuration.'
            : 'Define a variable to capture dynamic values.'}
        </SheetDescription>
      </SheetHeader>

      <form onSubmit={handleSubmit} className="space-y-4 px-4">
        <div className="space-y-2">
          <Label htmlFor="variable-name">Name</Label>
          <Input
            id="variable-name"
            value={form.name}
            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            placeholder="e.g., Page URL"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="variable-type">Variable Type</Label>
          <Select
            value={form.variable_type}
            onValueChange={(value) =>
              setForm((prev) => ({
                ...prev,
                variable_type: value as VariableType,
                config: {},
              }))
            }
          >
            <SelectTrigger id="variable-type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {VARIABLE_TYPES.map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {configFields.map((field) => (
          <div key={field.key} className="space-y-2">
            <Label htmlFor={`var-config-${field.key}`}>{field.label}</Label>
            <Input
              id={`var-config-${field.key}`}
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
            {isEditing ? 'Save Changes' : 'Create Variable'}
          </Button>
        </SheetFooter>
      </form>
    </>
  )
}

export function VariableEditor({ open, onOpenChange, containerId, variable }: VariableEditorProps) {
  const formKey = variable?.id ?? 'new'

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="sm:max-w-md w-full overflow-y-auto">
        <VariableEditorForm
          key={formKey}
          variable={variable}
          containerId={containerId}
          onClose={() => onOpenChange(false)}
        />
      </SheetContent>
    </Sheet>
  )
}
