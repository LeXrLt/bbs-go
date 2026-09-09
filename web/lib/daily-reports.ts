import type { Category } from "./api/types"

export const DAILY_REPORT_CATEGORY_NAME = "\u65e5\u62a5"

export function findDailyReportCategory(
  categories: Category[]
): Category | null {
  const match = categories.find(
    (category) => category.name === DAILY_REPORT_CATEGORY_NAME
  )
  if (match) return match
  for (const category of categories) {
    const child = findDailyReportCategory(category.children || [])
    if (child) return child
  }
  return null
}

export function parseDailyReportFilters(searchParams: URLSearchParams) {
  const userIds = [
    ...new Set(
      (searchParams.get("userIds") || "")
        .split(",")
        .map((id) => id.trim())
        .filter(Boolean)
    ),
  ].sort()
  const sort =
    searchParams.get("sort") === "latestReply" ? "latestReply" : "latestEdit"
  return { userIds, sort }
}

export function dailyReportSearchParams(
  current: URLSearchParams,
  filters: ReturnType<typeof parseDailyReportFilters>
) {
  const next = new URLSearchParams(current)
  if (filters.userIds.length) {
    next.set("userIds", [...new Set(filters.userIds)].sort().join(","))
  } else {
    next.delete("userIds")
  }
  next.set("sort", filters.sort)
  next.delete("cursor")
  return next
}
