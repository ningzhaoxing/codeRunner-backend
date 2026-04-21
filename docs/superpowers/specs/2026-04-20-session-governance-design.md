---
title: Session 治理与身份统一设计（前后端一体）
type: design
status: draft
created: 2026-04-20
supersedes_parts_of: 2026-03-28-agent-design.md
---

# Session 治理与身份统一设计

## 一、背景与目标

### 现状问题

前一版 Agent 设计（`2026-03-28-agent-design.md`）采用 JSONL 文件 + 短 TTL（1h）的 Session 存储，`session_id` 由客户端透传，没有身份概念。这在单人博客场景下暴露出几个问题：

1. **代码块上下文丢失**：用户关闭页面后 1 小时内回来能继续对话，超过 1 小时整段历史消失；即使在 TTL 内，前端也把 `aiMessages` 按 `blockId` 隔离存储，后端却只有扁平消息流，两端模型不对齐
2. **缺乏身份识别**：任何人拿到 `session_id` 都能接着聊，无法做配额、无法做"我的历史"
3. **存储耦合**：`SessionStore` 直接读写 JSONL 文件，未来接 MySQL 需要大改
4. **匿名与登录态混杂**：没有区分"试用"和"正式用户"，也无法把试用期产生的对话迁移到账号下

### 本次设计目标

- 给 Session 模块做**统一治理**：身份、存储、配额、生命周期、代码块粒度一口气定清楚
- 引入 **GitHub OAuth 2.0** 作为用户身份来源，匿名用户仍可试用
- Session 做到"**同一篇文章 × 同一个用户 = 同一个 Session**"，关闭网页再回来依然能看到历史
- 存储层做**依赖倒置**：SQLite 首发，MySQL 未来可插拔
- 前后端数据模型对齐：消息带 `block_id` 标签，前端按块渲染、后端按块过滤

### 非目标

- 多人协作 / 评论 / 点赞——单人博客不需要
- 邮箱/密码注册——只接 GitHub OAuth
- 跨文章对话——每篇文章独立 Session
- 代码历史版本回溯——只保对话历史，不保代码版本

### 为什么需要 Session 这层抽象

Session 不是为了"存历史"，而是**把一段对话变成后端可寻址、可归属、可治理的一等对象**。一旦它成为有主、有标签、可查询的数据资产，上面就能长出身份、配额、审计、画像、计费、协作、分析一整套能力。前端 localStorage 方案解决的是"这次还能看到"，Session 解决的是"这些对话属于谁、值多少、能用来做什么"。

#### 当前能解决的问题

1. **跨设备/跨时间的对话连续性**：没 Session 时，对话活在前端内存或 localStorage——换浏览器、清缓存、手机看博客就全没了。`(owner, article_id)` 作键后，只要登录，任何设备打开这篇文章都能接着上次聊。
2. **用户之间的数据隔离**：前端隔离只是"各自看不见"，后端没有 owner 抓手就做不了配额、统计、滥用检测。Session 给每段对话打上 `owner`，后端才能真正隔离与治理。
3. **配额与滥用防护**：Session 绑定 owner，才能做 `quota:{owner}:{date}` 精准计数。没有身份时只能按 IP 限流——IP 限流对 NAT/校园网用户误伤，对用代理刷量的人又没用。
4. **代码块级的上下文管理**：同一篇文章里不同代码块的问答不是一回事。Session + `block_id` 让后端按块过滤历史，AI 回答不会被隔壁块的上下文污染，token 也省。前端临时存 `aiMessages[blockId]` 做不到这点——前端分块了，但每次请求仍要把全部历史塞给后端，否则后端和 AI 都无状态。
5. **审计与问题排查**：用户反馈"AI 给我瞎改代码"，有 Session 能按 `session_id` 捞出完整对话 + 工具调用 + 提案 + 执行结果，可复现问题。
6. **匿名 → 登录的平滑过渡**：claim 机制把试用对话带进账号——没 Session 模型就做不到，因为前端历史在登录跳转过程中可能丢失，也无法和后端其他数据对齐。

