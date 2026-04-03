# CEX 做市风控系统 — 多 Agent 并行开发计划

> **版本**：V1.0
> **日期**：2026-04-02
> **定位**：基于 `mvp_development_plan.md` 的任务拆解，设计多 Agent 并行开发方案，通过 Orchestrator 调度实现最大并行度，将 6 周串行计划压缩至约 3.5~4 周。

---

## 1. 设计原则

### 1.1 为什么需要多 Agent 并行

MVP 任务表面上是 Phase 1 → Phase 2 → Phase 3 的线性链路，但实际上：

- Phase 1 内部的 Ingestor/指标/规则/告警/Watchdog 有大量可并行模块
- Phase 2 的前端开发与 Phase 1 的后端只需**接口契约**即可解耦，不必等后端全部完成
- Phase 3 的 RBAC/Canary 可以提前启动，不依赖 Phase 2 的前端

### 1.2 核心约束

| 约束 | 说明 |
|------|------|
| **单一真相源** | 所有 Agent 共享同一个 Git 仓库 + 同一份设计文档 |
| **接口先行** | Agent 间依赖通过 Interface/Contract 解耦，不等实现 |
| **集成由 Orchestrator 把控** | 每个 Sprint 的集成测试由 Orchestrator 调度，不由单个 Agent 自行判断 |
| **代码边界清晰** | 每个 Agent 只写自己的目录，跨目录改动需 Orchestrator 审批 |

---

## 2. Agent 角色定义

### 2.1 角色总览

```
                    ┌─────────────────────┐
                    │   Orchestrator (O)  │
                    │   调度 · 集成 · 质检  │
                    └─────┬───────────────┘
                          │ 调度指令 / Gate 检查
        ┌─────────────────┼─────────────────────────┐
        │                 │                          │
        ▼                 ▼                          ▼
  ┌───────────┐   ┌───────────┐   ┌───────────┐  ┌───────────┐  ┌───────────┐
  │ Agent-F   │   │ Agent-D   │   │ Agent-I   │  │ Agent-R   │  │ Agent-S   │
  │ Foundation│   │ Data      │   │ Indicator │  │ Rule+Alert│  │ Sentinel  │
  │ 基座层     │   │ 数据层    │   │ 指标层    │  │ 规则+告警  │  │ 哨兵层    │
  └───────────┘   └───────────┘   └───────────┘  └───────────┘  └───────────┘

  ┌───────────┐   ┌───────────┐
  │ Agent-W   │   │ Agent-Q   │
  │ Web/UI    │   │ QA        │
  │ 前端控制台  │   │ 测试+验收  │
  └───────────┘   └───────────┘
```

### 2.2 各 Agent 详细定义

#### Orchestrator (O) — 调度中枢

| 属性 | 说明 |
|------|------|
| **职责** | 任务分发、依赖 Gate 管理、集成测试触发、冲突仲裁、进度追踪 |
| **不做** | 不写业务代码，不做具体实现 |
| **输出** | Sprint 计划、Gate 检查结果、集成测试报告、阻塞问题升级 |
| **调度频率** | 每个 Sprint（~3天）一个调度周期 |

**Orchestrator 的 Gate 机制**：

```
Gate = 一组前置条件，全部满足后才允许下游 Agent 启动依赖任务

Gate-1: "基座就绪"
  ✅ MySQL Schema 已初始化并可连接
  ✅ Redis/Kafka/ClickHouse 容器已运行
  ✅ pkg/store/* 抽象层有可编译的接口
  ✅ pkg/model/* 数据模型已定义

Gate-2: "数据管道就绪"
  ✅ 公共 Ingestor 写入 Kafka + Redis 可验证
  ✅ 私有 Ingestor 写入 Kafka + Redis 可验证
  ✅ Gate-1 已通过

Gate-3: "指标层就绪"
  ✅ 指标引擎 T0~T2 正常产出到 Redis
  ✅ 85 个指标中至少 S-Class + L-Class 已实现
  ✅ Gate-2 已通过

Gate-4: "规则层就绪"
  ✅ 规则引擎 F0~F2 正常评估并产出 RiskEvent
  ✅ 告警服务可发 Telegram
  ✅ Gate-3 已通过

Gate-5: "API 就绪"
  ✅ Dashboard API 读取类端点全部可用
  ✅ Swagger 文档已生成
  ✅ Gate-4 已通过

Gate-6: "配置 API 就绪"
  ✅ 规则绑定 CRUD API 可用
  ✅ 变更审批 API 可用
  ✅ RBAC scope 中间件生效
  ✅ Gate-5 已通过
```

