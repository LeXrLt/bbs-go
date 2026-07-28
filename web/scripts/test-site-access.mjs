import assert from "node:assert/strict"

import {
  isPublicSitePath,
  shouldRequireSiteLogin,
} from "../lib/auth/site-access.js"

for (const path of [
  "/install",
  "/install/",
  "/user/signin",
  "/user/signup",
  "/user/password/forgot",
  "/user/password/reset",
  "/user/email/verify",
  "/user/signin/callback/google",
  "/user/signin/callback/github",
  "/user/signin/callback/weixin",
]) {
  assert.equal(isPublicSitePath(path), true, `${path} must remain public`)
}

for (const path of [
  "/",
  "/topic/abc",
  "/articles",
  "/search",
  "/about",
  "/user/123",
  "/user/signin/callback/google_bind",
  "/user/signin/callback/weixin_bind",
  "/user/signin/callback/unknown",
]) {
  assert.equal(isPublicSitePath(path), false, `${path} must remain protected`)
}

assert.equal(shouldRequireSiteLogin(true, null, "/topic/abc"), true)
assert.equal(shouldRequireSiteLogin(true, { id: 1 }, "/topic/abc"), false)
assert.equal(shouldRequireSiteLogin(false, null, "/topic/abc"), false)
assert.equal(shouldRequireSiteLogin(true, null, "/user/signin"), false)

console.log("private-site access routes are covered")
