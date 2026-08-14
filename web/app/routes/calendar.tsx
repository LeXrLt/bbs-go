"use client"

import { localizedTitle, pageMeta, rootDataFromMatches } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"
import { MainShell } from "@/components/layout/main-shell"
import { cn } from "@/lib/utils"
import dynamic from "@/lib/router/dynamic"
import * as React from "react"
import {
  CalendarIcon,
  ClockIcon,
  RefreshCwIcon,
} from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useTheme } from "@/components/theme-provider"

import "md-editor-rt/lib/style.css"

const MdPreview = dynamic(
  () => import("md-editor-rt").then((mod) => mod.MdPreview),
  { ssr: false }
) as React.ComponentType<{ modelValue: string; theme?: string }>

// ── types ──
interface CalendarEvent {
  id: number
  source_id: string
  event_date?: string
  event_time?: string
  report_date?: string
  ticker?: string
  company?: string
  indicator?: string
  event_type?: string
  title?: string
  description?: string
  country?: string
  importance?: number
  post?: {
    title: string
    content_md: string
    category: string
    tags: string[]
  }
  _kind: string
  _date: string
}

interface FeedResponse {
  generated_at: string
  economic: any[]
  earnings: any[]
  corporate: any[]
  ipo: any[]
}

interface EventDetailResponse {
  kind: string
  id: number
  post?: {
    title: string
    content_md: string
    category: string
    tags: string[]
  }
  [key: string]: any
}

// ── constants ──
const KIND_LABELS: Record<string, string> = {
  economic: "宏观",
  earnings: "财报",
  corporate: "公司事件",
  ipo: "IPO",
}

const KIND_BADGE_CLASSES: Record<string, string> = {
  economic:
    "bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-400",
  earnings:
    "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-400",
  corporate:
    "bg-purple-100 text-purple-800 dark:bg-purple-900/40 dark:text-purple-400",
  ipo: "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-400",
}

type Module = "upcoming" | "past"

// ── helpers ──
function getEventDate(ev: any): string | null {
  const raw = ev.event_time || ev.report_date || ev.event_date
  return raw ? raw.slice(0, 10) : null
}

const STORAGE_KEY_API_URL = "fc_api_url"

// ── component ──
export function meta({
  location,
  matches,
}: {
  location: { pathname: string }
  matches: Array<{ data?: unknown; loaderData?: unknown }>
}) {
  const rootData = rootDataFromMatches(matches)
  return pageMeta(
    rootData?.config,
    localizedTitle(rootData?.locale, "Calendar", "投研日历"),
    { canonicalPath: location.pathname }
  )
}