---

#### Agent-F (Foundation) — 基座层

| 属性 | 说明 |
|------|------|
| **职责** | 项目脚手架、DB Schema、存储抽象层、数据模型、Docker Compose、CI 基础 |
| **代码范围** | `pkg/store/*`, `pkg/model/*`, `deploy/*`, `cmd/*/main.go`(骨架), `internal/config/*` |
| **启动时机** | Day 1（无依赖，第一个启动） |
| **完成标志** | Gate-1 通过 |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| F-01 | Go module 初始化 + 目录结构 + Makefile | 0.3d | - |
| F-02 | `pkg/model/*` 全量数据模型定义（Go struct + JSON tag） | 0.5d | - |
| F-03 | `pkg/store/redis.go` Redis 客户端封装 + 指标读写接口 | 0.3d | F-01 |
| F-04 | `pkg/store/mysql.go` MySQL 客户端 + GORM 模型 + 迁移 | 0.3d | F-01 |
| F-05 | `pkg/store/kafka.go` Kafka 生产者/消费者封装 | 0.3d | F-01 |
| F-06 | `pkg/store/clickhouse.go` ClickHouse 写入封装 | 0.2d | F-01 |
| F-07 | `deploy/init_mysql.sql` 全表 DDL + 种子数据（82条规则+治理模型+用户） | 0.5d | F-02 |
| F-08 | `deploy/docker-compose.yml` 全套中间件 + 服务骨架 | 0.5d | F-01 |
| F-09 | `internal/config/*` 配置加载框架（Viper/环境变量） | 0.3d | F-01 |
| F-10 | 各 `cmd/*/main.go` 入口骨架（可编译但空逻辑） | 0.3d | F-09 |
| F-11 | Kafka Topic 创建脚本 + Redis Key 命名文档 | 0.2d | F-08 |

**预计工时**: 3.5d（Day 1 ~ Day 4）

---

#### Agent-D (Data) — 数据层

| 属性 | 说明 |
|------|------|
| **职责** | 公共/私有 Ingestor 完整实现 |
| **代码范围** | `internal/ingestor/*`, `pkg/exchange/*`, `cmd/ingestor-public/`, `cmd/ingestor-private/` |
| **启动时机** | Gate-1 通过后（约 Day 3） |
| **完成标志** | Gate-2 通过 |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| D-01 | `pkg/exchange/binance/` Binance API 适配层（WS + REST 客户端） | 1.5d | Gate-1 |
| D-02 | `internal/ingestor/ws/` WS 连接管理器（自动重连 + 心跳） | 1d | D-01 |
| D-03 | `internal/ingestor/rest/` REST 轮询调度器（频控管理 + 动态调速） | 1d | D-01 |
| D-04 | `internal/ingestor/schema/` Schema 校验（字段完整性 + 类型检查） | 0.5d | D-01 |
| D-05 | `internal/ingestor/publish/` 标准化 → Kafka + Redis + ClickHouse 发布 | 0.5d | Gate-1 |
| D-06 | 公共 Ingestor 集成（WS订阅 orderbook/trades/mark/ticker/funding + REST轮询 depth/OI/exchange_info） | 1d | D-02~D-05 |
| D-07 | 私有 Ingestor 集成（WS订阅 orders/positions/balance + REST轮询 position_snap/balance_snap） | 1d | D-02~D-05 |
| D-08 | 数据层自测：验证 Kafka Topic 有数据 + Redis 有最新状态 | 0.5d | D-06, D-07 |

**预计工时**: 7d（Day 3 ~ Day 12）

---

#### Agent-I (Indicator) — 指标层

| 属性 | 说明 |
|------|------|
| **职责** | 指标引擎框架 + 85 个指标的完整实现 |
| **代码范围** | `internal/indicator/*`, `cmd/indicator-engine/` |
| **启动时机** | Gate-1 通过后即可开始框架；Gate-2 通过后联调 |
| **完成标志** | Gate-3 通过 |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| I-01 | `internal/indicator/engine.go` T0~T4 Worker 调度框架 | 1.5d | Gate-1 |
| I-02 | Phase 1/2 串行计算保证（L0/L1先算 → L2/L3后算） | 0.5d | I-01 |
| I-03 | 指标 Registry 机制（指标注册 + 元数据 + 启停控制） | 0.5d | I-01 |
| I-04 | S-Class 指标实现（23个，含 S-014~S-018 健康治理指标） | 1.5d | I-01 |
| I-05 | L-Class 指标实现（清算类） | 0.5d | I-01 |
| I-06 | E-Class 指标实现（敞口类） | 0.5d | I-01 |
| I-07 | P/B/M/C/BASE/STR-Class 指标实现（剩余类别） | 1.5d | I-01 |
| I-08 | 与 Agent-D 联调：接入真实 Kafka 数据验证指标产出 | 0.5d | Gate-2, I-04~I-07 |

