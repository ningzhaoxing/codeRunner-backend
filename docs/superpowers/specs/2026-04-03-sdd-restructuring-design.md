# CodeRunner SDD 重构设计方案

> 基于《AI 新时代的 SDD：上下文即真理与驾驭工程》理念，建立完整的 Spec-Driven Development 工作流
> 
> 设计日期：2026-04-03

---

## 一、设计目标

将 CodeRunner 项目从"文档散落、AI 协作低效"的状态，重构为完整的 SDD 工作流体系：

1. **建立 CLAUDE.md 总纲** — AI 获得项目全局理解的唯一入口
2. **建立上下文文档体系** — PRD / 技术方案 / TestPlan 三层文档作为 AI 的"真相来源"
3. **建立 Agent 工作流** — PRD Agent / 技术方案 Agent / TestPlan Agent 标准化文档产出
4. **整理现有文档** — 将散落的技术方案、issues 纳入新体系

**核心原则：**
- 上下文即真理 — 不在文档里的东西对 AI 不存在
- 文档驱动代码 — 先有 PRD/方案/TestPlan，再写代码
- Agent 驱动文档 — 用 Agent 产出文档初稿，人审阅修正
- 人驾驭流程 — 人审文档、审节点、管熵增

---

## 二、目标目录结构

```
coderunner/
├── CLAUDE.md                              # 项目总纲（英文）
│
├── docs/
│   ├── context/                           # 上下文文档体系
│   │   ├── requirements/                  # PRD / 需求文档
│   │   │   └── (待后续用 PRD Agent 产出)
│   │   │
│   │   ├── designs/                       # 技术方案
│   │   │   ├── container-pool-design.md          # ← 原 容器池技术方案.md
│   │   │   ├── websocket-reliable-delivery-design.md  # ← 原 webSocket 请求可靠投递.md
│   │   │   └── architecture-roadmap.md    # ← 原 架构优化迭代方案.md（路线图，非单功能方案）
│   │   │
│   │   ├── test-plans/                    # TestPlan / 验收标准
│   │   │   └── (待后续用 TestPlan Agent 产出)
│   │   │
│   │   └── decisions/                     # ADR（架构决策记录，可选）
│   │       └── (后续需要时添加)
│   │
│   ├── agents/                            # Agent 工作流定义
│   │   ├── prd-agent.md                   # PRD Agent
│   │   ├── design-agent.md                # 技术方案 Agent
│   │   └── testplan-agent.md              # TestPlan Agent
│   │
│   ├── references/                        # 参考资料
│   │   └── issues.md                      # ← 原 docs/issues.md
│   │
│   └── superpowers/                       # 保留现有结构
│       ├── specs/
│       │   ├── 2026-03-28-agent-design.md
│       │   ├── 2026-03-29-code-learning-agent-prd.md
│       │   └── 2026-04-03-sdd-restructuring-design.md  # 本文档
│       └── plans/
│           └── 2026-03-28-code-learning-agent.md
│
├── internal/  ...                         # 源码不动
├── api/  ...
├── builds/  ...
└── ...
```

**设计理由：**
- **按文档类型组织**（而非按功能域）— 与 SDD 工作流直接对应，Agent 产出路径可固化
- **context/ 作为核心** — 所有驱动 AI Coding 的文档都在这里
- **agents/ 独立** — Agent 定义是元数据，不是上下文
- **references/ 存放辅助资料** — issues.md 是问题追踪，不是规格文档

---

## 三、文档迁移清单

| 原路径 | 新路径 | 处理方式 |
|--------|--------|----------|
| `docs/容器池技术方案.md` | `docs/context/designs/container-pool-design.md` | 移动 + 补充 frontmatter |
| `docs/webSocket 请求可靠投递.md` | `docs/context/designs/websocket-reliable-delivery-design.md` | 移动 + 补充 frontmatter |
| `docs/架构优化迭代方案.md` | `docs/context/designs/architecture-roadmap.md` | 移动 + 补充 frontmatter |
| `docs/issues.md` | `docs/references/issues.md` | 移动 |
| `docs/reliable-delivery.md` | 删除 | 已在 git 中标记删除，确认 |
| `docs/superpowers/` | `docs/superpowers/`（不动） | 保留 |

**frontmatter 统一格式：**

```markdown
---
title: 文档标题
type: design | requirement | testplan
status: draft | review | approved | deprecated
created: YYYY-MM-DD
updated: YYYY-MM-DD
related:
  - docs/context/requirements/xxx-prd.md
  - docs/context/test-plans/xxx-testplan.md
---
```

---

## 四、CLAUDE.md 设计

CLAUDE.md 使用英文撰写（AI 理解效率更高），作为整个项目的 Spec 总纲。

### 内容板块

**1. Project Overview（~10 行）**
- 项目定位：分布式代码执行系统，为博客平台提供在线代码运行能力
- 架构模式：DDD 四层（interfaces → application → domain → infrastructure）
- 通信协议：gRPC（外部调用）+ WebSocket（Server↔Worker 双向通信）+ HTTP Callback（异步结果回调）
- 技术栈：Go 1.23, Gin, gRPC, gorilla/websocket, Docker SDK, ETCD, Redis, Zap, Prometheus
- 双模式运行：Server 模式（云端任务编排）/ Client 模式（节点代码执行）

