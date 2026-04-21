# SDD 重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the CodeRunner project to adopt Spec-Driven Development — CLAUDE.md as project index, organized context documents, and three core Agent workflow definitions.

**Architecture:** This is a docs-only restructuring. No source code changes. Create new directory structure under `docs/`, migrate existing scattered documents into it with standardized frontmatter, write CLAUDE.md as the project spec index (English), and define three Agent workflows (PRD/Design/TestPlan) in Chinese.

**Tech Stack:** Markdown, git

**Spec:** `docs/context/decisions/2026-04-03-sdd-restructuring.md`

**Deferred:** Workflow validation (spec section 七, step 7 — "用结果缓存功能试跑一遍完整 SDD 流程") is not included in this plan. It should be done as a separate follow-up after all structural changes are in place. Track in `docs/references/issues.md` if desired.

---

## File Structure

**New files to create:**
- `CLAUDE.md` — Project spec index (English)
- `docs/agents/prd-agent.md` — PRD Agent workflow definition
- `docs/agents/design-agent.md` — Design Agent workflow definition
- `docs/agents/testplan-agent.md` — TestPlan Agent workflow definition
- `docs/context/requirements/.gitkeep` — Placeholder for future PRDs
- `docs/context/test-plans/.gitkeep` — Placeholder for future TestPlans
- `docs/context/decisions/.gitkeep` — Placeholder for future ADRs

**Files to move:**
- `docs/容器池技术方案.md` → `docs/context/designs/container-pool-design.md`
- `docs/webSocket 请求可靠投递.md` → `docs/context/designs/websocket-reliable-delivery-design.md`
- `docs/架构优化迭代方案.md` → `docs/context/designs/architecture-roadmap.md`
- `docs/issues.md` → `docs/references/issues.md`

**Files to delete:**
- `docs/reliable-delivery.md` (already deleted in git)

**Files unchanged:**
- `docs/superpowers/**` — All existing superpowers docs stay in place
- `internal/**`, `api/**`, `builds/**`, `cmd/**` — No source code changes

---

### Task 1: Create directory structure

**Files:**
- Create: `docs/context/requirements/.gitkeep`
- Create: `docs/context/designs/` (will be populated by migration)
- Create: `docs/context/test-plans/.gitkeep`
- Create: `docs/context/decisions/.gitkeep`
- Create: `docs/agents/` (will be populated by agent definitions)
- Create: `docs/references/` (will be populated by migration)

- [ ] **Step 1: Create all new directories with .gitkeep placeholders**

```bash
mkdir -p docs/context/requirements docs/context/designs docs/context/test-plans docs/context/decisions docs/agents docs/references
touch docs/context/requirements/.gitkeep docs/context/test-plans/.gitkeep docs/context/decisions/.gitkeep
```

- [ ] **Step 2: Verify directory structure**

```bash
find docs/context docs/agents docs/references -type f -o -type d | sort
```

Expected output should show all 6 directories and 3 .gitkeep files.

- [ ] **Step 3: Commit**

```bash
git add docs/context docs/agents docs/references
git commit -m "docs: create SDD directory structure"
```

---

### Task 2: Migrate existing documents

**Files:**
- Move: `docs/容器池技术方案.md` → `docs/context/designs/container-pool-design.md`
- Move: `docs/webSocket 请求可靠投递.md` → `docs/context/designs/websocket-reliable-delivery-design.md`
- Move: `docs/架构优化迭代方案.md` → `docs/context/designs/architecture-roadmap.md`
- Move: `docs/issues.md` → `docs/references/issues.md`
- Delete: `docs/reliable-delivery.md` (confirm removal)

**Note:** Some of these files may be untracked. Use `mv` + `git add` instead of `git mv` to handle both tracked and untracked files.

- [ ] **Step 1: Record original line counts for integrity check**

```bash
wc -l "docs/容器池技术方案.md" "docs/webSocket 请求可靠投递.md" "docs/架构优化迭代方案.md" "docs/issues.md"
```

Record the output for later comparison.

- [ ] **Step 2: Move documents**

```bash
mv "docs/容器池技术方案.md" docs/context/designs/container-pool-design.md
mv "docs/webSocket 请求可靠投递.md" docs/context/designs/websocket-reliable-delivery-design.md
mv "docs/架构优化迭代方案.md" docs/context/designs/architecture-roadmap.md
mv docs/issues.md docs/references/issues.md
```

- [ ] **Step 3: Confirm docs/reliable-delivery.md deletion**