#### 未来可扩展的方向

1. **"我的对话"列表**：`ListByOwner` 接口已预留，做个页面列出所有 Session，用户能翻过去在哪些文章下问过什么。对技术博客是留存利器——读者会因为"我之前在这里学过东西"再回来。
2. **Session 导出 / 分享**：把一段高质量对话导出为 Markdown，或生成只读分享链接（`/shared/{token}`）。内容创作者主动传播 = 免费推广。
3. **跨会话记忆 / 用户画像**：同一 owner 在多篇文章下的对话能提炼用户偏好（Go 新手、关心并发、喜欢简短回答），在新对话里作为系统提示注入，体验接近 ChatGPT 的 memory。
4. **精细化计费 / 订阅**：未来做付费版，Session 是计费单元的天然载体——免费 200 条/日、Pro 2000 条/日、按 token 后付费等策略都需要 owner 维度统计。
5. **A/B 实验与模型路由**：按 `owner` hash 分桶，确保同一用户始终走同一模型，体验一致。没 owner 只能按请求随机，同一对话里模型忽好忽坏。
6. **异步任务 / 长对话**：Session 持久化后可挂"后台任务"——让 AI 跑长时间代码分析，关掉页面，完成后通过站内信推送，`session_id` 即回调寻址键。
7. **协作（远期）**：`owner` 扩展为 `owners[]` 或增加 `session_participants` 表，就能支持"作者+读者共同调试"。当前不落地，但 schema 留可能性。
8. **训练数据 / 产品洞察**：哪些代码块引发最多提问？哪些问题 AI 答得最差（从追问次数推断）？Session + `block_id` 是这类分析的原始数据底座，可反哺文章改进和 prompt 调优。

---

## 二、核心决策（已锁定）

| # | 决策点 | 选定方案 | 说明 |
|---|--------|---------|------|
| 1 | 身份来源 | **GitHub OAuth 2.0** | 博客目标用户是开发者，GitHub 覆盖率最高；同时打通 issue/PR 等未来集成 |
| 2 | 试用策略 | **B：匿名 10 条/日，登录后 ~200 条/日** | 匿名用户用 localStorage 里的 `anon:{uuid}` 标识，登录后做 claim 迁移 |
| 3 | Session 粒度 | **Option 3：文章级 Session，消息打 `block_id` 标签** | AI 默认只看当前块消息 + 文章上下文；用户明确要求时才跨块 |
| 4 | 存储实现 | **SQLite 首发，Repository 接口抽象** | 依赖倒置，未来可切 MySQL/Postgres 不改业务层 |
| 5 | 匿名/登录共表 | **B1：同一张 `sessions` 表，`owner` 字段区分** | `owner = "anon:{uuid}"` 或 `"user:{github_id}"` |
| 6 | 登录冲突处理 | **C3：云端优先** | 如果云端已有该文章的登录态 Session，放弃匿名 Session |
| 7 | 配额单位 | **D1：按消息条数/日** | 最直观，不依赖 token 计数 |

---

## 三、身份模型

### 3.1 用户类型

```
Anonymous  →  owner = "anon:{uuid}"      存在 localStorage，浏览器级持久
Logged-in  →  owner = "user:{github_id}"  GitHub OAuth 返回的数字 ID
```

匿名身份首次访问时由前端生成 UUID v4，写入 `localStorage.anon_id`，后续所有请求通过 `X-Anon-Id` 头携带。登录后前端仍保留 `anon_id` 直到 claim 完成。

### 3.2 GitHub OAuth 流程

```
前端点击 "Sign in with GitHub"
  → 跳转 /auth/github/login?return_to=/posts/xxx
  → 后端生成 state，写 Redis（5 分钟 TTL），302 到 github.com/login/oauth/authorize
  → 用户授权 → GitHub 回调 /auth/github/callback?code=xxx&state=yyy
  → 后端用 code 换 access_token，用 token 拉 /user 信息
  → UPSERT users 表
  → 签发自有 JWT（7 天有效）写 HttpOnly Cookie
  → 302 回 return_to
```

