---
title: 用户反馈表单
date: 2026-04-20
status: draft
---

# 用户反馈表单设计

## 1. 背景与目标

博客读者目前没有渠道向作者反馈意见。本功能提供一个简单的反馈表单，用户提交的内容通过 QQ SMTP 直接发送到作者的 QQ 邮箱。

**目标**：
- 用户 5 秒内能找到反馈入口并开始填写；
- 提交后邮件能稳定到达作者邮箱；
- 防止被恶意机器人灌水（IP 限流 + 长度校验）。

**非目标（YAGNI）**：
- 不落库、不做管理后台；
- 不做验证码；
- 不做富文本、附件上传；
- 不做回执邮件给用户。

## 2. 用户故事

- 作为读者，我看到导航栏有「反馈」入口，点击进入表单页。
- 作为读者，我选择反馈类型（Bug / 建议 / 其他），填写内容，可选留下联系方式，点击提交。
- 作为读者，如果提交过于频繁，我会看到「提交过于频繁，请稍后再试」的提示。
- 作为作者，我在 QQ 邮箱收到一封主题含反馈类型的邮件，正文包含时间、IP、类型、内容、联系方式。

## 3. 架构概览

```
[Browser] --POST /api/feedback--> [codeRunner-backend]
                                      │
                    ┌─────────────────┼───────────────────┐
                    │                 │                   │
            IP RateLimit         Validation          SMTP Send
              (Redis)            (domain layer)    (smtp.qq.com:465)
                                                        │
                                                        ▼
                                                   [QQ Mailbox]
```

后端遵循现有 DDD 分层。邮件同步发送（QQ SMTP 延迟通常 <1s），不引入队列。

## 4. 前端设计

### 4.1 入口

在 Header 导航栏（`src/components/Header.tsx` 或同等位置）的「关于」后新增「反馈」链接，指向 `/feedback`。当前路径为 `/feedback` 时高亮。

### 4.2 页面

新建 `src/app/feedback/page.tsx`，客户端组件（`"use client"`），样式与现有页面一致（`max-w-[720px] mx-auto`）。

表单字段：

| 字段 | 类型 | 必填 | 约束 | UI |
|------|------|------|------|------|
| type | enum | 是 | bug / suggestion / other | 下拉选择 |
| content | string | 是 | 10–2000 字符 | textarea，6 行 |
| contact | string | 否 | 0–100 字符 | input，placeholder："邮箱 / 微信 / QQ（可选，用于回复）" |

交互：
- 前端做轻量预校验（长度），不通过的提示在字段下方。
- 提交时按钮变 Loading 态并禁用。
- 成功：替换表单为「感谢你的反馈！」绿色卡片，提供「返回首页」链接。
- 失败：表单上方红色提示；限流错误特殊展示「提交过于频繁，请稍后再试」。

### 4.3 API 客户端

在 `src/lib/api.ts` 中新增：

```ts
export async function submitFeedback(payload: {
  type: "bug" | "suggestion" | "other";
  content: string;
  contact?: string;
}): Promise<{ success: boolean; message: string }>;
```

统一处理 429 与其他错误，返回标准化结构给 UI。

## 5. 后端设计

### 5.1 目录结构

```
internal/
├── interfaces/controller/feedback/
│   └── handler.go          // HTTP handler：POST /api/feedback
├── application/feedback/
│   └── service.go          // FeedbackService.Submit(ctx, cmd) error
├── domain/feedback/
│   ├── feedback.go         // Feedback 实体 + 校验
│   └── errors.go           // ErrInvalidContent / ErrRateLimited / ...
└── infrastructure/mail/
    ├── smtp_mailer.go      // SMTPMailer 实现 Mailer 接口
    └── mailer.go           // Mailer interface（供 application 依赖）
```

限流依赖已有 Redis 客户端，放在 `infrastructure/ratelimit`（如已存在复用之，否则新增）。

### 5.2 HTTP 接口

`POST /api/feedback`

请求：
```json
{
  "type": "bug",
  "content": "...",
  "contact": "me@example.com"
}
```

成功响应（200）：
```json
{ "success": true, "message": "感谢反馈" }
```

失败：
- 400 `{ "success": false, "message": "内容长度需在 10-2000 字符之间" }`
- 429 `{ "success": false, "message": "提交过于频繁，请稍后再试" }`
- 500 `{ "success": false, "message": "发送失败，请稍后重试" }`（具体原因只记日志）

