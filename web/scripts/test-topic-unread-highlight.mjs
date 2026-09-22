import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const webRoot = resolve(import.meta.dirname, "..")
const source = readFileSync(
  resolve(webRoot, "components/topic/topic-list-item.tsx"),
  "utf8"
)

assert.match(source, /Mail[\s\S]*function UnreadTopicMark/)
assert.match(source, /\/api\/topic\/mark_read\/\$\{topic\.id\}/)
assert.match(source, /method: "POST"/)
assert.match(source, /unread && "bg-accent\/45"/)
assert.equal(
  source.match(/<UnreadTopicMark t=\{t\} \/>/g)?.length,
  2,
  "Both compact and default topic lists should render the unread icon"
)
assert.ok(
  (source.match(/onClick=\{handleTopicOpen\}/g) || []).length >= 5,
  "Every topic-opening affordance should mark the current topic as read"
)
assert.match(
  source,
  /const fullTopic = await apiFetch<Topic>[\s\S]*setExpandedTopic\(fullTopic\)[\s\S]*setUnread\(false\)/,
  "A successfully expanded server response should clear the current unread marker"
)

console.log("topic unread highlighting and read interactions are covered")
