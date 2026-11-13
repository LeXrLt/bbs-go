import { useLoaderData } from "react-router"

import { KalshiPage } from "@/components/kalshi/kalshi-page"
import { getKalshiDates, getKalshiReport, getKalshiSnapshots, type KalshiSlot } from "@/lib/api/kalshi"
import { calendarToday, isCalendarDate } from "@/lib/calendar"
import { localizedTitle, pageMeta, rootDataFromMatches } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"

async function load(request: Request) {
  const options = { request, signal: request.signal }
  try {
    const { dates } = await getKalshiDates(options)
    const params = new URL(request.url).searchParams
    const requested = params.get("date")
    const today = calendarToday()
    const date =
      isCalendarDate(requested) && requested <= today
        ? requested
        : dates[0] || today
    const rawSlot = params.get("slot")
    const requestedSlot: KalshiSlot | null = rawSlot === "morning" || rawSlot === "evening" ? rawSlot : null
    if (!dates.includes(date))
      return { dates, date, slot: requestedSlot, snapshots: [], report: null, error: false }
    try {
      const [snapshotData, directReport] = requestedSlot
        ? await Promise.all([
            getKalshiSnapshots(date, options),
            getKalshiReport(date, requestedSlot, options),
          ])
        : [await getKalshiSnapshots(date, options), null]
      const { snapshots } = snapshotData
      const slot: KalshiSlot | null = requestedSlot
        ? requestedSlot
        : snapshots.some((snapshot) => snapshot.slot === "evening")
          ? "evening"
          : snapshots.some((snapshot) => snapshot.slot === "morning") ? "morning" : null
      const response = directReport ?? (slot ? await getKalshiReport(date, slot, options) : null)
      if (response && (response.date !== date || response.slot !== slot ||
        (response.published && response.manifest.metadata.run_slot !== slot))) {
        throw new Error("Kalshi report slot mismatch")
      }
      return {
        dates,
        date,
        slot,
        snapshots,
        report: response?.published ? response : null,
        error: false,
      }
    } catch {
      return { dates, date, slot: requestedSlot, snapshots: [], report: null, error: true }
    }
  } catch {
    return { dates: [] as string[], date: "", slot: null, snapshots: [], report: null, error: true }
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
