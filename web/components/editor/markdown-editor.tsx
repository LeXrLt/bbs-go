"use client"

import * as React from "react"
import dynamic from "@/lib/router/dynamic"
import type { EditorProps, ExposeParam, ToolbarNames } from "md-editor-rt"

import "md-editor-rt/lib/style.css"

import { useTheme } from "@/components/theme-provider"
import { uploadEditorImage } from "@/components/editor/upload"
import { useI18n } from "@/lib/i18n/provider"

const MdEditor = dynamic(
  () => import("md-editor-rt").then((mod) => mod.MdEditor),
  { ssr: false }
) as React.ComponentType<EditorProps & React.RefAttributes<ExposeParam>>

const DEFAULT_TOOLBARS = [
  "bold",
  "underline",
  "italic",
  "strikeThrough",
  "-",
  "title",
  "sub",
  "sup",
  "quote",
  "unorderedList",
  "orderedList",
  "task",
  "-",
  "codeRow",
  "code",
  "link",
  "image",
  "table",
  "-",
  "revoke",
  "next",
  "-",
  "preview",
  "catalog",
  "=",
  "fullscreen",
] satisfies ToolbarNames[]

const COMPACT_TOOLBARS = [
  "bold",
  "italic",
  "strikeThrough",
  "title",
  "quote",
  "unorderedList",
  "orderedList",
  "codeRow",
  "code",
  "link",
  "table",
  "revoke",
  "next",
  "=",
  "preview",
] satisfies ToolbarNames[]

export type MarkdownEditorRef = {
  focus: () => void
  resetHistory: () => void
}

type MarkdownEditorProps = {
  value: string
  placeholder?: string
  height?: string
  compact?: boolean
  disabled?: boolean
  autoFocus?: boolean
  className?: string
  onChange: (value: string) => void
  onFocus?: () => void
  onBlur?: (event: FocusEvent) => void
}

export const MarkdownEditor = React.forwardRef<
  MarkdownEditorRef,
  MarkdownEditorProps
>(function MarkdownEditor(
  {
    value,
    placeholder,
    height = "400px",
    compact = false,
    disabled,
    autoFocus,
    className,
    onChange,
    onFocus,
    onBlur,
  },
  ref
) {
  const { resolvedTheme } = useTheme()
  const { locale } = useI18n()
  const editorRef = React.useRef<ExposeParam | null>(null)
  const pendingFocusRef = React.useRef(false)
  const setEditorRef = React.useCallback((editor: ExposeParam | null) => {
    editorRef.current = editor
    if (editor && pendingFocusRef.current) {
      pendingFocusRef.current = false
      editor.focus()
    }
  }, [])

  React.useImperativeHandle(ref, () => ({
    focus() {
      if (editorRef.current) {
        editorRef.current.focus()
      } else {
        pendingFocusRef.current = true
      }
    },
    resetHistory() {
      editorRef.current?.resetHistory()
    },
  }))

  async function uploadImg(files: File[], callback: (urls: string[]) => void) {
    const urls = await Promise.all(files.map((file) => uploadEditorImage(file)))
    callback(urls)
  }

  return (
    <MdEditor
      ref={setEditorRef}
      modelValue={value}
      theme={resolvedTheme === "dark" ? "dark" : "light"}
      toolbars={compact ? COMPACT_TOOLBARS : DEFAULT_TOOLBARS}
      style={{ height }}
      className={className}
      placeholder={placeholder}
      preview={!compact}
      language={locale}
      footers={[]}
      disabled={disabled}
      autoFocus={autoFocus}
      noPrettier={compact}
      onChange={onChange}
      onFocus={onFocus}
      onBlur={onBlur}
      onUploadImg={compact ? undefined : uploadImg}
    />
  )
})
