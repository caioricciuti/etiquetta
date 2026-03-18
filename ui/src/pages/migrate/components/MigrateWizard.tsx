import { useState } from 'react'
import type { MigrateSource, MigrateAnalysis } from '../../../lib/types'
import { StepSourceSelect } from './StepSourceSelect'
import { StepInstructions } from './StepInstructions'
import { StepUpload } from './StepUpload'
import { StepDomainSelect } from './StepDomainSelect'
import { StepColumnMapping } from './StepColumnMapping'
import { StepPreview } from './StepPreview'
import { StepImport } from './StepImport'
import { StepDone } from './StepDone'
import { GtmConverter } from './GtmConverter'

type WizardStep = 'source' | 'instructions' | 'upload' | 'domain' | 'mapping' | 'preview' | 'import' | 'done'

interface MigrateWizardProps {
  onComplete: () => void
}

export function MigrateWizard({ onComplete }: MigrateWizardProps) {
  const [step, setStep] = useState<WizardStep>('source')
  const [source, setSource] = useState<MigrateSource | null>(null)
  const [domain, setDomain] = useState<string | null>(null)
  const [analysis, setAnalysis] = useState<MigrateAnalysis | null>(null)
  const [columnMapping, setColumnMapping] = useState<Record<string, string>>({})
  const [jobId, setJobId] = useState<string | null>(null)

  const reset = () => {
    setStep('source')
    setSource(null)
    setDomain(null)
    setAnalysis(null)
    setColumnMapping({})
    setJobId(null)
  }

  // GTM is a separate flow
  if (source === 'gtm') {
    return <GtmConverter onBack={reset} />
  }

  switch (step) {
    case 'source':
      return <StepSourceSelect onSelect={(s) => { setSource(s); setStep('instructions') }} />
    case 'instructions':
      return <StepInstructions source={source!} onNext={() => setStep('upload')} onBack={() => setStep('source')} />
    case 'upload':
      return <StepUpload source={source!} onAnalyzed={(_f, a) => { setAnalysis(a); setStep('domain') }} onBack={() => setStep('instructions')} />
    case 'domain':
      return <StepDomainSelect onSelect={(d) => { setDomain(d); setStep(source === 'csv' ? 'mapping' : 'preview') }} onBack={() => setStep('upload')} />
    case 'mapping':
      return <StepColumnMapping analysis={analysis!} onConfirm={(m) => { setColumnMapping(m); setStep('preview') }} onBack={() => setStep('domain')} />
    case 'preview':
      return <StepPreview analysis={analysis!} mapping={columnMapping} onConfirm={() => setStep('import')} onBack={() => setStep(source === 'csv' ? 'mapping' : 'domain')} />
    case 'import':
      return <StepImport analysisId={analysis!.analysis_id} source={source!} domain={domain!} mapping={columnMapping} onJobStarted={setJobId} onComplete={() => setStep('done')} />
    case 'done':
      return <StepDone jobId={jobId} onNewImport={reset} onViewHistory={onComplete} />
    default:
      return null
  }
}
