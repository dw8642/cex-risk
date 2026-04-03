# 风控控制台 Dashboard 设计方案

> **版本**：V1.0
> **日期**：2026-04-02
> **技术栈**：React + Ant Design Pro + Go API
> **定位**：不是"监控看板"，而是**分层治理、按对象生效、按角色受限、支持审批发布**的风控控制台

---

## 1. 产品定位与三层能力

| 能力层 | 面向角色 | 核心能力 |
|--------|----------|----------|
| **观察层** | 值班、管理层、只读 | 系统健康度、告警态势、风险对象状态、指标产出状态 |
| **配置层** | 管理员、交易员 | 指标运行参数、规则阈值与动作、生效范围、继承覆盖 |
| **治理层** | 超级管理员 | 角色权限、配置审批与发布、变更审计、回滚 |

---

## 2. 信息架构

### 2.1 一级导航（左侧菜单，共 8 项）

```
┌─────────────────────────────────────────────────┐
│  🏠 总览 Home           ← 值班首页              │
│  💚 健康度 Health        ← 系统健康治理专页       │
│  🔔 告警中心 Alerts      ← 事件列表+处置         │
│  🏗️ 对象管理 Objects     ← 四层治理树            │
│  📊 指标库 Indicators    ← 指标治理              │
│  📐 规则库 Rules         ← 规则治理（最复杂）     │
│  📝 变更中心 Changes     ← 审批+审计+回滚        │
│  🔒 权限管理 Access      ← RBAC+Scope           │
└─────────────────────────────────────────────────┘
```

### 2.2 各页面角色可见性

| 页面 | SuperAdmin | Admin | Trader | Viewer |
|------|-----------|-------|--------|--------|
| Home | ✅ 全局 | ✅ 全局 | ✅ 本项目 | ✅ 本项目 |
| Health | ✅ 全局 | ✅ 全局 | ✅ 只读 | ✅ 只读 |
| Alerts | ✅ 全局+处置 | ✅ 全局+处置 | ✅ 本项目+确认 | ✅ 本项目只读 |
| Objects | ✅ 全局+编辑 | ✅ 全局+编辑 | ✅ 本项目只读 | ✅ 本项目只读 |
| Indicators | ✅ 全量+配置 | ✅ 全量+配置 | ✅ 列表只读 | ✅ 列表只读 |
| Rules | ✅ 模板+绑定 | ✅ 绑定 | ✅ 本项目绑定(有限) | ✅ 只读 |
| Changes | ✅ 审批+回滚 | ✅ 审批 | ✅ 查看本项目 | ❌ 不可见 |
| Access | ✅ 全功能 | ❌ 不可见 | ❌ 不可见 | ❌ 不可见 |

---

## 3. 页面详细设计

### 3.1 总览 Home

**目标**：值班人员打开首页，30 秒内知道系统有没有坏、坏在哪层、影响到哪些对象。

```
┌─────────────────────────────────────────────────────────────────┐
│  系统健康度                    运行态                            │
│  ┌──────────┐                ┌──────────────┐                  │
│  │   92     │                │   healthy    │                  │
│  │  /100    │                │   🟢 正常    │                  │
│  └──────────┘                └──────────────┘                  │
│                                                                 │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐                │
│  │数据采集│ │指标产出│ │规则执行│ │告警投递│ │基础设施│                │
│  │ 95 🟢 │ │ 90 🟢 │ │ 94 🟢 │ │ 96 🟢 │ │ 82 🟡 │                │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘                │
├─────────────────────────────────────────────────────────────────┤
│  今日告警                    外部依赖摘要                        │
│  ┌───────────────────┐      ┌──────────────────────┐          │
│  │ L3: 0  L2: 2      │      │ API频控 45% ✓        │          │
│  │ L1: 5  待处理: 3   │      │ 所有symbol可交易 ✓    │          │
│  └───────────────────┘      │ 契约漂移: 无 ✓        │          │
│                              └──────────────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│  关键链路状态                                                    │
│  WS公共 ✅ | WS私有 ✅ | Kafka ✅ | Redis ✅ | RuleEngine ✅    │
│  Watchdog ✅ | Notifier ✅ | Canary ✅                          │
├─────────────────────────────────────────────────────────────────┤
│  高风险对象 Top 5             最近告警事件                        │
│  ┌────────────────────┐     ┌──────────────────────────┐      │
│  │ Account-A  score:62│     │ 14:23 L2 E-001 净Delta   │      │
│  │ Strategy-B score:71│     │ 14:20 L1 S-014 Kafka积压  │      │
│  │ ...                │     │ 14:15 L1 M-001 波动率     │      │
│  └────────────────────┘     └──────────────────────────┘      │
└─────────────────────────────────────────────────────────────────┘
```

