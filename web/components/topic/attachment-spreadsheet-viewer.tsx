"use client"

import * as React from "react"
import { Download, Minus, Plus } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  attachmentDownloadPath,
  attachmentSpreadsheetPreviewPath,
} from "@/lib/api/attachments"
import type { Attachment } from "@/lib/api/types"
import { useI18n } from "@/lib/i18n/provider"

import type {
  SpreadsheetCell,
  SpreadsheetMerge,
  SpreadsheetSheet,
  SpreadsheetSizeOverride,
  SpreadsheetWorkerResponse,
} from "./attachment-spreadsheet-types"

const DEFAULT_COLUMN_WIDTH = 96
const DEFAULT_ROW_HEIGHT = 24
const COLUMN_HEADER_HEIGHT = 28
const ROW_HEADER_WIDTH = 52
const MIN_ZOOM = 75
const MAX_ZOOM = 150
const ZOOM_STEP = 25

type Viewport = {
  width: number
  height: number
  scrollLeft: number
  scrollTop: number
}

type SelectedCell = SpreadsheetCell & { address: string }

function lowerBound(values: number[], target: number) {
  let low = 0
  let high = values.length
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (values[middle] < target) low = middle + 1
    else high = middle
  }
  return low
}

class SpreadsheetAxis {
  private readonly indexes: number[]
  private readonly sizes = new Map<number, number>()
  private readonly deltaPrefix: number[]

  constructor(
    readonly count: number,
    private readonly defaultSize: number,
    overrides: SpreadsheetSizeOverride[],
    scale: number
  ) {
    for (const override of overrides) {
      this.sizes.set(override.index, override.size * scale)
    }
    this.indexes = Array.from(this.sizes.keys()).sort((a, b) => a - b)
    this.deltaPrefix = [0]
    for (const index of this.indexes) {
      const delta =
        (this.sizes.get(index) ?? defaultSize * scale) - defaultSize * scale
      this.deltaPrefix.push(
        this.deltaPrefix[this.deltaPrefix.length - 1] + delta
      )
    }
    this.defaultSize = defaultSize * scale
  }

  position(index: number) {
    const bounded = Math.min(Math.max(index, 0), this.count)
    const overrideCount = lowerBound(this.indexes, bounded)
    return bounded * this.defaultSize + this.deltaPrefix[overrideCount]
  }

  size(index: number) {
    return this.sizes.get(index) ?? this.defaultSize
  }

  totalSize() {
    return this.position(this.count)
  }

  indexAt(offset: number) {
    if (this.count <= 1) return 0
    const bounded = Math.min(Math.max(offset, 0), this.totalSize())
    let low = 0
    let high = this.count
    while (low < high) {
      const middle = Math.floor((low + high) / 2)
      if (this.position(middle + 1) <= bounded) low = middle + 1
      else high = middle
    }
    return Math.min(low, this.count - 1)
  }
}

function columnName(index: number) {
  let value = index + 1
  let result = ""
  while (value > 0) {
    value -= 1
    result = String.fromCharCode(65 + (value % 26)) + result
    value = Math.floor(value / 26)
  }
  return result
}

function cellKey(row: number, column: number) {
  return `${row}:${column}`
}

function containsCell(merge: SpreadsheetMerge, row: number, column: number) {
  return (
    row >= merge.startRow &&
    row <= merge.endRow &&
    column >= merge.startColumn &&
    column <= merge.endColumn
  )
}

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

function SpreadsheetGridCell({
  cell,
  left,
  top,
  width,
  height,
  selected,
  onSelect,
}: {
  cell?: SpreadsheetCell
  left: number
  top: number
  width: number
  height: number
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      role="gridcell"
      className="absolute flex items-center overflow-hidden border-r border-b border-neutral-300 px-1.5 text-left text-xs whitespace-nowrap outline-none hover:bg-sky-50 focus-visible:z-10 focus-visible:ring-2 focus-visible:ring-sky-600 focus-visible:ring-inset"
      style={{
        left,
        top,
        width,
        height,
        backgroundColor: cell?.fill || "#ffffff",
        color: cell?.color || "#171717",
        fontWeight: cell?.bold ? 600 : undefined,
        textAlign: cell?.align,
        boxShadow: selected ? "inset 0 0 0 2px #0284c7" : undefined,
      }}
      title={cell?.text}
      onClick={onSelect}
    >
      <span className="block w-full truncate leading-[inherit]">
        {cell?.text}
      </span>
    </button>
  )
}

