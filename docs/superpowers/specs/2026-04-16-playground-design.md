---
title: Playground 代码编辑器页面设计
type: design
status: draft
created: 2026-04-16
---

# Playground 代码编辑器页面设计

## 一、概述

在博客中新增独立的 `/playground` 页面，提���全屏代码编辑 + 运行体验。用户可以自由编写代码、切换语言、运行查看结果，并可选展开 AI 助手。支持从文章代码块一键跳转到 Playground 继续编辑，支持通过 URL 分享代码。

### 定位

单文件 Playground（类似 Go Playground / JSFiddle），架构上预留多文件扩展能力。

### 与文章代码块的区别

| 对比 | 文章代码块 | Playground |
|---|---|---|
| 编辑器高度 | 自适应 80-300px | 全屏，占满可用空间 |
| 代码持久化 | 不持久化 | localStorage，按语言独立存储 |
| 语言 | 由 Markdown 决�� | 用户可切换 |
| AI 上下文 | 有文章上下文 | 无文章上下文，纯代码模式 |
| 分享 | 无 | URL 编码分享 |

---

## 二、页面布局

### 默认状态

```
┌────────────────────────────────────────────────────────┐
│ Nav: CodeRunner Blog  [Playground]  首页 标签 关于      │
├────────────────────────────────────────────────────────┤
│ 工具栏: [Go ▾]  main.go          [▶ Run] [🤖 AI] [🔗] │
├──────────────────────���─────────────────────────────────┤
│                                                        │
│  Monaco Editor (全高)                                   │
│                                                        │
├───────���────────────────────────────────────────────────┤
│ Output                                          0.12s  │
│ > Hello, World!                                        │
├────────────────────────────────────────────────────────┤
│ 状态栏: Go | 128 chars | 7 lines | 已保存              │
└────────────────────────────────────────────────────────┘
```

### AI 面板展开状态

```
┌──────────────────────────────┬─────────────────────────┐
│ [Go ▾] main.go  [▶ Run] [🔗]│ 🤖 AI 助手 [解释][调试][测试]│
├──────────────────────────────┤─────────────────────────┤
│                              │                         │
│  Monaco Editor (flex: 6)     │  对话消息区               │
│                              │                         │
├──────────────────────────────┤                         │
│ Output               0.12s  │  输入框                  │
├──────────────────────────────┴─────────────────────────┤
│ 状态栏                                                 │
└────────────────────────────────────────────────────────┘
```

---

## 三、状态管理

### Zustand Store — `usePlaygroundStore`

独立于 `usePostStore`，Playground 有自己的生命周期和状态结构。

```typescript
interface PlaygroundStore {
  // 编辑器状态
  language: string;
  code: string;
  output: string | null;
  runError: string | null;
  isRunning: boolean;

  // AI 面板状态
  isAIOpen: boolean;
  aiMessages: ChatMessage[];
  sessionId: string | null;
  isStreaming: boolean;
  proposals: Record<string, Proposal>;

  // Actions
  setLanguage: (lang: string) => void;  // 切换语言（自动保存当前、加载目标）
  setCode: (code: string) => void;      // 更新代码（自动同步 localStorage）
  setOutput: (output: string | null, error: string | null) => void;
  setRunning: (running: boolean) => void;
  toggleAI: () => void;
  addAIMessage: (message: ChatMessage) => void;
  updateLastAIMessage: (content: string) => void;
  setSessionId: (id: string) => void;
  setStreaming: (streaming: boolean) => void;
  addProposal: (proposal: Proposal) => void;
  updateProposalStatus: (id: string, status: Proposal["status"]) => void;
  initFromURL: (lang: string, code: string) => void;  // URL 参数初始化
  loadFromStorage: () => void;  // localStorage 初始化
}
```

### localStorage 结构

```typescript
// key: "playground-state"
interface PlaygroundStorage {
  version: 1;  // schema 版本号，未来迁移用
  activeLanguage: string;
  files: Record<string, {
    code: string;
    lastModified: number;
  }>;
}
```

读取 localStorage 时检查 `version`，缺失或不匹配则丢弃旧数据，用默认值重建。

### 初始化优先级