**2. System Architecture（~15 行）**
- 核心调用链描述
- 关键组件及其在 DDD 层中的位置
- 支持的语言运行环境（Go, Python, JavaScript, Java, C++）

**3. Code Conventions（~15 行）**
- DDD 分层规则：domain 层不依赖 infrastructure，application 层编排 domain 服务
- 错误处理：使用 `internal/infrastructure/common/errors` 定义的错误类型
- 日志：统一使用 Zap（`internal/infrastructure/common/logger`），禁止 logrus
- Proto 变更：修改 `.proto` 后必须重新生成 `.pb.go`
- Commit 风格：conventional commits（feat/fix/docs/refactor/test）

**4. Documentation Map（~10 行）**
- 索引指向 `docs/context/` 各子目录
- 索引指向 `docs/agents/` Agent 定义
- 索引指向 `docs/references/` 参考资料

**5. Development Workflow（~10 行）**
- 新功能开发流程：PRD Agent → 技术方案 Agent → TestPlan Agent → 编码
- 配置文件：`configs/dev.yaml`（开发）/ `configs/product.yaml`（生产）
- 构建：`builds/` 目录下的 Dockerfile
- 编排：`docker-compose.yml` + `docker-compose/` 环境配置

### 设计原则

1. **不承载细节** — 每个板块 10-20 行，细节通过索引指向深层文档
2. **渐进式信息披露** — AI 先读 CLAUDE.md 获得全局理解，按需深入
3. **可机器解析** — 索引路径用明确的相对路径
4. **编码规范从现有代码提炼** — 不凭空发明规则

---

## 五、Agent 工作流定义

### 5.1 PRD Agent（`docs/agents/prd-agent.md`）

**职责：** 从业务需求或用户故事产出结构化 PRD

**输入：**
- 业务背景描述（口头或文字形式）
- `CLAUDE.md`（项目上下文）
- 相关的现有功能代码（可选，用于理解现状）

**输出模板：**

```markdown
---
title: {功能名称} PRD
type: requirement
status: draft
created: YYYY-MM-DD
---

## 背景与目标
（为什么做这个功能，解决什么问题）

## 用户场景
（典型使用场景描述）

## 功能清单
| 编号 | 功能点 | 描述 | 验收条件 | 优先级 |
|------|--------|------|----------|--------|
| F1   | ...    | ...  | ...      | Must   |

## 非功能需求
（性能、安全、兼容性要求）

## 约束与依赖
（技术约束、外部依赖、前置条件）

## 不做什么（边界声明）
（明确排除在范围外的内容）

## 开放问题
（待确认的事项）
```

**质量标准：**
- 每个功能点必须有可验证的验收条件
- 必须包含"不做什么"的边界声明
- 优先级使用 MoSCoW（Must/Should/Could/Won't）
- 必须引用 CLAUDE.md 中的技术约束

**产出路径：** `docs/context/requirements/{feature-name}-prd.md`

---

### 5.2 技术方案 Agent（`docs/agents/design-agent.md`）

**职责：** 从 PRD 产出技术设计方案

**输入：**
- 对应的 PRD 文档
- `CLAUDE.md`（项目架构、编码规范）
- 现有相关代码（自动检索）

**输出模板：**

```markdown
---
title: {功能名称} 技术方案
type: design
status: draft
created: YYYY-MM-DD
related:
  - docs/context/requirements/{name}-prd.md
---

## 现状分析
（当前架构中与本功能相关的现状和痛点）

## 设计目标
（本方案要达成的技术目标，与 PRD 目标对应）

## 方案设计

### 核心数据结构
（具体到字段级别的结构定义）

### 接口定义
（新增/修改的接口，含请求响应格式）

### 调用链
（完整的请求处理流程，用文本流程图表示）

### 配置变更
（新增/修改的配置项）

## 替代方案对比
| 方案 | 优点 | 缺点 | 淘汰理由 |
|------|------|------|----------|

## 风险评估与缓解
| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|

## 实施步骤
（可拆分为独立 PR 的步骤列表）

## 对现有功能的影响
（已有功能会受到什么影响，是否需要兼容处理）
```

**质量标准：**
- 必须包含至少 1 个替代方案及淘汰理由
- 数据结构和接口定义必须具体到字段级别
- 实施步骤必须可拆分为独立 PR
- 必须评估对现有功能的影响

**产出路径：** `docs/context/designs/{feature-name}-design.md`

**与现有 skill 的关系：**
- `docs/agents/design-agent.md` 定义"做什么"（输入、输出、质量标准）
- `.claude/skills/agent-tech-spec/` 定义"怎么调用 Claude Code 来做"
- 两者互补，不冲突

---

### 5.3 TestPlan Agent（`docs/agents/testplan-agent.md`）

**职责：** 从 PRD + 技术方案产出验收标准和测试计划

**输入：**
- 对应的 PRD 文档
- 对应的技术方案文档
- `CLAUDE.md`（测试规范）

