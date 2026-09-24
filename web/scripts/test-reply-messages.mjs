import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { fileURLToPath } from "node:url"

const webRoot = resolve(fileURLToPath(new URL("..", import.meta.url)))
const header = readFileSync(
  resolve(webRoot, "components/layout/site-header.tsx"),
  "utf8"
)
const privateCenter = readFileSync(
  resolve(webRoot, "components/user/private-user-center-page.tsx"),
  "utf8"
)
const handler = readFileSync(
  resolve(webRoot, "../internal/handlers/api/user_handlers.go"),
  "utf8"
)
const messageTypes = readFileSync(
  resolve(webRoot, "../internal/pkg/msg/data.go"),
  "utf8"
)

assert.match(header, /onOpenChange=\{setOpen\}/)
assert.match(header, /openDelay=\{100\}/)
assert.match(header, /onClick=\{\(\) => setOpen\(false\)\}/)
assert.match(header, /href="\/user\/messages"/)
assert.match(header, /common\.header\.repliesToMe/)
assert.match(header, /messageCountLabel\(count\)/)

assert.match(privateCenter, /showCounts=\{kind !== "messages"\}/)
assert.match(privateCenter, /showBadges=\{kind !== "messages"\}/)

for (const replyType of [
  "TypeTopicComment",
  "TypeCommentReply",
  "TypeArticleComment",
]) {
  assert.match(messageTypes, new RegExp(`int\\(${replyType}\\)`))
}
assert.match(handler, /In\("type", msg\.ReplyTypes\)/)
assert.match(handler, /GetUnreadReplyCount\(user\.Id\)/)
assert.match(handler, /MarkRepliesRead\(user\.Id\)/)

console.log("reply message checks passed")