**数据来源**：
- 健康度 → Redis `ind:IND-S-017:global`, `ind:IND-S-017a:*`
- 告警统计 → MySQL `risk_events` 聚合
- 链路状态 → Redis `ind:IND-S-001:*`, `ind:IND-S-002:*`, `ind:IND-S-008:*`, `ind:IND-S-009:*`, `ind:IND-S-018:*`
- 高风险对象 → Rule Engine 实时计算的 per-object risk score

### 3.2 健康度 Health

**独立成一级菜单**，对应 `system_health_score_design.md` 健康治理子系统。

#### Tab A：全局健康面板

- 内部健康分（大号数字 + 进度条 + 颜色）
- 运行态枚举（healthy / internal_degraded / external_degraded / critical）
- 外部依赖文本摘要
- 最近 24h 健康分趋势（折线图）

#### Tab B：五维健康分解

每个维度一个可展开卡片：

```
数据采集层  95/100  🟢
├── 公共WS连接        100  ✓
├── 私有WS连接        100  ✓
├── 数据快照年龄       85  正常
├── Kafka lag(数据侧)  92  lag=12
└── 趋势: [最近1h/24h 迷你折线图]
```

点击维度可展开详情：当前分数、子指标列表、趋势图、异常项、建议动作。

#### Tab C：关键健康规则

直接展示 S-014 ~ S-018 以及 S-004 / S-007 / S-008 / S-009 的实时状态卡片。

#### Tab D：容量与瓶颈

- Kafka lag（按 consumer group 分组，柱状图）
- Redis memory / evicted_keys（仪表盘）
- MySQL pool usage（仪表盘）
- Service CPU / Memory（per service 折线图）
- Notifier latency / Canary roundtrip

#### Tab E：健康报告中心

- 最近健康报告列表（可展开查看完整报告）
- Canary 结果时间线
- 运行态切换历史

### 3.3 告警中心 Alerts

**核心功能**：事件列表 + 处置流程

| 字段 | 说明 |
|------|------|
| 时间 | 事件触发时间 |
| 级别 | L1/L2/L3，颜色标识 |
| 规则 | 触发规则编号+名称 |
| 对象 | Project/Team/Strategy/Account |
| 状态 | open → ack → handling → resolved / false_positive |
| 操作 | 确认、处理、关闭、标记误报 |

**筛选器**：级别、状态、规则类别、对象范围、时间区间

**告警详情抽屉**：
- 触发指标值与阈值对比
- 触发时的数据新鲜度快照
- 关联的控制动作（如有）
- 处理时间线

### 3.4 对象管理 Objects ⭐

**这是最重要的串联页面**，把四层治理模型可视化。

左侧：四层树结构
```
📁 BTC做市项目-币安 (Project)
  ├── 📂 内部量化组 (Team)
  │   ├── 📄 mm-btc-binance-01 (Strategy)
  │   │   ├── 🔑 币安合约账户-01 (Account)
  │   │   └── 🔑 币安合约账户-02 (Account)
  │   └── 📄 mm-eth-binance-01 (Strategy)
  │       └── 🔑 币安合约账户-03 (Account)
  └── 📂 PartnerA (Team)
      └── 🔑 币安现货账户-PA01 (Account)  // Team直挂Account，无Strategy
```

右侧：选中对象的 5 个 Tab

