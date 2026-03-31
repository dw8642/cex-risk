<<<<<<< HEAD
# CEX 风控系统 — Agent 开发指令

## 项目概述
CEX 做市风控系统，Go 后端 + Next.js 前端 + MySQL/Redis/Kafka/ClickHouse。
架构文档：../04_系统架构与开发方案.md
每日计划：../05_每日开发计划_多Agent并行.md
风控方法论：../risk_control_plan_v2.6.md

## 技术栈
- Go 1.22, gorilla/websocket, zap logger, segmentio/kafka-go
- MySQL 8, Redis 7, Kafka 3.8 (KRaft), ClickHouse 23.12
- Next.js 14, TypeScript, Tailwind CSS

## 代码规范

### 1. 注释要求
- 所有公共函数、接口、结构体必须有中文注释
- 关键业务逻辑段落需加中文行内注释说明意图
- 复杂算法或风控规则需在函数头部写明：规则编号、触发条件、预期行为
- 示例：
```go
// CheckWithdrawPermission 检测提币权限异常（规则 P-001）
// 触发条件：API Key 当前具有提币权限（enableWithdrawals=true）
// 预期行为：生成 P1 级别 RiskEvent，通知管理员确认
func (r *P001Rule) CheckWithdrawPermission(ctx context.Context, account *models.Account) (*models.RiskEvent, error) {
```

### 2. 错误处理（禁止 try-catch / recover 兜底）
- Go 中禁止使用 `recover()` 做兜底，除非是顶层 goroutine 防 crash
- 所有错误必须显式检查和处理，不允许 `_ = someFunc()`
- 使用 sentinel error + fmt.Errorf("xxx: %w", err) 包装上下文
- 在调用链入口处做参数校验，提前返回明确错误，而不是让错误在下游爆发
- 示例：
```go
// ❌ 错误做法：忽略错误、兜底 recover
func bad() {
    defer func() { recover() }()
    result, _ := riskyCall()
}

// ✅ 正确做法：提前校验，显式处理，包装上下文
func good(accountID string) error {
    if accountID == "" {
        return fmt.Errorf("accountID 不能为空")
    }
    result, err := store.GetAccount(accountID)
    if err != nil {
        return fmt.Errorf("查询账户 %s 失败: %w", accountID, err)
    }
    if result == nil {
        return fmt.Errorf("账户 %s 不存在", accountID)
    }
    // ... 正常逻辑
    return nil
}
```

### 3. 测试要求
- 每个新模块/文件必须有对应的 `_test.go` 单元测试
- 测试用例需覆盖：正常路径、边界条件、错误路径
- 集成测试放在 `tests/` 目录，使用 `//go:build integration` tag
- 压力测试放在 `tests/benchmark/` 目录，使用 `testing.B`
- 测试完成后生成报告到 `tests/reports/` 目录
- 运行测试命令：
```bash
make test           # 全量单元测试
make test-integration   # 集成测试（需中间件）
make test-cover     # 覆盖率报告
```

### 4. Git 规范
- commit message 格式: `feat|fix|refactor|test|docs(模块): 中文描述`
- 每完成一个功能点立即 commit + push
- 不要积攒大量改动一次性 commit
- 示例：
```
feat(store): 实现四层治理 MySQL CRUD
fix(exchange): 修复 WS 重连后 listenKey 未更新的问题
test(rules): 添加 P-001 提币权限检测单元测试
docs(api): 更新 risk-events 接口文档
```

### 5. 文档要求
- 每个新服务需在 `docs/` 下创建设计文档
- 重要的架构决策需记录 ADR（Architecture Decision Record）
- API 接口需在代码中用注释标注请求/响应格式
- 复杂的数据流需画 ASCII 流程图

## 目录结构
```
cex-risk/
├── cmd/                    # 各服务入口
│   ├── healthcheck/
│   ├── ingestor/
│   ├── risk-engine/
│   ├── control-executor/
│   ├── api-server/
│   ├── telegram-notifier/
│   ├── private-data-sink/
│   └── watchdog/
├── pkg/                    # 公共包
│   ├── config/
│   ├── models/
│   ├── store/
│   ├── mq/
│   ├── exchange/
│   └── logger/
├── services/               # 各服务内部逻辑
│   ├── exchange-ingestor/
│   ├── risk-engine/
│   │   ├── metrics/
│   │   └── alert/
│   │       └── rules/
│   ├── control-executor/
│   └── api-server/
├── frontend/               # Next.js 前端
├── scripts/                # SQL 初始化、工具脚本
├── orchestrator/           # 多 Agent 调度系统
├── tests/                  # 集成测试、压测
│   ├── integration/
│   ├── benchmark/
│   └── reports/
├── deploy/                 # Docker Compose、监控
└── docs/                   # 架构文档
```

## 重要约束
- 不要修改其他 Agent 负责的文件（参见 Agent 专属指令）
- 公共接口（interface）修改必须在 main 分支上进行
- 每完成一个功能点，立即 commit + push 到自己的分支
- 所有数据库操作必须参数化，禁止拼接 SQL
- API Key 等敏感信息从 config.toml 读取，禁止硬编码
=======

## Agent-A 专属指令
你是 Agent-A（Backend Core），负责：
- pkg/ 下所有公共包（config, models, store, mq, logger）
- cmd/ 下的服务入口
- services/api-server/、services/telegram-notifier/
- services/control-executor/、services/private-data-sink/、services/watchdog/
- scripts/ 下的 SQL 初始化脚本

### 你的文件归属（只能修改这些）
- pkg/**、cmd/**
- services/api-server/**、services/telegram-notifier/**
- services/control-executor/**、services/private-data-sink/**、services/watchdog/**
- scripts/**、config.toml

### 禁止修改
- services/exchange-ingestor/**（Agent-B）
- services/risk-engine/**（Agent-C）
- frontend/**（Agent-D）
- deploy/**（Agent-E）
>>>>>>> agent-a/current
