# bbs-go 脚本自动化接口文档

本文档面向站点维护者编写自动化脚本，内容依据当前仓库中的路由、请求结构、
处理器和前端实际调用整理。示例中的 `https://bbs.example.com`、分类 ID、帖子 ID
和 token 均需替换为目标站点的实际值。

> 自动化程序应遵守站点规则。验证码、邮箱验证、观察期和禁言等限制属于服务端安全策略，不应绕过。

## 1. 接口概览

| 功能 | 方法 | 路径 | 登录 |
| --- | --- | --- | --- |
| 获取公开配置 | `GET` | `/api/config/configs` | 否 |
| 获取验证码 | `GET` | `/api/captcha/request` | 否 |
| 密码登录 | `POST` | `/api/login/signin` | 否 |
| 验证当前 token | `GET` | `/api/user/current` | 可选 |
| 获取发帖分类 | `GET` | `/api/topic/categories` | 按站点配置 |
| 创建帖子 | `POST` | `/api/topic/create` | 是 |
| 获取帖子详情 | `GET` | `/api/topic/{topicId}` | 按站点配置 |
| 获取帖子列表 | `GET` | `/api/topic/topics` | 按站点配置 |
| 获取/提交帖子编辑数据 | `GET` / `POST` | `/api/topic/edit/{topicId}` | 是 |
| 删除帖子 | `POST` | `/api/topic/delete/{topicId}` | 是 |
| 上传正文或评论图片 | `POST` | `/api/upload` | 是 |
| 上传帖子附件 | `POST` | `/api/attachment/upload` | 是 |
| 获取附件访问权 | `POST` | `/api/attachment/access/{attachmentId}` | 是 |
| 在线预览附件 | `GET` / `HEAD` | `/api/attachment/preview/{attachmentId}` | 是 |
| 下载附件原件 | `GET` / `HEAD` | `/api/attachment/download/{attachmentId}` | 是 |
| 获取一级评论 | `GET` | `/api/comment/comments` | 按站点配置 |
| 获取二级回复 | `GET` | `/api/comment/replies` | 按站点配置 |
| 创建评论或回复 | `POST` | `/api/comment/create` | 是 |
| 删除评论或回复 | `POST` | `/api/comment/delete/{commentId}` | 是 |
| 退出并注销 token | `GET` | `/api/login/signout` | 可选 |

基础地址约定：

```bash
export BBS_BASE_URL='https://bbs.example.com'
# BBS_TOKEN 由第 4 节的登录流程或运行环境中的密钥管理服务提供。
```

本文的命令行示例使用 `curl`；第 4 节的一次性登录流程还需要 `jq` 和 GNU `base64`。Python 示例要求 Python 3.10 或更高版本。

路径均相对于 `BBS_BASE_URL`，例如：

```text
https://bbs.example.com/api/topic/create
```

## 2. 通用协议

### 2.1 认证

脚本推荐只发送 `X-User-Token`：

```http
X-User-Token: <token>
```

也支持标准 Bearer 写法。`Bearer` 的大小写必须保持如下，并在其后保留一个空格：

```http
Authorization: Bearer <token>
```

服务端查找凭据的优先级是：

1. 查询参数或表单字段 `userToken`
2. Cookie `bbsgo_token`
3. `Authorization` Header
4. `X-User-Token` Header

不要在 URL 中传 `userToken`，否则 token 容易进入访问日志。也不要混用多种认证方式：
失效的高优先级 Cookie 或 Header 会覆盖后面的有效 token。使用
`requests.Session` 等会保存 Cookie 的客户端时尤其需要注意。

token 是服务端持久化的不透明字符串，不是 JWT。有效期由完整配置中的
`tokenExpireDays` 决定；代码回退值是 7 天，新安装站点的初始配置通常是 365 天，
应以目标站点返回值为准。

当前配置还支持 `loginRequired`。启用时，除了安装、登录、验证码、邮箱验证、
公开配置和当前用户等白名单接口，其他 `/api` 请求即使原本支持匿名读取，
也必须携带有效 token。脚本取得 token 后，最稳妥的做法是给后续所有 API 请求
统一添加认证 Header。

### 2.2 统一响应

普通成功响应：

```json
{
  "errorCode": 0,
  "message": "",
  "data": {},
  "success": true
}
```

普通失败响应：

```json
{
  "errorCode": 1000,
  "message": "验证码错误",
  "data": null,
  "success": false
}
```

业务成功和业务失败通常都会返回 HTTP `200`。脚本必须先检查 HTTP 状态，再检查 JSON 中的 `success`。不能只检查 `errorCode`，因为普通参数错误可能是：

```json
{
  "errorCode": 0,
  "message": "参数错误说明",
  "data": null,
  "success": false
}
```

### 2.3 ID 类型

| 对象 | API 中的常见类型 | 使用规则 |
| --- | --- | --- |
| 帖子 ID | 不透明字符串 | 原样使用创建、列表或详情响应中的 `topic.id` |
| 用户 ID | 不透明字符串 | 原样使用响应值 |
| 分类 ID | 正整数 | 使用分类接口返回的 `category.id` |
| 评论/回复 ID | 正整数 | 使用评论响应中的 `comment.id` |
| 附件 ID | UUID 字符串 | 使用附件上传响应中的 `id` |

帖子 ID 不应由脚本自行编码或解码。评论响应中的 `entityId` 是服务端内部整数，即使该评论属于帖子，也不要用它替代原始的外部帖子 ID。

### 2.4 游标分页与时间

列表接口统一在 `data` 中返回：

```json
{
  "results": [],
  "cursor": "下一页游标",
  "hasMore": false
}
```

