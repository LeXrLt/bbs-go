"use client"

import * as React from "react"
import {
  ListFilterIcon,
  RefreshCwIcon,
  SearchIcon,
  SlidersHorizontalIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { CalendarCounts } from "@/lib/api/calendar"
import {
  CALENDAR_KINDS,
  type CalendarImportanceFilter,
  type CalendarKind,
  type CalendarQueryState,
} from "@/lib/calendar"
import type { TFunction } from "@/lib/i18n"
import { cn } from "@/lib/utils"

const KIND_VALUES = ["", ...CALENDAR_KINDS] as const

function countForKind(counts: CalendarCounts, kind: CalendarKind | "") {
  if (kind) return counts[kind]
  return (
    counts.total ??
    counts.economic + counts.earnings + counts.corporate + counts.ipo
  )
}

function ImportanceSelect({
  value,
  t,
  className,
  onChange,
}: {
  value: CalendarImportanceFilter
  t: TFunction
  className?: string
  onChange: (value: CalendarImportanceFilter) => void
}) {
  return (
    <Select
      value={value ? String(value) : "all"}
      onValueChange={(next) => {
        const parsed = Number(next)
        onChange(parsed === 1 || parsed === 2 || parsed === 3 ? parsed : "")
      }}
    >
      <SelectTrigger
        className={cn("w-full sm:w-44", className)}
        aria-label={t("pages.calendar.importance")}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t("pages.calendar.importanceAll")}</SelectItem>
        {[1, 2, 3].map((importance) => (
          <SelectItem key={importance} value={String(importance)}>
            {t("pages.calendar.importanceAtLeast", { count: importance })}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function SearchField({
  value,
  t,
  onChange,
  onSubmit,
}: {
  value: string
  t: TFunction
  onChange: (value: string) => void
  onSubmit: () => void
}) {
  return (
    <form
      className="relative min-w-0 flex-1 sm:w-64 sm:flex-none"
      role="search"
      onSubmit={(event) => {
        event.preventDefault()
        onSubmit()
      }}
    >
      <label htmlFor="calendar-search" className="sr-only">
        {t("pages.calendar.searchLabel")}
      </label>
      <SearchIcon
        className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
        aria-hidden="true"
      />
      <Input
        id="calendar-search"
        type="search"
        value={value}
        maxLength={100}
        autoComplete="off"
        className="pl-8"
        placeholder={t("pages.calendar.searchPlaceholder")}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape") onChange("")
        }}
      />
    </form>
  )
}

export function CalendarToolbar({
  query,
  counts,
  loading,
  t,
  onQueryChange,
  onRefresh,
}: {
  query: CalendarQueryState
  counts: CalendarCounts
  loading: boolean
  t: TFunction
  onQueryChange: (patch: Partial<CalendarQueryState>, replace?: boolean) => void
  onRefresh: () => void
}) {
  const [searchValue, setSearchValue] = React.useState(query.keyword)
  const [filterOpen, setFilterOpen] = React.useState(false)

  React.useEffect(() => setSearchValue(query.keyword), [query.keyword])

  React.useEffect(() => {
    const keyword = searchValue.trim()
    if (keyword === query.keyword) return
    const timer = window.setTimeout(() => onQueryChange({ keyword }, true), 350)
    return () => window.clearTimeout(timer)
  }, [onQueryChange, query.keyword, searchValue])

  const submitSearch = React.useCallback(() => {
    onQueryChange({ keyword: searchValue.trim() }, true)
  }, [onQueryChange, searchValue])

  return (
    <div className="sticky top-14 z-30 border-y border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85">
      <div className="flex min-w-0 flex-col gap-2 px-3 py-2 sm:px-5 lg:flex-row lg:items-center lg:justify-between">
        <Tabs
          value={query.kind || "all"}
          onValueChange={(value) =>
            onQueryChange({
              kind: value === "all" ? "" : (value as CalendarKind),
            })
          }
          className="min-w-0"
        >
          <TabsList
            variant="line"
            aria-label={t("pages.calendar.kindFilter")}
            className="h-9 max-w-full justify-start overflow-x-auto overflow-y-hidden"
          >
            {KIND_VALUES.map((kind) => {
              const value = kind || "all"
              return (
                <TabsTrigger key={value} value={value} className="px-2.5">
                  <span>{t(`pages.calendar.kinds.${value}`)}</span>
                  <span className="min-w-5 rounded-sm bg-muted px-1 text-center text-[11px] text-muted-foreground tabular-nums">
                    {countForKind(counts, kind)}
                  </span>
                </TabsTrigger>
              )
            })}
          </TabsList>
        </Tabs>

        <div className="flex min-w-0 items-center gap-2">
          <div className="hidden md:block">
            <ImportanceSelect
              value={query.minImportance}
              t={t}
              onChange={(minImportance) => onQueryChange({ minImportance })}
            />
          </div>

          <SearchField
            value={searchValue}
            t={t}
            onChange={setSearchValue}
            onSubmit={submitSearch}
          />

          <Sheet open={filterOpen} onOpenChange={setFilterOpen}>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="relative md:hidden"
              aria-label={t("pages.calendar.filters")}
              onClick={() => setFilterOpen(true)}
            >
              <ListFilterIcon aria-hidden="true" />
              {query.minImportance ? (
                <span className="absolute top-1 right-1 size-1.5 rounded-full bg-destructive" />
              ) : null}
            </Button>
            <SheetContent side="bottom" className="rounded-t-lg pb-6">
              <SheetHeader>
                <SheetTitle className="flex items-center gap-2">
                  <SlidersHorizontalIcon
                    className="size-4"
                    aria-hidden="true"
                  />
                  {t("pages.calendar.filters")}
                </SheetTitle>
                <SheetDescription>
                  {t("pages.calendar.filterDescription")}
                </SheetDescription>
              </SheetHeader>
              <div className="px-4">
                <ImportanceSelect
                  value={query.minImportance}
                  t={t}
                  onChange={(minImportance) => {
                    onQueryChange({ minImportance })
                    setFilterOpen(false)
                  }}
                />
              </div>
            </SheetContent>
          </Sheet>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="icon"
                disabled={loading}
                aria-label={t("pages.calendar.refresh")}
                onClick={onRefresh}
              >
                <RefreshCwIcon
                  className={cn(loading && "animate-spin")}
                  aria-hidden="true"
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("pages.calendar.refresh")}</TooltipContent>
          </Tooltip>
        </div>
      </div>
      {loading ? (
        <div
          className="absolute inset-x-0 bottom-0 h-0.5 overflow-hidden bg-muted"
          role="progressbar"
          aria-label={t("pages.calendar.loading")}
        >
          <div className="h-full w-1/3 animate-pulse bg-foreground" />
        </div>
      ) : null}
    </div>
  )
}
