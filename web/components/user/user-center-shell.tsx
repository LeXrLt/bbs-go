import { UserProfileCard } from "@/components/user/user-profile-card"
import { UserCenterSidebar } from "@/components/user/user-sidebar"
import type { Badge, UserSummary } from "@/lib/api/types"
import type { TFunction } from "@/lib/i18n"
import { SHOW_COMMUNITY_EXTRAS } from "@/lib/features/community-extras"

export function UserCenterShell({
  user,
  currentUser,
  badges,
  profileBadges,
  fans,
  followed,
  showCounts = true,
  showBadges = true,
  t,
  children,
}: {
  user: UserSummary
  currentUser?: UserSummary | null
  badges: Badge[]
  profileBadges?: Badge[]
  fans: UserSummary[]
  followed: UserSummary[]
  showCounts?: boolean
  showBadges?: boolean
  t: TFunction
  children: React.ReactNode
}) {
  return (
    <section className="main">
      <div className="container">
        <UserProfileCard
          user={user}
          badges={profileBadges ?? badges}
          currentUser={currentUser}
        />
      </div>
      <div
        className={
          SHOW_COMMUNITY_EXTRAS
            ? "main-container right-main side-size-360 container"
            : "main-container container flex"
        }
      >
        {SHOW_COMMUNITY_EXTRAS ? (
          <UserCenterSidebar
            user={user}
            currentUser={currentUser}
            badges={badges}
            fans={fans}
            followed={followed}
            showCounts={showCounts}
            showBadges={showBadges}
            t={t}
          />
        ) : null}
        <div
          className={
            SHOW_COMMUNITY_EXTRAS
              ? "right-container"
              : "right-container min-w-0 flex-1"
          }
        >
          {children}
        </div>
      </div>
    </section>
  )
}
