import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

import {
  addCalendarDays,
  calendarSearchParams,
  calendarToday,
  calendarWeek,
  formatBeijingTime,
  isCalendarDate,
  mergeCalendarSearchState,
  millisecondsUntilNextCalendarMidnight,
  parseCalendarSearchParams,
  safeHttpsUrl,
  startOfCalendarWeek,
} from "../lib/calendar/index.ts"

assert.equal(calendarToday(new Date("2026-08-14T15:59:59Z")), "2026-08-14")
assert.equal(calendarToday(new Date("2026-08-14T16:00:00Z")), "2026-08-15")
assert.equal(
  millisecondsUntilNextCalendarMidnight(new Date("2026-08-14T15:59:59.000Z")),
  1_000
)
assert.equal(
  millisecondsUntilNextCalendarMidnight(new Date("2026-08-14T16:00:00.000Z")),
  24 * 60 * 60 * 1_000
)

assert.equal(isCalendarDate("2024-02-29"), true)
assert.equal(isCalendarDate("2026-02-29"), false)
assert.equal(isCalendarDate("2026-13-01"), false)
assert.equal(addCalendarDays("2026-08-31", 1), "2026-09-01")
assert.equal(startOfCalendarWeek("2026-08-16"), "2026-08-10")
assert.deepEqual(calendarWeek("2026-08-15"), [
  "2026-08-10",
  "2026-08-11",
  "2026-08-12",
  "2026-08-13",
  "2026-08-14",
  "2026-08-15",
  "2026-08-16",
])

assert.deepEqual(
  parseCalendarSearchParams(
    new URLSearchParams(
      "date=2026-08-15&kind=earnings&minImportance=2&q=%20AAPL%20"
    )
  ),
  {
    date: "2026-08-15",
    kind: "earnings",
    minImportance: 2,
    keyword: "AAPL",
  }
)

assert.deepEqual(
  parseCalendarSearchParams(
    new URLSearchParams("date=invalid&kind=other&minImportance=9"),
    new Date("2026-08-14T16:00:00Z")
  ),
  {
    date: "2026-08-15",
    kind: "",
    minImportance: "",
    keyword: "",
  }
)

const canonical = calendarSearchParams(
  new URLSearchParams("foo=bar&cursor=stale"),
  {
    date: "2026-08-15",
    kind: "ipo",
    minImportance: 3,
    keyword: "listing",
  }
)
assert.equal(canonical.get("date"), "2026-08-15")
assert.equal(canonical.get("kind"), "ipo")
assert.equal(canonical.get("minImportance"), "3")
assert.equal(canonical.get("q"), "listing")
assert.equal(canonical.get("cursor"), null)
assert.equal(canonical.get("foo"), "bar")

let desired = {
  query: parseCalendarSearchParams(
    new URLSearchParams("date=2026-08-15&foo=preserved")
  ),
  searchParams: new URLSearchParams("date=2026-08-15&foo=preserved"),
}
desired = mergeCalendarSearchState(desired, { date: "2026-08-22" })
desired = mergeCalendarSearchState(desired, { kind: "earnings" })
desired = mergeCalendarSearchState(desired, { minImportance: 2 })
desired = mergeCalendarSearchState(desired, { keyword: "AAPL" })
assert.deepEqual(desired.query, {
  date: "2026-08-22",
  kind: "earnings",
  minImportance: 2,
  keyword: "AAPL",
})
assert.equal(
  desired.searchParams.toString(),
  "date=2026-08-22&foo=preserved&kind=earnings&minImportance=2&q=AAPL"
)

assert.match(
  formatBeijingTime("2026-08-14T16:30:00Z", "zh-CN", true) || "",
  /2026.*8.*15.*00:30/
)
assert.equal(
  safeHttpsUrl("https://example.com/event?id=1"),
  "https://example.com/event?id=1"
)
assert.equal(safeHttpsUrl("http://example.com/event"), null)
assert.equal(safeHttpsUrl("javascript:alert(1)"), null)
assert.equal(safeHttpsUrl("https://user:secret@example.com/event"), null)

const repoRoot = resolve(import.meta.dirname, "../..")
const routeSource = readFileSync(
  resolve(repoRoot, "web/app/routes/calendar.tsx"),
  "utf8"
)
const apiSource = readFileSync(
  resolve(repoRoot, "web/lib/api/calendar.ts"),
  "utf8"
)
const workbenchSource = readFileSync(
  resolve(repoRoot, "web/components/calendar/calendar-workbench.tsx"),
  "utf8"
)
const dateNavigatorSource = readFileSync(
  resolve(repoRoot, "web/components/calendar/date-navigator.tsx"),
  "utf8"
)

assert.match(routeSource, /export async function loader/)
assert.match(routeSource, /export async function clientLoader/)
assert.match(apiSource, /\/api\/calendar\/events/)
assert.match(
  apiSource,
  /error instanceof ApiError && error\.status === CALENDAR_CURSOR_STALE_STATUS/
)
assert.doesNotMatch(
  routeSource,
  /localStorage|MdPreview|\/api\/feed|content_md/
)
assert.match(
  workbenchSource,
  /dataMatchesQuery\s*&&\s*eventsQueryKey === currentQueryKey/
)
assert.match(workbenchSource, /setEventsQueryKey\(dataQueryKey\)/)
assert.match(workbenchSource, /!resultsAreCurrent \? \(\s*<CalendarPending/)
assert.match(
  workbenchSource,
  /aria-hidden="true"\s+inert\s+className="pointer-events-none invisible select-none"/
)
assert.match(
  workbenchSource,
  /event=\{\s*resultsAreCurrent \? detailEvent \|\| selectedEvent : null\s*\}/
)
assert.match(
  workbenchSource,
  /if \(isCalendarCursorStaleError\(error\)\) \{[\s\S]*?setCursor\(""\)[\s\S]*?setHasMore\(false\)[\s\S]*?setLoadMoreFailed\(false\)[\s\S]*?revalidator\.revalidate\(\)/
)
assert.match(
  dateNavigatorSource,
  /calendarDateToLocalDate\(item\)\?\.getDate\(\)/
)
assert.match(
  dateNavigatorSource,
  /<span className="[^"]*whitespace-nowrap[^"]*">\s*\{dayNumber \?\? ""\}/
)

console.log("calendar route and date contract tests passed")
