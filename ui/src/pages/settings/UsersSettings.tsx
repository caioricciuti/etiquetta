import { useState, useEffect, useCallback } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { useLicense } from '@/hooks/useLicenseQuery'
import { fetchAPI, ApiError } from '@/lib/api'
import { Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from '@/components/ui/sheet'
import { Users as UsersIcon, Plus, Trash2, Loader2, Shield, Eye, AlertCircle, Pencil } from 'lucide-react'
import { toast } from 'sonner'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsLayout } from './SettingsLayout'

interface User {
  id: string
  email: string
  name: string
  role: 'admin' | 'viewer'
  created_at: number
}

export function UsersSettings() {
  const { user: currentUser, isAdmin } = useAuth()
  const { hasFeature, getLimit } = useLicense()
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Create user sheet state
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [newUser, setNewUser] = useState({
    email: '',
    name: '',
    password: '',
    confirmPassword: '',
    role: 'viewer' as 'admin' | 'viewer',
  })

  // Edit user sheet state
  const [editingUser, setEditingUser] = useState<User | null>(null)
  const [editForm, setEditForm] = useState({ name: '', role: '' as 'admin' | 'viewer', password: '' })
  const [saving, setSaving] = useState(false)
  const [editError, setEditError] = useState<string | null>(null)

  const maxUsers = getLimit('max_users')

  const fetchUsers = useCallback(async () => {
    try {
      const data = await fetchAPI<User[]>('/api/users')
      setUsers(data || [])
    } catch (err) {
      if (err instanceof ApiError && err.status === 402) {
        setError('User management requires a Pro license')
      } else {
        setError('Failed to load users')
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isAdmin) {
      fetchUsers()
    } else {
      setLoading(false)
      setError('Admin access required')
    }
  }, [fetchUsers, isAdmin])

  if (!isAdmin) {
    return <Navigate to="/settings/domains" replace />
  }

  if (!hasFeature('multi_user')) {
    return (
      <SettingsLayout title="Users" description="Manage team access to your analytics">
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <AlertCircle className="h-12 w-12 text-muted-foreground mb-4" />
            <h2 className="text-xl font-semibold mb-2">Pro Feature</h2>
            <p className="text-muted-foreground text-center max-w-md">
              User management is available with a Pro or Enterprise license.
              Upgrade to add team members and manage access.
            </p>
          </CardContent>
        </Card>
      </SettingsLayout>
    )
  }

  function openCreateSheet() {
    setNewUser({ email: '', name: '', password: '', confirmPassword: '', role: 'viewer' })
    setCreateError(null)
    setCreateOpen(true)
  }

  async function handleCreateUser(e: React.FormEvent) {
    e.preventDefault()
    setCreateError(null)

    if (!newUser.email || !newUser.email.includes('@')) {
      setCreateError('Please enter a valid email address')
      return
    }
    if (newUser.password.length < 8) {
      setCreateError('Password must be at least 8 characters')
      return
    }
    if (newUser.password !== newUser.confirmPassword) {
      setCreateError('Passwords do not match')
      return
    }

    setCreating(true)

    try {
      await fetchAPI('/api/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: newUser.email,
          name: newUser.name,
          password: newUser.password,
          role: newUser.role,
        }),
      })

      setCreateOpen(false)
      toast.success('User created')
      fetchUsers()
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create user')
    } finally {
      setCreating(false)
    }
  }

  async function handleDeleteUser(userId: string, userEmail: string) {
    if (userId === currentUser?.id) {
      toast.error('You cannot delete your own account')
      return
    }

    if (!confirm(`Are you sure you want to delete ${userEmail}?`)) {
      return
    }

    try {
      await fetchAPI(`/api/users/${userId}`, { method: 'DELETE' })
      toast.success('User deleted')
      fetchUsers()
    } catch (err) {
      toast.error('Failed to delete user', { description: err instanceof Error ? err.message : undefined })
    }
  }

  function openEditSheet(user: User) {
    setEditingUser(user)
    setEditForm({ name: user.name || '', role: user.role, password: '' })
    setEditError(null)
  }

  async function handleEditUser(e: React.FormEvent) {
    e.preventDefault()
    if (!editingUser) return
    setEditError(null)

    if (editForm.password && editForm.password.length < 8) {
      setEditError('Password must be at least 8 characters')
      return
    }

    setSaving(true)

    try {
      await fetchAPI(`/api/users/${editingUser.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: editForm.name,
          role: editForm.role,
          ...(editForm.password ? { password: editForm.password } : {}),
        }),
      })

      toast.success('User updated')
      setEditingUser(null)
      fetchUsers()
    } catch (err) {
      setEditError(err instanceof Error ? err.message : 'Failed to update user')
    } finally {
      setSaving(false)
    }
  }

  function formatDate(timestamp: number): string {
    return new Date(timestamp).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  }

  return (
    <SettingsLayout title="Users" description="Manage team access to your analytics">
      {/* Header with Add button */}
      <div className="flex justify-end -mt-4 mb-2">
        <Button
          onClick={openCreateSheet}
          disabled={maxUsers !== -1 && users.length >= maxUsers}
        >
          <Plus className="h-4 w-4 mr-2" />
          Add User
        </Button>
      </div>

      {/* Create user sheet */}
      <Sheet open={createOpen} onOpenChange={setCreateOpen}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Add User</SheetTitle>
            <SheetDescription>
              Create a new user account. They will be able to log in with the credentials you provide.
            </SheetDescription>
          </SheetHeader>
          <form onSubmit={handleCreateUser} className="flex flex-col flex-1 overflow-y-auto px-4">
            <div className="space-y-4">
              {createError && (
                <div className="p-3 rounded text-sm bg-destructive/10 text-destructive">
                  {createError}
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="create-email">Email</Label>
                <Input
                  id="create-email"
                  type="email"
                  placeholder="user@example.com"
                  value={newUser.email}
                  onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-name">Name (optional)</Label>
                <Input
                  id="create-name"
                  type="text"
                  placeholder="John Doe"
                  value={newUser.name}
                  onChange={(e) => setNewUser({ ...newUser, name: e.target.value })}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-role">Role</Label>
                <Select
                  value={newUser.role}
                  onValueChange={(value: 'admin' | 'viewer') => setNewUser({ ...newUser, role: value })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="viewer">
                      <div className="flex items-center gap-2">
                        <Eye className="h-4 w-4" />
                        Viewer
                      </div>
                    </SelectItem>
                    <SelectItem value="admin">
                      <div className="flex items-center gap-2">
                        <Shield className="h-4 w-4" />
                        Admin
                      </div>
                    </SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  Viewers can view analytics. Admins can manage settings and users.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-password">Password</Label>
                <Input
                  id="create-password"
                  type="password"
                  placeholder="Minimum 8 characters"
                  value={newUser.password}
                  onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-confirm">Confirm Password</Label>
                <Input
                  id="create-confirm"
                  type="password"
                  placeholder="Confirm password"
                  value={newUser.confirmPassword}
                  onChange={(e) => setNewUser({ ...newUser, confirmPassword: e.target.value })}
                  required
                />
              </div>
            </div>

            <SheetFooter className="px-0">
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={creating}>
                {creating && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create User
              </Button>
            </SheetFooter>
          </form>
        </SheetContent>
      </Sheet>

      {/* Edit user sheet */}
      <Sheet open={!!editingUser} onOpenChange={(open) => { if (!open) setEditingUser(null) }}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Edit User</SheetTitle>
            <SheetDescription>
              {editingUser?.email}
            </SheetDescription>
          </SheetHeader>
          <form onSubmit={handleEditUser} className="flex flex-col flex-1 overflow-y-auto px-4">
            <div className="space-y-4">
              {editError && (
                <div className="p-3 rounded text-sm bg-destructive/10 text-destructive">
                  {editError}
                </div>
              )}

              <div className="space-y-2">
                <Label>Name</Label>
                <Input
                  type="text"
                  placeholder="Name"
                  value={editForm.name}
                  onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
                />
              </div>

              <div className="space-y-2">
                <Label>Role</Label>
                <Select
                  value={editForm.role}
                  onValueChange={(value: 'admin' | 'viewer') => setEditForm({ ...editForm, role: value })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="viewer">
                      <div className="flex items-center gap-2">
                        <Eye className="h-4 w-4" />
                        Viewer
                      </div>
                    </SelectItem>
                    <SelectItem value="admin">
                      <div className="flex items-center gap-2">
                        <Shield className="h-4 w-4" />
                        Admin
                      </div>
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>New Password (optional)</Label>
                <Input
                  type="password"
                  placeholder="Leave blank to keep current"
                  value={editForm.password}
                  onChange={(e) => setEditForm({ ...editForm, password: e.target.value })}
                />
                <p className="text-xs text-muted-foreground">
                  Only fill this if you want to change the password.
                </p>
              </div>
            </div>

            <SheetFooter className="px-0">
              <Button type="button" variant="outline" onClick={() => setEditingUser(null)}>
                Cancel
              </Button>
              <Button type="submit" disabled={saving}>
                {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Save Changes
              </Button>
            </SheetFooter>
          </form>
        </SheetContent>
      </Sheet>

      {/* User limit warning */}
      {maxUsers !== -1 && users.length >= maxUsers && (
        <Card className="border-yellow-500/50 bg-yellow-500/5">
          <CardContent className="flex items-center gap-3 py-4">
            <AlertCircle className="h-5 w-5 text-yellow-500" />
            <div>
              <p className="font-medium text-yellow-600 dark:text-yellow-400">User limit reached</p>
              <p className="text-sm text-muted-foreground">
                Your license allows {maxUsers} user{maxUsers !== 1 ? 's' : ''}. Upgrade to add more team members.
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <UsersIcon className="h-5 w-5" />
            Team Members
          </CardTitle>
          <CardDescription>
            {maxUsers === -1
              ? `${users.length} user${users.length !== 1 ? 's' : ''}`
              : `${users.length} of ${maxUsers} user${maxUsers !== 1 ? 's' : ''}`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : error ? (
            <div className="text-center py-8 text-muted-foreground">{error}</div>
          ) : users.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              No users found. Add your first team member.
            </div>
          ) : (
            <div className="space-y-3">
              {users.map((user) => (
                <div
                  key={user.id}
                  className="flex items-center justify-between p-4 rounded-lg border border-border"
                >
                  <div className="flex items-center gap-3">
                    <div className="h-10 w-10 rounded-full bg-primary/10 flex items-center justify-center">
                      <span className="text-sm font-medium text-primary">
                        {(user.name || user.email).charAt(0).toUpperCase()}
                      </span>
                    </div>
                    <div>
                      <p className="font-medium">
                        {user.name || user.email}
                        {user.id === currentUser?.id && (
                          <span className="ml-2 text-xs text-muted-foreground">(you)</span>
                        )}
                      </p>
                      <p className="text-sm text-muted-foreground">{user.email}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <span
                      className={`px-2 py-1 text-xs font-medium rounded-full ${
                        user.role === 'admin'
                          ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                          : 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400'
                      }`}
                    >
                      {user.role === 'admin' ? (
                        <span className="flex items-center gap-1">
                          <Shield className="h-3 w-3" />
                          Admin
                        </span>
                      ) : (
                        <span className="flex items-center gap-1">
                          <Eye className="h-3 w-3" />
                          Viewer
                        </span>
                      )}
                    </span>
                    <span className="text-xs text-muted-foreground hidden sm:block">
                      {formatDate(user.created_at)}
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openEditSheet(user)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    {user.id !== currentUser?.id && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDeleteUser(user.id, user.email)}
                        className="text-destructive hover:text-destructive"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </SettingsLayout>
  )
}
