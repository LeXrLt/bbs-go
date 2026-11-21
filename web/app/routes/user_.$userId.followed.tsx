import { redirect } from "react-router"
import { UserFollowedClientPage } from "@/components/user/user-profile-client-page"
import { SHOW_COMMUNITY_EXTRAS } from "@/lib/features/community-extras"
import { localizedTitle, rootDataFromMatches, userMeta } from "@/lib/seo"

import {
  loadUserProfileRouteData,
  type UserProfileRouteData,
} from "../route-helpers/user-profile"

type RouteArgs = {
  request: Request
  params: { userId?: string }
}

export async function loader({ request, params }: RouteArgs) {
  if (!SHOW_COMMUNITY_EXTRAS) throw redirect(`/user/${params.userId || ""}`)
  return loadUserProfileRouteData({ request, userId: params.userId || "" })
}

export async function clientLoader({ params }: RouteArgs) {
  if (!SHOW_COMMUNITY_EXTRAS) throw redirect(`/user/${params.userId || ""}`)
  return loadUserProfileRouteData({ userId: params.userId || "" })
}

export function meta({
  data,
  location,
  matches,
}: {
  data?: UserProfileRouteData
  location: { pathname: string }
  matches: Array<{ data?: unknown; loaderData?: unknown }>
}) {
  const rootData = rootDataFromMatches(matches)
  return userMeta(
    rootData?.config,
    data?.user,
    localizedTitle(rootData?.locale, "Following", "关注"),
    location.pathname
  )
}

export default UserFollowedClientPage