当 `hasMore` 为 `true` 时，把 `cursor` 原样放到下一次同接口请求中。游标虽然经常只包含数字，但其含义会随接口和排序方式变化，脚本应将它视为不透明字符串。

空列表在部分处理路径中会序列化为 `results:null`，而不是 `results:[]`。
客户端应使用 `results = data.get("results") or []` 一类逻辑把两者统一为空数组。

`createTime`、`lastCommentTime`、`expiredAt` 等时间字段使用 Unix 毫秒时间戳。

## 3. 调用前探测

### 3.1 获取公开配置

```http
GET /api/config/configs
```

```bash
# 登录前可匿名读取登录所需的最小配置
curl -sS "$BBS_BASE_URL/api/config/configs"

# 登录后携带 token 可读取完整公开配置
curl -sS \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/config/configs"
```

当 `loginRequired=true` 且请求未登录时，只有站点标题、安全可公开的站点 Logo、
`loginRequired`、登录方式配置、`installed` 和 `language` 是有效的公开配置；
其他字段即使仍出现在 JSON 中，也会被清空或置为零值。不要把匿名响应中的
`topicCaptcha=false`、`tokenExpireDays=0` 等零值当成真实站点设置，取得 token 后
必须重新请求。

脚本至少应关注以下 `data` 字段：

| 字段 | 说明 |
| --- | --- |
| `installed` | 站点是否已经安装 |
| `loginRequired` | 是否强制登录后访问内容 API |
| `tokenExpireDays` | token 服务端有效天数 |
| `topicCaptcha` | 发帖是否需要验证码 |
| `createTopicEmailVerified` | 发帖是否要求已验证邮箱 |
| `createCommentEmailVerified` | 评论是否要求已验证邮箱 |
| `userObserveSeconds` | 新用户观察期秒数 |
| `modules.topic` | 普通帖子模块是否开启 |
| `modules.tweet` | 动态模块是否开启 |
| `modules.qa` | 问答模块是否开启 |
| `enableHideContent` | 回复后可见内容是否开启 |
| `enableQaBounty` | 问答悬赏是否开启 |
| `qaBountyMin` / `qaBountyMax` | 悬赏积分范围 |
| `qaBountyRequired` | 问答是否必须设置悬赏 |
| `attachmentConfig` | 附件开关、扩展名、大小和数量配置 |
| `loginConfig.passwordLogin.enabled` | 普通用户密码登录是否开启 |

站点未安装时，其他 API 会返回 `success:false,errorCode:-1`。

### 3.2 获取可用分类

```http
GET /api/topic/categories?type={topicType}
```

`type` 可省略；指定时取值为：

| 值 | 类型 |
| ---: | --- |
| `0` | 普通帖子 |
| `1` | 动态 |
| `2` | 问答 |

```bash
curl -sS \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/topic/categories?type=0"
```

成功响应的 `data` 是分类树：

```json
[
  {
    "id": 1,
    "parentId": 0,
    "name": "技术交流",
    "type": "normal",
    "logo": "/res/images/category_default.svg",
    "description": ""
  }
]
```

创建帖子时必须显式传入目标站点返回的有效正整数 `categoryId`。当前实现中的
默认分类回退只修改了校验函数的局部副本，不能可靠地写入最终帖子，
因此不要省略或传 `0`。

### 3.3 写操作的账户前置条件

创建帖子、评论、图片以及多数编辑或删除操作会统一检查当前用户状态。调用者必须已经登录、账号状态正常、未被禁言且已经结束新用户观察期。站点配置开启相应开关时，发帖还要求邮箱已验证，评论也可单独要求邮箱已验证。

这些检查失败时通常返回第 9 节中的 `1`、`1001`、`1002`、`1003` 或 `1004`。失败后应停止当前发布任务，不要用高频重试规避限制。

## 4. 登录与 token

### 4.1 获取字符验证码

密码登录始终要求验证码。适合命令行人工完成一次登录的旧字符验证码接口为：

```http
GET /api/captcha/request
```

```json
{
  "errorCode": 0,
  "message": "",
  "data": {
    "captchaId": "验证码ID",
    "captchaBase64": "不含 data:image/png;base64, 前缀的PNG Base64"
  },
  "success": true
}
```

Linux 命令行示例：

```bash
export BBS_TMP_DIR="$(mktemp -d)"
chmod 700 "$BBS_TMP_DIR"
trap 'rm -rf -- "$BBS_TMP_DIR"' EXIT

curl -sS "$BBS_BASE_URL/api/captcha/request" > "$BBS_TMP_DIR/captcha.json"
jq -r '.data.captchaBase64' "$BBS_TMP_DIR/captcha.json" \
  | base64 --decode > "$BBS_TMP_DIR/captcha.png"
export BBS_CAPTCHA_ID="$(jq -r '.data.captchaId' "$BBS_TMP_DIR/captcha.json")"
```

打开 `$BBS_TMP_DIR/captcha.png`，人工读取其中的 4 个字符。登录时设置
`captchaProtocol=0`。临时目录权限为 `0700`，退出当前 Shell 时会自动清理。

站点前端当前使用旋转验证码：

```http
GET /api/captcha/request_angle
```

其 `data` 包含 `id`、`imageBase64`、`thumbBase64` 和 `thumbSize`，提交时使用
`captchaProtocol=2`，并将人工旋转得到的角度作为 `captchaCode`。
自动化程序不应伪造或绕过验证码。

### 4.2 密码登录

```http
POST /api/login/signin
Content-Type: application/x-www-form-urlencoded
```

| 字段 | 必填 | 说明 |
| --- | ---: | --- |
| `username` | 是 | 用户名或邮箱 |
| `password` | 是 | 密码 |
| `captchaId` | 是 | 验证码 ID |
| `captchaCode` | 是 | 字符验证码或旋转角度 |
| `captchaProtocol` | 是 | `2` 为旋转验证码，其他值走字符验证码 |
| `redirect` | 否 | 原样返回给客户端的跳转地址 |

