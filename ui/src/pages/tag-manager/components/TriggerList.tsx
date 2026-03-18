import { useState } from 'react'
import { useTriggers, useDeleteTrigger } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { TriggerEditor } from './TriggerEditor'
import { TRIGGER_TYPE_LABELS } from './tag-templates'
import { Plus, Pencil, Trash2, Loader2, Zap, Hash, Code, Database, Type, Link, MousePointerClick, Eye, FormInput } from 'lucide-react'
import type { TMTrigger } from '@/lib/types'

interface TriggerListProps {
  containerId: string
  domain: string
}

const MATCH_TYPE_ICONS: Record<string, React.ReactNode> = {
  id: <Hash className="h-3 w-3" />,
  css: <Code className="h-3 w-3" />,
  data_attr: <Database className="h-3 w-3" />,
  text: <Type className="h-3 w-3" />,
  link_url: <Link className="h-3 w-3" />,
}

const TRIGGER_TYPE_ICONS: Record<string, React.ReactNode> = {
  click_specific: <MousePointerClick className="h-3.5 w-3.5" />,
  element_visibility: <Eye className="h-3.5 w-3.5" />,
  form_submit: <FormInput className="h-3.5 w-3.5" />,
}

function getTriggerSummary(trigger: TMTrigger): string | null {
  const config = trigger.config as Record<string, unknown>
  if (config.selector) {
    const selector = String(config.selector)
    return selector.length > 50 ? selector.slice(0, 50) + '...' : selector
  }
  if (config.event_name) return `Event: ${config.event_name}`
  if (config.percentage) return `${config.percentage}% scrolled`
  if (config.interval_ms) return `Every ${config.interval_ms}ms`
  if (config.threshold) return `${config.threshold}% visible`
  return null
}

export function TriggerList({ containerId, domain }: TriggerListProps) {
  const { data: triggers, isLoading } = useTriggers(containerId)
  const deleteTrigger = useDeleteTrigger(containerId)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingTrigger, setEditingTrigger] = useState<TMTrigger | undefined>()

  function handleAdd() {
    setEditingTrigger(undefined)
    setEditorOpen(true)
  }

  function handleEdit(trigger: TMTrigger) {
    setEditingTrigger(trigger)
    setEditorOpen(true)
  }

  function handleDelete(triggerId: string) {
    if (!confirm('Delete this trigger?')) return
    deleteTrigger.mutate(triggerId)
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Triggers</CardTitle>
          <Button size="sm" onClick={handleAdd}>
            <Plus className="h-4 w-4 mr-1" />
            Add Trigger
          </Button>
        </CardHeader>
        <CardContent>
          {!triggers || triggers.length === 0 ? (
            <div className="text-center py-8">
              <Zap className="h-10 w-10 text-muted-foreground/30 mx-auto mb-3" />
              <p className="text-sm text-muted-foreground">
                No triggers yet. Add a trigger to control when tags fire.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {triggers.map((trigger) => {
                const config = trigger.config as Record<string, unknown>
                const matchType = config.match_type as string | undefined
                const summary = getTriggerSummary(trigger)
                const conditionCount = Array.isArray(config.conditions) ? (config.conditions as unknown[]).length : 0

                return (
                  <div
                    key={trigger.id}
                    className="flex items-center justify-between p-3 rounded-lg border border-border hover:bg-muted/50 transition-colors"
                  >
                    <div className="min-w-0 space-y-1">
                      <div className="flex items-center gap-2">
                        {TRIGGER_TYPE_ICONS[trigger.trigger_type] && (
                          <span className="text-muted-foreground">
                            {TRIGGER_TYPE_ICONS[trigger.trigger_type]}
                          </span>
                        )}
                        <p className="font-medium text-sm truncate">{trigger.name}</p>
                      </div>
                      <div className="flex items-center gap-1.5 flex-wrap">
                        <Badge variant="secondary" className="text-xs">
                          {TRIGGER_TYPE_LABELS[trigger.trigger_type] ?? trigger.trigger_type}
                        </Badge>
                        {matchType && matchType !== 'css' && (
                          <TooltipProvider>
                            <Tooltip>
                              <TooltipTrigger>
                                <Badge variant="outline" className="text-[10px] gap-1">
                                  {MATCH_TYPE_ICONS[matchType]}
                                  {matchType === 'id' ? 'ID' : matchType === 'data_attr' ? 'Data Attr' : matchType === 'text' ? 'Text' : 'Link'}
                                </Badge>
                              </TooltipTrigger>
                              <TooltipContent>
                                <p>Matching by {matchType.replace('_', ' ')}</p>
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        )}
                        {conditionCount > 0 && (
                          <Badge variant="outline" className="text-[10px]">
                            {conditionCount} condition{conditionCount > 1 ? 's' : ''}
                          </Badge>
                        )}
                      </div>
                      {summary && (
                        <p className="text-xs text-muted-foreground font-mono truncate">{summary}</p>
                      )}
                    </div>
                    <div className="flex items-center gap-1 shrink-0 ml-2">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleEdit(trigger)}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDelete(trigger.id)}
                        disabled={deleteTrigger.isPending}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <TriggerEditor
        open={editorOpen}
        onOpenChange={setEditorOpen}
        containerId={containerId}
        domain={domain}
        trigger={editingTrigger}
      />
    </>
  )
}
