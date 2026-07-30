import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Sparkles, Check, ArrowRight } from 'lucide-react'
import { useLicense } from '../hooks/useLicenseQuery'
import { Button } from './ui/button'

interface FeatureInfo {
  name: string
  tagline: string
  perks: string[]
}

// Per-feature copy for the upgrade screen. Keyed by the same feature flags the
// backend gates on (see internal/licensing).
const FEATURE_INFO: Record<string, FeatureInfo> = {
  funnels: {
    name: 'Funnels, Goals & Retention',
    tagline: 'See where visitors drop off, track conversions, and measure how users come back over time.',
    perks: ['Multi-step conversion funnels', 'Goals with revenue attribution', 'Cohort retention grids'],
  },
  alerts: {
    name: 'Alerts',
    tagline: 'Get notified the moment your metrics cross a threshold — by email, Slack, or webhook.',
    perks: ['Traffic, error-rate & bot-rate alerts', 'Email, Slack & webhook delivery', 'Per-alert cooldowns'],
  },
  error_tracking: {
    name: 'Error Tracking',
    tagline: 'Capture client-side JavaScript errors with stack traces and triage them in one place.',
    perks: ['Automatic error capture', 'Stack traces & source context', 'Frequency and impact ranking'],
  },
  session_replay: {
    name: 'Session Replay',
    tagline: 'Watch real user sessions to understand behavior, reproduce bugs, and spot friction.',
    perks: ['Privacy-first session recording', 'Filter by device, browser & OS', 'Configurable retention'],
  },
  consent: {
    name: 'Consent Management',
    tagline: 'Ship configurable consent banners and keep an auditable record of every choice.',
    perks: ['Customizable consent banner', 'Per-category controls', 'Consent audit trail'],
  },
  ad_fraud: {
    name: 'Ad Fraud Detection',
    tagline: 'Detect and filter fraudulent ad traffic before it pollutes your analytics.',
    perks: ['Datacenter & bot detection', 'Suspicious-traffic scoring', 'Campaign-level fraud reports'],
  },
  tag_manager: {
    name: 'Tag Manager',
    tagline: 'Deploy tags, pixels, and scripts without touching your site’s code.',
    perks: ['Visual tag & trigger builder', 'Version snapshots', 'No redeploys required'],
  },
  sso: {
    name: 'Single Sign-On',
    tagline: 'Let your team sign in through your identity provider with OIDC.',
    perks: ['OIDC single sign-on', 'Domain-restricted access', 'Automatic user provisioning'],
  },
  multi_user: {
    name: 'Team Members',
    tagline: 'Invite teammates and control what each of them can see and change.',
    perks: ['Unlimited team members', 'Role-based access', 'Per-domain permissions'],
  },
}

const DEFAULT_INFO: FeatureInfo = {
  name: 'This feature',
  tagline: 'This feature is part of a paid Etiquetta plan.',
  perks: [],
}

function ProFeaturePage({ feature, tier }: { feature: string; tier: string }) {
  const info = FEATURE_INFO[feature] ?? DEFAULT_INFO
  const needed = feature === 'sso' ? 'Enterprise' : 'Pro'

  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="relative w-full max-w-lg overflow-hidden rounded-2xl border bg-card p-8 text-center shadow-sm">
        {/* subtle brand glow */}
        <div
          className="pointer-events-none absolute inset-x-0 -top-24 h-48 opacity-30 blur-3xl"
          style={{ background: 'radial-gradient(closest-side, hsl(262 83% 58%), transparent)' }}
          aria-hidden
        />

        <div className="relative">
          <div className="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-blue-500 to-violet-600 text-white shadow-lg">
            <Sparkles className="h-7 w-7" />
          </div>

          <div className="mb-1 inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-semibold text-amber-800 dark:bg-amber-900/30 dark:text-amber-400">
            {needed} feature
          </div>

          <h1 className="mt-3 text-2xl font-bold">{info.name}</h1>
          <p className="mx-auto mt-2 max-w-md text-muted-foreground">{info.tagline}</p>

          {info.perks.length > 0 && (
            <ul className="mx-auto mt-6 max-w-xs space-y-2 text-left">
              {info.perks.map((perk) => (
                <li key={perk} className="flex items-start gap-2.5 text-sm">
                  <span className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
                    <Check className="h-3 w-3" />
                  </span>
                  <span>{perk}</span>
                </li>
              ))}
            </ul>
          )}

          <div className="mt-8 flex flex-col items-center gap-3">
            <Button asChild className="w-full max-w-xs">
              <a href="https://etiquetta.com/pricing" target="_blank" rel="noopener noreferrer">
                Upgrade to {needed}
                <ArrowRight className="h-4 w-4" />
              </a>
            </Button>
            <Link
              to="/settings/license"
              className="text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
            >
              Already have a license? Add it in settings
            </Link>
          </div>

          <p className="mt-6 text-xs text-muted-foreground">
            You’re on the <span className="font-medium capitalize">{tier}</span> plan.
          </p>
        </div>
      </div>
    </div>
  )
}

/**
 * Wraps a route so users without the required license feature see an upgrade
 * screen instead of the (non-functional) page. While the license is still
 * loading, nothing is rendered to avoid flashing the gate on a licensed user.
 */
export function ProGate({ feature, children }: { feature: string; children: ReactNode }) {
  const { hasFeature, isLoading, license } = useLicense()

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
      </div>
    )
  }

  if (hasFeature(feature)) {
    return <>{children}</>
  }

  return <ProFeaturePage feature={feature} tier={license.tier} />
}
