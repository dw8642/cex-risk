# CEX 做市风控系统 — MVP 开发方案

> **版本**：V1.0
> **日期**：2026-04-02
> **定位**：从完整方案（`risk_control_plan_v3.0.md`）中提取 MVP 范围，定义分阶段交付计划、技术任务拆解、验收标准和里程碑。
> **技术栈**：Go（在线核心链路）+ React/Ant Design Pro（Dashboard）+ MySQL + Redis + Kafka + ClickHouse

---

## 1. MVP 目标与边界

### 1.1 MVP 必须回答的问题

| # | 问题 | 对应能力 |
|---|------|---------|
| 1 | 系统有没有坏？ | 健康度评分 + 五维分解 + 状态机 |
| 2 | 坏在哪层？影响谁？ | 链路状态 + 对象管理 + 生效规则 |
| 3 | 有没有高危风险正在发生？ | 72 条 MVP 规则 + Telegram 告警 |
| 4 | 规则配置怎么管？谁能改什么？ | 规则模板+绑定+继承 + RBAC + 审批 |
| 5 | 告警链路本身可靠吗？ | Canary 探针 + Watchdog |

### 1.2 MVP 不做什么（明确排除）

| 排除项 | 原因 | 延后到 |
|--------|------|--------|
| R3 自动平仓控制动作 | 风险极高，需充分回测 | V1.0 |
| 演进层（规则回测、基线校准） | MVP 先跑通管道 | V2.0 |
| 多交易所（>2） | MVP 只对接 Binance(+1 可选) | V1.0 |
| 微观结构风控 | 依赖演进层能力 | V2.0 |
| 级联失效自动检测 | 需要更多运行数据 | V1.0 |
| 移动端 | 先保证 Web 控制台 | V2.0 |
| 10 条 Deferred 规则 | 非 MVP 关键路径 | V1.0 |

### 1.3 MVP 规模基准

| 维度 | MVP 范围 |
|------|---------|
| 交易所 | 1~2（Binance 必选 + OKX 可选） |
| 交易对 | ~10 symbols |
| 账户 | ~5 |
| 项目/团队/策略 | 1 Project + 2 Teams + 2~3 Strategies |
| 规则 | 72 条（MVP 规则） |
| 指标 | 85 个 |
| 用户 | ~10（SA×2 + Admin×3 + Trader×3 + Viewer×2） |

---

## 2. 三阶段交付计划

### 总览

```
Phase 1: 核心管道          Phase 2: 观察层控制台       Phase 3: 配置层+治理层
  ~2 周                      ~2 周                       ~2 周
┌──────────────┐          ┌──────────────┐           ┌──────────────┐
│ Data Layer   │          │ Home Page    │           │ Rules Config │
│ Indicator Eng│          │ Health Page  │           │ Binding Edit │
│ Rule Engine  │          │ Alerts Page  │           │ Changes/Audit│
│ Alert Service│          │ Objects Page │           │ Full RBAC    │
│ Watchdog     │          │ RBAC Basic   │           │ Canary Probe │
│ Health Score │          │ Dashboard API│           │ Indicators   │
│ DB + Infra   │          │   (读取类)   │           │ Access Mgmt  │
└──────────────┘          └──────────────┘           └──────────────┘
       │                         │                          │
       ▼                         ▼                          ▼
  告警可达 ✓               值班可看 ✓                管理可配 ✓
```

---

## 3. Phase 1：核心管道（~2 周）

### 3.1 目标

> 三级管道（Data → Indicator → Rule）端到端跑通，72 条 MVP 规则生效，Telegram 告警可达，健康度评分可见。

### 3.2 技术任务拆解

#### Week 1：数据层 + 指标层

