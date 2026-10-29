import {
  addCalendarDays,
  calendarToday,
  formatCalendarDate,
  isCalendarDate,
} from "@/lib/calendar"

export const DAILY_REVIEW_TIMEZONE = "Asia/Shanghai"

export interface DailyReviewQuery {
  date: string
}

export function dailyReviewToday(now = new Date()) {
  return calendarToday(now)
}

export function parseDailyReviewSearchParams(
  searchParams: Pick<URLSearchParams, "get">,
  now = new Date()
): DailyReviewQuery {
  const today = dailyReviewToday(now)
  const value = searchParams.get("date")
  return {
    date: isCalendarDate(value) && value <= today ? value : today,
  }
}

export function dailyReviewSearchParams(
  current: URLSearchParams,
  query: DailyReviewQuery
) {
  const next = new URLSearchParams(current)
  next.set("date", query.date)
  return next
}

export function addDailyReviewDays(value: string, amount: number) {
  return addCalendarDays(value, amount)
}

export function formatDailyReviewDate(value: string, locale: string) {
  return formatCalendarDate(value, locale, {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "long",
  })
}

export function formatDailyReviewUpdatedAt(value: string, locale: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat(locale, {
    timeZone: DAILY_REVIEW_TIMEZONE,
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date)
}
