import { useLoaderData } from "react-router"

import {
  CalendarWorkbench,
  type CalendarRouteData,
} from "@/components/calendar/calendar-workbench"
import {
  getCalendarEvents,
  type CalendarEventsResponse,
} from "@/lib/api/calendar"
import {
  CALENDAR_PAGE_SIZE,
  CALENDAR_TIMEZONE,
  parseCalendarSearchParams,
} from "@/lib/calendar"
import { useI18n } from "@/lib/i18n/provider"
import { localizedTitle, pageMeta, rootDataFromMatches } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"

function emptyCalendarPage(date: string): CalendarEventsResponse {
  return {
    generatedAt: "",
    dateFrom: date,
    dateTo: date,
    timezone: CALENDAR_TIMEZONE,
    counts: {
      economic: 0,
      earnings: 0,
      corporate: 0,
      ipo: 0,
      total: 0,
    },
    total: 0,
    results: [],
    cursor: "",
    hasMore: false,
  }
}

async function loadCalendarRouteData(
  request: Request
): Promise<CalendarRouteData> {
  const query = parseCalendarSearchParams(new URL(request.url).searchParams)

  try {
    const page = await getCalendarEvents(
      {
        dateFrom: query.date,
        dateTo: query.date,
        kind: query.kind || undefined,
        keyword: query.keyword || undefined,
        minImportance: query.minImportance || undefined,
        limit: CALENDAR_PAGE_SIZE,
      },
      { request }
    )
    return { page, query, failed: false }
  } catch {
    return { page: emptyCalendarPage(query.date), query, failed: true }
  }
}

export async function loader({ request }: { request: Request }) {
  return loadCalendarRouteData(request)
}

export async function clientLoader({ request }: { request: Request }) {
  return loadCalendarRouteData(request)
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
    localizedTitle(rootData?.locale, "Financial Calendar", "金融日历"),
    { canonicalPath: location.pathname }
  )
}

export default function CalendarRoute() {
  const data = useLoaderData<typeof loader>()
  const { t } = useI18n()
  useDocumentTitle(t("pages.calendar.title"))
  return <CalendarWorkbench data={data} />
}
