"use client"

import * as React from "react"
import Link from "@/components/common/link"
import {
  usePathname,
  useRouter,
  useSearchParams,
} from "@/lib/router/navigation"
import {
  Bell,
  BellRing,
  ChevronDown,
  Heart,
  LayoutDashboard,
  ListChecks,
  LogOut,
  Menu,
  MessageCircleReply,
  Plus,
  Settings,
  User,
} from "lucide-react"

import { signoutAction } from "@/lib/actions/auth"
import {
  useAppConfig,
  useAppState,
  useCurrentUser,
  useUnreadMessageCount,
} from "@/components/app/app-provider"
import { UserAvatar } from "@/components/common/avatar"
import {
  ConfirmDialog,
  type ConfirmDialogState,
} from "@/components/common/confirm-dialog"
import { SearchInput } from "@/components/common/search-input"
import { ThemeToggle } from "@/components/theme-toggle"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Separator } from "@/components/ui/separator"
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from "@/components/ui/hover-card"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTrigger,
} from "@/components/ui/sheet"
import type { SiteConfig, SiteNav, UserSummary } from "@/lib/api/types"
import { userCanAccessDashboard } from "@/lib/auth/roles"
import type { TFunction } from "@/lib/i18n"
import { useI18n } from "@/lib/i18n/provider"
import { cn } from "@/lib/utils"

function getUserName(user: UserSummary) {
  return user.nickname || user.username || "User"
}

function hasChildren(nav: SiteNav) {
  return Array.isArray(nav.children) && nav.children.length > 0
}

function targetFor(openInNewWindow?: boolean) {
  return openInNewWindow ? "_blank" : undefined
}

function relFor(openInNewWindow?: boolean) {
  return openInNewWindow ? "noopener noreferrer" : undefined
}

function useCurrentFullPath() {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const search = searchParams.toString()
  return search ? `${pathname}?${search}` : pathname
}

function signinHref(fullPath: string) {
  const redirect = fullPath.startsWith("/user/signin") ? "/" : fullPath
  return `/user/signin?redirect=${encodeURIComponent(redirect)}`
}

function CreateTopicButton({
  config,
  t,
  className,
  onClick,
}: {
  config: SiteConfig | null
  t: TFunction
  className?: string
  onClick?: () => void
}) {
  if (!config?.modules?.topic) return null

  return (
    <Button className={cn("h-8", className)} asChild>
      <Link href="/topic/create" onClick={onClick}>
        <Plus />
        {t("common.createBtn.create")}
      </Link>
    </Button>
  )
}

function messageCountLabel(count: number) {
  return count > 99 ? "99+" : String(count)
}

function MsgNotice({ count, t }: { count: number; t: TFunction }) {
  const [open, setOpen] = React.useState(false)

  return (
    <HoverCard
      open={open}
      onOpenChange={setOpen}
      openDelay={100}
      closeDelay={150}
    >
      <HoverCardTrigger asChild>
        <Link
          href="/user/messages"
          aria-label={t("common.header.repliesToMe")}
          onClick={() => setOpen(false)}
          className="inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-md text-sm font-medium whitespace-nowrap ring-offset-background transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0"
        >
          {count > 0 ? (
            <BellRing size={18} className="animate-swing" />
          ) : (
            <Bell size={18} />
          )}
        </Link>
      </HoverCardTrigger>
      <HoverCardContent align="end" className="w-52 p-2">
        <Link
          href="/user/messages"
          onClick={() => setOpen(false)}
          className="flex min-h-11 items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none"
        >
          <MessageCircleReply
            className="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="flex-1">{t("common.header.repliesToMe")}</span>
          {count > 0 ? (
            <span className="text-destructive-foreground inline-flex min-w-5 items-center justify-center rounded-full bg-destructive px-1.5 py-0.5 text-[11px] leading-4 font-semibold tabular-nums">
              {messageCountLabel(count)}
            </span>
          ) : null}
        </Link>
      </HoverCardContent>
    </HoverCard>
  )
}

