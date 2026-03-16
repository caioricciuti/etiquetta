import { useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useContainer, useExportContainer, useImportContainer } from '@/hooks/useTagManager'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { TagList } from './components/TagList'
import { TriggerList } from './components/TriggerList'
import { VariableList } from './components/VariableList'
import { PrivacyAudit } from './components/PrivacyAudit'
import { PublishBar } from './components/PublishBar'
import { ArrowLeft, Loader2, Code, Zap, Variable, ShieldCheck, Download, Upload } from 'lucide-react'

export function TagManagerContainer() {
  const { containerId } = useParams<{ containerId: string }>()
  const navigate = useNavigate()
  const { data: container, isLoading, error } = useContainer(containerId ?? '')
  const exportContainer = useExportContainer(containerId)
  const importContainer = useImportContainer(containerId)
  const fileInputRef = useRef<HTMLInputElement>(null)

  if (!containerId) {
    navigate('/tag-manager')
    return null
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !container) {
    return (
      <div className="p-6 max-w-4xl mx-auto text-center py-24">
        <p className="text-muted-foreground">Container not found.</p>
        <Button variant="outline" className="mt-4" onClick={() => navigate('/tag-manager')}>
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back to Tag Manager
        </Button>
      </div>
    )
  }

  function handleImportFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const data = JSON.parse(ev.target?.result as string)
        if (!confirm('This will replace all tags, triggers, and variables in this container. Continue?')) return
        importContainer.mutate(data)
      } catch {
        alert('Invalid JSON file')
      }
    }
    reader.readAsText(file)
    // Reset so the same file can be re-imported
    e.target.value = ''
  }

  return (
    <div className="flex flex-col h-full">
      <PublishBar container={container} />

      <div className="p-6 max-w-6xl mx-auto w-full flex-1 space-y-6">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={() => navigate('/tag-manager')}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="flex-1">
            <h1 className="text-xl font-bold tracking-tight">{container.name}</h1>
            <p className="text-sm text-muted-foreground">
              {container.domain_name ?? container.domain ?? 'Unknown domain'}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <input
              type="file"
              ref={fileInputRef}
              accept=".json"
              onChange={handleImportFile}
              className="hidden"
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => fileInputRef.current?.click()}
              disabled={importContainer.isPending}
            >
              {importContainer.isPending ? (
                <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
              ) : (
                <Upload className="h-4 w-4 mr-1.5" />
              )}
              Import
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => exportContainer.mutate()}
              disabled={exportContainer.isPending}
            >
              {exportContainer.isPending ? (
                <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
              ) : (
                <Download className="h-4 w-4 mr-1.5" />
              )}
              Export
            </Button>
          </div>
        </div>

        <Tabs defaultValue="tags" className="w-full">
          <TabsList>
            <TabsTrigger value="tags" className="gap-1.5">
              <Code className="h-4 w-4" />
              Tags
            </TabsTrigger>
            <TabsTrigger value="triggers" className="gap-1.5">
              <Zap className="h-4 w-4" />
              Triggers
            </TabsTrigger>
            <TabsTrigger value="variables" className="gap-1.5">
              <Variable className="h-4 w-4" />
              Variables
            </TabsTrigger>
            <TabsTrigger value="audit" className="gap-1.5">
              <ShieldCheck className="h-4 w-4" />
              Audit
            </TabsTrigger>
          </TabsList>

          <TabsContent value="tags" className="mt-4">
            <TagList containerId={containerId} />
          </TabsContent>

          <TabsContent value="triggers" className="mt-4">
            <TriggerList containerId={containerId} />
          </TabsContent>

          <TabsContent value="variables" className="mt-4">
            <VariableList containerId={containerId} />
          </TabsContent>

          <TabsContent value="audit" className="mt-4">
            <PrivacyAudit containerId={containerId} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
