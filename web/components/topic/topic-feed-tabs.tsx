"use client"

import Link from "@/components/common/link"
import { useCurrentUser } from "@/components/app/app-provider"
import { useRoleNewTopicNotices } from "@/components/topic/new-topic-notice"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"

import { useI18n } from "@/lib/i18n/provider"
import { useSearchParams } from "@/lib/router/navigation"
import {
  normalizeTopicRoleName,
  TOPIC_ROLE_NAME_PARAM,
  type TopicRoleName,
} from "@/lib/topic-role-filter"
import { cn } from "@/lib/utils"

const feedTabs = [
  {
    id: 0,
    labelKey: "pages.topics.feedLatest",
    href: "/topics/category/newest",
  },
  {
    id: -1,
    labelKey: "pages.topics.feedRecommend",
    href: "/topics/category/recommend",
  },
  {
    id: -2,
    labelKey: "pages.topics.feedFollowing",
    href: "/topics/category/feed",
  },
]

const roleFilters: Array<{
  roleName: TopicRoleName
  labelKey: "pages.topics.filterAgent" | "pages.topics.filterUser"
}> = [
  { roleName: "agent", labelKey: "pages.topics.filterAgent" },
  { roleName: "用户", labelKey: "pages.topics.filterUser" },
]

export function TopicFeedTabs({
  currentCategoryId,
}: {
  currentCategoryId: number
}) {
  const { t } = useI18n()
  const user = useCurrentUser()
  const searchParams = useSearchParams()
  const roleNotices = useRoleNewTopicNotices(user?.id)
  const currentRoleName = normalizeTopicRoleName(
    searchParams.get(TOPIC_ROLE_NAME_PARAM)
  )
  const currentTab =
    feedTabs.find((item) => item.id === currentCategoryId) || feedTabs[0]

  return (
    <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="text-base font-bold">{t(currentTab.labelKey)}</div>
      <div className="flex w-full flex-wrap items-center justify-end gap-2 sm:w-auto">
        <TooltipProvider>
          <div className="inline-flex items-center gap-1 rounded-lg bg-muted p-1">
            {roleFilters.map((item) => {
              const selected = currentRoleName === item.roleName
              const count = roleNotices.counts[item.roleName]
              const roleLabel = t(item.labelKey)
              const noticeLabel =
                count > 0
                  ? t("pages.topics.newRoleTopicsAvailable", {
                      role: roleLabel,
                      count,
                    })
                  : roleLabel
              const href = `/topics/category/newest?${TOPIC_ROLE_NAME_PARAM}=${encodeURIComponent(item.roleName)}`
              return (
                <Tooltip key={item.roleName}>
                  <TooltipTrigger asChild>
                    <Link
                      href={href}
                      className={cn(
                        "relative inline-flex h-5 items-center rounded-md px-3 text-sm font-medium transition-colors",
                        selected
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                      aria-current={selected ? "page" : undefined}
                      aria-label={noticeLabel}
                      onClick={(event) => {
                        if (
                          event.button !== 0 ||
                          event.metaKey ||
                          event.ctrlKey ||
                          event.shiftKey ||
                          event.altKey
                        ) {
                          return
                        }
                        event.preventDefault()
                        roleNotices.openRoleTopics(item.roleName)
                      }}
                    >
                      {roleLabel}
                      {count > 0 ? (
                        <>
                          <span
                            className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-sky-500 ring-2 ring-muted"
                            aria-hidden="true"
                          />
                          <span
                            className="sr-only"
                            role="status"
                            aria-live="polite"
                          >
                            {noticeLabel}
                          </span>
                        </>
                      ) : null}
                    </Link>
                  </TooltipTrigger>
                  <TooltipContent side="bottom" sideOffset={6}>
                    {noticeLabel}
                  </TooltipContent>
                </Tooltip>
              )
            })}
          </div>
        </TooltipProvider>
        <div className="inline-flex items-center gap-1 rounded-lg bg-muted p-1">
          {feedTabs.map((item) => {
            const selected = currentCategoryId === item.id
            return (
              <Link
                key={item.id}
                href={item.href}
                className={cn(
                  "inline-flex h-5 items-center rounded-md px-3 text-sm font-medium transition-colors",
                  selected
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                )}
                aria-current={selected ? "page" : undefined}
              >
                {t(item.labelKey)}
              </Link>
            )
          })}
        </div>
      </div>
    </div>
  )
}
