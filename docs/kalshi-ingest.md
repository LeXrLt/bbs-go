# Kalshi Markdown 跨服务器推送对接

本文可直接交给生成 Markdown 的服务器上的 Codex，用于开发推送脚本和定时任务。BBS
接收完整 Markdown 并存入 PostgreSQL；生成服务器无需挂载 BBS 目录、连接数据库，
也无需检出整个 BBS 项目。必须先部署包含以下接口的 BBS 版本，旧版不会识别这些路径。

## 1. 数据与更新规则

页面地址为 `/kalshi`，默认显示最新已发布报告；可按自然日切换、选“今天”或“最新”，
未发布日期会显示空状态。每份报告由内嵌 `manifest.json` 的 `date` 与
`metadata.run_slot`（`morning` / `evening`）共同标识，不能从时间或文件名推断。
同档首次发布返回 `created`，同文件返回 `unchanged`；不同文件且新 `as_of`
严格晚于同档当前版本才返回 `updated` 并归档旧版，否则返回 HTTP 409。
早晚档互不覆盖；未标档的旧记录保留但不冒充其中一档。

必须推送**整个** UTF-8 `.md` 文件，不能只抽取摘要、表格或两份 JSON。BBS
保留原文、manifest、signals、trends 和代码块；页面依据数据展示独立看板，
不在论坛中执行 `index.html`。完整 Markdown 可从页面顶部“原文”下载。
`collection_status=partial` 且 `publication_ready=true` 可以发布，
页面保留缺口提示；`publication_ready=false` 不可发布。

生成端仍须在本地保存 `.md` 原件及历史归档，上传成功后不得删除或改写源文件。
现有 `daily_run.py` 的 `KALSHI_ARCHIVE_DIR`（默认在部署包上一级的
`Kalshi合约` 目录）会保存 `存档/` 中的批次文件和 `最新发布文档.md`；
推送任务应读取已完成归档的文件。BBS 的 PostgreSQL 原文与页面下载是额外副本，
不是生成端本地归档的替代品。

## 2. 与每日复盘共用 token

沿用每日复盘的账号和用户 token，无需新的登录流程或单独的 API Key。管理员在
`/dashboard/roles` 给**已有同步账号所用角色**再授予：

```text
dashboard.kalshi.ingest
```

原有的 `dashboard.dailyReview.ingest` 不变；两项权限可以属于同一个角色，同一个
`X-User-Token` 可用于两类推送。修改角色权限后，先分别验证两类接口的访问权限。
token 有效期以站点配置为准，过期时由管理员重新人工登录获取；不要让定时任务
绕过验证码自动登录。仅从服务器的安全环境注入 `BBSGO_USER_TOKEN`，不得放入
Markdown、代码仓库、命令行参数、日志或 Codex 提示词。不要同时发送 Cookie、
Bearer Token 或 URL 上的 `userToken`：它们的认证优先级可能覆盖 `X-User-Token`。
生产环境只使用 HTTPS；本地回环地址调试可用 HTTP。

## 3. HTTP 协议

两个推送用接口均需登录且具备 `dashboard.kalshi.ingest` 权限。响应沿用 BBS 的
`{ "errorCode": 0, "message": "", "success": true, "data": ... }` 格式；
HTTP 200 **不代表**业务成功，必须检查 `success === true`。

### 查询发布状态（不下载原文）

```http
GET /api/kalshi/status?date=YYYY-MM-DD&slot=morning
Accept: application/json
X-User-Token: <同一个同步账号的 token>
```

未发布的 `data`：

```json
{"date":"2026-11-09","slot":"morning","published":false}
```

已发布的 `data`（示意，哈希须以实际响应为准）：

```json
{"date":"2026-11-09","slot":"morning","published":true,"content_hash":"<64 位 SHA-256 小写十六进制>","as_of":"2026-11-09T08:01:55+08:00"}
```

此接口不会读取或传输整个 Markdown。调用方把本地文件**原始 UTF-8 字节**计算
SHA-256，并与 `content_hash` 比较；一致就跳过 POST。状态查询失败时不得推断
为“尚未发布”，也不得因此盲目上传。

### 提交完整文档

```http
POST /api/kalshi/ingest
Accept: application/json
Content-Type: application/json; charset=utf-8
X-User-Token: <同一个同步账号的 token>

{"markdown":"<完整 Markdown 文件文本>"}
```

成功时 `data` 包含 `result`、`date`、`slot`、`content_hash`。脚本必须核对
日期、档位与整份原文哈希，再查询同档状态核对哈希。以服务端
结果为准：即使查询状态后另一任务先上传，相同内容再次 POST 仍返回 `unchanged`。

文件 UTF-8 编码、大小须不超过 **2,000,000 字节**。manifest 的
`schema_version` 必须为 `kalshi-dashboard-handoff/2`，`publication_ready=true`，
`date` 合法，`as_of` 日期与 `date` 对应，`metadata.run_slot` 为 `morning` 或
`evening`，`collection_status` 为 `complete` 或
`partial`。`data/signals.json`、`data/trends.json`、`index.html` 命名代码块各
恰有一个；字节数及 SHA-256 要与 manifest 匹配，JSON 行数也须匹配。不要为了
通过验证擅自改写源文件。无效文档返回 HTTP 400；较旧快照返回 HTTP 409；
未登录或无权限可能返回 HTTP 200 但 `success=false`。

页面使用 `GET /api/kalshi/dates`、`GET /api/kalshi/snapshots?date=YYYY-MM-DD`
和 `GET /api/kalshi/report?date=YYYY-MM-DD&slot=morning`，沿用站点读权限策略。
缺档的 report 返回 `published:false`；推送脚本不必下载完整 Markdown。
看板 report 只返回结构化字段，不含 Markdown 或渲染后的 HTML；需要原文时
使用 `GET /api/kalshi/download?date=YYYY-MM-DD&slot=morning` 按需下载。
下载接口沿用站点读权限策略，未发布档位返回 HTTP 404。

## 4. 给定时任务的实现要求

1. 在源文件生成任务**成功完成**后运行，建议先写临时文件再原子重命名，不读取
   正在写入的文件；文件路径由生产服务器配置，不使用开发机路径。
2. 读取文件字节、校验 UTF-8 和大小，解析唯一的 `manifest.json` 代码块拿到
   `date` 和 `metadata.run_slot`；不按执行时间或文件名猜测档位。
3. 查询状态；哈希一致就输出 `unchanged` 并退出 0；否则提交完整 Markdown。
   校验 HTTP 状态、JSON `success`、返回日期、档位及哈希，重新查询同档状态后退出 0。
4. 网络故障或 HTTP 5xx 可对同一文件有限重试；4xx、业务失败、校验失败须
   非零退出并报警。响应丢失后重试同一文件安全，服务端按哈希幂等。保留源文件
   便于排查，不打印 token、完整请求、完整原文。
5. 建议调度器记录日期、文件路径、结果及退出码，防止任务重叠运行。每天多次
   更新可以多次执行相同脚本，不需要每天只发布一次。

仓库提供可参考的 Python 标准库客户端 `scripts/push_kalshi_report.py`，和每日
复盘脚本一样读取 `BBSGO_USER_TOKEN`；另一台服务器可以自行实现等价脚本，
无需把 BBS 代码或数据库搬过去。本地手动验证命令：

```bash
python3 scripts/push_kalshi_report.py \
  --base-url 'https://bbs.example.com' \
  --file '/服务器上的绝对路径/最新发布文档.md'
```

预期标准输出示例：`created: 2026-11-09 morning`、`updated: 2026-11-09 evening`。
不要在未获授权的生产站点运行测试上传。
