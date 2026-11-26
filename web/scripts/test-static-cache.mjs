import assert from "node:assert/strict"

import { staticCacheControl } from "./static-cache.mjs"

for (const pathname of [
  "/assets/root-CpAhZXvH.js",
  "/assets/markdown-editor-BX5Z6h4q.css",
  "/assets/font-3Fno_cA_.woff2",
]) {
  assert.equal(staticCacheControl(pathname), "public, max-age=31536000, immutable")
}

for (const pathname of [
  "/robots.txt",
  "/logo.png",
  "/assets/app.js",
  "/assets/avatar.png",
  "/assets/nested/root-CpAhZXvH.js",
  "/api/topic/topics",
]) {
  assert.equal(staticCacheControl(pathname), null)
}

console.log("Static asset cache policy passed")
