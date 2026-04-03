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

### Must Have（不通过不算完成）
- [ ] 具体的验收条件 1（可机械验证）
- [ ] 具体的验收条件 2

### Should Have（重要但不阻塞发布）
- [ ] ...

### Nice to Have（有则更好）
- [ ] ...

### Redline（绝对不能做的事）
- 禁止项 1（可自动检查）
- 禁止项 2

### 测试策略

#### 单元测试
（需要覆盖的核心函数/方法）

#### 集成测试
（端到端场景描述，包含正常路径和异常路径）

#### 性能测试（如适用）
（基准指标和测试方法）
```

## 质量标准

1. Must Have 条目必须可机械验证（能写成测试断言）
2. Redline 条目必须可自动检查（能写成 lint 规则或测试）
3. 每个测试场景要包含正常路径和异常路径
4. Must Have 条目必须与 PRD 的验收条件一一对应

## 产出路径

`docs/context/test-plans/{feature-name}-testplan.md`
