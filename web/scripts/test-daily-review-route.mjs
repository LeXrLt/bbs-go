import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

import { addCalendarDays, calendarToday } from "../lib/calendar/index.ts"

assert.equal(calendarToday(new Date("2026-10-27T15:59:59Z")), "2026-10-27")
assert.equal(calendarToday(new Date("2026-10-27T16:00:00Z")), "2026-10-28")
assert.equal(addCalendarDays("2026-10-31", 1), "2026-11-01")
assert.equal(
  addCalendarDays(calendarToday(new Date("2026-11-01T02:00:00Z")), -1),
  "2026-10-31"
)

const repoRoot = resolve(import.meta.dirname, "../..")
const routeSource = readFileSync(
  resolve(repoRoot, "web/app/routes/daily-review.tsx"),
  "utf8"
)
const apiSource = readFileSync(
  resolve(repoRoot, "web/lib/api/daily-review.ts"),
  "utf8"
)
const pageSource = readFileSync(
  resolve(repoRoot, "web/components/daily-review/daily-review-page.tsx"),
  "utf8"
)
const dateSource = readFileSync(
  resolve(repoRoot, "web/lib/daily-review.ts"),
  "utf8"
)

assert.match(routeSource, /export async function loader/)
assert.match(routeSource, /export async function clientLoader/)
assert.match(routeSource, /loadCategories\(request\)/)
assert.match(apiSource, /\/api\/daily-review\/dashboard/)
assert.match(apiSource, /\/api\/daily-review\/dates/)
assert.match(
  apiSource,
  /\/api\/daily-review\/report\/\$\{encodeURIComponent\(kind\)\}/
)
assert.deepEqual(
  Array.from(
    apiSource.matchAll(
      /"(shareholding|announcements|movers|focus|buybacks|shorts)"/g
    ),
    (match) => match[1]
  ),
  ["shareholding", "announcements", "movers", "focus", "buybacks", "shorts"]
)
assert.match(pageSource, /data\.dashboard\.sections\.map/)
assert.match(pageSource, /dailyReviewSearchParams/)
assert.match(
  pageSource,
  /<TopicsNavContent initialCategories=\{data\.categories\}/
)
assert.match(pageSource, /html=\{report\.content_html\}/)
assert.match(pageSource, /section\.kind === "announcements"/)
assert.match(pageSource, /function AnnouncementReviewContent/)
assert.match(pageSource, /new DOMParser\(\)/)
assert.match(pageSource, /<details[\s\S]*?key=\{entry\.id\}/)
assert.doesNotMatch(
  pageSource,
  /iframe|daily-review-dashboard|127\.0\.0\.1:8089/
)
assert.doesNotMatch(pageSource, /markdown/i)
assert.match(dateSource, /isCalendarDate\(value\) && value <= today/)
assert.match(dateSource, /const defaultDate = addDailyReviewDays\(today, -1\)/)
assert.match(dateSource, /value : defaultDate/)
assert.match(dateSource, /next\.set\("date", query\.date\)/)

console.log("daily review route, date, and six-section contracts passed")