```bash
export BBS_USERNAME='alice'
read -r -s -p '密码: ' BBS_PASSWORD
printf '\n'
read -r -p '验证码: ' BBS_CAPTCHA_CODE

printf '%s' "$BBS_PASSWORD" | curl -sS -X POST \
  "$BBS_BASE_URL/api/login/signin" \
  --data-urlencode "username=$BBS_USERNAME" \
  --data-urlencode 'password@-' \
  --data-urlencode "captchaId=$BBS_CAPTCHA_ID" \
  --data-urlencode "captchaCode=$BBS_CAPTCHA_CODE" \
  --data-urlencode 'captchaProtocol=0' > "$BBS_TMP_DIR/login.json"
unset BBS_PASSWORD BBS_CAPTCHA_CODE

jq '{success, errorCode, message, user: (.data.user // null)}' \
  "$BBS_TMP_DIR/login.json"
BBS_TOKEN="$(jq -r 'if .success then .data.token else empty end' \
  "$BBS_TMP_DIR/login.json")"
export BBS_TOKEN

rm -rf -- "$BBS_TMP_DIR"
trap - EXIT
unset BBS_TMP_DIR BBS_CAPTCHA_ID
```

密码通过标准输入交给 `curl`，不会出现在命令参数中；登录响应位于权限受限的临时目录，
且展示响应时会排除 token。继续操作前确认 `BBS_TOKEN` 非空。

成功响应的关键数据：

```json
{
  "success": true,
  "errorCode": 0,
  "message": "",
  "data": {
    "token": "32位不透明token",
    "redirect": "",
    "user": {
      "id": "编码后的用户ID",
      "username": "alice",
      "nickname": "Alice",
      "emailVerified": true
    }
  }
}
```

登录响应还会设置 HttpOnly Cookie `bbsgo_token`。脚本直接保存 `data.token` 即可，且必须像密码一样保管，不要提交到仓库或写入公开日志。

### 4.3 验证 token

```http
GET /api/user/current
X-User-Token: <token>
```

```bash
curl -sS \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/user/current"
```

token 有效时 `data` 为当前用户资料。无 token、token 无效或 token 过期时，
该接口仍返回 `success:true`，但 `data` 为 `null`，因此应同时检查这两个条件。

### 4.4 注销 token

```http
GET /api/login/signout
X-User-Token: <token>
```

该接口会把数据库中的当前 token 标记为删除，并发送 Cookie 删除指令。即使请求中
没有有效 token，也可能返回 `success:true`，因此成功响应只表示注销处理没有报错，
不代表请求头中的 token 已即时失效。它使用 `GET` 而不是 `POST`，脚本应按现有路由
调用。

> **当前实现风险：** 当前版本没有同步失效进程内的 `UserTokenCache`。已经进入缓存
> 的 `X-User-Token` 在注销成功后仍可能继续通过认证；缓存按访问续期，不能把残留
> 有效期简单理解为固定 60 分钟。脚本必须立即从本地存储和后续请求头中删除 token，
> 不要继续调用 `/api/user/current` 轮询失效，因为访问本身会延长缓存存活时间。需要
> 即时、可靠的服务端撤销时，当前版本的注销接口无法提供该保证。

## 5. 帖子接口

### 5.1 创建帖子

```http
POST /api/topic/create
Content-Type: application/json
X-User-Token: <token>
```

这是 JSON 专用接口，不要用表单提交。

最小可靠请求：

```bash
curl -sS -X POST "$BBS_BASE_URL/api/topic/create" \
  -H "X-User-Token: $BBS_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @- <<'JSON'
{
  "type": 0,
  "categoryId": 1,
  "title": "脚本发布的帖子",
  "contentType": "markdown",
  "content": "## 正文\n\n这是 **Markdown** 内容。",
  "tags": ["自动化"]
}
JSON
```

字段说明：

| 字段 | 必填 | 类型 | 规则 |
| --- | ---: | --- | --- |
| `type` | 是 | number | `0` 普通帖子、`1` 动态、`2` 问答 |
| `categoryId` | 是 | number | 有效正整数，且分类类型必须支持当前帖子类型 |
| `title` | 条件 | string | 普通帖子和问答必填，最多 128 个 Unicode 字符；动态可为空 |
| `content` | 是 | string | 所有类型均必填；服务端会去除首尾空白 |
| `contentType` | 是 | string | 推荐 `markdown`；还支持 `html`、`text`；动态强制为 `text` |
| `tags` | 否 | string[] | 标签名数组；不要发送空字符串标签 |
| `hideContent` | 否 | string | 回复后可见内容，仅在站点开启该功能时使用；问答会忽略并清空 |
| `imageList` | 否 | object[] | `[{"url":"..."}]`，主要用于动态图片 |
| `vote` | 否 | object/null | 投票配置；问答会忽略 |
| `bountyScore` | 否 | number | 仅在问答悬赏开启时使用；关闭时必须省略或传 `0` |
| `attachmentIds` | 否 | string[] | 普通帖子附件 UUID；必须先由附件接口上传 |
| `captchaId` | 条件 | string | `topicCaptcha=true` 时必填 |
| `captchaCode` | 条件 | string | `topicCaptcha=true` 时必填 |
| `captchaProtocol` | 条件 | number | `0` 为字符验证码，`2` 为旋转验证码 |

不要发送 `ip` 或 `userAgent`，处理器会用实际请求信息覆盖它们。