| 项 | 值 |
|---|---|
| OAuth scope | `read:user`（只读公开资料，不要 repo 权限） |
| JWT claims | `{ sub: "user:{github_id}", name, avatar, exp }` |
| Cookie | `cr_token`，HttpOnly + Secure + SameSite=Lax |
| 刷新策略 | 过期后前端跳回 `/auth/github/login`，无 silent refresh |

### 3.3 claim 流程

登录成功后，前端读 localStorage 里的 `anon_id`，调 `POST /auth/claim`（**仅依赖中间件解析的 `X-Anon-Id` + Cookie JWT，请求 body 为空**，避免客户端伪造任意 anon_id 抢夺他人 Session）：

```sql
-- 后端在事务内执行（伪 SQL）
BEGIN;
SELECT id, article_id FROM sessions
  WHERE owner = 'anon:{uuid}'
  FOR UPDATE;                            -- MySQL；SQLite 单写者，BEGIN IMMEDIATE 即可
-- 对每条匿名 Session：
--   若 (user:{github_id}, article_id) 已存在 → 跳过（C3 云端优先）
--   否则 UPDATE owner = 'user:{github_id}'
COMMIT;
```

claim 成功后前端清除 `localStorage.anon_id`。

**claim 期间的并发**：claim 事务期间任何持 `X-Anon-Id` 的 `/agent/chat` 请求需阻塞或重试——实现上由 `application/session/service.go` 在事务开始前向内存 map 写入 `anonOwner → claiming` 标记，`/agent/chat` 中间件检查到标记时返回 425（Too Early），前端收到后清空 `anon_id` 改用 Cookie 重发。

### 3.4 anon_id 信任模型

`anon_id` 是一串无主 UUID，仅做配额与 Session 归属的临时凭据，**不做身份认证**。已知风险：

- **抢占型 claim**：恶意用户若拿到他人 `anon_id` 并先一步登录调用 `/auth/claim`，会把他人的匿名 Session 划到自己账号下
- **缓解措施**：claim 仅消费当前请求 Cookie 中的 `X-Anon-Id`，不接受请求 body 注入；`anon_id` 通过 `HttpOnly` Cookie 而非纯 localStorage 传输（前端用 `document.cookie` 不可读，单页应用通过专门 endpoint 读取）—— 见 §3.5
- **不缓解的风险**：泄露后被消耗匿名配额（10 条/日），可接受
- **未来增强**：登录前若已有 ≥1 条匿名消息，可在 GitHub 回调成功后增加二次确认页"以下试用对话将合并到你的账号"，让用户视觉确认归属

### 3.5 身份头与 Cookie 策略

| 项 | 设置 |
| --- | --- |
| JWT Cookie 名 | `cr_token` |
| JWT Cookie 属性 | `HttpOnly; Secure; SameSite=Lax; Path=/` |
| anon_id Cookie 名 | `cr_anon` |
| anon_id Cookie 属性 | `HttpOnly; Secure; SameSite=Lax; Path=/`，写入有效期 1 年 |
| 前后端同源部署 | `SameSite=Lax` 满足 OAuth 302 与同源 fetch |
| 前后端跨域部署 | 必须改为 `SameSite=None; Secure` 并配置 CORS `Access-Control-Allow-Credentials: true` + 显式 `Allow-Origin`（不可用 `*`） |
| `Domain` | 同源时不设；跨子域共享时设父域 |

**中间件解析顺序**：`application/auth/middleware.go` 按 `Cookie cr_token → Cookie cr_anon → Header X-Anon-Id` 顺序解析。**JWT 与 anon_id 同时存在时 JWT 始终胜出**，`cr_anon` 保留仅为 claim 使用。`X-Anon-Id` 头仅作为 Cookie 不可用环境（如 CLI 调试）的兜底。

**OAuth state**：写 Redis `oauth_state:{state}` TTL 5 分钟；同时写一个临时 Cookie `cr_oauth_state` SameSite=Lax，回调时双重比对（防 Redis 单点失败被绕过）。

---

## 四、Session 模型

