"use client"

import * as React from "react"
import { CalendarX2Icon, CircleAlertIcon, RefreshCwIcon } from "lucide-react"
import { useNavigation, useRevalidator, useSearchParams } from "react-router"

import { CalendarDateNavigator } from "./date-navigator"
import { CalendarToolbar } from "./calendar-toolbar"
import { CalendarEventDetailSheet } from "./event-detail-sheet"
import { CalendarEventList } from "./event-list"
import { MainShell } from "@/components/layout/main-shell"
import { Button } from "@/components/ui/button"
import {
  getCalendarEvent,
  getCalendarEvents,
  isCalendarCursorStaleError,
  type CalendarEvent,
  type CalendarEventsResponse,
} from "@/lib/api/calendar"
import {
  CALENDAR_PAGE_SIZE,
  calendarSearchParams,
  formatBeijingTime,
  mergeCalendarSearchState,
  parseCalendarSearchParams,
  type CalendarSearchState,
  type CalendarQueryState,
} from "@/lib/calendar"
import { useI18n } from "@/lib/i18n/provider"

export interface CalendarRouteData {
  page: CalendarEventsResponse
  query: CalendarQueryState
  failed: boolean
}

const EMPTY_CALENDAR_COUNTS: CalendarEventsResponse["counts"] = {
  economic: 0,
  earnings: 0,
  corporate: 0,
  ipo: 0,
  total: 0,
}

function queryKey(query: CalendarQueryState) {
  return [query.date, query.kind, query.minImportance, query.keyword].join("|")
}

function CalendarHeader({
  generatedAt,
  loading,
}: {
  generatedAt: string
  loading: boolean
}) {
  const { locale, t } = useI18n()
  const [now, setNow] = React.useState(() => new Date())

  React.useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 30_000)
    return () => window.clearInterval(timer)
  }, [])

  const beijingTime = formatBeijingTime(now, locale)
  const updatedAt = generatedAt
    ? formatBeijingTime(generatedAt, locale, true)
    : null

  return (
    <header className="flex min-h-20 flex-col justify-center gap-2 border-b border-border bg-background px-3 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5">
      <div>
        <h1 className="text-xl font-semibold text-foreground sm:text-2xl">
          {t("pages.calendar.title")}
        </h1>
      </div>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground sm:justify-end">
        <span suppressHydrationWarning>
          {t("pages.calendar.beijingTime")}: {beijingTime}
        </span>
        <span aria-hidden="true">/</span>
        <span>
          {t("pages.calendar.updatedAt")}:{" "}
          {updatedAt || t("pages.calendar.unavailable")}
        </span>
        {loading ? (
          <RefreshCwIcon className="size-3.5 animate-spin" aria-hidden="true" />
        ) : null}
      </div>
    </header>
  )
}

function CalendarError({
  t,
  onRetry,
}: {
  t: ReturnType<typeof useI18n>["t"]
  onRetry: () => void
}) {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center px-5 py-12 text-center">
      <CircleAlertIcon className="size-8 text-destructive" aria-hidden="true" />
      <h2 className="mt-3 text-sm font-semibold text-foreground">
        {t("pages.calendar.loadError")}
      </h2>
      <p className="mt-1 max-w-md text-sm text-muted-foreground">
        {t("pages.calendar.loadErrorDescription")}
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="mt-4"
        onClick={onRetry}
      >
        <RefreshCwIcon aria-hidden="true" />
        {t("pages.calendar.retry")}
      </Button>
    </div>
  )
}

function CalendarEmpty({
  filtered,
  t,
}: {
  filtered: boolean
  t: ReturnType<typeof useI18n>["t"]
}) {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center px-5 py-12 text-center">
      <CalendarX2Icon
        className="size-8 text-muted-foreground"
        aria-hidden="true"
      />
      <h2 className="mt-3 text-sm font-semibold text-foreground">
        {t("pages.calendar.noEvents")}
      </h2>
      <p className="mt-1 max-w-md text-sm text-muted-foreground">
        {t(
          filtered
            ? "pages.calendar.noEventsFilteredDescription"
            : "pages.calendar.noEventsDescription"
        )}
      </p>
    </div>
  )
}

