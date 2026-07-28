# BBS-Go 项目约定

## 工作原则

- 以臆猜接口为耻，以查档求证为荣
- 以模糊开工为耻，以对齐需求为荣
- 以臆补业务为耻，以请示规则为荣
- 以新增冗余为耻，以复用存量为荣
- 以省略校验为耻，以完备测例为荣
- 以乱改架构为耻，以恪守规范为荣
- 以不懂装懂为耻，以坦诚存疑为荣
- 以批量乱改为耻，以分步迭代为荣

## 架构与运行

- 后端使用 Go/Gin，前端使用 React Router，开发部署使用 PostgreSQL Compose。
- `docker-compose.postgresql.yml` 对外 Web 端口为 `3001`，容器内 Web 端口为 `3000`，Go API 端口为 `8082`。
- 启动命令：`docker compose -f docker-compose.postgresql.yml up -d --build`。
- 持久化目录：`docker-data-postgresql/{data,logs,uploads}`。

## 产品与安全约束

- 这是内部论坛；全站默认仅登录可见，由 `BBSGO_LOGIN_REQUIRED` 控制，默认值为 `true`。
- 已关闭公开注册 API、登录页注册链接和注册页。
- 未登记手机号以及未绑定微信、Google、Google One Tap、GitHub 的身份不得自动创建账号；已有绑定仍可登录。
- 管理员通过 `/dashboard/users` 创建用户：用户名、邮箱必填，昵称选填；表单打开时必填项就应显示红色 `*`。
- 初始密码由后端安全随机生成且只返回一次，数据库只保存哈希；任何用户响应都不得包含密码哈希。
- 话题正文允许嵌入 HTTPS iframe；非 HTTPS、危险属性以及非话题内容仍须过滤。

## 脚本认证与发帖

- 先阅读 `docs/script-api.md`；现有方式是人工登录一次获取 token，脚本使用 `X-User-Token` 或 Bearer Token。
- 不得为脚本自动绕过或全局关闭登录验证码。
- 认证优先级：表单或查询参数 `userToken` > Cookie > Authorization > `X-User-Token`，调用时不要混用。
- HTTP 200 也可能表示业务失败，脚本必须检查 JSON 中的 `success`。
- 当前站点 `tokenExpireDays=365`；代码默认值为 7 天，不得在客户端写死有效期。
- 尚无专用 PAT/API Token。若实现，应支持管理员签发、密钥仅展示一次、作用域、有效期和撤销。
- 已知风险：`UserTokenService.Disable` 更新数据库成功后未正确清理 token 缓存；实现可靠撤销前必须修复。
- 严禁将真实 token、密码或 Cookie 写入仓库、日志或本文件。

## 验证

后端：

```bash
go test -count=1 ./...
```

前端：

```bash
cd web
corepack pnpm typecheck
corepack pnpm lint
node scripts/test-dashboard-routes.mjs
node scripts/test-dashboard-password-result.mjs
node scripts/test-signin-navigation.mjs
node scripts/test-site-access.mjs
```

宿主机可能没有 Go 工具链，可改用 Docker。提交前至少执行：

```bash
git diff --check
docker compose -f docker-compose.postgresql.yml ps
```

工作区中的 `docs/shareholding_change_formal.md` 是用户文件，除非明确要求，不得修改或删除。
