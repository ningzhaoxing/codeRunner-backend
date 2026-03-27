# CodeRunner - 分布式代码执行系统

基于 DDD 架构的分布式代码在线运行系统，为博客平台提供代码执行能力。

## 项目简介

CodeRunner 是一个微服务项目，让读者可以直接在博客平台上运行代码，无需跳转到 IDE。灵感来源于 Online Judge 系统。

### 核心特性

- 🚀 **分布式架构**: 云服务器调度 + 内网节点执行
- 🔒 **安全隔离**: Docker 容器沙箱，资源限制
- ⚖️ **负载均衡**: 基于节点负载的智能调度
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
│ codeRunner-server    │ ← 腾讯云服务器
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
| codeRunner-server → 博客后端 | 异步回调 | 推送执行结果 |
| 博客后端 → 前端 | SSE | 服务器推送 |

## 项目结构

```
code-runner/
├── codeRunner-server/       # 服务端（任务调度）
│   ├── cmd/server/          # 应用入口
│   ├── internal/
│   │   ├── domain/          # 领域层：核心业务逻辑
│   │   ├── application/     # 应用层：流程编排
│   │   ├── infrastructure/  # 基础设施层：技术实现
│   │   └── interfaces/      # 接口层：对外 API
│   └── README.md
│
├── codeRunner-client/       # 客户端（代码执行）
│   ├── cmd/client/          # 应用入口
│   ├── internal/
│   │   ├── domain/          # 领域层：沙箱、执行器
│   │   ├── application/     # 应用层：客户端主程序
│   │   └── infrastructure/  # 基础设施层：WebSocket、监控
│   └── README.md
│
├── shared/                  # 共享代码
│   ├── proto/               # gRPC protobuf 定义
│   ├── types/               # 共享类型定义
│   └── utils/               # 工具函数
│
└── redmine.md              # 详细设计文档（类图、时序图）
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
- **内容**: HTTP/gRPC 接口实现

## 快速开始

### 环境要求

**Server 端:**
- Go 1.21+
- ETCD 3.5+
- MySQL 8.0+ / PostgreSQL 14+

**Client 端:**
- Go 1.21+
- Docker 24.0+

### 启动 Server

```bash
cd codeRunner-server
go mod tidy
cd cmd/server
go run main.go
```

### 启动 Client

```bash
cd codeRunner-client
go mod tidy
cd cmd/client
go run main.go --server-url ws://server:8080/ws
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.21+ |
| 架构 | DDD (领域驱动设计) |
| RPC | gRPC |
| 实时通信 | WebSocket |
| 服务发现 | ETCD |
| 容器化 | Docker |
| 数据库 | MySQL / PostgreSQL |
| 消息推送 | SSE |

## 核心流程

### 代码执行流程

```
1. 用户在博客前端提交代码
   ↓
2. 博客后端通过 gRPC 调用 codeRunner-server
   ↓
3. codeRunner-server 创建任务并入队
   ↓
4. 调度器选择负载最低的 client 节点
   ↓
5. 通过 WebSocket 发送任务到 client
   ↓
6. client 在 Docker 沙箱中执行代码
   ↓
7. client 返回执行结果给 server
   ↓
8. server 通过异步回调推送结果给博客后端
   ↓
9. 博客后端通过 SSE 推送给前端
   ↓
10. 用户看到执行结果
```

### 客户端注册流程

```
1. client 启动，连接 server 的 WebSocket
   ↓
2. server 生成 clientId，创建 ClientNode
   ↓
3. server 将 client 信息注册到 ETCD
   ↓
4. client 定时发送心跳（CPU、内存、任务数）
   ↓
5. server 更新 ClientNode 状态
   ↓
6. 断线时从 ETCD 注销
```

## 负载均衡策略

基于以下指标计算节点负载评分：

```go
loadScore = (0.4 * cpuUsage) +
            (0.4 * memoryUsage) +
            (0.2 * runningTasks / maxTasks)
```

选择评分最低的节点执行任务。

## 安全措施

1. **容器隔离**: 每个任务在独立 Docker 容器中运行
2. **资源限制**: 严格限制 CPU、内存、执行时间
3. **网络隔离**: 容器默认无网络访问
4. **文件隔离**: 容器内无法访问宿主机文件系统
5. **代码检查**: 提交前进行基本的安全检查

## 文档

- [Server 端架构说明](codeRunner-server/README.md)
- [Client 端架构说明](codeRunner-client/README.md)
- [详细设计文档（类图、时序图）](redmine.md)

## 开发规范

1. **遵循 DDD 原则**: 领域层不依赖外部框架
2. **依赖倒置**: 通过接口定义依赖
3. **单一职责**: 每个包职责清晰
4. **测试覆盖**: 核心业务逻辑需要单元测试
5. **错误处理**: 使用 Go 的错误处理机制，不使用 panic


# 新增功能

1. 运行状态监控：将容器运行、代码运行追踪情况上报信息，并记录到监控平台
2. AI 自动代码补全

## License

MIT License
