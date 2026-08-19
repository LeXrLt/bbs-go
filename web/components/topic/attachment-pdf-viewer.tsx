"use client"

import * as React from "react"
import {
  ChevronLeft,
  ChevronRight,
  Download,
  Maximize2,
  Minus,
  Plus,
} from "lucide-react"
import pdfWorkerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url"
import { Document, Page, pdfjs } from "react-pdf"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  attachmentDownloadPath,
  attachmentPreviewPath,
} from "@/lib/api/attachments"
import type { Attachment } from "@/lib/api/types"
import { useI18n } from "@/lib/i18n/provider"

import "react-pdf/dist/Page/AnnotationLayer.css"
import "react-pdf/dist/Page/TextLayer.css"

pdfjs.GlobalWorkerOptions.workerSrc = pdfWorkerUrl

const MIN_ZOOM = 50
const MAX_ZOOM = 200
const ZOOM_STEP = 25

function ToolButton({
  label,
  children,
  ...props
}: React.ComponentProps<typeof Button> & { label: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          {...props}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

export function AttachmentPdfViewer({
  attachment,
}: {
  attachment: Attachment
}) {
  const { t } = useI18n()
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const [numPages, setNumPages] = React.useState(0)
  const [pageNumber, setPageNumber] = React.useState(1)
  const [pageInput, setPageInput] = React.useState("1")
  const [zoom, setZoom] = React.useState(100)
  const [viewportWidth, setViewportWidth] = React.useState(720)
  const [loadError, setLoadError] = React.useState(false)

  React.useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return

    const updateWidth = () => {
      setViewportWidth(Math.max(280, viewport.clientWidth - 32))
    }
    updateWidth()

    const observer = new ResizeObserver(updateWidth)
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [])

  React.useEffect(() => {
    setPageInput(String(pageNumber))
  }, [pageNumber])

  React.useEffect(() => {
    setNumPages(0)
    setPageNumber(1)
    setPageInput("1")
    setZoom(100)
    setLoadError(false)
  }, [attachment.id])

  const fitWidth = Math.min(viewportWidth, 960)
  const renderedWidth = Math.round((fitWidth * zoom) / 100)
  const file = React.useMemo(
    () => ({
      url: attachmentPreviewPath(attachment.id),
      withCredentials: true,
    }),
    [attachment.id]
  )

  function goToPage(value: number) {
    const page = Math.min(Math.max(1, value), Math.max(1, numPages))
    setPageNumber(page)
    setPageInput(String(page))
    viewportRef.current?.scrollTo({ top: 0 })
  }

  function commitPageInput() {
    const value = Number.parseInt(pageInput, 10)
    goToPage(Number.isFinite(value) ? value : pageNumber)
  }

  return (
    <div className="overflow-hidden border-y border-border bg-muted/40">
      <div className="sticky top-0 z-10 flex min-h-12 flex-wrap items-center justify-center gap-1 border-b border-border bg-background/95 px-2 py-2 backdrop-blur sm:gap-2">
        <div className="flex h-8 items-center gap-1">
          <ToolButton
            label={t("pages.topic.attachmentPreview.previousPage")}
            disabled={pageNumber <= 1}
            onClick={() => goToPage(pageNumber - 1)}
          >
            <ChevronLeft aria-hidden="true" />
          </ToolButton>
          <form
            className="flex h-8 items-center gap-1 text-sm text-muted-foreground"
            onSubmit={(event) => {
              event.preventDefault()
              commitPageInput()
            }}
          >
            <Input
              inputMode="numeric"
              aria-label={t("pages.topic.attachmentPreview.pageNumber")}
              className="h-8 w-14 px-1 text-center"
              value={pageInput}
              onChange={(event) => setPageInput(event.currentTarget.value)}
              onBlur={commitPageInput}
            />
            <span className="w-12 text-center whitespace-nowrap tabular-nums">
              / {numPages || "-"}
            </span>
          </form>
          <ToolButton
            label={t("pages.topic.attachmentPreview.nextPage")}
            disabled={!numPages || pageNumber >= numPages}
            onClick={() => goToPage(pageNumber + 1)}
          >
            <ChevronRight aria-hidden="true" />
          </ToolButton>
        </div>

        <div className="mx-1 hidden h-5 w-px bg-border sm:block" />

        <div className="flex h-8 items-center gap-1">
          <ToolButton
            label={t("pages.topic.attachmentPreview.zoomOut")}
            disabled={zoom <= MIN_ZOOM}
            onClick={() =>
              setZoom((value) => Math.max(MIN_ZOOM, value - ZOOM_STEP))
            }
          >
            <Minus aria-hidden="true" />
          </ToolButton>
          <span className="w-12 text-center text-sm text-muted-foreground tabular-nums">
            {zoom}%
          </span>
          <ToolButton
            label={t("pages.topic.attachmentPreview.zoomIn")}
            disabled={zoom >= MAX_ZOOM}
            onClick={() =>
              setZoom((value) => Math.min(MAX_ZOOM, value + ZOOM_STEP))
            }
          >
            <Plus aria-hidden="true" />
          </ToolButton>
          <ToolButton
            label={t("pages.topic.attachmentPreview.fitWidth")}
            disabled={zoom === 100}
            onClick={() => setZoom(100)}
          >
            <Maximize2 aria-hidden="true" />
          </ToolButton>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button asChild variant="ghost" size="icon-sm">
                <a
                  href={attachmentDownloadPath(attachment.id)}
                  aria-label={t("pages.topic.detail.download")}
                >
                  <Download aria-hidden="true" />
                </a>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("pages.topic.detail.download")}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      <div
        ref={viewportRef}
        className="h-[calc(100dvh-13rem)] min-h-80 overflow-auto px-4 py-5"
      >
        <Document
          file={file}
          className="mx-auto w-max min-w-full"
          loading={
            <div
              className="flex min-h-80 items-center justify-center text-sm text-muted-foreground"
              role="status"
            >
              {t("pages.topic.attachmentPreview.loading")}
            </div>
          }
          error={
            <div
              className="flex min-h-80 items-center justify-center px-4 text-center text-sm text-destructive"
              role="alert"
            >
              {t("pages.topic.attachmentPreview.loadError")}
            </div>
          }
          onLoadSuccess={({ numPages: nextNumPages }) => {
            setLoadError(false)
            setNumPages(nextNumPages)
            const nextPage = Math.min(pageNumber, nextNumPages)
            setPageNumber(nextPage)
            setPageInput(String(nextPage))
          }}
          onLoadError={() => setLoadError(true)}
        >
          {loadError ? null : (
            <Page
              pageNumber={pageNumber}
              width={renderedWidth}
              className="mx-auto overflow-hidden bg-white shadow-sm"
              loading={
                <div
                  className="mx-auto min-h-80 bg-white"
                  style={{ width: renderedWidth }}
                />
              }
            />
          )}
        </Document>
      </div>
    </div>
  )
}
