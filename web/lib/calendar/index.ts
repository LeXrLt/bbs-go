export const CALENDAR_TIMEZONE = "Asia/Shanghai"
export const CALENDAR_PAGE_SIZE = 50

export const CALENDAR_KINDS = [
  "economic",
  "earnings",
  "corporate",
  "ipo",
] as const

export type CalendarKind = (typeof CALENDAR_KINDS)[number]
export type CalendarKindFilter = CalendarKind | ""
export type CalendarImportance = 1 | 2 | 3
export type CalendarImportanceFilter = CalendarImportance | ""

export interface CalendarQueryState {
  date: string
  kind: CalendarKindFilter
  minImportance: CalendarImportanceFilter
  keyword: string
}

export interface CalendarSearchState {
  query: CalendarQueryState
  searchParams: URLSearchParams
}

const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/

function dateParts(value: string) {
  const match = DATE_PATTERN.exec(value)
  if (!match) return null

  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const date = new Date(Date.UTC(year, month - 1, day, 12))

  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return null
  }

  return { year, month, day }
}

function dateFromParts(year: number, month: number, day: number) {
  return `${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`
}

export function isCalendarDate(
  value: string | null | undefined
): value is string {
  return typeof value === "string" && dateParts(value) !== null
}

export function calendarToday(now = new Date()) {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: CALENDAR_TIMEZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now)
  const values = Object.fromEntries(
    parts.map((part) => [part.type, part.value])
  )
  return `${values.year}-${values.month}-${values.day}`
}

export function millisecondsUntilNextCalendarMidnight(now = new Date()) {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: CALENDAR_TIMEZONE,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(now)
  const values = Object.fromEntries(
    parts.map((part) => [part.type, part.value])
  )
  const elapsed =
    (Number(values.hour) * 60 * 60 +
      Number(values.minute) * 60 +
      Number(values.second)) *
      1000 +
    now.getMilliseconds()

  return 24 * 60 * 60 * 1000 - elapsed
}

export function addCalendarDays(value: string, amount: number) {
  const parts = dateParts(value)
  if (!parts) throw new Error(`Invalid calendar date: ${value}`)

  const date = new Date(
    Date.UTC(parts.year, parts.month - 1, parts.day + amount, 12)
  )
  return dateFromParts(
    date.getUTCFullYear(),
    date.getUTCMonth() + 1,
    date.getUTCDate()
  )
}

export function startOfCalendarWeek(value: string) {
  const parts = dateParts(value)
  if (!parts) throw new Error(`Invalid calendar date: ${value}`)

  const date = new Date(Date.UTC(parts.year, parts.month - 1, parts.day, 12))
  const weekday = date.getUTCDay()
  return addCalendarDays(value, weekday === 0 ? -6 : 1 - weekday)
}

export function calendarWeek(value: string) {
  const start = startOfCalendarWeek(value)
  return Array.from({ length: 7 }, (_, index) => addCalendarDays(start, index))
}

export function calendarDateToLocalDate(value: string) {
  const parts = dateParts(value)
  if (!parts) return undefined
  return new Date(parts.year, parts.month - 1, parts.day, 12)
}

export function localDateToCalendarDate(value: Date) {
  return dateFromParts(
    value.getFullYear(),
    value.getMonth() + 1,
    value.getDate()
  )
}

export function parseCalendarSearchParams(
  searchParams: Pick<URLSearchParams, "get">,
  now = new Date()
): CalendarQueryState {
  const rawDate = searchParams.get("date")
  const rawKind = searchParams.get("kind")
  const rawImportance = searchParams.get("minImportance")
  const parsedImportance = Number(rawImportance)

  return {
    date: isCalendarDate(rawDate) ? rawDate : calendarToday(now),
    kind: CALENDAR_KINDS.includes(rawKind as CalendarKind)
      ? (rawKind as CalendarKind)
      : "",
    minImportance:
      parsedImportance === 1 || parsedImportance === 2 || parsedImportance === 3
        ? parsedImportance
        : "",
    keyword: (searchParams.get("q") || "").trim().slice(0, 100),
  }
}

export function calendarSearchParams(
  current: URLSearchParams,
  query: CalendarQueryState
) {
  const next = new URLSearchParams(current)
  next.set("date", query.date)

  if (query.kind) next.set("kind", query.kind)
  else next.delete("kind")

  if (query.minImportance) {
    next.set("minImportance", String(query.minImportance))
  } else {
    next.delete("minImportance")
  }

  if (query.keyword) next.set("q", query.keyword)
  else next.delete("q")

  next.delete("cursor")
  return next
}

export function mergeCalendarSearchState(
  current: CalendarSearchState,
  patch: Partial<CalendarQueryState>
): CalendarSearchState {
  const query = { ...current.query, ...patch }
  return {
    query,
    searchParams: calendarSearchParams(current.searchParams, query),
  }
}

export function formatCalendarDate(
  value: string,
  locale: string,
  options: Intl.DateTimeFormatOptions
) {
  const parts = dateParts(value)
  if (!parts) return value
  const date = new Date(Date.UTC(parts.year, parts.month - 1, parts.day, 12))
  return new Intl.DateTimeFormat(locale, {
    ...options,
    timeZone: "UTC",
  }).format(date)
}

export function formatCalendarWeekRange(value: string, locale: string) {
  const dates = calendarWeek(value)
  const start = calendarDateToUTCDate(dates[0])
  const end = calendarDateToUTCDate(dates[6])
  const formatter = new Intl.DateTimeFormat(locale, {
    timeZone: "UTC",
    year: "numeric",
    month: "short",
    day: "numeric",
  })
  return formatter.formatRange(start, end)
}

function calendarDateToUTCDate(value: string) {
  const parts = dateParts(value)
  if (!parts) throw new Error(`Invalid calendar date: ${value}`)
  return new Date(Date.UTC(parts.year, parts.month - 1, parts.day, 12))
}

export function formatBeijingTime(
  value: string | Date,
  locale: string,
  includeDate = false
) {
  const date = typeof value === "string" ? new Date(value) : value
  if (Number.isNaN(date.getTime())) return null

  return new Intl.DateTimeFormat(locale, {
    timeZone: CALENDAR_TIMEZONE,
    ...(includeDate ? { year: "numeric", month: "short", day: "numeric" } : {}),
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date)
}

export function safeHttpsUrl(value: string | null | undefined) {
  if (!value) return null

  try {
    const url = new URL(value)
    if (url.protocol !== "https:" || url.username || url.password) return null
    return url.toString()
  } catch {
    return null
  }
}