| # | 任务 | 产出 | 工时估算 | 依赖 |
|---|------|------|---------|------|
| 1.1 | MySQL Schema 初始化（全表） | `init_mysql.sql`：四层治理表 + rule_bindings + risk_rules + api_key_configs + risk_events + users/user_scopes + config_drafts/audit_logs | 0.5d | - |
| 1.2 | MySQL 种子数据（规则+治理模型） | 82条规则配置 + 1 Project + 2 Teams + 5 Accounts + Global 默认绑定 | 0.5d | 1.1 |
| 1.3 | Redis Key 命名规范与初始化 | `ind:{id}:{grain}` 命名、TTL 策略文档 | 0.3d | - |
| 1.4 | Kafka Topic 规划与创建 | public.*/private.*/indicator.*/risk.events/system.config_change | 0.3d | - |
| 1.5 | 公共 Ingestor（Binance） | WS 连接管理 + REST 轮询 + Schema 校验 + 标准化 + 新鲜度标记 + 发布到 Kafka/Redis | 2d | 1.3, 1.4 |
| 1.6 | 私有 Ingestor（Binance） | WS(orders/positions/balance) + REST(position_snap/balance_snap) + 频控管理 | 2d | 1.3, 1.4 |
| 1.7 | 指标引擎框架 | T0~T4 Worker 调度框架 + Phase 1/2 串行计算保证 + Redis 读写 + Kafka 发布 | 1.5d | 1.3, 1.4 |
| 1.8 | 指标实现：S-Class（23个） | IND-S-001~S-018（含新增 S-014~S-018 健康治理指标） | 1.5d | 1.7 |
| 1.9 | 指标实现：L/E/P/M/B/C/BASE/STR（62个） | 按类别实现，每类一个 Go 文件 | 2d | 1.7 |

#### Week 2：规则层 + 告警 + 健康治理

| # | 任务 | 产出 | 工时估算 | 依赖 |
|---|------|------|---------|------|
| 2.1 | 规则引擎框架 | F0~F4 调度框架 + 四层治理继承解析（预编译缓存） + RiskEvent 产出 + 去重(cooldown) | 2d | 1.1 |
| 2.2 | 规则实现：72条 MVP 规则 | 按类别实现，每条规则一个函数，阈值从配置读取 | 3d | 2.1 |
| 2.3 | 告警服务 | Kafka 消费 RiskEvent → L1/L2/L3 分级 → Telegram Bot 发送 + MySQL 持久化 | 1d | 2.1 |
| 2.4 | Watchdog | 独立进程，监控全链路存活（WS/Kafka/Redis/MySQL/Indicator/Rule Engine 心跳） | 1d | - |
| 2.5 | S-017 健康度评分规则 | 五维加权计算 + Hard Cap + 状态机 + 定时报告 + 即时报告 | 1d | 1.8, 2.1 |
| 2.6 | Docker Compose 编排 | 全套服务 + 中间件，`docker-compose up` 一键启动 | 0.5d | ALL |
| 2.7 | 端到端联调测试 | WS 数据 → 指标 → 规则 → RiskEvent → Telegram 消息 | 1d | ALL |

### 3.3 Phase 1 验收标准

| # | 验收项 | 验收方法 |
|---|--------|---------|
| ✅ | 公共 WS 数据持续写入 Redis | `redis-cli get ind:IND-S-001:binance` |
| ✅ | 私有 WS 数据持续写入 Redis | `redis-cli get ind:IND-L-001:binance:BTC-USDT:acct001` |
| ✅ | 85 个指标全部有产出 | 指标覆盖率检查脚本 |
| ✅ | 触发测试告警后 Telegram 收到消息 | 手动触发或模拟阈值越限 |
| ✅ | 健康度评分写入 Redis | `redis-cli get ind:IND-S-017:global` |
| ✅ | Watchdog 独立运行并能报警 | 停掉 Rule Engine → Watchdog 发 Telegram |
| ✅ | Docker Compose 一键启动 | `docker-compose up -d` 后全部服务 healthy |

### 3.4 Phase 1 代码结构

