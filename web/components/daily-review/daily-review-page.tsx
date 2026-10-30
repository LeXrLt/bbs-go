"use client"

import * as React from "react"
import {
  ActivityIcon,
  Building2Icon,
  CalendarDaysIcon,
  ChartNoAxesCombinedIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  FileTextIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SearchIcon,
  TrendingUpIcon,
} from "lucide-react"
import {
  useLocation,
  useNavigate,
  useNavigation,
  useRevalidator,
} from "react-router"

import { HtmlImagePreview } from "@/components/common/image-preview"
import { MainShell } from "@/components/layout/main-shell"
import { TopicsNavContent } from "@/components/topic/topics-nav-content"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type {
  DailyReviewDashboard,
  DailyReviewDates,
  DailyReviewKind,
  DailyReviewReport,
  DailyReviewSection,
} from "@/lib/api/daily-review"
import type { Category } from "@/lib/api/types"
import {
  addDailyReviewDays,
  dailyReviewSearchParams,
  dailyReviewToday,
  formatDailyReviewDate,
  formatDailyReviewUpdatedAt,
  type DailyReviewQuery,
} from "@/lib/daily-review"
import { useI18n } from "@/lib/i18n/provider"
import { cn } from "@/lib/utils"

interface DailyReviewDetailState {
  report: DailyReviewReport | null
  failed: boolean
}

export interface DailyReviewRouteData {
  categories: Category[]
  query: DailyReviewQuery
  dashboard: DailyReviewDashboard
  dates: DailyReviewDates
  details: Partial<Record<DailyReviewKind, DailyReviewDetailState>>
  dashboardFailed: boolean
  datesFailed: boolean
}

const iconByKind = {
  shareholding: TrendingUpIcon,
  announcements: FileTextIcon,
  movers: ChartNoAxesCombinedIcon,
  focus: Building2Icon,
  buybacks: RotateCcwIcon,
  shorts: ActivityIcon,
} satisfies Record<DailyReviewKind, React.ComponentType<{ className?: string }>>

interface AnnouncementField {
  label: string
  html: string
  text: string
  wide: boolean
}

interface AnnouncementEntry {
  id: string
  market: string
  code: string
  company: string
  time: string
  title: string
  fields: AnnouncementField[]
  searchableText: string
}

interface AnnouncementGroup {
  title: string
  entries: AnnouncementEntry[]
}

interface ParsedAnnouncements {
  introHtml: string
  groups: AnnouncementGroup[]
}

interface FocusEntry {
  company: string
  changes: string[]
  sources: string[]
}

interface ParsedFocusReview {
  introHtml: string
  entries: FocusEntry[]
}

const announcementSummaryFields = new Set([
  "市场",
  "代码",
  "公司",
  "公告时间",
  "标题",
  "事项",
])

const announcementWideFields = new Set([
  "关注理由",
  "核心内容",
  "前序事件",
  "特殊事件要点",
  "后续跟踪",
  "判断",
])

function parseFocusHtml(html: string): ParsedFocusReview | null {
  if (typeof DOMParser === "undefined") return null

  const document = new DOMParser().parseFromString(html, "text/html")
  const introNodes: string[] = []
  const entries: FocusEntry[] = []
  let current: FocusEntry | null = null

  for (const child of Array.from(document.body.children)) {
    if (child.tagName === "H1") continue

    const companyMatch =
      child.tagName === "P"
        ? child.textContent?.trim().match(/^【(.+?)】$/)
        : null
    if (companyMatch) {
      current = { company: companyMatch[1], changes: [], sources: [] }
      entries.push(current)
      continue
    }

    if (!current) {
      introNodes.push(child.outerHTML)
      continue
    }

    if (child.tagName === "UL" || child.tagName === "OL") {
      for (const item of Array.from(child.querySelectorAll(":scope > li"))) {
        const text = item.textContent?.trim() || ""
        const itemHtml = item.innerHTML
        if (/^来源[:：]/.test(text)) {
          current.sources.push(itemHtml.replace(/^\s*来源[:：]\s*/, ""))
        } else {
          current.changes.push(
            itemHtml.replace(/^\s*新增变化[:：]\s*/, "")
          )
        }
      }
      continue
    }

    current.changes.push(child.outerHTML)
  }

  if (!entries.length) return null
  return { introHtml: introNodes.join(""), entries }
}

