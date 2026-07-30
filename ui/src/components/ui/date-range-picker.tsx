"use client"

import * as React from "react"
import {
  format,
  subDays,
  startOfMonth,
  endOfMonth,
  startOfDay,
  endOfDay,
  subMonths,
  differenceInCalendarDays,
  isSameDay,
  startOfYear,
} from "date-fns"
import { CalendarIcon, ChevronDown } from "lucide-react"
import type { DateRange, DayButton } from "react-day-picker"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Calendar, CalendarDayButton } from "@/components/ui/calendar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { useCalendarHeatmap } from "@/hooks/useCalendarHeatmap"

interface Preset {
  label: string
  value: string
  getRange: () => DateRange
}

const presets: Preset[] = [
  {
    label: "Today",
    value: "today",
    getRange: () => ({ from: startOfDay(new Date()), to: endOfDay(new Date()) }),
  },
  {
    label: "Yesterday",
    value: "yesterday",
    getRange: () => {
      const y = subDays(new Date(), 1)
      return { from: startOfDay(y), to: endOfDay(y) }
    },
  },
  {
    label: "Last 7 days",
    value: "last7days",
    getRange: () => ({ from: startOfDay(subDays(new Date(), 6)), to: endOfDay(new Date()) }),
  },
  {
    label: "Last 14 days",
    value: "last14days",
    getRange: () => ({ from: startOfDay(subDays(new Date(), 13)), to: endOfDay(new Date()) }),
  },
  {
    label: "Last 30 days",
    value: "last30days",
    getRange: () => ({ from: startOfDay(subDays(new Date(), 29)), to: endOfDay(new Date()) }),
  },
  {
    label: "Last 90 days",
    value: "last90days",
    getRange: () => ({ from: startOfDay(subDays(new Date(), 89)), to: endOfDay(new Date()) }),
  },
  {
    label: "This month",
    value: "thisMonth",
    getRange: () => ({ from: startOfMonth(new Date()), to: endOfDay(new Date()) }),
  },
  {
    label: "Last month",
    value: "lastMonth",
    getRange: () => {
      const lm = subMonths(new Date(), 1)
      return { from: startOfMonth(lm), to: endOfMonth(lm) }
    },
  },
  {
    label: "Year to date",
    value: "yearToDate",
    getRange: () => ({ from: startOfYear(new Date()), to: endOfDay(new Date()) }),
  },
]

// Smart detection: find which preset (if any) the current range corresponds to,
// so the right one highlights even when the range came from a URL or a nudge.
function detectPreset(range: DateRange | undefined): string {
  if (!range?.from || !range?.to) return "custom"
  for (const p of presets) {
    const r = p.getRange()
    if (r.from && r.to && isSameDay(r.from, range.from) && isSameDay(r.to, range.to)) {
      return p.value
    }
  }
  return "custom"
}

interface DateRangePickerProps {
  dateRange: DateRange | undefined
  onDateRangeChange: (range: DateRange | undefined) => void
  selectedPreset?: string
  onPresetChange?: (preset: string) => void
  className?: string
  align?: "start" | "center" | "end"
}