function CalendarPending({
  events,
  locale,
  total,
  t,
}: {
  events: CalendarEvent[]
  locale: ReturnType<typeof useI18n>["locale"]
  total: number
  t: ReturnType<typeof useI18n>["t"]
}) {
  return (
    <div className="relative min-h-64">
      {events.length ? (
        <div
          aria-hidden="true"
          inert
          className="pointer-events-none invisible select-none"
        >
          <CalendarEventList
            events={events}
            locale={locale}
            t={t}
            loadedTotal={events.length}
            total={total}
            hasMore={false}
            loadingMore={false}
            loadMoreError={false}
            onOpen={() => undefined}
            onLoadMore={() => undefined}
          />
        </div>
      ) : null}
      <div
        className="absolute inset-0 flex items-center justify-center"
        role="status"
        aria-live="polite"
      >
        <RefreshCwIcon
          className="size-5 animate-spin text-muted-foreground"
          aria-hidden="true"
        />
        <span className="sr-only">{t("pages.calendar.loading")}</span>
      </div>
    </div>
  )
}

export function CalendarWorkbench({ data }: { data: CalendarRouteData }) {
  const { locale, t } = useI18n()
  const [searchParams, setSearchParams] = useSearchParams()
  const navigation = useNavigation()
  const revalidator = useRevalidator()
  const urlQuery = React.useMemo(
    () => parseCalendarSearchParams(searchParams),
    [searchParams]
  )
  const urlSearch = searchParams.toString()
  const desiredStateRef = React.useRef<CalendarSearchState>({
    query: urlQuery,
    searchParams: calendarSearchParams(searchParams, urlQuery),
  })
  const [query, setQuery] = React.useState(desiredStateRef.current.query)
  const acceptedUrlSearchRef = React.useRef(urlSearch)
  const requestedUrlSearchRef = React.useRef<string | null>(null)
  const requestedNavigationObservedRef = React.useRef(false)
  const currentQueryKey = queryKey(query)
  const dataQueryKey = queryKey(data.query)
  const dataMatchesQuery = dataQueryKey === currentQueryKey

  const [events, setEvents] = React.useState(data.page.results || [])
  const [eventsQueryKey, setEventsQueryKey] = React.useState(dataQueryKey)
  const [cursor, setCursor] = React.useState(data.page.cursor || "")
  const [hasMore, setHasMore] = React.useState(Boolean(data.page.hasMore))
  const [total, setTotal] = React.useState(data.page.total || 0)
  const [loadingMore, setLoadingMore] = React.useState(false)
  const [loadMoreFailed, setLoadMoreFailed] = React.useState(false)
  const loadMoreController = React.useRef<AbortController | null>(null)
  const queryKeyRef = React.useRef(currentQueryKey)

  const [selectedEvent, setSelectedEvent] =
    React.useState<CalendarEvent | null>(null)
  const [detailEvent, setDetailEvent] = React.useState<CalendarEvent | null>(
    null
  )
  const [detailLoading, setDetailLoading] = React.useState(false)
  const [detailFailed, setDetailFailed] = React.useState(false)
  const detailController = React.useRef<AbortController | null>(null)

  const resultsAreCurrent =
    dataMatchesQuery && eventsQueryKey === currentQueryKey
  const loading =
    navigation.state !== "idle" ||
    revalidator.state !== "idle" ||
    !resultsAreCurrent

  React.useEffect(() => {
    const desiredSearch = desiredStateRef.current.searchParams.toString()
    const canonical = calendarSearchParams(searchParams, urlQuery)
    const canonicalSearch = canonical.toString()

    if (navigation.state !== "idle" && requestedUrlSearchRef.current !== null) {
      requestedNavigationObservedRef.current = true
    }

    if (urlSearch === desiredSearch) {
      acceptedUrlSearchRef.current = urlSearch
      requestedUrlSearchRef.current = null
      requestedNavigationObservedRef.current = false
      return
    }

    const committedUrlChanged = acceptedUrlSearchRef.current !== urlSearch
    const requestedNavigationEnded =
      navigation.state === "idle" &&
      requestedUrlSearchRef.current !== null &&
      requestedNavigationObservedRef.current

    if (
      navigation.state === "idle" &&
      (committedUrlChanged || requestedNavigationEnded)
    ) {
      const next = { query: urlQuery, searchParams: canonical }
      desiredStateRef.current = next
      acceptedUrlSearchRef.current = urlSearch
      requestedUrlSearchRef.current = null
      requestedNavigationObservedRef.current = false
      setQuery(urlQuery)

      if (canonicalSearch !== urlSearch) {
        requestedUrlSearchRef.current = canonicalSearch
        setSearchParams(canonical, {
          replace: true,
          preventScrollReset: true,
        })
      }
      return
    }

    if (
      navigation.state === "idle" &&
      requestedUrlSearchRef.current === null &&
      canonicalSearch !== urlSearch
    ) {
      requestedUrlSearchRef.current = canonicalSearch
      requestedNavigationObservedRef.current = false
      setSearchParams(canonical, {
        replace: true,
        preventScrollReset: true,
      })
    }
  }, [navigation.state, searchParams, setSearchParams, urlQuery, urlSearch])

  React.useEffect(() => {
    queryKeyRef.current = currentQueryKey
    loadMoreController.current?.abort()
    setLoadingMore(false)
    setLoadMoreFailed(false)

    if (!dataMatchesQuery) {
      setCursor("")
      setHasMore(false)
      return
    }

    setEvents(data.page.results || [])
    setEventsQueryKey(dataQueryKey)
    setCursor(data.page.cursor || "")
    setHasMore(Boolean(data.page.hasMore))
    setTotal(data.page.total || 0)
  }, [currentQueryKey, data, dataMatchesQuery, dataQueryKey])

  React.useEffect(() => {
    detailController.current?.abort()
    setSelectedEvent(null)
    setDetailEvent(null)
    setDetailLoading(false)
    setDetailFailed(false)
  }, [currentQueryKey])

  React.useEffect(
    () => () => {
      loadMoreController.current?.abort()
      detailController.current?.abort()
    },
    []
  )

  const updateQuery = React.useCallback(
    (patch: Partial<CalendarQueryState>, replace = false) => {
      const next = mergeCalendarSearchState(desiredStateRef.current, patch)
      desiredStateRef.current = next
      requestedUrlSearchRef.current = next.searchParams.toString()
      requestedNavigationObservedRef.current = false
      setQuery(next.query)
      setSearchParams(next.searchParams, {
        replace,
        preventScrollReset: true,
      })
    },
    [setSearchParams]
  )

  const loadDetail = React.useCallback(async (event: CalendarEvent) => {
    detailController.current?.abort()
    const controller = new AbortController()
    detailController.current = controller
    setDetailEvent(event)
    setDetailLoading(true)
    setDetailFailed(false)

    try {
      const detail = await getCalendarEvent(event.kind, event.id, {
        signal: controller.signal,
      })
      if (!controller.signal.aborted) setDetailEvent(detail)
    } catch {
      if (!controller.signal.aborted) setDetailFailed(true)
    } finally {
      if (!controller.signal.aborted) setDetailLoading(false)
    }
  }, [])

  const openEvent = React.useCallback(
    (event: CalendarEvent) => {
      setSelectedEvent(event)
      void loadDetail(event)
    },
    [loadDetail]
  )

  const closeDetails = React.useCallback((open: boolean) => {
    if (open) return
    detailController.current?.abort()
    setSelectedEvent(null)
    setDetailEvent(null)
    setDetailLoading(false)
    setDetailFailed(false)
  }, [])

  const loadMore = React.useCallback(async () => {
    if (loadingMore || !hasMore || !resultsAreCurrent) return
    loadMoreController.current?.abort()
    const controller = new AbortController()
    loadMoreController.current = controller
    const requestKey = currentQueryKey
    setLoadingMore(true)
    setLoadMoreFailed(false)

    try {
      const page = await getCalendarEvents(
        {
          dateFrom: query.date,
          dateTo: query.date,
          kind: query.kind || undefined,
          keyword: query.keyword || undefined,
          minImportance: query.minImportance || undefined,
          cursor: cursor || undefined,
          limit: CALENDAR_PAGE_SIZE,
        },
        { signal: controller.signal }
      )
      if (controller.signal.aborted || queryKeyRef.current !== requestKey)
        return

      setEvents((current) => {
        const seen = new Set(current.map((event) => event.key))
        const fresh = (page.results || []).filter(
          (event) => !seen.has(event.key)
        )
        return [...current, ...fresh]
      })
      setCursor(page.cursor || "")
      setHasMore(Boolean(page.hasMore))
      setTotal(page.total || 0)
    } catch (error) {
      if (controller.signal.aborted || queryKeyRef.current !== requestKey)
        return

      if (isCalendarCursorStaleError(error)) {
        setCursor("")
        setHasMore(false)
        setLoadMoreFailed(false)
        revalidator.revalidate()
        return
      }
      setLoadMoreFailed(true)
    } finally {
      if (!controller.signal.aborted && queryKeyRef.current === requestKey) {
        setLoadingMore(false)
      }
    }
  }, [
    currentQueryKey,
    cursor,
    hasMore,
    loadingMore,
    query,
    resultsAreCurrent,
    revalidator,
  ])

  const filtered = Boolean(query.kind || query.minImportance || query.keyword)

  return (
    <MainShell containerClassName="!max-w-none">
      <section className="min-h-[calc(100vh-5rem)] bg-background">
        <CalendarHeader
          generatedAt={dataMatchesQuery ? data.page.generatedAt : ""}
          loading={loading}
        />
        <CalendarDateNavigator
          date={query.date}
          locale={locale}
          t={t}
          onDateChange={(date) => updateQuery({ date })}
        />
        <CalendarToolbar
          query={query}
          counts={dataMatchesQuery ? data.page.counts : EMPTY_CALENDAR_COUNTS}
          loading={loading}
          t={t}
          onQueryChange={updateQuery}
          onRefresh={() => revalidator.revalidate()}
        />

        <div aria-busy={loading} className={loading ? "opacity-70" : undefined}>
          {dataMatchesQuery && data.failed ? (
            <CalendarError t={t} onRetry={() => revalidator.revalidate()} />
          ) : !resultsAreCurrent ? (
            <CalendarPending
              events={events}
              locale={locale}
              total={total}
              t={t}
            />
          ) : events.length === 0 ? (
            <CalendarEmpty filtered={filtered} t={t} />
          ) : (
            <CalendarEventList
              events={events}
              locale={locale}
              t={t}
              loadedTotal={events.length}
              total={total}
              hasMore={hasMore}
              loadingMore={loadingMore}
              loadMoreError={loadMoreFailed}
              onOpen={openEvent}
              onLoadMore={() => void loadMore()}
            />
          )}
        </div>
      </section>

      <CalendarEventDetailSheet
        event={resultsAreCurrent ? detailEvent || selectedEvent : null}
        loading={detailLoading}
        failed={detailFailed}
        locale={locale}
        t={t}
        onOpenChange={closeDetails}
        onRetry={() => {
          if (selectedEvent) void loadDetail(selectedEvent)
        }}
      />
    </MainShell>
  )
}
