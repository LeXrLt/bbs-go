# 每日复盘 Markdown 推送对接

本文面向生成每日复盘 Markdown 的定时任务或自动化 Codex。BBS 直接接收 Markdown
并写入 PostgreSQL，不依赖 `daily-review-dashboard`，也不需要六个板块同时完成。

生成 Markdown 的电脑不需要检出或运行 `bbs-go` 项目。`scripts/push_daily_review.py`
只是一个可独立下载的 API 客户端，只依赖 Python 3 标准库；BBS 服务端才需要部署
本仓库代码。

## 1. 推送模型

一条数据由以下两个字段唯一确定：

```text
(date, kind)
```

- `date` 是内容所属的复盘日期，不是实际推送时间，格式必须为 `YYYY-MM-DD`。
- `kind` 是板块类型，固定为下面六种之一。
- 每个 Markdown 文件抓取完成后即可立即单独推送。
- 同一天尚未完成的板块在页面显示为待更新，不影响已完成板块展示。
- 同一 `date + kind` 再次推送相同内容返回 `unchanged`。
- 同一 `date + kind` 推送新内容会更新当前版本并保留上一版本记录，返回 `updated`。

| `kind` | 页面板块 | 建议文件含义 |
| --- | --- | --- |
| `shareholding` | 港股增持 | 港股增持 Markdown |
| `announcements` | 公告筛选 | 公告筛选 Markdown |
| `movers` | 港股涨跌幅 | 港股涨跌幅 Markdown |
| `focus` | 重点公司 | 重点公司 Markdown |
| `buybacks` | 港股回购 | 港股回购 Markdown |
| `shorts` | 港股做空 | 港股做空 Markdown |

不要根据文件完成时间推导 `date`。例如 2026-10-28 的公告文件在
2026-10-29 00:15 才生成，仍应使用 `date=2026-10-28`。

## 2. 认证准备

1. 管理员在 BBS `/dashboard/users` 创建专用同步账户。
2. 在 `/dashboard/roles` 给该账户分配包含以下权限的角色：

   ```text
   dashboard.dailyReview.ingest
   ```

3. 人工登录一次获取用户 token。
4. 在运行定时任务的环境中把 token 注入为 `BBSGO_USER_TOKEN`。

token 不得写入仓库、Markdown、命令参数、日志或 Codex 提示词。请求只使用
`X-User-Token`，不要同时发送 Cookie、Bearer Token 或 `userToken` 查询参数。

生产 BBS 必须使用 HTTPS。本地 `localhost`、`127.0.0.1` 和 `::1` 可以使用 HTTP。

## 3. 推荐调用方式

### 3.1 在抓取电脑安装单文件客户端

抓取电脑无需下载完整项目。首次使用时，把经过固定 commit 版本化的脚本下载到本地：

```bash
install -d "$HOME/.local/bin"
curl -fsSL \
  'https://raw.githubusercontent.com/lugit123456/bbs-go/15428ce2b6b6f9312121f249db06885405473740/scripts/push_daily_review.py' \
  -o "$HOME/.local/bin/push_daily_review.py"
chmod 700 "$HOME/.local/bin/push_daily_review.py"
```

建议首次部署时由管理员核对下载脚本的内容和 commit，再交给定时任务调用。脚本不需要
安装第三方 Python package，也不需要访问 BBS 数据库。

### 3.2 每个 Markdown 完成后调用

每个文件生成成功后调用一次：

```bash
export BBS_BASE_URL='https://bbs.example.com'
export BBSGO_USER_TOKEN='由密钥管理服务注入，不要写死'

python3 "$HOME/.local/bin/push_daily_review.py" \
  --base-url "$BBS_BASE_URL" \
  --date '2026-10-28' \
  --kind announcements \
  --file '/absolute/path/to/announcements.md'
```

成功时脚本退出码为 `0`，标准输出示例：

```text
created: 2026-10-28 announcements
updated: 2026-10-28 announcements
unchanged: 2026-10-28 announcements
```

失败时退出码为 `1`，错误写入标准错误。调度器应记录日期、板块和退出码，但不得
记录 token 或完整请求头。

### 不同时间完成的六个任务

六个抓取任务可以各自在完成后执行自己的命令，不需要设置统一等待任务：

