import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"
import { resolve } from "node:path"

const webRoot = resolve(fileURLToPath(new URL("..", import.meta.url)))
const draftSource = readFileSync(resolve(webRoot, "lib/composer-draft.ts"), "utf8")

assert.match(draftSource, /window\.localStorage\.getItem\(key\)/)
assert.match(draftSource, /window\.localStorage\.setItem\(key, JSON\.stringify\(stored\)\)/)
assert.match(draftSource, /window\.localStorage\.removeItem\(key\)/)
assert.match(draftSource, /encodeURIComponent\(String\(userId\)\)/)
assert.match(draftSource, /stored\.version !== DRAFT_VERSION/)
assert.match(draftSource, /useComposerDraft/)

const formFiles = [
  "components/topic/topic-create-form.tsx",
  "components/topic/topic-edit-form.tsx",
  "components/article/article-form.tsx",
]

for (const file of formFiles) {
  const source = readFileSync(resolve(webRoot, file), "utf8")
  assert.match(source, /useComposerDraft/, `${file} should persist editor state`)
  assert.match(source, /draft\.clear\(\)/, `${file} should clear after publishing`)
}

console.log("composer draft persistence is covered")