### 4.1 粒度与键

- **Session 键**：`(owner, article_id)` 唯一，即每人每文章一个 Session
- **Message 标签**：每条消息带 `block_id`，AI 默认只拿当前 `block_id` 的历史 + 文章全文作为上下文
- **跨块上下文**：用户消息里显式提到"参考上面代码块"时，前端在请求里带 `include_blocks: ["block-0", "block-1"]`，后端据此放宽过滤

### 4.2 与前端状态对齐

前端现有 `PostPageState.codeBlocks[blockId].aiMessages` 保持不变，只是数据来源从"内存"变为"**后端 `/agent/history?article_id=xxx` 拉下来按 block_id 分组**"。

```typescript
// 首次打开文章页
const history = await fetch(`/agent/history?article_id=${articleId}`);
// 返回: { session_id, messages: [{ id, block_id, role, content, ... }] }
// 前端按 block_id 分组填充到 codeBlocks[blockId].aiMessages
```

### 4.3 生命周期

| 事件 | 匿名 Session | 登录 Session |
|------|-------------|-------------|
| 创建 | 第一次发消息时 lazy 创建 | 同左 |
| TTL | 7 天无活动 GC | 永久（除非用户主动删除） |
| 删除 | 定时任务扫描 | `DELETE /agent/session?article_id=xxx` |
| 迁移 | claim 时转为登录态 | 不迁移 |

**GC 调度**：`application/session/service.go` 启动后台 goroutine，每 6h 触发一次 `GCExpiredAnonymous(ctx, now-7d)`；多实例部署通过 Redis `SET NX PX 10min` 抢锁，单实例运行。`messages` 表通过 `ON DELETE CASCADE` 跟随 `sessions` 级联删除，无需单独清理。

### 4.4 配额

| 角色 | 每日消息上限 | 重置时间 | 超限表现 |
|------|-------------|---------|---------|
| 匿名 | 10 | 服务器 UTC 0 点 | 返回 429，前端提示"今天试用额度用完了，登录解锁更多额度" |
| 登录 | 200 | 同上 | 返回 429，前端提示"今天额度用完了，明天继续" |

配额计数存 Redis：`quota:{owner}:{yyyymmdd-utc}`，`INCR` + `EXPIRE 90000`（25h，覆盖换日窗口）。前端 UI 显示"今日剩余"时一律按服务器返回的 `quota.remaining` 渲染，**不**自行按本地时区计算，避免时区漂移导致显示与重置不同步。

**Redis 是部署强依赖**（既用于配额，也用于 OAuth state）。Redis 不可用时：
- 配额计数路径：fail-open（放行请求 + 打 `quota_redis_unavailable_total` 指标 + WARN 日志），避免一次故障让所有用户被锁死
- OAuth state 路径：fail-closed（拒绝登录回调 + 503），因为身份签发不容降级

---

## 五、存储层设计（依赖倒置）

### 5.1 Repository 接口

```go
// internal/domain/session/repository.go
package session

type Repository interface {
    // Session —— owner 入参均为完整 owner 串（"anon:{uuid}" / "user:{github_id}"），
    // 接口层不做前缀拼接，避免拼装逻辑分散。
    GetOrCreate(ctx context.Context, owner, articleID string) (*Session, error)
    GetByID(ctx context.Context, sessionID string) (*Session, error)
    ListByOwner(ctx context.Context, owner string) ([]*Session, error)
    DeleteByOwnerArticle(ctx context.Context, owner, articleID string) error
    // ClaimAnonymous 在事务内完成：① SELECT … FOR UPDATE 锁定匿名 Session；
    // ② 对每条匿名 Session，若目标 (userOwner, article_id) 已存在则 skip，否则 UPDATE owner。
    // 调用方传入完整 owner 串。
    ClaimAnonymous(ctx context.Context, anonOwner, userOwner string) (claimed int, skipped int, err error)

    // Message
    AppendMessage(ctx context.Context, sessionID string, msg Message) error
    ListMessages(ctx context.Context, sessionID string, filter MessageFilter) ([]Message, error)

    // GC —— 删除 sessions 行即可，messages 表通过 ON DELETE CASCADE 自动清理
    GCExpiredAnonymous(ctx context.Context, before time.Time) (int, error)
}

type MessageFilter struct {
    BlockIDs []string // 空表示不过滤；含 "__meta__" 时返回非代码块消息
    Limit    int      // 0 = 不限；建议默认 200
    Since    time.Time
    Cursor   string   // 翻页游标（消息 id），后端实现按 created_at + id 排序
}
```

