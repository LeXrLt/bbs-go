"use client"

import * as React from "react"
import { Image as ImageIcon, Plus, X } from "lucide-react"

import { PreviewableImage } from "@/components/common/image-preview"
import {
  MarkdownEditor,
  type MarkdownEditorRef,
} from "@/components/editor/markdown-editor"
import { uploadEditorImage } from "@/components/editor/upload"
import { Button } from "@/components/ui/button"
import type { ImageInfo } from "@/lib/api/types"
import { useI18n } from "@/lib/i18n/provider"
import { useToastActions } from "@/lib/toast"
import { cn } from "@/lib/utils"

export type TextEditorRef = {
  focus: () => void
  reset: () => void
}

const COMMENT_IMAGE_LIMIT = 9

function imageSrc(image: ImageInfo) {
  return image.url || image.preview || ""
}

export const TextEditor = React.forwardRef<
  TextEditorRef,
  {
    content: string
    imageList: ImageInfo[]
    height?: number
    focusHeight?: number
    disabled?: boolean
    onContentChange: (content: string) => void
    onImageListChange: (imageList: ImageInfo[]) => void
    onSubmit: () => void
  }
>(function TextEditor(
  {
    content,
    imageList,
    height = 160,
    focusHeight = 0,
    disabled,
    onContentChange,
    onImageListChange,
    onSubmit,
  },
  ref
) {
  const { t } = useI18n()
  const { catchError, msgWarning } = useToastActions()
  const wrapperRef = React.useRef<HTMLDivElement>(null)
  const markdownEditorRef = React.useRef<MarkdownEditorRef>(null)
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const isOpeningImagePickerRef = React.useRef(false)
  const unlockImagePickerTimerRef = React.useRef<number | null>(null)
  const [isFocus, setIsFocus] = React.useState(false)
  const [showImageUpload, setShowImageUpload] = React.useState(false)
  const [imageUploading, setImageUploading] = React.useState(false)
  const currentImages = imageList || []
  const canAddImage = currentImages.length < COMMENT_IMAGE_LIMIT

  React.useImperativeHandle(ref, () => ({
    focus() {
      markdownEditorRef.current?.focus()
    },
    reset() {
      setIsFocus(false)
      setShowImageUpload(false)
      window.setTimeout(() => markdownEditorRef.current?.resetHistory(), 0)
    },
  }))

  React.useEffect(() => {
    return () => {
      if (unlockImagePickerTimerRef.current) {
        window.clearTimeout(unlockImagePickerTimerRef.current)
      }
    }
  }, [])

  function unlockImagePickerBlurGuard() {
    if (unlockImagePickerTimerRef.current) {
      window.clearTimeout(unlockImagePickerTimerRef.current)
    }
    unlockImagePickerTimerRef.current = window.setTimeout(() => {
      isOpeningImagePickerRef.current = false
      unlockImagePickerTimerRef.current = null
    }, 0)
  }

  const uploadFiles = React.useCallback(
    async (files: File[]) => {
      if (disabled) {
        return
      }
      const images = files.filter((file) => file.type.startsWith("image/"))
      if (!images.length || imageUploading) {
        return
      }
      if (imageList.length >= COMMENT_IMAGE_LIMIT) {
        msgWarning(
          t("component.imageUpload.countLimitError", {
            limit: COMMENT_IMAGE_LIMIT,
          })
        )
        return
      }

      const remainingCount = COMMENT_IMAGE_LIMIT - imageList.length
      const uploadImages = images.slice(0, remainingCount)
      if (images.length > remainingCount) {
        msgWarning(
          t("component.imageUpload.countLimitError", {
            limit: COMMENT_IMAGE_LIMIT,
          })
        )
      }

      setShowImageUpload(true)
      setImageUploading(true)
      try {
        const uploaded: ImageInfo[] = []
        for (const file of uploadImages) {
          uploaded.push({ url: await uploadEditorImage(file) })
        }
        onImageListChange([...(imageList || []), ...uploaded])
      } catch (error) {
        catchError(error)
      } finally {
        setImageUploading(false)
        markdownEditorRef.current?.focus()
      }
    },
    [
      catchError,
      disabled,
      imageList,
      imageUploading,
      msgWarning,
      onImageListChange,
      t,
    ]
  )

  function openImagePicker() {
    if (disabled) {
      return
    }
    setShowImageUpload(true)
    setIsFocus(true)
    markdownEditorRef.current?.focus()
    if (!canAddImage) {
      msgWarning(
        t("component.imageUpload.countLimitError", {
          limit: COMMENT_IMAGE_LIMIT,
        })
      )
      return
    }
    isOpeningImagePickerRef.current = true
    window.addEventListener("focus", unlockImagePickerBlurGuard, { once: true })
    fileInputRef.current?.click()
  }

  function submit() {
    if (disabled) {
      return
    }
    if (imageUploading) {
      msgWarning(t("component.textEditor.pleaseWait"))
      return
    }
    onSubmit()
  }

  function onPaste(event: React.ClipboardEvent<HTMLDivElement>) {
    if (disabled) {
      return
    }
    const files = Array.from(event.clipboardData.items)
      .filter((item) => item.type.startsWith("image/"))
      .map((item) => item.getAsFile())
      .filter((file): file is File => Boolean(file))

    if (!files.length) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    void uploadFiles(files)
  }

  function onDrop(event: React.DragEvent<HTMLDivElement>) {
    if (disabled) {
      return
    }
    const files = Array.from(event.dataTransfer.files).filter((file) =>
      file.type.startsWith("image/")
    )
    if (!files.length) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    void uploadFiles(files)
  }

  function onBlur(event: React.FocusEvent<HTMLDivElement>) {
    const nextTarget = event.relatedTarget
    if (nextTarget && wrapperRef.current?.contains(nextTarget)) {
      return
    }
    if (isOpeningImagePickerRef.current) {
      return
    }
    setIsFocus(false)
    setShowImageUpload(false)
  }

  const dynamicHeight =
    isFocus && focusHeight > 0
      ? focusHeight + (showImageUpload ? 90 : 0)
      : height + (showImageUpload ? 90 : 0)

  return (
    <div
      ref={wrapperRef}
      className={cn(
        "flex flex-col rounded-lg border border-transparent bg-muted transition-all duration-200",
        focusHeight > 0 && "has-editor-focus",
        isFocus && "border-ring bg-background"
      )}
      style={{ height: dynamicHeight }}
      onBlur={onBlur}
      onPasteCapture={onPaste}
      onDropCapture={onDrop}
      onKeyDownCapture={(event) => {
        if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
          event.preventDefault()
          submit()
        }
      }}
    >
      <div className="min-h-0 flex-1 overflow-hidden rounded-t-lg">
        <MarkdownEditor
          ref={markdownEditorRef}
          value={content}
          placeholder={t("component.textEditor.placeholder")}
          height="100%"
          compact
          disabled={disabled}
          className={cn(
            "comment-markdown-editor",
            isFocus && "comment-markdown-editor-focused"
          )}
          onChange={(value) => {
            onContentChange(value)
            if (value) {
              setIsFocus(true)
            }
          }}
          onFocus={() => {
            setIsFocus(true)
            if (currentImages.length) {
              setShowImageUpload(true)
            }
          }}
        />
      </div>
      {showImageUpload ? (
        <div
          className="flex h-[90px] shrink-0 flex-wrap gap-2 overflow-auto p-2.5"
          onMouseDown={(event) => event.preventDefault()}
        >
          {currentImages.map((image, index) => (
            <div
              key={`${image.url || image.preview || index}`}
              className="group relative h-[60px] w-[60px] overflow-hidden rounded bg-background"
            >
              <PreviewableImage
                src={imageSrc(image)}
                previewSrcList={currentImages.map(imageSrc)}
                initialIndex={index}
                alt=""
                className="h-full w-full object-cover"
              />
              <button
                type="button"
                aria-label={t("component.textEditor.removeImage")}
                title={t("component.textEditor.removeImage")}
                disabled={disabled}
                className="absolute top-1 right-1 rounded bg-black/60 p-0.5 text-white opacity-80 hover:opacity-100 focus-visible:opacity-100 disabled:cursor-not-allowed disabled:opacity-40"
                onClick={() =>
                  onImageListChange(
                    imageList.filter((_, imageIndex) => imageIndex !== index)
                  )
                }
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          ))}
          {!imageUploading && canAddImage ? (
            <button
              type="button"
              aria-label={t("component.textEditor.addImage")}
              title={t("component.textEditor.addImage")}
              disabled={disabled}
              className="flex h-[60px] w-[60px] items-center justify-center rounded border border-dashed border-border bg-background text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-50"
              onClick={openImagePicker}
            >
              <Plus className="h-5 w-5" />
            </button>
          ) : null}
          {imageUploading ? (
            <div className="flex h-[60px] min-w-[60px] items-center justify-center rounded bg-background px-2 text-xs text-muted-foreground">
              {t("component.imageUpload.uploading")}
            </div>
          ) : null}
        </div>
      ) : null}
      <div
        className={cn(
          "flex h-9 shrink-0 items-center justify-between rounded-b-lg bg-muted px-2.5 py-[3px]",
          isFocus && "bg-background"
        )}
      >
        <button
          type="button"
          aria-label={t("component.textEditor.addImage")}
          title={t("component.textEditor.addImage")}
          disabled={disabled}
          className={cn(
            "flex cursor-pointer items-center gap-1 text-muted-foreground select-none hover:text-primary disabled:cursor-not-allowed disabled:opacity-50",
            showImageUpload && "font-medium text-primary"
          )}
          onClick={openImagePicker}
        >
          <ImageIcon className="h-5 w-5" />
        </button>
        <Button
          type="button"
          className="h-6"
          disabled={disabled}
          onClick={submit}
        >
          {t("component.textEditor.publish")}
        </Button>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          className="hidden"
          onChange={(event) => {
            unlockImagePickerBlurGuard()
            const files = Array.from(event.currentTarget.files || [])
            void uploadFiles(files)
            event.currentTarget.value = ""
          }}
        />
      </div>
    </div>
  )
})
