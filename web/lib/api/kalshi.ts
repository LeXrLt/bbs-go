import { apiFetch, type ApiRequestOptions } from "./client"

export type KalshiRow = Record<string, unknown>

export interface KalshiPayload {
  rows: KalshiRow[]
  meta: Record<string, unknown>
}

export interface KalshiReport {
  date: string
  as_of: string
  updated_at: string
  manifest: {
    schema_version: string
    date: string
    as_of: string
    publication_ready: boolean
    collection_status: string
    counts: Record<string, number>
    metadata: Record<string, unknown>
    files: Record<string, { bytes: number; sha256: string }>
  }
  signals: KalshiPayload
  trends: KalshiPayload
  narrative_html: string
  content_html: string
  markdown: string
}

type ReadOptions = Pick<ApiRequestOptions, "request" | "signal">

export function getKalshiDates(options: ReadOptions = {}) {
  return apiFetch<{ dates: string[] }>("/api/kalshi/dates", options)
}

export function getKalshiReport(date: string, options: ReadOptions = {}) {
  return apiFetch<KalshiReport>("/api/kalshi/report", {
    ...options,
    params: { date },
  })
}
