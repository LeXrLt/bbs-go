import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const webRoot = resolve(import.meta.dirname, "..")
const topicDetailSource = readFileSync(
  resolve(webRoot, "components/topic/topic-detail-client-page.tsx"),
  "utf8"
)

assert.match(
  topicDetailSource,
  /return \(\s*<MainShell>/,
  "topic detail should use the full-width MainShell layout"
)
assert.doesNotMatch(
  topicDetailSource,
  /<MainShell[\s\S]*?\baside=/,
  "topic detail should not reserve a right sidebar"
)
assert.doesNotMatch(
  topicDetailSource,
  /UserInfo|TopicToc|side-size-360/,
  "topic detail should not render author or table-of-contents sidebar content"
)

console.log("topic detail full-width layout is covered")