export function AttachmentSpreadsheetViewer({
  attachment,
}: {
  attachment: Attachment
}) {
  const { t } = useI18n()
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const workerRef = React.useRef<Worker | null>(null)
  const latestRequestRef = React.useRef(0)
  const [status, setStatus] = React.useState<"loading" | "ready" | "error">(
    "loading"
  )
  const [sheets, setSheets] = React.useState<SpreadsheetSheet[]>([])
  const [activeSheetPosition, setActiveSheetPosition] = React.useState(0)
  const [cells, setCells] = React.useState<SpreadsheetCell[]>([])
  const [merges, setMerges] = React.useState<SpreadsheetMerge[]>([])
  const [selectedCell, setSelectedCell] = React.useState<SelectedCell | null>(
    null
  )
  const [zoom, setZoom] = React.useState(100)
  const [viewport, setViewport] = React.useState<Viewport>({
    width: 0,
    height: 0,
    scrollLeft: 0,
    scrollTop: 0,
  })

  const activeSheet = sheets[activeSheetPosition]
  const scale = zoom / 100
  const columnAxis = React.useMemo(
    () =>
      activeSheet
        ? new SpreadsheetAxis(
            activeSheet.columnCount,
            DEFAULT_COLUMN_WIDTH,
            activeSheet.columnSizes,
            scale
          )
        : null,
    [activeSheet, scale]
  )
  const rowAxis = React.useMemo(
    () =>
      activeSheet
        ? new SpreadsheetAxis(
            activeSheet.rowCount,
            DEFAULT_ROW_HEIGHT,
            activeSheet.rowSizes,
            scale
          )
        : null,
    [activeSheet, scale]
  )

  React.useEffect(() => {
    const worker = new Worker(
      new URL("./attachment-spreadsheet.worker.ts", import.meta.url),
      { type: "module" }
    )
    workerRef.current = worker
    setStatus("loading")
    setSheets([])
    setCells([])
    setMerges([])
    setSelectedCell(null)
    setActiveSheetPosition(0)

    worker.onmessage = (event: MessageEvent<SpreadsheetWorkerResponse>) => {
      const response = event.data
      if (response.type === "ready") {
        setSheets(response.sheets)
        setStatus("ready")
        return
      }
      if (response.type === "cells") {
        if (response.requestId !== latestRequestRef.current) return
        setCells(response.cells)
        setMerges(response.merges)
        return
      }
      if (
        response.requestId == null ||
        response.requestId === latestRequestRef.current
      ) {
        setStatus("error")
      }
    }
    worker.onerror = () => setStatus("error")
    worker.postMessage({
      type: "load",
      url: attachmentSpreadsheetPreviewPath(attachment.id),
    })

    return () => {
      worker.terminate()
      if (workerRef.current === worker) workerRef.current = null
    }
  }, [attachment.id])

  React.useEffect(() => {
    const element = viewportRef.current
    if (!element) return
    let frame = 0
    const update = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        setViewport({
          width: element.clientWidth,
          height: element.clientHeight,
          scrollLeft: element.scrollLeft,
          scrollTop: element.scrollTop,
        })
      })
    }
    update()
    element.addEventListener("scroll", update, { passive: true })
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => {
      cancelAnimationFrame(frame)
      element.removeEventListener("scroll", update)
      observer.disconnect()
    }
  }, [status])

  const visibleRange = React.useMemo(() => {
    if (!activeSheet || !columnAxis || !rowAxis) return null
    const left = Math.max(0, viewport.scrollLeft - ROW_HEADER_WIDTH)
    const top = Math.max(0, viewport.scrollTop - COLUMN_HEADER_HEIGHT)
    const dataWidth = Math.max(1, viewport.width - ROW_HEADER_WIDTH)
    const dataHeight = Math.max(1, viewport.height - COLUMN_HEADER_HEIGHT)
    return {
      startColumn: Math.max(0, columnAxis.indexAt(left) - 2),
      endColumn: Math.min(
        activeSheet.columnCount - 1,
        columnAxis.indexAt(left + dataWidth) + 2
      ),
      startRow: Math.max(0, rowAxis.indexAt(top) - 4),
      endRow: Math.min(
        activeSheet.rowCount - 1,
        rowAxis.indexAt(top + dataHeight) + 4
      ),
    }
  }, [activeSheet, columnAxis, rowAxis, viewport])

  React.useEffect(() => {
    const worker = workerRef.current
    if (!worker || !activeSheet || !visibleRange || status !== "ready") return
    const requestId = latestRequestRef.current + 1
    latestRequestRef.current = requestId
    worker.postMessage({
      type: "cells",
      requestId,
      sheetIndex: activeSheet.index,
      startColumn: activeSheet.startColumn + visibleRange.startColumn,
      endColumn: activeSheet.startColumn + visibleRange.endColumn,
      startRow: activeSheet.startRow + visibleRange.startRow,
      endRow: activeSheet.startRow + visibleRange.endRow,
    })
  }, [activeSheet, status, visibleRange])

  const cellMap = React.useMemo(
    () => new Map(cells.map((cell) => [cellKey(cell.row, cell.column), cell])),
    [cells]
  )

  function selectSheet(position: number) {
    setActiveSheetPosition(position)
    setCells([])
    setMerges([])
    setSelectedCell(null)
    viewportRef.current?.scrollTo({ left: 0, top: 0 })
  }

  function selectCell(row: number, column: number, cell?: SpreadsheetCell) {
    setSelectedCell({
      row,
      column,
      text: cell?.text || "",
      formula: cell?.formula,
      fill: cell?.fill,
      color: cell?.color,
      bold: cell?.bold,
      align: cell?.align,
      address: `${columnName(column)}${row + 1}`,
    })
  }

  const renderedCells: React.ReactNode[] = []
  if (activeSheet && columnAxis && rowAxis && visibleRange) {
    for (
      let row = visibleRange.startRow;
      row <= visibleRange.endRow;
      row += 1
    ) {
      const actualRow = activeSheet.startRow + row
      for (
        let column = visibleRange.startColumn;
        column <= visibleRange.endColumn;
        column += 1
      ) {
        const actualColumn = activeSheet.startColumn + column
        if (
          merges.some((merge) => containsCell(merge, actualRow, actualColumn))
        ) {
          continue
        }
        const cell = cellMap.get(cellKey(actualRow, actualColumn))
        renderedCells.push(
          <SpreadsheetGridCell
            key={`${row}:${column}`}
            cell={cell}
            left={ROW_HEADER_WIDTH + columnAxis.position(column)}
            top={COLUMN_HEADER_HEIGHT + rowAxis.position(row)}
            width={columnAxis.size(column)}
            height={rowAxis.size(row)}
            selected={
              selectedCell?.row === actualRow &&
              selectedCell.column === actualColumn
            }
            onSelect={() => selectCell(actualRow, actualColumn, cell)}
          />
        )
      }
    }

    for (const merge of merges) {
      const startColumn = merge.startColumn - activeSheet.startColumn
      const endColumn = merge.endColumn - activeSheet.startColumn
      const startRow = merge.startRow - activeSheet.startRow
      const endRow = merge.endRow - activeSheet.startRow
      const cell = cellMap.get(cellKey(merge.startRow, merge.startColumn))
      renderedCells.push(
        <SpreadsheetGridCell
          key={`merge:${merge.startRow}:${merge.startColumn}`}
          cell={cell}
          left={ROW_HEADER_WIDTH + columnAxis.position(startColumn)}
          top={COLUMN_HEADER_HEIGHT + rowAxis.position(startRow)}
          width={
            columnAxis.position(endColumn + 1) -
            columnAxis.position(startColumn)
          }
          height={rowAxis.position(endRow + 1) - rowAxis.position(startRow)}
          selected={
            selectedCell?.row === merge.startRow &&
            selectedCell.column === merge.startColumn
          }
          onSelect={() => selectCell(merge.startRow, merge.startColumn, cell)}
        />
      )
    }
  }

  if (status === "loading") {
    return (
      <div
        className="flex min-h-[50dvh] items-center justify-center border-y border-border text-sm text-muted-foreground"
        role="status"
      >
        {t("pages.topic.attachmentPreview.loading")}
      </div>
    )
  }

  if (status === "error" || !activeSheet || !columnAxis || !rowAxis) {
    return (
      <div className="flex min-h-[50dvh] flex-col items-center justify-center gap-3 border-y border-border px-4 text-center">
        <p className="text-sm text-destructive" role="alert">
          {t("pages.topic.attachmentPreview.spreadsheetLoadError")}
        </p>
        <Button asChild>
          <a href={attachmentDownloadPath(attachment.id)}>
            <Download aria-hidden="true" />
            {t("pages.topic.detail.download")}
          </a>
        </Button>
      </div>
    )
  }

  return (
    <div className="overflow-hidden border-y border-border bg-neutral-100">
      <div className="flex h-12 items-center gap-2 border-b border-border bg-background px-2 sm:px-3">
        <div className="flex h-8 min-w-0 flex-1 items-center overflow-hidden rounded border border-border bg-muted/30 text-xs">
          <span className="w-16 shrink-0 border-r border-border px-2 text-center font-medium tabular-nums">
            {selectedCell?.address || ""}
          </span>
          <span
            className="min-w-0 flex-1 truncate px-2 text-muted-foreground"
            title={selectedCell?.formula || selectedCell?.text}
          >
            {selectedCell?.formula || selectedCell?.text || ""}
          </span>
        </div>
        <div className="flex h-8 shrink-0 items-center gap-0.5">
          <ToolButton
            label={t("pages.topic.attachmentPreview.zoomOut")}
            disabled={zoom <= MIN_ZOOM}
            onClick={() =>
              setZoom((value) => Math.max(MIN_ZOOM, value - ZOOM_STEP))
            }
          >
            <Minus aria-hidden="true" />
          </ToolButton>
          <span className="w-11 text-center text-xs text-muted-foreground tabular-nums">
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

      <div className="relative h-[calc(100dvh-16rem)] min-h-96 overflow-hidden bg-white">
        <div
          ref={viewportRef}
          role="grid"
          aria-rowcount={activeSheet.rowCount}
          aria-colcount={activeSheet.columnCount}
          className="absolute inset-0 overflow-auto"
        >
          <div
            className="relative"
            style={{
              width: ROW_HEADER_WIDTH + columnAxis.totalSize(),
              height: COLUMN_HEADER_HEIGHT + rowAxis.totalSize(),
            }}
          >
            {renderedCells}
          </div>
        </div>

        <div
          className="pointer-events-none absolute top-0 right-0 left-0 h-7 overflow-hidden border-b border-neutral-300 bg-neutral-100"
          aria-hidden="true"
        >
          {visibleRange
            ? Array.from(
                {
                  length: visibleRange.endColumn - visibleRange.startColumn + 1,
                },
                (_, offset) => visibleRange.startColumn + offset
              ).map((column) => (
                <div
                  key={column}
                  className="absolute top-0 flex h-7 items-center justify-center border-r border-neutral-300 text-[11px] font-medium text-neutral-600"
                  style={{
                    left:
                      ROW_HEADER_WIDTH +
                      columnAxis.position(column) -
                      viewport.scrollLeft,
                    width: columnAxis.size(column),
                  }}
                >
                  {columnName(activeSheet.startColumn + column)}
                </div>
              ))
            : null}
        </div>

        <div
          className="pointer-events-none absolute top-0 bottom-0 left-0 w-[52px] overflow-hidden border-r border-neutral-300 bg-neutral-100"
          aria-hidden="true"
        >
          {visibleRange
            ? Array.from(
                { length: visibleRange.endRow - visibleRange.startRow + 1 },
                (_, offset) => visibleRange.startRow + offset
              ).map((row) => (
                <div
                  key={row}
                  className="absolute left-0 flex w-[52px] items-center justify-center border-b border-neutral-300 text-[11px] font-medium text-neutral-600 tabular-nums"
                  style={{
                    top:
                      COLUMN_HEADER_HEIGHT +
                      rowAxis.position(row) -
                      viewport.scrollTop,
                    height: rowAxis.size(row),
                  }}
                >
                  {activeSheet.startRow + row + 1}
                </div>
              ))
            : null}
        </div>
        <div
          className="pointer-events-none absolute top-0 left-0 h-7 w-[52px] border-r border-b border-neutral-300 bg-neutral-200"
          aria-hidden="true"
        />
      </div>

      <div className="flex h-9 items-stretch justify-between border-t border-border bg-background">
        <div
          className="flex min-w-0 flex-1 items-stretch overflow-x-auto"
          role="tablist"
          aria-label={t("pages.topic.attachmentPreview.worksheets")}
        >
          {sheets.map((sheet, position) => (
            <button
              key={sheet.index}
              type="button"
              role="tab"
              aria-selected={position === activeSheetPosition}
              className="min-w-24 shrink-0 border-r border-border px-3 text-xs font-medium whitespace-nowrap hover:bg-muted focus-visible:ring-2 focus-visible:ring-sky-600 focus-visible:outline-none focus-visible:ring-inset aria-selected:border-b-2 aria-selected:border-b-sky-600 aria-selected:bg-sky-50 aria-selected:text-sky-800"
              onClick={() => selectSheet(position)}
            >
              {sheet.name}
            </button>
          ))}
        </div>
        <div className="hidden shrink-0 items-center border-l border-border px-3 text-[11px] text-muted-foreground tabular-nums sm:flex">
          {t("pages.topic.attachmentPreview.sheetDimensions", {
            rows: activeSheet.rowCount,
            columns: activeSheet.columnCount,
          })}
        </div>
      </div>
    </div>
  )
}
