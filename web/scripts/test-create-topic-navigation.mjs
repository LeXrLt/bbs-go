import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const headerSource = readFileSync(
  resolve(import.meta.dirname, "../components/layout/site-header.tsx"),
  "utf8"
)

assert.match(
  headerSource,
  /function CreateTopicButton[\s\S]*?config\?\.modules\?\.topic[\s\S]*?<Button[\s\S]*?asChild[\s\S]*?<Link href="\/topic\/create"/,
  "The publish button should link directly to the normal-topic editor when the topic module is enabled"
)
assert.equal(
  headerSource.includes("function moduleItems"),
  false,
  "The publish button should not build a content-type selection menu"
)
for (const indirectTarget of [
  "/topic/create?type=1",
  "/topic/create?type=2",
  "/article/create",
]) {
  assert.equal(
    headerSource.includes(indirectTarget),
    false,
    `The publish button should not offer ${indirectTarget}`
  )
}
assert.match(
  headerSource,
  /<CreateTopicButton[\s\S]*?onClick=\{closeMobileMenu\}/,
  "The mobile publish button should close the navigation sheet when it opens the editor"
)

console.log("create-topic navigation behavior is covered")
