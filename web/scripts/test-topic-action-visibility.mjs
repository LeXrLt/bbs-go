import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const webRoot = resolve(import.meta.dirname, "..")
const topicListItemSource = readFileSync(
  resolve(webRoot, "components/topic/topic-list-item.tsx"),
  "utf8"
)
const topicDetailActionsSource = readFileSync(
  resolve(webRoot, "components/topic/topic-detail-actions.tsx"),
  "utf8"
)

assert.match(
  topicListItemSource,
  /const SHOW_TOPIC_LIST_QUICK_LIKE = false/,
  "topic list should hide the quick-like action by default"
)
assert.match(
  topicDetailActionsSource,
  /const SHOW_TOPIC_DETAIL_LIKE_AND_FAVORITE_ACTIONS = false/,
  "topic detail should hide the like and favorite actions by default"
)

console.log("topic action visibility is covered")
