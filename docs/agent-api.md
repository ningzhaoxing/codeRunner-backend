# Agent API 前端对接文档

Base URL: `http://<server>:7979`

## 鉴权

如果后端配置了 `AGENT_API_KEY`，所有 `/agent/*` 请求需携带：

```http
X-Agent-API-Key: <your-api-key>
```

未配置时无需鉴权。

---

## 1. 对话 — POST /agent/chat

发起或继续一轮 Agent 对话，响应为 SSE 流。

### 请求

```json
{
  "session_id": "",
  "user_message": "请解释这段代码",
  "article_ctx": {
    "article_id": "art-123",
    "article_content": "文章正文...",
    "code_blocks": [
      { "language": "python", "code": "print('hello')" }
    ]
  }
}
```

### 三种模式

| 模式 | 条件 | 行为 |
| ---- | ---- | ---- |
| 创建 | `session_id` 为空 + 有 `article_ctx` | 创建新会话，返回 `session_created` 事件 |
| 重置 | `session_id` 非空 + 有 `article_ctx` | 清空旧会话，以新文章上下文重建 |
| 继续 | `session_id` 非空 + 无 `article_ctx` | 在已有会话上继续对话 |

### SSE 响应

响应 `Content-Type: text/event-stream`，每个事件格式：

```text
event: <event_type>
data: <json_payload>

```

### SSE 事件类型

#### session_created

新会话创建成功时发送（仅创建模式）。

```json
event: session_created
data: {"type":"session_created","session_id":"uuid-xxx"}
```

前端应保存 `session_id`，后续请求用它继续对话。

#### stream_chunk

Agent 流式输出的文本片段，逐字/逐句到达。

```json
event: stream_chunk
data: {"type":"stream_chunk","content":"这段代码的"}
```

```json
event: stream_chunk
data: {"type":"stream_chunk","content":"作用是..."}
```

前端应将 `content` 拼接追加到当前回复气泡中。

#### message

Agent 的完整非流式消息（较少出现，通常走 stream_chunk）。

```json
event: message
data: {"type":"message","content":"完整的回复内容"}
```

#### tool_result

Agent 调用工具后返回的结果。

```json
event: tool_result
data: {"type":"tool_result","content":"stdout: hello\n"}
```

#### interrupt

Agent 请求用户确认执行代码（HITL 中断）。

```json
event: interrupt
data: {"type":"proposal","proposal":{"proposal_id":"p-xxx","code":"print(1)","language":"python"}}
```

前端应展示代码预览和「确认执行」按钮，用户确认后调用 `POST /agent/confirm`。

#### error

处理过程中发生错误。

```json
event: error
data: {"type":"error","error":"qwen API timeout"}
```

#### done

本轮对话结束。

```json
event: done
data: {"type":"done"}
```

收到 `done` 后关闭 EventSource 连接。

#### Keep-Alive（心跳）

每 5 秒发送一次 SSE 注释帧，防止代理/浏览器超时断开：

```text
: keepalive

```

这是 SSE 标准注释，`EventSource` 会自动忽略，无需处理。

---

## 2. 确认执行 — POST /agent/confirm

用户确认 Agent 提议的代码执行。响应同样为 SSE 流（事件类型与 `/agent/chat` 一致）。

### 请求体

```json
{
  "session_id": "uuid-xxx",
  "proposal_id": "p-xxx"
}
```

### SSE 响应

SSE 流，依次可能出现：`message`（executing）→ `stream_chunk` → `done`。

---

## 3. 会话列表 — GET /agent/sessions

获取所有活跃（未过期）的会话元信息。

### 会话列表响应

```json
{
  "sessions": [
    {
      "id": "uuid-xxx",
      "instruction": "You are a helpful...",
      "created_at": "2026-04-19T10:00:00Z",
      "last_active_at": "2026-04-19T10:05:00Z"
    }
  ]
}
```

---

## 4. 会话消息历史 — GET /agent/sessions/:id/messages

获取指定会话的完整对话记录。

### 响应

```json
{
  "session_id": "uuid-xxx",
  "messages": [
    { "role": "user", "content": "请解释这段代码" },
    { "role": "assistant", "content": "这段代码的作用是..." }
  ]
}
```

---

## 前端对接示例

```javascript
function chat(sessionId, message, articleCtx) {
  const body = {
    session_id: sessionId || '',
    user_message: message,
  }
  if (articleCtx) body.article_ctx = articleCtx

  const response = await fetch('/agent/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      // 'X-Agent-API-Key': 'sk-xxx',  // 公共博客系统需要
    },
    body: JSON.stringify(body),
  })

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop()

    let currentEvent = ''
    for (const line of lines) {
      if (line.startsWith('event: ')) {
        currentEvent = line.slice(7)
      } else if (line.startsWith('data: ')) {
        const data = JSON.parse(line.slice(6))
        handleEvent(currentEvent, data)
      }
    }
  }
}

function handleEvent(event, data) {
  switch (event) {
    case 'session_created':
      // 保存 data.session_id
      break
    case 'stream_chunk':
      // 追加 data.content 到回复气泡
      break
    case 'message':
      // 展示完整消息 data.content
      break
    case 'tool_result':
      // 展示工具执行结果 data.content
      break
    case 'interrupt':
      // 展示代码预览 data.proposal，等待用户确认
      break
    case 'error':
      // 展示错误 data.error
      break
    case 'done':
      // 对话结束
      break
  }
}
```