```
cex-risk/
├── cmd/
│   ├── ingestor-public/    # 公共 Ingestor 入口
│   ├── ingestor-private/   # 私有 Ingestor 入口
│   ├── indicator-engine/   # 指标引擎入口
│   ├── rule-engine/        # 规则引擎入口
│   ├── alert-service/      # 告警服务入口
│   └── watchdog/           # Watchdog 入口
├── internal/
│   ├── ingestor/           # Ingestor 核心逻辑
│   │   ├── ws/             # WS 连接管理
│   │   ├── rest/           # REST 轮询
│   │   ├── schema/         # Schema 校验
│   │   └── publish/        # Kafka+Redis 发布
│   ├── indicator/          # 指标引擎
│   │   ├── engine.go       # T0~T4 调度
│   │   ├── s_class.go      # S-系统指标（含 S-014~S-018）
│   │   ├── l_class.go      # L-清算指标
│   │   ├── e_class.go      # E-敞口指标
│   │   └── ...             # 其他类别
│   ├── rule/               # 规则引擎
│   │   ├── engine.go       # F0~F4 调度
│   │   ├── resolver.go     # 四层治理继承解析
│   │   ├── s_class.go      # S-系统规则（含 S-014~S-018）
│   │   └── ...             # 其他类别
│   ├── health/             # 健康治理子系统
│   │   ├── scorer.go       # 五维加权 + Hard Cap
│   │   ├── state_machine.go # 运行态状态机
│   │   └── reporter.go     # 健康报告生成
│   ├── alert/              # 告警服务
│   │   ├── notifier.go     # Telegram 通知
│   │   └── dedup.go        # 告警去重
│   ├── watchdog/           # Watchdog
│   └── config/             # 配置加载
├── pkg/
│   ├── exchange/           # 交易所 API 适配层
│   │   ├── binance/
│   │   └── okx/
│   ├── store/              # 存储抽象
│   │   ├── redis.go
│   │   ├── mysql.go
│   │   ├── kafka.go
│   │   └── clickhouse.go
│   └── model/              # 数据模型
├── deploy/
│   ├── docker-compose.yml
│   └── init_mysql.sql
└── docs/                   # 设计文档
```

---

## 4. Phase 2：观察层控制台（~2 周）

### 4.1 目标

> 值班人员可以通过浏览器看到系统全貌：健康度、告警、对象状态、指标状态。

### 4.2 技术任务拆解

#### Week 3：前端框架 + Home + Health

| # | 任务 | 产出 | 工时估算 | 依赖 |
|---|------|------|---------|------|
| 3.1 | Dashboard 前端脚手架 | React + Ant Design Pro + 路由 + 布局（左侧导航+顶栏） | 1d | - |
| 3.2 | JWT 鉴权框架 | 登录页 + Token 管理 + 请求拦截器 + 角色信息注入 | 0.5d | - |
| 3.3 | Go Dashboard API 路由组 | Gin/Echo 路由框架 + CORS + JWT 中间件 + Swagger | 0.5d | - |
| 3.4 | Health API（5个端点） | `GET /health/score`, `/health/dimensions`, `/health/rules`, `/health/capacity`, `/health/reports` | 1d | P1 |
| 3.5 | Alerts API（3个端点） | `GET /alerts`(list+filter), `GET /alerts/:id`, `PATCH /alerts/:id`(ack/resolve) | 1d | P1 |
| 3.6 | 总览 Home 页面 | 健康度卡片 + 告警统计 + 链路状态 + 高风险对象 + 最近告警 | 1.5d | 3.3, 3.4, 3.5 |
| 3.7 | 健康度 Health 页面 | 5 Tab（全局面板+五维分解+健康规则+容量+报告），趋势图用 Recharts | 2d | 3.4 |

#### Week 4：Alerts + Objects + RBAC 基础

| # | 任务 | 产出 | 工时估算 | 依赖 |
|---|------|------|---------|------|
| 4.1 | Objects API（5个端点） | `GET /objects/tree`, `GET /objects/:id`, `/objects/:id/rules`, `/objects/:id/indicators`, `/objects/:id/alerts` | 1.5d | P1 |
| 4.2 | Indicators API（3个端点） | `GET /indicators`(catalog), `GET /indicators/runtime`, `GET /indicators/:id` | 1d | P1 |
| 4.3 | 告警中心 Alerts 页面 | 多维筛选 + 事件列表 + 详情抽屉 + 确认/处理/关闭按钮 | 1.5d | 3.5 |
| 4.4 | 对象管理 Objects 页面 | 左侧四层治理树 + 右侧 5 Tab（概览/生效规则/指标/告警/变更） | 2d | 4.1 |
| 4.5 | RBAC 基础实现 | users 表 + JWT claims 含 role + 前端角色判断 + 菜单可见性 | 1d | 3.2 |
| 4.6 | 联调测试 + Bug Fix | 各页面数据联调，确保实时刷新 | 1d | ALL |