`GetOrCreate` 实现必须用 "INSERT … ON CONFLICT DO NOTHING; SELECT" 模式应对并发：SQLite 用 `ON CONFLICT(owner, article_id) DO NOTHING`，MySQL 用 `INSERT IGNORE` 或 `ON DUPLICATE KEY UPDATE updated_at=updated_at`，避免并发首消息时的 UNIQUE 冲突错误。

### 5.2 首个实现：SQLite

- 位置：`internal/agent/session/store/sqlite/`
- 依赖：`modernc.org/sqlite`（纯 Go，无 CGO）
- 连接：单文件 `data/sessions.db`，WAL 模式，单进程够用
- 迁移：`migrations/sqlite/` 下放 SQL 文件，启动时 `go-migrate` 自动 apply

### 5.3 未来：MySQL/Postgres

`internal/agent/session/store/mysql/` 实现同一 `Repository` 接口；应用层通过配置切换：

```yaml
agent:
  session:
    driver: sqlite   # sqlite | mysql
    sqlite:
      path: data/sessions.db
    mysql:
      dsn: user:pass@tcp(host:3306)/coderunner?parseTime=true
```

`internal/agent/session/factory.go` 按配置实例化对应 Repository。

### 5.4 Schema

```sql
-- users: 仅登录用户
CREATE TABLE users (
    id          TEXT PRIMARY KEY,      -- "user:{github_id}"
    github_id   INTEGER NOT NULL UNIQUE,
    login       TEXT NOT NULL,         -- GitHub username
    name        TEXT,
    avatar_url  TEXT,
    created_at  DATETIME NOT NULL,
    last_seen   DATETIME NOT NULL
);

-- sessions: 匿名和登录共表
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,      -- uuid
    owner       TEXT NOT NULL,         -- "anon:{uuid}" or "user:{github_id}"
    article_id  TEXT NOT NULL,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    UNIQUE(owner, article_id)
);
CREATE INDEX idx_sessions_owner ON sessions(owner);
CREATE INDEX idx_sessions_updated ON sessions(updated_at);  -- GC 用

-- messages
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    block_id    TEXT NOT NULL,         -- "block-0", "block-1", ... 或 "__meta__" 代表非代码块上下文
    role        TEXT NOT NULL,         -- user | assistant | tool
    content     TEXT NOT NULL,         -- JSON，见下方 message content schema
    created_at  DATETIME NOT NULL
);
CREATE INDEX idx_messages_session_block ON messages(session_id, block_id, created_at);
```

**`messages.content` JSON schema**（复用 `2026-03-28-agent-design.md` 中 Eino Message 形态）：

```typescript
// role = "user"
{ "text": string, "current_code"?: string, "include_blocks"?: string[] }

// role = "assistant"
{ "text": string, "tool_calls"?: [{ "id": string, "name": string, "args": object }] }

// role = "tool"
{ "tool_call_id": string, "name": string, "result": object }

// 特殊：proposal 消息（role=assistant，附带 tool_call "propose_code"）与 execution_result 消息
// （role=tool，name="execute_code"）共享上述结构，前端靠 tool name + args/result 字段识别类型。
```

**`__meta__` block_id**：保留给系统消息（如"session 创建"、"文章上下文 digest"），`/agent/history` 响应中作为独立分组返回（`messages.__meta__`），前端渲染在 AI 面板顶部"会话信息"折叠区，不混入任何代码块的 `aiMessages`。

### 5.5 从 JSONL 迁移

