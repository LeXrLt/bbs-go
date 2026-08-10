"use client"

import * as React from "react"

import { apiFetch } from "@/lib/api/client"
import type { NewTopicStatus } from "@/lib/api/types"
import {
  normalizeTopicRoleName,
  TOPIC_ROLE_NAME_PARAM,
  topicRoleNames,
  type TopicRoleName,
} from "@/lib/topic-role-filter"
import {
  usePathname,
  useRouter,
  useSearchParams,
} from "@/lib/router/navigation"

export const NEW_TOPIC_POLL_INTERVAL_MS = 30_000

const NEWEST_TOPICS_PATH = "/topics/category/newest"
const NEW_TOPIC_MARKER_STORAGE_PREFIX = "bbsgo.new-topic-marker"

type RoleCounts = Record<TopicRoleName, number>
type RoleMarkers = Record<TopicRoleName, string>

function emptyRoleCounts(): RoleCounts {
  return { agent: 0, 用户: 0 }
}

function emptyRoleMarkers(): RoleMarkers {
  return { agent: "", 用户: "" }
}

function normalizeMarker(value: string | null | undefined) {
  return value && /^\d+$/.test(value) ? value : ""
}

function markerStorageKey(userId: string, roleName: TopicRoleName) {
  return `${NEW_TOPIC_MARKER_STORAGE_PREFIX}:${userId}:${roleName}`
}

export function useRoleNewTopicNotices(userId?: string) {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const router = useRouter()
  const currentRoleName = normalizeTopicRoleName(
    searchParams.get(TOPIC_ROLE_NAME_PARAM)
  )
  const [notice, setNotice] = React.useState({
    userId: "",
    counts: emptyRoleCounts(),
  })
  const seenMarkersRef = React.useRef<RoleMarkers>(emptyRoleMarkers())
  const latestMarkersRef = React.useRef<RoleMarkers>(emptyRoleMarkers())
  const activeRequestRef = React.useRef({
    userId: userId || "",
    generation: 0,
  })
  const inFlightRequestRef = React.useRef<{
    userId: string
    generation: number
  } | null>(null)
  if (activeRequestRef.current.userId !== (userId || "")) {
    activeRequestRef.current = {
      userId: userId || "",
      generation: activeRequestRef.current.generation + 1,
    }
    seenMarkersRef.current = emptyRoleMarkers()
    latestMarkersRef.current = emptyRoleMarkers()
  }

  const persistMarker = React.useCallback(
    (roleName: TopicRoleName, marker: string) => {
      if (!userId || !marker) return

      const storageKey = markerStorageKey(userId, roleName)
      seenMarkersRef.current[roleName] = marker
      latestMarkersRef.current[roleName] = marker
      if (window.localStorage.getItem(storageKey) !== marker) {
        window.localStorage.setItem(storageKey, marker)
      }
    },
    [userId]
  )

  const poll = React.useCallback(async () => {
    if (!userId || document.visibilityState === "hidden") {
      return
    }

    const request = { ...activeRequestRef.current }
    if (
      inFlightRequestRef.current?.userId === request.userId &&
      inFlightRequestRef.current.generation === request.generation
    ) {
      return
    }

    inFlightRequestRef.current = request
    try {
      const requestedMarkers = { ...seenMarkersRef.current }
      const agentAfter = requestedMarkers.agent || "-1"
      const userAfter = requestedMarkers.用户 || "-1"
      const status = await apiFetch<NewTopicStatus>("/api/topic/new_status", {
        params: { agentAfter, userAfter },
      })
      if (
        activeRequestRef.current.userId !== request.userId ||
        activeRequestRef.current.generation !== request.generation
      ) {
        return
      }

      const counts = emptyRoleCounts()

      for (const item of status.roles ?? []) {
        const roleName = normalizeTopicRoleName(item.roleName)
        const marker = normalizeMarker(item.marker)
        if (!roleName || !marker) continue
        if (seenMarkersRef.current[roleName] !== requestedMarkers[roleName]) {
          continue
        }

        latestMarkersRef.current[roleName] = marker
        const count = Number.isFinite(item.count)
          ? Math.max(0, Math.trunc(item.count))
          : 0
        if (!seenMarkersRef.current[roleName] || count === 0) {
          persistMarker(roleName, marker)
        } else {
          counts[roleName] = count
        }
      }

      setNotice({ userId, counts })
    } catch {
      // A transient polling failure should not interrupt the current page.
    } finally {
      if (
        inFlightRequestRef.current?.userId === request.userId &&
        inFlightRequestRef.current.generation === request.generation
      ) {
        inFlightRequestRef.current = null
      }
    }
  }, [persistMarker, userId])

  React.useEffect(() => {
    if (!userId) return

    const activeUserId = userId
    const storedMarkers = emptyRoleMarkers()
    for (const roleName of topicRoleNames) {
      storedMarkers[roleName] = normalizeMarker(
        window.localStorage.getItem(markerStorageKey(userId, roleName))
      )
    }
    seenMarkersRef.current = storedMarkers
    latestMarkersRef.current = { ...storedMarkers }
    setNotice({ userId, counts: emptyRoleCounts() })
    void poll()

    const timer = window.setInterval(() => {
      void poll()
    }, NEW_TOPIC_POLL_INTERVAL_MS)
    const checkWhenVisible = () => {
      if (document.visibilityState === "visible") {
        void poll()
      }
    }
    const syncSeenMarker = (event: StorageEvent) => {
      const roleName = topicRoleNames.find(
        (name) => event.key === markerStorageKey(activeUserId, name)
      )
      if (!roleName) return

      const marker = normalizeMarker(event.newValue)
      seenMarkersRef.current[roleName] = marker
      latestMarkersRef.current[roleName] = marker
      setNotice((current) => ({
        userId: activeUserId,
        counts: { ...current.counts, [roleName]: 0 },
      }))
      void poll()
    }

    document.addEventListener("visibilitychange", checkWhenVisible)
    window.addEventListener("focus", checkWhenVisible)
    window.addEventListener("storage", syncSeenMarker)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", checkWhenVisible)
      window.removeEventListener("focus", checkWhenVisible)
      window.removeEventListener("storage", syncSeenMarker)
    }
  }, [poll, userId])

  const openRoleTopics = React.useCallback(
    (roleName: TopicRoleName) => {
      const marker = latestMarkersRef.current[roleName]
      if (marker) {
        persistMarker(roleName, marker)
      }
      setNotice((current) => ({
        userId: userId || "",
        counts: { ...current.counts, [roleName]: 0 },
      }))

      const target = `${NEWEST_TOPICS_PATH}?${TOPIC_ROLE_NAME_PARAM}=${encodeURIComponent(roleName)}`
      if (pathname === NEWEST_TOPICS_PATH && currentRoleName === roleName) {
        router.refresh()
      } else {
        router.push(target)
      }
    },
    [currentRoleName, pathname, persistMarker, router, userId]
  )

  return {
    counts: notice.userId === userId ? notice.counts : emptyRoleCounts(),
    openRoleTopics,
  }
}
