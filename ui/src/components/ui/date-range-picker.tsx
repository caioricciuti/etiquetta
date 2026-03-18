"use client"

import * as React from "react"
import { format, subDays, startOfMonth, endOfMonth, startOfDay, endOfDay, subMonths } from "date-fns"
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

const presets = [
  {
    label: "Today",
    value: "today",
    getRange: (): DateRange => {
      const today = new Date()
      return { from: startOfDay(today), to: endOfDay(today) }
    },
  },
  {
    label: "Yesterday",
    value: "yesterday",
    getRange: (): DateRange => {
      const yesterday = subDays(new Date(), 1)
      return { from: startOfDay(yesterday), to: endOfDay(yesterday) }
    },
  },
  {
    label: "Last 7 days",
    value: "last7days",
    getRange: (): DateRange => ({
      from: startOfDay(subDays(new Date(), 6)),
      to: endOfDay(new Date()),
    }),
  },
  {
    label: "Last 14 days",
    value: "last14days",
    getRange: (): DateRange => ({
      from: startOfDay(subDays(new Date(), 13)),
      to: endOfDay(new Date()),
    }),
  },
  {
    label: "Last 30 days",
    value: "last30days",
    getRange: (): DateRange => ({
      from: startOfDay(subDays(new Date(), 29)),
      to: endOfDay(new Date()),
    }),
  },
  {
    label: "Last 90 days",
    value: "last90days",
    getRange: (): DateRange => ({
      from: startOfDay(subDays(new Date(), 89)),
      to: endOfDay(new Date()),
    }),
  },
  {
    label: "This month",
    value: "thisMonth",
    getRange: (): DateRange => ({
      from: startOfMonth(new Date()),
      to: endOfDay(new Date()),
    }),
  },
  {
    label: "Last month",
    value: "lastMonth",
    getRange: (): DateRange => {
      const lastMonth = subMonths(new Date(), 1)
      return {
        from: startOfMonth(lastMonth),
        to: endOfMonth(lastMonth),
      }
    },
  },
]

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
  selectedPreset = "last7days",
  onPresetChange,
  className,
  align = "end",
}: DateRangePickerProps) {
  const [isOpen, setIsOpen] = React.useState(false)
  const [displayedMonth, setDisplayedMonth] = React.useState(
    () => dateRange?.from ? subMonths(dateRange.from, 0) : subMonths(new Date(), 1)
  )

  // Fetch heatmap data for displayed months
  const { data: heatmapData } = useCalendarHeatmap(displayedMonth, isOpen)

  // Build lookup map and compute max sessions for heatmap
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

  // Reset displayed month when popover opens
  const handleOpenChange = (open: boolean) => {
    if (open) {
      setDisplayedMonth(dateRange?.from ? subMonths(dateRange.from, 0) : subMonths(new Date(), 1))
    }
    setIsOpen(open)
  }

  const handlePresetSelect = (preset: typeof presets[number]) => {
    onPresetChange?.(preset.value)
    onDateRangeChange(preset.getRange())
    setIsOpen(false)
  }

  const handleCalendarSelect = (range: DateRange | undefined) => {
    onPresetChange?.("custom")
    // Ensure full day coverage with startOfDay/endOfDay
    if (range?.from) {
      const normalizedRange: DateRange = {
        from: startOfDay(range.from),
        to: range.to ? endOfDay(range.to) : endOfDay(range.from)
      }
      onDateRangeChange(normalizedRange)
    } else {
      onDateRangeChange(range)
    }
  }

  const formatDateRange = () => {
    if (!dateRange?.from) return "Select date range"
    if (!dateRange.to) return format(dateRange.from, "MMM d, yyyy")

    // Same month and year
    if (
      dateRange.from.getMonth() === dateRange.to.getMonth() &&
      dateRange.from.getFullYear() === dateRange.to.getFullYear()
    ) {
      return `${format(dateRange.from, "MMM d")} - ${format(dateRange.to, "d, yyyy")}`
    }

    // Same year
    if (dateRange.from.getFullYear() === dateRange.to.getFullYear()) {
      return `${format(dateRange.from, "MMM d")} - ${format(dateRange.to, "MMM d, yyyy")}`
    }

    return `${format(dateRange.from, "MMM d, yyyy")} - ${format(dateRange.to, "MMM d, yyyy")}`
  }

  // Custom DayButton with heatmap overlay
  const HeatmapDayButton = React.useCallback(
    (props: React.ComponentProps<typeof DayButton>) => {
      const { day, modifiers, style: dayStyle, ...rest } = props
      const dateStr = format(day.date, "yyyy-MM-dd")
      const sessions = heatmapMap.get(dateStr)
      const isOutside = modifiers.outside
      const isSelected = modifiers.selected || modifiers.range_start || modifiers.range_end || modifiers.range_middle

      const showHeatmap = !isOutside && sessions && sessions > 0 && !isSelected && maxSessions > 0
      const ratio = showHeatmap ? sessions / maxSessions : 0

      let heatColor: string | undefined
      if (showHeatmap) {
        if (ratio > 0.66) heatColor = 'rgba(239, 68, 68, 0.3)'
        else if (ratio > 0.33) heatColor = 'rgba(249, 115, 22, 0.25)'
        else heatColor = 'rgba(251, 191, 36, 0.2)'
      }

      const mergedStyle = heatColor
        ? {
            ...dayStyle,
            backgroundColor: heatColor,
            borderRadius: '6px',
            padding: '2px',
            backgroundClip: 'content-box' as const,
          }
        : dayStyle

      return (
        <CalendarDayButton
          day={day}
          modifiers={modifiers}
          style={mergedStyle}
          {...rest}
        >
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
            "justify-start text-left font-normal min-w-[240px]",
            !dateRange && "text-muted-foreground",
            className
          )}
        >
          <CalendarIcon className="mr-2 h-4 w-4" />
          <span className="flex-1 truncate">{formatDateRange()}</span>
          <ChevronDown className="ml-2 h-4 w-4 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={align}>
        <div className="flex">
          {/* Presets sidebar */}
          <div className="border-r border-border p-2 space-y-1 max-w-[130px]">
            {presets.map((preset) => (
              <button
                key={preset.value}
                onClick={() => handlePresetSelect(preset)}
                className={cn(
                  "w-full text-left px-3 py-2 text-sm rounded-md transition-colors",
                  selectedPreset === preset.value
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-accent hover:text-accent-foreground"
                )}
              >
                {preset.label}
              </button>
            ))}
            <div className="border-t border-border my-2" />
            <button
              onClick={() => onPresetChange?.("custom")}
              className={cn(
                "w-full text-left px-3 py-2 text-sm rounded-md transition-colors",
                selectedPreset === "custom"
                  ? "bg-primary text-primary-foreground"
                  : "hover:bg-accent hover:text-accent-foreground"
              )}
            >
              Custom
            </button>
          </div>

          {/* Calendar */}
          <div className="p-3">
            <Calendar
              mode="range"
              selected={dateRange}
              onSelect={handleCalendarSelect}
              numberOfMonths={2}
              month={displayedMonth}
              onMonthChange={setDisplayedMonth}
              disabled={(date) => date > new Date()}
              components={{
                DayButton: HeatmapDayButton,
              }}
            />
          </div>
        </div>

        {/* Footer with apply button for custom ranges */}
        {selectedPreset === "custom" && (
          <div className="border-t border-border p-3 flex justify-end">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIsOpen(false)}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => setIsOpen(false)}
              disabled={!dateRange?.from || !dateRange?.to}
            >
              Apply
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