> **悬赏关闭时的当前实现风险：** 参数校验对 `bountyScore` 的清零只作用于局部副本，
> 后续发布仍可能使用调用方传入的正值并扣除积分。因此当 `enableQaBounty=false` 时，
> 脚本必须省略该字段或显式传 `0`，不能依赖服务端替调用方清零。

`vote` 对象结构：

```json
{
  "type": 1,
  "title": "选择一个选项",
  "expiredAt": 1893456000000,
  "voteNum": 1,
  "options": [
    {"content": "选项 A"},
    {"content": "选项 B"}
  ]
}
```

投票规则：`type=1` 为单选，`type=2` 为多选；标题最多 128 字；选项数为 2 至 20，
每项最多 256 字；`expiredAt` 必须是未来的毫秒时间戳；多选时 `voteNum` 必须在
`1..选项数` 内。

开启发帖验证码时，可复用第 4.1 节的验证码请求流程，并把验证码字段放进创建帖子的 JSON。验证码挑战应在提交前即时获取。

成功响应中的 `data` 是帖子摘要，关键字段如下：

```json
{
  "success": true,
  "errorCode": 0,
  "message": "",
  "data": {
    "id": "不透明帖子ID",
    "type": 0,
    "title": "脚本发布的帖子",
    "summary": "正文摘要",
    "content": "",
    "status": 0,
    "createTime": 1780000000000
  }
}
```

普通帖子创建响应不返回原始正文，只返回摘要；后续操作应保存 `data.id`。
`status=0` 表示正常，内容命中违禁词时接口仍可能创建成功，但 `status=2`
表示待审核。

### 5.2 获取帖子详情

```http
GET /api/topic/{topicId}
```

```bash
curl -sS \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/topic/$TOPIC_ID"
```

对 Markdown 帖子，详情响应中的 `data.content` 是服务端转换后的 HTML，不是创建时的原始 Markdown。该接口每调用一次都会增加浏览量，不要把它作为高频轮询接口。

待审核帖子仅作者本人或站长可查看；此时请求需要携带 token。

### 5.3 获取帖子列表

```http
GET /api/topic/topics
```

| Query 参数 | 必填 | 说明 |
| --- | ---: | --- |
| `categoryId` | 否 | `0` 全部、`-1` 推荐、`-2` 关注流、正数为分类 |
| `cursor` | 否 | 上一页返回的游标 |
| `qaStatus` | 否 | `unsolved` 或 `solved`；设置后只查问答 |
| `sort` | 否 | `latestPublish` 或 `latestReply`；默认按最新回复 |

```bash
curl -sS \
  -H "X-User-Token: $BBS_TOKEN" \
  --get "$BBS_BASE_URL/api/topic/topics" \
  --data-urlencode 'categoryId=1' \
  --data-urlencode 'sort=latestPublish'
```

`categoryId=-2` 的关注流必须登录。普通分页每页最多 30 条，但第一页可能额外包含置顶帖子，脚本不应依赖固定结果数量。

### 5.4 编辑帖子

先读取原始编辑数据：

```http
GET /api/topic/edit/{topicId}
X-User-Token: <token>
```

该响应包含原始 `content`、`contentType`、`categoryId`、`title`、`hideContent`、`tags` 和附件信息。动态不支持这个编辑表单接口。

提交编辑：

```http
POST /api/topic/edit/{topicId}
Content-Type: application/json
X-User-Token: <token>
```

```json
{
  "categoryId": 1,
  "title": "更新后的标题",
  "content": "更新后的正文",
  "hideContent": "",
  "tags": ["自动化", "更新"],
  "attachmentIds": []
}
```

这是全量更新语义，不是 JSON Merge Patch：应先读取编辑数据、修改目标字段，
再提交完整内容。省略 `tags` 会清空标签；`attachmentIds` 省略时保持原附件，
显式传 `[]` 时清空附件。接口不支持修改 `contentType`。

只有帖子作者或站长可编辑。标题必填且最多 128 字，分类必须存在并支持原帖子类型。

### 5.5 删除帖子

```http
POST /api/topic/delete/{topicId}
X-User-Token: <token>
```

```bash
curl -sS -X POST \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/topic/delete/$TOPIC_ID"
```

只有帖子作者或站长可删除。删除为软删除；目标已不存在或已删除时仍返回成功。

## 6. 评论与回复

### 6.1 创建一级评论

```http
POST /api/comment/create
Content-Type: application/x-www-form-urlencoded
X-User-Token: <token>
```

评论接口按现有前端调用使用表单，不要把 `imageList` 直接作为 JSON 数组提交。

| 字段 | 必填 | 说明 |
| --- | ---: | --- |
| `entityType` | 是 | 帖子评论为 `topic`，文章评论为 `article`，二级回复为 `comment` |
| `entityId` | 是 | 一级帖子评论传外部帖子 ID；二级回复传一级评论 ID |
| `content` | 是 | 评论原文；服务端会去除首尾空白 |
| `contentType` | 否 | `text` 或 `markdown`；缺省为 `text`，其他值会被拒绝 |
| `imageList` | 否 | JSON 字符串，例如 `[{"url":"https://..."}]` |
| `quoteId` | 否 | 回复某条二级回复时传被引用的评论 ID |

```bash
curl -sS -X POST "$BBS_BASE_URL/api/comment/create" \
  -H "X-User-Token: $BBS_TOKEN" \
  --data-urlencode 'entityType=topic' \
  --data-urlencode "entityId=$TOPIC_ID" \
  --data-urlencode 'contentType=markdown' \
  --data-urlencode 'content=这是脚本发布的 **Markdown** 评论'
```

成功响应关键字段：

