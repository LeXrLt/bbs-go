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

import type { KalshiRow } from "@/lib/api/kalshi"

const integer = new Intl.NumberFormat("zh-CN")
const num = (v: unknown) =>
  typeof v === "number" && Number.isFinite(v) ? v : null
const pct = (v: unknown) =>
  num(v) == null ? "—" : `${Math.round(num(v)! * 100)}%`

export default function TrendPlot({ points }: { points: KalshiRow[] }) {
  const first = points[0]
  const last = points[points.length - 1]
  if (!first || !last) return null
  const opened = Date.parse(String(first.opened))
  const firstTs = Date.parse(String(first.ts))
  const start = Number.isFinite(opened) ? Math.min(opened, firstTs) : firstTs
  const finish = Date.parse(String(last.ts))
  const data = points.map((p) => {
    const bid = num(p.bid),
      ask = num(p.ask)
    return {
      ts: Date.parse(String(p.ts)),
      band: bid != null && ask != null && bid <= ask ? [bid, ask] : null,
      middle:
        bid != null && ask != null && bid <= ask && ask - bid <= 0.100001
          ? (bid + ask) / 2
          : null,
      close: num(p.close),
      vol: num(p.vol),
    }
  })
  return (
    <ResponsiveContainer width="100%" height="100%" initialDimension={{ width: 286, height: 128 }}>
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
              `${price} · 小时成交量 ${volume == null ? "无数据" : `${integer.format(Math.round(volume))} 张`}`,
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
  )
}
