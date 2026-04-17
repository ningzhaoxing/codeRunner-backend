---
name: agent-tech-spec
description: Agent 技术方案文档编写指南 - 基于 Eino ReAct + Skill Middleware 架构生成标准化 Agent MVP 实现文档
version: 1.0.0
---

# Agent 技术方案文档编写指南

> 基于报告导出 Agent MVP 实现文档的风格，生成 Eino ReAct + Skill Middleware 架构下的 Agent 技术方案文档。

**触发条件**：
- "写 Agent 技术方案"、"编写 Agent 实现文档"
- "Agent MVP 方案"、"Agent 技术文档"

---

## 一、使用前提

编写前必须准备好以下输入：

| 输入 | 来源 | 说明 |
|------|------|------|
| Agent 功能定位 | PRD / 需求讨论 | 这个 Agent 做什么、不做什么 |
| 核心用户场景 | PM | 2-3 个端到端场景描述 |
| 涉及的数据模型 | 后端代码 / 数据库 | 需要查询/写入哪些表 |
| 现有基础设施 | 代码仓库 | 可复用的组件（SmartMemory、SSE、Handler 模式等） |

---

## 二、文档结构模板

严格按以下章节顺序编写，每个章节的写法规范见后文。

```
# {Agent名称} — MVP 实现方案

> **状态**：设计草案 · **日期**：{YYYY-MM-DD}

## 1. 概述
## 2. 架构
## 3. Skill 设计
## 4. 业务字段说明（如有 Mapping 类 Skill）
## 5. 数据查询 Skill 实现复杂性说明（如有查询类 Skill）
## 6. {核心推理逻辑}（如 Mapping 推理、意图识别等）
## 7. 上下文管理（SmartMemory）
## 8. 用户意图澄清（如有多轮交互）
## 9. SSE 事件机制
## 10. 文件交付（如有文件输出）
## 11. 关键技术决策
## 12. 迭代计划
```

非所有章节必写——根据 Agent 特征裁剪。但章节顺序和编号风格保持一致。

---

## 三、各章节写法规范

### 3.1 概述（Section 1）

**必须包含：**

1. **一句话定位**：这个 Agent 是什么（独立 Go Agent / 子 Agent），复用哪些基础设施
2. **核心流程**：编号列表，描述从用户触发到最终交付的完整步骤（5-8 步）
3. **关键约束**：用 `> **关键约束**：` blockquote 格式，写明最重要的数据关系或业务规则
4. **MVP / Post-MVP 边界**：明确写出 MVP 包含什么、不包含什么
5. **文件结构**：用 `tree` 格式展示目录结构，每个文件用 `#` 注释说明用途

**风格要点：**
- 核心流程中每步的主语是"用户"或"Agent"，不要写"系统"
- 文件结构中脚本文件名用 snake_case，Skill 目录名用 kebab-case
- 注释写在同一行的 `#` 后面，保持对齐

**示例：**

```text
tara/service/agent/{agent_name}/
├── agent.go                         # {Agent}初始化与 Stream() 入口
├── system_prompt.txt                # Agent 系统提示词（Bootstrap）
└── skills/
    ├── {skill-name}/
    │   ├── SKILL.md
    │   └── scripts/
    │       ├── {script_name}.py     # {功能描述}（{N}列）
```

---

### 3.2 架构（Section 2）

**必须包含：**

1. **技术选型表**：`| 组件 | 选型 |` 格式，列出 Agent 框架、Skill 运行时、记忆系统、流式输出、数据访问等
2. **架构图**：用 ASCII art 画出从 HTTP Handler 到 SSE 输出的完整链路

**架构图规范：**
- 用 `├──` / `└──` 表示层级
- 每个组件用括号标注框架来源，如 `（Eino v0.8）`
- SSEWriter 作为最终汇聚点
- 中间件钩子用缩进 + 事件名列表表示

---

### 3.3 Skill 设计（Section 3）

**每个 Skill 的描述格式：**

```markdown
### Skill {N}：`{skill-name}`

{一段话描述该 Skill 的职责和在流程中的位置}

\`\`\`text
Use when: {触发条件，用分号分隔多个场景}
NOT for: {明确排除的场景}
\`\`\`

脚本：

- `{script_name}.py {--参数说明}`
  {一句话功能描述}（{N}列）
  输出：`{ "key": "value", ... }`
```

**写法规则：**