```json
{
  "success": true,
  "errorCode": 0,
  "message": "",
  "data": {
    "id": 123,
    "entityType": "topic",
    "entityId": 456,
    "contentType": "markdown",
    "content": "<p>这是脚本发布的 <strong>Markdown</strong> 评论</p>",
    "imageList": [],
    "commentCount": 0,
    "quoteId": 0,
    "createTime": 1780000000000
  }
}
```

保存 `data.id`，它是后续回复、查询回复和删除所需的十进制评论 ID。

数据库保存评论原文和 `contentType`。创建、列表及回复查询响应中的 Markdown
`content` 是服务端转换并清洗后的 HTML，不是原始 Markdown。省略 `contentType`
的旧脚本仍按纯文本处理，已有纯文本评论也不会改变显示语义。

当前服务只校验 `entityType` 非空和 `entityId` 可解码为正数，并未完整校验目标
是否真实存在。脚本应只使用查询接口返回的真实对象 ID，并将 `entityType`
限制为上述预期值，避免产生无法关联的评论数据。

### 6.2 回复一级评论

把一级评论 ID 作为 `entityId`，并把 `entityType` 设为 `comment`：

```bash
curl -sS -X POST "$BBS_BASE_URL/api/comment/create" \
  -H "X-User-Token: $BBS_TOKEN" \
  --data-urlencode 'entityType=comment' \
  --data-urlencode "entityId=$PARENT_COMMENT_ID" \
  --data-urlencode 'contentType=markdown' \
  --data-urlencode 'content=这是对一级评论的回复'
```

回复同一楼层中的某条二级回复时，`entityId` 仍然是一级评论 ID，并额外传 `quoteId`：

```bash
curl -sS -X POST "$BBS_BASE_URL/api/comment/create" \
  -H "X-User-Token: $BBS_TOKEN" \
  --data-urlencode 'entityType=comment' \
  --data-urlencode "entityId=$PARENT_COMMENT_ID" \
  --data-urlencode "quoteId=$QUOTED_REPLY_ID" \
  --data-urlencode 'contentType=markdown' \
  --data-urlencode 'content=这是引用回复'
```

### 6.3 获取一级评论

```http
GET /api/comment/comments?entityType=topic&entityId={topicId}&cursor={cursor}
```

```bash
curl -sS --get "$BBS_BASE_URL/api/comment/comments" \
  -H "X-User-Token: $BBS_TOKEN" \
  --data-urlencode 'entityType=topic' \
  --data-urlencode "entityId=$TOPIC_ID"
```

每页最多 20 条，按评论 ID 倒序。问答帖已采纳的答案会在第一页置顶，并占用其中一个位置。每条一级评论最多内嵌最早的 3 条二级回复；完整回复应调用下一节接口。

### 6.4 获取二级回复

```http
GET /api/comment/replies?commentId={parentCommentId}&cursor={cursor}
```

```bash
curl -sS --get "$BBS_BASE_URL/api/comment/replies" \
  -H "X-User-Token: $BBS_TOKEN" \
  --data-urlencode "commentId=$PARENT_COMMENT_ID"
```

每页最多 10 条，按回复 ID 正序。继续翻页时使用响应中的 `data.cursor`。

### 6.5 删除评论或回复

```http
POST /api/comment/delete/{commentId}
X-User-Token: <token>
```

```bash
curl -sS -X POST \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/comment/delete/$COMMENT_ID"
```

评论作者可删除自己的评论；具有 `dashboard.comment.delete` 权限的管理用户也可删除。仅仅是所在帖子的作者，并不能删除其他用户的评论。删除为软删除，不会级联删除其二级回复。当前删除逻辑也不会同步减少帖子、父评论或用户的累计评论数，脚本不应根据删除响应自行假定计数已经变化。

### 6.6 采纳问答答案

问答帖作者或站长可采纳该帖下的一条一级评论：

```http
POST /api/topic/accept_answer/{topicId}
Content-Type: application/x-www-form-urlencoded
X-User-Token: <token>
```

```bash
curl -sS -X POST "$BBS_BASE_URL/api/topic/accept_answer/$TOPIC_ID" \
  -H "X-User-Token: $BBS_TOKEN" \
  --data-urlencode "commentId=$COMMENT_ID"
```

取消采纳：

```http
POST /api/topic/unaccept_answer/{topicId}
X-User-Token: <token>
```

> **这两个接口当前不是可安全重放的状态操作。** 重复调用采纳接口会再次向答案作者
> 发放悬赏积分；取消采纳不会追回已经发放的积分，取消后重新采纳也会再次发放。
> 自动化脚本必须先检查帖子的 `acceptedCommentId` 和 `qaStatus`，只提交一次，
> 且绝不能对采纳或取消采纳请求做自动重试。存在悬赏时，取消采纳不具备积分回滚语义。

## 7. 图片与附件

### 7.1 上传正文或评论图片

```http
POST /api/upload
Content-Type: multipart/form-data
X-User-Token: <token>
```

文件字段名必须是 `image`：

```bash
curl -sS -X POST "$BBS_BASE_URL/api/upload" \
  -H "X-User-Token: $BBS_TOKEN" \
  -F 'image=@/path/to/image.png'
```

成功数据：

```json
{
  "success": true,
  "errorCode": 0,
  "message": "",
  "data": {
    "url": "https://cdn.example.com/path/to/image.png"
  }
}
```

普通 Markdown 帖子可把 URL 写入正文：

```markdown
![说明](https://cdn.example.com/path/to/image.png)
```

动态或评论图片使用 `imageList`。评论接口中的 `imageList` 是 JSON 字符串，
而帖子创建接口中的 `imageList` 是真正的 JSON 数组，这是两个接口的重要差异。

