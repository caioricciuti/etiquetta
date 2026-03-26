import { Link } from 'react-router-dom'
import { AlertTriangle, ArrowRight } from 'lucide-react'
import { useErrors } from '../../hooks/useAnalyticsQueries'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { FeatureGate, FeatureBadge } from '../FeatureGate'
import { useLicense } from '../../hooks/useLicenseQuery'

export function ErrorsCard() {
  const { hasFeature } = useLicense()
  const { data } = useErrors(hasFeature('error_tracking'))
  const errors = data ?? []

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-lg font-semibold">Errors</CardTitle>
            <FeatureBadge feature="error_tracking" />
          </div>
          {errors.length > 0 && (
            <Link to="/errors" className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors">
              View all <ArrowRight className="h-3 w-3" />
            </Link>
          )}
        </div>
        <CardDescription>JavaScript errors from your users</CardDescription>
      </CardHeader>
      <CardContent>
        <FeatureGate feature="error_tracking">
          {errors.length > 0 ? (
            <div className="space-y-3">
              {errors.slice(0, 4).map((err, i) => (
                <div key={i} className="flex items-start gap-3 p-3 rounded-lg bg-destructive/5 border border-destructive/10">
                  <AlertTriangle className="h-4 w-4 text-destructive mt-0.5 shrink-0" />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">
                      {err.error_message}
                    </p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      {err.occurrences} occurrences
                    </p>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm text-center py-4">No errors recorded</p>
          )}
        </FeatureGate>
      </CardContent>
    </Card>
  )
}
