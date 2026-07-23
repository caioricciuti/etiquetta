import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { fetchAPI } from '../lib/api'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Loader2,
  AlertCircle,
  Mail,
  Lock,
  Zap,
  Shield,
  BarChart3,
  Activity,
  Eye,
  EyeOff,
} from 'lucide-react'

const emailRegex = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/

type Mode = 'login' | 'onboarding'

export function Login() {
  const [mode, setMode] = useState<Mode>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [ssoProviders, setSsoProviders] = useState<Array<{ id: string; name: string }>>([])
  const { login, refresh } = useAuth()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  useEffect(() => {
    if (searchParams.get('onboarding') === 'true') {
      setMode('onboarding')
    }
  }, [searchParams])

  useEffect(() => {
    fetchAPI<Array<{ id: string; name: string }>>('/api/auth/sso')
      .then(providers => setSsoProviders(providers ?? []))
      .catch(() => {})
  }, [])

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      await login(email, password)
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  const handleOnboarding = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    if (!email || !emailRegex.test(email)) {
      setError('Please enter a valid email address')
      return
    }

    if (password.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }

    if (password !== confirmPassword) {
      setError('Passwords do not match')
      return
    }

    setLoading(true)

    try {
      const response = await fetch('/api/auth/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ email, password }),
      })

      if (!response.ok) {
        const data = await response.json()
        throw new Error(data.error || 'Setup failed')
      }

      await refresh()
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex bg-background">
      {/* Left Panel - Form */}
      <div className="w-full lg:w-[45%] flex flex-col p-8 lg:p-12">
        {/* Logo - Top */}
        <div className="flex items-center gap-3">
          <img src="/favicon-pixellated.png" alt="Etiquetta" className="h-10 w-10" />
          <div>
            <h1 className="text-lg font-semibold text-foreground">Etiquetta</h1>
            <p className="text-xs text-muted-foreground">Web Analytics</p>
          </div>
        </div>

        {/* Form - Centered */}
        <div className="flex-1 flex items-center justify-center">
          <div className="w-full max-w-sm">
            {mode === 'login' && (
              <>
                <div className="mb-8">
                  <h2 className="text-2xl font-semibold tracking-tight text-foreground">
                    Welcome back
                  </h2>
                  <p className="text-muted-foreground mt-1">
                    Sign in to access your analytics dashboard
                  </p>
                </div>

                <form onSubmit={handleLogin} className="space-y-5">
                  {error && (
                    <div className="flex items-center gap-2 text-sm text-destructive bg-destructive/10 border border-destructive/20 p-3 rounded-lg">
                      <AlertCircle className="h-4 w-4 shrink-0" />
                      <span>{error}</span>
                    </div>
                  )}

                  <div className="space-y-2">
                    <Label htmlFor="email" className="text-sm text-foreground">
                      Email address
                    </Label>
                    <div className="relative">
                      <Mail className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                      <Input
                        id="email"
                        type="email"
                        value={email}
                        onChange={(e) => setEmail(e.target.value)}
                        placeholder="admin@example.com"
                        required
                        autoFocus
                        autoComplete="email"
                        className="pl-10 h-11"
                      />
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="password" className="text-sm text-foreground">
                      Password
                    </Label>
                    <div className="relative">
                      <Lock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                      <Input
                        id="password"
                        type={showPassword ? 'text' : 'password'}
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        placeholder="Enter your password"
                        required
                        autoComplete="current-password"
                        className="pl-10 pr-10 h-11"
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                      </button>
                    </div>
                  </div>

                  <Button type="submit" className="w-full h-11" disabled={loading}>
                    {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Sign in
                  </Button>
                </form>

                {ssoProviders.length > 0 && (
                  <div className="mt-6 space-y-3">
                    <div className="relative">
                      <div className="absolute inset-0 flex items-center">
                        <span className="w-full border-t border-border" />
                      </div>
                      <div className="relative flex justify-center text-xs uppercase">
                        <span className="bg-background px-2 text-muted-foreground">Or continue with</span>
                      </div>
                    </div>
                    <div className="space-y-2">
                      {ssoProviders.map(p => (
                        <Button
                          key={p.id}
                          variant="outline"
                          className="w-full h-11"
                          onClick={() => {
                            window.location.href = `/api/auth/sso/${p.id}/login`
                          }}
                        >
                          <Shield className="h-4 w-4 mr-2" />
                          Continue with {p.name}
                        </Button>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}

            {mode === 'onboarding' && (
              <>
                <div className="mb-8">
                  <h2 className="text-2xl font-semibold tracking-tight text-foreground">
                    Welcome to Etiquetta
                  </h2>
                  <p className="text-muted-foreground mt-1">
                    Set up your admin account to get started
                  </p>
                </div>

                <form onSubmit={handleOnboarding} className="space-y-5">
                  {error && (
                    <div className="flex items-center gap-2 text-sm text-destructive bg-destructive/10 border border-destructive/20 p-3 rounded-lg">
                      <AlertCircle className="h-4 w-4 shrink-0" />
                      <span>{error}</span>
                    </div>
                  )}

                  <div className="space-y-2">
                    <Label htmlFor="onboarding-email" className="text-sm text-foreground">
                      Email address
                    </Label>
                    <div className="relative">
                      <Mail className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                      <Input
                        id="onboarding-email"
                        type="email"
                        value={email}
                        onChange={(e) => setEmail(e.target.value)}
                        placeholder="admin@example.com"
                        required
                        autoFocus
                        autoComplete="email"
                        className="pl-10 h-11"
                      />
                    </div>
                    <p className="text-xs text-muted-foreground">Your email will be used to sign in</p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="onboarding-password" className="text-sm text-foreground">
                      Password
                    </Label>
                    <div className="relative">
                      <Lock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                      <Input
                        id="onboarding-password"
                        type={showPassword ? 'text' : 'password'}
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        placeholder="Enter a secure password"
                        required
                        autoComplete="new-password"
                        minLength={8}
                        className="pl-10 pr-10 h-11"
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                      </button>
                    </div>
                    <p className="text-xs text-muted-foreground">Minimum 8 characters</p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="onboarding-confirm" className="text-sm text-foreground">
                      Confirm password
                    </Label>
                    <div className="relative">
                      <Lock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                      <Input
                        id="onboarding-confirm"
                        type={showConfirmPassword ? 'text' : 'password'}
                        value={confirmPassword}
                        onChange={(e) => setConfirmPassword(e.target.value)}
                        placeholder="Confirm your password"
                        required
                        autoComplete="new-password"
                        className="pl-10 pr-10 h-11"
                      />
                      <button
                        type="button"
                        onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                      </button>
                    </div>
                  </div>

                  <Button type="submit" className="w-full h-11" disabled={loading}>
                    {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Create Account
                  </Button>
                </form>
              </>
            )}
          </div>
        </div>

        {/* Footer - Bottom */}
        <p className="text-xs text-muted-foreground text-center">
          Self-hosted. Privacy-focused. Your data stays yours.
        </p>
      </div>

      {/* Right Panel - Hero */}
      <div className="hidden lg:flex lg:w-[55%] bg-card relative overflow-hidden flex-col border-l border-border">
        {/* Gradient Orbs */}
        <div className="absolute top-1/4 right-1/4 w-96 h-96 bg-primary/20 rounded-full blur-[120px]" />
        <div className="absolute bottom-1/3 left-1/4 w-72 h-72 bg-blue-500/15 rounded-full blur-[100px]" />

        {/* Content - Centered */}
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center px-12 xl:px-16">
          <div className="max-w-lg">
            {/* Logo */}
            <div className="flex justify-center mb-8">
              <img src="/logo-complete.png" alt="Etiquetta" className="h-12" />
            </div>

            {/* Headlines */}
            <div className="text-center mb-6">
              <h2 className="text-3xl xl:text-4xl font-bold tracking-tight leading-tight text-foreground">
                Own Your Data.
              </h2>
              <h2 className="text-3xl xl:text-4xl font-bold tracking-tight leading-tight">
                <span className="bg-gradient-to-r from-blue-500 to-purple-500 bg-clip-text text-transparent">
                  Know Your Users.
                </span>
              </h2>
            </div>

            <p className="text-muted-foreground text-base xl:text-lg mb-10 leading-relaxed text-center">
              Self-hosted web analytics built for teams that value privacy and want to understand their users without compromising their data.
            </p>

            {/* Feature Cards */}
            <div className="grid grid-cols-2 gap-3 mb-10">
              <div className="p-4 rounded-xl bg-secondary/50">
                <Shield className="h-5 w-5 text-primary mb-2" />
                <h3 className="text-sm font-semibold text-foreground">Privacy-First</h3>
                <p className="text-xs text-muted-foreground mt-0.5">Self-hosted with configurable privacy controls</p>
              </div>
              <div className="p-4 rounded-xl bg-secondary/50">
                <Activity className="h-5 w-5 text-primary mb-2" />
                <h3 className="text-sm font-semibold text-foreground">Core Web Vitals</h3>
                <p className="text-xs text-muted-foreground mt-0.5">LCP, FCP, CLS, INP</p>
              </div>
              <div className="p-4 rounded-xl bg-secondary/50">
                <BarChart3 className="h-5 w-5 text-primary mb-2" />
                <h3 className="text-sm font-semibold text-foreground">Real-time</h3>
                <p className="text-xs text-muted-foreground mt-0.5">Live analytics dashboard</p>
              </div>
              <div className="p-4 rounded-xl bg-secondary/50">
                <Zap className="h-5 w-5 text-primary mb-2" />
                <h3 className="text-sm font-semibold text-foreground">Lightweight</h3>
                <p className="text-xs text-muted-foreground mt-0.5">Single binary deploy</p>
              </div>
            </div>

            {/* Stats */}
            <div className="pt-6 border-t border-border">
              <p className="text-xs text-muted-foreground uppercase tracking-wider mb-4 text-center">
                Built for teams who value
              </p>
              <div className="flex items-center justify-center gap-10">
                <div className="text-center">
                  <p className="text-2xl font-bold text-foreground">100%</p>
                  <p className="text-xs text-muted-foreground">Data Ownership</p>
                </div>
                <div className="text-center">
                  <p className="text-2xl font-bold text-foreground">0</p>
                  <p className="text-xs text-muted-foreground">Third Parties</p>
                </div>
                <div className="text-center">
                  <p className="text-2xl font-bold text-foreground">&lt;1KB</p>
                  <p className="text-xs text-muted-foreground">Script Size</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="relative z-10 px-12 xl:px-16 pb-8">
          <div className="flex items-center justify-center text-xs text-muted-foreground">
            <span>Etiquetta v0.1.0</span>
          </div>
        </div>
      </div>
    </div>
  )
}
