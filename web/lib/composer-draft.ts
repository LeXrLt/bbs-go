import * as React from "react"

import type { ImageInfo, TopicAttachment } from "@/lib/api/types"

const DRAFT_VERSION = 1
const DRAFT_STORAGE_PREFIX = "bbsgo.composer-draft"

type StoredDraft = {
  version: number
  savedAt: number
  value: unknown
}

export type ComposerDraftKind =
  | "topic-create"
  | "topic-edit"
  | "article-create"
  | "article-edit"

export function isDraftRecord(
  value: unknown
): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}

export function normalizeDraftStringList(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : []
}

export function normalizeDraftImageList(value: unknown): ImageInfo[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.flatMap((item) => {
    if (!isDraftRecord(item)) {
      return []
    }
    const image: ImageInfo = {}
    if (typeof item.name === "string") image.name = item.name
    if (typeof item.url === "string") image.url = item.url
    if (typeof item.preview === "string") image.preview = item.preview
    if (typeof item.size === "number" && Number.isFinite(item.size)) {
      image.size = item.size
    }
    return image.url || image.preview ? [image] : []
  })
}

export function normalizeDraftAttachmentList(
  value: unknown
): TopicAttachment[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.flatMap((item) => {
    if (
      !isDraftRecord(item) ||
      (typeof item.id !== "string" && typeof item.id !== "number")
    ) {
      return []
    }
    return [
      {
        id: String(item.id),
        fileName: typeof item.fileName === "string" ? item.fileName : "",
        fileSize:
          typeof item.fileSize === "number" && Number.isFinite(item.fileSize)
            ? item.fileSize
            : 0,
        fileType: typeof item.fileType === "string" ? item.fileType : "",
        previewable: Boolean(item.previewable),
        accessGranted: Boolean(item.accessGranted),
        downloadScore:
          typeof item.downloadScore === "number" &&
          Number.isFinite(item.downloadScore)
            ? item.downloadScore
            : undefined,
        downloadCount:
          typeof item.downloadCount === "number" &&
          Number.isFinite(item.downloadCount)
            ? item.downloadCount
            : undefined,
        downloaded: Boolean(item.downloaded),
        createTime:
          typeof item.createTime === "number" &&
          Number.isFinite(item.createTime)
            ? item.createTime
            : undefined,
      },
    ]
  })
}

export function composerDraftKey(
  kind: ComposerDraftKind,
  userId: string | number | null | undefined,
  entityId?: string | number
) {
  if (userId === null || userId === undefined || userId === "") {
    return null
  }

  const parts = [DRAFT_STORAGE_PREFIX, kind, encodeURIComponent(String(userId))]
  if (entityId !== undefined && entityId !== null && entityId !== "") {
    parts.push(encodeURIComponent(String(entityId)))
  }
  return parts.join(":")
}

function readStoredDraft(key: string): unknown | null {
  if (typeof window === "undefined") {
    return null
  }

  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) {
      return null
    }
    const stored = JSON.parse(raw) as StoredDraft
    if (
      !stored ||
      typeof stored !== "object" ||
      stored.version !== DRAFT_VERSION ||
      !("value" in stored)
    ) {
      return null
    }
    return stored.value
  } catch {
    return null
  }
}

function writeStoredDraft<T>(key: string, value: T) {
  if (typeof window === "undefined") {
    return
  }

  try {
    const stored: StoredDraft = {
      version: DRAFT_VERSION,
      savedAt: Date.now(),
      value,
    }
    window.localStorage.setItem(key, JSON.stringify(stored))
  } catch {
    // A full or disabled localStorage must not prevent editing or publishing.
  }
}

export function clearComposerDraft(key: string | null | undefined) {
  if (!key || typeof window === "undefined") {
    return
  }

  try {
    window.localStorage.removeItem(key)
  } catch {
    // A disabled localStorage must not prevent successful publishing.
  }
}

type UseComposerDraftOptions<T> = {
  key: string | null
  initialValue: T
  deserialize?: (value: unknown) => T | null
  isEmpty?: (value: T) => boolean
}

export function useComposerDraft<T>({
  key,
  initialValue,
  deserialize,
  isEmpty,
}: UseComposerDraftOptions<T>) {
  const [value, setValue] = React.useState(initialValue)
  const [loadedKey, setLoadedKey] = React.useState<string | null>(null)
  const [restored, setRestored] = React.useState(false)
  const initialValueRef = React.useRef(initialValue)
  const deserializeRef = React.useRef(deserialize)
  initialValueRef.current = initialValue
  deserializeRef.current = deserialize

  React.useEffect(() => {
    if (!key) {
      setValue(initialValueRef.current)
      setLoadedKey(null)
      setRestored(false)
      return
    }

    const stored = readStoredDraft(key)
    const currentInitialValue = initialValueRef.current
    const restoredValue =
      stored === null
        ? null
        : deserializeRef.current
          ? deserializeRef.current(stored)
          : (stored as T)

    setValue(restoredValue ?? currentInitialValue)
    setLoadedKey(key)
    setRestored(restoredValue !== null)
  }, [key])

  React.useEffect(() => {
    if (!key || loadedKey !== key) {
      return
    }

    if (isEmpty?.(value)) {
      clearComposerDraft(key)
      return
    }

    writeStoredDraft(key, value)
  }, [isEmpty, key, loadedKey, value])

  const clear = React.useCallback(() => {
    clearComposerDraft(key)
  }, [key])

  return {
    value,
    setValue,
    clear,
    restored,
    loaded: Boolean(key) && loadedKey === key,
  }
}
