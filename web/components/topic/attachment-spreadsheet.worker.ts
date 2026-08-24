import {
  read,
  utils,
  type CellObject,
  type ColInfo,
  type DenseWorkSheet,
  type Range,
  type RowInfo,
  type WorkBook,
} from "xlsx"

import type {
  SpreadsheetCell,
  SpreadsheetMerge,
  SpreadsheetSheet,
  SpreadsheetSizeOverride,
  SpreadsheetWorkerRequest,
  SpreadsheetWorkerResponse,
} from "./attachment-spreadsheet-types"

type WorkerScope = {
  onmessage: ((event: MessageEvent<SpreadsheetWorkerRequest>) => void) | null
  postMessage(message: SpreadsheetWorkerResponse): void
}

type SpreadsheetWorkSheet = DenseWorkSheet & {
  "!cols"?: ColInfo[]
  "!rows"?: RowInfo[]
  "!merges"?: Range[]
}

const scope = self as unknown as WorkerScope
const DEFAULT_COLUMN_WIDTH = 96
const DEFAULT_ROW_HEIGHT = 24
const MIN_COLUMN_WIDTH = 32
const MAX_COLUMN_WIDTH = 480
const MIN_ROW_HEIGHT = 20
const MAX_ROW_HEIGHT = 240

let workbook: WorkBook | null = null

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(Math.max(value, minimum), maximum)
}

function columnSize(value: ColInfo) {
  if (value?.hidden) return 0
  const width = value?.wpx ?? (value?.wch ? value.wch * 7 + 5 : undefined)
  return width == null
    ? DEFAULT_COLUMN_WIDTH
    : clamp(Math.round(width), MIN_COLUMN_WIDTH, MAX_COLUMN_WIDTH)
}

function rowSize(value: RowInfo) {
  if (value?.hidden) return 0
  const height = value?.hpt ? (value.hpt * 96) / 72 : value?.hpx
  return height == null
    ? DEFAULT_ROW_HEIGHT
    : clamp(Math.round(height), MIN_ROW_HEIGHT, MAX_ROW_HEIGHT)
}

function sizeOverrides<T>(
  values: T[] | undefined,
  start: number,
  count: number,
  getSize: (value: T) => number,
  defaultSize: number
) {
  const result: SpreadsheetSizeOverride[] = []
  values?.forEach((value, index) => {
    if (!value || index < start || index >= start + count) return
    const size = getSize(value)
    if (size !== defaultSize) result.push({ index: index - start, size })
  })
  return result
}

function sheetMetadata(sheetIndex: number): SpreadsheetSheet | null {
  if (!workbook) return null
  const name = workbook.SheetNames[sheetIndex]
  const sheet = workbook.Sheets[name] as SpreadsheetWorkSheet | undefined
  if (!sheet?.["!ref"]) return null

  const range = utils.decode_range(sheet["!ref"])
  const columnCount = range.e.c - range.s.c + 1
  const rowCount = range.e.r - range.s.r + 1
  if (columnCount <= 0 || rowCount <= 0) return null

  return {
    index: sheetIndex,
    name,
    startColumn: range.s.c,
    startRow: range.s.r,
    columnCount,
    rowCount,
    columnSizes: sizeOverrides(
      sheet["!cols"],
      range.s.c,
      columnCount,
      columnSize,
      DEFAULT_COLUMN_WIDTH
    ),
    rowSizes: sizeOverrides(
      sheet["!rows"],
      range.s.r,
      rowCount,
      rowSize,
      DEFAULT_ROW_HEIGHT
    ),
  }
}

function cssColor(value: unknown) {
  if (!value || typeof value !== "object") return undefined
  const rgb = (value as { rgb?: unknown }).rgb
  if (typeof rgb !== "string") return undefined
  const normalized = rgb.length === 8 ? rgb.slice(2) : rgb
  return /^[0-9a-f]{6}$/i.test(normalized) ? `#${normalized}` : undefined
}

function contrastingTextColor(background: string) {
  const red = Number.parseInt(background.slice(1, 3), 16)
  const green = Number.parseInt(background.slice(3, 5), 16)
  const blue = Number.parseInt(background.slice(5, 7), 16)
  return red * 0.299 + green * 0.587 + blue * 0.114 < 145
    ? "#ffffff"
    : undefined
}