```bash
git rm docs/reliable-delivery.md 2>/dev/null || echo "Already removed"
```

- [ ] **Step 4: Add frontmatter to container-pool-design.md**

Prepend the following frontmatter to `docs/context/designs/container-pool-design.md` (insert before line 1):

```markdown
---
title: 容器池技术方案
type: design
status: draft
created: 2026-03-27
updated: 2026-04-02
related:
  - docs/context/designs/architecture-roadmap.md
---

```

Then update the internal relative link:

```bash
sed -i '' 's|\./架构优化迭代方案\.md|./architecture-roadmap.md|g' docs/context/designs/container-pool-design.md
```

- [ ] **Step 5: Add frontmatter to websocket-reliable-delivery-design.md**

Prepend the following frontmatter to `docs/context/designs/websocket-reliable-delivery-design.md` (insert before line 1):

```markdown
---
title: WebSocket 请求可靠投递 ACK 确认机制
type: design
status: approved
created: 2026-03-27
updated: 2026-03-27
---

```

- [ ] **Step 6: Add frontmatter to architecture-roadmap.md**

Prepend the following frontmatter to `docs/context/designs/architecture-roadmap.md` (insert before line 1):

```markdown
---
title: 架构优化迭代方案
type: design
status: approved
created: 2026-04-01
updated: 2026-04-01
---

```

- [ ] **Step 7: Verify content integrity**

```bash
# Compare line counts: migrated files should have original lines + frontmatter lines (8-10 lines added)
wc -l docs/context/designs/container-pool-design.md docs/context/designs/websocket-reliable-delivery-design.md docs/context/designs/architecture-roadmap.md docs/references/issues.md
```

container-pool-design.md should be ~357 lines (347 + 10 frontmatter), websocket ~178 (170 + 8), architecture ~428 (420 + 8), issues ~291 (unchanged).

- [ ] **Step 8: Verify no stale files in docs root**

```bash
ls docs/*.md 2>/dev/null && echo "WARN: stale files remain" || echo "OK: docs root clean"
```

Expected: "OK: docs root clean"

- [ ] **Step 9: Stage and commit**

```bash
git add docs/context/designs/ docs/references/ "docs/容器池技术方案.md" "docs/webSocket 请求可靠投递.md" "docs/架构优化迭代方案.md" docs/issues.md
git commit -m "docs: migrate existing documents into SDD context structure"
```

---

### Task 3: Write CLAUDE.md

**Files:**
- Create: `CLAUDE.md` (project root)

Reference: spec section 四 (CLAUDE.md 设计), README.md for current project description, existing code structure.

- [ ] **Step 1: Write CLAUDE.md**

Create `CLAUDE.md` in project root with the following content (English). Note: the file contains a plain-text diagram, not a code block — write the diagram as indented text or a code block with language `text`.

````
# CodeRunner

Distributed code execution system for blog platforms. Users submit code via the blog frontend, which is routed through gRPC to a server that dispatches execution to worker nodes via WebSocket. Results are returned asynchronously via HTTP callbacks.

## Project Overview

- **Architecture:** DDD (Domain-Driven Design) with four layers: interfaces → application → domain → infrastructure
- **Communication:** gRPC (external API) + WebSocket (Server ↔ Worker bidirectional) + HTTP Callback (async result delivery)
- **Dual-mode binary:** Server mode (cloud-side task orchestration) / Client mode (worker node code execution), switched in `cmd/api/main.go`
- **Supported languages:** Go, Python, JavaScript, Java, C++
- **Tech stack:** Go 1.23, Gin, gRPC, gorilla/websocket, Docker SDK, ETCD, Redis, Zap, Prometheus

## System Architecture

Core request flow:

```text
Blog Backend → gRPC Execute → P2C+EWMA Load Balancer → WebSocket Send to Worker
    → Worker receives task → Docker container execution → HTTP Callback to Blog Backend
```