function prepareTableHtml(html: string) {
  if (typeof DOMParser === "undefined") return null

  const document = new DOMParser().parseFromString(html, "text/html")
  for (const table of Array.from(document.body.querySelectorAll("table"))) {
    table.classList.add("daily-review-data-table")
    const wrapper = document.createElement("div")
    wrapper.className = "daily-review-table-scroll"
    table.parentNode?.insertBefore(wrapper, table)
    wrapper.appendChild(table)
  }
  return document.body.innerHTML
}

function parseAnnouncementHtml(html: string): ParsedAnnouncements | null {
  if (typeof DOMParser === "undefined") return null

  const document = new DOMParser().parseFromString(html, "text/html")
  const headings = Array.from(document.body.querySelectorAll("h2"))
  const groups: AnnouncementGroup[] = []

  headings.forEach((heading, groupIndex) => {
    let sibling = heading.nextElementSibling
    while (sibling && sibling.tagName !== "H2" && sibling.tagName !== "TABLE") {
      sibling = sibling.nextElementSibling
    }
    if (!(sibling instanceof HTMLTableElement)) return

    const headers = Array.from(
      sibling.querySelectorAll<HTMLTableCellElement>("thead th"),
      (cell) => cell.textContent?.trim() || ""
    )
    if (!headers.length) return

    const entries = Array.from(
      sibling.querySelectorAll<HTMLTableRowElement>("tbody tr"),
      (row, rowIndex) => {
        const cells = Array.from(row.cells)
        const value = (name: string) => {
          const index = headers.indexOf(name)
          const cell = index >= 0 ? cells[index] : undefined
          return {
            html: cell?.innerHTML || "",
            text: cell?.textContent?.trim() || "",
          }
        }
        const market = value("市场").text
        const code = value("代码").text
        const company = value("公司").text
        const time = value("公告时间").text
        const title = value("标题").text || value("事项").text
        const fields = headers.flatMap((label, cellIndex) => {
          const cell = cells[cellIndex]
          if (!cell || announcementSummaryFields.has(label)) return []
          return [
            {
              label,
              html: cell.innerHTML,
              text: cell.textContent?.trim() || "",
              wide: announcementWideFields.has(label),
            },
          ]
        })

        return {
          id: `${groupIndex}-${rowIndex}`,
          market,
          code,
          company,
          time,
          title,
          fields,
          searchableText: [
            market,
            code,
            company,
            time,
            title,
            ...fields.map((field) => field.text),
          ]
            .join(" ")
            .toLocaleLowerCase(),
        }
      }
    )

    if (entries.length) {
      groups.push({ title: heading.textContent?.trim() || "", entries })
    }
  })

  if (!groups.length) return null

  const introNodes: string[] = []
  for (const child of Array.from(document.body.children)) {
    if (child.tagName === "H2") break
    if (child.tagName !== "H1") introNodes.push(child.outerHTML)
  }

  return { introHtml: introNodes.join(""), groups }
}

function TableReviewContent({
  html,
  kind,
}: {
  html: string
  kind: Exclude<DailyReviewKind, "focus" | "announcements">
}) {
  const [preparedHtml, setPreparedHtml] = React.useState<string>()

  React.useEffect(() => {
    setPreparedHtml(prepareTableHtml(html) || html)
  }, [html])

  return (
    <HtmlImagePreview
      html={preparedHtml || html}
      className={cn("daily-review-content", `daily-review-content-${kind}`)}
    />
  )
}

