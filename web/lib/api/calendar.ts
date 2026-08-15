import { ApiError, apiFetch, type ApiRequestOptions } from "./client"
import type { CalendarImportanceFilter, CalendarKind } from "@/lib/calendar"

const CALENDAR_CURSOR_STALE_STATUS = 409

export interface CalendarCounts {
  economic: number
  earnings: number
  corporate: number
  ipo: number
  total?: number
}

export interface CalendarEvent {
  kind: CalendarKind
  id: number
  key: string
  sourceId?: string
  fetchedAt?: string
  source?: string
  date: string
  dateTime?: string
  title: string
  country?: string
  indicator?: string
  importance?: number
  actual?: string | number
  forecast?: string | number
  previous?: string | number
  ticker?: string
  exchange?: string
  company?: string
  period?: string
  eventType?: string
  description?: string
  timezone?: string
  sourceUrl?: string
  priceLow?: string | number
  priceHigh?: string | number
  status?: string
  reportDate?: string
  eventDate?: string
  eventTime?: string
}

export interface CalendarEventsResponse {
  generatedAt: string
  dateFrom: string
  dateTo: string
  timezone: string
  counts: CalendarCounts
  total: number
  results: CalendarEvent[]
  cursor?: string
  hasMore: boolean
}

export interface CalendarEventQuery {
  dateFrom: string
  dateTo: string
  kind?: CalendarKind
  keyword?: string
  minImportance?: CalendarImportanceFilter
  cursor?: string
  limit?: number
}

export function isCalendarCursorStaleError(error: unknown): error is ApiError {
  return (
    error instanceof ApiError && error.status === CALENDAR_CURSOR_STALE_STATUS
  )
}

export function getCalendarEvents(
  query: CalendarEventQuery,
  options: Pick<ApiRequestOptions, "request" | "signal"> = {}
) {
  return apiFetch<CalendarEventsResponse>("/api/calendar/events", {
    ...options,
    params: {
      dateFrom: query.dateFrom,
      dateTo: query.dateTo,
      kind: query.kind,
      keyword: query.keyword,
      minImportance: query.minImportance,
      cursor: query.cursor,
      limit: query.limit,
    },
  })
}

export function getCalendarEvent(
  kind: CalendarKind,
  id: number,
  options: Pick<ApiRequestOptions, "signal"> = {}
) {
  return apiFetch<CalendarEvent>(
    `/api/calendar/events/${encodeURIComponent(kind)}/${id}`,
    options
  )
}
