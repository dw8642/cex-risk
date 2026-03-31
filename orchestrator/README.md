# Orchestrator — 多 Agent 并行开发调度系统

## 架构概览

```
你（架构师）
  │
  ▼
orchestrator.sh  ←── tasks.json（任务定义）
  │
  ├─→ Agent-A (Backend Core)   ── cex-risk-agent-a/ worktree
  ├─→ Agent-B (Exchange)       ── cex-risk-agent-b/ worktree
  ├─→ Agent-C (Risk Engine)    ── cex-risk-agent-c/ worktree
  ├─→ Agent-D (Frontend)       ── cex-risk-agent-d/ worktree
  └─→ Agent-E (Infra/DevOps)   ── cex-risk-agent-e/ worktree
       │
       ▼
  test-report.sh  → tests/reports/  （测试报告留存）
  ci-helper.sh    → GitHub push     （代码上传）
```

## 文件说明

| 文件 | 作用 |
|------|------|
| `orchestrator.sh` | 调度主脚本 — 初始化环境、调度任务、管理合并 |
| `tasks.json` | 任务定义 — D1~D3 的完整任务、prompt、依赖、验收标准 |
| `test-report.sh` | 测试报告生成器 — 单元/集成/压力测试 + Markdown/JSON 报告 |
| `ci-helper.sh` | CI 辅助 — git push、远程仓库配置、文档生成、CHANGELOG |
| `.state.json` | 状态文件 — 自动生成，记录任务进度（不要手动编辑） |
| `logs/` | Agent 执行日志目录 |

## 快速开始

### 第 0 步：前置条件

```bash
# 确认工具已安装
claude --version    # Claude Code CLI
jq --version        # JSON 处理
go version          # Go 1.22+
docker --version    # Docker
```

### 第 1 步：配置 GitHub 仓库

```bash
cd orchestrator/
./ci-helper.sh setup-remote git@github.com:yourname/cex-risk.git
```

### 第 2 步：初始化 Agent 环境

```bash
# 初始化 Agent-A 和 Agent-B（D1 只需要这两个）
./orchestrator.sh init a b
```

### 第 3 步：查看 D1 任务计划

```bash
./orchestrator.sh plan D1
```

### 第 4 步：执行任务

**方式一：自动执行全天任务（按依赖顺序）**
```bash
./orchestrator.sh run D1
```

**方式二：手动逐个执行**
```bash
# 先执行 Agent-A 的基础任务（阻塞点）
./orchestrator.sh run-task D1-A1

# Agent-A 完成后，合并到 main
./orchestrator.sh merge a

# 同步到 Agent-B
./orchestrator.sh sync

# 执行 Agent-B 的任务
./orchestrator.sh run-task D1-B1
```

### 第 5 步：每日收工

```bash
# 合并所有 Agent，推送到 GitHub
./orchestrator.sh merge-all D1
./ci-helper.sh push-all

# 运行测试，生成报告
./test-report.sh unit
```

### 第 6 步：验收

```bash
./orchestrator.sh gate D1
```

## 日常操作速查

```bash
# 查看状态
./orchestrator.sh status

# 查看计划
./orchestrator.sh plan D2

# 执行一天
./orchestrator.sh run D2

# 合并单个 Agent
./orchestrator.sh merge b

# 同步 main 到所有 worktree
./orchestrator.sh sync

# 每日收工
./orchestrator.sh merge-all D2
./ci-helper.sh push-all

# 跑测试
./test-report.sh unit
./test-report.sh integration
./test-report.sh benchmark

# 提交前检查
./ci-helper.sh pre-commit

# 生成文档
./ci-helper.sh gen-docs

# 检查中间件
./orchestrator.sh health
```

## 任务依赖模型

每个任务有三种类型：

- **🔒 blocker** — 阻塞点，下游任务必须等这个完成
- **⚡ parallel** — 可与同天其他任务并行执行
- **⏳ half_day_dep** — 依赖同天某个任务的产出（通常上午→下午）

Orchestrator 的 `run` 命令会自动按 `merge_sequence` 定义的顺序执行，确保依赖关系正确。

## 扩展任务定义

`tasks.json` 目前包含 D1~D3（最小闭环版本）的完整任务定义。后续 D4~D40 可以按同样格式添加。每个任务的 `prompt` 字段就是发给 Claude Code Agent 的完整指令。

添加新天数的步骤：
1. 在 `tasks.json` 的 `days` 对象中添加新的 day
2. 定义 tasks 数组（id, agent, type, phase, title, prompt, acceptance, depends_on）
3. 定义 merge_sequence（合并顺序）
4. 如果是版本发布日，添加 release 对象

## 开发规范执行

Orchestrator 在每个 Agent 的 prompt 尾部自动附加开发规范提醒：

1. 编译检查（go build）
2. 测试通过（go test）
3. Git commit（规范的 commit message）
4. 单元测试要求

这些规范来自项目根目录的 `CLAUDE.md`。