function DesktopNav({ navs }: { navs: SiteNav[] }) {
  return (
    <nav className="hidden items-center md:flex" aria-label="Main">
      <div className="group/navigation-menu relative flex max-w-max flex-1 items-center justify-center">
        <div className="group flex flex-1 list-none items-center justify-center gap-1">
          {navs.map((nav, index) =>
            hasChildren(nav) ? (
              <DropdownMenu key={`${nav.title}-${index}`} modal={false}>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    className={cn(
                      buttonVariants({ variant: "ghost", size: "default" }),
                      "bg-transparent"
                    )}
                  >
                    {nav.title}
                    <ChevronDown
                      className="relative top-px ml-1 size-3 transition duration-300"
                      aria-hidden="true"
                    />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="w-[200px]">
                  {nav.children?.map((child, childIndex) => (
                    <DropdownMenuItem
                      key={`${child.title}-${childIndex}`}
                      asChild
                    >
                      <Link
                        href={child.url}
                        target={targetFor(child.openInNewWindow)}
                        rel={relFor(child.openInNewWindow)}
                      >
                        {child.title}
                      </Link>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <Link
                key={`${nav.title}-${index}`}
                href={nav.url}
                target={targetFor(nav.openInNewWindow)}
                rel={relFor(nav.openInNewWindow)}
                className={cn(
                  buttonVariants({ variant: "ghost", size: "default" }),
                  "bg-transparent"
                )}
              >
                {nav.title}
              </Link>
            )
          )}
        </div>
      </div>
    </nav>
  )
}

function UserMenu({
  user,
  t,
  onSignedOut,
}: {
  user: UserSummary
  t: TFunction
  onSignedOut: () => void
}) {
  const router = useRouter()
  const canAccessDashboard = userCanAccessDashboard(user)
  const [pending, startTransition] = React.useTransition()
  const [confirmState, setConfirmState] =
    React.useState<ConfirmDialogState>(null)

  function signout() {
    startTransition(async () => {
      await signoutAction()
      onSignedOut()
      window.location.href = "/"
      router.refresh()
    })
  }

  function confirmSignout() {
    setConfirmState({
      description: t("common.header.confirmLogout"),
      confirmText: t("common.header.logout"),
      onConfirm: signout,
    })
  }

  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger className="flex items-center space-x-2 rounded-md px-1 py-1 transition-colors hover:bg-accent hover:text-accent-foreground">
          <UserAvatar user={user} size={30} />
          <span className="max-w-20 truncate text-sm font-medium">
            {getUserName(user)}
          </span>
          <ChevronDown className="h-4 w-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem asChild>
            <Link
              href={`/user/${user.id}`}
              className="flex cursor-pointer items-center"
            >
              <User className="mr-2 h-4 w-4" />
              {t("common.header.profile")}
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href="/tasks" className="flex cursor-pointer items-center">
              <ListChecks className="mr-2 h-4 w-4" />
              {t("common.header.tasks")}
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link
              href="/user/favorites"
              className="flex cursor-pointer items-center"
            >
              <Heart className="mr-2 h-4 w-4" />
              {t("common.header.favorites")}
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link
              href="/user/profile"
              className="flex cursor-pointer items-center"
            >
              <Settings className="mr-2 h-4 w-4" />
              {t("common.header.editProfile")}
            </Link>
          </DropdownMenuItem>
          {canAccessDashboard ? (
            <DropdownMenuItem asChild>
              <Link
                href="/dashboard"
                target="_blank"
                rel="noopener noreferrer"
                className="flex cursor-pointer items-center"
              >
                <LayoutDashboard className="mr-2 h-4 w-4" />
                {t("common.header.dashboard")}
              </Link>
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="cursor-pointer text-destructive"
            disabled={pending}
            onSelect={(event) => {
              event.preventDefault()
              confirmSignout()
            }}
          >
            <LogOut className="mr-2 h-4 w-4" />
            {t("common.header.logout")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        state={confirmState}
        onOpenChange={(open) => {
          if (!open) setConfirmState(null)
        }}
      />
    </>
  )
}

function MobileMenu({
  navs,
  config,
  user,
  t,
  showColorModeToggle,
  onSignedOut,
}: {
  navs: SiteNav[]
  config: SiteConfig | null
  user: UserSummary | null
  t: TFunction
  showColorModeToggle: boolean
  onSignedOut: () => void
}) {
  const router = useRouter()
  const fullPath = useCurrentFullPath()
  const [open, setOpen] = React.useState(false)
  const [openIndexes, setOpenIndexes] = React.useState<number[]>([])
  const [pending, startTransition] = React.useTransition()
  const [confirmState, setConfirmState] =
    React.useState<ConfirmDialogState>(null)

  function toggleMobileNav(index: number) {
    setOpenIndexes((current) =>
      current.includes(index)
        ? current.filter((item) => item !== index)
        : [...current, index]
    )
  }

  function closeMobileMenu() {
    setOpen(false)
  }

  function signout() {
    startTransition(async () => {
      await signoutAction()
      onSignedOut()
      window.location.href = "/"
      router.refresh()
    })
  }

  function confirmSignout() {
    setConfirmState({
      description: t("common.header.confirmLogout"),
      confirmText: t("common.header.logout"),
      onConfirm: signout,
    })
  }

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" className="md:hidden">
          <Menu className="h-5 w-5" />
          <span className="sr-only">{t("common.header.toggleMenu")}</span>
        </Button>
      </SheetTrigger>
      <SheetContent
        side="right"
        className="w-[300px] sm:w-[400px]"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <div className="mt-12 flex flex-col space-y-4">
          {user ? (
            <div className="flex items-center space-x-3 rounded-lg bg-accent/50 p-3">
              <UserAvatar user={user} size={40} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {getUserName(user)}
                </p>
                <p className="text-xs text-muted-foreground">
                  {user.description}
                </p>
              </div>
            </div>
          ) : null}

          <nav className="flex flex-col space-y-2" aria-label="Mobile">
            {navs.map((nav, index) => (
              <div key={`${nav.title}-${index}`} className="flex flex-col">
                {hasChildren(nav) ? (
                  <div className="flex flex-col">
                    <button
                      type="button"
                      className="flex items-center justify-between px-3 py-2.5 text-left text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
                      onClick={() => toggleMobileNav(index)}
                    >
                      <span>{nav.title}</span>
                      <ChevronDown
                        className={cn(
                          "h-4 w-4 transition-transform",
                          openIndexes.includes(index) && "rotate-180"
                        )}
                      />
                    </button>
                    {openIndexes.includes(index) ? (
                      <div className="flex flex-col border-t border-border/50 py-1">
                        {nav.children?.map((child, childIndex) => (
                          <SheetClose
                            key={`${index}-${childIndex}`}
                            asChild
                            onClick={closeMobileMenu}
                          >
                            <Link
                              href={child.url}
                              target={
                                child.openInNewWindow ? "_blank" : "_self"
                              }
                              rel={relFor(child.openInNewWindow)}
                              className="flex items-center px-6 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
                            >
                              {child.title}
                            </Link>
                          </SheetClose>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ) : (
                  <SheetClose asChild onClick={closeMobileMenu}>
                    <Link
                      href={nav.url}
                      target={nav.openInNewWindow ? "_blank" : "_self"}
                      rel={relFor(nav.openInNewWindow)}
                      className="flex items-center rounded-md px-3 py-2.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
                    >
                      {nav.title}
                    </Link>
                  </SheetClose>
                )}
              </div>
            ))}
          </nav>

          <Separator />

          <div className="px-3">
            <SearchInput placeholder={t("component.searchInput.placeholder")} />
          </div>

          <div className="px-3">
            <CreateTopicButton
              config={config}
              t={t}
              onClick={closeMobileMenu}
            />
          </div>

          {user ? (
            <div className="flex flex-col space-y-2 px-3">
              <SheetClose asChild onClick={closeMobileMenu}>
                <Link
                  href={`/user/${user.id}`}
                  className="flex items-center rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
                >
                  <User className="mr-3 h-4 w-4" />
                  {t("common.header.profile")}
                </Link>
              </SheetClose>
              <SheetClose asChild onClick={closeMobileMenu}>
                <Link
                  href="/user/favorites"
                  className="flex items-center rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
                >
                  <Heart className="mr-3 h-4 w-4" />
                  {t("common.header.favorites")}
                </Link>
              </SheetClose>
              <SheetClose asChild onClick={closeMobileMenu}>
                <Link
                  href="/tasks"
                  className="flex items-center rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
                >
                  <ListChecks className="mr-3 h-4 w-4" />
                  {t("common.header.tasks")}
                </Link>
              </SheetClose>
              <SheetClose asChild onClick={closeMobileMenu}>
                <Link
                  href="/user/profile"
                  className="flex items-center rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
                >
                  <Settings className="mr-3 h-4 w-4" />
                  {t("common.header.editProfile")}
                </Link>
              </SheetClose>
              <Button
                className="justify-start"
                variant="destructive"
                disabled={pending}
                onClick={confirmSignout}
              >
                <LogOut className="mr-2 h-4 w-4" />
                {t("common.header.logout")}
              </Button>
            </div>
          ) : (
            <div className="px-3">
              <SheetClose asChild onClick={closeMobileMenu}>
                <Button className="w-full" asChild>
                  <Link href={signinHref(fullPath)}>
                    {t("common.header.login")}
                  </Link>
                </Button>
              </SheetClose>
            </div>
          )}

          {showColorModeToggle ? (
            <>
              <Separator />

              <div className="flex px-3">
                <ThemeToggle variant="ghost" size="icon-sm" />
              </div>
            </>
          ) : null}
        </div>
      </SheetContent>
      <ConfirmDialog
        state={confirmState}
        onOpenChange={(open) => {
          if (!open) setConfirmState(null)
        }}
      />
    </Sheet>
  )
}

export function SiteHeader() {
  const config = useAppConfig()
  const user = useCurrentUser()
  const { setCurrentUser } = useAppState()
  const unreadMessageCount = useUnreadMessageCount()
  const { t } = useI18n()
  const fullPath = useCurrentFullPath()
  const navs = config?.siteNavs ?? []
  const title = config?.siteTitle || "BBS-GO"
  const logo = config?.siteLogo
  const showColorModeToggle = true

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border/40 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <div className="flex h-14 items-center justify-between">
          <div className="flex items-center space-x-8">
            <Link href="/" className="flex items-center space-x-2">
              {logo ? (
                <img src={logo} alt={title} className="h-8 w-auto" />
              ) : (
                <span className="text-sm font-semibold">{title}</span>
              )}
            </Link>

            <DesktopNav navs={navs} />
          </div>

          <div className="hidden items-center space-x-4 md:flex">
            <SearchInput
              className="hidden xl:block"
              placeholder={t("component.searchInput.placeholder")}
            />
            <CreateTopicButton config={config} t={t} />
            {user ? <MsgNotice count={unreadMessageCount} t={t} /> : null}
            {user ? (
              <UserMenu
                user={user}
                t={t}
                onSignedOut={() => setCurrentUser(null)}
              />
            ) : (
              <Button variant="outline" className="h-8" asChild>
                <Link href={signinHref(fullPath)}>
                  {t("common.header.login")}
                </Link>
              </Button>
            )}
            {showColorModeToggle ? (
              <ThemeToggle variant="ghost" size="icon-sm" />
            ) : null}
          </div>

          <MobileMenu
            navs={navs}
            config={config}
            user={user}
            t={t}
            showColorModeToggle={showColorModeToggle}
            onSignedOut={() => setCurrentUser(null)}
          />
        </div>
      </div>
    </header>
  )
}
