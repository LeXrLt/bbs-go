"use client"

import * as React from "react"
import {
  ActivityIcon,
  ArrowDownRightIcon,
  ArrowUpRightIcon,
  CalendarDaysIcon,
  DownloadIcon,
  ExternalLinkIcon,
  FileTextIcon,
  SearchIcon,
} from "lucide-react"
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"
import { useNavigate } from "react-router"

import { HtmlImagePreview } from "@/components/common/image-preview"
import { MainShell } from "@/components/layout/main-shell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { KalshiReport, KalshiRow } from "@/lib/api/kalshi"

type PageData = {
  dates: string[]
  date: string
  report: KalshiReport | null
  error: boolean
}

const views = [
  { key: "overview", label: "概览" },
  { key: "signals", label: "合约线索" },
  { key: "trends", label: "小时走势" },
  { key: "metadata", label: "采集与来源" },
  { key: "document", label: "完整文档" },
] as const

type View = (typeof views)[number]["key"]

const fieldLabels: Record<string, string> = {
  date: "所属日期", signal: "信号", category: "类别", title_zh: "事件", title_en: "英文标题",
  event_ticker: "事件代码", series_ticker: "系列代码", market_ticker: "盘口代码",
  outcome: "代表盘口", prob: "最近成交价", prev_prob: "接口前值", change_pp: "变动（百分点）",
  volume: "事件成交份数", market_volume: "盘口成交份数", opened: "开盘时间",
  related: "相关标的", note: "研究含义", link: "合约链接", key: "记录标识",
  bid: "买价", ask: "卖价", spread_pp: "买卖价差（百分点）",
  rules_primary: "主要结算规则", rules_secondary: "补充结算规则",
  observed_at: "报价观察时间", selection_basis: "盘口选择依据",
  prev_prob_basis: "接口前值口径", editorial_status: "内容复核状态",
  source_url: "数据来源", source_search_total_volume: "搜索返回成交份数",
  cohort: "入选批次", rank: "批次名次", event_volume: "事件成交份数",
  ts: "K 线时间（UTC）", close: "小时成交价", vol: "小时成交份数",
}

function text(value: unknown) {
  if (value === null || value === undefined || value === "") return "—"
  if (typeof value === "object") return JSON.stringify(value, null, 2)
  return String(value)
}

function numeric(value: unknown) {
  return typeof value === "number" ? value : null
}

function percent(value: unknown) {
  const n = numeric(value)
  return n === null ? "—" : `${(n * 100).toFixed(n * 100 % 1 ? 1 : 0)}%`
}

function number(value: unknown) {
  const n = numeric(value)
  return n === null ? "—" : new Intl.NumberFormat("zh-CN").format(n)
}

function time(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai", month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", hourCycle: "h23",
  }).format(date)
}

function safeURL(value: unknown) {
  if (typeof value !== "string") return null
  try {
    const url = new URL(value)
    return url.protocol === "https:" ? url.toString() : null
  } catch {
    return null
  }
}

function DetailFields({ row }: { row: KalshiRow }) {
  return (
    <div className="kalshi-fields">
      {Object.entries(row).map(([key, value]) => {
        const href = (key === "link" || key === "source_url") && safeURL(value)
        return (
          <div className="kalshi-field" key={key}>
            <dt>{fieldLabels[key] || key}</dt>
            <dd>{href ? <a href={href} target="_blank" rel="noopener noreferrer">{href} <ExternalLinkIcon className="inline size-3" /></a> : text(value)}</dd>
          </div>
        )
      })}
    </div>
  )
}

