"use client"

import * as React from "react"
import {
  CalendarDaysIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  DownloadIcon,
  ExternalLinkIcon,
  SearchIcon,
} from "lucide-react"
import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"
import { useNavigate } from "react-router"

import { MainShell } from "@/components/layout/main-shell"
import type { KalshiReport, KalshiRow, KalshiSlot, KalshiSnapshot } from "@/lib/api/kalshi"
import { addCalendarDays, calendarToday, isCalendarDate } from "@/lib/calendar"

type PageData = {
  dates: string[]
  date: string
  slot: KalshiSlot | null
  snapshots: KalshiSnapshot[]
  report: KalshiReport | null
  error: boolean
}
type Sort = "date" | "rise" | "abs" | "vol"
const signals = ["新上线", "价格异动", "观察"] as const
const signalOrder: Record<string, number> = { 价格异动: 0, 新上线: 1, 观察: 2 }
const integer = new Intl.NumberFormat("zh-CN")

function value(v: unknown): string {
  if (v == null || v === "") return "—"
  return typeof v === "object" ? JSON.stringify(v, null, 2) : String(v)
}
function num(v: unknown): number | null {
  return typeof v === "number" && Number.isFinite(v) ? v : null
}
function count(v: unknown): string {
  const n = num(v)
  return n == null ? "—" : integer.format(n)
}
function pct(v: unknown): string {
  const n = num(v)
  return n == null ? "—" : `${Math.round(n * 100)}%`
}
function safeLink(v: unknown): string | null {
  if (typeof v !== "string") return null
  try {
    const url = new URL(v)
    return url.protocol === "https:" ? url.toString() : null
  } catch {
    return null
  }
}
function compareRows(a: KalshiRow, b: KalshiRow, sort: Sort): number {
  const fallback =
    value(b.date).localeCompare(value(a.date)) ||
    (signalOrder[value(a.signal)] ?? 3) - (signalOrder[value(b.signal)] ?? 3) ||
    Math.abs(num(b.change_pp) ?? 0) - Math.abs(num(a.change_pp) ?? 0)
  if (sort === "date") return fallback
  const metric = (r: KalshiRow) =>
    sort === "vol"
      ? num(r.volume)
      : sort === "abs"
        ? num(r.change_pp) == null
          ? null
          : Math.abs(num(r.change_pp)!)
        : num(r.change_pp)
  const x = metric(a),
    y = metric(b)
  return x == null || y == null
    ? x == null && y == null
      ? fallback
      : x == null
        ? 1
        : -1
    : y - x || fallback
}

function Detail({ row }: { row: KalshiRow }) {
  return (
    <dl className="kalshi-detail-grid">
      {Object.entries(row).map(([key, entry]) => {
        const href =
          key === "link" || key === "source_url" ? safeLink(entry) : null
        return (
          <div key={key}>
            <dt>{key}</dt>
            <dd>
              {href ? (
                <a href={href} target="_blank" rel="noopener noreferrer">
                  {href} <ExternalLinkIcon size={12} />
                </a>
              ) : (
                value(entry)
              )}
            </dd>
          </div>
        )
      })}
    </dl>
  )
}

