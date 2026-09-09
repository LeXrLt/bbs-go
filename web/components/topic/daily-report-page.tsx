import * as React from "react"
import { LayoutGrid, LoaderCircle, Search, X } from "lucide-react"
import { useLocation, useNavigate, useNavigation } from "react-router"

import type { DailyReportRouteData } from "@/app/route-helpers/loaders"
import { UserAvatar } from "@/components/common/avatar"
import { EmptyState } from "@/components/common/empty-state"
import Link from "@/components/common/link"
import { LoadMore } from "@/components/common/load-more"
import { MainShell } from "@/components/layout/main-shell"
import { TopicListItem } from "@/components/topic/topic-list-item"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { apiFetch } from "@/lib/api/client"
import type { PageData, Topic } from "@/lib/api/types"
import {
  dailyReportSearchParams,
  parseDailyReportFilters,
} from "@/lib/daily-reports"
import { useI18n } from "@/lib/i18n/provider"
import { useDocumentTitle } from "@/lib/use-document-title"

export function DailyReportPage({ data }: { data: DailyReportRouteData }) {
  const { t } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const navigation = useNavigation()
  const [search, setSearch] = React.useState("")
  const pending = navigation.location?.pathname === location.pathname
  const currentSearch = pending ? navigation.location!.search : location.search
  const filters = parseDailyReportFilters(new URLSearchParams(currentSearch))
  const desiredSearch = React.useRef(new URLSearchParams(currentSearch))
  const category = data.category
  const title = category?.name || t("pages.dailyReports.title")
  useDocumentTitle(title)

  React.useEffect(() => {
    desiredSearch.current = new URLSearchParams(currentSearch)
  }, [currentSearch])

  function updateFilters(update: (current: typeof filters) => typeof filters) {
    const next = dailyReportSearchParams(
      desiredSearch.current,
      update(parseDailyReportFilters(desiredSearch.current))
    )
    desiredSearch.current = next
    void navigate({ search: next.toString() }, { preventScrollReset: true })
  }

  function toggleAuthor(id: string, checked: boolean) {
    updateFilters((current) => ({
      ...current,
      userIds: checked
        ? [...current.userIds, id]
        : current.userIds.filter((value) => value !== id),
    }))
  }

  const authors = data.authors.filter((user) =>
    `${user.nickname || ""} ${user.username || ""} ${user.id}`
      .toLocaleLowerCase()
      .includes(search.trim().toLocaleLowerCase())
  )
  const resultsAreCurrent =
    data.filters.sort === filters.sort &&
    data.filters.userIds.join(",") === filters.userIds.join(",")

  return (
    <MainShell>
      {category ? (
        <div className="grid gap-3 md:grid-cols-[220px_minmax(0,1fr)]">
          <aside
            className="daily-report-authors min-w-0 self-start bg-background md:sticky md:top-[68px]"
            aria-label={t("pages.dailyReports.users")}
          >
            <div className="flex h-14 items-center justify-between border-b px-3">
              <h2 className="text-sm font-semibold">
                {t("pages.dailyReports.users")}
              </h2>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                title={t("pages.dailyReports.clearUsers")}
                aria-label={t("pages.dailyReports.clearUsers")}
                disabled={!filters.userIds.length}
                onClick={() =>
                  updateFilters((current) => ({ ...current, userIds: [] }))
                }
              >
                <X className="size-4" />
              </Button>
            </div>
            <div className="relative m-3">
              <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t("pages.dailyReports.searchUsers")}
                aria-label={t("pages.dailyReports.searchUsers")}
                className="h-9 pl-8"
              />
            </div>
            <div className="max-h-48 overflow-y-auto px-2 pb-2 md:max-h-[calc(100vh-220px)]">
              {authors.map((user) => {
                const id = String(user.id)
                const checked = filters.userIds.includes(id)
                return (
                  <label
                    key={id}
                    htmlFor={`daily-author-${id}`}
                    className="flex min-h-11 cursor-pointer items-center gap-2 px-2 py-1.5 hover:bg-muted/60 has-[[data-state=checked]]:bg-accent"
                  >
                    <Checkbox
                      id={`daily-author-${id}`}
                      checked={checked}
                      disabled={!checked && filters.userIds.length >= 100}
                      onCheckedChange={(value) =>
                        toggleAuthor(id, value === true)
                      }
                      className="data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground"
                    />
                    <UserAvatar user={user} size={28} linkToProfile={false} />
                    <span className="min-w-0 text-sm break-words">
                      {user.nickname || user.username || id}
                    </span>
                  </label>
                )
              })}
              {!authors.length ? (
                <p className="px-2 py-4 text-sm text-muted-foreground">
                  {t("pages.dailyReports.noUsers")}
                </p>
              ) : null}
            </div>
          </aside>
          <div
            className="topics-main min-w-0 bg-background"
            aria-busy={!resultsAreCurrent}
          >
            <div className="flex min-h-14 flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h1 className="text-base font-semibold">{title}</h1>
                <span className="text-xs text-muted-foreground">
                  {filters.userIds.length
                    ? t("pages.dailyReports.selectedUsers", {
                        count: filters.userIds.length,
                      })
                    : t("pages.dailyReports.allUsers")}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <select
                  className="h-8 max-w-full rounded-md border bg-background px-2 text-sm"
                  aria-label={t("pages.dailyReports.sort")}
                  value={filters.sort}
                  onChange={(event) =>
                    updateFilters((current) => ({
                      ...current,
                      sort: event.target.value,
                    }))
                  }
                >
                  <option value="latestPublish">
                    {t("pages.topics.filterLatestPublish")}
                  </option>
                  <option value="latestReply">
                    {t("pages.topics.filterLatestReply")}
                  </option>
                </select>
                <Link
                  href="/topics"
                  className="inline-flex size-8 items-center justify-center rounded-md hover:bg-muted"
                  title={t("pages.topics.title")}
                  aria-label={t("pages.topics.title")}
                >
                  <LayoutGrid className="size-4" />
                </Link>
              </div>
            </div>
            {!resultsAreCurrent ? (
              <div
                role="status"
                className="flex min-h-48 items-center justify-center text-muted-foreground"
              >
                <LoaderCircle
                  className="size-5 animate-spin"
                  aria-hidden="true"
                />
                <span className="sr-only">{t("common.loadMore.loading")}</span>
              </div>
            ) : (
              <LoadMore<Topic>
                initialItems={data.topics.results}
                initialCursor={data.topics.cursor || ""}
                initialHasMore={data.topics.hasMore}
                autoLoadOnScroll
                resetKey={`daily:${category.id}:${filters.sort}:${filters.userIds.join(",")}`}
                labels={{
                  loadMore: t("common.loadMore.loadMore"),
                  noMore: t("common.loadMore.noMore"),
                }}
                loadPage={({ cursor }) =>
                  apiFetch<PageData<Topic>>("/api/topic/topics", {
                    params: {
                      categoryId: category.id,
                      cursor,
                      sort: filters.sort,
                      userIds: filters.userIds.join(","),
                    },
                  })
                }
                renderItems={(items) => (
                  <ul className="divide-y divide-border">
                    {items.map((topic) => (
                      <TopicListItem
                        key={topic.id}
                        topic={topic}
                        showSticky
                        t={t}
                      />
                    ))}
                  </ul>
                )}
                renderEmpty={() => <EmptyState title={t("common.noData")} />}
              />
            )}
          </div>
        </div>
      ) : (
        <EmptyState title={t("pages.dailyReports.missingCategory")} />
      )}
    </MainShell>
  )
}