| Tab | 内容 |
|-----|------|
| **概览** | 对象基本信息、状态、子对象数量、最近活动 |
| **生效规则** | 当前该对象生效的完整规则列表（含继承来源标注，这是核心功能）|
| **指标状态** | 与该对象相关的指标实时值、更新时间、是否异常 |
| **告警历史** | 该对象触发的告警事件列表 |
| **配置变更** | 该对象相关的配置变更历史 |

**"生效规则" Tab 是最核心的功能**，回答 "这个账户现在到底用了哪些规则和什么阈值"：

```
┌─────────────────────────────────────────────────────────────┐
│  账户: 币安合约账户-01  生效规则 (42条)                       │
├──────┬────────────┬────────┬──────────┬────────┬───────────┤
│ 规则  │ 生效状态    │ 来源    │ 阈值      │ 动作    │ 最近命中  │
├──────┼────────────┼────────┼──────────┼────────┼───────────┤
│P-001 │ ✅ 启用     │ Global │ 默认     │ R0     │ 从未     │
│E-001 │ ✅ 启用     │ Team↓  │ delta≤8  │ R2     │ 2h前     │
│E-002 │ ✅ 启用     │Account↓│ 自定义   │ R1     │ 14:23    │
│M-001 │ ❌ 已关闭   │ Team✗  │ -       │ -      │ -       │
│...   │            │        │          │        │          │
└──────┴────────────┴────────┴──────────┴────────┴───────────┘
```

来源列标注：
- `Global` → 使用全局默认
- `Project` → 项目级覆盖
- `Team↓` → 团队级覆盖（向下继承）
- `Strategy↓` → 策略级覆盖
- `Account↓` → 账户级专有覆盖
- `Team✗` → 团队级显式关闭

### 3.5 指标库 Indicators

分成 4 个 Tab 页：

#### Tab 3.5.1：指标目录

全量指标列表（85 个），字段：

| 字段 | 说明 |
|------|------|
| 指标ID | IND-S-014 等 |
| 名称 | kafka_consumer_lag 等 |
| 类别 | L/E/S/M/B/C/P/BASE/STR |
| 层级 | L0/L1/L2/L3 |
| 类型 | A/B/C/D |
| 粒度 | exchange+symbol 等 |
| 更新频率 | 5s / 30s / 1min 等 |
| 消费规则 | S-014, S-017 等（可点击跳转） |
| 运行状态 | 🟢正常 / 🟡静默 / 🔴错误 / ⚪禁用 |

支持按类别、层级、类型、状态筛选。

#### Tab 3.5.2：指标运行态

偏监控视图：

| 字段 | 说明 |
|------|------|
| 最近更新时间 | 2026-04-02 15:03:22 |
| 数据年龄 | 1.2s |
| 输出覆盖率 | 98%（多少 entity 在正常产出） |
| 计算耗时 P99 | 12ms |
| 异常率 | 0.1% |
| 最近错误 | （如有） |

#### Tab 3.5.3：指标配置

**只开放运行参数，不允许改数学定义。**

| 可配置项 | 说明 | 权限 |
|----------|------|------|
| 是否启用 | 全局开关 | Admin+ |
| 更新频率 | 覆盖默认频率 | Admin+ |
| freshness SLA | 新鲜度要求 | Admin+ |
| 静默阈值 | 多久不更新算静默 | Admin+ |
| 计算优先级 | high/medium/low | SuperAdmin |
| 是否纳入健康度评分 | 参与 S-017 计算 | SuperAdmin |

#### Tab 3.5.4：指标依赖图

可视化展示指标的上下游关系：

```
[交易所WS] → IND-S-001 → S-001(规则) → risk_event
                       ↘ S-004(规则)
                       ↘ IND-S-017(健康度)
```

使用 Ant Design 的 DAG 图或 React Flow 实现。点击任意节点可以看到当前状态。

**排查价值**：某个指标挂了，一眼看到影响哪些规则。

### 3.6 规则库 Rules ⭐⭐

**最复杂的页面**，核心设计原则：规则模板 + 分层绑定 + 继承覆盖。

#### 3.6.1 规则模板（Rule Template）