function TrendCard({ rows }: { rows: KalshiRow[] }) {
  const points = [...rows].sort((a, b) =>
    value(a.ts).localeCompare(value(b.ts))
  )
  const first = points[0],
    last = points[points.length - 1]
  if (!first || !last) return null
  const opened = Date.parse(value(first.opened))
  const start = Number.isFinite(opened)
    ? Math.min(opened, Date.parse(value(first.ts)))
    : Date.parse(value(first.ts))
  const finish = Date.parse(value(last.ts))
  const lastTrade = [...points].reverse().find((p) => num(p.close) != null)
  const middle = (p: KalshiRow) => {
    const bid = num(p.bid),
      ask = num(p.ask)
    return bid != null && ask != null && bid <= ask && ask - bid <= 0.100001
      ? (bid + ask) / 2
      : null
  }
  const data = points.map((p) => ({
    ts: Date.parse(value(p.ts)),
    band:
      num(p.bid) != null && num(p.ask) != null && num(p.bid)! <= num(p.ask)!
        ? [num(p.bid)!, num(p.ask)!]
        : null,
    middle: middle(p),
    close: num(p.close),
    vol: num(p.vol),
  }))
  const spread =
    num(last.bid) != null && num(last.ask) != null
      ? `${Math.round((num(last.ask)! - num(last.bid)!) * 100)}个百分点`
      : "—"
  const href = safeLink(first.link)
  return (
    <article className="kalshi-trend-item">
      <h3>
        {href ? (
          <a href={href} target="_blank" rel="noopener noreferrer">
            {value(first.rank)}. {value(first.title_zh)}
          </a>
        ) : (
          `${value(first.rank)}. ${value(first.title_zh)}`
        )}
      </h3>
      <p>
        结果 <b>{value(first.outcome)}</b> · 中间价 <b>{pct(middle(last))}</b> ·
        价差 <b>{spread}</b> · 最近成交{" "}
        <b>{lastTrade ? pct(lastTrade.close) : "无"}</b> · 成交量{" "}
        <b>{count(first.event_volume)}</b>
      </p>
      <div
        className="kalshi-mini-chart"
        role="img"
        aria-label={`${value(first.title_zh)} 价格走势`}
      >
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart
            data={data}
            margin={{ top: 5, right: 8, bottom: 0, left: -16 }}
          >
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis
              dataKey="ts"
              type="number"
              domain={[start, Math.max(finish, start + 3600000)]}
              scale="time"
              tickFormatter={(s: number) =>
                new Date(s).toISOString().slice(5, 10).replace("-", "/")
              }
              minTickGap={35}
              tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
              tickLine={false}
            />
            <YAxis
              domain={[0, 1]}
              ticks={[0, 0.5, 1]}
              tickFormatter={(n: number) => `${Math.round(n * 100)}%`}
              tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
              tickLine={false}
            />
            <Tooltip
              contentStyle={{
                background: "var(--background)",
                border: "1px solid var(--border)",
                borderRadius: 6,
                color: "var(--foreground)",
              }}
              labelStyle={{ color: "var(--muted-foreground)" }}
              labelFormatter={(s) =>
                `${new Date(Number(s)).toISOString().slice(0, 16).replace("T", " ")} UTC`
              }
              formatter={(v, name, item) => {
                const price = Array.isArray(v)
                  ? `${pct(v[0])}–${pct(v[1])}`
                  : pct(v)
                if (name !== "实际成交价") return [price, name]
                const volume = num(item.payload?.vol)
                return [
                  `${price} · 小时成交量 ${volume == null ? "无数据" : `${count(Math.round(volume))} 张`}`,
                  name,
                ]
              }}
            />
            <Area
              dataKey="band"
              name="买卖报价区间"
              type="stepAfter"
              fill="var(--kalshi-accent)"
              fillOpacity={0.13}
              stroke="none"
              connectNulls={false}
              isAnimationActive={false}
            />
            <Line
              dataKey="middle"
              name="窄价差中间价"
              type="stepAfter"
              stroke="var(--kalshi-accent)"
              strokeWidth={1.5}
              dot={false}
              connectNulls={false}
              isAnimationActive={false}
            />
            <Line
              dataKey="close"
              name="实际成交价"
              stroke="none"
              dot={{ r: 2.5, fill: "var(--foreground)", stroke: "none" }}
              activeDot={{ r: 4 }}
              connectNulls={false}
              isAnimationActive={false}
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>
    </article>
  )
}

