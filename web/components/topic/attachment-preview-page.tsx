"use client"

import * as React from "react"
import { ArrowLeft, Download, FileWarning, LockKeyhole } from "lucide-react"

import Link from "@/components/common/link"
import {
  ConfirmDialog,
  type ConfirmDialogState,
} from "@/components/common/confirm-dialog"
import { MainShell } from "@/components/layout/main-shell"
import { Button } from "@/components/ui/button"
import {
  attachmentDownloadPath,
  compactAttachmentName,
  grantAttachmentAccess,
  hasAttachmentAccess,
} from "@/lib/api/attachments"
import type { Attachment, Topic } from "@/lib/api/types"
import { useI18n } from "@/lib/i18n/provider"
import { useDocumentTitle } from "@/lib/use-document-title"
import { useToastActions } from "@/lib/toast"

const AttachmentPdfViewer = React.lazy(() =>
  import("./attachment-pdf-viewer").then((module) => ({
    default: module.AttachmentPdfViewer,
  }))
)

export function AttachmentPreviewPage({
  topic,
  attachmentId,
}: {
  topic: Topic
  attachmentId: string
}) {
  const { t } = useI18n()
  const { catchError } = useToastActions()
  const initialAttachment = topic.attachments?.find(
    (item) => String(item.id) === attachmentId
  )
  const [attachment, setAttachment] = React.useState<Attachment | null>(
    initialAttachment || null
  )
  const [unlocking, setUnlocking] = React.useState(false)
  const [confirmState, setConfirmState] =
    React.useState<ConfirmDialogState>(null)
  const [clientReady, setClientReady] = React.useState(false)
  useDocumentTitle(
    attachment?.fileName || t("pages.topic.attachmentPreview.title")
  )

  React.useEffect(() => setClientReady(true), [])
  React.useEffect(() => {
    setAttachment(initialAttachment || null)
    setConfirmState(null)
  }, [attachmentId, initialAttachment])

  async function unlock() {
    if (!attachment) return

    setUnlocking(true)
    try {
      const granted = await grantAttachmentAccess(attachment.id)
      setAttachment(granted)
    } catch (error) {
      catchError(error)
    } finally {
      setUnlocking(false)
    }
  }

  function requestUnlock() {
    if (!attachment) return

    setConfirmState({
      title: t("pages.topic.attachmentPreview.unlockTitle"),
      description: t("pages.topic.attachmentPreview.unlockDescription", {
        fileName: compactAttachmentName(attachment),
        score: attachment.downloadScore || 0,
      }),
      confirmText: t("pages.topic.attachmentPreview.unlockConfirm"),
      onConfirm: () => void unlock(),
    })
  }

  return (
    <MainShell containerClassName="max-w-[1600px]">
      <div className="overflow-hidden rounded-md bg-background">
        <header className="flex min-h-14 items-center gap-2 px-2 py-2 sm:px-4">
          <Button asChild variant="ghost" size="icon-sm">
            <Link
              href={`/topic/${encodeURIComponent(topic.id)}`}
              aria-label={t("pages.topic.attachmentPreview.backToTopic")}
              title={t("pages.topic.attachmentPreview.backToTopic")}
            >
              <ArrowLeft aria-hidden="true" />
            </Link>
          </Button>
          <div className="min-w-0 flex-1">
            <h1
              className="truncate text-sm font-semibold sm:text-base"
              title={attachment?.fileName}
            >
              {attachment?.fileName || t("pages.topic.attachmentPreview.title")}
            </h1>
            {topic.title ? (
              <p className="truncate text-xs text-muted-foreground">
                {topic.title}
              </p>
            ) : null}
          </div>
        </header>

        {!attachment ? (
          <div className="flex min-h-[50dvh] flex-col items-center justify-center gap-3 border-y border-border px-4 text-center">
            <FileWarning
              className="size-8 text-muted-foreground"
              aria-hidden="true"
            />
            <p className="text-sm text-muted-foreground">
              {t("pages.topic.attachmentPreview.notFound")}
            </p>
          </div>
        ) : !attachment.previewable ? (
          <div className="flex min-h-[50dvh] flex-col items-center justify-center gap-3 border-y border-border px-4 text-center">
            <FileWarning
              className="size-8 text-muted-foreground"
              aria-hidden="true"
            />
            <p className="text-sm text-muted-foreground">
              {t("pages.topic.attachmentPreview.notPreviewable")}
            </p>
            {hasAttachmentAccess(attachment) ? (
              <Button asChild>
                <a href={attachmentDownloadPath(attachment.id)}>
                  <Download aria-hidden="true" />
                  {t("pages.topic.detail.download")}
                </a>
              </Button>
            ) : (
              <Button
                type="button"
                onClick={requestUnlock}
                disabled={unlocking}
              >
                <LockKeyhole aria-hidden="true" />
                {t("pages.topic.attachmentPreview.unlockConfirm")}
              </Button>
            )}
          </div>
        ) : !hasAttachmentAccess(attachment) ? (
          <div className="flex min-h-[50dvh] flex-col items-center justify-center gap-3 border-y border-border px-4 text-center">
            <LockKeyhole
              className="size-8 text-muted-foreground"
              aria-hidden="true"
            />
            <p className="text-sm font-medium">
              {t("pages.topic.attachmentPreview.locked", {
                score: attachment.downloadScore || 0,
              })}
            </p>
            <Button type="button" onClick={requestUnlock} disabled={unlocking}>
              <LockKeyhole aria-hidden="true" />
              {unlocking
                ? t("pages.topic.attachmentPreview.unlocking")
                : t("pages.topic.attachmentPreview.unlockConfirm")}
            </Button>
          </div>
        ) : clientReady ? (
          <React.Suspense
            fallback={
              <div
                className="flex min-h-[50dvh] items-center justify-center border-y border-border text-sm text-muted-foreground"
                role="status"
              >
                {t("pages.topic.attachmentPreview.loading")}
              </div>
            }
          >
            <AttachmentPdfViewer attachment={attachment} />
          </React.Suspense>
        ) : (
          <div className="min-h-[50dvh] border-y border-border" />
        )}
      </div>
      <ConfirmDialog
        state={confirmState}
        onOpenChange={(open) => {
          if (!open) setConfirmState(null)
        }}
      />
    </MainShell>
  )
}
