import { useEffect, useCallback, useRef, useState } from 'react'
import { usePreviewToken } from '@/hooks/useTagManager'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from '@/components/ui/dialog'
import { Crosshair, Loader2, X } from 'lucide-react'
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

function ensureProtocol(domain: string): string {
  if (domain.startsWith('http://') || domain.startsWith('https://')) return domain
  return `https://${domain}`
}

export function ElementPicker({ open, onOpenChange, containerId, domain, onSelect }: ElementPickerProps) {
  const previewToken = usePreviewToken(containerId)
  const [iframeUrl, setIframeUrl] = useState<string | null>(null)
  const [urlInput, setUrlInput] = useState('')
  const [token, setToken] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const iframeRef = useRef<HTMLIFrameElement>(null)

  // Generate token and set initial URL when dialog opens
  useEffect(() => {
    if (!open) return
    setLoading(true)
    setIframeUrl(null)
    setToken(null)

    const initialUrl = ensureProtocol(domain)
    setUrlInput(initialUrl)

    previewToken.mutate(undefined, {
      onSuccess: (data) => {
        const t = data.token
        setToken(t)
        setIframeUrl(`/api/tagmanager/pick-proxy?url=${encodeURIComponent(initialUrl)}&token=${encodeURIComponent(t)}`)
      },
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, containerId, domain])

  function navigateToUrl(url: string) {
    if (!token) return
    setUrlInput(url)
    setIframeUrl(`/api/tagmanager/pick-proxy?url=${encodeURIComponent(url)}&token=${encodeURIComponent(token)}`)
    setLoading(true)
  }

  function handleNavigate() {
    if (!urlInput.trim()) return
    navigateToUrl(ensureProtocol(urlInput.trim()))
  }

  // Listen for postMessage from iframe
  const handleMessage = useCallback((event: MessageEvent) => {
    const data = event.data
    if (!data || typeof data !== 'object') return

    switch (data.type) {
      case 'etiquetta_picker_ready':
        setLoading(false)
        break
      case 'etiquetta_picker_select':
        if (data.suggestion) {
          onSelect(data.suggestion as PickerSuggestion)
          onOpenChange(false)
        }
        break
      case 'etiquetta_picker_cancel':
        onOpenChange(false)
        break
    }
  }, [onSelect, onOpenChange])

  useEffect(() => {
    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [handleMessage])

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

        {/* iframe */}
        <div className="flex-1 relative bg-muted/30">
          {iframeUrl && (
            <iframe
              ref={iframeRef}
              src={iframeUrl}
              onLoad={() => setLoading(false)}
              sandbox="allow-scripts allow-same-origin allow-forms"
              className="w-full h-full border-0"
              title="Element Picker Preview"
            />
          )}
          {loading && (
            <div className="absolute inset-0 bg-background/80 flex items-center justify-center">
              <div className="flex items-center gap-2">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                <span className="text-sm text-muted-foreground">Loading page...</span>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