平台维护的规则标准定义。对应 MySQL `risk_rules` 表。

| 字段 | 说明 |
|------|------|
| 规则编号 | S-014 |
| 规则名称 | Kafka 消息管道积压 |
| 类别 | S-系统 |
| 默认优先级 | P1 |
| 默认阈值 | `{"high_priority_warn_lag": 100, ...}` |
| 默认动作 | L1→Telegram-info, L2→Telegram-critical, L3→语音 |
| 默认时间策略 | F1, 5s, cooldown=300 |
| 消费指标 | IND-S-014 |
| 启用状态 | ✅ |

**权限**：只有 SuperAdmin 和 Admin 可以修改规则模板。

#### 3.6.2 规则绑定（Rule Binding）— 三个视图

**视图 A：规则视图（从规则出发）**

选中一条规则（如 E-001），看到它在各层级的绑定情况：

```
E-001 净 Delta 过高
├── 默认模板: threshold=10, cooldown=300, action=R2
├── Global: [继承默认]
├── Project: BTC做市项目
│   └── threshold=8 (覆盖)
├── Team: 内部量化组
│   └── cooldown=600 (覆盖), threshold=8 (继承自Project)
├── Team: PartnerA
│   └── threshold=5 (覆盖，更严格)
├── Strategy: mm-btc-binance-01
│   └── [继承Team配置]
├── Account: 币安合约账户-01
│   └── threshold=12 (覆盖，放宽)
└── Account: 币安合约账户-02
    └── [继承Strategy配置]
```

**适用场景**：风控负责人统一管理一条规则在各层的配置。

**视图 B：对象视图（从对象出发）**

选中一个对象（如某 Account），看到所有生效规则及其来源。这和 Objects 页面的"生效规则"Tab 复用同一组件。

**适用场景**：回答"这个账户到底用了什么阈值"。

**视图 C：差异视图（Diff View）**

支持对比：
- 默认模板 vs 某对象的生效配置
- 上层配置 vs 下层配置
- 版本 A vs 版本 B（配置变更前后对比）

```
┌──────────────────────┬──────────────────────┐
│ 默认模板              │ Account-01 生效配置   │
├──────────────────────┼──────────────────────┤
│ threshold: 10        │ threshold: 12 ← 修改 │
│ cooldown: 300        │ cooldown: 600 ← 来自Team│
│ action: R2           │ action: R2           │
│ priority: P1         │ priority: P1         │
└──────────────────────┴──────────────────────┘
```

**适用场景**：审计排查、误配置定位。

#### 3.6.3 规则绑定的配置操作

每个绑定支持三种操作：

| 操作 | 含义 | UI 表现 |
|------|------|--------|
| **继承** | 不做任何配置，沿用上层 | 灰色文字"继承自 {layer}" |
| **覆盖** | 只改部分参数，其余继承 | 黄色高亮被覆盖的字段 |
| **显式关闭** | 即使上层启用，这一层也关闭 | 红色 ❌ + "已在 {layer} 关闭" |

**配置编辑表单**：

```
┌─────────────────────────────────────────┐
│ 规则 E-001 - Account: 币安合约账户-01    │
│                                          │
│ 启用状态: ○继承(✅) ○覆盖启用 ○显式关闭  │
│                                          │
│ 阈值 threshold:                          │
│   ○继承(8, 来自Project) ●覆盖: [12]     │
│                                          │
│ 冷却时间 cooldown:                       │
│   ●继承(600, 来自Team) ○覆盖: [___]     │
│                                          │
│ 优先级 priority:                         │
│   ●继承(P1, 来自Global) ○覆盖: [___]    │
│                                          │
│ 动作 action:                             │
│   ●继承(R2, 来自Global) ○覆盖: [___]    │
│                                          │
│ [保存为草稿]  [取消]                      │
└─────────────────────────────────────────┘
```

### 3.7 变更中心 Changes

**核心原则**：高风险配置变更不能直接生效。

#### 双轨配置发布流程

```
草稿 Draft → 预览 Preview → 审批 Approval → 发布 Publish
                                              ↓
                                          回滚 Rollback
```

