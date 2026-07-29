import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

import {
  COMMENT_CONTENT_TYPE,
  buildCommentCreateFields,
} from "../components/comment/comment-payload.ts"

const webRoot = resolve(import.meta.dirname, "..")
const commentSource = readFileSync(
  resolve(webRoot, "components/comment/index.tsx"),
  "utf8"
)
const editorSource = readFileSync(
  resolve(webRoot, "components/comment/text-editor.tsx"),
  "utf8"
)
const typesSource = readFileSync(resolve(webRoot, "lib/api/types.ts"), "utf8")
const contentStyles = readFileSync(
  resolve(webRoot, "styles/content.css"),
  "utf8"
)

function sourceBetween(source, start, end) {
  const startIndex = source.indexOf(start)
  const endIndex = source.indexOf(end, startIndex + start.length)
  assert.notEqual(startIndex, -1, `missing source marker: ${start}`)
  assert.notEqual(endIndex, -1, `missing source marker: ${end}`)
  return source.slice(startIndex, endIndex)
}

const uploadFilesSource = sourceBetween(
  editorSource,
  "const uploadFiles",
  "function openImagePicker"
)
const pasteHandlerSource = sourceBetween(
  editorSource,
  "function onPaste",
  "function onDrop"
)
const dropHandlerSource = sourceBetween(
  editorSource,
  "function onDrop",
  "function onBlur"
)

assert.equal(COMMENT_CONTENT_TYPE, "markdown")
assert.deepEqual(
  buildCommentCreateFields({
    entityType: "comment",
    entityId: 42,
    quoteId: 7,
    content: "![remote](https://images.example.test/a.png)",
    imageList: [{ url: "/uploads/local.png" }],
  }),
  {
    entityType: "comment",
    entityId: 42,
    quoteId: 7,
    contentType: "markdown",
    content: "![remote](https://images.example.test/a.png)",
    imageList: JSON.stringify([{ url: "/uploads/local.png" }]),
  }
)

assert.equal(
  commentSource.match(/buildCommentCreateFields\(\{/g)?.length,
  3,
  "top-level comments and both reply paths should share the Markdown payload builder"
)
assert.match(
  commentSource,
  /<CommentContent comment=\{comment\.quote\} size="quote" \/>/,
  "quoted replies should use the shared safe HTML renderer"
)
assert.match(
  commentSource,
  /comment\.contentType === "text" && "whitespace-pre-wrap"/,
  "escaped legacy text should use the same safe HTML renderer while preserving line breaks"
)
assert.match(
  commentSource,
  /return <HtmlImagePreview html=\{comment\.content\} className=\{className\} \/>/,
  "all comment content sizes should consume server-rendered safe HTML"
)
assert.match(editorSource, /<MarkdownEditor[\s\S]*?compact/)
assert.match(editorSource, /onPasteCapture=\{onPaste\}/)
assert.match(
  pasteHandlerSource,
  /event\.preventDefault\(\)[\s\S]*?event\.stopPropagation\(\)/,
  "image paste should not also reach the Markdown editor upload handler"
)
assert.match(
  uploadFilesSource,
  /if \(disabled\) \{[\s\S]*?return/,
  "comment attachment uploads should be blocked while a comment is being sent"
)
assert.match(pasteHandlerSource, /if \(disabled\) \{[\s\S]*?return/)
assert.match(dropHandlerSource, /if \(disabled\) \{[\s\S]*?return/)
assert.match(
  editorSource,
  /aria-label=\{t\("component\.textEditor\.removeImage"\)\}[\s\S]*?disabled=\{disabled\}/,
  "comment attachments should not be removable while a comment is being sent"
)
assert.equal(
  editorSource.match(
    /aria-label=\{t\("component\.textEditor\.addImage"\)\}[\s\S]{0,160}?disabled=\{disabled\}/g
  )?.length,
  2,
  "both comment image buttons should be disabled while a comment is being sent"
)
assert.match(editorSource, /onDropCapture=\{onDrop\}/)
assert.match(editorSource, /event\.metaKey \|\| event\.ctrlKey/)
assert.match(editorSource, /const COMMENT_IMAGE_LIMIT = 9/)
assert.match(typesSource, /"text" \| "html" \| "markdown"/)
assert.match(contentStyles, /\.bbs-comment-content/)
assert.match(contentStyles, /\.comment-markdown-editor/)
assert.match(
  readFileSync(resolve(webRoot, "components/editor/markdown-editor.tsx"), "utf8"),
  /onUploadImg=\{compact \? undefined : uploadImg\}/,
  "compact comment editors should not register the built-in image uploader"
)
assert.match(
  contentStyles,
  /\.comment-markdown-editor \.md-editor-toolbar-wrapper \{[\s\S]*?overflow-x: auto/,
  "compact toolbar should scroll horizontally instead of overflowing"
)

console.log("comment Markdown tests passed")