Key components by DDD layer:
- **interfaces/**: gRPC controllers (`internal/interfaces/controller/`), middleware interceptors
- **application/**: Service orchestration for server and client modes (`internal/application/service/`)
- **domain/**: Client entity, ClientManager service, domain events (`internal/domain/`)
- **infrastructure/**: WebSocket, Docker containers, ETCD discovery, P2C balancer, config, logging, metrics, tracing (`internal/infrastructure/`)

## Code Conventions

- **DDD layering:** Domain layer MUST NOT import infrastructure. Application layer orchestrates domain services.
- **Error handling:** Use error types from `internal/infrastructure/common/errors/`
- **Logging:** Zap only (`internal/infrastructure/common/logger/`). Do NOT use logrus.
- **Proto changes:** After modifying `.proto` files in `api/proto/`, regenerate `.pb.go` files.
- **Commits:** Conventional commits — `feat:`, `fix:`, `docs:`, `refactor:`, `test:`

## Documentation Map

| Category | Path | Description |
|----------|------|-------------|
| PRD / Requirements | `docs/context/requirements/` | Product requirements (generated by PRD Agent) |
| Technical Designs | `docs/context/designs/` | Architecture and implementation designs |
| Test Plans | `docs/context/test-plans/` | Acceptance criteria and test strategies |
| Agent Definitions | `docs/agents/` | PRD / Design / TestPlan Agent workflows |
| Issue Tracking | `docs/references/issues.md` | Known bugs and improvement items |
| Architecture Decisions | `docs/context/decisions/` | ADRs (when needed) |

## Development Workflow

**New feature flow (SDD):**
1. PRD Agent → `docs/context/requirements/{name}-prd.md` → human review
2. Design Agent → `docs/context/designs/{name}-design.md` → human review
3. TestPlan Agent → `docs/context/test-plans/{name}-testplan.md` → human review
4. AI Coding (using above 3 docs as context) → Code Review
5. Acceptance (verify TestPlan Must Have items, confirm no Redline violations)

**Maintenance cadence:**
- After completing a feature: update Documentation Map index if new doc categories are added
- After tech stack changes: update Project Overview and Code Conventions
- After establishing new conventions: update Code Conventions
- Periodically: review `docs/references/issues.md`, mark resolved items

**Configuration:** `configs/dev.yaml` (development) / `configs/product.yaml` (production)
**Build:** Dockerfiles in `builds/api/` (server) and `builds/runners/` (language runtimes)
**Orchestration:** `docker-compose.yml` with environment overrides in `docker-compose/`
````

- [ ] **Step 2: Verify CLAUDE.md index paths resolve**

```bash
for path in docs/context/requirements docs/context/designs docs/context/test-plans docs/agents docs/references/issues.md docs/context/decisions; do
  [ -e "$path" ] && echo "OK: $path" || echo "MISSING: $path"
done
```

Expected: all OK.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add CLAUDE.md as project spec index"
```

---

### Task 4: Write PRD Agent definition

**Files:**
- Create: `docs/agents/prd-agent.md`

- [ ] **Step 1: Write prd-agent.md**

Create `docs/agents/prd-agent.md` with the following content:

````markdown
# PRD Agent

## 职责

从业务需求或用户故事产出结构化 PRD（产品需求文档）。

## 输入

- 业务背景描述（口头或文字形式）
- `CLAUDE.md`（项目上下文）
- 相关的现有功能代码（可选，用于理解现状）

## 输出模板

产出文件必须严格遵循以下模板：

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
（典型使用场景描述，包含用户角色、操作步骤、期望结果）

## 功能清单
| 编号 | 功能点 | 描述 | 验收条件 | 优先级 |
|------|--------|------|----------|--------|
| F1   | ...    | ...  | ...      | Must   |

## 非功能需求
（性能、安全、兼容性要求）

## 约束与依赖
（技术约束、外部依赖、前置条件——引用 CLAUDE.md 中的技术约束）

## 不做什么（边界声明）
（明确排除在范围外的内容）

## 开放问题
（待确认的事项）
```

## 质量标准

1. 每个功能点必须有可验证的验收条件
2. 必须包含"不做什么"的边界声明
3. 优先级使用 MoSCoW 分级（Must / Should / Could / Won't）
4. 必须引用 CLAUDE.md 中的技术约束

## 产出路径

`docs/context/requirements/{feature-name}-prd.md`
````

- [ ] **Step 2: Verify file has all required sections**

```bash
grep "^## " docs/agents/prd-agent.md
```

Expected: 职责、输入、输出模板、质量标准、产出路径（5 sections）。

- [ ] **Step 3: Commit**

```bash
git add docs/agents/prd-agent.md
git commit -m "docs: add PRD Agent workflow definition"
```

---

### Task 5: Write Design Agent definition

**Files:**
- Create: `docs/agents/design-agent.md`

- [ ] **Step 1: Write design-agent.md**

Create `docs/agents/design-agent.md` with the following content:

````markdown
# 技术方案 Agent

## 职责

从 PRD 产出技术设计方案。

## 输入

- 对应的 PRD 文档
- `CLAUDE.md`（项目架构、编码规范）
- 现有相关代码（自动检索）

## 输出模板

产出文件必须严格遵循以下模板：

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

## 质量标准

1. 必须包含至少 1 个替代方案及淘汰理由
2. 数据结构和接口定义必须具体到字段级别
3. 实施步骤必须可拆分为独立 PR
4. 必须评估对现有功能的影响

## 与现有 skill 的关系

- `docs/agents/design-agent.md`（本文件）定义"做什么"：输入、输出、质量标准
- `.claude/skills/agent-tech-spec/` 定义"怎么调用 Claude Code 来做"：具体的 prompt 和工具调用方式
- 两者互补，不冲突

## 产出路径

`docs/context/designs/{feature-name}-design.md`
````

- [ ] **Step 2: Verify file has all required sections**

```bash
grep "^## " docs/agents/design-agent.md
```

Expected: 职责、输入、输出模板、质量标准、与现有 skill 的关系、产出路径（6 sections）。

- [ ] **Step 3: Commit**

```bash
git add docs/agents/design-agent.md
git commit -m "docs: add Design Agent workflow definition"
```

---

### Task 6: Write TestPlan Agent definition

**Files:**
- Create: `docs/agents/testplan-agent.md`

- [ ] **Step 1: Write testplan-agent.md**

Create `docs/agents/testplan-agent.md` with the following content:

````markdown
# TestPlan Agent

## 职责

从 PRD + 技术方案产出验收标准和测试计划。

## 输入

- 对应的 PRD 文档
- 对应的技术方案文档
- `CLAUDE.md`（项目上下文和编码规范）

## 输出模板

产出文件必须严格遵循以下模板：

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

## Should Have（重要但不阻塞发布）
- [ ] ...

## Nice to Have（有则更好）
- [ ] ...

## Redline（绝对不能做的事）
- 禁止项 1（可自动检查）
- 禁止项 2

## 测试策略

### 单元测试
（需要覆盖的核心函数/方法）

### 集成测试
（端到端场景描述，包含正常路径和异常路径）

### 性能测试（如适用）
（基准指标和测试方法）
```

## 质量标准

1. Must Have 条目必须可机械验证（能写成测试断言）
2. Redline 条目必须可自动检查（能写成 lint 规则或测试）
3. 每个测试场景要包含正常路径和异常路径
4. Must Have 条目必须与 PRD 的验收条件一一对应

## 产出路径

`docs/context/test-plans/{feature-name}-testplan.md`
````

- [ ] **Step 2: Verify file has all required sections**

```bash
grep "^## " docs/agents/testplan-agent.md
```

Expected: 职责、输入、输出模板、质量标准、产出路径（5 sections）。

- [ ] **Step 3: Commit**

```bash
git add docs/agents/testplan-agent.md
git commit -m "docs: add TestPlan Agent workflow definition"
```

---

### Task 7: Final verification

**Files:** All files from Tasks 1-6.

- [ ] **Step 1: Verify complete directory structure**

```bash
find docs/context docs/agents docs/references -type f | sort
```

Expected:
```
docs/agents/design-agent.md
docs/agents/prd-agent.md
docs/agents/testplan-agent.md
docs/context/decisions/.gitkeep
docs/context/designs/architecture-roadmap.md
docs/context/designs/container-pool-design.md
docs/context/designs/websocket-reliable-delivery-design.md
docs/context/requirements/.gitkeep
docs/context/test-plans/.gitkeep
docs/references/issues.md
```

- [ ] **Step 2: Verify CLAUDE.md exists at project root**

```bash
head -5 CLAUDE.md
```

Expected: `# CodeRunner` header.

- [ ] **Step 3: Verify no stale files in docs root**

```bash
ls docs/*.md 2>/dev/null
```

Expected: no output (all .md files moved to subdirectories).

- [ ] **Step 4: Verify frontmatter on migrated design docs**

```bash
head -8 docs/context/designs/container-pool-design.md
head -8 docs/context/designs/websocket-reliable-delivery-design.md
head -8 docs/context/designs/architecture-roadmap.md
```

Expected: each starts with `---` frontmatter block containing title, type, status, created, updated fields.

- [ ] **Step 5: Verify CLAUDE.md index paths all resolve**

```bash
for path in docs/context/requirements docs/context/designs docs/context/test-plans docs/agents docs/references/issues.md docs/context/decisions; do
  [ -e "$path" ] && echo "OK: $path" || echo "MISSING: $path"
done
```

Expected: all OK.

- [ ] **Step 6: Verify git status is clean**

```bash
git status
```

Expected: working tree clean (all SDD restructuring changes committed).
