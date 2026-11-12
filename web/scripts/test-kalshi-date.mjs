import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

import { addCalendarDays, calendarToday } from "../lib/calendar/index.ts"

assert.equal(calendarToday(new Date("2026-11-10T15:59:59Z")), "2026-11-10")
assert.equal(calendarToday(new Date("2026-11-10T16:00:00Z")), "2026-11-11")
assert.equal(addCalendarDays("2026-10-31", 1), "2026-11-01")
assert.equal(addCalendarDays("2026-11-01", -1), "2026-10-31")

const repoRoot = resolve(import.meta.dirname, "../..")
const route = readFileSync(resolve(repoRoot, "web/app/routes/kalshi.tsx"), "utf8")
const page = readFileSync(
  resolve(repoRoot, "web/components/kalshi/kalshi-page.tsx"),
  "utf8"
)

assert.match(route, /isCalendarDate\(requested\) && requested <= today/)
assert.match(route, /: dates\[0\] \|\| today/)
assert.match(route, /!dates\.includes\(date\)/)
assert.match(page, /addCalendarDays\(data\.date, -1\)/)
assert.match(page, /addCalendarDays\(data\.date, 1\)/)
assert.match(page, /changeDate\(today\)/)
assert.match(page, /latest && void navigate\(`/)
assert.match(page, /type="date"/)
assert.match(page, /max=\{today\}/)
assert.match(page, /暂无已发布的 Kalshi 报告/)
assert.match(route, /getKalshiSnapshots\(date, options\)/)
assert.match(route, /response\.slot !== slot/)
assert.match(page, /kalshi-\$\{report\.date\}-\$\{report\.slot\}\.md/)

console.log("Kalshi date navigation contracts passed")
