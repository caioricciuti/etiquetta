import { useEffect, useRef, useCallback, useState } from 'react'
import { usePreviewToken } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Loader2,
  Crosshair,
  ExternalLink,
  Hash,
  Code,
  Database,
  Type,
  Link,
  Check,
  AlertTriangle,
  RefreshCw,
  X,
} from 'lucide-react'
import type { SelectorMatchType } from '@/lib/types'

export interface PickerSuggestion {
  type: SelectorMatchType
  label: string
  selector: string
  specificity: number
  data_attr_name?: string
  data_attr_value?: string
}

interface ElementPickerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  containerId: string
  domain: string
  onSelect: (suggestion: PickerSuggestion) => void
}

const SUGGESTION_ICONS: Record<string, React.ReactNode> = {
  id: <Hash className="h-3.5 w-3.5" />,
  css: <Code className="h-3.5 w-3.5" />,
  data_attr: <Database className="h-3.5 w-3.5" />,
  text: <Type className="h-3.5 w-3.5" />,
  link_url: <Link className="h-3.5 w-3.5" />,
}

const SUGGESTION_TYPE_LABELS: Record<string, string> = {
  id: 'Element ID',
  css: 'CSS Selector',
  data_attr: 'Data Attribute',
  text: 'Text Content',
  link_url: 'Link URL',
}

type PickerStatus = 'generating' | 'loading' | 'ready' | 'selected'

interface HoverInfo {
  tag: string
  id: string
  classes: string
  text: string
}

function ensureProtocol(domain: string): string {
  if (domain.startsWith('http://') || domain.startsWith('https://')) return domain
  return `https://${domain}`
}