1. **Use when / NOT for 必写**：这是 Skill 发现阶段 LLM 判断是否激活的关键依据
2. **脚本参数格式统一**：`--param-name <type>`，参数名 kebab-case
3. **输出示例必写**：至少写第一个脚本的完整输出 JSON 格式
4. **列数标注**：每个查询脚本在注释中标明返回多少列，如 `（6列）`
5. **双输出模式**（查询类 Skill）：如果脚本同时服务 Filling 和 Q&A，必须说明 `--output jsonl` / `--output summary` 两种模式的参数和返回值差异
6. **Skill 间分隔**：每个 Skill 之间用 `---` 水平线分隔
7. **Skill 编号连续**：Skill 1、Skill 2、...，与文件结构中的顺序一致

**Skill 分类参考：**

| 类型 | 特征 | 示例 |
|------|------|------|
| 操作类 | 对外部资源进行增删改 | excel-file-operations、manage-field-mappings |
| 知识包类 | SKILL.md 本身是核心价值，含领域知识 + 推理策略 | manage-field-mappings（Mapping 知识包） |
| 查询类 | 从数据库查数据，支持双输出模式 | query-asset-data、query-damage-data |

---

### 3.4 业务字段说明（Section 4）

**仅当 Agent 涉及"将外部输入映射到内部字段"时才需要。**

**格式：** 按领域分组，每组一个表格：

```markdown
### {领域名}（`{表名}`）

| 字段名 | 类型 | 语义描述 |
| --- | --- | --- |
| `field_name` | string | {描述}；{枚举值（如有）} |
```

**规则：**
- 枚举值写中文展示值（DB 存储代码值由脚本翻译）
- 字段名用 snake_case
- 说明这些字段信息写入哪个 Skill 的 SKILL.md（通常是知识包类 Skill）

---

### 3.5 数据查询复杂性说明（Section 5）

**仅当 Agent 包含多个查询脚本、涉及多表 JOIN 时才需要。**

**必须涵盖的子章节：**

1. **数据层级展开**：说明数据模型的嵌套层数，画出层级树，列表说明每个脚本的展开层数和输出行语义
2. **空值边界处理**：中间层为空时的行为约定（内连接 / 左连接 / 占位行）
3. **各脚本 JOIN 结构差异**：每个脚本的 JOIN 表数和策略
4. **字段归一化与枚举翻译**：DB 列名 → 输出 key 的命名风格、枚举代码值 → 中文展示值
5. **Data Sidecar 模式**：大数据不经过 LLM 的处理机制（JSONL 文件 + data_ref 元数据）

**用表格汇总各脚本的展开层数和 JOIN 复杂度。**

---

### 3.6 核心推理逻辑（Section 6）

**描述 Agent 中纯 LLM 推理（不调用工具）的关键环节。**

**必须包含：**
- 推理的输入（LLM 此时看到什么）
- 推理的输出（LLM 产出什么）
- 哪些信息从哪个 Skill 加载
- 歧义/错误处理策略

---

### 3.7 上下文管理（Section 7）

**固定结构：**

1. SmartMemory 配置代码块（Go）
2. **注入方式**：说明 `MessageModifier` 机制，消息序列为 `[system prompt] + [历史对话] + [当前用户消息]`
3. **跨轮次恢复**：描述 Load → MessageModifier inject → Persist 的三步循环

---

### 3.8 用户意图澄清（Section 8）

**仅当 Agent 有多轮交互场景时才需要。**

**固定结构：**

1. **触发条件**：什么情况下 LLM 输出问句
2. **ReAct 终止与恢复**：说明"LLM 输出文本且无 tool_call → ReAct Loop 自然结束 → 新 Run() 恢复"的机制
3. **SSE 事件分类**：`AfterChatModel` 如何区分 `clarification_required` / `final_answer`

---

### 3.9 SSE 事件机制（Section 9）

**固定结构：**

1. **两个来源**：Eino 框架事件（AsyncIterator）+ 业务自定义事件（ChatModelAgentMiddleware 钩子）
2. **事件注入点表**：`| 事件类型 | 注入方式 | 机制 |`
3. **事件字段映射表**：`| 事件 | 业务数据 → SSE 字段 |`

---

### 3.10 文件交付（Section 10）

**仅当 Agent 有文件输出时才需要。**

**必须包含：**
- 文件生成 → SSE 通知 → 下载端点的完整链路
- Go 下载端点的路由和鉴权逻辑
- 数据库新增字段（如 `output_path`）

---

