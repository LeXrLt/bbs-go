import { Readable } from "node:stream"
import { pipeline } from "node:stream/promises"
import { createGzip } from "node:zlib"

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

export function compressKalshiReport(pathname, method, requestHeaders, response) {
  if (
    pathname !== "/api/kalshi/report" ||
    method !== "GET" ||
    !response.ok ||
    !response.body ||
    !response.headers.get("content-type")?.includes("application/json")
  ) return false

  return (requestHeaders["accept-encoding"] || "")
    .split(",")
    .some((entry) => {
      const [encoding, ...parameters] = entry.trim().toLowerCase().split(";")
      return encoding === "gzip" && !parameters.some((param) =>
        /^q\s*=\s*0(?:\.0*)?$/.test(param.trim())
      )
    })
}

export function forwardUpstreamHeaders(pathname, response, target, { gzip = false } = {}) {
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

  if (pathname === "/api/kalshi/report") {
    target.setHeader("Cache-Control", "private, no-store")
    const vary = response.headers.get("vary")
    if (!vary?.split(",").some((value) => value.trim().toLowerCase() === "accept-encoding")) {
      target.setHeader("Vary", vary ? `${vary}, Accept-Encoding` : "Accept-Encoding")
    }
    if (gzip) {
      target.setHeader("Content-Encoding", "gzip")
    }
  }
}

export async function streamUpstreamBody(response, target, { gzip = false } = {}) {
  if (!response.body) {
    target.end()
    return
  }
  if (gzip) {
    await pipeline(Readable.fromWeb(response.body), createGzip(), target)
  } else {
    await pipeline(Readable.fromWeb(response.body), target)
  }
}