**变更分级**：

| 变更风险等级 | 触发条件 | 发布要求 |
|-------------|----------|---------|
| 🟢 低风险 | 非关键规则参数微调（如 cooldown） | Trader 可直接发布 |
| 🟡 中风险 | 关键规则阈值变更、启用/禁用非保命规则 | Admin 审批后发布 |
| 🔴 高风险 | L/S/P0/P1 类规则变更、全局规则修改、健康治理规则变更 | SuperAdmin 审批后发布 |

**变更列表页**：

| 字段 | 说明 |
|------|------|
| 变更ID | 自动生成 |
| 变更类型 | 规则绑定 / 指标配置 / 对象管理 |
| 变更内容 | JSON diff 预览 |
| 影响范围 | 影响 N 个账户 / N 条规则 |
| 风险等级 | 🟢/🟡/🔴 |
| 提交人 | 谁提交的 |
| 审批人 | 谁审批的 |
| 状态 | draft / pending_approval / approved / published / rolled_back |
| 操作 | 审批 / 拒绝 / 发布 / 回滚 |

**预览功能**：发布前展示影响分析：
- 受影响的对象列表
- 变更前后参数对比
- 是否触发高风险标记

### 3.8 权限管理 Access

仅 SuperAdmin 可见。

---

## 4. 权限模型：RBAC + Scope

### 4.1 四种角色

| 角色 | 代号 | 定位 | 覆盖原方案的角色 |
|------|------|------|----------------|
| **SuperAdmin** | SA | 平台最高管理员 | Super Admin + Auditor |
| **Admin** | ADM | 风控管理员 / 项目负责人 | Risk Admin + Project Owner |
| **Trader** | TRD | 交易员 / 策略运营 | Team Lead + Strategy Operator |
| **Viewer** | VW | 只读观察员 / 值班 | Viewer |

> **设计决策**：另一个模型建议 7 种角色，但 MVP 阶段过于细碎。4 种角色通过 Scope 绑定实现细粒度控制。Admin 可以被限定到特定 Project，Trader 可以被限定到特定 Team/Strategy。

### 4.2 Scope 绑定

每个用户除了角色，还有 scope 限定：

```sql
-- users 表
CREATE TABLE users (
  id          VARCHAR(36) PK,
  username    VARCHAR(64) NOT NULL UNIQUE,
  email       VARCHAR(256),
  role        ENUM('super_admin','admin','trader','viewer') NOT NULL,
  status      ENUM('active','disabled') NOT NULL DEFAULT 'active',
  created_at  TIMESTAMP,
  updated_at  TIMESTAMP
);

-- user_scopes 表 — 限定用户的可操作范围
CREATE TABLE user_scopes (
  id          VARCHAR(36) PK,
  user_id     VARCHAR(36) NOT NULL,
  scope_type  ENUM('global','project','team','strategy','account') NOT NULL,
  scope_id    VARCHAR(36),   -- global 时为 NULL
  UNIQUE(user_id, scope_type, scope_id)
);
```

**示例**：
- SA 用户：`scope_type=global`，全局可见
- ADM 用户 A：`scope_type=project, scope_id=P001`，只能管理 P001 项目
- TRD 用户 B：`scope_type=team, scope_id=T002`，只能管理 T002 团队

### 4.3 权限矩阵（三个维度）

#### 维度一：菜单级

| 页面 | SA | ADM | TRD | VW |
|------|-----|------|------|-----|
| Home | ✅ | ✅ | ✅ | ✅ |
| Health | ✅ | ✅ | ✅(只读) | ✅(只读) |
| Alerts | ✅ | ✅ | ✅ | ✅(只读) |
| Objects | ✅ | ✅ | ✅(只读) | ✅(只读) |
| Indicators | ✅ | ✅ | ✅(只读) | ✅(只读) |
| Rules - 模板 | ✅ | ✅(只读) | ❌ | ❌ |
| Rules - 绑定 | ✅ | ✅ | ✅(scope内) | ✅(只读) |
| Changes | ✅ | ✅ | ✅(scope内) | ❌ |
| Access | ✅ | ❌ | ❌ | ❌ |

