/**
 * @param {unknown} result
 * @param {{ title: string, passwordLabel: string }} [display]
 * @returns {{ password: string, title?: string, passwordLabel?: string } | null}
 */
export function dashboardPasswordResult(result, display) {
  if (
    !result ||
    typeof result !== "object" ||
    !("password" in result) ||
    typeof result.password !== "string" ||
    result.password === ""
  ) {
    return null
  }

  return display
    ? { password: result.password, ...display }
    : { password: result.password }
}