**预计工时**: 7d（Day 3 ~ Day 12），与 Agent-D **并行**启动框架，Gate-2 后联调

**关键并行点**：Agent-I 在 Gate-2 之前可以用 **Mock Kafka 数据**开发和单元测试所有指标逻辑，不必等 Agent-D 的 Ingestor 就绪。

---

#### Agent-R (Rule + Alert) — 规则层 + 告警

| 属性 | 说明 |
|------|------|
| **职责** | 规则引擎框架 + 72 条 MVP 规则 + 告警服务 + 健康度评分 |
| **代码范围** | `internal/rule/*`, `internal/health/*`, `internal/alert/*`, `cmd/rule-engine/`, `cmd/alert-service/` |
| **启动时机** | Gate-1 通过后即可开始框架；Gate-3 通过后联调 |
| **完成标志** | Gate-4 通过 |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| R-01 | `internal/rule/engine.go` F0~F4 调度框架 | 1.5d | Gate-1 |
| R-02 | `internal/rule/resolver.go` 四层治理继承解析 + 预编译缓存 | 1d | R-01 |
| R-03 | RiskEvent 产出 + Kafka 发布 + Cooldown 去重（Redis） | 0.5d | R-01 |
| R-04 | S-Class 规则实现（18条，含 S-014~S-018） | 1d | R-01 |
| R-05 | L/E/P/M/B/C/STR-Class 规则实现（54条） | 2.5d | R-01 |
| R-06 | `internal/health/scorer.go` 五维加权 + Hard Cap | 0.5d | R-04 |
| R-07 | `internal/health/state_machine.go` 运行态状态机 | 0.5d | R-06 |
| R-08 | `internal/health/reporter.go` 定时/即时健康报告 | 0.5d | R-07 |
| R-09 | `internal/alert/notifier.go` Telegram Bot L1/L2/L3 通知 | 0.5d | R-03 |
| R-10 | `internal/alert/dedup.go` 告警去重 + 持久化 | 0.3d | R-03 |
| R-11 | 与 Agent-I 联调：接入真实指标数据验证规则触发 | 0.5d | Gate-3 |

**预计工时**: 9.5d（Day 3 ~ Day 14）

**关键并行点**：Agent-R 在 Gate-3 之前用 **Mock 指标值**开发规则逻辑。resolver.go 只需 MySQL 即可测试（Gate-1 即可）。

---

#### Agent-S (Sentinel) — 哨兵层

| 属性 | 说明 |
|------|------|
| **职责** | Watchdog + Canary 探针（两个独立于主链路的守护进程） |
| **代码范围** | `internal/watchdog/*`, `internal/canary/*`, `cmd/watchdog/`, `cmd/canary/` |
| **启动时机** | Gate-1 通过后（几乎无主链路依赖） |
| **完成标志** | Watchdog 能独立报警 + Canary 每 15min 验证 Telegram |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| S-01 | `internal/watchdog/` 进程存活检测（Redis/Kafka/MySQL/各服务心跳） | 1d | Gate-1 |
| S-02 | Watchdog 独立 Telegram 通知渠道（不走主链路的 Alert Service） | 0.5d | S-01 |
| S-03 | `internal/canary/` Canary 探针（独立 Bot Token + sendMessage + getUpdates 验证） | 1d | Gate-1 |
| S-04 | Canary 结果写入 IND-S-018 | 0.3d | S-03 |
| S-05 | Canary + Watchdog 集成测试 | 0.5d | S-01~S-04 |

**预计工时**: 3.3d（Day 3 ~ Day 7）

---

#### Agent-W (Web/UI) — 前端控制台

