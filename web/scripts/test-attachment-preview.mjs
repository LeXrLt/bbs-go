import assert from "node:assert/strict"
import { existsSync, readFileSync } from "node:fs"
import { resolve } from "node:path"
import { Writable } from "node:stream"

import {
  forwardUpstreamHeaders,
  isAttachmentBinaryPath,
  streamUpstreamBody,
  upstreamRequestHeaders,
} from "./http-proxy.mjs"

const webRoot = resolve(import.meta.dirname, "..")
const read = (path) => readFileSync(resolve(webRoot, path), "utf8")

const createForm = read("components/topic/topic-create-form.tsx")
const editForm = read("components/topic/topic-edit-form.tsx")
const field = read("components/topic/topic-attachment-field.tsx")
const attachments = read("components/topic/topic-attachments.tsx")
const previewPage = read("components/topic/attachment-preview-page.tsx")
const pdfViewer = read("components/topic/attachment-pdf-viewer.tsx")
const attachmentApi = read("lib/api/attachments.ts")
const apiTypes = read("lib/api/types.ts")
const ssrServer = read("scripts/serve-ssr.mjs")
const packageJson = JSON.parse(read("package.json"))
const routePath = resolve(
  webRoot,
  "app/routes/topic.$id_.attachment.$attachmentId.preview.tsx"
)

assert.equal(
  existsSync(routePath),
  true,
  "the attachment preview route must exist"
)
const previewRoute = readFileSync(routePath, "utf8")

for (const form of [createForm, editForm]) {
  assert.match(
    form,
    /import \{ TopicAttachmentField \} from "@\/components\/topic\/topic-attachment-field"/,
    "topic forms must share the attachment field"
  )
  assert.doesNotMatch(
    form,
    /function TopicAttachmentField/,
    "topic forms must not duplicate attachment upload behavior"
  )
}

for (const extension of ["pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx"]) {
  assert.match(
    field,
    new RegExp(`"\\.${extension}"`),
    `${extension} must be accepted`
  )
}
assert.match(field, /file\.size > maxSizeMB \* 1024 \* 1024/)
assert.match(field, /allowedTypes\.includes\(fileExtension\(file\.name\)\)/)
assert.match(field, /value\.length >= maxCount/)

assert.match(apiTypes, /fileType: string/)
assert.match(apiTypes, /previewable: boolean/)
assert.match(apiTypes, /accessGranted: boolean/)
assert.match(
  attachmentApi,
  /attachment\/access\/\$\{encodeURIComponent\(attachmentId\)\}/
)
assert.match(attachmentApi, /method: "POST"/)
assert.match(attachments, /grantAttachmentAccess\(attachment\.id\)/)
assert.match(attachments, /attachment\.previewable/)

assert.match(previewRoute, /await requireUser\(args\)/)
assert.match(previewRoute, /await requireUserClient\(args\)/)
assert.match(previewRoute, /noindexRouteMeta/)
assert.match(previewPage, /React\.lazy/)
assert.match(pdfViewer, /from "react-pdf"/)
assert.match(pdfViewer, /pdf\.worker\.min\.mjs\?url/)
assert.match(pdfViewer, /attachmentPreviewPath\(attachment\.id\)/)
assert.match(pdfViewer, /withCredentials: true/)
assert.doesNotMatch(
  pdfViewer,
  /apiFetch/,
  "PDF binary responses must not use the JSON API client"
)

assert.equal(packageJson.dependencies["react-pdf"], "10.4.1")
assert.match(
  ssrServer,
  /file\.endsWith\("\.js"\) \|\| file\.endsWith\("\.mjs"\)/,
  "the SSR server must serve the module worker with a JavaScript MIME type"
)

assert.equal(isAttachmentBinaryPath("/api/attachment/preview/file-id"), true)
assert.equal(isAttachmentBinaryPath("/api/attachment/download/file-id"), true)
assert.equal(isAttachmentBinaryPath("/api/topic/file-id"), false)

const requestHeaders = upstreamRequestHeaders({
  connection: "keep-alive, X-Proxy-Only",
  expect: "100-continue",
  host: "localhost:3001",
  "keep-alive": "timeout=5",
  "transfer-encoding": "chunked",
  "x-proxy-only": "remove-me",
  "x-request-id": "keep-me",
})
assert.equal(requestHeaders.has("connection"), false)
assert.equal(requestHeaders.has("expect"), false)
assert.equal(requestHeaders.has("host"), false)
assert.equal(requestHeaders.has("keep-alive"), false)
assert.equal(requestHeaders.has("transfer-encoding"), false)
assert.equal(requestHeaders.has("x-proxy-only"), false)
assert.equal(requestHeaders.get("x-request-id"), "keep-me")

function forwardedHeaders(pathname, headers) {
  const forwarded = new Map()
  forwardUpstreamHeaders(
    pathname,
    { headers: new Headers(headers) },
    { setHeader: (key, value) => forwarded.set(key, value) }
  )
  return forwarded
}

const attachmentHeaders = forwardedHeaders("/api/attachment/preview/file-id", {
  "content-length": "10",
  "content-range": "bytes 0-9/100",
  "content-type": "application/pdf",
})
assert.equal(attachmentHeaders.get("content-length"), "10")
assert.equal(attachmentHeaders.get("content-range"), "bytes 0-9/100")

const hopByHopResponseHeaders = forwardedHeaders(
  "/api/attachment/preview/file-id",
  {
    connection: "keep-alive, X-Upstream-Only",
    "keep-alive": "timeout=5",
    "transfer-encoding": "chunked",
    "x-upstream-only": "remove-me",
    "x-response-id": "keep-me",
  }
)
assert.equal(hopByHopResponseHeaders.has("connection"), false)
assert.equal(hopByHopResponseHeaders.has("keep-alive"), false)
assert.equal(hopByHopResponseHeaders.has("transfer-encoding"), false)
assert.equal(hopByHopResponseHeaders.has("x-upstream-only"), false)
assert.equal(hopByHopResponseHeaders.get("x-response-id"), "keep-me")

const genericHeaders = forwardedHeaders("/api/topic/file-id", {
  "content-length": "10",
  "content-type": "application/json",
})
assert.equal(genericHeaders.has("content-length"), false)

const encodedAttachmentHeaders = forwardedHeaders(
  "/api/attachment/download/file-id",
  {
    "content-encoding": "gzip",
    "content-length": "10",
  }
)
assert.equal(encodedAttachmentHeaders.has("content-encoding"), false)
assert.equal(encodedAttachmentHeaders.has("content-length"), false)

let streamed = ""
const streamTarget = new Writable({
  write(chunk, _encoding, callback) {
    streamed += chunk.toString()
    callback()
  },
})
await streamUpstreamBody(new Response("0123456789"), streamTarget)
assert.equal(streamed, "0123456789")

const emptyTarget = new Writable({
  write(_chunk, _encoding, callback) {
    assert.fail("a response without a body must not write any data")
    callback()
  },
})
await streamUpstreamBody(new Response(null), emptyTarget)
assert.equal(emptyTarget.writableEnded, true)

console.log("attachment upload and preview routes are covered")