function Signals({ rows }: { rows: KalshiRow[] }) {
  const [query, setQuery] = React.useState("")
  const [signal, setSignal] = React.useState("")
  const [category, setCategory] = React.useState("")
  const [expanded, setExpanded] = React.useState<string | null>(null)
  const categories = [...new Set(rows.map((r) => text(r.category)).filter((v) => v !== "—"))].sort()
  const matching = rows.filter((r) =>
    (!signal || r.signal === signal) && (!category || r.category === category) &&
    (!query || Object.values(r).some((v) => text(v).toLocaleLowerCase().includes(query.toLocaleLowerCase())))
  )
  return (
    <section className="kalshi-section">
      <div className="kalshi-section-heading"><div><h2>合约线索</h2><p>共 {rows.length} 条，当前显示 {matching.length} 条；每条记录可展开全部原始字段。</p></div></div>
      <div className="kalshi-toolbar">
        <label className="kalshi-search"><SearchIcon className="size-4" /><Input aria-label="搜索合约线索" placeholder="搜索事件、代码或规则" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <select aria-label="筛选信号" value={signal} onChange={(e) => setSignal(e.target.value)}><option value="">所有信号</option>{[...new Set(rows.map((r) => text(r.signal)))].map((v) => <option key={v} value={v}>{v}</option>)}</select>
        <select aria-label="筛选类别" value={category} onChange={(e) => setCategory(e.target.value)}><option value="">所有类别</option>{categories.map((v) => <option key={v} value={v}>{v}</option>)}</select>
      </div>
      <div className="kalshi-table-scroll"><table className="kalshi-table"><thead><tr><th>信号 / 类别</th><th>事件与盘口</th><th>成交价</th><th>变动</th><th>成交份数</th><th>观察时间</th></tr></thead><tbody>
        {matching.map((row, index) => {
          const id = text(row.key) + index
          const open = expanded === id
          const delta = numeric(row.change_pp)
          return <React.Fragment key={id}>
            <tr className="kalshi-clickable" onClick={() => setExpanded(open ? null : id)} onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); setExpanded(open ? null : id) } }} tabIndex={0} aria-expanded={open}>
              <td><span className={`kalshi-signal kalshi-signal-${row.signal === "新上线" ? "new" : row.signal === "价格异动" ? "move" : "watch"}`}>{text(row.signal)}</span><small>{text(row.category)}</small></td>
              <td className="kalshi-title-cell"><strong>{text(row.title_zh)}</strong><small>{text(row.outcome)} · {text(row.event_ticker)}</small></td>
              <td className="kalshi-num">{percent(row.prob)}</td>
              <td className={`kalshi-num ${delta !== null && delta > 0 ? "kalshi-up" : delta !== null && delta < 0 ? "kalshi-down" : ""}`}>{delta === null ? "—" : `${delta > 0 ? "+" : ""}${delta}`}</td>
              <td className="kalshi-num">{number(row.volume)}</td><td>{row.observed_at ? time(text(row.observed_at)) : "—"}</td>
            </tr>
            {open ? <tr className="kalshi-expanded"><td colSpan={6}><DetailFields row={row} /></td></tr> : null}
          </React.Fragment>
        })}
      </tbody></table></div>
      {!matching.length && <p className="kalshi-empty">没有符合条件的记录。</p>}
    </section>
  )
}