| 属性 | 说明 |
|------|------|
| **职责** | React + Ant Design Pro 前端 + Dashboard API（Go 后端路由） |
| **代码范围** | `web/*`(新增), `internal/api/*`, `cmd/api-server/` |
| **启动时机** | **Day 1 即可启动**（前端用 Mock API；后端 API 在 Gate-1 后开始） |
| **完成标志** | Gate-5（读取类）+ Gate-6（配置类） |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| W-01 | React 脚手架 + Ant Design Pro + 路由 + 布局 + Mock Service | 1d | - |
| W-02 | JWT 鉴权框架（前端 Token 管理 + 登录页） | 0.5d | W-01 |
| W-03 | 前端：总览 Home 页 | 1d | W-01 |
| W-04 | 前端：健康度 Health 页（5 Tab + Recharts 趋势图） | 1.5d | W-01 |
| W-05 | 前端：告警中心 Alerts 页 | 1d | W-01 |
| W-06 | 前端：对象管理 Objects 页（四层树 + 5 Tab） | 1.5d | W-01 |
| W-07 | 前端：指标库 Indicators 页（4 Tab + 依赖图） | 1d | W-01 |
| W-08 | 前端：规则库 Rules 页（4 视图 + 绑定编辑弹窗） | 2d | W-01 |
| W-09 | 前端：变更中心 Changes 页 | 1d | W-01 |
| W-10 | 前端：权限管理 Access 页 | 0.5d | W-01 |
| W-11 | Go: `internal/api/` Dashboard API 路由框架 + JWT 中间件 + Scope 中间件 | 1d | Gate-1 |
| W-12 | Go: Health/Alerts/Objects/Indicators 读取类 API（~15 端点） | 2d | Gate-4 |
| W-13 | Go: Rules CRUD API + Changes 审批 API + Access API（~15 端点） | 2d | W-12 |
| W-14 | Go: RBAC 完整实现（4 角色 + scope 过滤 + 操作级权限） | 1d | W-11 |
| W-15 | 前端对接真实 API（替换 Mock） | 1.5d | W-12, W-13 |

**预计工时**: 16.5d（Day 1 ~ Day 22）

**关键并行点**：
- W-01 ~ W-10（前端页面）从 **Day 1** 即可开始，使用 Mock API 开发
- W-11（API 框架）Gate-1 后启动
- W-12（读取类 API）Gate-4 后启动（需要真实数据源）
- W-13（配置类 API）紧跟 W-12
- W-15（联调）最后阶段

---

#### Agent-Q (QA) — 测试 + 验收

| 属性 | 说明 |
|------|------|
| **职责** | 编写集成测试、Gate 验收脚本、端到端测试、性能基准 |
| **代码范围** | `tests/*`(新增), 各模块 `*_test.go` |
| **启动时机** | Gate-1 后逐步介入 |
| **完成标志** | 全部 Gate 验收通过 + 端到端测试通过 |

**任务清单**：

| ID | 任务 | 工时 | 依赖 |
|----|------|------|------|
| Q-01 | Gate 验收脚本框架（自动化检查每个 Gate 条件） | 0.5d | Gate-1 |
| Q-02 | 数据层测试：Ingestor → Kafka → Redis 数据流验证 | 0.5d | Gate-2 |
| Q-03 | 指标层测试：85 个指标覆盖率检查 + 衍生指标依赖链验证 | 0.5d | Gate-3 |
| Q-04 | 规则层测试：模拟阈值越限 → 验证 RiskEvent 产出 | 0.5d | Gate-4 |
| Q-05 | 告警端到端测试：RiskEvent → Telegram 消息到达 | 0.3d | Gate-4 |
| Q-06 | 健康度测试：模拟维度降级 → 验证 Hard Cap + 状态机切换 | 0.5d | Gate-4 |
| Q-07 | API 测试：读取类 + 配置类 API 全覆盖 | 1d | Gate-5 |
| Q-08 | RBAC 测试：4 角色 × 关键操作矩阵验证 | 0.5d | Gate-6 |
| Q-09 | 端到端全流程测试 | 1d | Gate-6 |
| Q-10 | 性能基准：指标计算延迟 + 规则评估延迟 + API 响应时间 | 0.5d | Q-09 |

**预计工时**: 5.8d（分散在整个开发周期）

---

## 3. 并行时序图

### 3.1 全景甘特图