function Dashboard({ report }: { report: KalshiReport }) {
  const [cohort, setCohort] = React.useState("")
  const [signal, setSignal] = React.useState("")
  const [category, setCategory] = React.useState("")
  const [sort, setSort] = React.useState<Sort>("date")
  const [query, setQuery] = React.useState("")
  const [range, setRange] = React.useState<"30d" | "all">("30d")
  const [expanded, setExpanded] = React.useState<string | null>(null)
  const rows = report.signals.rows,
    trends = report.trends.rows
  const cohorts = [...new Set(trends.map((r) => value(r.cohort)))]
    .sort()
    .reverse()
  const selectedCohort = cohorts.includes(cohort) ? cohort : cohorts[0]
  const cohortRows = trends.filter((r) => r.cohort === selectedCohort)
  const markets = [...new Set(cohortRows.map((r) => value(r.market_ticker)))]
    .map((ticker) => cohortRows.filter((r) => r.market_ticker === ticker))
    .sort(
      (a, b) => (num(a[0]?.rank) ?? Infinity) - (num(b[0]?.rank) ?? Infinity)
    )
  const categories = [...new Set(rows.map((r) => value(r.category)))].sort()
  const term = query.trim().toLocaleLowerCase()
  const visible = rows
    .filter(
      (r) =>
        (!signal || r.signal === signal) &&
        (!category || r.category === category) &&
        (!term ||
          [
            r.title_zh,
            r.title_en,
            r.event_ticker,
            r.series_ticker,
            r.related,
            r.note,
            r.outcome,
          ].some(
            (v) => v != null && String(v).toLocaleLowerCase().includes(term)
          ))
    )
    .sort((a, b) => compareRows(a, b, sort))
  const end = report.date
  const since = new Date(`${end}T00:00:00Z`)
  since.setUTCDate(since.getUTCDate() - 29)
  const from = since.toISOString().slice(0, 10)
  const categoryRows = rows.filter(
    (r) => range === "all" || (value(r.date) >= from && value(r.date) <= end)
  )
  const totals = [...new Set(categoryRows.map((r) => value(r.category)))]
    .map((name) => ({
      name,
      total: categoryRows.filter((r) => value(r.category) === name).length,
    }))
    .sort((a, b) => b.total - a.total || a.name.localeCompare(b.name))
  const max = Math.max(1, ...totals.map((r) => r.total))
  const counts = Object.fromEntries(
    signals.map((name) => [
      name,
      rows.filter((r) => r.signal === name && r.date === end).length,
    ])
  )
  const firstDate = [...rows.map((r) => value(r.date))].sort()[0]
  return (
    <>
      <header className="kalshi-heading">
        <span className="kalshi-eyebrow">KALSHI / 每日跟踪</span>
        <h1>市场线索</h1>
        <p className="kalshi-run-state">
          扫描截至 {report.as_of} ·{" "}
          {report.manifest.collection_status === "complete"
            ? "采集完成"
            : "部分采集"}{" "}
          · 分类标签搜索样本，非全站穷尽
        </p>
        <p>
          截至扫描（<b>{end}</b>）累计记录 <b>{count(rows.length)}</b>{" "}
          条线索；本日新上线事件 <b>{counts["新上线"]}</b> 条，价格异动{" "}
          <b>{counts["价格异动"]}</b> 条，持续观察 <b>{counts["观察"]}</b>{" "}
          条。体育类逐场盘口和15分钟、每小时、每日滚动合约不计入。
        </p>
      </header>
      {report.manifest.collection_status !== "complete" && (
        <p className="kalshi-alert">
          本次采集存在覆盖缺口，请结合文档中的数据缺口说明查看。
        </p>
      )}
      <section className="kalshi-tiles" aria-label="信号计数">
        <div>
          <span>今日新上线</span>
          <strong>{counts["新上线"]}</strong>
          <small>Kalshi新开出的非体育合约</small>
        </div>
        <div>
          <span>今日价格异动</span>
          <strong>{counts["价格异动"]}</strong>
          <small>较接口前值变动至少10个百分点</small>
        </div>
        <div>
          <span>今日观察</span>
          <strong>{counts["观察"]}</strong>
          <small>成交活跃、与投资相关的存量合约</small>
        </div>
      </section>
      <section className="kalshi-panel" id="signals">
        <div className="kalshi-panel-heading">
          <h2>浏览市场线索</h2>
          <span>{visible.length} 条结果</span>
        </div>
        <p>
          点标题打开Kalshi合约页。成交价为所选代表盘口的最近成交价，变动与同一盘口的接口前值比较；前值周期未独立核实。成交很薄的合约价格仅供参考。
        </p>
        <div className="kalshi-filters">
          <div className="kalshi-segment" role="group" aria-label="信号类型">
            {["", ...signals].map((s) => (
              <button
                key={s}
                type="button"
                aria-pressed={signal === s}
                onClick={() => setSignal(s)}
              >
                {s || "全部"}
              </button>
            ))}
          </div>
          <select
            aria-label="类别"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
          >
            <option value="">全部类别</option>
            {categories.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <label className="kalshi-select-label">
            排序{" "}
            <select
              aria-label="排序方式"
              value={sort}
              onChange={(e) => setSort(e.target.value as Sort)}
            >
              <option value="date">最新日期</option>
              <option value="rise">涨幅最大（百分点）</option>
              <option value="abs">变动幅度最大（涨跌都算）</option>
              <option value="vol">成交量最大</option>
            </select>
          </label>
          <label className="kalshi-search">
            <SearchIcon size={16} />
            <input
              type="search"
              aria-label="搜索"
              placeholder="搜索标题、代码、相关股票"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
        </div>
        <div
          className="kalshi-market-list"
          role="list"
          aria-label="市场线索列表"
          tabIndex={0}
        >
          {visible.map((r, i) => {
            const key = `${value(r.key)}-${i}`
            const href = safeLink(r.link)
            const delta = num(r.change_pp)
            return (
              <article className="kalshi-market" role="listitem" key={key}>
                <div className="kalshi-market-meta">
                  <span
                    className={`kalshi-pill ${r.signal === "新上线" ? "new" : r.signal === "价格异动" ? "move" : "watch"}`}
                  >
                    {value(r.signal)}
                  </span>
                  <span>{value(r.category)}</span>
                  <time dateTime={value(r.date)}>{value(r.date)}</time>
                </div>
                <div className="kalshi-market-summary">
                  <h3>
                    {href ? (
                      <a href={href} target="_blank" rel="noopener noreferrer">
                        {value(r.title_zh)}{" "}
                        <ExternalLinkIcon size={14} aria-hidden="true" />
                      </a>
                    ) : (
                      value(r.title_zh)
                    )}
                  </h3>
                  <span className="kalshi-market-ticker">
                    {value(r.event_ticker || r.series_ticker)}
                  </span>
                  <p className="kalshi-market-note">{value(r.note)}</p>
                  <span className="kalshi-market-related">
                    相关标的：{value(r.related)}
                  </span>
                </div>
                <dl className="kalshi-market-metrics">
                  <div>
                    <dt>代表盘口结果</dt>
                    <dd>{value(r.outcome)}</dd>
                  </div>
                  <div>
                    <dt>最近成交价</dt>
                    <dd className="kalshi-market-price">{pct(r.prob)}</dd>
                  </div>
                  <div>
                    <dt>变动</dt>
                    <dd
                      className={
                        delta != null && delta > 0
                          ? "kalshi-up"
                          : delta != null && delta < 0
                            ? "kalshi-down"
                            : ""
                      }
                    >
                      {delta == null
                        ? "—"
                        : `${delta > 0 ? "+" : ""}${delta} pp`}
                    </dd>
                  </div>
                  <div>
                    <dt>事件成交量</dt>
                    <dd>
                      {count(r.volume)}{num(r.volume) == null ? "" : " 份"}
                    </dd>
                  </div>
                </dl>
                <button
                  type="button"
                  className="kalshi-detail-toggle"
                  aria-label={`${value(r.title_zh)}全部字段`}
                  aria-expanded={expanded === key}
                  onClick={() => setExpanded(expanded === key ? null : key)}
                >
                  明细 <ChevronDownIcon size={14} aria-hidden="true" />
                </button>
                {expanded === key && <Detail row={r} />}
              </article>
            )
          })}
        </div>
        {!visible.length && (
          <p className="kalshi-empty">没有符合筛选条件的线索。</p>
        )}
      </section>
      <section className="kalshi-panel" id="top10-trend">
        <h2>新上线 Top10 走势</h2>
        <p>
          每次扫描按事件成交量选出新上线合约前10名（成交量为0的不入选），上线后连续跟踪7天。线为价差不超过10个百分点的买卖中间价（每小时），浅色带为买价到卖价区间，圆点为实际成交价。买卖价差越宽，价格越不可靠。
        </p>
        <label className="kalshi-select-label">
          批次{" "}
          <select
            aria-label="入选日期"
            value={selectedCohort || ""}
            onChange={(e) => setCohort(e.target.value)}
          >
            {cohorts.map((c) => (
              <option key={c} value={c}>
                {c}（
                {
                  new Set(
                    trends
                      .filter((r) => r.cohort === c)
                      .map((r) => value(r.market_ticker))
                  ).size
                }
                个）
              </option>
            ))}
          </select>
        </label>
        {markets.length ? (
          <div className="kalshi-trend-grid">
            {markets.map((series) => (
              <TrendCard key={value(series[0]?.market_ticker)} rows={series} />
            ))}
          </div>
        ) : (
          <p className="kalshi-empty">这一批次暂无走势数据。</p>
        )}
      </section>
      <section className="kalshi-panel" id="by-category">
        <h2>线索按类别分布</h2>
        <p>
          {range === "all"
            ? `全部历史记录（${firstDate || "—"} 至 ${end}）`
            : `近30天（${from} 至 ${end}）`}
          每个类别的线索条数
        </p>
        <div className="kalshi-segment" role="group" aria-label="时间范围">
          <button
            type="button"
            aria-pressed={range === "30d"}
            onClick={() => setRange("30d")}
          >
            近30天
          </button>
          <button
            type="button"
            aria-pressed={range === "all"}
            onClick={() => setRange("all")}
          >
            全部
          </button>
        </div>
        {totals.length ? (
          <div
            className="kalshi-category-chart"
            role="img"
            aria-label="线索按类别分布"
          >
            {totals.map((r) => (
              <div className="kalshi-category-row" key={r.name}>
                <span title={r.name}>{r.name}</span>
                <div>
                  <i style={{ width: `${(r.total / max) * 100}%` }} />
                </div>
                <b>{count(r.total)}</b>
              </div>
            ))}
          </div>
        ) : (
          <p className="kalshi-empty">这个时间范围内没有线索。</p>
        )}
      </section>
      <details className="kalshi-method">
        <summary>口径与说明</summary>
        <dl>
          <dt>数据来源</dt>
          <dd>
            Kalshi
            公开行情接口，每日按类别和标签抓取最新上线及热门合约，与已记录线索对比去重。扫描截至{" "}
            {report.as_of}，状态：{report.manifest.collection_status}。
          </dd>
          <dt>新上线</dt>
          <dd>
            事件最早开盘时间在本次扫描窗口内，且不属于滚动刷新的短周期合约。
          </dd>
          <dt>价格异动</dt>
          <dd>
            同一代表盘口最近成交价与 previous_price_dollars
            相差至少10个百分点，事件累计成交至少1000份；接口前值周期未独立核实。
          </dd>
          <dt>观察</dt>
          <dd>
            存量事件或新增阈值，不视为新事件；近期成交价不能证明新闻属实。
          </dd>
          <dt>成交价</dt>
          <dd>
            prob
            为最近成交价，不是预测概率或买卖中点；无成交留空，宽价差会影响参考价值。
          </dd>
          <dt>Top10走势</dt>
          <dd>
            本轮新事件按累计成交量选前10，跟踪7天、保留30天。小时买卖报价带、中间价和成交点分别展示；缺失不补造，不延伸为实时报价。
          </dd>
          <dt>成交量</dt>
          <dd>
            信号表使用搜索接口 total_volume，代表盘口和小时成交分别使用
            volume_fp。不同层级不混加，份数不等于资金流入。
          </dd>
          <dt>相关标的、投资含义</dt>
          <dd>
            只标记标题或规则明确点名的公司及直接相关板块；未交叉核验的判断标为待核验。英文标题不等于已完成人工翻译。
          </dd>
          <dt>局限</dt>
          <dd>
            按配置的类别标签及有限搜索页发现，不保证全站完整。搜索、事件详情、K线分别记状态；部分失败时保留成功部分，具体缺口可下载原文核查。
          </dd>
        </dl>
      </details>
    </>
  )
}

export function KalshiPage({ data }: { data: PageData }) {
  const navigate = useNavigate()
  const report = data.report
  const today = calendarToday()
  const latest = data.dates[0]
  function changeDate(date: string) {
    if (!isCalendarDate(date) || date > today || date === data.date) return
    void navigate(`/kalshi?date=${encodeURIComponent(date)}`)
  }
  function changeSlot(slot: KalshiSlot) {
    if (slot === data.slot) return
    void navigate(`/kalshi?date=${encodeURIComponent(data.date)}&slot=${slot}`)
  }
  function download() {
    if (!report) return
    const url = URL.createObjectURL(
      new Blob([report.markdown], { type: "text/markdown;charset=utf-8" })
    )
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = `kalshi-${report.date}-${report.slot}.md`
    anchor.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return (
    <MainShell className="kalshi-main" containerClassName="kalshi-container">
      <div className="kalshi-page">
        <div className="kalshi-controls">
          <div className="kalshi-date-controls">
            <button
              type="button"
              className="kalshi-date-arrow"
              title="前一天"
              aria-label="前一天"
              onClick={() => changeDate(addCalendarDays(data.date, -1))}
            >
              <ChevronLeftIcon size={16} aria-hidden="true" />
            </button>
            <label className="kalshi-date-field">
              <CalendarDaysIcon size={16} aria-hidden="true" />
              <span className="sr-only">报告日期</span>
              <input
                type="date"
                value={data.date}
                max={today}
                list="kalshi-dates"
                aria-label="选择报告日期"
                onChange={(e) => changeDate(e.target.value)}
              />
            </label>
            <datalist id="kalshi-dates">
              {data.dates.map((date) => (
                <option key={date} value={date} />
              ))}
            </datalist>
            <button
              type="button"
              className="kalshi-date-arrow"
              disabled={data.date >= today}
              title="后一天"
              aria-label="后一天"
              onClick={() => changeDate(addCalendarDays(data.date, 1))}
            >
              <ChevronRightIcon size={16} aria-hidden="true" />
            </button>
            <button
              type="button"
              disabled={data.date === today}
              onClick={() => changeDate(today)}
            >
              今天
            </button>
            <button
              type="button"
              disabled={!latest}
              onClick={() => latest && void navigate(`/kalshi?date=${encodeURIComponent(latest)}`)}
            >
              最新
            </button>
            <div className="kalshi-slot-controls" role="group" aria-label="报告档位">
              {(["morning", "evening"] as const).map((slot) => (
                <button key={slot} type="button" aria-pressed={data.slot === slot}
                  title={data.snapshots.some((snapshot) => snapshot.slot === slot) ? undefined : "尚未发布"}
                  onClick={() => changeSlot(slot)}>
                  {slot === "morning" ? "早间" : "晚间"}
                </button>
              ))}
            </div>
          </div>
          <button
            type="button"
            disabled={!report}
            onClick={download}
            title="下载完整 Markdown"
          >
            <DownloadIcon size={16} /> 原文
          </button>
        </div>
        <nav className="kalshi-section-nav" aria-label="看板内容">
          <a href="#signals">市场线索</a>
          <a href="#top10-trend">走势跟踪</a>
          <a href="#by-category">类别分布</a>
        </nav>
        {data.error ? (
          <p className="kalshi-alert">报告暂时无法加载，请稍后重试。</p>
        ) : report ? (
          <Dashboard key={`${report.date}-${report.slot}`} report={report} />
        ) : (
          <p className="kalshi-empty">
            {data.dates.length
              ? `${data.date}${data.slot ? ` ${data.slot === "morning" ? "早间" : "晚间"}档` : ""} 暂无已发布的 Kalshi 报告。`
              : "尚无已发布的 Kalshi 报告。"}
          </p>
        )}
      </div>
    </MainShell>
  )
}
