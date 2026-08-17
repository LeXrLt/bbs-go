import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const webRoot = resolve(import.meta.dirname, "..")
const topicListItemSource = readFileSync(
  resolve(webRoot, "components/topic/topic-list-item.tsx"),
  "utf8"
)

assert.match(
  topicListItemSource,
  /topic\.type === 0\s*\?\s*"whitespace-pre-line"\s*:\s*"line-clamp-3"/,
  "Regular topic previews should preserve paragraphs without a line clamp"
)

console.log("topic paragraph preview display is covered")