function FocusReviewContent({ html }: { html: string }) {
  const { t } = useI18n()
  const [parsed, setParsed] = React.useState<ParsedFocusReview | null>()

  React.useEffect(() => {
    setParsed(parseFocusHtml(html))
  }, [html])

  if (!parsed) {
    return (
      <HtmlImagePreview
        html={html}
        className="daily-review-content daily-review-content-focus"
      />
    )
  }

  return (
    <div className="daily-review-focus">
      {parsed.introHtml ? (
        <HtmlImagePreview
          html={parsed.introHtml}
          className="daily-review-focus-intro daily-review-content"
        />
      ) : null}
      <div className="daily-review-table-scroll">
        <table className="daily-review-data-table daily-review-focus-table">
          <thead>
            <tr>
              <th>{t("pages.dailyReview.table.company")}</th>
              <th>{t("pages.dailyReview.table.change")}</th>
              <th>{t("pages.dailyReview.table.source")}</th>
            </tr>
          </thead>
          <tbody>
            {parsed.entries.map((entry) => (
              <tr key={entry.company}>
                <th scope="row">{entry.company}</th>
                <td>
                  <ul className="daily-review-focus-changes">
                    {entry.changes.map((change, index) => (
                      <li key={`${entry.company}-change-${index}`}>
                        <HtmlImagePreview
                          html={change}
                          className="daily-review-cell-content"
                        />
                      </li>
                    ))}
                  </ul>
                </td>
                <td>
                  <div className="daily-review-focus-sources">
                    {entry.sources.map((source, index) => (
                      <HtmlImagePreview
                        key={`${entry.company}-source-${index}`}
                        html={source}
                        className="daily-review-cell-content"
                      />
                    ))}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function AnnouncementReviewContent({ html }: { html: string }) {
  const { t } = useI18n()
  const [query, setQuery] = React.useState("")
  const [parsed, setParsed] = React.useState<ParsedAnnouncements | null>()
  const [expandedIds, setExpandedIds] = React.useState<Set<string>>(
    () => new Set()
  )

  React.useEffect(() => {
    setParsed(parseAnnouncementHtml(html))
    setExpandedIds(new Set())
  }, [html])

  const normalizedQuery = query.trim().toLocaleLowerCase()
  const total =
    parsed?.groups.reduce((count, group) => count + group.entries.length, 0) ||
    0
  const groups = (parsed?.groups || []).map((group) => ({
    ...group,
    entries: normalizedQuery
      ? group.entries.filter((entry) =>
          entry.searchableText.includes(normalizedQuery)
        )
      : group.entries,
  }))
  const visible = groups.reduce(
    (count, group) => count + group.entries.length,
    0
  )

  if (parsed === undefined) {
    return (
      <p className="daily-review-message">
        {t("pages.dailyReview.organizingAnnouncements")}
      </p>
    )
  }
  if (parsed === null) {
    return <HtmlImagePreview html={html} className="daily-review-content" />
  }

  return (
    <div className="daily-review-announcements">
      {parsed.introHtml ? (
        <HtmlImagePreview
          html={parsed.introHtml}
          className="daily-review-announcement-intro daily-review-content"
        />
      ) : null}

      <div className="daily-review-announcement-toolbar">
        <label className="daily-review-announcement-search">
          <SearchIcon aria-hidden="true" />
          <span className="sr-only">
            {t("pages.dailyReview.searchAnnouncements")}
          </span>
          <Input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("pages.dailyReview.searchAnnouncementsPlaceholder")}
            aria-label={t("pages.dailyReview.searchAnnouncements")}
          />
        </label>
        <span className="daily-review-announcement-count" aria-live="polite">
          {t("pages.dailyReview.announcementCount", { visible, total })}
        </span>
      </div>

      {groups.map((group) => (
        <section key={group.title} className="daily-review-announcement-group">
          <div className="daily-review-announcement-group-heading">
            <h3>{group.title}</h3>
            <span>{group.entries.length}</span>
          </div>
          <div className="daily-review-table-scroll">
            <table className="daily-review-data-table daily-review-announcement-table">
              <thead>
                <tr>
                  <th>{t("pages.dailyReview.table.market")}</th>
                  <th>{t("pages.dailyReview.table.company")}</th>
                  <th>{t("pages.dailyReview.table.announcement")}</th>
                  <th>{t("pages.dailyReview.table.reason")}</th>
                  <th>{t("pages.dailyReview.table.time")}</th>
                  <th aria-label={t("pages.dailyReview.table.details")} />
                </tr>
              </thead>
              <tbody>
                {group.entries.map((entry) => {
                  const expanded = expandedIds.has(entry.id)
                  const highlight = entry.fields.find((field) =>
                    ["关注理由", "判断"].includes(field.label)
                  )
                  const marketCap = entry.fields.find(
                    (field) => field.label === "市值是否达标"
                  )
                  const coreContent = entry.fields.find((field) =>
                    ["核心内容", "特殊事件要点"].includes(field.label)
                  )
                  const followUp = entry.fields.find((field) =>
                    ["后续跟踪", "后续关注"].includes(field.label)
                  )
                  return (
                    <React.Fragment key={entry.id}>
                      <tr className="daily-review-announcement-main-row">
                        <td>
                          <span
                            className={cn(
                              "daily-review-market-badge",
                              entry.market === "A股" &&
                                "daily-review-market-badge-a"
                            )}
                          >
                            {entry.market || "--"}
                          </span>
                        </td>
                        <th scope="row">
                          <span
                            className="daily-review-announcement-company-stack"
                            title={
                              [
                                [entry.company || entry.title, entry.code]
                                  .filter(Boolean)
                                  .join(" "),
                                marketCap?.text,
                              ]
                                .filter(Boolean)
                                .join("\n")
                            }
                          >
                            <span className="daily-review-announcement-company-cell">
                              <span className="daily-review-announcement-company">
                                {entry.company || entry.title}
                              </span>
                              {entry.code ? (
                                <span className="daily-review-announcement-code">
                                  {entry.code}
                                </span>
                              ) : null}
                            </span>
                            {marketCap ? (
                              <span className="daily-review-announcement-secondary">
                                <span className="daily-review-announcement-secondary-label">
                                  {t("pages.dailyReview.table.marketCapShort")}
                                </span>
                                <span className="daily-review-announcement-secondary-text">
                                  {marketCap.text}
                                </span>
                              </span>
                            ) : null}
                          </span>
                        </th>
                        <td
                          className="daily-review-announcement-summary-cell"
                          title={[entry.title, coreContent?.text]
                            .filter(Boolean)
                            .join("\n")}
                        >
                          <span className="daily-review-announcement-title">
                            {entry.title}
                          </span>
                          {coreContent ? (
                            <span className="daily-review-announcement-secondary">
                              <span className="daily-review-announcement-secondary-label">
                                {t("pages.dailyReview.table.coreContentShort")}
                              </span>
                              <span className="daily-review-announcement-secondary-text">
                                {coreContent.text}
                              </span>
                            </span>
                          ) : null}
                        </td>
                        <td
                          className="daily-review-announcement-summary-cell"
                          title={[highlight?.text, followUp?.text]
                            .filter(Boolean)
                            .join("\n")}
                        >
                          {highlight ? (
                            <HtmlImagePreview
                              html={highlight.html}
                              className="daily-review-cell-content daily-review-announcement-primary"
                            />
                          ) : (
                            "--"
                          )}
                          {followUp ? (
                            <span className="daily-review-announcement-secondary">
                              <span className="daily-review-announcement-secondary-label">
                                {t("pages.dailyReview.table.followUpShort")}
                              </span>
                              <span className="daily-review-announcement-secondary-text">
                                {followUp.text}
                              </span>
                            </span>
                          ) : null}
                        </td>
                        <td
                          className="daily-review-announcement-time"
                          title={entry.time}
                        >
                          {entry.time || "--"}
                        </td>
                        <td className="daily-review-announcement-action">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            aria-expanded={expanded}
                            title={t(
                              expanded
                                ? "pages.dailyReview.collapseDetails"
                                : "pages.dailyReview.expandDetails"
                            )}
                            onClick={() =>
                              setExpandedIds((current) => {
                                const next = new Set(current)
                                if (next.has(entry.id)) next.delete(entry.id)
                                else next.add(entry.id)
                                return next
                              })
                            }
                          >
                            <ChevronDownIcon
                              className={cn(
                                "daily-review-announcement-chevron",
                                expanded && "is-expanded"
                              )}
                              aria-hidden="true"
                            />
                          </Button>
                        </td>
                      </tr>
                      {expanded ? (
                        <tr className="daily-review-announcement-detail-row">
                          <td colSpan={6}>
                            <div className="daily-review-announcement-detail">
                              {entry.fields.map((field) => (
                                <div
                                  key={field.label}
                                  className={cn(
                                    "daily-review-announcement-field",
                                    field.wide &&
                                      "daily-review-announcement-field-wide"
                                  )}
                                >
                                  <span>{field.label}</span>
                                  <HtmlImagePreview
                                    html={field.html}
                                    className="daily-review-announcement-value"
                                  />
                                </div>
                              ))}
                            </div>
                          </td>
                        </tr>
                      ) : null}
                    </React.Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
        </section>
      ))}

      {!visible ? (
        <p className="daily-review-announcement-empty">
          {t("pages.dailyReview.noAnnouncementMatches")}
        </p>
      ) : null}
    </div>
  )
}

function statusLabel(
  status: DailyReviewSection["status"],
  t: ReturnType<typeof useI18n>["t"]
) {
  if (status === "ready") return t("pages.dailyReview.status.ready")
  if (status === "no_items") return t("pages.dailyReview.status.noItems")
  return t("pages.dailyReview.status.pending")
}

function DailyReviewSectionPanel({
  section,
  detail,
  index,
}: {
  section: DailyReviewSection
  detail?: DailyReviewDetailState
  index: number
}) {
  const { locale, t } = useI18n()
  const Icon = iconByKind[section.kind]
  const report = detail?.report
  const metrics = report?.metrics?.length ? report.metrics : section.metrics
  const updatedAt = report?.updated_at || section.updated_at

  return (
    <section
      id={section.kind}
      className="daily-review-section scroll-mt-20"
      aria-labelledby={`daily-review-heading-${section.kind}`}
    >
      <header className="daily-review-section-header">
        <div className="min-w-0">
          <p className="daily-review-section-kicker">
            {String(index + 1).padStart(2, "0")} / 06
          </p>
          <div className="mt-1.5 flex min-w-0 items-center gap-2.5">
            <span className="daily-review-section-icon" aria-hidden="true">
              <Icon className="size-4.5" />
            </span>
            <h2
              id={`daily-review-heading-${section.kind}`}
              className="min-w-0 text-lg font-semibold sm:text-xl"
            >
              {section.label}
            </h2>
          </div>
        </div>
        <div className="daily-review-section-meta">
          <span
            className={cn(
              "daily-review-status",
              `daily-review-status-${section.status}`
            )}
          >
            {statusLabel(section.status, t)}
          </span>
          {updatedAt ? (
            <span>
              {t("pages.dailyReview.updatedAt", {
                value: formatDailyReviewUpdatedAt(updatedAt, locale),
              })}
            </span>
          ) : null}
        </div>
      </header>

      {metrics.length ? (
        <div className="daily-review-metrics">
          {metrics.map((metric) => (
            <div
              key={`${metric.label}-${metric.value}`}
              className="daily-review-metric"
            >
              <span>{metric.label}</span>
              <strong>{metric.value}</strong>
            </div>
          ))}
        </div>
      ) : null}

      <div className="daily-review-section-body">
        {section.status === "pending" ? (
          <p className="daily-review-message">
            {t("pages.dailyReview.pendingDescription")}
          </p>
        ) : detail?.failed || !report ? (
          <p className="daily-review-message text-destructive">
            {t("pages.dailyReview.sectionLoadError")}
          </p>
        ) : report.content_html ? (
          <>
            {section.kind === "focus" ? (
              <FocusReviewContent html={report.content_html} />
            ) : section.kind === "announcements" ? (
              <AnnouncementReviewContent html={report.content_html} />
            ) : (
              <TableReviewContent
                html={report.content_html}
                kind={section.kind}
              />
            )}
            <footer className="daily-review-section-footer">
              {t("pages.dailyReview.sourceNote")}
            </footer>
          </>
        ) : (
          <p className="daily-review-message">
            {t("pages.dailyReview.emptyContent")}
          </p>
        )}
      </div>
    </section>
  )
}

export function DailyReviewPage({ data }: { data: DailyReviewRouteData }) {
  const { locale, t } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const navigation = useNavigation()
  const revalidator = useRevalidator()
  const today = dailyReviewToday()
  const loading =
    navigation.location?.pathname === location.pathname ||
    revalidator.state === "loading"
  const updatedSections = data.dashboard.sections.filter(
    (section) => section.status !== "pending"
  ).length

  function changeDate(date: string) {
    if (date > today) return
    const next = dailyReviewSearchParams(new URLSearchParams(location.search), {
      date,
    })
    void navigate({ search: next.toString() })
  }

  return (
    <MainShell>
      <div className="topics-wrapper">
        <TopicsNavContent initialCategories={data.categories} />
        <div className="topics-main min-w-0">
          <div className="daily-review-page" aria-busy={loading}>
            <header className="daily-review-header">
              <div className="min-w-0">
                <p className="text-xs font-semibold text-muted-foreground uppercase">
                  {t("pages.dailyReview.eyebrow")}
                </p>
                <h1 className="mt-1 text-xl font-semibold text-foreground sm:text-2xl">
                  {t("pages.dailyReview.title")}
                </h1>
                <p className="mt-1 text-sm text-muted-foreground">
                  {formatDailyReviewDate(data.query.date, locale)}
                </p>
              </div>
              <div className="daily-review-coverage" aria-live="polite">
                <span className="daily-review-coverage-dot" />
                {t("pages.dailyReview.coverage", {
                  updated: updatedSections,
                  total: data.dashboard.sections.length || 6,
                })}
                {loading ? (
                  <RefreshCwIcon
                    className="size-3.5 animate-spin"
                    aria-hidden="true"
                  />
                ) : null}
              </div>
            </header>

            <div className="daily-review-date-controls">
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                title={t("pages.dailyReview.previousDay")}
                aria-label={t("pages.dailyReview.previousDay")}
                onClick={() =>
                  changeDate(addDailyReviewDays(data.query.date, -1))
                }
              >
                <ChevronLeftIcon aria-hidden="true" />
              </Button>
              <label className="daily-review-date-field">
                <CalendarDaysIcon aria-hidden="true" />
                <span className="sr-only">
                  {t("pages.dailyReview.chooseDate")}
                </span>
                <input
                  type="date"
                  value={data.query.date}
                  max={today}
                  list="daily-review-dates"
                  aria-label={t("pages.dailyReview.chooseDate")}
                  onChange={(event) => changeDate(event.target.value)}
                />
              </label>
              <datalist id="daily-review-dates">
                {data.dates.dates.map((date) => (
                  <option key={date} value={date} />
                ))}
              </datalist>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                disabled={data.query.date >= today}
                title={t("pages.dailyReview.nextDay")}
                aria-label={t("pages.dailyReview.nextDay")}
                onClick={() =>
                  changeDate(addDailyReviewDays(data.query.date, 1))
                }
              >
                <ChevronRightIcon aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={data.query.date === today}
                onClick={() => changeDate(today)}
              >
                {t("pages.dailyReview.today")}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={
                  !data.dates.latest || data.dates.latest === data.query.date
                }
                onClick={() =>
                  data.dates.latest && changeDate(data.dates.latest)
                }
              >
                {t("pages.dailyReview.latest")}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                title={t("pages.dailyReview.refresh")}
                aria-label={t("pages.dailyReview.refresh")}
                onClick={() => revalidator.revalidate()}
              >
                <RefreshCwIcon
                  className={cn(loading && "animate-spin")}
                  aria-hidden="true"
                />
              </Button>
            </div>

            {data.dashboardFailed ? (
              <div className="daily-review-load-error">
                <h2>{t("pages.dailyReview.loadError")}</h2>
                <p>{t("pages.dailyReview.loadErrorDescription")}</p>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => revalidator.revalidate()}
                >
                  <RefreshCwIcon aria-hidden="true" />
                  {t("pages.dailyReview.retry")}
                </Button>
              </div>
            ) : (
              <div className="daily-review-sections">
                {data.dashboard.sections.map((section, index) => (
                  <DailyReviewSectionPanel
                    key={section.kind}
                    section={section}
                    detail={data.details[section.kind]}
                    index={index}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </MainShell>
  )
}
