import { useLoaderData } from "react-router"

import {
  DailyReviewPage,
  type DailyReviewRouteData,
} from "@/components/daily-review/daily-review-page"
import {
  getDailyReviewDashboard,
  getDailyReviewDates,
  getDailyReviewReport,
  type DailyReviewDashboard,
  type DailyReviewDates,
} from "@/lib/api/daily-review"
import { parseDailyReviewSearchParams } from "@/lib/daily-review"
import { useI18n } from "@/lib/i18n/provider"
import { localizedTitle, pageMeta, rootDataFromMatches } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"

import { loadCategories } from "../route-helpers/loaders"

function emptyDates(): DailyReviewDates {
  return { dates: [] }
}

function emptyDashboard(date: string): DailyReviewDashboard {
  return { date, sections: [] }
}

async function loadDailyReviewRouteData(
  request: Request
): Promise<DailyReviewRouteData> {
  const query = parseDailyReviewSearchParams(new URL(request.url).searchParams)
  const [categories, datesResult, dashboardResult] = await Promise.all([
    loadCategories(request),
    getDailyReviewDates({ request, signal: request.signal })
      .then((dates) => ({ dates, failed: false }))
      .catch(() => ({ dates: emptyDates(), failed: true })),
    getDailyReviewDashboard(query.date, {
      request,
      signal: request.signal,
    })
      .then((dashboard) => ({ dashboard, failed: false }))
      .catch(() => ({ dashboard: emptyDashboard(query.date), failed: true })),
  ])

  const details: DailyReviewRouteData["details"] = {}
  if (!dashboardResult.failed) {
    await Promise.all(
      dashboardResult.dashboard.sections
        .filter((section) => section.status !== "pending")
        .map(async (section) => {
          try {
            details[section.kind] = {
              report: await getDailyReviewReport(section.kind, query.date, {
                request,
                signal: request.signal,
              }),
              failed: false,
            }
          } catch {
            details[section.kind] = { report: null, failed: true }
          }
        })
    )
  }

  return {
    categories,
    query,
    dashboard: dashboardResult.dashboard,
    dates: datesResult.dates,
    details,
    dashboardFailed: dashboardResult.failed,
    datesFailed: datesResult.failed,
  }
}

export async function loader({ request }: { request: Request }) {
  return loadDailyReviewRouteData(request)
}

export async function clientLoader({ request }: { request: Request }) {
  return loadDailyReviewRouteData(request)
}

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
    localizedTitle(rootData?.locale, "Daily Review", "每日复盘"),
    { canonicalPath: location.pathname }
  )
}

export default function DailyReviewRoute() {
  const data = useLoaderData<typeof loader>()
  const { t } = useI18n()
  useDocumentTitle(t("pages.dailyReview.title"))
  return <DailyReviewPage data={data} />
}
