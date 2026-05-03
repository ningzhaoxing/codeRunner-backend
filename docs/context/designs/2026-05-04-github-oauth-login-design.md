# GitHub OAuth 登录身份层设计

- 日期：2026-05-04
- 状态：已确认方案，待实现
- 范围：后端 HTTP 登录身份层
- 不涉及：Agent session 归属、匿名会话 claim、配额、用户数据库、前端 UI

---

## 一、背景

当前项目已有 gRPC TokenIssuer，用于服务间调用鉴权：

- `tokenIssuer.GenerateToken` 使用 `AUTH_PASSWORD` 签发 JWT
- gRPC middleware 从 metadata `token` 验证 JWT
- HTTP Agent 接口目前只支持可选 `X-Agent-API-Key`
- Agent 会话归属仍依赖请求中的 `visitor_id`

本次需求是新增 GitHub OAuth2 登录能力，先提供用户身份识别闭环。它不替换现有 gRPC token，也不改变 Agent 会话存储和归属模型。

---

## 二、目标

实现一个最小可用的 GitHub OAuth2 登录身份层：

1. 用户可通过 GitHub OAuth 登录。
2. 登录成功后，后端写入同域 HttpOnly Cookie。
3. 前端可通过 `/auth/me` 获取当前登录用户。
4. 用户可通过 `/auth/logout` 清除登录态。
5. 不保存 GitHub access token，不引入用户表。

---

## 三、非目标

本次明确不做：

- 不改 `/agent/chat` 的 `visitor_id` 参数。
- 不把 Agent session 绑定到 GitHub 用户。
- 不做 `/auth/claim`。
- 不引入 SQLite/MySQL 用户表。
- 不引入 Redis。
- 不做配额系统。
- 不请求 GitHub repo 权限。
- 不实现前端登录按钮或用户菜单。

后续如果推进 session governance，可基于本次 JWT claims 中的 `github_id` 迁移到 users 表和 session owner 模型。

---

## 四、确认决策

| 决策点 | 结论 | 理由 |
| --- | --- | --- |
| 实施范围 | 只做登录身份层 | 边界清晰，降低对 Agent 会话的影响 |
| Cookie 部署模型 | 同域部署 | 使用 `SameSite=Lax`，避免跨站 Cookie 复杂度 |
| 配置来源 | `configs/*.yaml` + 环境变量 | 与现有 Viper 配置方式一致 |
| 用户存储 | 无数据库，JWT 内嵌 GitHub 用户信息 | 最小闭环，不引入 migration |
| OAuth state | HMAC 签名 state，无服务端存储 | 不引入 Redis/DB，5 分钟 TTL 控制风险 |
| 登录有效期 | 7 天 | 体验和无状态 JWT 撤销限制之间的折中 |

---

## 五、配置设计

在 `configs/dev.yaml` 和 `configs/product.yaml` 新增：

```yaml
auth:
  github:
    client_id: ${GITHUB_CLIENT_ID}
    client_secret: ${GITHUB_CLIENT_SECRET}
    redirect_url: ${GITHUB_REDIRECT_URL}
  jwt:
    secret: ${AUTH_JWT_SECRET}
    ttl_seconds: 604800
  cookie:
    name: "cr_auth"
    secure: true
```

开发环境可将 `auth.cookie.secure` 设为 `false`；生产环境必须为 `true`。

`AUTH_JWT_SECRET` 与现有 `JWT_SECRET` 分开。现有 `JWT_SECRET` 服务于 gRPC token；`AUTH_JWT_SECRET` 服务于浏览器用户登录 Cookie。两个用途不应混用。

---

## 六、HTTP API

### 6.1 `GET /auth/github/login`

查询参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `return_to` | 否 | 登录成功后跳回的路径 |

行为：

1. 校验 `return_to`。
2. 生成签名 state，包含 `nonce`、`return_to`、`exp`。
3. 302 跳转 GitHub authorize URL。

`return_to` 规则：

- 空值默认 `/`。
- 只允许以单个 `/` 开头的相对路径。
- 禁止 `http://...`、`https://...`、`//evil.com`、反斜杠等可疑形式。

OAuth scope：

```text
read:user
```

不请求 email 和 repo 权限。

### 6.2 `GET /auth/github/callback`

查询参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `code` | 是 | GitHub OAuth 授权码 |
| `state` | 是 | 登录入口生成的签名 state |

行为：

1. 校验 `code` 和 `state` 存在。
2. 验证 state 签名和过期时间。
3. 使用 code 换取 GitHub access token。
4. 调用 GitHub `/user` 获取基础用户信息。
5. 签发本系统 JWT。
6. 写入登录 Cookie。
7. 302 跳转到 state 中的 `return_to`。

错误处理：

- 缺少 `code` 或 `state`：400。
- state 签名无效或过期：400。
- GitHub token exchange 失败：502。
- GitHub profile fetch 失败：502。

### 6.3 `GET /auth/me`

行为：

1. 读取登录 Cookie。
2. 验证 JWT 签名和过期时间。
3. 从 claims 返回用户信息。

成功响应：

```json
{
  "user": {
    "id": "github:123",
    "github_id": 123,
    "login": "octocat",
    "name": "The Octocat",
    "avatar_url": "https://avatars.githubusercontent.com/u/123?v=4"
  }
}
```

未登录、Cookie 缺失、JWT 无效或过期时返回 401：

```json
{ "message": "unauthorized" }
```