#### 维度二：操作级

| 操作 | SA | ADM | TRD | VW |
|------|-----|------|------|-----|
| 修改规则模板 | ✅ | ❌ | ❌ | ❌ |
| 创建/修改规则绑定 | ✅ | ✅(scope内) | ✅(scope内,低风险) | ❌ |
| 审批高风险变更 | ✅ | ❌ | ❌ | ❌ |
| 审批中风险变更 | ✅ | ✅ | ❌ | ❌ |
| 发布低风险变更 | ✅ | ✅ | ✅(scope内) | ❌ |
| 确认告警 | ✅ | ✅ | ✅(scope内) | ❌ |
| 执行控制动作 | ✅ | ✅ | ❌ | ❌ |
| 回滚配置 | ✅ | ❌ | ❌ | ❌ |
| 管理用户/角色 | ✅ | ❌ | ❌ | ❌ |

#### 维度三：字段级

| 字段 | SA | ADM | TRD |
|------|-----|------|------|
| rule_template.* | 读写 | 只读 | 不可见 |
| binding.enabled | 读写 | 读写 | 读写(scope内) |
| binding.threshold | 读写 | 读写 | 读写(scope内) |
| binding.cooldown | 读写 | 读写 | 读写(scope内) |
| binding.priority | 读写 | 读写 | 只读 |
| binding.action | 读写 | 读写 | 只读 |
| indicator.enabled | 读写 | 读写 | 只读 |
| indicator.update_freq | 读写 | 读写 | 只读 |

### 4.4 Scope 过滤逻辑

所有 API 请求统一经过 scope 中间件：

```go
// 伪代码
func scopeMiddleware(ctx context.Context, userID string) {
    user := getUser(userID)
    scopes := getUserScopes(userID)

    // SuperAdmin scope=global 时跳过过滤
    if user.Role == "super_admin" && hasGlobalScope(scopes) {
        return
    }

    // 其他角色：注入 scope 过滤条件
    // 所有查询自动 WHERE project_id IN (...) OR team_id IN (...)
    ctx = injectScopeFilter(ctx, scopes)
}
```

---

## 5. 规则配置的数据模型

### 5.1 与现有 DB Schema 的对齐

已有的 `rule_bindings` 表完美支持分层绑定：

```sql
-- 已有表 (database-schema.md §2.3)
rule_bindings (
  id          VARCHAR(36) PK,
  rule_code   VARCHAR(16) NOT NULL,
  scope_type  ENUM('global','project','team','strategy','account'),
  scope_id    VARCHAR(36),
  enabled     TINYINT(1),
  priority    ENUM('P0','P1','P2','P3'),
  config      JSON,          -- 该层级的参数覆盖
  UNIQUE(rule_code, scope_type, scope_id)
)
```

解析优先级：`Account > Strategy > Team > Project > Global`

### 5.2 规则生效解析算法

```go
// 获取某账户的某条规则的最终生效配置
func resolveRuleConfig(ruleCode string, account Account) EffectiveConfig {
    // 1. 加载规则模板默认配置
    base := getRuleTemplate(ruleCode).DefaultConfig

    // 2. 按层级逐层 merge（就近生效）
    layers := []struct{ scopeType string; scopeID string }{
        {"global", ""},
        {"project", account.ProjectID},
        {"team", account.TeamID},       // 可能为 NULL，跳过
        {"strategy", account.StrategyID}, // 可能为 NULL，跳过
        {"account", account.ID},
    }

    for _, layer := range layers {
        if layer.scopeID == "" && layer.scopeType != "global" {
            continue // 该层缺失，跳过
        }
        binding := getBinding(ruleCode, layer.scopeType, layer.scopeID)
        if binding == nil {
            continue // 该层无绑定，继承上层
        }
        if !binding.Enabled {
            return EffectiveConfig{Enabled: false, Source: layer.scopeType}
            // 显式关闭，直接返回
        }
        base = mergeConfig(base, binding.Config) // 部分覆盖
        base.Source = layer.scopeType
    }

    return base
}
```