function cellView(cell: CellObject, row: number, column: number) {
  const style =
    cell.s && typeof cell.s === "object"
      ? (cell.s as {
          patternType?: string
          fgColor?: unknown
          fill?: { patternType?: string; fgColor?: unknown }
          font?: { bold?: boolean; color?: unknown }
          alignment?: { horizontal?: string }
        })
      : undefined
  const patternType = style?.patternType ?? style?.fill?.patternType
  const fill =
    patternType === "solid"
      ? cssColor(style?.fgColor ?? style?.fill?.fgColor)
      : undefined
  const explicitColor = cssColor(style?.font?.color)
  const horizontal = style?.alignment?.horizontal
  const align =
    horizontal === "center" || horizontal === "right" || horizontal === "left"
      ? horizontal
      : cell.t === "n"
        ? "right"
        : undefined

  return {
    column,
    row,
    text: cell.w ?? utils.format_cell(cell),
    formula: cell.f ? `=${cell.f}` : undefined,
    fill,
    color: explicitColor ?? (fill ? contrastingTextColor(fill) : undefined),
    bold: style?.font?.bold || undefined,
    align,
  } satisfies SpreadsheetCell
}

function intersects(
  range: Range,
  startRow: number,
  endRow: number,
  startColumn: number,
  endColumn: number
) {
  return (
    range.s.r <= endRow &&
    range.e.r >= startRow &&
    range.s.c <= endColumn &&
    range.e.c >= startColumn
  )
}

function mergeView(range: Range) {
  return {
    startColumn: range.s.c,
    startRow: range.s.r,
    endColumn: range.e.c,
    endRow: range.e.r,
  } satisfies SpreadsheetMerge
}

function visibleCells(
  request: Extract<SpreadsheetWorkerRequest, { type: "cells" }>
) {
  if (!workbook) throw new Error("workbook is not loaded")
  const name = workbook.SheetNames[request.sheetIndex]
  const sheet = workbook.Sheets[name] as SpreadsheetWorkSheet | undefined
  if (!sheet?.["!ref"]) throw new Error("worksheet is not available")

  const usedRange = utils.decode_range(sheet["!ref"])
  const startRow = clamp(request.startRow, usedRange.s.r, usedRange.e.r)
  const endRow = clamp(request.endRow, startRow, usedRange.e.r)
  const startColumn = clamp(request.startColumn, usedRange.s.c, usedRange.e.c)
  const endColumn = clamp(request.endColumn, startColumn, usedRange.e.c)
  const cells = new Map<string, SpreadsheetCell>()

  const addCell = (row: number, column: number) => {
    const cell = sheet["!data"]?.[row]?.[column]
    if (cell) cells.set(`${row}:${column}`, cellView(cell, row, column))
  }

  for (let row = startRow; row <= endRow; row += 1) {
    for (let column = startColumn; column <= endColumn; column += 1) {
      addCell(row, column)
    }
  }

  const merges = (sheet["!merges"] ?? []).filter((range) =>
    intersects(range, startRow, endRow, startColumn, endColumn)
  )
  for (const merge of merges) addCell(merge.s.r, merge.s.c)

  return {
    cells: Array.from(cells.values()),
    merges: merges.map(mergeView),
  }
}

async function loadWorkbook(url: string) {
  const response = await fetch(url, {
    credentials: "include",
    cache: "no-store",
  })
  if (!response.ok)
    throw new Error(`spreadsheet request failed: ${response.status}`)
  const data = await response.arrayBuffer()
  workbook = read(data, {
    type: "array",
    dense: true,
    cellStyles: true,
    cellDates: false,
  })

  const visibleSheetIndexes = workbook.SheetNames.map(
    (_, index) => index
  ).filter((index) => !workbook?.Workbook?.Sheets?.[index]?.Hidden)
  const indexes = visibleSheetIndexes.length
    ? visibleSheetIndexes
    : workbook.SheetNames.map((_, index) => index)
  const sheets = indexes
    .map(sheetMetadata)
    .filter((sheet): sheet is SpreadsheetSheet => sheet != null)
  if (!sheets.length) throw new Error("workbook has no visible worksheets")
  scope.postMessage({ type: "ready", sheets })
}

scope.onmessage = (event) => {
  const request = event.data
  if (request.type === "load") {
    workbook = null
    void loadWorkbook(request.url).catch(() => {
      workbook = null
      scope.postMessage({ type: "error" })
    })
    return
  }

  try {
    const result = visibleCells(request)
    scope.postMessage({
      type: "cells",
      requestId: request.requestId,
      ...result,
    })
  } catch {
    scope.postMessage({ type: "error", requestId: request.requestId })
  }
}
