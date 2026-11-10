import { useLoaderData } from "react-router"

import { KalshiPage } from "@/components/kalshi/kalshi-page"
import { getKalshiDates, getKalshiReport } from "@/lib/api/kalshi"
import { calendarToday, isCalendarDate } from "@/lib/calendar"
import { localizedTitle, pageMeta, rootDataFromMatches } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"

async function load(request: Request) {
  const options = { request, signal: request.signal }
  try {
    const { dates } = await getKalshiDates(options)
    const requested = new URL(request.url).searchParams.get("date")
    const today = calendarToday()
    const date =
      isCalendarDate(requested) && requested <= today
        ? requested
        : dates[0] || today
    if (!dates.includes(date))
      return { dates, date, report: null, error: false }
    try {
      return {
        dates,
        date,
        report: await getKalshiReport(date, options),
        error: false,
      }
    } catch {
      return { dates, date, report: null, error: true }
    }
  } catch {
    return { dates: [] as string[], date: "", report: null, error: true }
  }
}

export async function loader({ request }: { request: Request }) {
  return load(request)
}

export async function clientLoader({ request }: { request: Request }) {
  return load(request)
}

export function meta({
  location,
  matches,
}: {
  location: { pathname: string }
  matches: Array<{ data?: unknown; loaderData?: unknown }>
}) {
  const root = rootDataFromMatches(matches)
  return pageMeta(
    root?.config,
    localizedTitle(root?.locale, "Kalshi Markets", "Kalshi 合约"),
    {
      canonicalPath: location.pathname,
    }
  )
}

export default function KalshiRoute() {
  useDocumentTitle("Kalshi 合约")
  return <KalshiPage data={useLoaderData<typeof loader>()} />
}
