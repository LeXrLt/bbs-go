import { apiFetch } from "./client"

import type { Attachment } from "./types"

export function attachmentPreviewPath(attachmentId: string) {
  return `/api/attachment/preview/${encodeURIComponent(attachmentId)}`
}

export function attachmentDownloadPath(attachmentId: string) {
  return `/api/attachment/download/${encodeURIComponent(attachmentId)}`
}

export function compactAttachmentName(attachment: Attachment) {
  const name = attachment.fileName || attachment.id
  if (name.length <= 32) return name
  return `${name.slice(0, 21)}...${name.slice(-8)}`
}

export function grantAttachmentAccess(attachmentId: string) {
  return apiFetch<Attachment>(
    `/api/attachment/access/${encodeURIComponent(attachmentId)}`,
    { method: "POST" }
  )
}

export function hasAttachmentAccess(attachment: Attachment) {
  if (typeof attachment.accessGranted === "boolean") {
    return attachment.accessGranted
  }

  return Boolean(attachment.downloaded || !attachment.downloadScore)
}