旧 JSONL 存储不保留历史数据——单人博客阶段数据量小且未对外，首次部署时清空 `data/agent-sessions/` 即可。不写迁移脚本。

---

## 六、HTTP API 变更

### 6.1 新增：身份

| Method | Path | 用途 |
|--------|------|------|
| GET | `/auth/github/login?return_to=` | 302 到 GitHub |
| GET | `/auth/github/callback?code=&state=` | 换 token，签 JWT，302 回 `return_to` |
| GET | `/auth/me` | 返回当前用户（Cookie 解析）或 401 |
| POST | `/auth/logout` | 清 Cookie |
| POST | `/auth/claim` | 无 body，仅依赖 Cookie JWT + `cr_anon` Cookie；返回 `{ claimed, skipped }` |

### 6.2 新增：Session 管理

#### `GET /agent/history`

```
GET /agent/history?article_id=go-concurrency&block_id=block-1&limit=200&cursor=
```

| 参数 | 必填 | 说明 |
|------|-----|------|
| `article_id` | 是 | 文章 ID |
| `block_id` | 否 | 过滤指定代码块；不传返回所有块（含 `__meta__`） |
| `limit` | 否 | 默认 200，上限 500 |
| `cursor` | 否 | 翻页游标，传上一页响应中的 `next_cursor` |

```json
// 200 响应
{
  "session_id": "uuid",       // null 表示该文章尚无 Session
  "owner_kind": "anon",       // anon | user
  "messages": [
    {
      "id": "msg-uuid",
      "block_id": "block-1",
      "role": "user",
      "content": { /* 见 §5.4 message content schema */ },
      "created_at": "2026-04-20T08:30:00Z"
    }
  ],
  "next_cursor": "msg-uuid",  // 无更多时为 null
  "quota": { "remaining": 8, "limit": 10, "reset_at": "2026-04-21T00:00:00Z" }
}
```

`quota` 字段始终返回，前端用于初始化 `session.quota` 状态。

#### `DELETE /agent/session`

```
DELETE /agent/session?article_id=go-concurrency
```

返回 `204 No Content`。删除后下次 `/agent/chat` 会重新 lazy 创建。

### 6.3 修改：`/agent/chat`

- **身份解析**：中间件按 §3.5 顺序（Cookie JWT → Cookie cr_anon → Header X-Anon-Id），JWT 存在时其余忽略，写入 `ctx.owner`
- **Session 解析**：不再接收客户端 `session_id`，改由 `(owner, article_id)` 查找或 lazy 创建
- **claim 期间**：若中间件检测到 `application/session` 模块对当前 anon_owner 的 `claiming` 标记，返回 `425 Too Early`，body `{ "code": "claim_in_progress" }`，前端 200ms 后重试
- **新增请求字段**：`block_id`（必填）、`include_blocks`（可选）
- **配额中间件**：Redis INCR 计数，超限返回 429 + `{ "code": "quota_exceeded", "quota": {...} }`

```json
// POST /agent/chat 新版 body
{
  "article_id": "go-concurrency",
  "block_id": "block-1",
  "include_blocks": ["block-0"],       // 可选
  "user_message": "为什么没输出？",
  "current_code": "package main...",
  "article_ctx": { ... }               // 首次传入
}
```

SSE 事件格式保持不变（`session`、`chunk`、`proposal`、`done`、`execution_result`、`interrupted`、`error`）。

### 6.4 修改：`/agent/confirm`

不变，继续按 `session_id + proposal_id` 处理，只是 `session_id` 由 `/agent/chat` 的 `session` 事件下发。

---

## 七、模块结构

### 7.1 后端

> 本项目遵循 DDD 四层（`interfaces → application → domain → infrastructure`），领域层不依赖基础设施。新增模块按层划分如下：