export default function CalendarRoute() {
  useDocumentTitle("投研日历")
  const { resolvedTheme } = useTheme()

  // ── state ──
  const [module, setModule] = React.useState<Module>("upcoming")
  const [events, setEvents] = React.useState<CalendarEvent[]>([])
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [apiUrl, setApiUrl] = React.useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem(STORAGE_KEY_API_URL) || "http://localhost:8088"
    }
    return "http://localhost:8088"
  })
  const [kindFilter, setKindFilter] = React.useState("")
  const [hasMore, setHasMore] = React.useState(true)
  const [page, setPage] = React.useState(0)
  const PAGE_SIZE = 50

  // ── detail modal state ──
  const [selectedEvent, setSelectedEvent] = React.useState<CalendarEvent | null>(null)
  const [detail, setDetail] = React.useState<EventDetailResponse | null>(null)
  const [detailLoading, setDetailLoading] = React.useState(false)
  const [detailError, setDetailError] = React.useState<string | null>(null)

  // ── derived ──
  const today = React.useMemo(() => {
    const d = new Date()
    return d.toISOString().slice(0, 10)
  }, [])

  const filteredEvents = React.useMemo(() => {
    let filtered = events

    // filter by kind
    if (kindFilter) {
      filtered = filtered.filter((ev) => ev._kind === kindFilter)
    }

    // filter by module
    if (module === "upcoming") {
      filtered = filtered
        .filter((ev) => ev._date && ev._date >= today)
        .sort((a, b) => (a._date < b._date ? -1 : a._date > b._date ? 1 : 0))
    } else {
      filtered = filtered
        .filter((ev) => ev._date && ev._date < today)
        .sort((a, b) => (a._date > b._date ? -1 : a._date < b._date ? 1 : 0))
    }

    return filtered
  }, [events, kindFilter, module, today])

  // ── fetch ──
  const fetchEvents = React.useCallback(
    async (reset = false) => {
      setLoading(true)
      setError(null)
      if (reset) {
        setEvents([])
        setPage(0)
        setHasMore(true)
      }

      try {
        const params = new URLSearchParams()
        if (kindFilter) params.set("kind", kindFilter)

        const base = apiUrl.replace(/\/+$/, "")
        const url = `${base}/api/feed?${params.toString()}`
        const res = await fetch(url)
        if (!res.ok) throw new Error(`API ${res.status}`)
        const data: FeedResponse = await res.json()

        const kinds = ["economic", "earnings", "corporate", "ipo"] as const
        const flat: CalendarEvent[] = []
        for (const k of kinds) {
          for (const ev of data[k] || []) {
            flat.push({
              ...ev,
              _kind: k as string,
              _date: getEventDate(ev) || "",
            })
          }
        }

        if (reset) {
          setEvents(flat)
        } else {
          setEvents((prev) => {
            const seen = new Set(prev.map((e) => `${e._kind}:${e.source_id}`))
            const fresh = flat.filter(
              (e) => !seen.has(`${e._kind}:${e.source_id}`)
            )
            return [...prev, ...fresh]
          })
        }

        setHasMore(false) // FC returns all events at once, no pagination
        setPage((p) => p + 1)
      } catch (err: any) {
        setError(err.message || "加载失败")
      } finally {
        setLoading(false)
      }
    },
    [apiUrl, kindFilter]
  )

  // initial load
  React.useEffect(() => {
    fetchEvents(true)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // ── detail fetch ──
  const fetchDetail = React.useCallback(
    async (ev: CalendarEvent) => {
      setDetailLoading(true)
      setDetailError(null)
      setDetail(null)
      try {
        const base = apiUrl.replace(/\/+$/, "")
        const url = `${base}/api/event/${ev._kind}/${ev.id}`
        const res = await fetch(url)
        if (!res.ok) throw new Error(`API ${res.status}`)
        const data: EventDetailResponse = await res.json()
        setDetail(data)
      } catch (err: any) {
        setDetailError(err.message || "加载详情失败")
      } finally {
        setDetailLoading(false)
      }
    },
    [apiUrl]
  )

  const handleEventClick = (ev: CalendarEvent) => {
    setSelectedEvent(ev)
    fetchDetail(ev)
  }

  const handleCloseDetail = () => {
    setSelectedEvent(null)
    setDetail(null)
    setDetailError(null)
  }

  // ── handlers ──
  const handleApiUrlChange = (url: string) => {
    setApiUrl(url)
    localStorage.setItem(STORAGE_KEY_API_URL, url)
  }

  const handleRefresh = () => fetchEvents(true)
  const handleModuleChange = (m: Module) => setModule(m)

  // ── render ──
  return (
    <MainShell>
      <div className="topics-wrapper">
        {/* ── sidebar ── */}
        <nav className="topics-nav">
          <div className="dock-nav">
            <ul className="dock-nav-list">
              <li
                className={cn(module === "upcoming" && "active")}
                data-node-id="upcoming"
              >
                <a
                  href="#"
                  onClick={(e) => {
                    e.preventDefault()
                    handleModuleChange("upcoming")
                  }}
                >
                  <CalendarIcon
                    className="node-logo node-logo-icon"
                    aria-hidden="true"
                  />
                  <div className="node-name">未来模块</div>
                </a>
              </li>
              <li
                className={cn(module === "past" && "active")}
                data-node-id="past"
              >
                <a
                  href="#"
                  onClick={(e) => {
                    e.preventDefault()
                    handleModuleChange("past")
                  }}
                >
                  <ClockIcon
                    className="node-logo node-logo-icon"
                    aria-hidden="true"
                  />
                  <div className="node-name">当前模块</div>
                </a>
              </li>
            </ul>
          </div>
        </nav>

        {/* ── main content ── */}
        <div className="topics-main">
          {/* toolbar */}
          <div className="mb-4 flex flex-wrap items-center gap-3 rounded-lg bg-background p-3">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span className="hidden sm:inline">API</span>
              <input
                type="text"
                value={apiUrl}
                onChange={(e) => handleApiUrlChange(e.target.value)}
                placeholder="http://localhost:8088"
                className="h-8 w-52 rounded border border-border bg-background px-2 text-sm text-foreground"
              />
            </div>

            <select
              value={kindFilter}
              onChange={(e) => setKindFilter(e.target.value)}
              className="h-8 rounded border border-border bg-background px-2 text-sm text-foreground"
            >
              <option value="">全部类型</option>
              {Object.entries(KIND_LABELS).map(([k, label]) => (
                <option key={k} value={k}>
                  {label}
                </option>
              ))}
            </select>

            <button
              onClick={handleRefresh}
              disabled={loading}
              className="inline-flex items-center gap-1.5 rounded border border-border bg-background px-3 py-1.5 text-sm text-foreground hover:border-primary disabled:opacity-50"
            >
              <RefreshCwIcon
                className={cn("size-3.5", loading && "animate-spin")}
              />
              刷新
            </button>

            <span className="ml-auto text-xs text-muted-foreground">
              共 {filteredEvents.length} 条
            </span>
          </div>

          {/* event list */}
          {error ? (
            <div className="rounded-lg bg-background p-8 text-center text-sm text-red-500">
              {error}
            </div>
          ) : loading && events.length === 0 ? (
            <div className="rounded-lg bg-background p-8 text-center text-sm text-muted-foreground">
              <RefreshCwIcon className="mx-auto mb-2 size-5 animate-spin" />
              加载中...
            </div>
          ) : filteredEvents.length === 0 ? (
            <div className="rounded-lg bg-background p-8 text-center text-sm text-muted-foreground">
              {module === "upcoming" ? "暂无未来事件" : "暂无历史事件"}
            </div>
          ) : (
            <div className="space-y-2">
              {filteredEvents.map((ev) => (
                <EventCard
                  key={`${ev._kind}:${ev.source_id}`}
                  event={ev}
                  onClick={() => handleEventClick(ev)}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      {/* ── event detail modal ── */}
      <Dialog open={selectedEvent !== null} onOpenChange={(open) => !open && handleCloseDetail()}>
        <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {detail?.post?.title || selectedEvent?.post?.title || "事件详情"}
            </DialogTitle>
          </DialogHeader>

          {detailLoading ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              <RefreshCwIcon className="mx-auto mb-2 size-5 animate-spin" />
              加载详情中...
            </div>
          ) : detailError ? (
            <div className="rounded-lg bg-red-50 p-4 text-sm text-red-500 dark:bg-red-950/30">
              {detailError}
            </div>
          ) : detail ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                {detail.kind && (
                  <span
                    className={cn(
                      "inline-flex items-center rounded px-1.5 py-0.5 font-semibold",
                      KIND_BADGE_CLASSES[detail.kind] ||
                        "bg-muted text-muted-foreground"
                    )}
                  >
                    {KIND_LABELS[detail.kind] || detail.kind}
                  </span>
                )}
                {detail.post?.category && (
                  <span>分类: {detail.post.category}</span>
                )}
                {detail.post?.tags && detail.post.tags.length > 0 && (
                  <span>标签: {detail.post.tags.join(", ")}</span>
                )}
              </div>

              {(detail.post?.content_md || selectedEvent?.post?.content_md) && (
                <MdPreview
                  modelValue={detail.post?.content_md || selectedEvent?.post?.content_md || ""}
                  theme={resolvedTheme === "dark" ? "dark" : "light"}
                />
              )}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </MainShell>
  )
}

// ── event card ──
function EventCard({
  event: ev,
  onClick,
}: {
  event: CalendarEvent
  onClick?: () => void
}) {
  const post = ev.post
  const title =
    post?.title || ev.indicator || ev.company || ev.title || "(无标题)"

  const date = ev._date || "-"
  const meta: string[] = []
  if (ev.country) meta.push(ev.country)
  if (ev.ticker) meta.push(ev.ticker)
  if (ev.importance) meta.push("★".repeat(ev.importance))

  return (
    <div
      onClick={onClick}
      className={cn(
        "rounded-lg border border-border bg-background p-4 shadow-sm transition-colors hover:border-primary",
        onClick && "cursor-pointer"
      )}
    >
      <div className="flex items-start gap-3">
        <span
          className={cn(
            "inline-flex shrink-0 items-center rounded px-1.5 py-0.5 text-xs font-semibold",
            KIND_BADGE_CLASSES[ev._kind] ||
              "bg-muted text-muted-foreground"
          )}
        >
          {KIND_LABELS[ev._kind] || ev._kind}
        </span>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold text-foreground">{title}</div>
          {meta.length > 0 && (
            <div className="mt-0.5 text-xs text-muted-foreground">
              {meta.join(" · ")}
            </div>
          )}
        </div>
        <div className="shrink-0 text-xs text-muted-foreground">{date}</div>
      </div>
    </div>
  )
}
