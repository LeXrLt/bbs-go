import assert from "node:assert/strict"

import { dashboardPasswordResult } from "../components/dashboard/data/dashboard-password-result.js"

assert.deepEqual(dashboardPasswordResult(null), null)
assert.deepEqual(dashboardPasswordResult({}), null)
assert.deepEqual(dashboardPasswordResult({ password: "" }), null)
assert.deepEqual(dashboardPasswordResult({ password: 123 }), null)

assert.deepEqual(dashboardPasswordResult({ password: "reset-secret" }), {
  password: "reset-secret",
})

assert.deepEqual(
  dashboardPasswordResult(
    { password: "initial-secret", ignored: true },
    {
      title: "User created",
      passwordLabel: "Initial password",
    }
  ),
  {
    password: "initial-secret",
    title: "User created",
    passwordLabel: "Initial password",
  }
)

console.log("dashboard password result tests passed")