### 6.4 `POST /auth/logout`

行为：

1. 写入同名过期 Cookie。
2. 返回 204。

---

## 七、Cookie 设计

登录 Cookie：

| 属性 | 值 |
| --- | --- |
| Name | 配置项 `auth.cookie.name`，默认 `cr_auth` |
| Path | `/` |
| HttpOnly | `true` |
| Secure | 配置项 `auth.cookie.secure` |
| SameSite | `Lax` |
| MaxAge | 604800 秒 |

使用 `SameSite=Lax` 的原因：

- 同域部署下可满足正常 API 请求。
- GitHub callback 顶层导航回站点时 Cookie 可正常写入。
- 不需要跨站 `SameSite=None` 带来的 Secure/credentials 复杂度。

---

## 八、JWT Claims

签名算法：

```text
HS256
```

Claims：

```json
{
  "sub": "github:123",
  "github_id": 123,
  "login": "octocat",
  "name": "The Octocat",
  "avatar_url": "https://avatars.githubusercontent.com/u/123?v=4",
  "iat": 1777862400,
  "exp": 1778467200
}
```

约束：

- `sub` 使用 `github:{github_id}` 格式。
- `github_id` 是后续引入 users 表时的稳定外部身份。
- JWT 中不包含 GitHub access token。
- 用户资料变更不会实时反映，需要重新登录刷新 claims；本轮接受该限制。

---

## 九、OAuth State 设计

State 使用无状态 HMAC 签名结构。

Payload 字段：

```json
{
  "nonce": "base64url-random",
  "return_to": "/posts/go-concurrency",
  "exp": 1777862700
}
```

编码格式：

```text
base64url(json(payload)) + "." + base64url(hmac_sha256(payload_part, AUTH_JWT_SECRET))
```

校验规则：

1. state 必须由两段组成。
2. payload 必须是合法 JSON。
3. HMAC 必须恒定时间比较通过。
4. `exp` 必须晚于当前时间。
5. `return_to` 必须再次通过相对路径校验。

TTL：

```text
5 分钟
```

已知权衡：

- 无状态 state 不能一次性消费。
- 在 5 分钟 TTL 和 HMAC 防篡改前提下，本轮接受该风险。
- 后续如果接入 Redis，可替换为 `oauth_state:{state}` TTL 存储并 callback 后删除。

---

## 十、模块设计

新增独立 HTTP auth 模块，不复用现有 `internal/application/service/auth`。原因：现有 auth 是 gRPC token issuer，语义是服务鉴权；本次 auth 是浏览器用户登录。混用会造成边界不清。

建议目录：

```text
internal/auth/
├── config.go          # AuthConfig / GitHubConfig / JWTConfig / CookieConfig
├── github_client.go   # GitHub OAuth token exchange + /user profile fetch
├── jwt.go             # 用户登录 JWT 签发/解析
├── state.go           # OAuth state 签名/校验
├── service.go         # 编排 login URL、callback、me、logout
└── handler.go         # Gin handler
```

路由注册：

```text
GET  /auth/github/login
GET  /auth/github/callback
GET  /auth/me
POST /auth/logout
```

建议在 `internal/interfaces/adapter/router/router.go` 中注册到主 HTTP router。`/auth/*` 不挂 Agent API key middleware。

---

## 十一、依赖

建议新增：

```text
golang.org/x/oauth2
```

GitHub `/user` 可直接用标准库 `net/http` 调用，避免为了一个接口引入完整 GitHub SDK。

---

## 十二、安全要求

1. `return_to` 必须防 open redirect。
2. OAuth scope 只使用 `read:user`。
3. 不保存 GitHub access token。
4. 登录 JWT secret 与 gRPC JWT secret 分离。
5. Cookie 必须 HttpOnly。
6. 生产环境 Cookie 必须 Secure。
7. state 必须 HMAC 签名并设置 5 分钟过期。
8. 所有 token、secret 不写日志。

---

## 十三、测试策略

### 单元测试

覆盖：

- `return_to` 校验：
  - `/posts/1` 通过
  - 空值归一化为 `/`
  - `http://evil.com` 拒绝
  - `https://evil.com` 拒绝
  - `//evil.com` 拒绝
  - `\evil` 拒绝
- state：
  - 正常签发和验证
  - payload 篡改失败
  - 签名篡改失败
  - 过期失败
- JWT：
  - 正常签发和解析
  - secret 错误失败
  - 过期失败
- Cookie：
  - 登录设置 `HttpOnly`、`SameSite=Lax`、`Path=/`
  - logout 写过期 Cookie

### Handler 测试

覆盖：

- `/auth/github/login` 返回 302，Location 指向 GitHub authorize URL。
- `/auth/me` 无 Cookie 返回 401。
- `/auth/me` 有有效 Cookie 返回用户信息。
- `/auth/logout` 返回 204 并清 Cookie。

GitHub callback handler 应通过 fake GitHub client 测试，不访问真实 GitHub。

---

## 十四、后续演进

本设计为后续功能保留接口：

- 引入 users 表：以 `github_id` upsert 用户。
- Agent session owner：将 `visitor_id` 替换为 Cookie JWT 解析出的 `github:{id}` 或匿名 owner。
- `/auth/claim`：登录后迁移匿名会话。
- 配额：按 owner 维度接 Redis 计数。
- 用户页：列出当前用户历史文章会话。

这些能力不属于本轮实现。
