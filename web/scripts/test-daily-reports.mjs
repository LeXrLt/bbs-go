import assert from "node:assert/strict"

import {
  dailyReportSearchParams,
  findDailyReportCategory,
  parseDailyReportFilters,
} from "../lib/daily-reports.ts"

const daily = { id: 42, name: "\u65e5\u62a5" }
assert.deepEqual(
  findDailyReportCategory([{ id: 1, name: "Other" }, daily]),
  daily
)
assert.deepEqual(
  findDailyReportCategory([{ id: 1, name: "Work", children: [daily] }]),
  daily
)
assert.equal(findDailyReportCategory([{ id: 5, name: "Other" }]), null)
assert.equal(findDailyReportCategory([]), null)
assert.deepEqual(parseDailyReportFilters(new URLSearchParams()), {
  userIds: [],
  sort: "latestPublish",
})
assert.deepEqual(
  parseDailyReportFilters(
    new URLSearchParams("userIds=B,A,B&sort=latestReply")
  ),
  { userIds: ["A", "B"], sort: "latestReply" }
)
assert.deepEqual(parseDailyReportFilters(new URLSearchParams("sort=invalid")), {
  userIds: [],
  sort: "latestPublish",
})

const selected = dailyReportSearchParams(
  new URLSearchParams("cursor=stale&keep=yes"),
  { userIds: ["B", "A"], sort: "latestReply" }
)
assert.equal(selected.get("cursor"), null)
assert.equal(selected.get("keep"), "yes")
assert.deepEqual(parseDailyReportFilters(selected), {
  userIds: ["A", "B"],
  sort: "latestReply",
})
const cleared = dailyReportSearchParams(selected, {
  userIds: [],
  sort: "latestReply",
})
assert.equal(cleared.has("userIds"), false)
assert.equal(cleared.get("sort"), "latestReply")
console.log("Daily reports category and filter URL tests passed")