### 5.3 新增 DB 表（支持变更发布流程）

```sql
-- 配置变更草稿
CREATE TABLE config_drafts (
  id            VARCHAR(36) PK,
  change_type   ENUM('rule_binding','indicator_config','object_config') NOT NULL,
  target_type   VARCHAR(32) NOT NULL,   -- 'rule_binding' / 'indicator' / etc.
  target_id     VARCHAR(36) NOT NULL,   -- 被修改对象的 ID
  before_value  JSON,                   -- 变更前快照
  after_value   JSON,                   -- 变更后值
  risk_level    ENUM('low','medium','high') NOT NULL,
  impact_scope  JSON,                   -- 影响范围（受影响的 account IDs）
  status        ENUM('draft','pending_approval','approved','published','rejected','rolled_back'),
  submitted_by  VARCHAR(36) NOT NULL,
  approved_by   VARCHAR(36),
  published_at  TIMESTAMP,
  rolled_back_at TIMESTAMP,
  created_at    TIMESTAMP,
  updated_at    TIMESTAMP
);

-- 配置审计日志
CREATE TABLE config_audit_logs (
  id            VARCHAR(36) PK,
  draft_id      VARCHAR(36),            -- 关联变更草稿
  action        ENUM('create','update','delete','approve','reject','publish','rollback'),
  actor_id      VARCHAR(36) NOT NULL,
  actor_role    VARCHAR(32) NOT NULL,
  target_type   VARCHAR(32) NOT NULL,
  target_id     VARCHAR(36) NOT NULL,
  detail        JSON,
  created_at    TIMESTAMP
);

索引：idx_draft(draft_id), idx_actor(actor_id), idx_target(target_type, target_id), idx_created(DESC)
```

---

## 6. 指标配置与规则配置的边界

**一句话原则：指标页面解决数据与计算，规则页面解决判定与动作。**

| 配置类型 | 归属 | 示例 |
|----------|------|------|
| 怎么采、怎么算、多久更新 | **指标库** | 是否启用、更新频率、静默阈值、freshness SLA |
| 拿哪个指标、怎么判、判完怎么处理 | **规则库** | 阈值、适用对象、优先级、告警等级、动作、冷却时间 |

---

## 7. API 设计概要

### 7.1 健康度 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/health/score` | 当前健康分 + 维度子分 + 运行态 |
| GET | `/api/v1/health/dimensions/{dim}` | 某维度详情（子指标列表+分数） |
| GET | `/api/v1/health/reports` | 健康报告列表 |
| GET | `/api/v1/health/canary` | Canary 状态历史 |
| GET | `/api/v1/health/capacity` | 容量瓶颈视图数据 |

### 7.2 告警 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/alerts` | 告警列表（支持分页+筛选） |
| GET | `/api/v1/alerts/{id}` | 告警详情 |
| PUT | `/api/v1/alerts/{id}/ack` | 确认告警 |
| PUT | `/api/v1/alerts/{id}/resolve` | 关闭告警 |
| GET | `/api/v1/alerts/stats` | 告警统计（按级别/状态/规则） |

### 7.3 对象 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/objects/tree` | 四层治理树（根据用户 scope 过滤） |
| GET | `/api/v1/projects/{id}` | 项目详情 |
| GET | `/api/v1/teams/{id}` | 团队详情 |
| GET | `/api/v1/strategies/{id}` | 策略详情 |
| GET | `/api/v1/accounts/{id}` | 账户详情 |
| GET | `/api/v1/accounts/{id}/effective-rules` | 账户生效规则列表 |
| GET | `/api/v1/accounts/{id}/indicators` | 账户关联指标 |
| GET | `/api/v1/accounts/{id}/alerts` | 账户告警历史 |

### 7.4 指标 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/indicators` | 指标目录（列表+筛选） |
| GET | `/api/v1/indicators/{id}` | 指标详情 |
| GET | `/api/v1/indicators/{id}/runtime` | 指标运行态 |
| PUT | `/api/v1/indicators/{id}/config` | 修改指标配置（→ 走变更流程） |
| GET | `/api/v1/indicators/{id}/deps` | 指标依赖图 |