```
Day:  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25
      │           │                    │                 │                    │
      ▼           ▼                    ▼                 ▼                    ▼
     Start      Gate-1              Gate-2/3          Gate-4/5             Gate-6
                                                                          MVP Done

Agent-F ████████████░
         F-01~F-11  │
                    │
Agent-D             ░░░░████████████████████░
                       D-01~D-05  D-06~D-08 │
                                            │
Agent-I             ░░░░████████████████████░
                       I-01~I-03  I-04~I-07 I-08
                                            │
Agent-R             ░░░░████████████████████████████░
                       R-01~R-03  R-04~R-05   R-06~R-11
                                              │
Agent-S             ░░░░████████░             │
                       S-01~S-05              │
                                              │
Agent-W ████████████████████████████░░░░░░░░░░████████████████████████████░
         W-01~W-10(Mock前端)        W-11  W-12~W-13  W-14  W-15
                                              │
Agent-Q         ░░░░░░░░░░░░░░░░░░░░░░████████████████████████████████████░
                 Q-01        Q-02~Q-06         Q-07~Q-10

Legend: ████ = 活跃开发  ░░░░ = 等待/准备  │ = Gate 检查点
```

### 3.2 Sprint 划分

| Sprint | 天数 | 活跃 Agent | 目标 Gate | 交付物 |
|--------|------|-----------|----------|--------|
| **Sprint 0** | Day 1~4 | F, W(前端Mock) | Gate-1 | 基座就绪：DB+中间件+模型+脚手架 |
| **Sprint 1** | Day 3~10 | D, I, R, S, W(前端) | Gate-2 | 数据管道就绪 + Watchdog/Canary |
| **Sprint 2** | Day 8~14 | D, I, R, Q | Gate-3, Gate-4 | 指标+规则+告警就绪 |
| **Sprint 3** | Day 12~18 | R, W(API), Q | Gate-5 | Dashboard 读取类 API 就绪 |
| **Sprint 4** | Day 16~22 | W(API+联调), Q | Gate-6 | 配置类 API + RBAC + 全量联调 |
| **Sprint 5** | Day 22~25 | W, Q | MVP Done | 端到端测试 + Bug Fix + 交付 |

### 3.3 Agent 并行度分析

| 时间段 | 并行 Agent 数 | 说明 |
|--------|-------------|------|
| Day 1~3 | **2** | F(基座) + W(前端Mock) |
| Day 3~7 | **6** | D + I + R + S + W + Q(准备) — **最大并行度** |
| Day 7~14 | **5** | D + I + R + W + Q |
| Day 14~18 | **3** | W(API) + R(健康度) + Q |
| Day 18~25 | **2** | W(联调) + Q |

**瓶颈分析**：关键路径是 `F → D/I → R → W(API) → W(联调)`，总长约 22~25 天。
不在关键路径上的 Agent（S, Q）不影响总工期。

---

## 4. Orchestrator 调度协议

### 4.1 调度模型

Orchestrator 采用 **事件驱动 + Gate 检查** 的混合调度模型：

```
┌─────────────────────────────────────────────────────┐
│                  Orchestrator                        │
│                                                     │
│  ┌──────────────┐   ┌───────────────────────────┐  │
│  │ Task Queue   │   │ Gate Registry             │  │
│  │              │   │                           │  │
│  │ [F-01] ready │   │ Gate-1: [✅✅✅✅] OPEN   │  │
│  │ [D-01] wait  │   │ Gate-2: [✅❌❌] CLOSED  │  │
│  │ [I-01] wait  │   │ Gate-3: [❌❌❌] CLOSED  │  │
│  │ ...          │   │ ...                       │  │
│  └──────┬───────┘   └──────────┬────────────────┘  │
│         │                      │                    │
│         ▼                      ▼                    │
│  ┌──────────────────────────────────────────┐      │
│  │ Scheduler Loop                           │      │
│  │                                          │      │
│  │ 1. 检查所有 Gate 状态                     │      │
│  │ 2. 对每个 Agent 的待办任务:               │      │
│  │    - 依赖 Gate 已 OPEN? → 分发任务       │      │
│  │    - 依赖 Gate 未 OPEN? → 保持等待       │      │
│  │ 3. Agent 完成任务 → 更新 Gate 条件        │      │
│  │ 4. Gate 条件全部满足 → Gate OPEN          │      │
│  │ 5. 触发 Agent-Q 执行对应 Gate 验收        │      │
│  └──────────────────────────────────────────┘      │
│                                                     │
│  ┌──────────────────────────────────────────┐      │
│  │ Event Bus                                │      │
│  │                                          │      │
│  │ Agent-F:DONE(F-07) → check Gate-1        │      │
│  │ Agent-D:DONE(D-08) → check Gate-2        │      │
│  │ Agent-I:DONE(I-08) → check Gate-3        │      │
│  │ Agent-R:DONE(R-11) → check Gate-4        │      │
│  │ Agent-W:DONE(W-12) → check Gate-5        │      │
│  │ Agent-W:DONE(W-14) → check Gate-6        │      │
│  └──────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────┘
```

