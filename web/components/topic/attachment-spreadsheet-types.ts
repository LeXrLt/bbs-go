export type SpreadsheetCell = {
  column: number
  row: number
  text: string
  formula?: string
  fill?: string
  color?: string
  bold?: boolean
  align?: "left" | "center" | "right"
}

export type SpreadsheetMerge = {
  startColumn: number
  startRow: number
  endColumn: number
  endRow: number
}

export type SpreadsheetSizeOverride = {
  index: number
  size: number
}

export type SpreadsheetSheet = {
  index: number
  name: string
  startColumn: number
  startRow: number
  columnCount: number
  rowCount: number
  columnSizes: SpreadsheetSizeOverride[]
  rowSizes: SpreadsheetSizeOverride[]
}

export type SpreadsheetWorkerRequest =
  | { type: "load"; url: string }
  | {
      type: "cells"
      requestId: number
      sheetIndex: number
      startColumn: number
      startRow: number
      endColumn: number
      endRow: number
    }

export type SpreadsheetWorkerResponse =
  | { type: "ready"; sheets: SpreadsheetSheet[] }
  | {
      type: "cells"
      requestId: number
      cells: SpreadsheetCell[]
      merges: SpreadsheetMerge[]
    }
  | { type: "error"; requestId?: number }
