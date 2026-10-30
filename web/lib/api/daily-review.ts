import { apiFetch, type ApiRequestOptions } from "./client"

export const DAILY_REVIEW_KINDS = [
  "focus",
  "shareholding",
  "announcements",
  "movers",
  "buybacks",
  "shorts",
] as const

export type DailyReviewKind = (typeof DAILY_REVIEW_KINDS)[number]
export type DailyReviewStatus = "ready" | "no_items" | "pending"

export interface DailyReviewMetric {
  label: string
  value: string
}

export interface DailyReviewSection {
  kind: DailyReviewKind
  label: string
  icon: string
  status: DailyReviewStatus
  metrics: DailyReviewMetric[]
  updated_at?: string
}

export interface DailyReviewDashboard {
  date: string
  sections: DailyReviewSection[]
}

export interface DailyReviewDates {
  dates: string[]
  latest?: string
}

export interface DailyReviewReport {
  kind: DailyReviewKind
  label: string
  date: string
  title: string
  status: Exclude<DailyReviewStatus, "pending">
  metrics: DailyReviewMetric[]
  content_html: string
  updated_at?: string
}

type ReadOptions = Pick<ApiRequestOptions, "request" | "signal">

export function getDailyReviewDashboard(
  date: string,
  options: ReadOptions = {}
) {
  return apiFetch<DailyReviewDashboard>("/api/daily-review/dashboard", {
    ...options,
    params: { date },
  })
}

export function getDailyReviewDates(options: ReadOptions = {}) {
  return apiFetch<DailyReviewDates>("/api/daily-review/dates", options)
}

export function getDailyReviewReport(
  kind: DailyReviewKind,
  date: string,
  options: ReadOptions = {}
) {
  return apiFetch<DailyReviewReport>(
    `/api/daily-review/report/${encodeURIComponent(kind)}`,
    { ...options, params: { date } }
  )
}