### 4.2 调度指令格式

Orchestrator 向 Agent 发送的调度指令：

```yaml
# 任务分发指令
type: TASK_ASSIGN
agent: Agent-D
tasks:
  - id: D-01
    description: "Binance API 适配层"
    files: ["pkg/exchange/binance/"]
    gate_dependency: Gate-1
    estimated_hours: 12
    acceptance_criteria:
      - "binance.NewClient() 可连接 WS"
      - "binance.NewRESTClient() 可调用 depth API"
    context_docs:
      - "docs/risk_control_plan_v3.0.md §8.2"
      - "docs/indicator_library_v2.0.md"

# Gate 检查指令
type: GATE_CHECK
gate: Gate-2
checks:
  - name: "Kafka public topic 有数据"
    command: "kafka-console-consumer --topic public.binance.orderbook --max-messages 1"
    expect: "非空 JSON"
  - name: "Redis 有公共数据最新状态"
    command: "redis-cli EXISTS ind:IND-S-001:binance"
    expect: "1"
  - name: "Kafka private topic 有数据"
    command: "kafka-console-consumer --topic private.binance.orders --max-messages 1"
    expect: "非空 JSON"

# 阻塞升级指令
type: BLOCK_ESCALATION
agent: Agent-I
blocked_task: I-08
reason: "Gate-2 未通过，Agent-D D-06 延迟"
action: "Agent-D 优先处理 D-06，Agent-I 继续用 Mock 数据开发 I-07"
```

### 4.3 Agent 间通信协议

Agent 之间**不直接通信**，所有协调通过 Orchestrator 中转：

```
Agent-I 需要 Agent-D 的 Kafka Topic Schema?
  → Agent-I 向 Orchestrator 发 REQUEST_CONTRACT
  → Orchestrator 从 Agent-D 获取 Schema 或引用 docs/
  → Orchestrator 将 Schema 发给 Agent-I

Agent-R 发现 pkg/model/ 缺少字段?
  → Agent-R 向 Orchestrator 发 CHANGE_REQUEST(pkg/model/risk_event.go)
  → Orchestrator 评估影响范围（是否影响其他 Agent）
  → 无冲突 → 批准 Agent-R 修改
  → 有冲突 → 协调相关 Agent 对齐
```

### 4.4 冲突仲裁规则

| 冲突类型 | 仲裁策略 |
|---------|---------|
| 两个 Agent 需要改同一个文件 | Orchestrator 指定主修改方，另一方提 PR review |
| 接口契约不一致 | 以设计文档为准，Orchestrator 更新接口定义 |
| Gate 依赖阻塞 | 被阻塞 Agent 继续用 Mock 开发，不 idle 等待 |
| 进度延迟 | Orchestrator 从其他已完成的 Agent 抽调算力支援 |

---

## 5. 接口契约（Agent 间解耦的关键）

### 5.1 数据层 → 指标层 契约

```go
// Kafka Message Schema (Agent-D 产出, Agent-I 消费)
type MarketDataMessage struct {
    Exchange   string    `json:"exchange"`
    Symbol     string    `json:"symbol"`
    DataType   string    `json:"data_type"`   // orderbook|trade|mark_price|...
    Data       json.RawMessage `json:"data"`
    ExchangeTS int64     `json:"exchange_ts"` // 交易所时间戳 ms
    ReceivedAt int64     `json:"received_at"` // 本地接收时间戳 ms
    Freshness  string    `json:"freshness"`   // fresh|stale|unknown
}

// Redis Key Convention (Agent-D 写入, Agent-I/R 读取)
// 原始数据: raw:{exchange}:{data_type}:{symbol}
// 指标值:   ind:{indicator_id}:{grain_values}
```

### 5.2 指标层 → 规则层 契约

```go
// Kafka Indicator Event (Agent-I 产出, Agent-R 消费)
type IndicatorEvent struct {
    IndicatorID string  `json:"indicator_id"` // IND-S-001
    Value       float64 `json:"value"`
    Grain       string  `json:"grain"`        // binance:BTC-USDT:acct001
    Timestamp   int64   `json:"timestamp"`
    IsSignal    bool    `json:"is_signal"`    // signal 类型标记
}

// Redis 读取约定 (Agent-R 从 Redis 读 Agent-I 写入的指标)
// Key: ind:{indicator_id}:{grain}
// Value: JSON { "value": 7.8, "ts": 1712000000, "fresh": true }
```