```
codeRunner-backend/
├── internal/
│   ├── interfaces/
│   │   └── http/
│   │       ├── auth_handler.go           # /auth/github/* /auth/me /auth/logout /auth/claim
│   │       └── agent_handler.go          # /agent/chat /agent/confirm /agent/history /agent/session
│   │
│   ├── application/
│   │   ├── auth/
│   │   │   ├── service.go                # 编排：换 token → 拉用户 → upsert → 签 JWT
│   │   │   └── middleware.go             # Gin 中间件：解析 Cookie/X-Anon-Id → 注入 owner 到 ctx
│   │   ├── agent/
│   │   │   └── chat_service.go           # 编排 Session 解析、配额扣减、调 Eino、回填消息
│   │   └── session/
│   │       └── service.go                # claim、reset、history 的应用服务（事务边界在这里）
│   │
│   ├── domain/
│   │   ├── user/
│   │   │   ├── model.go                  # User 实体
│   │   │   └── repository.go             # UserRepository 接口
│   │   └── session/
│   │       ├── model.go                  # Session / Message 实体
│   │       └── repository.go             # Repository 接口（见 §5.1）
│   │
│   └── infrastructure/
│       ├── auth/
│       │   ├── github/client.go          # OAuth2 client（golang.org/x/oauth2 + go-github）
│       │   └── jwt/signer.go             # 签发/验签
│       ├── persistence/
│       │   ├── db/
│       │   │   ├── sqlite.go             # 连接池 + migration runner
│       │   │   └── mysql.go              # 预留
│       │   ├── user/
│       │   │   └── sqlite_repository.go  # UserRepository 实现
│       │   └── session/
│       │       ├── factory.go            # 按配置选 driver
│       │       ├── sqlite_repository.go  # Repository 实现
│       │       ├── mysql_repository.go   # 预留
│       │       └── migrations/
│       │           ├── sqlite/
│       │           └── mysql/
│       └── quota/
│           └── redis.go                  # INCR 计数（domain 层只暴露 QuotaService 接口）
│
└── configs/
    ├── dev.yaml                          # 新增 agent.session + auth + redis 段
    └── product.yaml
```

**层级约束复述**：

- `domain/session` 与 `domain/user` 只定义实体与仓储接口，禁止 import 任何 `infrastructure/*`
- `application/*` 持有 `Repository` 接口，构造时由 `cmd/api/main.go` 注入具体实现
- `interfaces/http/*` 只做参数解析、调 application、写响应，不直接碰 Repository

### 7.2 前端

```
codeRunner-front/
├── src/
│   ├── stores/
│   │   ├── auth.ts                       # 新增：user / anonId / login / logout / claim
│   │   └── post.ts                       # 改：session 从 /agent/history 初始化
│   ├── components/
│   │   ├── Navbar/
│   │   │   └── AuthButton.tsx            # 新增：未登录显示 GitHub 按钮，登录显示头像+菜单
│   │   ├── CodeBlock/
│   │   │   ├── AIPanel/
│   │   │   │   ├── QuotaIndicator.tsx    # 新增："今天还能聊 X 条"
│   │   │   │   └── QuotaExhausted.tsx    # 新增：超限后显示登录引导
│   │   │   └── ...
│   │   └── Layout/
│   │       └── ClaimToast.tsx            # 新增：登录成功后如有可迁移 Session，Toast 提示
│   ├── lib/
│   │   ├── anonId.ts                     # localStorage.anon_id 读写
│   │   ├── api/
│   │   │   ├── auth.ts                   # /auth/* 封装
│   │   │   └── agent.ts                  # 所有请求自动带 Cookie 或 X-Anon-Id
│   │   └── fetchSSE.ts                   # 已有，改为透传身份头
│   └── app/
│       └── auth/
│           └── callback/page.tsx         # 可选：如果用 SPA 模式处理 callback
```

### 7.3 前端状态变更

