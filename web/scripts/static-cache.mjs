const immutableAsset = /^\/assets\/[^/]+-[A-Za-z0-9_-]{8,}\.(?:css|js|mjs|png|jpe?g|gif|webp|svg|woff2?)$/

export function staticCacheControl(pathname) {
  return immutableAsset.test(pathname)
    ? "public, max-age=31536000, immutable"
    : null
}