### 4.3 Phase 2 验收标准

| # | 验收项 | 验收方法 |
|---|--------|---------|
| ✅ | 浏览器登录后看到 Home 页 | 访问 URL，健康度分数与 Redis 一致 |
| ✅ | Health 页五维分解可展开 | 点击每个维度卡片，看到子指标 |
| ✅ | Alerts 页可筛选+确认告警 | 筛选 L2 告警 → 点确认 → 状态变为 ack |
| ✅ | Objects 页可看到生效规则+来源 | 选中账户 → 生效规则 Tab → 可见 Global/Project/Team 标注 |
| ✅ | SA 和 Viewer 登录看到不同菜单 | Viewer 看不到 Changes 和 Access |

---

## 5. Phase 3：配置层与治理层（~2 周）

### 5.1 目标

> 管理员可以通过控制台配置规则绑定、审批变更，完整 RBAC 4 角色生效。

### 5.2 技术任务拆解

#### Week 5：规则配置 + 变更审批

| # | 任务 | 产出 | 工时估算 | 依赖 |
|---|------|------|---------|------|
| 5.1 | Rules API（7个端点） | `GET /rules/templates`, `GET /rules/:code/bindings`(规则视图), `GET /objects/:id/effective-rules`(对象视图), `GET /rules/diff`, `POST /rules/bindings`(创建), `PUT /rules/bindings/:id`, `DELETE /rules/bindings/:id` | 2d | P1 |
| 5.2 | Changes API（5个端点） | `GET /changes`, `POST /changes`(提交草稿), `PATCH /changes/:id/approve`, `PATCH /changes/:id/publish`, `PATCH /changes/:id/rollback` | 1.5d | 5.1 |
| 5.3 | 规则库 Rules 页面 | 4 视图：模板列表 + 规则视图(分层绑定树) + 对象视图 + 差异视图 | 2.5d | 5.1 |
| 5.4 | 规则绑定编辑弹窗 | 继承/覆盖/显式关闭 radio 表单 + 保存为草稿 | 1d | 5.1, 5.2 |
| 5.5 | 变更中心 Changes 页面 | 变更列表 + 风险分级展示 + 审批/拒绝/发布/回滚操作 + 预览(影响分析) | 1.5d | 5.2 |

#### Week 6：指标配置 + Canary + 完整 RBAC + 权限管理

| # | 任务 | 产出 | 工时估算 | 依赖 |
|---|------|------|---------|------|
| 6.1 | Indicators 配置 API + 页面 | 指标目录 + 运行态 + 运行参数配置（Admin+ 可编辑） | 1.5d | P2 |
| 6.2 | 指标依赖图组件 | SVG/React Flow 可视化指标→规则依赖关系 | 0.5d | 6.1 |
| 6.3 | Canary 探针服务 | 独立容器，每 15min 发 Telegram 测试消息 + getUpdates 验证 + 写 IND-S-018 | 1d | P1 |
| 6.4 | S-018 Canary 规则实现 | 连续失败计数 + L2/L3 升级逻辑 | 0.5d | 6.3 |
| 6.5 | 完整 RBAC 实现 | user_scopes 表 + scope 中间件 + Admin/Trader 角色权限矩阵 + 操作级+字段级控制 | 1.5d | P2 |
| 6.6 | Access 权限管理页面 | 用户列表 + 角色管理 + Scope 绑定 + 权限矩阵展示 | 1d | 6.5 |
| 6.7 | 变更风险分级自动计算 | 根据变更内容自动标记 🟢/🟡/🔴 + 匹配审批要求 | 0.5d | 5.2 |
| 6.8 | 全量联调 + 端到端测试 | 所有页面+角色+操作的完整测试 | 1.5d | ALL |

