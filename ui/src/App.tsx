import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, Link } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { fetchAPI } from './lib/api'
import { queryClient } from './lib/query-client'
import { AuthProvider, useAuth } from './hooks/useAuth'
import { ThemeProvider, useTheme } from './components/theme/theme-provider'
import { Dashboard } from './components/dashboard/Dashboard'
import { LicenseSettings } from './components/LicenseSettings'
import {
  DomainsSettings,
  EmailSettings,
  GeoIPSettings,
  AccountSettings,
  UsersSettings,
  TrackingSettings,
  ConnectionsSettings,
} from './pages/settings'
import { ConsentDashboard, ConsentConfig } from './pages/consent'
import { PrivacyCenter } from './pages/privacy'
import { TagManager, TagManagerContainer } from './pages/tag-manager'
import { Explorer } from './pages/Explorer'
import { Login } from './pages/Login'
import { BotAnalysis } from './pages/BotAnalysis'
import { Compare } from './pages/Compare'
import { Connections } from './pages/Connections'
import { AdFraud } from './pages/AdFraud'
import { DomainPicker } from './components/DomainPicker'
import { FeatureBadge } from './components/FeatureGate'
import {
  BarChart3,
  Settings as SettingsIcon,
  Key,
  LogOut,
  Moon,
  Sun,
  Monitor,
  Bot,
  GitCompareArrows,
  ShieldAlert,
  Shield,
  Tags,
  Users as UsersIcon,
  ChevronsUpDown,
  ChevronRight,
  Globe,
  Mail,
  MapPin,
  User,
  Database,
  Fingerprint,
  Activity,
  Cable,
} from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from './components/ui/dropdown-menu'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from './components/ui/collapsible'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
  useSidebar,
} from './components/ui/sidebar'
import './index.css'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, setupRequired } = useAuth()
  const location = useLocation()

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    )
  }

  if (setupRequired) {
    return <Navigate to="/login?onboarding=true" replace />
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}

function PublicRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, setupRequired } = useAuth()

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    )
  }

  if (isAuthenticated && !setupRequired) {
    return <Navigate to="/" replace />
  }

  return <>{children}</>
}

