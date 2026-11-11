# Kalshi 抓取脚本接入 BBS-Go 教程

本文件供维护“抓取数据并生成每日 Markdown”的脚本使用。目标是将**已完成复核的完整
Markdown** 推送至 BBS-Go 的独立 `/kalshi` 看板，同时继续在抓取服务器本地保存
原件和历史归档。BBS-Go 不执行文档内的 `index.html`，而是校验并读取其中的数据块。
接口的完整约定见 `docs/kalshi-ingest.md`；以下是生成端的实施顺序。

## 1. 先明确两种状态

| 字段 | 含义 | 生成端与 BBS-Go 的处理 |
| --- | --- | --- |
| `publication_ready=false` | 中文复核未完成，或生成端判定不可交付 | **不推送**；BBS-Go 也会拒绝 |
| `publication_ready=true`、`collection_status=complete` | 已复核且本次配置的采集范围内没有记录到缺口 | 可推送；页面不显示覆盖缺口提示 |
| `publication_ready=true`、`collection_status=partial` | 已复核，但有请求失败、上限省略或可能分页截断等缺口 | **可推送**；页面显示覆盖缺口提示，报告必须写明具体原因 |

`complete` 只表示本次**配置范围**内无已知缺口，不表示 Kalshi 全站穷尽。
搜索请求全部成功也不等于完整覆盖：2026-11-11 的本地报告中，121/121 次搜索
成功且请求失败为 0，但 `Financials/KPIs` 与 `Elections` 两项到达每项最多 20 页
的上限，后续仍可能有结果，因此 `collection_status=partial`。页面上的“本次采集
存在覆盖缺口”是基于 manifest 的这个字段显示的**数据质量提醒**，不是接口错误。

生成端应保留 `metadata.coverage` 中每项搜索的页码、状态和
`potential_truncation`，以及 `metadata.failures`、`new_candidates_omitted_by_cap`
等原始证据；在 Markdown 的“阅读摘要 / 运行结果与覆盖缺口”和“数据缺口与来源”
中给出可读解释。不要仅为了消除页面提示而将 `partial` 改成 `complete`。
如果改进分页或拆分搜索，应在确认未遗漏后重新生成整份文档，并按新快照推送；
不要手工改 manifest，因为文件字节数、哈希和内容须一致。

## 2. 生成并保存完整文件

1. 抓取结束后先生成 `signals`、`trends`、覆盖记录和阅读摘要，完成中文复核。
2. 在 Markdown 中提供**各恰好一个**带路径标记的代码块：
   `json manifest.json`、`json data/signals.json`、`json data/trends.json`、
   `html index.html`。注意这里总共是四个代码块，其中 manifest 单独作为清单。
3. manifest 的 `schema_version` 为 `kalshi-dashboard-handoff/2`；`date` 为
   `YYYY-MM-DD`，`as_of` 为对应北京时间日期的 RFC 3339 时间；
   `publication_ready` 为 `true` 才能提交，`collection_status` 只能为
   `complete` 或 `partial`，`metadata` 必须是有效 JSON。
4. `manifest.files` 对后三个数据/页面代码块逐个记录**代码块内部文本**的
   UTF-8 字节数和 SHA-256；`counts.signals_total`、`counts.trend_rows` 与
   两份 JSON 的 `rows` 数量一致。文件整体必须是非空 UTF-8，且不超过
   2,000,000 字节。不要在计算哈希之后修改换行或代码块内容。
5. 先完成本地文件写入并归档，再启动推送；不要读取仍在写入的文件。现有
   `daily_run.py` 在 `publication_ready=true` 时生成
   `产物/最新发布文档.md` 并归档批次文件。建议推送任务读取这份已完成的文件，
   并用原子写入/重命名避免并发读到半成品；生产服务器路径以实际配置为准。
   **上传成功不能删除或改写本地 MD 及其历史归档。**

