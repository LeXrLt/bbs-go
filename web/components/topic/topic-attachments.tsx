"use client"

import * as React from "react"
import { CheckCircle2, Download, Eye, LockKeyhole } from "lucide-react"

import Link from "@/components/common/link"
import {
  ConfirmDialog,
  type ConfirmDialogState,
} from "@/components/common/confirm-dialog"
import { Button } from "@/components/ui/button"
import {
  attachmentDownloadPath,
  compactAttachmentName,
  grantAttachmentAccess,
  hasAttachmentAccess,
} from "@/lib/api/attachments"
import type { Attachment } from "@/lib/api/types"
import type { TFunction } from "@/lib/i18n"
import { useRouter } from "@/lib/router/navigation"
import { useToastActions } from "@/lib/toast"

function formatFileSize(size?: number) {
  if (!size || size <= 0) return "0 B"
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.ceil(size / 1024)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

type AttachmentAction = "preview" | "download"

export function TopicAttachments({
  topicId,
  attachments,
  t,
}: {
  topicId: string
  attachments?: Attachment[]
  t: TFunction
}) {
  const router = useRouter()
  const { catchError } = useToastActions()
  const [items, setItems] = React.useState(attachments || [])
  const [busyId, setBusyId] = React.useState<string | null>(null)
  const [confirmState, setConfirmState] =
    React.useState<ConfirmDialogState>(null)

  React.useEffect(() => {
    setItems(attachments || [])
  }, [attachments])

  function previewPath(attachmentId: string) {
    return `/topic/${encodeURIComponent(topicId)}/attachment/${encodeURIComponent(attachmentId)}/preview`
  }

  function continueAction(attachment: Attachment, action: AttachmentAction) {
    if (action === "preview") {
      router.push(previewPath(attachment.id))
      return
    }

    window.location.assign(attachmentDownloadPath(attachment.id))
  }

  async function unlockAndContinue(
    attachment: Attachment,
    action: AttachmentAction
  ) {
    setBusyId(attachment.id)
    try {
      const granted = await grantAttachmentAccess(attachment.id)
      setItems((current) =>
        current.map((item) =>
          item.id === attachment.id ? { ...item, ...granted } : item
        )
      )
      continueAction(granted, action)
    } catch (error) {
      catchError(error)
    } finally {
      setBusyId(null)
    }
  }

  function requestAction(attachment: Attachment, action: AttachmentAction) {
    if (hasAttachmentAccess(attachment)) {
      continueAction(attachment, action)
      return
    }

    setConfirmState({
      title: t("pages.topic.attachmentPreview.unlockTitle"),
      description: t("pages.topic.attachmentPreview.unlockDescription", {
        fileName: compactAttachmentName(attachment),
        score: attachment.downloadScore || 0,
      }),
      confirmText: t("pages.topic.attachmentPreview.unlockConfirm"),
      onConfirm: () => void unlockAndContinue(attachment, action),
    })
  }

  if (!items.length) return null

  return (
    <>
      <section className="mx-4 mb-4 rounded-md border border-border bg-muted/30 p-3">
        <h2 className="mb-2 text-sm font-medium text-foreground">
          {t("pages.topic.detail.attachments")}
        </h2>
        <ul className="space-y-3">
          {items.map((attachment) => {
            const accessGranted = hasAttachmentAccess(attachment)
            const busy = busyId === attachment.id

            return (
              <li
                key={attachment.id}
                className="flex flex-col gap-3 rounded border border-border bg-background px-3 py-3 text-sm sm:flex-row sm:flex-wrap sm:items-center sm:justify-between"
              >
                <div className="min-w-0 flex-1">
                  <div
                    className="truncate font-medium text-foreground"
                    title={attachment.fileName || attachment.id}
                  >
                    {attachment.fileName || attachment.id}
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-muted-foreground">
                    <span>{formatFileSize(attachment.fileSize)}</span>
                    {typeof attachment.downloadCount === "number" ? (
                      <span>
                        {t("pages.topic.detail.attachmentDownloadCount", {
                          count: attachment.downloadCount,
                        })}
                      </span>
                    ) : null}
                    {attachment.downloadScore && !accessGranted ? (
                      <>
                        <span aria-hidden="true">·</span>
                        <span>
                          {t("pages.topic.detail.attachmentScoreRequired", {
                            score: attachment.downloadScore,
                          })}
                        </span>
                      </>
                    ) : accessGranted && attachment.downloadScore ? (
                      <>
                        <span aria-hidden="true">·</span>
                        <span>
                          {t("pages.topic.detail.attachmentPurchased")}
                        </span>
                      </>
                    ) : (
                      <>
                        <span aria-hidden="true">·</span>
                        <span>{t("pages.topic.detail.attachmentFree")}</span>
                      </>
                    )}
                  </div>
                </div>
                <div className="flex w-full gap-2 sm:w-auto">
                  {attachment.previewable ? (
                    accessGranted ? (
                      <Button
                        asChild
                        variant="outline"
                        size="sm"
                        className="flex-1 sm:flex-none"
                      >
                        <Link href={previewPath(attachment.id)}>
                          <Eye aria-hidden="true" />
                          {t("pages.topic.detail.preview")}
                        </Link>
                      </Button>
                    ) : (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="flex-1 sm:flex-none"
                        disabled={busy}
                        onClick={() => requestAction(attachment, "preview")}
                      >
                        <LockKeyhole aria-hidden="true" />
                        {t("pages.topic.detail.preview")}
                      </Button>
                    )
                  ) : null}
                  {accessGranted ? (
                    <Button
                      asChild
                      variant="outline"
                      size="sm"
                      className="flex-1 sm:flex-none"
                    >
                      <a href={attachmentDownloadPath(attachment.id)}>
                        {attachment.downloaded ? (
                          <CheckCircle2 aria-hidden="true" />
                        ) : (
                          <Download aria-hidden="true" />
                        )}
                        {t("pages.topic.detail.download")}
                      </a>
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="flex-1 sm:flex-none"
                      disabled={busy}
                      onClick={() => requestAction(attachment, "download")}
                    >
                      <LockKeyhole aria-hidden="true" />
                      {t("pages.topic.detail.download")}
                    </Button>
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      </section>
      <ConfirmDialog
        state={confirmState}
        onOpenChange={(open) => {
          if (!open) setConfirmState(null)
        }}
      />
    </>
  )
}