### 3.11 关键技术决策（Section 11）

**每个决策的格式：**

```markdown
### 决策 {N}：{决策标题}

**结论**：{一句话结论}

**原因**：

- {原因 1}
- {原因 2}
- {原因 3}
```

**什么应该写成决策：**
- 在方案设计中做出的非显而易见的取舍（"为什么不合并"、"为什么选 A 不选 B"）
- 对后续开发有约束力的架构选择
- 容易被质疑或遗忘理由的关键判断

**决策编号可以不连续**（允许跳号，因为有些决策可能在后续迭代中新增）。

---

### 3.12 迭代计划（Section 12）

**原则：纵向切片，每个迭代交付可端到端测试的能力。**

**每个迭代的格式：**

```markdown
### Iter {N}：{迭代名称}（~{天数}d）

**目标**：{一句话目标}

**范围** / **在 Iter {N-1} 基础上新增**：
- {具体交付物列表}

**不做**：{明确排除项}

**验证**：{端到端验证场景描述}
```

**规则：**
- Iter 0 必须是"最窄端到端通路"，用最简单的输入走通全链路
- 每个迭代的"验证"必须是可操作的场景描述，不是抽象目标
- 最后一个迭代用"收尾加固"收尾：边界处理、单测、Prompt 调优
- 末尾用 `| 迭代 | 交付物 | 累计可用能力 | 预估 |` 总览表汇总

---

## 四、写作风格规范

### 4.1 语言风格

| 规则 | 正确 | 错误 |
|------|------|------|
| 主语明确 | "Agent 调用脚本" | "系统进行处理" |
| 动词直接 | "写入"、"返回"、"查询" | "进行写入操作"、"完成返回流程" |
| 避免冗余修饰 | "LLM 只接收元数据" | "LLM 只需要接收简洁的元数据信息" |
| 用代码格式引用 | `template_id`、`SKILL.md` | template_id、SKILL.md |

### 4.2 格式规范

- **表格**：表头用 `| --- |` 分隔，列内容左对齐
- **代码块**：SQL 用 `sql`、JSON 用 `json`、目录结构用 `text`、Go 代码用 `go`
- **引用块**：`> **关键约束**：` 用于强调重要业务规则
- **水平线**：`---` 用于 Skill 之间分隔、章节之间分隔
- **脚本参数**：`--param-name <type>` 格式，参数名 kebab-case
- **字段名**：snake_case，用反引号包裹
- **Skill 目录名**：kebab-case
- **脚本文件名**：snake_case + `.py`

### 4.3 数据描述规范

- 查询脚本必须标明列数：`（6列）`、`（9列）`
- 输出 JSON 示例用内联代码或代码块，key 为 snake_case
- 展开关系用 `×` 表示笛卡尔积：`一个资产 × 一个威胁 × 一个攻击路径 = 一行`
- JOIN 复杂度用 `N 表 JOIN` 描述

### 4.4 关键约定

- **脚本不经过 LLM 传递大数据**：所有超过 ~50 行的数据必须走 Data Sidecar（JSONL 文件），LLM 只看到 `data_ref` + `total` + `columns`
- **Skill 之间通过 LLM 上下文传递控制信息**（如 `template_id`、`data_ref`），不通过脚本间直接调用
- **`session_id` 和 `project_id` 来自 Handler 注入**，不让用户手动提供
- **幂等设计**：保存类脚本（Mapping 保存、文件登记）必须说明幂等策略

---

## 五、参考文档

编写时应查阅以下文档获取架构细节：

| 文档 | 路径 | 用途 |
|------|------|------|
| Eino Skill 机制 | `tara/Eino 框架技术调研/Skill 机制.md` | Skill Middleware 原理、FrontMatter 结构、渐进式披露 |
| Skill vs Tool 分析 | `tara/Agent 调研/skill vs tool 本质区别分析.md` | 何时用 Skill、何时用 Tool |
| Agent 重构指南 | `AIcanTARA_PM_Context/04_workspace/agent-redesign-guide/SKILL.md` | PRD 整理框架、MCP vs Skill 决策矩阵 |
| Eino ADK v0.8 调研 | `AIcanTARA_PM_Context/05_drafts/.../06_Eino_ADK_v0.8.0-beta_Skill_Middleware原生能力调研.md` | 框架原生能力边界 |
| 报告导出 MVP 文档 | `tara/service/agent/ai_report_export/报告导出 MVP 实现文档.md` | 风格参考标杆 |