### 5.3 Phase 3 验收标准

| # | 验收项 | 验收方法 |
|---|--------|---------|
| ✅ | 规则视图可看到分层绑定树 | 选 E-001 → 看到 Global/Project/Team/Account 的覆盖配置 |
| ✅ | 对象视图可看到生效规则 + 来源 | 选币安合约账户-01 → 看到 42 条生效规则 |
| ✅ | 差异视图可对比默认 vs 生效 | 选 E-001 + 账户 → 看到 threshold 10→12 |
| ✅ | 规则绑定可编辑并走审批流 | Trader 编辑 → 保存草稿 → Admin 审批 → 发布 → 规则引擎热加载 |
| ✅ | Canary 每 15min 验证一次 | Canary 结果时间线可见 |
| ✅ | 4 角色权限正确隔离 | Trader 看不到 Access 页，不能审批高风险变更 |
| ✅ | 回滚功能可用 | SA 回滚某个已发布的变更 → 规则恢复原配置 |

---

## 6. 技术风险与应对

| 风险 | 影响 | 应对策略 | Phase |
|------|------|---------|-------|
| 交易所 API 变更 | Ingestor 数据异常 | Schema 校验 + 交易所适配层抽象 + 快速切换 | P1 |
| Kafka consumer lag 积压 | 告警延迟 | S-014 规则监控 + skip-to-latest 机制 | P1 |
| 四层继承解析性能 | 规则评估延迟 | 预编译缓存 + 增量更新 | P1 |
| Redis 滑动窗口泄漏 | 内存增长 | ZREMRANGEBYSCORE + 实体级 TTL + 定时清理 | P1 |
| 前端状态管理复杂 | 规则配置页 Bug | Ant Design Pro 内置状态管理 + 接口分层 | P2/P3 |
| RBAC Scope 过滤遗漏 | 数据泄露 | 统一 scope 中间件 + 测试覆盖 | P3 |
| 规则热更新与运行时冲突 | 配置不一致 | Kafka config_change 事件 + 版本号对比 | P3 |

---

## 7. 里程碑时间线

```
Week 0          Week 2          Week 4          Week 6
  │               │               │               │
  │  Phase 1      │  Phase 2      │  Phase 3      │
  │  核心管道      │  观察层控制台  │  配置+治理     │
  │               │               │               │
  ├── W1: Data    ├── W3: Home    ├── W5: Rules   │
  │   + Indicator │   + Health    │   + Changes   │
  │               │               │               │
  ├── W2: Rule    ├── W4: Alerts  ├── W6: RBAC    │
  │   + Alert     │   + Objects   │   + Canary    │
  │   + Health    │   + RBAC基础  │   + 联调      │
  │               │               │               │
  ▼               ▼               ▼               ▼
  M1: 告警可达     M2: 值班可看    M3: 管理可配    MVP Done ✓
```

| 里程碑 | 日期（预估） | 标志 |
|--------|-------------|------|
| M0: 环境就绪 | Day 1 | Docker Compose + MySQL Schema + Kafka Topics |
| M1: 告警可达 | Week 2 | 三级管道端到端 + Telegram 收到告警 |
| M2: 值班可看 | Week 4 | 浏览器打开控制台，30 秒内看清系统状态 |
| M3: 管理可配 | Week 6 | 规则可配置 + 变更可审批 + 4 角色隔离 |

---

## 8. 文档依赖索引

本 MVP 方案依赖以下设计文档，开发时需同步参考：

| 文档 | 用途 |
|------|------|
| `risk_control_plan_v3.0.md` | 完整系统架构（本方案的上层文档） |
| `rule_library_v3.0_checklist.md` | 82 条规则的完整定义（阈值、动作、时间策略） |
| `indicator_library_v2.0.md` | 85 个指标的完整定义（公式、层级、粒度） |
| `system_health_score_design.md` | 健康治理子系统设计（五维权重、Hard Cap、状态机） |
| `dashboard_design.md` | 风控控制台设计（8 页面、RBAC、API、数据模型） |
| `database-schema.md` | 数据库表结构 |