**输出模板：**

```markdown
---
title: {功能名称} TestPlan
type: testplan
status: draft
created: YYYY-MM-DD
related:
  - docs/context/requirements/{name}-prd.md
  - docs/context/designs/{name}-design.md
---

## Must Have（不通过不算完成）
- [ ] 具体的验收条件 1（可机械验证）
- [ ] 具体的验收条件 2
...

## Should Have（重要但不阻塞发布）
- [ ] ...

## Nice to Have（有则更好）
- [ ] ...

## Redline（绝对不能做的事）
- 禁止项 1（可自动检查）
- 禁止项 2
...

## 测试策略

### 单元测试
（需要覆盖的核心函数/方法）

### 集成测试
（端到端场景描述）

### 性能测试（如适用）
（基准指标和测试方法）
```

**质量标准：**
- Must Have 条目必须可机械验证（能写成测试断言）
- Redline 条目必须可自动检查（能写成 lint 规则或测试）
- 每个测试场景要包含正常路径和异常路径
- Must Have 条目必须与 PRD 的验收条件一一对应

**产出路径：** `docs/context/test-plans/{feature-name}-testplan.md`

---

### Agent 间上下游关系

```
用户输入业务需求
    ↓
PRD Agent → docs/context/requirements/{name}-prd.md
    ↓（人审阅修正）
技术方案 Agent → docs/context/designs/{name}-design.md
    ↓（人审阅修正）
TestPlan Agent → docs/context/test-plans/{name}-testplan.md
    ↓（人审阅修正）
AI Coding（上下文 = CLAUDE.md + 以上三份文档）
    ↓（Code Review）
验收（对照 TestPlan Must Have 逐项检查）
```

---

## 六、SDD 完整工作流

### 新功能开发流程

以"容器池"功能为例：

**Step 1: PRD Agent 产出需求**
```
输入: "我需要一个容器池来消除冷启动延迟"
产出: docs/context/requirements/container-pool-prd.md
人审: 确认功能范围、优先级、验收条件
```

**Step 2: 技术方案 Agent 产出设计**
```
输入: container-pool-prd.md + CLAUDE.md
产出: docs/context/designs/container-pool-design.md
人审: 确认架构决策、接口设计、风险评估
```

**Step 3: TestPlan Agent 产出验收标准**
```
输入: container-pool-prd.md + container-pool.md
产出: docs/context/test-plans/container-pool-testplan.md
人审: 确认验收条件、Redline
```

**Step 4: AI Coding**
```
上下文: CLAUDE.md 索引 → 自动读取以上三份文档
产出: 代码实现
人审: Code Review（重点看架构变更和安全相关）
```

**Step 5: 验收**
```
对照 TestPlan 的 Must Have 逐项检查
确认 Redline 没有违反
```

### Harness Engineering 实践

**造马具（迭代文档体系）：**
- 定期回顾 Agent 产出质量，调整 agent.md 模板和规则
- 发现系统性偏差时修改对应 agent.md 的质量标准
- 每使用 3-5 次后回顾一次，持续打磨

**拉缰绳（审阅关键节点）：**
- PRD 审阅：功能范围、优先级
- 技术方案审阅：架构决策、接口设计
- TestPlan 审阅：验收条件、Redline 完整性
- Code Review：架构变更和安全相关

**清马道（管理熵增）：**
- 每完成一个功能后更新 CLAUDE.md 索引
- 定期清理 `docs/references/issues.md`
- 代码库出现技术债时更新 CLAUDE.md 规范

### CLAUDE.md 维护节奏

| 时机 | 更新内容 |
|------|----------|
| 新功能开发完成 | 更新 Documentation Map 索引 |
| 技术栈变更 | 更新 Tech Stack、Code Conventions |
| 新编码规范确立 | 更新 Code Conventions |
| 新 Agent 定义添加 | 更新 Development Workflow |

---

## 七、实施计划概要

| 步骤 | 内容 | 产出 | 验收标准 |
|------|------|------|----------|
| 1 | 创建目录结构 | `docs/context/`, `docs/agents/`, `docs/references/` | 目录存在且为空（或含 .gitkeep） |
| 2 | 迁移现有文档 | 三份技术方案移动到 `docs/context/designs/`，issues 移动到 `docs/references/` | 原路径文件不存在，新路径文件内容完整，frontmatter 已补充 |
| 3 | 编写 CLAUDE.md | 项目总纲（英文） | 包含 5 个板块，索引路径可正确解析 |
| 4 | 编写 PRD Agent 定义 | `docs/agents/prd-agent.md` | 包含输入/输出模板/质量标准/产出路径 |
| 5 | 编写技术方案 Agent 定义 | `docs/agents/design-agent.md` | 包含输入/输出模板/质量标准/产出路径 |
| 6 | 编写 TestPlan Agent 定义 | `docs/agents/testplan-agent.md` | 包含输入/输出模板/质量标准/产出路径 |
| 7 | 验证工作流 | 用"结果缓存"功能试跑一遍完整 SDD 流程 | PRD → 技术方案 → TestPlan 三份文档产出，格式符合模板 |
