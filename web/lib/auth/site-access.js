import { getOAuthCallback } from "./oauth-callback.js"

const publicSitePaths = new Set([
  "/install",
  "/user/signin",
  "/user/signup",
  "/user/password/forgot",
  "/user/password/reset",
  "/user/email/verify",
])

function normalizePathname(pathname) {
  if (!pathname || pathname === "/") return pathname || "/"
  return pathname.replace(/\/+$/, "")
}

export function isPublicSitePath(pathname) {
  const normalizedPathname = normalizePathname(pathname)
  if (publicSitePaths.has(normalizedPathname)) return true

  return getOAuthCallback(normalizedPathname)?.kind === "login"
}

export function shouldRequireSiteLogin(loginRequired, currentUser, pathname) {
  return loginRequired === true && !currentUser && !isPublicSitePath(pathname)
}