### 5.3 规则层 → 告警/API 契约

```go
// RiskEvent (Agent-R 产出, Agent-W API 读取)
type RiskEvent struct {
    EventID     string `json:"event_id"`
    RuleCode    string `json:"rule_code"`    // S-014
    Severity    string `json:"severity"`     // L1|L2|L3
    Priority    string `json:"priority"`     // P0|P1|P2|P3
    ScopeType   string `json:"scope_type"`   // global|project|team|strategy|account
    ScopeID     string `json:"scope_id"`
    TriggerValue float64 `json:"trigger_value"`
    Threshold   float64 `json:"threshold"`
    Message     string `json:"message"`
    Status      string `json:"status"`       // open|ack|handling|resolved
    CreatedAt   int64  `json:"created_at"`
}
```

### 5.4 Dashboard API 契约（Agent-W 后端提供, 前端消费）

```yaml
# 读取类 API（Gate-5 交付）
GET /api/v1/health/score          → { score, state, dimensions[], external_summary }
GET /api/v1/health/dimensions     → { dimensions[]{name, score, items[]} }
GET /api/v1/health/rules          → { rules[]{code, name, status, last_eval, current_value} }
GET /api/v1/health/reports        → { reports[]{time, type, score, state, findings} }
GET /api/v1/alerts?level=&status= → { total, items[]{RiskEvent} }
GET /api/v1/alerts/:id            → { RiskEvent + detail }
PATCH /api/v1/alerts/:id          → { action: ack|resolve|false_positive }
GET /api/v1/objects/tree           → { tree: nested object hierarchy }
GET /api/v1/objects/:id            → { object detail }
GET /api/v1/objects/:id/rules      → { effective_rules[]{code, status, source, threshold} }
GET /api/v1/objects/:id/indicators → { indicators[]{id, value, updated_at, status} }
GET /api/v1/indicators             → { items[]{id, name, category, level, type, status} }
GET /api/v1/indicators/runtime     → { items[]{id, last_update, age, coverage, p99} }

# 配置类 API（Gate-6 交付）
GET    /api/v1/rules/templates                → { templates[] }
GET    /api/v1/rules/:code/bindings           → { binding_tree }
GET    /api/v1/rules/diff?rule=&left=&right=  → { diff }
POST   /api/v1/rules/bindings                 → { created binding }
PUT    /api/v1/rules/bindings/:id             → { updated binding }
DELETE /api/v1/rules/bindings/:id             → {}
GET    /api/v1/changes                        → { items[] }
POST   /api/v1/changes                        → { draft }
PATCH  /api/v1/changes/:id/approve            → {}
PATCH  /api/v1/changes/:id/publish            → {}
PATCH  /api/v1/changes/:id/rollback           → {}
GET    /api/v1/users                          → { users[] }
POST   /api/v1/users                          → { created user }
PUT    /api/v1/users/:id                      → { updated user }
```

---

## 6. Orchestrator 实现方案

### 6.1 技术选型

Orchestrator 本身可以是：

| 方案 | 适用场景 | 复杂度 |
|------|---------|--------|
| **A: 人工 Orchestrator**（推荐 MVP） | 团队 ≤5 人，每日站会 + 看板 | 最低 |
| **B: Claude Agent SDK** | 全自动 AI Agent 开发 | 中 |
| **C: Temporal/Airflow** | 大规模自动化工作流 | 高 |

**推荐方案 A+B 混合**：人工把控 Gate 决策和架构仲裁，Claude Agent SDK 执行具体编码任务。

### 6.2 Claude Agent SDK 实现架构

