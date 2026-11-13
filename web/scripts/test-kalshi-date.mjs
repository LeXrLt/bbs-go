import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { Writable } from "node:stream"
import { gunzipSync } from "node:zlib"

import { addCalendarDays, calendarToday } from "../lib/calendar/index.ts"
import { compressKalshiReport, forwardUpstreamHeaders, streamUpstreamBody } from "./http-proxy.mjs"

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
assert.match(route, /requestedDate \|\| dates\[0\] \|\| today/)
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
assert.match(page, /\/api\/kalshi\/download\?date=/)
assert.doesNotMatch(page, /report\.markdown/)
assert.match(route, /Promise\.all\(\[getKalshiDates\(options\), prefetch\]\)/)

const body = JSON.stringify({ success: true, data: { rows: Array(3000).fill("市场线索") } })
const response = new Response(body, {
  headers: { "content-type": "application/json", "content-length": String(Buffer.byteLength(body)), vary: "Origin" },
})
const path = "/api/kalshi/report"
assert.equal(compressKalshiReport(path, "GET", { "accept-encoding": "br, gzip" }, response), true)
assert.equal(compressKalshiReport(path, "GET", { "accept-encoding": "gzip;q=0, br" }, response), false)
assert.equal(compressKalshiReport(path, "HEAD", { "accept-encoding": "gzip" }, response), false)
assert.equal(compressKalshiReport("/api/kalshi/download", "GET", { "accept-encoding": "gzip" }, response), false)
assert.equal(compressKalshiReport(path, "GET", { "accept-encoding": "gzip" }, new Response("{}", { status: 401 })), false)

const headers = new Map()
forwardUpstreamHeaders(path, response, { setHeader: (name, value) => headers.set(name.toLowerCase(), value) }, { gzip: true })
assert.equal(headers.get("content-length"), undefined)
assert.equal(headers.get("content-encoding"), "gzip")
assert.equal(headers.get("cache-control"), "private, no-store")
assert.equal(headers.get("vary"), "Origin, Accept-Encoding")

const chunks = []
await streamUpstreamBody(response, new Writable({
  write(chunk, _encoding, callback) {
    chunks.push(chunk)
    callback()
  },
}), { gzip: true })
const compressed = Buffer.concat(chunks)
assert.equal(gunzipSync(compressed).toString(), body)
assert.ok(compressed.length < Buffer.byteLength(body) / 5)

console.log("Kalshi date navigation contracts passed")
