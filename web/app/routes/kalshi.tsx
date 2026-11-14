import { useLoaderData } from "react-router"

import { KalshiPage } from "@/components/kalshi/kalshi-page"
import {
  getKalshiDates,
  getKalshiSnapshots,
  type KalshiSlot,
} from "@/lib/api/kalshi"
import {
  cachedKalshiDates,
  cachedKalshiSnapshots,
} from "@/lib/api/kalshi-page-cache"
import { calendarToday, isCalendarDate } from "@/lib/calendar"
import { localizedTitle, pageMeta, rootDataFromMatches } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"

async function load(request: Request, client: boolean) {
  const options = { request, signal: request.signal }
  const params = new URL(request.url).searchParams
  const requested = params.get("date")
  const today = calendarToday()
  const requestedDate =
    isCalendarDate(requested) && requested <= today ? requested : null
  const rawSlot = params.get("slot")
  const requestedSlot: KalshiSlot | null =
    rawSlot === "morning" || rawSlot === "evening" ? rawSlot : null
  try {
    const dates = client
      ? await cachedKalshiDates()
      : (await getKalshiDates(options)).dates
    const date = requestedDate || dates[0] || today
    if (!dates.includes(date))
      return { dates, date, slot: requestedSlot, snapshots: [], error: false }
    try {
      const snapshots = client
        ? await cachedKalshiSnapshots(date)
        : (await getKalshiSnapshots(date, options)).snapshots
      const slot: KalshiSlot | null = requestedSlot
        ? requestedSlot
        : snapshots.some((snapshot) => snapshot.slot === "evening")
          ? "evening"
          : snapshots.some((snapshot) => snapshot.slot === "morning")
            ? "morning"
            : null
      return { dates, date, slot, snapshots, error: false }
    } catch {
      return { dates, date, slot: requestedSlot, snapshots: [], error: true }
    }
  } catch {
    return {
      dates: [] as string[],
      date: "",
      slot: null,
      snapshots: [],
      error: true,
    }
  }
}

export async function loader({ request }: { request: Request }) {
  return load(request, false)
}

export async function clientLoader({ request }: { request: Request }) {
  return load(request, true)
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
