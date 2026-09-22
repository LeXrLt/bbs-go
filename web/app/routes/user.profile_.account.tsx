import { useAppState } from "@/components/app/app-provider"
import { RequireUser } from "@/components/auth/require-user"
import { WidgetCard } from "@/components/common/widget-card"
import { AccountSettings } from "@/components/user/account-settings"
import { ProfileBackLink } from "@/components/user/profile-back-link"
import { ProfileShell } from "@/components/user/profile-shell"
import { useI18n } from "@/lib/i18n/provider"
import { noindexRouteMeta } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"
import { AUTH_COOKIE } from "@/lib/cookies"
import { rootDataContext } from "../route-helpers/context"
import { useLoaderData, type RouterContextProvider } from "react-router"

import { requireUser, requireUserClient } from "../route-helpers/auth"

function readCookie(request: Request, name: string) {
  const cookieHeader = request.headers.get("cookie") || ""
  for (const part of cookieHeader.split(";")) {
    const separator = part.indexOf("=")
    if (separator < 0) continue
    const key = part.slice(0, separator).trim()
    if (key !== name) continue
    const value = part.slice(separator + 1).trim()
    try {
      return decodeURIComponent(value)
    } catch {
      return value
    }
  }
  return ""
}

type AccountRouteData = {
  bbsToken: string
  baseURL: string
}

export async function loader({
  request,
  context,
}: {
  request: Request
  context?: RouterContextProvider
}): Promise<AccountRouteData> {
  await requireUser({ request, context })
  const rootDataProvider = context?.get(rootDataContext)
  const rootData = rootDataProvider ? await rootDataProvider() : null
  return {
    bbsToken: readCookie(request, AUTH_COOKIE),
    baseURL: rootData?.config?.baseURL || new URL(request.url).origin,
  }
}

export async function clientLoader({
  request,
  serverLoader,
}: {
  request: Request
  serverLoader: <T = unknown>() => Promise<T>
}) {
  await requireUserClient({ request })
  return serverLoader<AccountRouteData>()
}

export function meta({
  matches,
}: {
  matches: Array<{ data?: unknown; loaderData?: unknown }>
}) {
  return noindexRouteMeta(matches, "Account settings", "账号设置")
}

export default function AccountRoute() {
  const { t } = useI18n()
  useDocumentTitle(t("user.profile.account.title"))
  const { config, currentUser } = useAppState()
  const routeData = useLoaderData<typeof loader>()
  return (
    <RequireUser initialUser={currentUser} redirectPath="/user/profile/account">
      <ProfileShell active="account" t={t}>
        <WidgetCard
          title={t("user.profile.account.title")}
          actions={<ProfileBackLink />}
        >
          <AccountSettings
            user={currentUser || undefined}
            config={config}
            bindInfo={{}}
            skillSetup={routeData}
          />
        </WidgetCard>
      </ProfileShell>
    </RequireUser>
  )
}