当 `loginRequired=true` 且上传使用本地存储时，`/res/uploads/...` 下的图片资源
也要求有效登录。脚本直接下载这类 URL 时需要携带 token；浏览器页面通常通过
登录 Cookie 访问。外部对象存储或 CDN URL 是否需要额外鉴权由对应存储配置决定。

### 7.2 上传帖子附件

仅普通帖子支持附件，并且公开配置中的 `attachmentConfig.enabled` 必须为 `true`。
上传接口接受的扩展名、单文件大小和每帖数量以 `attachmentConfig` 为准；单文件
默认及最高上限均为 `256MB`。默认配置允许文档、纯文本和常见压缩包，其中以下格式
支持站内在线预览：

| 类型 | 扩展名 | 预览处理 |
| --- | --- | --- |
| PDF | `.pdf` | 校验后直接预览；仅含权限限制且可用空口令打开时生成规范化预览 |
| Word | `.doc`、`.docx` | 上传时同步转换为 PDF |
| Excel | `.xls`、`.xlsx` | 上传时生成 PDF 兼容预览；站内页面读取原件并显示为可滚动工作簿 |
| PowerPoint | `.ppt`、`.pptx` | 上传时同步转换为 PDF |

扩展名匹配不代表文件会被信任。服务端会核对 PDF 签名、旧版 Office OLE 头，
以及 OOXML ZIP 结构和实际文档类型；空文件、伪造扩展名、异常压缩包、加密的
Office 文件，以及需要输入打开密码的 PDF 会被拒绝。仅设置打印、复制或修改权限、
无需输入密码即可打开的 PDF 会保留原文件用于下载，并生成无加密的独立在线预览。
宏专用扩展名以及检测到 VBA 项目的 OOXML 文件同样会被拒绝；
`.docm`、`.xlsm`、`.pptm`、WPS、OpenDocument 等不属于本期预览格式。站点配置允许
的其他附件仍可下载，但响应中的 `previewable` 为 `false`。

OOXML 校验会流式统计 ZIP 条目，解压后总量最高允许 `1GiB`，超过时会作为异常压缩包
拒绝；该限制用于在接受 256MB 原件的同时约束压缩炸弹风险。

```http
POST /api/attachment/upload
Content-Type: multipart/form-data
X-User-Token: <token>
```

```bash
curl -sS -X POST "$BBS_BASE_URL/api/attachment/upload" \
  -H "X-User-Token: $BBS_TOKEN" \
  -F 'file=@/path/to/document.pdf' \
  -F 'downloadScore=0'
```

`downloadScore` 小于 `0` 时按 `0` 处理；`0` 表示免费访问，正整数表示其他用户
需要先支付的积分。成功响应中的 `data` 是附件元数据，不包含对象存储直链：

```json
{
  "id": "附件UUID",
  "fileName": "document.pdf",
  "fileSize": 12345,
  "fileType": "application/pdf",
  "previewable": true,
  "accessGranted": true,
  "downloadScore": 0,
  "downloadCount": 0,
  "downloaded": false
}
```

`fileType` 是服务端识别后的规范 MIME，不能用来替代上传前的扩展名检查。
`previewable=true` 表示预览 PDF 已准备好；`accessGranted=true` 表示当前用户已经
可以读取预览和原件。`downloaded` 保留“已经登记过附件访问记录”的语义，调用方
判断是否需要解锁时应使用 `accessGranted`。

Office 转 PDF 在上传请求内同步完成。转换服务不可用、超时、输出超过部署限制或
输出不是有效 PDF 时，整个上传失败且不会返回附件 ID；不要发布一个未成功上传的
占位附件。Excel 工作簿预览显示文件中已保存的计算结果，不会在浏览器中重新计算公式，
也不提供编辑或筛选交互；页面保留工作表、行列、列宽、行高、合并单元格、常用数字
格式和单元格填充色，并通过横向、纵向滚动浏览完整数据。PowerPoint 预览不播放动画，
字体替换和复杂排版也可能与原始 Office 客户端存在差异。

将一个或多个 UUID 放入创建帖子的 `attachmentIds` 数组后，服务端才会把附件绑定到
该帖子。附件必须属于当前用户、未绑定其他帖子，且总数不能超过配置限制。

### 7.3 获取附件访问权

附件详情不会返回可绕过鉴权的永久 URL。读取原件或预览前先检查帖子详情中附件的
`accessGranted`；为 `false` 时，显式调用：

```http
POST /api/attachment/access/{attachmentId}
X-User-Token: <token>
```

```bash
curl -sS -X POST \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/attachment/access/$ATTACHMENT_ID"
```

该接口是授权和积分变更的唯一入口。附件免费、调用者是帖主或已经取得访问权时，
不会扣除积分；其他用户访问收费附件时，会在同一事务中校验余额、扣除一次积分并
登记访问记录。对同一用户和附件重复调用具有幂等语义，不会重复扣分。成功响应的
`data` 是更新后的附件元数据，此时 `accessGranted=true`。

脚本不得通过预览或下载请求尝试触发购买，也不得在积分不足或无权查看帖子时循环
重试。网络结果不确定时可以重新获取帖子详情；只有仍为 `accessGranted=false` 时才
需要再次调用访问接口。

### 7.4 在线预览与下载原件

取得访问权后，PDF 和 Office 附件统一从以下接口在线浏览：

```http
GET /api/attachment/preview/{attachmentId}
HEAD /api/attachment/preview/{attachmentId}
```

`GET` 返回 `Content-Type: application/pdf` 和内联展示的 `Content-Disposition`。
Office 附件返回的是转换后的 PDF，不会改写原件。接口支持单段 HTTP Range；有效范围
返回 `206 Partial Content`，不可满足的范围返回 `416 Range Not Satisfiable`。
`HEAD` 返回与 `GET` 相同的关键文件头但没有响应体，适合脚本先探测大小和范围能力。