```bash
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind shareholding  --file "$SHAREHOLDING_MD"
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind announcements --file "$ANNOUNCEMENTS_MD"
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind movers        --file "$MOVERS_MD"
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind focus         --file "$FOCUS_MD"
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind buybacks      --file "$BUYBACKS_MD"
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind shorts        --file "$SHORTS_MD"
```

实际部署时只在对应文件已经完整写入且抓取任务成功后执行该行。建议抓取器先写临时文件，
完成后原子重命名为最终 `.md` 文件，再触发推送，避免上传半成品。

## 4. HTTP 接口

不使用仓库脚本时，可直接调用：

```http
POST /api/daily-review/ingest/{kind}?date=YYYY-MM-DD
Content-Type: application/json; charset=utf-8
Accept: application/json
X-User-Token: <token>

{"markdown":"# 公告筛选\n\n正文"}
```

限制：

- Markdown 必须是非空 UTF-8 文本。
- 单个 Markdown 最大 `1,000,000` 字节。
- `date` 必填，且必须严格使用 `YYYY-MM-DD`。
- `kind` 必须是本文列出的六个固定值。

成功响应：

```json
{
  "errorCode": 0,
  "message": "",
  "success": true,
  "data": {
    "result": "created",
    "kind": "announcements",
    "date": "2026-10-28"
  }
}
```

必须同时满足以下条件才算成功：

1. HTTP 状态码为 `2xx`。
2. JSON 可解析且 `success` 严格等于 `true`。
3. `data.result` 是 `created`、`updated` 或 `unchanged`。

BBS 的部分业务错误可能仍返回 HTTP `200`，因此不能只检查 HTTP 状态或命令退出码。

## 5. 重试和幂等

`date + kind + Markdown 内容` 使用 SHA-256 做幂等判断。以下情况可以使用完全相同的
日期、板块和文件有限重试：

- 连接失败或超时。
- HTTP `500` 至 `599`。
- 请求已发出，但调用方没有收到明确响应。

不要自动重试以下错误：

- HTTP `400`、`401`、`403`、`404`。
- JSON 中 `success=false`。
- 文件为空、不是 UTF-8、超过大小限制或日期/板块无效。

仓库脚本已经实现最多三次尝试，并只重试网络错误及 HTTP `5xx`。如果第三次仍失败，
由调度系统告警；修复原因后可以再次运行同一命令。

## 6. 给自动化 Codex 的执行约束

可以把下面内容作为另一个 Codex 任务的主体，并替换实际路径和站点地址：

```text
当指定 Markdown 文件完整生成后，把它推送到 BBS 每日复盘接口。

不需要 bbs-go 项目代码或数据库连接。
上传脚本：$HOME/.local/bin/push_daily_review.py
如果脚本不存在，从下面固定 commit 地址下载并设置为仅当前用户可执行：
https://raw.githubusercontent.com/lugit123456/bbs-go/15428ce2b6b6f9312121f249db06885405473740/scripts/push_daily_review.py
BBS 地址：从环境变量 BBS_BASE_URL 读取
认证：只从环境变量 BBSGO_USER_TOKEN 读取，不得打印、回显或写入文件
复盘日期：使用上游任务明确提供的 REPORT_DATE，格式 YYYY-MM-DD；不得用当前时间代替
板块：使用上游任务明确提供的 DAILY_REVIEW_KIND
文件：使用上游任务明确提供的绝对路径 MARKDOWN_FILE

执行前检查文件存在、非空且上游抓取任务成功。然后在任意目录运行：
python3 "$HOME/.local/bin/push_daily_review.py" --base-url "$BBS_BASE_URL" --date "$REPORT_DATE" --kind "$DAILY_REVIEW_KIND" --file "$MARKDOWN_FILE"

只有退出码为 0 才标记推送成功。记录脚本返回的 created、updated 或 unchanged，
但不要记录 token。失败时保留原 Markdown，报告日期、板块和错误信息，不要删除文件，
也不要更改为其他日期或板块重试。
```

上游任务必须明确传入以下三项，Codex 不应猜测：

```text
REPORT_DATE=2026-10-28
DAILY_REVIEW_KIND=announcements
MARKDOWN_FILE=/absolute/path/to/announcements.md
```

## 7. 上线后验证

推送完成后可登录 BBS 打开：

```text
/daily-review?date=2026-10-28
```

对应板块应由“待更新”变为“已更新”。六个板块可以分批出现，顶部日期选择统一控制
同一日期下的全部板块。

更完整的 BBS 脚本认证规则见 `docs/script-api.md`。
