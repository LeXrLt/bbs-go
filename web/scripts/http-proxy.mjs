import { Readable } from "node:stream"
import { pipeline } from "node:stream/promises"

const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
])

function connectionHeaders(headers) {
  return new Set(
    (headers.get("connection") || "")
      .split(",")
      .map((value) => value.trim().toLowerCase())
      .filter(Boolean)
  )
}

export function upstreamRequestHeaders(requestHeaders) {
  const headers = new Headers(requestHeaders)
  const connectionSpecific = connectionHeaders(headers)

  for (const key of HOP_BY_HOP_HEADERS) headers.delete(key)
  for (const key of connectionSpecific) headers.delete(key)
  headers.delete("expect")
  headers.delete("host")

  return headers
}

export function isAttachmentBinaryPath(pathname) {
  return (
    pathname.startsWith("/api/attachment/preview/") ||
    pathname.startsWith("/api/attachment/download/")
  )
}

export function forwardUpstreamHeaders(pathname, response, target) {
  const preserveContentLength =
    isAttachmentBinaryPath(pathname) &&
    !response.headers.has("content-encoding")
  const connectionSpecific = connectionHeaders(response.headers)

  response.headers.forEach((value, key) => {
    if (
      HOP_BY_HOP_HEADERS.has(key) ||
      connectionSpecific.has(key) ||
      key === "content-encoding" ||
      (key === "content-length" && !preserveContentLength)
    ) {
      return
    }
    target.setHeader(key, value)
  })
}

export async function streamUpstreamBody(response, target) {
  if (!response.body) {
    target.end()
    return
  }
  await pipeline(Readable.fromWeb(response.body), target)
}