Excel 站内工作簿查看器通过以下受保护接口读取原件：

```http
GET /api/attachment/preview/{attachmentId}/spreadsheet
HEAD /api/attachment/preview/{attachmentId}/spreadsheet
```

该接口只接受 `.xls`、`.xlsx` 附件，沿用相同的帖子可见性和附件解锁校验，响应使用原始
Excel MIME 和 `Content-Disposition: inline`，支持单段 HTTP Range，并且不会增加
`downloadCount`。它用于站内解析，不是对象存储直链，也不能绕过附件积分授权。

```bash
curl -fsS \
  -H "X-User-Token: $BBS_TOKEN" \
  -H 'Range: bytes=0-65535' \
  "$BBS_BASE_URL/api/attachment/preview/$ATTACHMENT_ID" \
  -o preview.part
```

原件下载接口为：

```http
GET /api/attachment/download/{attachmentId}
HEAD /api/attachment/download/{attachmentId}
```

```bash
curl -fsS -OJ \
  -H "X-User-Token: $BBS_TOKEN" \
  "$BBS_BASE_URL/api/attachment/download/$ATTACHMENT_ID"
```

下载响应使用服务端保存的原始文件名并同样支持 Range。预览和下载请求都不会扣积分，
也不会隐式创建购买记录；预览永不增加 `downloadCount`。原件的成功 `GET` 会在响应体
完整写出后增加一次 `downloadCount`，成功的 `206 Range` 也算一次实际传输；`HEAD` 和
`416` 不计数。未登录、无帖子查看权、尚未取得付费附件访问权、附件已删除或尚未绑定
有效帖子时，请求会失败。不要直接访问 `/res/uploads/attachments/...`、
`/res/uploads/attachment-previews/...` 或对象存储地址，这些路径不属于公开附件契约。

这些流接口直接使用 HTTP 状态表达失败，不返回通常的业务 JSON：未登录为 `401`，
无帖子查看权或尚未解锁为 `403`，附件不存在、已删除、未绑定或预览不可用为 `404`。

### 7.5 转换服务部署限制

官方 Compose 会启动仅内部网络可达的 LibreOffice 转换容器，不映射宿主机端口。
自定义部署需要为应用配置以下环境变量：

| 环境变量 | Compose 默认值 | 约束与用途 |
| --- | --- | --- |
| `BBSGO_DOCUMENT_CONVERTER_URL` | `http://document-converter:3000` | Gotenberg 根 URL；只允许路径为空或 `/` 且不含凭据、查询和片段的绝对 HTTP(S) URL |
| `BBSGO_DOCUMENT_CONVERTER_TIMEOUT_SECONDS` | `300` | 单次转换超时，范围 `1..300` 秒 |
| `BBSGO_DOCUMENT_PREVIEW_MAX_OUTPUT_MB` | `256` | 转换后 PDF 最大体积，范围 `1..256` MB |

仓库 Compose 将单个附件上限设为 `256MB`，并为 multipart 封装把 Gotenberg 请求体
限制设为 `257MB`；转换输出上限、临时盘和容器内存也已按该边界同步配置。若管理员
降低 `attachmentConfig.maxSizeMB`，上传接口和前端会使用更小的值。转换 URL 必须指向受信任的内网服务，因为 Office
原件会发送给该服务；该 URL 只供后端使用，不能暴露给浏览器或脚本。

使用 OSS、COS 或 S3 时，上传器不会发送对象 ACL；这样可兼容已启用 Bucket Owner
Enforced、禁用 ACL 的现代 S3 配置。管理员必须通过私有桶或 bucket policy 保证
`attachments/` 与 `attachment-previews/` 前缀不可公开读取，并确保 CDN 不会绕过
BBS 鉴权。S3 部署应启用 Block Public Access 和 Object Ownership；内部或付费附件
建议使用与公开图片分离的私有桶。上线前必须以匿名请求验证两个前缀均返回 `403/404`，
附件只能通过上述鉴权接口读取。

## 8. Python 完整示例

安装依赖：

```bash
python3 -m pip install requests
```

脚本从环境变量读取已经人工登录取得的 token，不把账号密码或验证码写入源代码：

