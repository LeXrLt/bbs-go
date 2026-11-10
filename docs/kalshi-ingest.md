# Kalshi Markdown 跨服务器推送对接

本文可直接交给生成 Markdown 的服务器上的 Codex，用于开发推送脚本和定时任务。BBS
接收完整 Markdown 并存入 PostgreSQL；生成服务器无需挂载 BBS 目录、连接数据库，
也无需检出整个 BBS 项目。必须先部署包含以下接口的 BBS 版本，旧版不会识别这些路径。

## 1. 数据与更新规则

页面地址为 `/kalshi`。每份文件由内嵌 `manifest.json` 的 `date` 确定所属日期，
而非推送当天的系统日期。每天可多次推送：同日同内容返回 `unchanged`；同日内容
改变返回 `updated`，旧版保存在 revision 表；首次发布返回 `created`。如果新文件的
`as_of` 早于该日已发布版本，服务端拒绝覆盖。同一时间戳内容变化可以更新，推送端
仍应保证来源明确，避免用旧文件覆盖新文件。

必须推送**整个** UTF-8 `.md` 文件，不能只抽取摘要、表格或两份 JSON。BBS
保留原文、manifest、signals、trends 和代码块；`index.html` 作为代码展示，
不在论坛中执行。`collection_status=partial` 且 `publication_ready=true` 可以发布，
页面保留缺口提示；`publication_ready=false` 不可发布。

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
GET /api/kalshi/status?date=YYYY-MM-DD
Accept: application/json
X-User-Token: <同一个同步账号的 token>
```

未发布的 `data`：

```json
{"date":"2026-11-09","published":false}
```

已发布的 `data`（示意，哈希须以实际响应为准）：

```json
{"date":"2026-11-09","published":true,"content_hash":"<64 位 SHA-256 小写十六进制>","as_of":"2026-11-09T19:01:55+08:00"}
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

成功时 `data` 的格式是 `{"result":"created|updated|unchanged","date":"YYYY-MM-DD"}`。
脚本还应核对响应日期与 manifest 日期相同，`result` 属于这三个固定值。以服务端
结果为准：即使查询状态后另一任务先上传，相同内容再次 POST 仍返回 `unchanged`。

文件 UTF-8 编码、大小须不超过 **2,000,000 字节**。manifest 的
`schema_version` 必须为 `kalshi-dashboard-handoff/2`，`publication_ready=true`，
`date` 合法，`as_of` 日期与 `date` 对应，`collection_status` 为 `complete` 或
`partial`。`data/signals.json`、`data/trends.json`、`index.html` 命名代码块各
恰有一个；字节数及 SHA-256 要与 manifest 匹配，JSON 行数也须匹配。不要为了
通过验证擅自改写源文件。无效文档返回 HTTP 400；较旧快照返回 HTTP 409；
未登录或无权限可能返回 HTTP 200 但 `success=false`。

供页面使用的 `GET /api/kalshi/dates`、`GET /api/kalshi/report?date=YYYY-MM-DD`
沿用站点读权限策略；推送脚本不必下载包含完整 Markdown 的 report 接口。

## 4. 给定时任务的实现要求

1. 在源文件生成任务**成功完成**后运行，建议先写临时文件再原子重命名，不读取
   正在写入的文件；文件路径由生产服务器配置，不使用开发机路径。
2. 读取文件字节、校验 UTF-8 和大小，解析唯一的 `manifest.json` 代码块拿到
   `date`；只使用文件自己的 `date`，不要用定时任务执行日期代替。
3. 查询状态；哈希一致就输出 `unchanged` 并退出 0；否则提交完整 Markdown。
   校验 HTTP 状态、JSON `success`、返回日期及结果类型，成功退出 0。
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

预期标准输出示例：`created: 2026-11-09`、`updated: 2026-11-09` 或
`unchanged: 2026-11-09`。不要在未获授权的生产站点运行测试上传。
