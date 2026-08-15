"use client"

import { ChevronRightIcon, CircleDollarSignIcon, StarIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { CalendarEvent } from "@/lib/api/calendar"
import { formatBeijingTime, type CalendarKind } from "@/lib/calendar"
import type { Locale, TFunction } from "@/lib/i18n"
import { cn } from "@/lib/utils"

const KIND_CLASSES: Record<CalendarKind, string> = {
  economic:
    "border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-300",
  earnings:
    "border-sky-300 bg-sky-50 text-sky-800 dark:border-sky-800 dark:bg-sky-950/40 dark:text-sky-300",
  corporate:
    "border-rose-300 bg-rose-50 text-rose-800 dark:border-rose-800 dark:bg-rose-950/40 dark:text-rose-300",
  ipo: "border-emerald-300 bg-emerald-50 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300",
}

function hasValue(value: unknown): value is string | number {
  return value !== null && value !== undefined && value !== ""
}

function displayValue(value: unknown, t: TFunction) {
  return hasValue(value) ? String(value) : t("pages.calendar.unavailable")
}

function priceRange(event: CalendarEvent, t: TFunction) {
  const low = hasValue(event.priceLow) ? String(event.priceLow) : ""
  const high = hasValue(event.priceHigh) ? String(event.priceHigh) : ""
  if (low && high) return `${low} - ${high}`
  return low || high || t("pages.calendar.unavailable")
}

export function CalendarKindBadge({
  kind,
  t,
}: {
  kind: CalendarKind
  t: TFunction
}) {
  return (
    <span
      className={cn(
        "inline-flex h-5 shrink-0 items-center rounded-sm border px-1.5 text-[11px] font-semibold whitespace-nowrap",
        KIND_CLASSES[kind]
      )}
    >
      {t(`pages.calendar.kinds.${kind}`)}
    </span>
  )
}

function EventImportance({
  importance,
  t,
}: {
  importance?: number
  t: TFunction
}) {
  if (!importance) return null
  const normalized = Math.max(1, Math.min(3, Math.round(importance)))

  return (
    <span
      className="inline-flex items-center gap-0.5 text-amber-600 dark:text-amber-400"
      aria-label={t("pages.calendar.importanceValue", { count: normalized })}
    >
      {Array.from({ length: normalized }, (_, index) => (
        <StarIcon
          key={index}
          className="size-3 fill-current"
          aria-hidden="true"
        />
      ))}
    </span>
  )
}

function EventIdentity({ event }: { event: CalendarEvent }) {
  const values =
    event.kind === "economic"
      ? [event.country, event.indicator]
      : [event.ticker, event.exchange, event.company]
  const identity = values.filter(Boolean).filter((value, index, all) => {
    return value !== event.title && all.indexOf(value) === index
  })

  return identity.length ? identity.join(" / ") : null
}

function EconomicMetrics({ event, t }: { event: CalendarEvent; t: TFunction }) {
  return (
    <div className="grid grid-cols-3 gap-3">
      {[
        ["actual", event.actual],
        ["forecast", event.forecast],
        ["previous", event.previous],
      ].map(([label, value]) => (
        <div key={String(label)} className="min-w-0">
          <span className="block text-[10px] font-medium text-muted-foreground uppercase">
            {t(`pages.calendar.fields.${label}`)}
          </span>
          <span className="block truncate text-xs font-semibold text-foreground tabular-nums">
            {displayValue(value, t)}
          </span>
        </div>
      ))}
    </div>
  )
}

function EventKeyData({ event, t }: { event: CalendarEvent; t: TFunction }) {
  if (event.kind === "economic") return <EconomicMetrics event={event} t={t} />

  if (event.kind === "earnings") {
    return (
      <div className="truncate text-xs">
        <span className="font-medium text-foreground">
          {displayValue(event.period, t)}
        </span>
        {event.exchange ? (
          <span className="text-muted-foreground"> / {event.exchange}</span>
        ) : null}
      </div>
    )
  }

  if (event.kind === "corporate") {
    return (
      <div className="min-w-0 text-xs">
        <div className="truncate font-medium text-foreground">
          {displayValue(event.eventType, t)}
        </div>
        {event.description ? (
          <div className="truncate text-muted-foreground">
            {event.description}
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <div className="min-w-0 text-xs">
      <div className="flex items-center gap-1 truncate font-medium text-foreground">
        <CircleDollarSignIcon
          className="size-3.5 shrink-0"
          aria-hidden="true"
        />
        <span className="truncate">{priceRange(event, t)}</span>
      </div>
      {event.status ? (
        <div className="truncate text-muted-foreground">{event.status}</div>
      ) : null}
    </div>
  )
}

function mobileSummary(event: CalendarEvent, t: TFunction) {
  if (event.kind === "economic") {
    return [
      `${t("pages.calendar.fields.actual")}: ${displayValue(event.actual, t)}`,
      `${t("pages.calendar.fields.forecast")}: ${displayValue(event.forecast, t)}`,
      `${t("pages.calendar.fields.previous")}: ${displayValue(event.previous, t)}`,
    ].join(" / ")
  }
  if (event.kind === "earnings") {
    return [event.period, event.exchange].filter(Boolean).join(" / ")
  }
  if (event.kind === "corporate") {
    return [event.eventType, event.description].filter(Boolean).join(" / ")
  }
  return [priceRange(event, t), event.status].filter(Boolean).join(" / ")
}

function CalendarEventRow({
  event,
  locale,
  t,
  onOpen,
}: {
  event: CalendarEvent
  locale: Locale
  t: TFunction
  onOpen: (event: CalendarEvent) => void
}) {
  const time = event.dateTime ? formatBeijingTime(event.dateTime, locale) : null
  const identity = EventIdentity({ event })

  return (
    <button
      type="button"
      className="grid min-h-16 w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 border-b border-border px-3 py-2 text-left transition-colors outline-none last:border-b-0 hover:bg-muted/50 focus-visible:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset sm:px-5 md:grid-cols-[7rem_7rem_minmax(0,1fr)_minmax(14rem,24rem)_1.5rem] md:gap-x-4"
      aria-label={t("pages.calendar.openEvent", { title: event.title })}
      onClick={() => onOpen(event)}
    >
      <div className="hidden md:block">
        <CalendarKindBadge kind={event.kind} t={t} />
      </div>
      <div className="hidden text-xs font-medium text-foreground tabular-nums md:block">
        {time || t("pages.calendar.allDayTbd")}
      </div>
      <div className="hidden min-w-0 md:block">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-semibold text-foreground">
            {event.title}
          </span>
          <EventImportance importance={event.importance} t={t} />
        </div>
        {identity ? (
          <div className="mt-0.5 truncate text-xs text-muted-foreground">
            {identity}
          </div>
        ) : null}
      </div>
      <div className="hidden min-w-0 md:block">
        <EventKeyData event={event} t={t} />
      </div>
      <ChevronRightIcon
        className="hidden size-4 text-muted-foreground md:block"
        aria-hidden="true"
      />

      <div className="min-w-0 md:hidden">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-semibold text-foreground">
            {event.title}
          </span>
          <EventImportance importance={event.importance} t={t} />
        </div>
        <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <span className="shrink-0 font-medium text-foreground tabular-nums">
            {time || t("pages.calendar.allDayTbd")}
          </span>
          <span aria-hidden="true">/</span>
          <span className="truncate">{mobileSummary(event, t)}</span>
        </div>
      </div>
      <div className="md:hidden">
        <CalendarKindBadge kind={event.kind} t={t} />
      </div>
    </button>
  )
}

export function CalendarEventList({
  events,
  locale,
  t,
  loadedTotal,
  total,
  hasMore,
  loadingMore,
  loadMoreError,
  onOpen,
  onLoadMore,
}: {
  events: CalendarEvent[]
  locale: Locale
  t: TFunction
  loadedTotal: number
  total: number
  hasMore: boolean
  loadingMore: boolean
  loadMoreError: boolean
  onOpen: (event: CalendarEvent) => void
  onLoadMore: () => void
}) {
  return (
    <div aria-label={t("pages.calendar.eventList")}>
      <div className="hidden min-h-9 grid-cols-[7rem_7rem_minmax(0,1fr)_minmax(14rem,24rem)_1.5rem] items-center gap-x-4 border-b border-border bg-muted/40 px-5 text-[11px] font-semibold text-muted-foreground uppercase md:grid">
        <span>{t("pages.calendar.columns.type")}</span>
        <span>{t("pages.calendar.columns.time")}</span>
        <span>{t("pages.calendar.columns.event")}</span>
        <span>{t("pages.calendar.columns.keyData")}</span>
        <span className="sr-only">{t("pages.calendar.columns.action")}</span>
      </div>

      <div role="list">
        {events.map((event) => (
          <div key={event.key} role="listitem">
            <CalendarEventRow
              event={event}
              locale={locale}
              t={t}
              onOpen={onOpen}
            />
          </div>
        ))}
      </div>

      <div className="flex min-h-16 flex-col items-center justify-center gap-1 border-t border-border px-4 py-3">
        <p className="text-xs text-muted-foreground" aria-live="polite">
          {t("pages.calendar.loadedCount", {
            loaded: loadedTotal,
            total,
          })}
        </p>
        {hasMore ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={loadingMore}
            onClick={onLoadMore}
          >
            {loadingMore
              ? t("pages.calendar.loadingMore")
              : loadMoreError
                ? t("pages.calendar.retryLoadMore")
                : t("pages.calendar.loadMore")}
          </Button>
        ) : events.length ? (
          <span className="text-xs text-muted-foreground">
            {t("pages.calendar.noMore")}
          </span>
        ) : null}
      </div>
    </div>
  )
}
