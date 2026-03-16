import { useTags } from '@/hooks/useTagManager'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { getTemplate } from './tag-templates'
import type { TagPrivacyMeta } from './tag-templates'
import { Shield, AlertTriangle, Globe, Cookie, ExternalLink } from 'lucide-react'

interface PrivacyAuditProps {
  containerId: string
}

function getRiskColor(risk: TagPrivacyMeta['privacyRisk']) {
  switch (risk) {
    case 'low':
      return 'bg-green-500/10 text-green-700 border-green-200'
    case 'medium':
      return 'bg-yellow-500/10 text-yellow-700 border-yellow-200'
    case 'high':
      return 'bg-red-500/10 text-red-700 border-red-200'
  }
}

function getRiskBadgeVariant(risk: TagPrivacyMeta['privacyRisk']) {
  switch (risk) {
    case 'low':
      return 'outline' as const
    case 'medium':
      return 'secondary' as const
    case 'high':
      return 'destructive' as const
  }
}

export function PrivacyAudit({ containerId }: PrivacyAuditProps) {
  const { data: tags } = useTags(containerId)

  const enabledTags = tags?.filter((t) => t.is_enabled) ?? []

  // Compute aggregate stats
  const allDomains = new Set<string>()
  let cookieCount = 0
  let highRiskCount = 0
  let mediumRiskCount = 0
  let lowRiskCount = 0
  let noConsentCount = 0

  const tagAudits = enabledTags.map((tag) => {
    const template = getTemplate(tag.tag_type)
    const privacy = template?.privacy ?? {
      externalDomains: [],
      setsCookies: false,
      privacyRisk: 'medium' as const,
      privacyNote: 'Unknown tag type — cannot assess privacy impact.',
    }

    privacy.externalDomains.forEach((d) => allDomains.add(d))
    if (privacy.setsCookies) cookieCount++
    if (privacy.privacyRisk === 'high') highRiskCount++
    else if (privacy.privacyRisk === 'medium') mediumRiskCount++
    else lowRiskCount++
    if (tag.consent_category === 'necessary') noConsentCount++

    return { tag, privacy, template }
  })

  return (
    <div className="space-y-6">
      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1.5">
              <Shield className="h-4 w-4" />
              Total Tags
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{enabledTags.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1.5">
              <Globe className="h-4 w-4" />
              External Domains
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{allDomains.size}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1.5">
              <Cookie className="h-4 w-4" />
              Set Cookies
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{cookieCount}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1.5">
              <AlertTriangle className="h-4 w-4" />
              High Risk
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-600">{highRiskCount}</div>
          </CardContent>
        </Card>
      </div>

      {/* Aggregate summary */}
      {enabledTags.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Summary</CardTitle>
          </CardHeader>
          <CardContent className="text-sm space-y-1.5">
            <p>
              {enabledTags.length} active tag{enabledTags.length !== 1 ? 's' : ''} load scripts from{' '}
              <strong>{allDomains.size}</strong> external domain{allDomains.size !== 1 ? 's' : ''}.
            </p>
            {allDomains.size > 0 && (
              <div className="flex flex-wrap gap-1.5 mt-2">
                {[...allDomains].map((d) => (
                  <Badge key={d} variant="outline" className="text-xs font-mono">
                    <ExternalLink className="h-3 w-3 mr-1" />
                    {d}
                  </Badge>
                ))}
              </div>
            )}
            <div className="flex gap-3 mt-2 text-xs text-muted-foreground">
              <span className="text-green-600">{lowRiskCount} low risk</span>
              <span className="text-yellow-600">{mediumRiskCount} medium risk</span>
              <span className="text-red-600">{highRiskCount} high risk</span>
            </div>
            {noConsentCount > 0 && (
              <p className="text-yellow-600 text-xs mt-1">
                {noConsentCount} tag{noConsentCount !== 1 ? 's' : ''} marked as "necessary" — will fire without consent.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {/* Per-tag audit table */}
      {tagAudits.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            No active tags to audit. Add and enable tags to see privacy analysis.
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="text-left font-medium px-4 py-2.5">Tag</th>
                <th className="text-left font-medium px-4 py-2.5">Type</th>
                <th className="text-left font-medium px-4 py-2.5">Risk</th>
                <th className="text-left font-medium px-4 py-2.5">Consent</th>
                <th className="text-left font-medium px-4 py-2.5">Cookies</th>
                <th className="text-left font-medium px-4 py-2.5">External Domains</th>
              </tr>
            </thead>
            <tbody>
              {tagAudits.map(({ tag, privacy, template }) => (
                <tr key={tag.id} className={`border-b last:border-0 ${getRiskColor(privacy.privacyRisk)}`}>
                  <td className="px-4 py-2.5 font-medium">{tag.name}</td>
                  <td className="px-4 py-2.5 text-muted-foreground">{template?.name ?? tag.tag_type}</td>
                  <td className="px-4 py-2.5">
                    <Badge variant={getRiskBadgeVariant(privacy.privacyRisk)} className="text-xs">
                      {privacy.privacyRisk}
                    </Badge>
                  </td>
                  <td className="px-4 py-2.5">
                    <Badge variant="outline" className="text-xs">
                      {tag.consent_category}
                    </Badge>
                  </td>
                  <td className="px-4 py-2.5">{privacy.setsCookies ? 'Yes' : 'No'}</td>
                  <td className="px-4 py-2.5 text-xs font-mono">
                    {privacy.externalDomains.length > 0
                      ? privacy.externalDomains.join(', ')
                      : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Detailed notes */}
      {tagAudits.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Privacy Notes</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {tagAudits.map(({ tag, privacy }) => (
              <div key={tag.id} className="text-sm">
                <span className="font-medium">{tag.name}:</span>{' '}
                <span className="text-muted-foreground">{privacy.privacyNote}</span>
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