推送的是整个 `.md` 的文本，不是摘要、JSON 片段或生成后的 HTML。BBS-Go
会保存原文并从页面顶部提供下载副本。

## 3. 配置账号与权限

由管理员给同步账号的角色授予 `dashboard.kalshi.ingest`。沿用人工登录取得的
用户 token；脚本只发 `X-User-Token`，不要同时发送 Cookie、Bearer Token 或
URL 参数 `userToken`，以免较高优先级的无效凭据覆盖它。token 只从服务器的
密钥环境注入 `BBSGO_USER_TOKEN`，不可放入 Markdown、代码仓库、命令参数、
日志或交接消息；不要为定时任务绕过验证码自动登录。有效期以站点配置为准。
生产地址使用 HTTPS；仅本机回环调试可使用 HTTP。

## 4. 推送流程

已有可复用的标准库客户端：`scripts/push_kalshi_report.py`。将此脚本部署在
抓取服务器，向运行环境安全注入 `BBSGO_USER_TOKEN` 后，在完成归档的任务后执行：

```bash
python3 scripts/push_kalshi_report.py \
  --base-url 'https://bbs.example.com' \
  --file '/抓取服务器上的绝对路径/产物/最新发布文档.md'
```

将示例域名和文件路径替换为实际值；不要把 token 写进命令。脚本执行以下步骤，
自行实现客户端时也须遵守：

1. 读取文件**原始字节**，校验大小和 UTF-8，从唯一的 `manifest.json` 代码块
   获取 `date`。不能用推送当天的系统日期替代它。
2. 以 `X-User-Token` 请求
   `GET /api/kalshi/status?date=YYYY-MM-DD`。响应为 BBS 的 JSON 包装对象，
   必须同时检查 HTTP 状态与 `success === true`。`data.published=true` 时，
   对比 `data.content_hash` 与**整个 MD 文件原始字节**的 SHA-256；相同则
   直接返回 `unchanged`。状态查询失败不可当作“尚未发布”。
3. 哈希不同时发送 `POST /api/kalshi/ingest`，请求头为
   `Content-Type: application/json; charset=utf-8` 和 `X-User-Token`，JSON
   请求体只有 `{"markdown":"<完整 Markdown 文本>"}`。核对响应的
   `success === true`、`data.date` 与 manifest 日期相同，且 `data.result`
   是 `created`、`updated`、`unchanged` 之一。
4. 网络故障或 HTTP 5xx 可以有限重试同一文件；HTTP 400（文档不合法）、
   HTTP 409（比已发布快照更旧）、业务失败或权限失败应非零退出并告警。
   BBS-Go 会保存同日旧版，较新的 `as_of` 可以更新同日报告；同一内容重复
   推送是幂等的。不要在日志中打印 token、完整请求或完整文档。

同步账号需要权限才能查询状态和推送；普通页面 `GET /api/kalshi/dates` 和
`GET /api/kalshi/report?date=YYYY-MM-DD` 依照站点的登录可见策略读取。

## 5. 上线核验

- 推送结果输出应为 `created: <date>`、`updated: <date>` 或
  `unchanged: <date>`；非零退出视为失败，不可只看 HTTP 200。
- 登录 BBS-Go，访问 `/kalshi?date=<manifest.date>`，确认扫描时间、线索数量、
  代表盘口指标、走势及原文下载。`partial` 时应看到缺口提醒，并与 MD 摘要的
  具体原因一致；不要把历史样例或缺失值伪装为真实行情。
- 确认抓取服务器仍保留原始 MD 与历史归档；BBS 的数据库和下载副本不能
  代替本地归档。

当前 BBS 页面仅按 `collection_status` 显示通用缺口提示，不会自动将
`metadata.coverage` 的每个原因写在提示里。若希望页面直接显示“哪两项搜索
可能截断”，还需要修改 BBS 看板前端；生成端应先保证原因在 MD 中准确可查。
