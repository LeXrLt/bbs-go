import { apiFetch, type ApiRequestOptions } from "./client"

export type KalshiRow = Record<string, unknown>

export interface KalshiPayload {
  rows: KalshiRow[]
  meta: Record<string, unknown>
}

export interface KalshiReport {
  date: string
  slot: "morning" | "evening"
  published: true
  content_hash: string
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
}

type ReadOptions = Pick<ApiRequestOptions, "request" | "signal">

export type KalshiSlot = "morning" | "evening"
export interface KalshiSnapshot {
  slot: KalshiSlot
  as_of: string
  collection_status: string
}

export function getKalshiDates(options: ReadOptions = {}) {
  return apiFetch<{ dates: string[] }>("/api/kalshi/dates", options)
}

export function getKalshiSnapshots(date: string, options: ReadOptions = {}) {
  return apiFetch<{ date: string; snapshots: KalshiSnapshot[] }>("/api/kalshi/snapshots", {
    ...options,
    params: { date },
  })
}

export function getKalshiReport(date: string, slot: KalshiSlot, options: ReadOptions = {}) {
  return apiFetch<KalshiReport | { date: string; slot: KalshiSlot; published: false }>("/api/kalshi/report", {
    ...options,
    params: { date, slot },
  })
}
