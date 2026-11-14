import {
  getKalshiDates,
  getKalshiReport,
  getKalshiSnapshots,
  type KalshiReport,
  type KalshiSlot,
  type KalshiSnapshot,
} from "./kalshi"

const metadataTTL = 60_000
const reportTTL = 5 * 60_000
const datesCache: {
  value?: string[]
  until: number
  pending?: Promise<string[]>
} = { until: 0 }
const snapshotsCache = new Map<
  string,
  { value: KalshiSnapshot[]; until: number }
>()
const reportCache = new Map<
  string,
  { value: KalshiReport | null; until: number }
>()
const pendingReports = new Map<string, Promise<KalshiReport | null>>()

export function seedKalshiMetadata(
  dates: string[],
  date: string,
  snapshots: KalshiSnapshot[]
) {
  if (typeof window === "undefined") return
  datesCache.value = dates
  datesCache.until = Date.now() + metadataTTL
  snapshotsCache.set(date, {
    value: snapshots,
    until: Date.now() + metadataTTL,
  })
}

export async function cachedKalshiDates(): Promise<string[]> {
  if (datesCache.value && datesCache.until > Date.now()) return datesCache.value
  if (!datesCache.pending) {
    datesCache.pending = getKalshiDates()
      .then(({ dates }) => {
        datesCache.value = dates
        datesCache.until = Date.now() + metadataTTL
        return dates
      })
      .finally(() => {
        datesCache.pending = undefined
      })
  }
  return datesCache.pending
}

export async function cachedKalshiSnapshots(
  date: string
): Promise<KalshiSnapshot[]> {
  const cached = snapshotsCache.get(date)
  if (cached && cached.until > Date.now()) return cached.value
  const { snapshots } = await getKalshiSnapshots(date)
  snapshotsCache.set(date, {
    value: snapshots,
    until: Date.now() + metadataTTL,
  })
  return snapshots
}

export function peekKalshiReport(
  date: string,
  slot: KalshiSlot
): KalshiReport | null | undefined {
  const cached = reportCache.get(`${date}:${slot}`)
  return cached && cached.until > Date.now() ? cached.value : undefined
}

export function cachedKalshiReport(
  date: string,
  slot: KalshiSlot
): Promise<KalshiReport | null> {
  const key = `${date}:${slot}`
  const hit = peekKalshiReport(date, slot)
  if (hit !== undefined) return Promise.resolve(hit)
  const pending = pendingReports.get(key)
  if (pending) return pending
  const request = getKalshiReport(date, slot)
    .then((response) => {
      if (
        response.date !== date ||
        response.slot !== slot ||
        (response.published && response.manifest.metadata.run_slot !== slot)
      ) {
        throw new Error("Kalshi report slot mismatch")
      }
      const report = response.published ? response : null
      reportCache.delete(key)
      reportCache.set(key, { value: report, until: Date.now() + reportTTL })
      if (reportCache.size > 6)
        reportCache.delete(reportCache.keys().next().value!)
      return report
    })
    .finally(() => {
      pendingReports.delete(key)
    })
  pendingReports.set(key, request)
  return request
}
