"use client"

import * as React from "react"
import { useRouter } from "@/lib/router/navigation"

import { TagInput } from "@/components/common/tag-input"
import { ContentEditor } from "@/components/editor/content-editor"
import { CategoryQuickSelector } from "@/components/topic/category-selector"
import { TopicAttachmentField } from "@/components/topic/topic-attachment-field"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { apiFetch } from "@/lib/api/client"
import type { SiteConfig, TopicAttachment, Category } from "@/lib/api/types"
import type { TopicEditData } from "@/lib/api/topics"
import { useI18n } from "@/lib/i18n/provider"
import {
  filterCategoryTree,
  getFirstCategoryId,
  hasCategory,
} from "@/lib/categories"
import { msg, useToastActions } from "@/lib/toast"

type TopicEditFormState = {
  id: string
  type: number
  categoryId: number
  title: string
  content: string
  contentType: "html" | "markdown"
  hideContent: string
  tags: string[]
}

function categoryTypeMatches(topicType: number) {
  return (node: Category) =>
    topicType === 2 ? node.type === "qa" : node.type !== "qa"
}

function normalizeEditData(topic: TopicEditData): TopicEditFormState {
  return {
    id: topic.id,
    type: Number(topic.type) || 0,
    categoryId: Number(topic.categoryId) || 0,
    title: topic.title || "",
    content: topic.content || "",
    contentType: topic.contentType === "markdown" ? "markdown" : "html",
    hideContent: topic.hideContent || "",
    tags: Array.isArray(topic.tags) ? topic.tags : [],
  }
}

export function TopicEditForm({
  topic,
  config,
  categories,
}: {
  topic: TopicEditData
  config: SiteConfig | null
  categories: Category[]
}) {
  const router = useRouter()
  const { t } = useI18n()
  const { catchError, msgWarning } = useToastActions()
  const lastSubmitAtRef = React.useRef(0)
  const [publishing, setPublishing] = React.useState(false)
  const [attachmentUploading, setAttachmentUploading] = React.useState(false)
  const [attachmentList, setAttachmentList] = React.useState<TopicAttachment[]>(
    () => (Array.isArray(topic.attachments) ? topic.attachments : [])
  )
  const [form, setForm] = React.useState<TopicEditFormState>(() =>
    normalizeEditData(topic)
  )
  const availableNodes = React.useMemo(
    () => filterCategoryTree(categories, categoryTypeMatches(form.type)),
    [form.type, categories]
  )
  const effectiveCategoryId = hasCategory(availableNodes, form.categoryId)
    ? form.categoryId
    : getFirstCategoryId(availableNodes)

  function updateForm(next: Partial<TopicEditFormState>) {
    setForm((current) => ({ ...current, ...next }))
  }

  async function submit() {
    const now = Date.now()
    if (now - lastSubmitAtRef.current < 500 || publishing) {
      return
    }
    lastSubmitAtRef.current = now

    if (attachmentUploading) {
      msgWarning(t("pages.topic.create.attachmentUploading"))
      return
    }

    setPublishing(true)
    try {
      await apiFetch<null>(`/api/topic/edit/${form.id}`, {
        method: "POST",
        body: {
          categoryId: effectiveCategoryId,
          title: form.title,
          content: form.content,
          hideContent: form.hideContent,
          tags: form.tags,
          attachmentIds:
            form.type === 0 ? attachmentList.map((item) => item.id) : [],
        },
      })
      msg({
        message: t("pages.topic.edit.success"),
        onClose() {
          router.push(`/topic/${form.id}`)
        },
      })
    } catch (error) {
      catchError(error)
      setPublishing(false)
    }
  }

  return (
    <div className="publish-form">
      <div className="form-title">
        <div className="form-title-name">{t("pages.topic.edit.title")}</div>
      </div>

      <div className="field">
        <CategoryQuickSelector
          value={effectiveCategoryId}
          categories={availableNodes}
          onChange={(categoryId) => updateForm({ categoryId })}
        />
      </div>

      <div className="field">
        <Input
          value={form.title}
          placeholder={t("pages.topic.edit.titlePlaceholder")}
          onChange={(event) => updateForm({ title: event.currentTarget.value })}
        />
      </div>

      <div className="field">
        <ContentEditor
          contentType={form.contentType}
          value={form.content}
          placeholder={t("pages.topic.edit.contentPlaceholder")}
          height="400px"
          onChange={(content) => updateForm({ content })}
        />
      </div>

      {form.type !== 2 && (config?.enableHideContent || form.hideContent) ? (
        <div className="field">
          <ContentEditor
            contentType="html"
            value={form.hideContent}
            height="200px"
            onChange={(hideContent) => updateForm({ hideContent })}
          />
        </div>
      ) : null}

      <div className="field">
        <TagInput
          value={form.tags}
          recommendTags={config?.recommendTags}
          placeholder={t("component.tagInput.placeholder")}
          onChange={(tags) => updateForm({ tags })}
        />
      </div>

      {form.type === 0 && config?.attachmentConfig?.enabled ? (
        <div className="field">
          <TopicAttachmentField
            value={attachmentList}
            config={config.attachmentConfig}
            categoryId={effectiveCategoryId}
            uploading={attachmentUploading}
            onUploadingChange={setAttachmentUploading}
            onChange={setAttachmentList}
          />
        </div>
      ) : null}

      <div className="form-footer">
        <Button
          type="button"
          disabled={publishing || attachmentUploading}
          onClick={() => void submit()}
        >
          {t("pages.topic.edit.submitBtn")}
        </Button>
      </div>
    </div>
  )
}
