# P0 修复计划：Fork 炸弹与跨用户隔离

日期：2026-04-20
对应问题：[concurrency-risks.md](concurrency-risks.md) 的 #1 和 #2。
目标：两个 PR，分别闭合「宿主机级 DoS」和「跨用户代码可见」。

---

## PR-1：pids_limit 修复 Fork 炸弹（#1）

### 改动范围
- `docker-compose/dev/client/docker-compose.yml`
- `docker-compose/product/client/docker-compose.yml`

每个 runner 服务（go-0/1、python-0/1、cpp-0/1、java-0、js-0/1）加：

```yaml
pids_limit: 64
```

client 容器（`code-runner-api-client` / `coderunner-client`）本身也加 `pids_limit: 512`，作为兜底。

### 为什么是 64
- 正常 `go run` / `python` / `javac+java` 进程树峰值 10-30；Java 最多（JVM 线程）。
- 留 2 倍余量。如果回归测试 Java 有溢出，单独把 java runner 放宽到 128。

### 验证步骤
1. 本地 `docker compose -f docker-compose/dev/client/docker-compose.yml up -d` 启动。
2. 正常回归：跑现有测试套件，确保每种语言的 hello world + 一个较复杂样例都能通过。
3. 攻击回归：提交以下 payload，预期返回错误而不是容器 / 宿主机崩溃：
   - Python: `import os\nwhile True: os.fork()`
   - C: `#include <unistd.h>\nint main(){while(1)fork();}`
   - Bash via `sh`：`:(){ :|:& };:`
4. 在宿主机 `docker exec code-runner-python-0 ps aux | wc -l` 观察进程数卡在 ~64。
5. 攻击期间 server 容器仍能响应其他 runner 的请求。

### 回滚
改动是纯 YAML；出问题 `git revert` 后 `docker compose up -d` 即可。

### 预计工作量
- 编码：30 分钟（含 dev/product 两份）。
- 测试：1 小时。
- 风险：低。

---

## PR-2：跨用户隔离（#2）

分两步做，**先堵泄漏，再做彻底清理**。

### Step 2a：收紧 client 容器的 /tmp 挂载

**问题根源**：[docker-compose/*/client/docker-compose.yml:10](../../docker-compose/product/client/docker-compose.yml#L10) 把宿主机 `/tmp` 整个挂进 client 容器。client 容器里一旦被攻破（理论上只能通过 Docker socket，但也算攻击面），就能读所有 runner 目录。

**改动**：client 只挂载自己真正需要的目录，不挂 `/tmp`。

查一下 client 代码到底读/写哪些路径：

```bash
grep -rn "/app/tmp\|/tmp/" internal/ | grep -v _test.go
```

根据结果，把挂载改成：

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
  - /tmp/coderunner:/app/tmp:rw    # 只挂 coderunner 专用子目录
```

runner 相应改成 `/tmp/coderunner/<lang>-N:/app`。

### Step 2b：执行后彻底清理 slot 目录

**当前问题**：[runCode.go:216](../../internal/infrastructure/containerBasic/runCode.go#L216) 只 `RemoveAll(rc.path)`（UUID 子目录），但用户代码如果写到 `/app` 根目录或 `/app/../`，残留会被下个用户看见。

**改动**：`RunCode` 成功 / 失败路径都在释放 slot 前，通过 `docker exec` 清空容器内 `/app` 所有内容：

```go
// 伪代码，在 ReleaseSlot 之前
_, _ = r.InContainerRunCode(slot.Name, "sh", []string{"-c", "rm -rf /app/* /app/.[!.]* 2>/dev/null || true"})
```

放在 `defer` 里，保证即使执行超时也会清理。清理失败则 `healthy=false` 让 pool 触发 `replenish`（重启容器彻底归零）。

### 验证步骤
1. 攻击场景 A：用户甲提交 `python: open('/app/secret.txt','w').write('HELLO')`，成功后等 2 秒，用户乙提交 `python: print(open('/app/secret.txt').read())`，**期望乙侧报 FileNotFoundError**。
2. 攻击场景 B：用户甲提交 `python: open('/app/../golang-0/main.go','r').read()`，**期望读不到**（因为 client 容器不再整挂 `/tmp`）。
3. 性能回归：清理加入后单次执行耗时增加 ≤ 20ms。

### 回滚
- Step 2a 是 YAML，直接 revert。
- Step 2b 是代码，revert 后 `go build` 验证。

### 预计工作量
- 编码：2 小时（主要是梳理 client 挂载依赖）。
- 测试：2 小时（攻击场景 + 性能 + 各语言回归）。
- 风险：中。Step 2a 改挂载路径可能影响 docker-in-docker 通信，需在 dev 先跑一轮。

---

## 时间表

| 日期 | 动作 |
|------|------|
| 2026-04-21 | 开 PR-1 分支，改 YAML，本地 dev 验证，提交 review |
| 2026-04-22 | PR-1 合并 + product 部署（低风险可直接上） |
| 2026-04-23 | 开 PR-2 分支，Step 2a 挂载调整 |
| 2026-04-24 | Step 2b 代码清理 + 攻击测试 |
| 2026-04-25 | PR-2 code review |
| 2026-04-28 | PR-2 合并 + product 部署（观察 1 天指标） |

## 验收标准

- [ ] fork 炸弹 payload 不再打崩容器 / 宿主机
- [ ] 跨用户代码文件互相不可见（两个验证场景通过）
- [ ] 现有功能回归全绿
- [ ] p99 执行延迟增长 < 30ms
- [ ] 生产监控观察 3 天，无新增异常

## 不在本次范围

P1/P2 的 #3 限流、#4 容器复用清理进程、#5-#9 纵深防御项留待后续 PR，参考 [concurrency-risks.md](concurrency-risks.md) 的分阶段建议。
