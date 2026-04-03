# CodeRunner - 分布式代码执行系统

基于 DDD 架构的分布式代码在线运行系统，为博客平台提供代码执行能力。

## 项目简介

CodeRunner 是一个微服务项目，让读者可以直接在博客平台上运行代码，无需跳转到 IDE。灵感来源于 Online Judge 系统。

### 核心特性

- 🚀 **分布式架构**: 云服务器调度 + 内网节点执行
- 🔒 **安全隔离**: Docker 容器沙箱，资源限制
- ⚖️ **自适应负载均衡**: P2C + EWMA 算法，根据节点实时延迟和在途请求数动态调度
- 📡 **实时通信**: WebSocket + SSE 实时推送结果
- 🎯 **DDD 架构**: 领域驱动设计，清晰的分层结构
- 🔄 **服务发现**: ETCD 实现节点注册与发现

## 系统架构

![架构图](image.png)

### 组件说明

```
┌─────────────────┐
│  博客系统前端    │ ← 用户提交代码
└────────┬────────┘
         │ HTTP + SSE
┌────────▼────────┐
│  博客系统后端    │
└────────┬────────┘
         │ gRPC
┌────────▼─────────────┐
│ codeRunner-server    │ ← 云服务器
│ (任务调度、客户端管理) │
└────────┬─────────────┘
         │ WebSocket
    ┌────┴────┬────────┐
    │         │        │
┌───▼──┐ ┌───▼──┐ ┌───▼──┐
│Client1│ │Client2│ │Client3│ ← 内网服务器节点
│(执行) │ │(执行) │ │(执行) │   (实际运行代码)
└───────┘ └───────┘ └───────┘
```

### 通信机制

| 通信路径 | 协议 | 用途 |
|---------|------|------|
| 博客后端 → codeRunner-server | gRPC | 同步提交任务 |
| codeRunner-server → codeRunner-client | WebSocket | 双向实时通信 |
| codeRunner-client → 博客后端 | HTTP 异步回调 | 推送执行结果 |
| 博客后端 → 前端 | SSE | 服务器推送 |

## 项目结构

```
coderunner/
├── cmd/api/                   # 应用入口（Server/Client 模式切换）
├── api/proto/                 # gRPC protobuf 定义及生成代码
├── configs/                   # 配置文件（dev.yaml / product.yaml）
├── docs/                      # 文档
│   ├── context/               # 上下文文档（PRD / 技术方案 / TestPlan）
│   ├── agents/                # Agent 工作流定义
│   └── references/            # 参考资料（issues 等）
├── builds/                    # 各语言 Docker 镜像构建文件
├── docker-compose/            # 容器编排配置
└── internal/
    ├── interfaces/            # 接口层：gRPC/HTTP/WebSocket 控制器
    ├── application/           # 应用层：业务流程编排
    ├── domain/                # 领域层：核心业务逻辑
    └── infrastructure/        # 基础设施层：技术实现
        ├── balanceStrategy/   # 负载均衡（P2C + EWMA）
        ├── websocket/         # WebSocket Server/Client
        ├── containerBasic/    # Docker 容器管理
        ├── etcd/              # 服务注册与发现
        ├── grpc/              # gRPC 服务注册
        └── common/            # 配置、日志、Token 等公共组件
```

## DDD 分层架构

### 领域层 (Domain Layer)

- **职责**: 核心业务逻辑和规则
- **特点**: 不依赖任何外部框架
- **内容**: 实体、值对象、领域服务、仓储接口

### 应用层 (Application Layer)

- **职责**: 编排业务流程，协调领域对象
- **特点**: 无业务逻辑，只做编排
- **内容**: 应用服务、命令对象、领域事件

### 基础设施层 (Infrastructure Layer)

- **职责**: 技术实现和外部集成
- **特点**: 实现领域层定义的接口
- **内容**: 数据库、消息队列、缓存、外部 API

### 接口层 (Interface Layer)

- **职责**: 对外提供 API
- **特点**: 转换数据格式，处理协议细节
- **内容**: gRPC/HTTP 接口实现

## 快速开始

### 环境要求