function ThemeSelector() {
  const { theme, setTheme } = useTheme()
  const { state } = useSidebar()
  const isCollapsed = state === 'collapsed'

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <SidebarMenuButton
          size="sm"
          className="w-full"
          tooltip="Theme"
        >
          {theme === 'dark' ? (
            <Moon className="h-4 w-4" />
          ) : theme === 'light' ? (
            <Sun className="h-4 w-4" />
          ) : (
            <Monitor className="h-4 w-4" />
          )}
          {!isCollapsed && <span className="capitalize">{theme}</span>}
        </SidebarMenuButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="end" className="w-40">
        <DropdownMenuItem onClick={() => setTheme('light')}>
          <Sun className="mr-2 h-4 w-4" />
          Light
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setTheme('dark')}>
          <Moon className="mr-2 h-4 w-4" />
          Dark
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setTheme('system')}>
          <Monitor className="mr-2 h-4 w-4" />
          System
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UserMenu() {
  const { user, logout } = useAuth()
  const { state } = useSidebar()
  const isCollapsed = state === 'collapsed'

  const handleLogout = async () => {
    await logout()
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <SidebarMenuButton
          size="lg"
          className="w-full"
          tooltip={user?.email}
        >
          <div className="h-8 w-8 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shrink-0">
            <span className="text-xs font-bold text-white">
              {(user?.name || user?.email || 'U').charAt(0).toUpperCase()}
            </span>
          </div>
          {!isCollapsed && (
            <>
              <div className="flex flex-col items-start min-w-0 flex-1">
                <span className="text-sm font-medium truncate w-full">
                  {user?.name || user?.email}
                </span>
                <span className="text-xs text-muted-foreground capitalize">
                  {user?.role}
                </span>
              </div>
              <ChevronsUpDown className="h-4 w-4 text-muted-foreground" />
            </>
          )}
        </SidebarMenuButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="end" className="w-56">
        <div className="px-2 py-1.5">
          <p className="text-sm font-medium">{user?.name || user?.email}</p>
          <p className="text-xs text-muted-foreground">{user?.email}</p>
        </div>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={handleLogout} className="text-destructive focus:text-destructive">
          <LogOut className="mr-2 h-4 w-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function AppSidebar() {
  const location = useLocation()
  const { state } = useSidebar()
  const { isAdmin } = useAuth()
  const isCollapsed = state === 'collapsed'
  const [version, setVersion] = useState<string>('...')
  const [settingsOpen, setSettingsOpen] = useState(location.pathname.startsWith('/settings'))

  useEffect(() => {
    fetchAPI<{ version: string }>('/api/version')
      .then(data => setVersion(data.version))
      .catch(() => setVersion('dev'))
  }, [])

  // Auto-expand settings when navigating to a settings page
  useEffect(() => {
    if (location.pathname.startsWith('/settings')) {
      setSettingsOpen(true)
    }
  }, [location.pathname])

  const navigation = [
    { path: '/', name: 'Dashboard', icon: BarChart3 },
    { path: '/compare', name: 'Compare', icon: GitCompareArrows },
    { path: '/bots', name: 'Bot Analysis', icon: Bot },
    { path: '/consent', name: 'Consent', icon: Shield, pro: 'consent' },
    { path: '/connections', name: 'Connections', icon: Cable, pro: 'connections' },
    { path: '/fraud', name: 'Ad Fraud', icon: ShieldAlert, pro: 'ad_fraud' },
    { path: '/privacy', name: 'Privacy Center', icon: Fingerprint, adminOnly: true },
    { path: '/explorer', name: 'Data Explorer', icon: Database, adminOnly: true },
    { path: '/tag-manager', name: 'Tag Manager', icon: Tags, pro: 'tag_manager' },
  ]

  const settingsItems = [
    { path: '/settings/domains', name: 'Domains', icon: Globe },
    { path: '/settings/tracking', name: 'Tracking', icon: Activity, adminOnly: true },
    { path: '/settings/email', name: 'Email', icon: Mail, adminOnly: true },
    { path: '/settings/geoip', name: 'GeoIP', icon: MapPin, adminOnly: true },
    { path: '/settings/connections', name: 'Connections', icon: Cable, adminOnly: true },
    { path: '/settings/account', name: 'Account', icon: User },
    { path: '/settings/users', name: 'Users', icon: UsersIcon, adminOnly: true, pro: 'multi_user' },
    { path: '/settings/license', name: 'License', icon: Key },
  ]

  const visibleSettingsItems = settingsItems.filter(
    item => !item.adminOnly || isAdmin
  )

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader className="border-b border-sidebar-border">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link to="/">
                <img src="/favicon-pixellated.png" alt="Etiquetta" className="h-8 w-8 shrink-0" />
                {!isCollapsed && (
                  <div className="flex flex-col items-start">
                    <span className="font-bold">Etiquetta</span>
                    <span className="text-xs text-muted-foreground">Analytics</span>
                  </div>
                )}
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      {/* Domain picker - only show when expanded */}
      {!isCollapsed && (
        <div className="p-2 border-b border-sidebar-border">
          <DomainPicker />
        </div>
      )}

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Navigation</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navigation
                .filter(item => !item.adminOnly || isAdmin)
                .map((item) => (
                <SidebarMenuItem key={item.path}>
                  <SidebarMenuButton
                    asChild
                    isActive={item.path === '/' ? location.pathname === '/' : location.pathname.startsWith(item.path)}
                    tooltip={item.name}
                  >
                    <Link to={item.path}>
                      <item.icon className="h-4 w-4" />
                      <span className="flex-1">{item.name}</span>
                      {item.pro && !isCollapsed && <FeatureBadge feature={item.pro} />}
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}

              {/* Settings collapsible menu */}
              <Collapsible
                open={settingsOpen}
                onOpenChange={setSettingsOpen}
                className="group/collapsible"
              >
                <SidebarMenuItem>
                  <CollapsibleTrigger asChild>
                    <SidebarMenuButton
                      isActive={location.pathname.startsWith('/settings')}
                      tooltip="Settings"
                    >
                      <SettingsIcon className="h-4 w-4" />
                      <span className="flex-1">Settings</span>
                      {!isCollapsed && (
                        <ChevronRight className="h-4 w-4 transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                      )}
                    </SidebarMenuButton>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <SidebarMenuSub>
                      {visibleSettingsItems.map((item) => (
                        <SidebarMenuSubItem key={item.path}>
                          <SidebarMenuSubButton
                            asChild
                            isActive={location.pathname === item.path}
                          >
                            <Link to={item.path}>
                              <item.icon className="h-4 w-4" />
                              <span className="flex-1">{item.name}</span>
                              {item.pro && <FeatureBadge feature={item.pro} />}
                            </Link>
                          </SidebarMenuSubButton>
                        </SidebarMenuSubItem>
                      ))}
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </SidebarMenuItem>
              </Collapsible>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="border-t border-sidebar-border">
        <SidebarMenu>
          <SidebarMenuItem>
            <ThemeSelector />
          </SidebarMenuItem>
          <SidebarMenuItem>
            <UserMenu />
          </SidebarMenuItem>
        </SidebarMenu>
        {!isCollapsed && (
          <p className="text-xs text-muted-foreground text-center py-2">
            {version}
          </p>
        )}
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  )
}

function AppLayout() {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-12 items-center gap-2 border-b px-4 md:hidden">
          <SidebarTrigger />
          <span className="font-semibold">Etiquetta</span>
        </header>
        <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
          <div className="max-w-[1800px] mx-auto w-full h-full overflow-hidden">
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/compare" element={<Compare />} />
              <Route path="/bots" element={<BotAnalysis />} />
              <Route path="/connections" element={<Connections />} />
              <Route path="/fraud" element={<AdFraud />} />
              <Route path="/explorer" element={<Explorer />} />
              {/* Settings routes */}
              <Route path="/settings" element={<Navigate to="/settings/domains" replace />} />
              <Route path="/settings/domains" element={<DomainsSettings />} />
              <Route path="/settings/tracking" element={<TrackingSettings />} />
              <Route path="/settings/email" element={<EmailSettings />} />
              <Route path="/settings/geoip" element={<GeoIPSettings />} />
              <Route path="/settings/connections" element={<ConnectionsSettings />} />
              <Route path="/settings/account" element={<AccountSettings />} />
              <Route path="/settings/users" element={<UsersSettings />} />
              <Route path="/privacy" element={<PrivacyCenter />} />
              <Route path="/consent" element={<ConsentDashboard />} />
              <Route path="/consent/settings" element={<ConsentConfig />} />
              <Route path="/tag-manager" element={<TagManager />} />
              <Route path="/tag-manager/:containerId" element={<TagManagerContainer />} />
              <Route path="/settings/license" element={
                <div className="p-6 max-w-4xl mx-auto">
                  <div className="mb-6">
                    <h1 className="text-2xl font-bold text-foreground">License</h1>
                    <p className="text-muted-foreground">Manage your license and subscription</p>
                  </div>
                  <LicenseSettings />
                </div>
              } />
            </Routes>
          </div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

function App() {
  return (
    <ThemeProvider defaultTheme="dark" storageKey="etiquetta-ui-theme">
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AuthProvider>
            <Routes>
              <Route path="/login" element={
                <PublicRoute>
                  <Login />
                </PublicRoute>
              } />
              <Route path="/*" element={
                <ProtectedRoute>
                  <AppLayout />
                </ProtectedRoute>
              } />
            </Routes>
          </AuthProvider>
        </BrowserRouter>
        <Toaster richColors position="bottom-right" />
      </QueryClientProvider>
    </ThemeProvider>
  )
}

export default App