function Trends({ rows }: { rows: KalshiRow[] }) {
  const markets = React.useMemo(() => [...new Set(rows.map((r) => text(r.market_ticker)))], [rows])
  const [market, setMarket] = React.useState(markets[0] || "")
  const selected = markets.includes(market) ? market : markets[0]
  const points = rows.filter((r) => r.market_ticker === selected).sort((a, b) => text(a.ts).localeCompare(text(b.ts)))
  const chart = points.map((r) => ({
    ts: text(r.ts), label: time(text(r.ts)),
    close: numeric(r.close) === null ? null : numeric(r.close)! * 100,
    bid: numeric(r.bid) === null ? null : numeric(r.bid)! * 100,
    ask: numeric(r.ask) === null ? null : numeric(r.ask)! * 100,
  }))
  return <section className="kalshi-section">
    <div className="kalshi-section-heading"><div><h2>小时走势</h2><p>{rows.length} 条原始 K 线，按盘口查看；缺失成交价保持空值，不插补。</p></div><select aria-label="选择盘口" value={selected || ""} onChange={(e) => setMarket(e.target.value)}>{markets.map((m) => <option key={m} value={m}>{m}</option>)}</select></div>
    {points.length ? <>
      <div className="kalshi-market-heading"><strong>{text(points[0].title_zh)}</strong><span>{text(points[0].outcome)} · 批次 {text(points[0].cohort)} · 名次 {text(points[0].rank)}</span></div>
      <div className="kalshi-chart" role="img" aria-label={`${text(points[0].title_zh)} 小时走势`}>
        <ResponsiveContainer width="100%" height="100%"><LineChart data={chart} margin={{ top: 14, right: 16, left: -18, bottom: 4 }}><CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" /><XAxis dataKey="label" minTickGap={32} tick={{ fontSize: 11 }} /><YAxis domain={[0, 100]} tickFormatter={(v: number) => `${v}%`} tick={{ fontSize: 11 }} /><Tooltip formatter={(v, name) => [`${Number(v).toFixed(1)}%`, name]} /><Legend /><Line type="stepAfter" dataKey="bid" name="买价" stroke="#457b83" dot={false} connectNulls={false} strokeWidth={1.5} /><Line type="stepAfter" dataKey="ask" name="卖价" stroke="#c79056" dot={false} connectNulls={false} strokeWidth={1.5} /><Line type="stepAfter" dataKey="close" name="成交价" stroke="#365eb6" dot={{ r: 2 }} connectNulls={false} strokeWidth={2} /></LineChart></ResponsiveContainer>
      </div>
      <div className="kalshi-table-scroll"><table className="kalshi-table"><thead><tr><th>时间（UTC）</th><th>买价</th><th>卖价</th><th>成交价</th><th>小时成交份数</th><th>完整字段</th></tr></thead><tbody>{points.map((row, i) => <TrendRow key={`${text(row.key)}-${i}`} row={row} />)}</tbody></table></div>
    </> : <p className="kalshi-empty">没有走势数据。</p>}
  </section>
}

function TrendRow({ row }: { row: KalshiRow }) {
  const [open, setOpen] = React.useState(false)
  return <><tr><td>{text(row.ts).replace("T", " ")}</td><td>{percent(row.bid)}</td><td>{percent(row.ask)}</td><td>{percent(row.close)}</td><td>{number(row.vol)}</td><td><button className="kalshi-detail-button" type="button" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? "收起" : "查看"}</button></td></tr>{open && <tr className="kalshi-expanded"><td colSpan={6}><DetailFields row={row} /></td></tr>}</>
}

function Metadata({ report }: { report: KalshiReport }) {
  const { manifest } = report
  const coverage = Array.isArray(manifest.metadata.coverage) ? manifest.metadata.coverage : []
  return <section className="kalshi-section kalshi-metadata"><div className="kalshi-section-heading"><div><h2>采集与来源</h2><p>完整的 manifest、字段口径与覆盖记录均保存在原始文档中。</p></div></div>
    <dl className="kalshi-metadata-grid"><div><dt>内容日期</dt><dd>{manifest.date}</dd></div><div><dt>扫描截至</dt><dd>{time(manifest.as_of)}</dd></div><div><dt>采集状态</dt><dd>{manifest.collection_status}</dd></div><div><dt>复核待完成</dt><dd>{text(manifest.metadata.editorial_pending)}</dd></div><div><dt>搜索请求</dt><dd>{text(manifest.metadata.search_success)} / {text(manifest.metadata.search_queries)}</dd></div><div><dt>覆盖记录</dt><dd>{coverage.length}</dd></div></dl>
    <h3>完整 manifest</h3><pre className="kalshi-json">{JSON.stringify(manifest, null, 2)}</pre>
    <h3>signals.json / trends.json 元数据</h3><pre className="kalshi-json">{JSON.stringify({ signals: report.signals.meta, trends: report.trends.meta }, null, 2)}</pre>
  </section>
}

