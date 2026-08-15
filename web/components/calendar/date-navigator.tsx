"use client"

import * as React from "react"
import { enUS, zhCN } from "date-fns/locale"
import {
  CalendarDaysIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { Locale, TFunction } from "@/lib/i18n"
import {
  addCalendarDays,
  calendarDateToLocalDate,
  calendarToday,
  calendarWeek,
  formatCalendarDate,
  formatCalendarWeekRange,
  localDateToCalendarDate,
  millisecondsUntilNextCalendarMidnight,
} from "@/lib/calendar"
import { cn } from "@/lib/utils"

export function CalendarDateNavigator({
  date,
  locale,
  t,
  onDateChange,
}: {
  date: string
  locale: Locale
  t: TFunction
  onDateChange: (date: string) => void
}) {
  const [calendarOpen, setCalendarOpen] = React.useState(false)
  const [today, setToday] = React.useState(() => calendarToday())
  const dates = React.useMemo(() => calendarWeek(date), [date])
  const dateLocale = locale === "zh-CN" ? zhCN : enUS

  React.useEffect(() => {
    let timer: number

    const updateToday = () => {
      const now = new Date()
      setToday(calendarToday(now))
      timer = window.setTimeout(
        updateToday,
        millisecondsUntilNextCalendarMidnight(now)
      )
    }

    updateToday()
    return () => window.clearTimeout(timer)
  }, [])

  return (
    <div className="border-b border-border bg-background px-3 py-3 sm:px-5 sm:py-4">
      <div
        className="flex items-center justify-between gap-2"
        role="group"
        aria-label={t("pages.calendar.dateNavigation")}
      >
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label={t("pages.calendar.previousWeek")}
                onClick={() => onDateChange(addCalendarDays(date, -7))}
              >
                <ChevronLeftIcon aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("pages.calendar.previousWeek")}</TooltipContent>
          </Tooltip>

          <Button
            type="button"
            variant="outline"
            size="sm"
            className="min-w-14"
            onClick={() => onDateChange(today)}
          >
            {t("pages.calendar.today")}
          </Button>
        </div>

        <Popover open={calendarOpen} onOpenChange={setCalendarOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              className="max-w-[15rem] min-w-0 gap-2 px-2 text-sm font-semibold sm:max-w-none"
              aria-label={t("pages.calendar.chooseDate")}
            >
              <CalendarDaysIcon aria-hidden="true" />
              <span className="truncate">
                {formatCalendarWeekRange(date, locale)}
              </span>
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-0" align="center">
            <Calendar
              mode="single"
              locale={dateLocale}
              selected={calendarDateToLocalDate(date)}
              defaultMonth={calendarDateToLocalDate(date)}
              onSelect={(value) => {
                if (!value) return
                onDateChange(localDateToCalendarDate(value))
                setCalendarOpen(false)
              }}
            />
          </PopoverContent>
        </Popover>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              aria-label={t("pages.calendar.nextWeek")}
              onClick={() => onDateChange(addCalendarDays(date, 7))}
            >
              <ChevronRightIcon aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("pages.calendar.nextWeek")}</TooltipContent>
        </Tooltip>
      </div>

      <div className="mt-3 grid grid-cols-7 gap-1" role="list">
        {dates.map((item) => {
          const selected = item === date
          const isToday = item === today
          const dayNumber = calendarDateToLocalDate(item)?.getDate()
          return (
            <div key={item} role="listitem" className="min-w-0">
              <button
                type="button"
                aria-pressed={selected}
                aria-current={isToday ? "date" : undefined}
                aria-label={formatCalendarDate(item, locale, {
                  weekday: "long",
                  year: "numeric",
                  month: "long",
                  day: "numeric",
                })}
                onClick={() => onDateChange(item)}
                className={cn(
                  "flex h-15 w-full min-w-0 flex-col items-center justify-center border-b-2 px-1 text-center transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 sm:h-16",
                  selected
                    ? "border-foreground bg-muted text-foreground"
                    : "border-transparent text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                  isToday && !selected && "text-primary"
                )}
              >
                <span className="w-full truncate text-[11px] font-medium sm:text-xs">
                  {formatCalendarDate(item, locale, { weekday: "short" })}
                </span>
                <span className="mt-1 text-base font-semibold whitespace-nowrap tabular-nums">
                  {dayNumber ?? ""}
                </span>
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}
