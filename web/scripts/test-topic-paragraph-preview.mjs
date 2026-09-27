import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const webRoot = resolve(import.meta.dirname, "..")
const topicListItemSource = readFileSync(
  resolve(webRoot, "components/topic/topic-list-item.tsx"),
  "utf8"
)
const topicContentSource = readFileSync(
  resolve(webRoot, "components/topic/topic-content.tsx"),
  "utf8"
)
const editorStyles = readFileSync(resolve(webRoot, "styles/editor.css"), "utf8")

assert.match(
  topicListItemSource,
  /topic\.type === 0\s*\?\s*"whitespace-pre-line"\s*:\s*"line-clamp-3"/,
  "Regular topic previews should preserve paragraphs without a line clamp"
)

assert.match(
  topicContentSource,
  /\[&_p:not\(:last-child\)\]:mb-3/,
  "Expanded daily report paragraphs should use the standard 0.75rem gap"
)

assert.match(
  editorStyles,
  /\.editor-content \.tiptap p\s*\{[^}]*margin:\s*0 0 0\.75rem;/,
  "The visual editor paragraph gap should match expanded daily reports"
)

console.log("topic paragraph preview display is covered")