export function DateRangePicker({
  dateRange,
  onDateRangeChange,
  onPresetChange,
  className,
  align = "end",
}: DateRangePickerProps) {
  const [isOpen, setIsOpen] = React.useState(false)
  // Draft range while the user is picking. Nothing is committed to the app
  // (and no queries refire) until a full range is selected — this is what
  // keeps the dashboard from reloading twice on every single selection.
  const [draft, setDraft] = React.useState<DateRange | undefined>(dateRange)
  const [displayedMonth, setDisplayedMonth] = React.useState<Date>(
    () => (dateRange?.to ? subMonths(startOfMonth(dateRange.to), 1) : subMonths(startOfMonth(new Date()), 1))
  )

  // Highlight the preset that matches the live selection, always correct.
  const activePreset = detectPreset(draft ?? dateRange)

  const { data: heatmapData } = useCalendarHeatmap(displayedMonth, isOpen)

  const { heatmapMap, maxSessions } = React.useMemo(() => {
    const map = new Map<string, number>()
    let max = 0
    if (heatmapData) {
      for (const point of heatmapData) {
        map.set(point.date, point.sessions)
        if (point.sessions > max) max = point.sessions
      }
    }
    return { heatmapMap: map, maxSessions: max }
  }, [heatmapData])

  function handleOpenChange(open: boolean) {
    if (open) {
      // Start editing from the committed range, and show the two months that
      // end on the selection.
      setDraft(dateRange)
      setDisplayedMonth(
        dateRange?.to ? subMonths(startOfMonth(dateRange.to), 1) : subMonths(startOfMonth(new Date()), 1)
      )
    }
    setIsOpen(open)
  }

  function commit(range: DateRange, preset: string) {
    onPresetChange?.(preset)
    onDateRangeChange(range)
    setDraft(range)
    setIsOpen(false)
  }

  function handlePresetSelect(preset: Preset) {
    commit(preset.getRange(), preset.value)
  }

  function handleCalendarSelect(range: DateRange | undefined) {
    // A complete range → normalize to full-day coverage and commit once.
    if (range?.from && range?.to) {
      const norm: DateRange = { from: startOfDay(range.from), to: endOfDay(range.to) }
      commit(norm, detectPreset(norm))
      return
    }
    // A partial selection (start only) — update the draft for visual feedback
    // but don't touch the app state yet.
    setDraft(range)
  }

  const dayCount =
    dateRange?.from && dateRange?.to
      ? differenceInCalendarDays(dateRange.to, dateRange.from) + 1
      : 0

  function formatLabel(): string {
    if (!dateRange?.from) return "Select date range"
    // Prefer the friendly preset name when the range matches one.
    const preset = presets.find((p) => p.value === activePreset)
    if (preset) return preset.label
    if (!dateRange.to) return format(dateRange.from, "MMM d, yyyy")

    const sameYear = dateRange.from.getFullYear() === dateRange.to.getFullYear()
    const sameMonth = sameYear && dateRange.from.getMonth() === dateRange.to.getMonth()
    if (sameMonth) return `${format(dateRange.from, "MMM d")} – ${format(dateRange.to, "d, yyyy")}`
    if (sameYear) return `${format(dateRange.from, "MMM d")} – ${format(dateRange.to, "MMM d, yyyy")}`
    return `${format(dateRange.from, "MMM d, yyyy")} – ${format(dateRange.to, "MMM d, yyyy")}`
  }

  const HeatmapDayButton = React.useCallback(
    (props: React.ComponentProps<typeof DayButton>) => {
      const { day, modifiers, style: dayStyle, ...rest } = props
      const dateStr = format(day.date, "yyyy-MM-dd")
      const sessions = heatmapMap.get(dateStr)
      const isOutside = modifiers.outside
      const isSelected =
        modifiers.selected || modifiers.range_start || modifiers.range_end || modifiers.range_middle

      const showHeatmap = !isOutside && sessions && sessions > 0 && !isSelected && maxSessions > 0
      const ratio = showHeatmap ? sessions / maxSessions : 0

      let heatColor: string | undefined
      if (showHeatmap) {
        if (ratio > 0.66) heatColor = "rgba(34, 197, 94, 0.50)"
        else if (ratio > 0.33) heatColor = "rgba(34, 197, 94, 0.30)"
        else heatColor = "rgba(34, 197, 94, 0.15)"
      }

      const mergedStyle = heatColor
        ? {
            ...dayStyle,
            backgroundColor: heatColor,
            borderRadius: "6px",
            padding: "2px",
            backgroundClip: "content-box" as const,
          }
        : dayStyle

      return (
        <CalendarDayButton day={day} modifiers={modifiers} style={mergedStyle} {...rest}>
          {props.children}
        </CalendarDayButton>
      )
    },
    [heatmapMap, maxSessions]
  )

  return (
    <Popover open={isOpen} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          className={cn(
            "justify-start text-left font-normal min-w-[200px]",
            !dateRange && "text-muted-foreground",
            className
          )}
        >
          <CalendarIcon className="mr-2 h-4 w-4 shrink-0" />
          <span className="flex-1 truncate">{formatLabel()}</span>
          <ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={align}>
        <div className="flex">
          {/* Presets */}
          <div className="flex flex-col gap-0.5 border-r border-border p-2 max-w-[140px]">
            {presets.map((preset) => (
              <button
                key={preset.value}
                onClick={() => handlePresetSelect(preset)}
                className={cn(
                  "w-full rounded-md px-3 py-1.5 text-left text-sm transition-colors",
                  activePreset === preset.value
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-accent hover:text-accent-foreground"
                )}
              >
                {preset.label}
              </button>
            ))}
            {activePreset === "custom" && (
              <div className="mt-1 rounded-md bg-accent px-3 py-1.5 text-left text-sm text-accent-foreground">
                Custom
              </div>
            )}
          </div>

          {/* Calendar */}
          <div className="flex flex-col">
            <div className="p-3">
              <Calendar
                mode="range"
                selected={draft}
                onSelect={handleCalendarSelect}
                numberOfMonths={2}
                month={displayedMonth}
                onMonthChange={setDisplayedMonth}
                disabled={(date) => date > new Date()}
                components={{ DayButton: HeatmapDayButton }}
              />
            </div>
            {dayCount > 0 && (
              <div className="border-t border-border px-4 py-2 text-xs text-muted-foreground">
                {dayCount === 1 ? "1 day selected" : `${dayCount} days selected`}
              </div>
            )}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