export function ElementPicker({ open, onOpenChange, containerId, domain, onSelect }: ElementPickerProps) {
  const previewToken = usePreviewToken(containerId)
  const [status, setStatus] = useState<PickerStatus>('generating')
  const [suggestions, setSuggestions] = useState<PickerSuggestion[]>([])
  const [elementInfo, setElementInfo] = useState<{ tag: string; text: string } | null>(null)
  const [hoverInfo, setHoverInfo] = useState<HoverInfo | null>(null)
  const [iframeUrl, setIframeUrl] = useState<string | null>(null)
  const [urlInput, setUrlInput] = useState('')
  const [token, setToken] = useState<string | null>(null)
  const [warning, setWarning] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const readyTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Generate token and set initial URL when dialog opens
  useEffect(() => {
    if (!open) return
    setStatus('generating')
    setSuggestions([])
    setElementInfo(null)
    setHoverInfo(null)
    setWarning(false)
    setIframeUrl(null)
    setToken(null)

    const initialUrl = ensureProtocol(domain)
    setUrlInput(initialUrl)

    previewToken.mutate(undefined, {
      onSuccess: (data) => {
        const t = data.token
        setToken(t)
        const proxyUrl = `/api/tagmanager/pick-proxy?url=${encodeURIComponent(initialUrl)}&token=${encodeURIComponent(t)}`
        setIframeUrl(proxyUrl)
        setStatus('loading')
      },
      onError: () => {
        setStatus('generating')
      },
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, containerId, domain])

  // Navigate to a URL within the picker
  function navigateToUrl(url: string) {
    if (!token) return
    setUrlInput(url)
    const proxyUrl = `/api/tagmanager/pick-proxy?url=${encodeURIComponent(url)}&token=${encodeURIComponent(token)}`
    setIframeUrl(proxyUrl)
    setStatus('loading')
    setWarning(false)
    setSuggestions([])
    setElementInfo(null)
    setHoverInfo(null)
  }

  function handleNavigate() {
    if (!urlInput.trim()) return
    navigateToUrl(ensureProtocol(urlInput.trim()))
  }

  // Handle iframe load event — start timeout for picker_ready
  const handleIframeLoad = useCallback(() => {
    if (readyTimeoutRef.current) clearTimeout(readyTimeoutRef.current)
    readyTimeoutRef.current = setTimeout(() => {
      // If still loading after 5s, show warning
      setStatus((prev) => {
        if (prev === 'loading') {
          setWarning(true)
          return 'ready' // let them still try
        }
        return prev
      })
    }, 5000)
  }, [])

  // Listen for postMessage from iframe
  const handleMessage = useCallback((event: MessageEvent) => {
    const data = event.data
    if (!data || typeof data !== 'object') return

    switch (data.type) {
      case 'etiquetta_picker_ready':
        if (readyTimeoutRef.current) clearTimeout(readyTimeoutRef.current)
        setWarning(false)
        setStatus('ready')
        break
      case 'etiquetta_picker_hover':
        setHoverInfo({
          tag: data.tag ?? '',
          id: data.id ?? '',
          classes: data.classes ?? '',
          text: data.text ?? '',
        })
        break
      case 'etiquetta_picker_result':
        setSuggestions(
          (data.suggestions as PickerSuggestion[]).sort((a, b) => b.specificity - a.specificity)
        )
        setElementInfo({ tag: data.tag, text: data.text })
        setStatus('selected')
        break
      case 'etiquetta_picker_navigate':
        if (data.url && typeof data.url === 'string') {
          navigateToUrl(data.url)
        }
        break
      case 'etiquetta_picker_cancel':
        onOpenChange(false)
        break
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [onOpenChange, token])

  useEffect(() => {
    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [handleMessage])

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (readyTimeoutRef.current) clearTimeout(readyTimeoutRef.current)
    }
  }, [])

  function handleSelectSuggestion(suggestion: PickerSuggestion) {
    onSelect(suggestion)
    onOpenChange(false)
  }

  function handlePickAnother() {
    setSuggestions([])
    setElementInfo(null)
    setHoverInfo(null)
    setStatus('ready')
  }

  const statusColor =
    status === 'generating' || status === 'loading'
      ? 'bg-amber-500'
      : status === 'ready'
        ? 'bg-green-500'
        : 'bg-blue-500'

  const statusLabel =
    status === 'generating'
      ? 'Generating...'
      : status === 'loading'
        ? 'Loading page...'
        : status === 'ready'
          ? 'Ready — click an element'
          : 'Element selected'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex flex-col max-w-[100vw] max-h-[100vh] w-screen h-screen p-0 rounded-none border-0 gap-0 [&>button]:hidden">
        <DialogTitle className="sr-only">Element Picker</DialogTitle>
        {/* Top toolbar */}
        <div className="flex items-center gap-3 px-4 py-2 border-b bg-background shrink-0">
          <Crosshair className="h-4 w-4 text-primary shrink-0" />
          <span className="text-sm font-semibold shrink-0">Element Picker</span>
          <div className="flex-1 flex items-center gap-2 max-w-xl">
            <Input
              value={urlInput}
              onChange={(e) => setUrlInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') handleNavigate() }}
              placeholder="https://example.com"
              className="h-8 text-xs font-mono"
            />
            <Button size="sm" variant="outline" onClick={handleNavigate} disabled={!token} className="h-8 shrink-0">
              Go
            </Button>
          </div>
          <Button size="sm" variant="ghost" onClick={() => onOpenChange(false)} className="h-8 w-8 p-0 shrink-0">
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Main area */}
        <div className="flex flex-1 min-h-0">
          {/* Left: iframe */}
          <div className="w-[70%] relative bg-muted/30">
              {iframeUrl && (
                <iframe
                  ref={iframeRef}
                  src={iframeUrl}
                  onLoad={handleIframeLoad}
                  sandbox="allow-scripts allow-same-origin allow-forms"
                  className="w-full h-full border-0"
                  title="Element Picker Preview"
                />
              )}
              {/* Loading overlay */}
              {(status === 'generating' || status === 'loading') && (
                <div className="absolute inset-0 bg-background/80 flex items-center justify-center">
                  <div className="flex items-center gap-2">
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                    <span className="text-sm text-muted-foreground">
                      {status === 'generating' ? 'Generating preview token...' : 'Loading page...'}
                    </span>
                  </div>
                </div>
              )}
          </div>

          {/* Right: sidebar */}
          <div className="w-[30%] border-l h-full overflow-y-auto p-4 space-y-4">
              {/* Status */}
              <div className="flex items-center gap-2">
                <div className={`h-2 w-2 rounded-full ${statusColor} ${status === 'loading' || status === 'generating' ? 'animate-pulse' : ''}`} />
                <span className="text-xs font-medium">{statusLabel}</span>
              </div>

              {/* Warning */}
              {warning && (
                <div className="flex items-start gap-2 rounded-md bg-amber-500/10 border border-amber-500/20 p-3">
                  <AlertTriangle className="h-3.5 w-3.5 text-amber-500 mt-0.5 shrink-0" />
                  <div className="text-xs text-amber-700 dark:text-amber-400 space-y-1">
                    <p>The picker overlay may not have loaded correctly.</p>
                    <a
                      href={ensureProtocol(urlInput || domain)}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 underline"
                    >
                      Open in new tab <ExternalLink className="h-3 w-3" />
                    </a>
                  </div>
                </div>
              )}

              {/* Live hover preview */}
              {(status === 'ready' || status === 'selected') && hoverInfo && !elementInfo && (
                <div className="space-y-1.5">
                  <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
                    Hovering
                  </p>
                  <div className="rounded-md bg-muted/50 border border-border p-2.5 space-y-1">
                    <Badge variant="secondary" className="font-mono text-[10px]">
                      &lt;{hoverInfo.tag}&gt;
                    </Badge>
                    {hoverInfo.id && (
                      <p className="text-[10px] font-mono text-muted-foreground">#{hoverInfo.id}</p>
                    )}
                    {hoverInfo.classes && (
                      <p className="text-[10px] font-mono text-muted-foreground truncate">.{hoverInfo.classes.replace(/ /g, '.')}</p>
                    )}
                    {hoverInfo.text && (
                      <p className="text-[10px] text-muted-foreground truncate">
                        &quot;{hoverInfo.text.slice(0, 60)}&quot;
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* Selected element + suggestions */}
              {status === 'selected' && elementInfo && (
                <div className="space-y-3">
                  <div className="rounded-md bg-muted/50 border border-border p-3 space-y-1">
                    <div className="flex items-center gap-2">
                      <Badge variant="secondary" className="font-mono text-xs">
                        &lt;{elementInfo.tag}&gt;
                      </Badge>
                      {elementInfo.text && (
                        <span className="text-xs text-muted-foreground truncate">
                          &quot;{elementInfo.text.slice(0, 60)}{elementInfo.text.length > 60 ? '...' : ''}&quot;
                        </span>
                      )}
                    </div>
                  </div>

                  <div className="space-y-1.5">
                    <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
                      Selector Suggestions
                    </p>
                    {suggestions.map((suggestion, i) => (
                      <button
                        key={i}
                        type="button"
                        onClick={() => handleSelectSuggestion(suggestion)}
                        className="w-full flex items-center gap-2.5 rounded-md border border-border p-2.5 text-left hover:bg-muted/50 transition-colors group"
                      >
                        <div className="shrink-0 text-muted-foreground">
                          {SUGGESTION_ICONS[suggestion.type]}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="text-[11px] font-medium">{SUGGESTION_TYPE_LABELS[suggestion.type]}</span>
                            {i === 0 && (
                              <Badge className="text-[9px] px-1 py-0">best</Badge>
                            )}
                          </div>
                          <code className="text-[10px] font-mono text-muted-foreground truncate block">
                            {suggestion.selector}
                          </code>
                        </div>
                        <Check className="h-3.5 w-3.5 text-primary opacity-0 group-hover:opacity-100 transition-opacity shrink-0" />
                      </button>
                    ))}
                  </div>

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handlePickAnother}
                    className="w-full text-xs gap-1.5"
                  >
                    <RefreshCw className="h-3 w-3" />
                    Pick Another
                  </Button>
                </div>
              )}

              {/* Idle hint for ready state */}
              {status === 'ready' && !hoverInfo && (
                <p className="text-xs text-muted-foreground">
                  Hover over elements in the page to see details. Click to select.
                </p>
              )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