- Go 1.23+
- ETCD 3.5+
- Docker 24.0+

### 环境变量

启动前必须设置以下环境变量：

```bash
export JWT_SECRET="your-strong-secret-here"
export AUTH_PASSWORD="your-strong-password-here"
```

### 启动 Server（云服务器端）

```bash
# 修改 cmd/api/main.go，调用 initialize.RunServer()
go run cmd/api/main.go
```

### 启动 Client（内网节点）

```bash
# 修改 cmd/api/main.go，调用 initialize.RunClient()
go run cmd/api/main.go
```

配置文件位于 `configs/dev.yaml`（开发）或 `configs/product.yaml`（生产）。

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.23+ |
| 架构 | DDD (领域驱动设计) |
| RPC | gRPC |
| 实时通信 | WebSocket (Gorilla) |
| 服务发现 | ETCD |
| 容器化 | Docker SDK |
| 配置管理 | Viper |
| 日志 | Zap |
| 消息推送 | SSE |

## 核心流程

### 代码执行流程

```
1. 用户在博客前端提交代码
   ↓
2. 博客后端通过 gRPC 调用 codeRunner-server
   ↓
3. codeRunner-server 通过 P2C 算法选择负载最低的 client 节点
   ↓
4. 通过 WebSocket 发送任务到 client
   ↓
5. client 在 Docker 沙箱中执行代码
   ↓
6. client 通过 HTTP 回调将结果发送给博客后端（失败自动重试）
   ↓
7. 博客后端通过 SSE 推送给前端
   ↓
8. 用户看到执行结果
```

### 客户端注册流程

```
1. client 启动，通过 WebSocket 连接 server（携带权重参数）
   ↓
2. server 将 client 加入 P2C 负载均衡节点池
   ↓
3. 双向心跳保活：server 每 30s 主动 Ping，60s 无响应自动断开
   ↓
4. 断线时从节点池移除，自动触发重新选择其他节点
```

## 负载均衡策略

使用 **P2C（Power of Two Choices）+ EWMA** 自适应算法：

```
随机选 2 个节点，比较各自 load，选 load 更低的

load = √(ewma_发送延迟 + 1) × (在途请求数 + 1) / 权重
```

- **自适应**：慢节点自动减少流量，无需手动调权重
- **防饥饿**：超过 1s 未被选中的节点会被强制选择一次
- **健康感知**：成功率低于 50% 的节点自动被惩罚

详见 [P2C 负载均衡说明](internal/infrastructure/balanceStrategy/p2cBalance/README.md)

## 安全措施

1. **容器隔离**: 每个任务在独立 Docker 容器中运行
2. **资源限制**: 严格限制 CPU、内存、执行时间
3. **网络隔离**: 容器默认无网络访问
4. **文件隔离**: 容器内无法访问宿主机文件系统
5. **代码长度限制**: codeBlock 最大 64KB，拒绝超大请求
6. **认证鉴权**: gRPC 请求需携带 JWT Token，Secret 从环境变量读取

## 开发规范

1. **遵循 DDD 原则**: 领域层不依赖外部框架
2. **依赖倒置**: 通过接口定义依赖
3. **单一职责**: 每个包职责清晰
4. **测试覆盖**: 核心业务逻辑需要单元测试
5. **错误处理**: 使用 Go 的错误处理机制，不使用 panic

## 文档体系

项目采用 **Spec-Driven Development（SDD）** 工作流，详见 [CLAUDE.md](CLAUDE.md)。

| 类别 | 路径 | 说明 |
|------|------|------|
| 需求文档 | `docs/context/requirements/` | PRD（由 PRD Agent 产出） |
| 技术方案 | `docs/context/designs/` | 架构设计与实现方案 |
| 测试计划 | `docs/context/test-plans/` | 验收标准与测试策略 |
| Agent 定义 | `docs/agents/` | PRD / 技术方案 / TestPlan Agent 工作流 |
| 问题追踪 | `docs/references/issues.md` | Bug 与改进项 |

**新功能开发流程：** PRD Agent → 技术方案 Agent → TestPlan Agent → 编码 → 验收

## License

MIT License