### 5.3 校验规则（domain）

- `type ∈ {bug, suggestion, other}`，否则 `ErrInvalidType`
- `10 ≤ len(content) ≤ 2000`
- `len(contact) ≤ 100`
- 所有字符串 `strings.TrimSpace` 后参与长度判断

### 5.4 限流

Redis keys：
- `feedback:rl:min:{ip}` — TTL 60s，阈值 1 次
- `feedback:rl:day:{ip}` — TTL 86400s，阈值 10 次

使用 `INCR` + `EXPIRE`（首次设置）。任一超限即返回 429。

IP 获取优先级：`X-Forwarded-For` 首段（去空格）→ `X-Real-IP` → `RemoteAddr`。

### 5.5 邮件

依赖 `gopkg.in/gomail.v2`（已广泛使用、稳定）。TLS 直连 `smtp.qq.com:465`。

- 主题：`[CodeRunner反馈][{type}] {content 前 30 字}`
- 正文（HTML，字段全部 HTML 转义）：
  ```
  时间：2026-04-20 21:30:00
  IP：1.2.3.4
  类型：bug
  联系方式：me@example.com

  ---
  {content}
  ```
- Reply-To 头部：若 contact 形如合法邮箱，则设置为该邮箱以便作者直接回复。

SMTPMailer 构造：
```go
type SMTPMailer struct {
    dialer *gomail.Dialer
    from   string
    to     string
}
```

### 5.6 配置

`configs/dev.yaml` / `product.yaml` 新增：

```yaml
mail:
  enabled: true
  host: smtp.qq.com
  port: 465
  username: your_qq@qq.com
  password: ${MAIL_PASSWORD}
  from: your_qq@qq.com
  to: your_qq@qq.com

feedback:
  rate_limit_per_min: 1
  rate_limit_per_day: 10
  content_min: 10
  content_max: 2000
  contact_max: 100
```

`MAIL_PASSWORD` 通过环境变量注入，禁止提交仓库。`docker-compose.yml` 增加对应环境变量透传。

### 5.7 依赖注入 / 路由

在现有 server 启动流程中：
1. 从配置构造 `SMTPMailer` 与 `RateLimiter`；
2. 构造 `FeedbackService`；
3. 注册路由到 controller handler。

遵循现有 `enter.go` 风格。

## 6. 错误处理与日志

- domain 层只返回业务错误（类型已枚举）；
- application 层：限流 → 返回 `ErrRateLimited`；SMTP 失败 → 包装为 `ErrMailSend`，原始错误写入 `log.Errorw`；
- controller 层：根据错误类型映射 HTTP 状态码，用户可见文案固定几条，不拼错误细节；
- 请求日志只记录 type 与 content 长度，不记录完整 content、contact（最小化 PII）。

## 7. 测试策略

**单元测试**：
- `domain/feedback`：校验各边界（长度、type 白名单、trim）；
- `application/feedback`：mock `Mailer` + `RateLimiter`，覆盖成功 / 限流 / 邮件失败；
- `infrastructure/mail`：对 `gomail.Dialer` 做接口抽象便于 mock，或用本地 mock SMTP 服务器（如 `smtp-mock`）。

**集成测试**：
- controller 层用 `httptest`，串通 service + 假 mailer + miniredis。

**手工冒烟**：
- 本地起后端 + 前端；
- 用真实 QQ 授权码；
- 提交一次 → 确认收到邮件；
- 连续提交 → 确认 429；
- 填非法长度 → 确认 400。

## 8. 安全与隐私

- 授权码仅通过 env 注入，禁止入库；
- Reply-To 注入防护：只有当 contact 匹配严格 email 正则才设置为 Reply-To，否则不设；
- 邮件正文全部 HTML 转义，避免在邮件客户端执行注入；
- IP 仅写入邮件正文供作者参考，不长期存储；
- CORS 仅允许前端站点域名。

## 9. 上线步骤

1. QQ 邮箱开启 SMTP 服务并获取授权码；
2. 服务器 `.env` 中配置 `MAIL_PASSWORD`；
3. 部署后端 → 验证 `/api/feedback` 返回 200；
4. 部署前端 → 真实提交一次 → 收到邮件即通过。

## 10. 未来扩展（非本期）

- 落库 + 管理页面；
- 增加 hCaptcha 验证；
- 允许附件（截图）；
- 通过队列异步发送，提升接口响应速度。
