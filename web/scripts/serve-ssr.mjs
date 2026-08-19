import { createServer } from "node:http"
import { createReadStream, existsSync, statSync } from "node:fs"
import path from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"
import { createRequestListener } from "@react-router/node"

import {
  forwardUpstreamHeaders,
  streamUpstreamBody,
  upstreamRequestHeaders,
} from "./http-proxy.mjs"

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
const envPath = path.join(root, ".env")
if (existsSync(envPath)) {
  process.loadEnvFile(envPath)
}

const clientDir = path.join(root, "build/client")
const serverBuildPath = path.join(root, "build/server/index.js")
const port = Number(process.env.PORT || 3000)
const serverURL = process.env.BBSGO_SERVER_URL || process.env.SERVER_URL
if (!serverURL) {
  throw new Error("BBSGO_SERVER_URL is required. Set it in web/.env.")
}

const build = await import(pathToFileURL(serverBuildPath).href)
const frameworkRequestListener = createRequestListener({
  build,
  mode: process.env.NODE_ENV || "production",
})

function contentType(file) {
  if (file.endsWith(".js") || file.endsWith(".mjs")) return "text/javascript"
  if (file.endsWith(".css")) return "text/css"
  if (file.endsWith(".svg")) return "image/svg+xml"
  if (file.endsWith(".png")) return "image/png"
  if (file.endsWith(".ico")) return "image/x-icon"
  if (file.endsWith(".json")) return "application/json"
  return "application/octet-stream"
}

function resolveStaticFile(pathname) {
  let decodedPath
  try {
    decodedPath = decodeURIComponent(pathname)
  } catch {
    return null
  }

  const filePath = path.resolve(clientDir, `.${decodedPath}`)
  const relativePath = path.relative(clientDir, filePath)
  if (
    relativePath.startsWith("..") ||
    path.isAbsolute(relativePath) ||
    relativePath === ""
  ) {
    return null
  }

  try {
    return statSync(filePath).isFile() ? filePath : null
  } catch {
    return null
  }
}

function shouldProxyToServer(pathname) {
  return (
    pathname.startsWith("/api/") ||
    pathname.startsWith("/res/") ||
    pathname === "/sitemap.xml"
  )
}

createServer(async (req, res) => {
  const url = new URL(req.url || "/", `http://${req.headers.host}`)

  if (shouldProxyToServer(url.pathname)) {
    const abortController = new AbortController()
    const abortUpstream = () => {
      if (!res.writableEnded) abortController.abort()
    }
    req.once("aborted", abortUpstream)
    res.once("close", abortUpstream)

    try {
      const upstream = new URL(`${url.pathname}${url.search}`, serverURL)
      const response = await fetch(upstream, {
        method: req.method,
        headers: upstreamRequestHeaders(req.headers),
        body: req.method === "GET" || req.method === "HEAD" ? undefined : req,
        duplex: "half",
        signal: abortController.signal,
      })

      res.statusCode = response.status
      forwardUpstreamHeaders(url.pathname, response, res)
      await streamUpstreamBody(response, res)
    } catch (error) {
      if (abortController.signal.aborted || res.destroyed) return
      if (res.headersSent) {
        res.destroy()
        return
      }
      res.statusCode = 502
      res.end("Bad Gateway")
      console.error("upstream proxy failed", error)
    } finally {
      req.off("aborted", abortUpstream)
      res.off("close", abortUpstream)
    }
    return
  }

  const filePath = url.pathname === "/" ? null : resolveStaticFile(url.pathname)

  if (filePath) {
    res.setHeader("Content-Type", contentType(filePath))
    if (req.method === "HEAD") {
      res.end()
      return
    }
    createReadStream(filePath)
      .on("error", (error) => {
        if (!res.headersSent) {
          res.statusCode = 500
        }
        res.end(error instanceof Error ? error.message : String(error))
      })
      .pipe(res)
    return
  }

  try {
    await frameworkRequestListener(req, res)
  } catch (error) {
    res.statusCode = 500
    res.end(error instanceof Error ? error.stack : String(error))
  }
}).listen(port, () => {
  console.log(`web SSR server listening on http://localhost:${port}`)
})
