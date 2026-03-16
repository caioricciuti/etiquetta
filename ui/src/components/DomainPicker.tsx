import { useState } from 'react'
import { ChevronsUpDown, Globe, Check, Plus, Settings } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { useSelectedDomain } from '@/hooks/useSelectedDomain'
import { cn } from '@/lib/utils'

function getAvatarColor(str: string): string {
  const colors = [
    'bg-red-500', 'bg-orange-500', 'bg-amber-500', 'bg-yellow-500',
    'bg-lime-500', 'bg-green-500', 'bg-emerald-500', 'bg-teal-500',
    'bg-cyan-500', 'bg-sky-500', 'bg-blue-500', 'bg-indigo-500',
    'bg-violet-500', 'bg-purple-500', 'bg-fuchsia-500', 'bg-pink-500',
    'bg-rose-500',
  ]
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    hash = str.charCodeAt(i) + ((hash << 5) - hash)
  }
  return colors[Math.abs(hash) % colors.length]
}

function getInitials(name: string): string {
  const words = name.trim().split(/\s+/)
  if (words.length === 1) return name.slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function DomainAvatar({ name, className }: { name: string; className?: string }) {
  return (
    <div className={cn('flex h-7 w-7 items-center justify-center rounded-md text-xs font-semibold text-white shrink-0', getAvatarColor(name), className)}>
      {getInitials(name)}
    </div>
  )
}

export function DomainPicker() {
  const { domains, selectedDomain, setSelectedDomainId, isLoading } = useSelectedDomain()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)

  if (isLoading) {
    return (
      <Button variant="outline" className="w-full justify-start" disabled>
        <div className="h-7 w-7 rounded-md bg-muted animate-pulse shrink-0" />
        <span className="ml-2">Loading...</span>
      </Button>
    )
  }

  if (domains.length === 0) {
    return (
      <Button variant="outline" className="w-full justify-start" asChild>
        <Link to="/settings">
          <Plus className="mr-2 h-4 w-4" />
          Add Domain
        </Link>
      </Button>
    )
  }

  const handleSelect = (domainId: string | null) => {
    setSelectedDomainId(domainId)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" role="combobox" aria-expanded={open} className="w-full justify-between h-auto py-2">
          <span className="flex items-center gap-2 min-w-0">
            {selectedDomain ? (
              <>
                <DomainAvatar name={selectedDomain.name} />
                <div className="flex flex-col items-start min-w-0 flex-1">
                  <span className="font-medium truncate w-full text-left">{selectedDomain.name}</span>
                  <span className="text-xs text-muted-foreground truncate w-full text-left">{selectedDomain.domain}</span>
                </div>
              </>
            ) : (
              <>
                <div className="flex h-7 w-7 items-center justify-center rounded-md bg-muted shrink-0">
                  <Globe className="h-4 w-4 text-muted-foreground" />
                </div>
                <span className="font-medium text-muted-foreground">Select Domain</span>
              </>
            )}
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[280px] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search domains..." />
          <CommandList>
            <CommandEmpty>No domains found.</CommandEmpty>
            <CommandGroup heading="Your Domains">
              {domains.map((domain) => (
                <CommandItem key={domain.id} value={`${domain.name} ${domain.domain}`} onSelect={() => handleSelect(domain.id)} className="py-2">
                  <DomainAvatar name={domain.name} />
                  <div className="flex flex-col min-w-0 flex-1">
                    <span className="font-medium truncate">{domain.name}</span>
                    <span className="text-xs text-muted-foreground truncate">{domain.domain}</span>
                  </div>
                  {selectedDomain?.id === domain.id && <Check className="ml-auto h-4 w-4 shrink-0 text-primary" />}
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
            <CommandGroup>
              <CommandItem
                value="manage-domains"
                onSelect={() => {
                  setOpen(false)
                  navigate('/settings')
                }}
                className="py-2"
              >
                <div className="flex h-7 w-7 items-center justify-center rounded-md bg-muted shrink-0">
                  <Settings className="h-4 w-4 text-muted-foreground" />
                </div>
                <span>Manage Domains</span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