```typescript
// stores/auth.ts
interface AuthState {
  user: { id: string; login: string; avatarUrl: string } | null;
  anonId: string;              // 始终存在，登录后也保留直到 claim 完成
  status: 'loading' | 'anon' | 'authed';

  login(returnTo: string): void;           // 跳 /auth/github/login
  logout(): Promise<void>;
  claimIfNeeded(): Promise<{ claimed: number; skipped: number }>;
}

// stores/post.ts 修改
interface PostPageState {
  article: Article;
  codeBlocks: Record<string, CodeBlockState>;  // 结构不变
  session: {
    sessionId: string | null;              // 改为从 /agent/history 拉
    activeBlockId: string | null;
    isStreaming: boolean;
    sseConnected: boolean;
    proposals: Record<string, Proposal>;
    globalError: string | null;
    quota: { remaining: number; limit: number } | null;  // 新增
  };

  loadHistory(articleId: string): Promise<void>;  // 新增：初始化时调用
  resetSession(): Promise<void>;                  // 新增：DELETE /agent/session
}
```

---

## 八、前端 UI 变更

### 8.1 登录入口

- 位置：Navbar 右侧，主题切换按钮旁
- 未登录：`[🔑 Sign in with GitHub]` 按钮（绿色边框 + GitHub icon）
- 已登录：头像 + 用户名下拉菜单 `[我的对话] [登出]`

### 8.2 配额指示

AI 面板 header 右下角小字：
- 匿名：`试用额度 7/10` 灰色，接近上限变黄
- 登录：`今天 42/200`，充足时可隐藏
- 超限：输入框禁用，显示引导：
  - 匿名 → `试用额度用完了。[使用 GitHub 登录] 解锁更多额度`
  - 登录 → `今天额度用完了，明天继续 ✨`

### 8.3 Claim Toast

登录成功跳回文章页后，若后端返回 `claimed > 0`：

```
┌─────────────────────────────────────────┐
│ ✅ 已同步 3 篇文章的试用对话到你的账号     │
└─────────────────────────────────────────┘
```

若 `skipped > 0`（C3 冲突）：

```
┌─────────────────────────────────────────┐
│ ℹ️ 1 篇文章在云端已有对话，试用记录已丢弃 │
└─────────────────────────────────────────┘
```

---

## 九、安全与隐私

- OAuth scope 只要 `read:user`，不碰仓库权限
- JWT 存 HttpOnly Cookie，防 XSS 窃取
- `state` 参数 + Redis 5min TTL，防 CSRF
- `anon_id` 只做试用计数，不当身份认证；即使泄露最多被人消耗配额，不影响已登录数据
- 不记录 GitHub email（scope 没要）
- 用户主动登出时 Cookie 清除，但 Session 数据保留，下次登录仍能看到
- 未来考虑：`DELETE /auth/me` 彻底删号 + 级联删 sessions

---

## 十、指标（Prometheus）

在现有 Agent metrics 基础上新增：

| 指标 | 类型 | 标签 |
|------|------|------|
| `auth_login_total` | Counter | `result=success\|fail` |
| `auth_claim_total` | Counter | `outcome=claimed\|skipped` |
| `session_active_total` | Gauge | `owner_kind=anon\|user` |
| `session_message_total` | Counter | `owner_kind`, `block_mode=single\|multi` |
| `quota_exceeded_total` | Counter | `owner_kind` |
| `session_gc_deleted_total` | Counter | — |

---

## 十一、实施阶段建议

非强制顺序，留给 writing-plans 阶段细化：

1. **基础设施**：SQLite 连接池、migration 工具、`db` 封装
2. **Repository + SQLite 实现**：脱离 HTTP 层先把数据层和单测跑通
3. **身份模块**：`/auth/github/*` + JWT + 中间件
4. **Chat handler 改造**：接入 `(owner, article_id)` Session 解析、消息带 `block_id`
5. **配额中间件**：Redis 计数 + 429 响应
6. **`/agent/history` + `/auth/claim`**：最后一公里
7. **前端**：`auth` store → Navbar 按钮 → `loadHistory` → 配额 UI → Claim Toast

---

## 十二、待议（不阻塞本次设计）

- 是否把 SQLite 也封到 `infrastructure/common/db/`，还是 `agent/session/store/sqlite` 独占——先独占，以后有别的模块要用 SQLite 再提
- MySQL 实现何时落地——视访问量，>1k 日活或多实例部署时
- "我的对话"页（列出所有 Session）——本次不做，留给后续独立 feature
- Session 导出/下载——同上
