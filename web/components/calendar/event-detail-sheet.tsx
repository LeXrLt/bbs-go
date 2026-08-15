"use client"

import {
  ExternalLinkIcon,
  LoaderCircleIcon,
  RefreshCwIcon,
  XIcon,
} from "lucide-react"

import { CalendarKindBadge } from "./event-list"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { useIsMobile } from "@/hooks/use-mobile"
import type { CalendarEvent } from "@/lib/api/calendar"
import {
  formatBeijingTime,
  formatCalendarDate,
  safeHttpsUrl,
} from "@/lib/calendar"
import type { Locale, TFunction } from "@/lib/i18n"

function hasValue(value: unknown): value is string | number {
  return value !== null && value !== undefined && value !== ""
}

function displayValue(value: unknown, t: TFunction) {
  return hasValue(value) ? String(value) : t("pages.calendar.unavailable")
}

function detailRows(event: CalendarEvent) {
  const common: Array<[string, unknown]> = [
    ["date", event.date],
    ["time", event.dateTime],
  ]

  if (event.kind === "economic") {
    return [
      ...common,
      ["country", event.country],
      ["indicator", event.indicator],
      ["importance", event.importance],
      ["actual", event.actual],
      ["forecast", event.forecast],
      ["previous", event.previous],
    ] as Array<[string, unknown]>
  }
  if (event.kind === "earnings") {
    return [
      ...common,
      ["ticker", event.ticker],
      ["company", event.company],
      ["exchange", event.exchange],
      ["period", event.period],
    ] as Array<[string, unknown]>
  }
  if (event.kind === "corporate") {
    return [
      ...common,
      ["ticker", event.ticker],
      ["company", event.company],
      ["eventType", event.eventType],
      ["timezone", event.timezone],
    ] as Array<[string, unknown]>
  }
  return [
    ...common,
    ["ticker", event.ticker],
    ["company", event.company],
    ["exchange", event.exchange],
    ["priceLow", event.priceLow],
    ["priceHigh", event.priceHigh],
    ["status", event.status],
  ] as Array<[string, unknown]>
}

function rowValue(
  field: string,
  value: unknown,
  event: CalendarEvent,
  locale: Locale,
  t: TFunction
) {
  if (field === "date") {
    return formatCalendarDate(event.date, locale, {
      year: "numeric",
      month: "long",
      day: "numeric",
      weekday: "long",
    })
  }
  if (field === "time") {
    return event.dateTime
      ? formatBeijingTime(event.dateTime, locale, true) ||
          t("pages.calendar.allDayTbd")
      : t("pages.calendar.allDayTbd")
  }
  if (field === "importance" && hasValue(value)) {
    return t("pages.calendar.importanceValue", { count: Number(value) })
  }
  return displayValue(value, t)
}

export function CalendarEventDetailSheet({
  event,
  loading,
  failed,
  locale,
  t,
  onOpenChange,
  onRetry,
}: {
  event: CalendarEvent | null
  loading: boolean
  failed: boolean
  locale: Locale
  t: TFunction
  onOpenChange: (open: boolean) => void
  onRetry: () => void
}) {
  const isMobile = useIsMobile()
  const sourceUrl = safeHttpsUrl(event?.sourceUrl)

  return (
    <Sheet open={event !== null} onOpenChange={onOpenChange}>
      <SheetContent
        side={isMobile ? "bottom" : "right"}
        showCloseButton={false}
        className={
          isMobile ? "max-h-[88dvh] rounded-t-lg" : "w-full sm:max-w-[34rem]"
        }
      >
        {event ? (
          <>
            <SheetHeader className="border-b border-border pr-14">
              <div className="mb-1 flex items-center gap-2">
                <CalendarKindBadge kind={event.kind} t={t} />
                {event.source ? (
                  <span className="truncate text-xs text-muted-foreground">
                    {event.source}
                  </span>
                ) : null}
              </div>
              <SheetTitle className="text-base leading-6 sm:text-lg">
                {event.title}
              </SheetTitle>
              <SheetDescription>
                {formatCalendarDate(event.date, locale, {
                  year: "numeric",
                  month: "long",
                  day: "numeric",
                })}
              </SheetDescription>
            </SheetHeader>

            <SheetClose asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="absolute top-4 right-4"
                aria-label={t("pages.calendar.closeDetails")}
              >
                <XIcon aria-hidden="true" />
              </Button>
            </SheetClose>

            <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-6">
              {loading ? (
                <div
                  className="flex items-center gap-2 border-b border-border py-3 text-xs text-muted-foreground"
                  role="status"
                >
                  <LoaderCircleIcon
                    className="size-4 animate-spin"
                    aria-hidden="true"
                  />
                  {t("pages.calendar.loadingDetails")}
                </div>
              ) : null}

              {failed ? (
                <div className="my-4 border-y border-destructive/30 py-4 text-sm text-destructive">
                  <p>{t("pages.calendar.detailLoadError")}</p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="mt-2 text-destructive"
                    onClick={onRetry}
                  >
                    <RefreshCwIcon aria-hidden="true" />
                    {t("pages.calendar.retry")}
                  </Button>
                </div>
              ) : null}

              <dl>
                {detailRows(event).map(([field, value]) => (
                  <div
                    key={field}
                    className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-3 border-b border-border py-3"
                  >
                    <dt className="text-xs font-medium text-muted-foreground">
                      {t(`pages.calendar.fields.${field}`)}
                    </dt>
                    <dd className="min-w-0 text-sm break-words text-foreground">
                      {rowValue(field, value, event, locale, t)}
                    </dd>
                  </div>
                ))}
              </dl>

              {event.description ? (
                <section className="border-b border-border py-4">
                  <h3 className="text-xs font-medium text-muted-foreground">
                    {t("pages.calendar.fields.description")}
                  </h3>
                  <p className="mt-2 text-sm leading-6 whitespace-pre-wrap text-foreground">
                    {event.description}
                  </p>
                </section>
              ) : null}

              <dl>
                {event.sourceId ? (
                  <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-3 border-b border-border py-3">
                    <dt className="text-xs font-medium text-muted-foreground">
                      {t("pages.calendar.fields.sourceId")}
                    </dt>
                    <dd className="font-mono text-xs break-all text-foreground">
                      {event.sourceId}
                    </dd>
                  </div>
                ) : null}
                {event.fetchedAt ? (
                  <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-3 border-b border-border py-3">
                    <dt className="text-xs font-medium text-muted-foreground">
                      {t("pages.calendar.fields.fetchedAt")}
                    </dt>
                    <dd className="text-sm text-foreground">
                      {formatBeijingTime(event.fetchedAt, locale, true) ||
                        t("pages.calendar.unavailable")}
                    </dd>
                  </div>
                ) : null}
              </dl>

              {sourceUrl ? (
                <Button
                  asChild
                  variant="outline"
                  className="mt-5 w-full justify-center"
                >
                  <a target="_blank" rel="noopener noreferrer" href={sourceUrl}>
                    {t("pages.calendar.openSource")}
                    <ExternalLinkIcon aria-hidden="true" />
                  </a>
                </Button>
              ) : null}
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