### 7.5 规则 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/rules` | 规则模板列表 |
| GET | `/api/v1/rules/{code}` | 规则模板详情 |
| PUT | `/api/v1/rules/{code}` | 修改规则模板（SA only） |
| GET | `/api/v1/rules/{code}/bindings` | 规则绑定列表（规则视图） |
| POST | `/api/v1/rule-bindings` | 创建绑定（→ 走变更流程） |
| PUT | `/api/v1/rule-bindings/{id}` | 修改绑定（→ 走变更流程） |
| GET | `/api/v1/rule-bindings/resolve` | 解析生效配置（对象视图） |
| GET | `/api/v1/rule-bindings/diff` | 配置差异（差异视图） |

### 7.6 变更 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/changes` | 创建变更草稿 |
| GET | `/api/v1/changes` | 变更列表 |
| GET | `/api/v1/changes/{id}` | 变更详情+影响分析 |
| POST | `/api/v1/changes/{id}/approve` | 审批通过 |
| POST | `/api/v1/changes/{id}/reject` | 审批拒绝 |
| POST | `/api/v1/changes/{id}/publish` | 发布生效 |
| POST | `/api/v1/changes/{id}/rollback` | 回滚 |

### 7.7 权限 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/users` | 用户列表 |
| POST | `/api/v1/users` | 创建用户 |
| PUT | `/api/v1/users/{id}` | 修改用户信息/角色 |
| GET | `/api/v1/users/{id}/scopes` | 用户 scope 列表 |
| PUT | `/api/v1/users/{id}/scopes` | 修改用户 scope |
| GET | `/api/v1/auth/me` | 当前登录用户信息+权限 |

---

## 8. MVP 分期策略

### Phase 1（MVP，2 周）

| 页面 | 范围 |
|------|------|
| Home | 健康度卡片 + 告警统计 + 链路状态 |
| Health | 全局面板 + 五维分解（不含趋势图） |
| Alerts | 事件列表 + 确认/关闭 |
| Objects | 四层树 + 生效规则 Tab |
| Rules | 规则目录 + 绑定编辑（不含 Diff View） |
| Auth | 基础 RBAC（4 角色 + scope 过滤） |

### Phase 2（+2 周）

| 页面 | 范围 |
|------|------|
| Health | 趋势图 + 容量瓶颈 + 报告中心 |
| Indicators | 指标目录 + 运行态 + 配置 |
| Rules | 对象视图 + Diff View |
| Changes | 草稿/审批/发布/回滚全流程 |

### Phase 3（+2 周）

| 页面 | 范围 |
|------|------|
| Indicators | 依赖图可视化 |
| Objects | 指标状态 Tab + 配置变更 Tab |
| Access | 完整权限管理 UI |
| 全局 | 操作审计日志、配置版本回滚 |

---

## 9. 另一模型建议的采纳情况

| 建议 | 决定 | 说明 |
|------|------|------|
| 三层能力（观察/配置/治理） | ✅ 采纳 | 清晰的产品定位分层 |
| 8 个一级菜单 | ✅ 采纳（微调） | Home/Health/Alerts/Objects/Indicators/Rules/Changes/Access |
| 健康度独立一级菜单 | ✅ 采纳 | 对应健康治理子系统 |
| Objects 页面四层树 | ✅ 采纳 | 串联四层治理模型 |
| 规则三视图（规则/对象/差异） | ✅ 采纳 | 覆盖不同使用场景 |
| 双轨配置发布 | ✅ 采纳 | 草稿→预览→审批→发布→回滚 |
| 7 种角色 | ❌ 简化为 4 种 | 通过 Scope 绑定实现细粒度，MVP 够用 |
| 字段级权限 | ✅ 采纳 | priority/action 限 Admin+，threshold/cooldown 开放 Trader |
| 指标依赖图 | ✅ 采纳（Phase 3） | 排查价值大，但实现优先级不高 |
| 指标/规则边界清晰分离 | ✅ 采纳 | 指标管数据计算，规则管判定动作 |