```
1. URL 有 ?code=xxx&lang=go → 解码使用 URL 参数
2. localStorage 有保存的数据 → 恢复上次状态
3. 都没有 → Go 语言 + Hello World 默认模板
```

**URL 参数与 localStorage 的关系**：URL 参数初始化编辑器但不立即写入 localStorage。只有当用户在编辑器中修改代码后，才会触发 localStorage 保存。页面刷新时，若 URL 参数仍在，继续使用 URL 参数。

**localStorage 写入策略**：`setCode` 触发的 localStorage 写入使用 500ms 防抖，避免每次按键都写入。StatusBar 的"已保存"指示器在防抖完成后显示。

**浏览器历史**：从文章跳转到 Playground 使用 `router.push`（保留 Back 返回文章）。Playground 内部的语言切换使用 `router.replace`（不污染历史栈）。

---

## 四、URL 分享

### 编码方案

```
/playground?lang=go&code=eJxLSS0u0cvIL0pVSMsvyklRBAAgVgVN
```

- 编码：使用 `pako.deflateRaw` 压缩后再 Base64url 编码（与 Go Playground 同方案），有效缩短 URL
- 解码：Base64url 解码后 `pako.inflateRaw`
- 降级：若 pako 加载失败，fallback 到 `btoa(unescape(encodeURIComponent(code)))`
- 点击 🔗 分享按钮 → 生成 URL → 复制到剪贴板 → toast "链接已复制"
- 压缩后 URL 仍超过 2KB → toast "代码过长，建议手动复制分享"
- URL code 参数解码失败（被截断、手动篡改等）→ 忽略 code 参数，使用 localStorage 或默认模板，toast "分享链接已损坏"

依赖：`npm install pako`（~45KB gzipped，轻量）

### 从文章跳转

文章代码块 `CodeBlockHeader` 新增按钮 `↗`（tooltip: "在 Playground 中打开"）：

```
[▶ Run] [🤖] [⤢] [↗]
```

点击后：`router.push(/playground?lang=${language}&code=${encode(code)})`

跳转时使用编辑器中的**当前代码**（可能已被用户修改），不是原始代码。

---

## 五、默认模板

每种语言有一个 Hello World 模板，存储在 `src/lib/templates.ts`：

| 语言 | 文件名 | 模板 |
|---|---|---|
| go | main.go | `package main` + `fmt.Println("Hello, World!")` |
| python | main.py | `print("Hello, World!")` |
| javascript | index.js | `console.log("Hello, World!");` |
| java | Main.java | `public class Main { public static void main(...) }` |
| c | main.c | `#include <stdio.h>` + `printf("Hello, World!\n")` |

### 语言切换流程

```
用户选择新语言
  → 保存当前代码到 localStorage files[currentLang]
  → 查找 localStorage files[targetLang]
    有 → 加载保存的代码
    无 → 加载默认模板
  → 更新 Monaco Editor 语言 + 代码 + 文件名
  → 清空 Output
  → AI 消息处理：
    - 若 AI 消息为空 → 静默清空 sessionId
    - 若 AI 消息非空 → 清空 AI 消息和 sessionId，显示 toast "AI 对话已清空"
  （新语言是新上下文，有意为之，用 toast 提示而非弹窗确认）
```

---

## 六、组件设计

### 新建组件

| 组件 | 文件 | 职责 |
|---|---|---|
| `PlaygroundToolbar` | `src/components/PlaygroundToolbar.tsx` | 工具栏：语言下拉 + 文件名 + Run/AI/分享按钮 |
| `LanguageSelector` | `src/components/LanguageSelector.tsx` | 语言下拉选择器（5 种语言） |
| `StatusBar` | `src/components/StatusBar.tsx` | 底部状态栏：语言、字符数、行数、保存状态 |

### 复用组件

| 组件 | 复用方式 |
|---|---|
| Monaco Editor (`@monaco-editor/react`) | 直接复用，高度改为 `100%` |
| `OutputPanel` | 直接复用 |
| `AIPanel` | 复用，`articleId=""`，`articleContent=""`，`allCodeBlocks` 只含当前代码。AIPanel 的 props 已定义为可选，空字符串合法，无需接口修改 |
| `ChatMessages` / `ChatInput` / `CodeSuggestion` | 完全复用 |