```
┌────────────────────────────────────────────────────────────┐
│  Orchestrator Process (Python / TypeScript)                │
│                                                            │
│  ┌──────────────────────────────────────────────────┐     │
│  │  Task DAG (有向无环图)                            │     │
│  │                                                   │     │
│  │  F-01 ──→ F-03 ──→ F-08 ──→ (Gate-1)             │     │
│  │  F-01 ──→ F-04 ──→ F-07 ──→ (Gate-1)             │     │
│  │                       │                            │     │
│  │  (Gate-1) ──→ D-01 ──→ D-06 ──→ D-08 → (Gate-2) │     │
│  │  (Gate-1) ──→ I-01 ──→ I-04 ──→ I-08 → (Gate-3) │     │
│  │  (Gate-1) ──→ R-01 ──→ R-04 ──→ R-11 → (Gate-4) │     │
│  │  (Gate-1) ──→ S-01 ──→ S-03 ──→ S-05             │     │
│  │           ──→ W-01 ──→ W-03 ~ W-10               │     │
│  │  (Gate-4) ──→ W-12 ──→ W-13 ──→ W-15 → (Gate-6) │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  ┌──────────────────────────────────────────────────┐     │
│  │  Agent Pool (Claude Agent SDK Subprocesses)       │     │
│  │                                                   │     │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐            │     │
│  │  │Agent-F  │ │Agent-D  │ │Agent-I  │ ...         │     │
│  │  │worktree │ │worktree │ │worktree │             │     │
│  │  │ /tmp/f  │ │ /tmp/d  │ │ /tmp/i  │             │     │
│  │  └────┬────┘ └────┬────┘ └────┬────┘             │     │
│  │       │           │           │                   │     │
│  │       └───────────┴───────────┘                   │     │
│  │                   │                               │     │
│  │                   ▼                               │     │
│  │          Git Main Branch                          │     │
│  │          (merge via Orchestrator review)           │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  ┌──────────────────────────────────────────────────┐     │
│  │  Scheduler Loop (每 60s 执行)                     │     │
│  │                                                   │     │
│  │  for task in task_dag.ready_tasks():              │     │
│  │      agent = select_agent(task)                   │     │
│  │      if agent.is_idle():                          │     │
│  │          agent.assign(task)                       │     │
│  │                                                   │     │
│  │  for agent in agent_pool.completed():             │     │
│  │      result = agent.get_result()                  │     │
│  │      task_dag.mark_done(result.task_id)           │     │
│  │      gate_registry.update(result)                 │     │
│  │      if gate_registry.any_newly_open():           │     │
│  │          trigger_qa_verification()                │     │
│  │          notify_downstream_agents()               │     │
│  └──────────────────────────────────────────────────┘     │
└────────────────────────────────────────────────────────────┘
```

### 6.3 每个 Agent 的 System Prompt 模板

```markdown
# Agent-{X} System Prompt

## 你的角色
你是 CEX 做市风控系统的 {角色名} 开发 Agent。

## 你的代码职责范围
只允许修改以下目录：
- {目录列表}

## 当前任务
{Orchestrator 分发的任务描述}

## 接口契约
{与上下游 Agent 的数据契约}

## 参考文档
{相关设计文档路径}

## 约束
- 不要修改职责范围外的文件
- 所有对外接口必须严格遵守契约定义
- 完成后运行单元测试并报告结果
- 遇到需要修改共享代码的情况，报告给 Orchestrator，不要自行修改
```

---

## 7. 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| Agent 间接口理解不一致 | 集成失败 | §5 的接口契约文档作为单一真相源；Orchestrator 在分发任务时同步契约 |
| Gate-2 延迟（交易所 API 不稳定） | 下游 Agent 阻塞 | Agent-I/R 用 Mock 数据继续开发，不 idle |
| 多 Agent 同时修改 pkg/model/ | Git 冲突 | pkg/model/ 由 Agent-F 一次性定义完整，后续修改需 Orchestrator 审批 |
| 前端 Mock API 与真实 API 不一致 | 联调返工 | §5.4 的 API 契约 + Swagger 文档在 Gate-1 后由 Agent-W 率先定义 |
| 单个 Agent 进度落后 | 整体延期 | Orchestrator 从已完成 Agent 调配资源支援；优先保关键路径 |

---

## 8. 总结

| 维度 | 串行方案 | 多 Agent 并行 | 压缩比 |
|------|---------|-------------|--------|
| 总工期 | ~6 周 (30 工作日) | ~5 周 (25 工作日) | **17%** |
| 峰值并行度 | 1 | 6 Agent | — |
| 最大风险 | 单点瓶颈 | 集成冲突 | — |
| Gate 检查点 | 无 | 6 个 | — |

**关键收益**：
- Agent-W（前端）从 Day 1 开始，与后端完全并行
- Agent-S（哨兵）独立开发，不阻塞主链路
- Agent-D 和 Agent-I 并行启动框架，仅在联调阶段串行
- Gate 机制保证每个阶段的质量，避免"都写完了但集成不起来"