```python
import json
import os
from typing import Any

import requests


class BBSGoError(RuntimeError):
    pass


class BBSGoClient:
    def __init__(self, base_url: str, token: str) -> None:
        self.base_url = base_url.rstrip("/")
        self.session = requests.Session()
        self.session.headers.update({"X-User-Token": token})

    def request(self, method: str, path: str, **kwargs: Any) -> Any:
        response = self.session.request(
            method,
            f"{self.base_url}{path}",
            timeout=(5, 30),
            **kwargs,
        )
        response.raise_for_status()
        try:
            envelope = response.json()
        except ValueError as exc:
            raise BBSGoError("服务器未返回 JSON") from exc

        if envelope.get("success") is not True:
            code = envelope.get("errorCode")
            message = envelope.get("message") or "未知业务错误"
            raise BBSGoError(f"API error {code}: {message}")
        return envelope.get("data")

    def current_user(self) -> dict[str, Any]:
        user = self.request("GET", "/api/user/current")
        if user is None:
            raise BBSGoError("token 无效或已过期")
        return user

    def categories(self, topic_type: int = 0) -> list[dict[str, Any]]:
        return self.request(
            "GET",
            "/api/topic/categories",
            params={"type": topic_type},
        )

    def create_topic(
        self,
        category_id: int,
        title: str,
        content: str,
        tags: list[str] | None = None,
    ) -> dict[str, Any]:
        return self.request(
            "POST",
            "/api/topic/create",
            json={
                "type": 0,
                "categoryId": category_id,
                "title": title,
                "contentType": "markdown",
                "content": content,
                "tags": tags or [],
            },
        )

    def create_comment(
        self,
        topic_id: str,
        content: str,
        image_urls: list[str] | None = None,
        content_type: str = "markdown",
    ) -> dict[str, Any]:
        image_list = [{"url": url} for url in image_urls or []]
        return self.request(
            "POST",
            "/api/comment/create",
            data={
                "entityType": "topic",
                "entityId": topic_id,
                "contentType": content_type,
                "content": content,
                "imageList": json.dumps(image_list, ensure_ascii=False),
            },
        )

    def reply_comment(
        self,
        parent_comment_id: int,
        content: str,
        quote_id: int = 0,
        content_type: str = "markdown",
    ) -> dict[str, Any]:
        return self.request(
            "POST",
            "/api/comment/create",
            data={
                "entityType": "comment",
                "entityId": str(parent_comment_id),
                "quoteId": quote_id,
                "contentType": content_type,
                "content": content,
            },
        )


def main() -> None:
    base_url = os.environ["BBS_BASE_URL"]
    token = os.environ["BBS_TOKEN"]
    category_id = int(os.environ["BBS_CATEGORY_ID"])

    client = BBSGoClient(base_url, token)
    user = client.current_user()
    print(f"当前用户: {user.get('nickname') or user.get('username')}")

    topic = client.create_topic(
        category_id=category_id,
        title="脚本发布的帖子",
        content="## 正文\n\n由维护脚本发布。",
        tags=["自动化"],
    )
    topic_id = topic["id"]
    print(f"帖子 ID: {topic_id}, status: {topic.get('status')}")

    comment = client.create_comment(topic_id, "首条自动评论")
    print(f"评论 ID: {comment['id']}")


if __name__ == "__main__":
    main()
```

运行：

```bash
export BBS_BASE_URL='https://bbs.example.com'
read -r -s -p 'BBS token: ' BBS_TOKEN
printf '\n'
export BBS_TOKEN
export BBS_CATEGORY_ID='1'
python3 automation.py
```

生产任务应优先由运行环境的密钥管理服务注入 `BBS_TOKEN`。上述隐藏输入只用于交互式
运行，避免把 token 明文写进 Shell 历史。

当 `topicCaptcha=true` 时，上面的 `create_topic` 还必须接收并发送即时获取、人工完成的验证码字段。

## 9. 常用错误码

| `errorCode` | 含义 | 建议处理 |
| ---: | --- | --- |
| `-1` | 站点未安装 | 停止任务并检查部署状态 |
| `1` | 未登录 | 验证或重新获取 token |
| `2` | 无权限 | 停止任务，不要重试 |
| `1000` | 验证码错误 | 获取新验证码并人工完成 |
| `1001` | 用户被禁言 | 停止发布任务 |
| `1002` | 用户已禁用 | 停止任务并联系管理员 |
| `1003` | 新用户观察期 | 等待响应消息指出的时长后再尝试 |
| `1004` | 邮箱未验证 | 先完成邮箱验证 |

参数错误、分类不匹配、模块关闭、密码错误等情况常使用 `errorCode=0`，仍应以 `success` 和 `message` 为准。

## 10. 自动化实现注意事项

1. 创建帖子和评论没有幂等键。请求超时并不代表服务端没有成功，不能立即盲目重试；应保存本地任务 ID，并通过列表查询或内容指纹去重。
2. 只对可安全重复的 `GET` 请求做自动重试。对创建、编辑、删除等写请求，在确认服务端结果前不要自动重放。附件 `POST /api/attachment/access/{id}` 是明确的幂等例外，但结果不确定时仍应优先重查帖子详情中的 `accessGranted`。
3. 启动任务时先调用 `/api/user/current`；只有 `success=true` 且 `data` 非空才继续。
4. 每次运行先匿名读取登录配置；取得 token 后再次读取完整配置和分类。不要把站点开关、分类 ID、验证码策略或 token 有效期写死。
5. 帖子 ID 按字符串保存，评论 ID 按整数保存。不要把评论响应中的内部 `entityId` 当作外部帖子 ID。
6. 设置连接和读取超时，记录 `errorCode`、`message`、本地任务 ID 与服务端返回 ID，但不要记录 token、密码或验证码。
7. 限制并发和发布频率。当前代码没有启用发帖频率策略，但这不代表调用方可以无限并发；数据库、搜索索引、通知和任务事件仍有写入成本。
8. 内容命中违禁词时可能返回创建成功但 `status=2`。脚本应把它记录为“待审核”，不能当作公开发布成功。

## 11. 源码索引

接口发生变化时，优先核对以下文件：

- 路由：`internal/server/router.go`
- 登录与验证码：`internal/handlers/api/login_handlers.go`、`internal/handlers/api/captcha_handlers.go`
- token 解析：`internal/services/user_token_service.go`
- 全站登录限制：`internal/middleware/login_required_middleware.go`
- 发帖处理器：`internal/handlers/api/topic_handlers.go`
- 发帖请求与校验：`internal/models/req/request.go`、`internal/services/topic_publish_service.go`
- 评论处理器与服务：`internal/handlers/api/comment_handlers.go`、`internal/services/comment_service.go`
- 附件处理器与服务：`internal/handlers/api/attachment_handlers.go`、`internal/services/attachment_service.go`
- 文档校验与转换：`internal/pkg/docpreview/`
- 统一响应：`internal/pkg/ginx/response.go`
- 返回结构：`internal/models/resp/response.go`
- 前端实际调用：`web/lib/api/client.ts`、`web/components/topic/topic-create-form.tsx`、`web/components/comment/index.tsx`