Playground 传给 AIPanel 的 `allCodeBlocks` 格式：`[{ block_id: "playground", language: currentLang, code: currentCode }]`

### 修改已有组件

| 组件 | 修改内容 |
|---|---|
| `Navbar` | 添加 "Playground" 导航链接 |
| `CodeBlockHeader` | 添加 "↗" 按钮（在 Playground 中打开） |

### 新建工具

| 文件 | 职责 |
|---|---|
| `src/lib/templates.ts` | 语言默认模板定义 |
| `src/lib/share.ts` | URL 编码/解码 + 剪贴板复制 |

### 新建页面

| 文件 | 说明 |
|---|---|
| `src/app/playground/page.tsx` | Playground 页面，client component |

---

## 七、键盘快捷键

| 快捷键 | 动作 |
|---|---|
| `Ctrl/Cmd + Enter` | 运行代码 |
| `Ctrl/Cmd + S` | 手动保存（实际已自动保存，显示 toast 反馈） |

---

## 八、AI 助手适配

Playground 的 AI 助手复用文章页的 `AIPanel` 组件，但上下文不同：

| 参数 | 文章页 | Playground |
|---|---|---|
| `articleId` | 文章 slug | `""` |
| `articleContent` | 文章 Markdown | `""` |
| `allCodeBlocks` | 文章所有代码块 | 仅当前编辑的代码（1 个 block） |

AI 面板在 Playground 中：
- 默认关闭，点击 🤖 按钮展开
- 展开时编辑器和 AI 面板左右分栏（flex 6:4）
- 切换语言时清空 AI 消息和 sessionId
- 快捷按钮同文章页：解释 / 调试 / 测试
- AIPanel 需新增 `mode` prop（`"article" | "playground"`），Playground 模式下后端 system prompt 不引用文章内容（纯代码分析）

---

## 九、错误处理与边界场景

| 场景 | 处理 |
|---|---|
| URL code 参数解码失败 | 忽略 code 参数，fallback 到 localStorage 或默认模板，toast "分享链接已损坏" |
| localStorage 数据损坏 / version 不匹配 | 丢弃旧数据，用默认值重建 |
| 代码执行超时（30s） | 取消请求，setRunning(false)，Output 显示 "执行超时" |
| 网络请求失败 | Output 显示 "网络错误，请重试" |
| 分享时 URL 过长（>2KB） | toast "代码过长，建议手动复制分享"，不生成链接 |
| 文件名与代码不一致（如 Java 类名修改） | 文件名纯展示用途，不影响执行。后端按语言决定执行方式 |
| 移动端访问 | MVP 不做移动端适配，Playground 页面在小屏设备上显示提示"建议在桌面端使用" |

---

## 十、键盘快捷键

| 快捷键 | 动作 |
|---|---|
| `Ctrl/Cmd + Enter` | 运行代码 |
| `Ctrl/Cmd + S` | 防止浏览器默认保存行为，显示 toast "已保存"（实际已自动保存） |

---

## 十一、文件结构总览

```
src/
  app/playground/
    page.tsx              # Playground 页面 (new)
  components/
    PlaygroundToolbar.tsx  # 工具栏 (new)
    LanguageSelector.tsx   # 语言选择器 (new)
    StatusBar.tsx          # 状态栏 (new)
    Navbar.tsx             # 加 Playground 链接 (modify)
    CodeBlockHeader.tsx    # 加"↗"按钮 (modify)
  store/
    usePlaygroundStore.ts  # Playground store (new)
  lib/
    templates.ts           # 语言模板 (new)
    share.ts               # URL 编解码 (new)
```

---

## 十二、不在本期范围

| 功能 | 原因 |
|---|---|
| 多文件项目 | 架构预留（files Record），MVP 只做单文件 |
| 代码片段库 / 历史记录 | 需要后端存储，复杂度高 |
| 协作编辑 | 需要 WebSocket + OT/CRDT，远超 MVP |
| 自定义主题切换 | 与博客主题切换一起做 |
| Output 面板拖拽调整高度 | 增加复杂度，MVP 用固定高度 160px |
