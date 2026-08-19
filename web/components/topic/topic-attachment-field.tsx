"use client"

import * as React from "react"
import { Plus, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { apiFetch } from "@/lib/api/client"
import type { SiteConfig, TopicAttachment } from "@/lib/api/types"
import { useI18n } from "@/lib/i18n/provider"
import { useToastActions } from "@/lib/toast"

export const DEFAULT_ATTACHMENT_TYPES = [
  ".pdf",
  ".doc",
  ".docx",
  ".xls",
  ".xlsx",
  ".ppt",
  ".pptx",
  ".txt",
  ".md",
  ".csv",
  ".zip",
  ".rar",
  ".7z",
  ".tar",
  ".gz",
] as const

function normalizedAllowedTypes(config?: SiteConfig["attachmentConfig"]) {
  const configuredTypes = config?.allowedTypes
    ?.map((type) => type.trim().toLowerCase())
    .map((type) => (type.startsWith(".") ? type : `.${type}`))
    .filter((type) => /^\.[a-z0-9]+$/.test(type))

  return configuredTypes?.length
    ? Array.from(new Set(configuredTypes))
    : [...DEFAULT_ATTACHMENT_TYPES]
}

function fileExtension(fileName: string) {
  const match = /\.[^.]+$/.exec(fileName.toLowerCase())
  return match?.[0] || ""
}

function formatFileSize(size?: number) {
  if (!size || size <= 0) return "0 B"
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.ceil(size / 1024)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

export function TopicAttachmentField({
  value,
  config,
  uploading,
  onUploadingChange,
  onChange,
}: {
  value: TopicAttachment[]
  config?: SiteConfig["attachmentConfig"]
  uploading: boolean
  onUploadingChange: (value: boolean) => void
  onChange: (value: TopicAttachment[]) => void
}) {
  const { t } = useI18n()
  const { catchError, msgWarning } = useToastActions()
  const inputRef = React.useRef<HTMLInputElement>(null)
  const maxCount = config?.maxCount ?? 5
  const maxSizeMB = config?.maxSizeMB ?? 10
  const allowedTypes = React.useMemo(
    () => normalizedAllowedTypes(config),
    [config]
  )
  const accept = allowedTypes.join(",")

  async function upload(file: File) {
    if (value.length >= maxCount) {
      msgWarning(t("pages.topic.create.attachment.maxCountError", { maxCount }))
      return
    }

    if (!allowedTypes.includes(fileExtension(file.name))) {
      msgWarning(
        t("pages.topic.create.attachment.typeError", {
          types: allowedTypes.join(", "),
        })
      )
      return
    }

    if (file.size > maxSizeMB * 1024 * 1024) {
      msgWarning(t("pages.topic.create.attachment.sizeError", { maxSizeMB }))
      return
    }

    onUploadingChange(true)
    try {
      const body = new FormData()
      body.append("file", file, file.name)
      body.append("downloadScore", "0")
      const attachment = await apiFetch<TopicAttachment>(
        "/api/attachment/upload",
        {
          method: "POST",
          body,
        }
      )
      onChange([...value, attachment])
    } catch (error) {
      catchError(error)
    } finally {
      onUploadingChange(false)
    }
  }

  async function updateScore(
    attachment: TopicAttachment,
    downloadScore: number
  ) {
    onChange(
      value.map((item) =>
        item.id === attachment.id ? { ...item, downloadScore } : item
      )
    )
    try {
      await apiFetch<null>("/api/attachment/update_download_score", {
        method: "POST",
        body: { id: attachment.id, downloadScore },
      })
    } catch (error) {
      catchError(error)
    }
  }

  return (
    <div className="rounded-md border border-dashed bg-muted/20 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted-foreground">
          {t("pages.topic.create.attachment.label")}
        </span>
        <span className="text-xs text-muted-foreground">
          {t("pages.topic.create.attachment.limitHint", {
            maxCount,
            maxSizeMB,
          })}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={uploading || value.length >= maxCount}
          onClick={() => inputRef.current?.click()}
        >
          <Plus aria-hidden="true" />
          {t("pages.topic.create.attachment.add")}
        </Button>
        <input
          ref={inputRef}
          type="file"
          className="hidden"
          accept={accept}
          disabled={uploading || value.length >= maxCount}
          onChange={(event) => {
            const file = event.currentTarget.files?.[0]
            if (file) void upload(file)
            event.currentTarget.value = ""
          }}
        />
      </div>
      {uploading ? (
        <div className="mt-2 text-xs text-muted-foreground" role="status">
          {t("pages.topic.create.attachmentUploading")}
        </div>
      ) : null}
      {value.length ? (
        <ul className="mt-2 space-y-2 text-sm">
          {value.map((attachment, index) => (
            <li
              key={attachment.id || index}
              className="flex flex-col gap-2 rounded border bg-background p-2 sm:flex-row sm:items-center"
            >
              <div className="min-w-0 flex-1">
                <span className="block truncate font-medium">
                  {attachment.fileName}
                </span>
                <span className="text-xs text-muted-foreground">
                  {formatFileSize(attachment.fileSize)}
                </span>
              </div>
              <label className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                {t("pages.topic.create.attachment.scorePlaceholder")}
                <Input
                  type="number"
                  min="0"
                  step="1"
                  className="h-8 w-20"
                  value={attachment.downloadScore ?? 0}
                  onChange={(event) =>
                    void updateScore(
                      attachment,
                      Math.max(0, Number(event.currentTarget.value) || 0)
                    )
                  }
                />
              </label>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="text-muted-foreground hover:text-destructive"
                aria-label={t("pages.topic.create.attachment.remove")}
                onClick={() =>
                  onChange(value.filter((_, itemIndex) => itemIndex !== index))
                }
              >
                <Trash2 aria-hidden="true" />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
