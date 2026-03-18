import { useCallback } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { SELECTOR_MATCH_TYPES } from './tag-templates'
import { Code, Hash, Database, Type, Link, Crosshair, HelpCircle } from 'lucide-react'
import type { SelectorMatchType } from '@/lib/types'

const MATCH_TYPE_ICONS: Record<string, React.ReactNode> = {
  css: <Code className="h-3.5 w-3.5" />,
  id: <Hash className="h-3.5 w-3.5" />,
  data_attr: <Database className="h-3.5 w-3.5" />,
  text: <Type className="h-3.5 w-3.5" />,
  link_url: <Link className="h-3.5 w-3.5" />,
}

export interface SelectorState {
  selector: string
  match_type: SelectorMatchType
  data_attr_name: string
  data_attr_value: string
  text_match_mode: string
}

interface SelectorBuilderProps {
  state: SelectorState
  onChange: (state: SelectorState) => void
  onPickElement?: () => void
  triggerType: string
  isPickerAvailable?: boolean
}

function buildSelectorPreview(s: SelectorState): string {
  switch (s.match_type) {
    case 'id':
      return s.selector ? `#${s.selector.replace(/^#/, '')}` : ''
    case 'data_attr':
      if (!s.data_attr_name) return ''
      return s.data_attr_value ? `[data-${s.data_attr_name}="${s.data_attr_value}"]` : `[data-${s.data_attr_name}]`
    case 'text':
      return s.selector ? `*:contains("${s.selector}")` : ''
    case 'link_url':
      return s.selector ? `a[href*="${s.selector}"]` : ''
    case 'css':
    default:
      return s.selector
  }
}


export function SelectorBuilder({
  state,
  onChange,
  onPickElement,
  triggerType,
  isPickerAvailable = false,
}: SelectorBuilderProps) {
  const handleChange = useCallback((partial: Partial<SelectorState>) => {
    onChange({ ...state, ...partial })
  }, [state, onChange])

  const handleMatchTypeChange = useCallback((newType: SelectorMatchType) => {
    onChange({
      selector: '',
      match_type: newType,
      data_attr_name: '',
      data_attr_value: '',
      text_match_mode: state.text_match_mode,
    })
  }, [state.text_match_mode, onChange])

  const preview = buildSelectorPreview(state)
  const isFormSubmit = triggerType === 'form_submit'

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <Label className="text-sm font-medium">
          {isFormSubmit ? 'Form Selector' : 'Element Selector'}
        </Label>
        {isPickerAvailable && onPickElement && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={onPickElement}
                  className="gap-1.5 text-xs"
                >
                  <Crosshair className="h-3.5 w-3.5" />
                  Pick Element
                </Button>
              </TooltipTrigger>
              <TooltipContent side="left">
                <p>Open your site and click to select an element</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </div>

      {/* Match Type Selector */}
      <div className={isFormSubmit ? 'grid grid-cols-2 gap-1' : 'grid grid-cols-5 gap-1'}>
        {SELECTOR_MATCH_TYPES.map((mt) => {
          if (isFormSubmit && !['css', 'id'].includes(mt.value)) return null
          const isActive = state.match_type === mt.value
          return (
            <TooltipProvider key={mt.value}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={() => handleMatchTypeChange(mt.value as SelectorMatchType)}
                    className={`flex flex-col items-center gap-1 rounded-md border p-2 text-xs transition-colors ${
                      isActive
                        ? 'border-primary bg-primary/10 text-primary'
                        : 'border-border hover:bg-muted/50 text-muted-foreground'
                    }`}
                  >
                    {MATCH_TYPE_ICONS[mt.value]}
                    <span className="truncate w-full text-center text-[10px] leading-tight">{mt.label}</span>
                  </button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{mt.description}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )
        })}
      </div>

      {/* Input Fields Based on Match Type */}
      <div className="space-y-2">
        {state.match_type === 'css' && (
          <div className="space-y-1.5">
            <Input
              value={state.selector}
              onChange={(e) => handleChange({ selector: e.target.value })}
              placeholder={isFormSubmit ? 'form#checkout, form.signup' : '#buy-btn, .cta-button, [data-action]'}
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              Use any valid CSS selector. Separate multiple selectors with commas.
            </p>
          </div>
        )}

        {state.match_type === 'id' && (
          <div className="space-y-1.5">
            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground font-mono text-sm">#</span>
              <Input
                value={state.selector.replace(/^#/, '')}
                onChange={(e) => handleChange({ selector: e.target.value.replace(/^#/, '') })}
                placeholder="buy-button"
                className="font-mono text-xs"
              />
            </div>
            <p className="text-[11px] text-muted-foreground">
              The element's ID attribute (without the # prefix).
            </p>
          </div>
        )}

        {state.match_type === 'data_attr' && (
          <div className="space-y-2">
            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground font-mono text-xs whitespace-nowrap">data-</span>
              <Input
                value={state.data_attr_name}
                onChange={(e) => handleChange({ data_attr_name: e.target.value })}
                placeholder="action"
                className="font-mono text-xs"
              />
              <span className="text-muted-foreground font-mono text-xs">=</span>
              <Input
                value={state.data_attr_value}
                onChange={(e) => handleChange({ data_attr_value: e.target.value })}
                placeholder="checkout (optional)"
                className="font-mono text-xs"
              />
            </div>
            <p className="text-[11px] text-muted-foreground">
              Match elements with a specific data attribute. Leave value empty to match any element with the attribute.
            </p>
          </div>
        )}

        {state.match_type === 'text' && (
          <div className="space-y-2">
            <Input
              value={state.selector}
              onChange={(e) => handleChange({ selector: e.target.value })}
              placeholder="Add to Cart"
              className="text-xs"
            />
            <div className="flex items-center gap-2">
              <Select
                value={state.text_match_mode || 'contains'}
                onValueChange={(v) => handleChange({ text_match_mode: v })}
              >
                <SelectTrigger className="w-[140px] h-7 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="contains">Contains text</SelectItem>
                  <SelectItem value="exact">Exact match</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-[11px] text-muted-foreground">
                Match visible text content of clickable elements.
              </p>
            </div>
          </div>
        )}

        {state.match_type === 'link_url' && (
          <div className="space-y-1.5">
            <Input
              value={state.selector}
              onChange={(e) => handleChange({ selector: e.target.value })}
              placeholder="/checkout, /pricing"
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              Match links whose href contains this text.
            </p>
          </div>
        )}
      </div>

      {/* Selector Preview */}
      {preview && (
        <div className="flex items-center gap-2 rounded-md bg-muted/50 border border-border px-3 py-2">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger>
                <HelpCircle className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              </TooltipTrigger>
              <TooltipContent>
                <p>This is the generated selector that will be used at runtime</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <code className="text-xs font-mono text-foreground/80 truncate flex-1">{preview}</code>
          <Badge variant="secondary" className="text-[10px] shrink-0">
            {state.match_type === 'text' ? 'runtime match' : 'CSS'}
          </Badge>
        </div>
      )}
    </div>
  )
}
