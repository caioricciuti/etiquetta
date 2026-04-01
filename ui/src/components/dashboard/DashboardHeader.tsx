import { useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { format } from 'date-fns'
import { RefreshCw, ExternalLink, Bot, Download } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { downloadCSV } from '../../lib/csv-export'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu'
import { useDateRangeStore, paramsToDateRange } from '../../stores/useDateRangeStore'
import { useDomainStore } from '../../stores/useDomainStore'
import { useFilterStore } from '../../stores/useFilterStore'
import { useSelectedDomain } from '../../hooks/useSelectedDomain'
import { useLicense } from '../../hooks/useLicenseQuery'
import { Button } from '../ui/button'
import { DateRangePicker } from '../ui/date-range-picker'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import { Card, CardContent } from '../ui/card'

export function DashboardHeader() {
  const queryClient = useQueryClient()
  const { selectedDomain, domains } = useSelectedDomain()
  const { setSelectedDomainId } = useDomainStore()
  const { dateRange, setDateRange, selectedPreset, setPreset } = useDateRangeStore()
  const { filters, setFilter, removeFilter } = useFilterStore()
  const [searchParams, setSearchParams] = useSearchParams()
  const initializedFromUrl = useRef(false)

  // Initialize from URL params on first load
  useEffect(() => {
    if (initializedFromUrl.current) return
    initializedFromUrl.current = true

    const startParam = searchParams.get('start')
    const endParam = searchParams.get('end')
    const domainParam = searchParams.get('domain')

    const urlDateRange = paramsToDateRange(startParam, endParam)
    if (urlDateRange) {
      setDateRange(urlDateRange)
      setPreset('custom')
    }

    if (domainParam && domains.length > 0) {
      const domainFromUrl = domains.find((d) => d.domain === domainParam)
      if (domainFromUrl) {
        setSelectedDomainId(domainFromUrl.id)
      }
    }

    // Restore filters from URL
    const store = useFilterStore.getState()
    const countryParam = searchParams.get('country')
    const browserParam = searchParams.get('browser')
    const deviceParam = searchParams.get('device')
    const pageParam = searchParams.get('page')
    const referrerParam = searchParams.get('referrer')
    if (countryParam) store.setFilter('country', countryParam)
    if (browserParam) store.setFilter('browser', browserParam)
    if (deviceParam) store.setFilter('device', deviceParam)
    if (pageParam) store.setFilter('page', pageParam)
    if (referrerParam) store.setFilter('referrer', referrerParam)
  }, [searchParams, domains, setDateRange, setPreset, setSelectedDomainId])

  // Sync state to URL
  useEffect(() => {
    const params = new URLSearchParams()
    if (dateRange?.from && dateRange?.to) {
      params.set('start', format(dateRange.from, 'yyyy-MM-dd'))
      params.set('end', format(dateRange.to, 'yyyy-MM-dd'))
    }
    if (selectedDomain) {
      params.set('domain', selectedDomain.domain)
    }
    for (const [key, value] of Object.entries(filters)) {
      if (value) params.set(key, value)
    }
    setSearchParams(params, { replace: true })
  }, [dateRange, selectedDomain, filters, setSearchParams])

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['stats'] })
  }

  const getQs = () => {
    const p = new URLSearchParams()
    if (dateRange?.from && dateRange?.to) {
      p.set('start', dateRange.from.toISOString())
      p.set('end', dateRange.to.toISOString())
    } else {
      p.set('days', '7')
    }
    if (selectedDomain) p.set('domain', selectedDomain.domain)
    for (const [key, value] of Object.entries(filters)) {
      if (value) p.set(key, value)
    }
    return p.toString()
  }

  const exportData = (type: string) => {
    const qs = getQs()
    const data = queryClient.getQueryData<Record<string, unknown>[]>(['stats', type, qs])
    if (!data?.length) return
    const domain = selectedDomain?.domain ?? 'analytics'
    downloadCSV(data, `${domain}-${type}`)
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Dashboard</h1>
          <div className="flex items-center gap-3 mt-1 flex-wrap">
            {selectedDomain && (
              <span className="text-sm text-muted-foreground">
                {selectedDomain.domain}
              </span>
            )}
          </div>
        </div>
        <div className="flex items-center gap-3">
          <Select
            value={filters.bot_filter || 'humans'}
            onValueChange={(value) => {
              if (value === 'humans') {
                removeFilter('bot_filter')
              } else {
                setFilter('bot_filter', value)
              }
            }}
          >
            <SelectTrigger className="w-[140px] h-9 text-xs">
              <Bot className="h-3.5 w-3.5 mr-1.5" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="humans">Humans only</SelectItem>
              <SelectItem value="all">All traffic</SelectItem>
              <SelectItem value="bots">Bots only</SelectItem>
              <SelectItem value="good_bots">Good bots</SelectItem>
              <SelectItem value="bad_bots">Bad bots</SelectItem>
              <SelectItem value="ai_crawlers">AI Crawlers</SelectItem>
              <SelectItem value="suspicious">Suspicious</SelectItem>
            </SelectContent>
          </Select>
          <DateRangePicker
            dateRange={dateRange}
            onDateRangeChange={setDateRange}
            selectedPreset={selectedPreset}
            onPresetChange={setPreset}
          />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline">
                <Download className="h-4 w-4 mr-2" />
                Export
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => exportData('pages')}>Pages</DropdownMenuItem>
              <DropdownMenuItem onClick={() => exportData('referrers')}>Referrers</DropdownMenuItem>
              <DropdownMenuItem onClick={() => exportData('geo')}>Countries</DropdownMenuItem>
              <DropdownMenuItem onClick={() => exportData('devices')}>Devices</DropdownMenuItem>
              <DropdownMenuItem onClick={() => exportData('browsers')}>Browsers</DropdownMenuItem>
              <DropdownMenuItem onClick={() => exportData('campaigns')}>Campaigns</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button onClick={handleRefresh} size="sm" variant="outline">
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
        </div>
      </div>

      <LicenseBanner />
    </div>
  )
}

function LicenseBanner() {
  const { license } = useLicense()

  if (license?.tier !== 'community') return null

  return (
    <Card className="bg-gradient-to-r from-blue-600 to-purple-600 border-0">
      <CardContent className="p-4">
        <div className="flex items-center justify-between flex-wrap gap-4">
          <div className="text-white">
            <p className="font-semibold">Upgrade to Pro</p>
            <p className="text-sm text-white/80">
              Unlock performance monitoring, error tracking, and more.
            </p>
          </div>
          <Button variant="secondary" size="sm" asChild>
            <a href="https://etiquetta.com/pricing" target="_blank" rel="noopener noreferrer">
              View Plans
              <ExternalLink className="h-3 w-3 ml-1" />
            </a>
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