export function KalshiPage({ data }: { data: PageData }) {
  const navigate = useNavigate()
  const [view, setView] = React.useState<View>("overview")
  const [documentMode, setDocumentMode] = React.useState<"rendered" | "source">("rendered")
  const report = data.report
  const counts = report?.manifest.counts
  const download = () => {
    if (!report) return
    const url = URL.createObjectURL(new Blob([report.markdown], { type: "text/markdown;charset=utf-8" }))
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = `kalshi-${report.date}.md`
    anchor.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return <MainShell containerClassName="kalshi-container"><div className="kalshi-page">
    <header className="kalshi-header"><div className="kalshi-heading"><div className="kalshi-eyebrow"><ActivityIcon className="size-4" /> 市场观察 / Kalshi</div><h1>Kalshi 合约跟踪</h1><p>按文档快照查看合约线索、报价与采集范围</p></div><div className="kalshi-header-actions"><label className="kalshi-date"><CalendarDaysIcon className="size-4" /><select aria-label="选择报告日期" value={data.date} disabled={!data.dates.length} onChange={(e) => navigate(`/kalshi?date=${encodeURIComponent(e.target.value)}`)}>{data.dates.length ? data.dates.map((date) => <option key={date} value={date}>{date}</option>) : <option value="">暂无报告</option>}</select></label><Button variant="outline" size="sm" onClick={download} disabled={!report} title="下载完整 Markdown"><DownloadIcon className="size-4" /> <span>原文</span></Button></div></header>
    {data.error ? <p className="kalshi-notice kalshi-error">报告暂时无法加载，请稍后重试。</p> : !report ? <p className="kalshi-empty">尚无已发布的 Kalshi 报告。</p> : <>
      {report.manifest.collection_status !== "complete" && <p className="kalshi-notice">本次采集状态为 {report.manifest.collection_status}，存在覆盖缺口；请查看“采集与来源”。</p>}
      <div className="kalshi-snapshot"><span>内容日期 <strong>{report.date}</strong></span><span>扫描截至 <strong>{time(report.as_of)}</strong></span><span>发布更新 <strong>{time(report.updated_at)}</strong></span><span className="kalshi-scope">{text(report.manifest.metadata.scope)}</span></div>
      <div className="kalshi-stats"><div><span>累计线索</span><strong>{number(counts?.signals_total)}</strong><small>样本范围内</small></div><div><span>本日新上线</span><strong>{number(counts?.today_new)}</strong><ArrowUpRightIcon className="size-4" /></div><div><span>价格异动</span><strong>{number(counts?.today_moves)}</strong><ArrowDownRightIcon className="size-4" /></div><div><span>持续观察</span><strong>{number(counts?.today_watch)}</strong><small>跟踪中</small></div><div><span>小时数据</span><strong>{number(counts?.trend_rows)}</strong><small>原始 K 线</small></div></div>
      <nav className="kalshi-tabs" aria-label="Kalshi 页面视图">{views.map(({ key, label }) => <button type="button" key={key} className={view === key ? "active" : ""} aria-current={view === key ? "page" : undefined} onClick={() => setView(key)}>{key === "document" && <FileTextIcon className="size-4" />}{label}</button>)}</nav>
      {view === "overview" && <section className="kalshi-section kalshi-prose"><HtmlImagePreview html={report.narrative_html} className="daily-review-content" /></section>}
      {view === "signals" && <Signals key={report.date} rows={report.signals.rows} />}
      {view === "trends" && <Trends key={report.date} rows={report.trends.rows} />}
      {view === "metadata" && <Metadata report={report} />}
      {view === "document" && <section className="kalshi-section kalshi-document"><div className="kalshi-section-heading"><div><h2>完整文档</h2><p>保留所有章节、原始 JSON 和 HTML 模板；代码仅展示，不在论坛中执行。</p></div><div className="kalshi-document-modes" role="group" aria-label="文档查看方式"><button type="button" aria-pressed={documentMode === "rendered"} onClick={() => setDocumentMode("rendered")}>排版</button><button type="button" aria-pressed={documentMode === "source"} onClick={() => setDocumentMode("source")}>原始 Markdown</button></div></div>{documentMode === "rendered" ? <HtmlImagePreview html={report.content_html} className="daily-review-content" /> : <pre className="kalshi-source">{report.markdown}</pre>}</section>}
    </>}
  </div></MainShell>
}
