# 系统健康度治理方案（System Health Governance）— 最终版

> **版本**：V3.0（最终采纳版）
> **日期**：2026-04-02
> **定位**：本文档是系统健康度治理子系统的唯一设计口径。确认后将合并至 `indicator_library_v2.0.md` 和 `rule_library_v3.0_checklist.md`。

---

## 1. 设计动机

### 1.1 现有监控缺口

§8.9.2 识别了 6 项性能风险，其中 4 项缺少对应的监控规则：

| 性能风险 | 现有覆盖 | 缺口 |
|----------|----------|------|
| Kafka consumer lag | 无 | 需要 S-014 |
| Redis/MySQL 资源压力 | 无 | 需要 S-015 |
| 指标产出静默（数据新鲜但指标不出值） | S-007 只看计算延迟 | 需要 S-016 |
| 告警链路本身失效 | 无 | 需要 S-018 |

### 1.2 全局视角缺失

现有 S-Class 规则各自独立告警，值班人员需要打开多个告警才能判断"系统整体是否正常"。需要一个**全局健康度视图** (S-017) 来聚合所有维度。

### 1.3 健康治理子系统的四个目标

1. **发现故障**：识别数据、指标、规则、告警、中间件的异常
2. **解释故障**：让值班人员知道问题在哪一层，不只是看到一个低分
3. **指导动作**：报告中建议该扩容哪一层、该降级到什么模式
4. **形成闭环**：验证"系统不仅在运行，而且真的还能发出告警"（canary 机制）

---

## 2. 新增规则集合（5 条）

| 规则编号 | 规则名称 | 优先级 | 作用定位 | MVP |
|----------|----------|--------|----------|-----|
| `S-014` | Kafka 消息管道积压 | P1 | 检测 Data → Indicator → Rule 链路堵塞 | ✅ |
| `S-015` | 中间件资源压力 | P1 | 检测 Redis / MySQL / Kafka / 应用进程资源瓶颈 | ✅ |
| `S-016` | 指标产出静默 | P2 | 检测"数据新鲜但指标不出值"的隐蔽故障 | ✅ |
| `S-017` | 系统健康度评分与健康报告 | P3/动态 | 输出全局健康视图、分维度评分与定时报告 | ✅ |
| `S-018` | 告警链路健康 / Canary 告警 | P1 | 检测规则触发后是否真的能发出告警 | ✅ |

### 2.1 与已有规则的关系

三层定位，互补不替代：

- `S-004` 回答：**系统是不是已经瞎了？**（紧急硬判定，P0 级）
- `S-014/015/016/018` 回答：**具体是哪一层出了问题？**（分层检测）
- `S-017` 回答：**系统整体现在健康到什么程度？**（全局综合视图）

---

## 3. 健康度评分模型

### 3.1 双分体系：内部健康分 + 外部依赖摘要

#### 设计原则

"交易所状态差"不等于"风控系统自己不健康"。如果混成一个总分，会导致值班人员误判：以为是我方系统故障，实际只是交易所外部环境恶化。

#### 最终方案

| 输出 | 类型 | 用途 |
|------|------|------|
| `internal_health_score` | 0~100 数值 | 系统自身健康度，作为降级建议和扩容建议的依据 |
| `external_dependency_summary` | 文本摘要 | 外部依赖状态描述（MVP 不做独立数值评分） |
| `overall_runtime_state` | 枚举 | `healthy` / `internal_degraded` / `external_degraded` / `critical` |

> **MVP 决策**：`internal_health_score` 做完整数值评分；外部依赖以文本摘要形式附在健康报告中，暂不做成独立数值。等 MVP 运行 1-2 个月积累运营经验后，再将外部分独立为 `external_dependency_score`。

### 3.2 内部健康分维度设计（5 维）

| 维度 | 权重 | 核心输入 | 说明 |
|------|------|----------|------|
| **数据采集层** | 25% | S-001 公共WS, S-002 私有WS, S-003 数据年龄, S-014 Kafka lag(数据侧) | 最核心——数据不来，后面全废 |
| **指标产出层** | 25% | S-007 计算延迟, S-016 产出静默, S-014 Kafka lag(指标侧) | 指标引擎是管道枢纽 |
| **规则执行层** | 20% | S-008 引擎健康, S-009 Watchdog | 规则引擎 + 看门狗 |
| **告警投递层** | 15% | S-018 canary 成功率, alert_delivery_latency, alert_emit_success | 告警链路闭环验证 |
| **基础设施层** | 15% | S-015 Redis/MySQL/Kafka, service_cpu_usage, service_memory_usage | 中间件 + 应用进程资源 |

> 注：权重调整说明——相比初版，告警投递层从 0% 提升到 15%（因新增 S-018），中间件从 15% 保持不变，外部接口从 10% 移出到文本摘要。指标产出层从 25% 保持不变，数据采集层从 30% 调整为 25%，规则执行层 20% 不变。

### 3.3 单项评分映射规则

每个输入映射为 0~100 的子分。映射方式分三类：

**A. 布尔型指标**（连接状态、引擎健康等）：
```
正常 → 100
异常 → 0
```

**B. 阈值型指标**（延迟、使用率、积压等）：
```
值在 [0, warn_threshold)           → 100
值在 [warn_threshold, crit_threshold) → 线性插值 100→30
值在 [crit_threshold, +∞)          → 线性插值 30→0（封底 0）
```

示例 — Kafka consumer lag：
```
[0, 100)     → 100
[100, 1000)  → 100→30 线性
[1000, +∞)   → 30→0（封底 0）
```

**C. 多实例聚合**（多交易所、多账户等）：
```
维度分 = min(所有实例的子分)   // 取最差的那个
```

### 3.4 Hard Cap 机制（硬封顶）

加权平均会掩盖关键链路故障。必须在计算完加权平均后，应用 hard cap 规则：

```
final_score = min(weighted_avg_score, min(applicable_caps))
```

| 触发条件 | 分数上限 | 说明 |
|----------|----------|------|
| `S-004` 系统失明 = true | `≤ 40` | 数据链路已断，风控决策失效 |
| `S-009` watchdog 主链路失败 | `≤ 30` | 看门狗都挂了，谁来看门？ |
| `S-018` canary 告警连续失败 | `≤ 40` | 告警通道不通，等同于聋了 |
| T0/F0 高优先级 consumer lag 严重（≥1000） | `≤ 50` | 高优先级管道堵塞 |
| 多个关键指标同时静默（≥3 个 T0/T1 指标） | `≤ 50` | 指标引擎可能已宕机 |

### 3.5 未实现项处理

**原则：未实现项从分母剔除，不计 100。**

- 未实现 / 未接入的指标不参与所在维度的分母
- 只对已接入、可观测项归一化评分
- 这避免了总分虚高的问题

### 3.6 总分等级定义

| 分数区间 | 等级 | 颜色 | 运行态 | 告警动作 |
|----------|------|------|--------|---------|
| **90~100** | 优良 Healthy | 🟢 | `healthy` | 仅定时报告展示 |
| **70~89** | 注意 Degraded | 🟡 | `internal_degraded` | 定时报告高亮 + L1 Telegram-info |
| **50~69** | 警告 Warning | 🟠 | `internal_degraded` | 立即 L2 Telegram-critical |
| **<50** | 危险 Critical | 🔴 | `critical` | 立即 L3 Telegram + 语音电话 |

### 3.7 运行态（Runtime State）

`overall_runtime_state` 由内部健康分 + 外部依赖摘要共同决定：

| 运行态 | 触发条件 | 建议动作（报告中标注，MVP 不自动执行） |
|--------|----------|----------------------------------------|
| `healthy` | 内部健康分 ≥ 90 且外部依赖无严重异常 | 正常运行 |
| `internal_degraded` | 内部健康分 < 90 或关键维度 < 70 | 建议：降低低优先级规则频率，停掉部分报表任务 |
| `external_degraded` | 内部健康分 ≥ 90 但外部依赖存在严重异常 | 建议：关注交易所状态，准备手动干预策略 |
| `critical` | 内部健康分 < 50 或命中 hard cap | 建议：仅保留保命规则 + 全局高优先级告警 |

> **MVP 决策**：运行态仅在报告中展示建议动作，**不自动执行降级**。等系统稳定运行 2-3 个月后，再开放半自动降级（值班人员在 Telegram 确认后执行）。

---

## 4. 新增指标定义（10 个）

### 4.1 汇总

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 更新频率 | 消费规则 |
|--------|----------|------|------|------|------|----------|----------|
| IND-S-014 | kafka_consumer_lag | L0 | D | metric | consumer_group | 5s (T1) | S-014, S-017 |
| IND-S-015 | redis_memory_usage_pct | L0 | D | metric | redis_instance | 30s (T2) | S-015, S-017 |
| IND-S-015a | redis_ops_per_sec | L0 | D | metric | redis_instance | 30s (T2) | S-015 |
| IND-S-015b | mysql_conn_pool_usage_pct | L0 | D | metric | mysql_instance | 30s (T2) | S-015, S-017 |
| IND-S-015c | service_cpu_usage_pct | L0 | D | metric | service_name | 30s (T2) | S-015, S-017 |
| IND-S-015d | service_memory_usage_pct | L0 | D | metric | service_name | 30s (T2) | S-015, S-017 |
| IND-S-016 | indicator_output_age_ms | L1 | D | metric | indicator_id+grain | 10s (T1) | S-016, S-017 |
| IND-S-017 | internal_health_score | L3 | D | signal | global | 60s (T2) | S-017 |
| IND-S-017a | dimension_health_scores | L2 | D | metric | dimension(5维) | 60s (T2) | S-017 |
| IND-S-018 | canary_alert_success | L0 | D | metric | alert_channel | 15min (T3) | S-018, S-017 |

### 4.2 详细定义

#### IND-S-014 Kafka消费积压 `kafka_consumer_lag`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：Kafka consumer group 的 lag（= latest offset - committed offset），按 consumer group 分别计算
- **direction_semantics**：值越大越危险
- **unit**：消息数
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：consumer_group（如 `t0-indicator-worker`, `f0-rule-worker`, `ingestor-public`, `ingestor-private`）
- **time_mode**：point_in_time
- **source_of_truth**：Kafka AdminClient API（ListConsumerGroupOffsets）
- **refresh / freshness_sla**：5s / ≤10s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：S-014, S-017
- **关键 consumer group 分类**：
  - `t0-indicator-worker`：T0 事件驱动指标消费组 → 高优先级
  - `f0-rule-worker`：F0 事件驱动规则消费组 → 高优先级
  - `t1t2-indicator-worker`：T1/T2 指标消费组 → 中优先级
  - `f1f2-rule-worker`：F1/F2 规则消费组 → 中优先级
  - `clickhouse-archiver`：ClickHouse 归档消费组 → 低优先级

#### IND-S-015 Redis内存使用率 `redis_memory_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：used_memory / maxmemory × 100%（来自 Redis INFO memory）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：redis_instance
- **time_mode**：point_in_time
- **source_of_truth**：Redis INFO 命令
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

#### IND-S-015a Redis操作吞吐 `redis_ops_per_sec`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：instantaneous_ops_per_sec（来自 Redis INFO stats）
- **direction_semantics**：值越大越需关注（接近性能上限时危险）
- **unit**：ops/s
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：redis_instance
- **time_mode**：point_in_time
- **source_of_truth**：Redis INFO 命令
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015

#### IND-S-015b MySQL连接池使用率 `mysql_conn_pool_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：active_connections / max_connections × 100%
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：mysql_instance
- **time_mode**：point_in_time
- **source_of_truth**：MySQL `SHOW STATUS` / 连接池 metrics
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

#### IND-S-015c 服务CPU使用率 `service_cpu_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：应用进程 CPU 使用率（通过 runtime metrics 或 /proc/stat 采集）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：service_name（如 `exchange-ingestor`, `indicator-engine`, `rule-engine`, `telegram-notifier`）
- **time_mode**：point_in_time
- **source_of_truth**：Go runtime metrics / Prometheus client
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

#### IND-S-015d 服务内存使用率 `service_memory_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：应用进程 RSS / 容器 memory limit × 100%（无容器时用系统总内存）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：service_name
- **time_mode**：point_in_time
- **source_of_truth**：Go runtime metrics / cgroup memory stats
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

#### IND-S-016 指标产出年龄 `indicator_output_age_ms`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_updated_at（读取 Redis 中指标 key 的最后更新时间戳）
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：indicator_id + entity_grain（如 `IND-L-001:binance:BTC-USDT:acct001`）
- **time_mode**：point_in_time
- **source_of_truth**：Redis 指标 key 的 updated_at 字段
- **refresh / freshness_sla**：10s / ≤15s
- **suggested_update_time**：10s（T1）
- **downstream_rules**：S-016, S-017
- **与 S-003 / S-007 的区别**：
  - S-003 / IND-S-003 看的是**原始数据快照**（来自交易所）的年龄
  - S-007 看的是**指标计算延迟**（计算耗时）
  - S-016 / IND-S-016 看的是**指标计算输出**的年龄（结果是否更新）
  - 三者正交：数据新鲜 ≠ 计算快 ≠ 结果新鲜

#### IND-S-017 内部系统健康度评分 `internal_health_score`

- **layer**：L3 — 判定信号
- **type**：D — 系统自观测(SYS)
- **indicator_class**：signal
- **mathematical_definition**：见 §3 评分模型——5 维加权汇总 + hard cap
- **direction_semantics**：值越小越危险
- **unit**：分（0~100）
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：global（系统级唯一）
- **time_mode**：point_in_time
- **source_of_truth**：指标引擎内部聚合计算（读取其他 S-Class 指标的最新值）
- **refresh / freshness_sla**：60s / ≤120s
- **suggested_update_time**：60s（T2）
- **downstream_rules**：S-017
- **deps**：IND-S-001, S-001a, S-001b, S-002, S-002a, S-002b, S-003, S-007, S-008, S-009, S-014, S-015, S-015b, S-015c, S-015d, S-016, S-018

#### IND-S-017a 维度健康子分 `dimension_health_scores`

- **layer**：L2 — 衍生指标
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：每个维度内子指标按 §3.3 映射后加权平均
- **direction_semantics**：值越小越危险
- **unit**：分（0~100）
- **value_type**：float，**output_shape**：vector(5)（data_collection, indicator_output, rule_execution, alert_delivery, infrastructure）
- **entity_grain**：dimension（5 个维度各一个值）
- **time_mode**：point_in_time
- **source_of_truth**：指标引擎内部计算
- **refresh / freshness_sla**：60s / ≤120s
- **suggested_update_time**：60s（T2）
- **downstream_rules**：S-017

#### IND-S-018 Canary告警成功标志 `canary_alert_success`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：最近一次 canary 测试告警是否成功完成 roundtrip（发送 → 确认接收）
- **direction_semantics**：0=失败，1=成功
- **unit**：bool（0/1）
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：alert_channel（如 `telegram-main`, `telegram-health`）
- **time_mode**：point_in_time
- **source_of_truth**：notifier 服务内部记录（发送测试消息后通过 Telegram Bot getUpdates 验证）
- **refresh / freshness_sla**：15min / ≤20min
- **suggested_update_time**：15min（T3）
- **downstream_rules**：S-018, S-017
- **附加指标**（S-018 规则内部使用，不独立注册为指标库条目）：
  - `alert_emit_success_pct`：最近 1h 内告警发送成功率
  - `alert_delivery_latency_ms`：告警投递延迟（从规则触发到消息到达 Telegram）
  - `last_successful_alert_age_ms`：距上次成功告警的时间间隔
  - `canary_alert_roundtrip_ms`：canary 探针 roundtrip 耗时

---

## 5. 新增规则定义（5 条）

### 5.1 S-014 Kafka 消息管道积压

- **规则编号**：S-014
- **规则名称**：Kafka 消息管道积压
- **优先级**：P1
- **规则层级**：一级：平台统一主规则库
- **指标类型**：D — 系统自观测
- **风险对象**：风控数据管道（Data → Indicator → Rule 级间 Kafka 总线）
- **业务目的**：检测 Kafka consumer group 的消息积压。积压意味着数据/指标/规则管道的处理速度跟不上生产速度，直接导致风控决策延迟，T0/F0 的 <500ms 延迟保证将被打破。
- **监控指标**：
  - IND-S-014 `kafka_consumer_lag`（按 consumer group）
- **规则条件**：
  - 高优先级 group（T0/F0）：`lag >= 100` → L2
  - 中优先级 group（T1/T2/F1/F2）：`lag >= 500` → L1
  - 低优先级 group（archiver）：`lag >= 10000` → L1
  - 任意高优先级 group：`lag >= 1000` → L3（管道严重积压）
- **时间策略**：
  - `evaluation_tier = F1`
  - `evaluation_interval = 5s`
  - `cooldown_seconds = 300`
  - `recover_hysteresis = 连续 3 个周期 lag < warn_threshold`
- **系统动作建议**：
  - L1：Telegram-info，记录积压曲线
  - L2：Telegram-critical，标记管道为降级态
  - L3：Telegram + 语音，联动 S-004 系统失明（如果 T0/F0 积压严重，等同于风控决策已失效）
- **恢复条件**：lag 回到阈值以下，连续 3 个检测周期稳定
- **与 S-001/S-002 的关系**：S-001/S-002 看"数据是否到了"，S-014 看"到了的数据是否被及时消费"

### 5.2 S-015 中间件与应用资源压力

- **规则编号**：S-015
- **规则名称**：中间件与应用资源压力（Redis / MySQL / Kafka / 应用进程）
- **优先级**：P1
- **规则层级**：一级：平台统一主规则库
- **指标类型**：D — 系统自观测
- **风险对象**：风控系统基础设施 + 应用进程
- **业务目的**：监控 Redis 内存/吞吐、MySQL 连接池、应用进程 CPU/内存等资源。资源耗尽会导致整条管道卡死——Redis 满了写不进新指标、MySQL 连接池满了读不到规则配置、应用 CPU 打满导致 goroutine 调度饥饿。
- **监控指标**：
  - IND-S-015 `redis_memory_usage_pct`
  - IND-S-015a `redis_ops_per_sec`
  - IND-S-015b `mysql_conn_pool_usage_pct`
  - IND-S-015c `service_cpu_usage_pct`
  - IND-S-015d `service_memory_usage_pct`
- **规则条件**：
  - Redis 内存：`>= 70%` → L1，`>= 85%` → L2，`>= 95%` → L3
  - Redis ops：`>= 80K` → L1（接近单实例上限）
  - MySQL 连接池：`>= 70%` → L1，`>= 90%` → L2
  - 服务 CPU：`>= 70%` → L1，`>= 85%` → L2，`>= 95%` → L3
  - 服务内存：`>= 75%` → L1，`>= 90%` → L2，`>= 95%` → L3
- **时间策略**：
  - `evaluation_tier = F2`
  - `evaluation_interval = 30s`
  - `cooldown_seconds = 600`
  - `recover_hysteresis = 连续 2 个周期回落到 warn 阈值以下`
- **系统动作建议**：
  - L1：Telegram-info
  - L2：Telegram-critical + 触发自动清理（Redis 过期 key 扫描、ClickHouse 归档旧数据）
  - L3：Telegram + 语音 + 标记为系统降级态
- **恢复条件**：资源使用率回到 warn 阈值以下

### 5.3 S-016 指标产出静默

- **规则编号**：S-016
- **规则名称**：指标产出静默
- **优先级**：P2
- **规则层级**：一级：平台统一主规则库
- **指标类型**：D — 系统自观测
- **风险对象**：指标引擎产出
- **业务目的**：检测指标引擎是否在运行但某些指标长时间未更新。与 S-003（看原始数据陈旧）和 S-007（看计算延迟）不同——S-016 发现的是"数据是新的、引擎也在跑、但某个指标就是没新值"的隐蔽故障。可能原因：指标代码 bug、特定 entity 的计算卡死、Redis 写入失败等。
- **与 S-003 / S-007 的关系**：
  ```
  S-003：原始数据是否新鲜？（数据层问题）
  S-007：指标算得快不快？（计算层性能问题）
  S-016：指标是否真的产出了新值？（计算层功能问题）
  三者正交，各自覆盖不同故障模式
  ```
- **监控指标**：
  - IND-S-016 `indicator_output_age_ms`（按 indicator_id + grain）
- **规则条件**：
  - T0/T1 类指标：`output_age > suggested_update_time × 5` → L1
  - T0/T1 类指标：`output_age > suggested_update_time × 10` → L2
  - T2 类指标：`output_age > suggested_update_time × 3` → L1
  - 多个指标同时静默（≥3 个 T0/T1 指标）：联动 S-004 系统失明检查 + hard cap 生效
- **时间策略**：
  - `evaluation_tier = F1`
  - `evaluation_interval = 10s`
  - `cooldown_seconds = 600`
  - `recover_hysteresis = 指标恢复产出，连续 2 个周期正常`
- **系统动作建议**：
  - L1：Telegram-info，标记受影响的指标和关联规则
  - L2：Telegram-critical + 标记关联规则为"指标不可用"态（规则引擎遇到此态时降级处理）
- **恢复条件**：指标产出恢复正常

### 5.4 S-017 系统健康度综合评分与定期报告

- **规则编号**：S-017
- **规则名称**：系统健康度综合评分与定期报告
- **优先级**：P3（定时报告）/ 动态升级（P2→P1→P0 根据分值）
- **规则层级**：一级：平台统一主规则库
- **指标类型**：D — 系统自观测（L3 信号，元规则）
- **风险对象**：风控系统整体
- **业务目的**：
  1. **定时报告（每1小时）**：自动推送系统健康报告到 Telegram 专用健康频道，包含内部健康分、5 个维度子分、外部依赖摘要、运行态建议、异常项明细。即使一切正常也推送（绿色报告），让值班人员确认系统在正常运行
  2. **阈值告警（实时）**：当内部健康分降到阈值以下时立即告警到主告警群，不等定时报告
- **监控指标**：
  - IND-S-017 `internal_health_score`
  - IND-S-017a `dimension_health_scores`
- **规则条件**：

  **定时报告模式（F4 层，1h 间隔）**：
  - 每 1 小时固定输出一次健康报告到**专用健康频道**
  - 报告格式见 §6 输出模板

  **实时监控模式（F2 层，60s 间隔）**：
  - `score >= 90` → 正常，不额外告警
  - `score in [70, 90)` → L1 Telegram-info（首次进入黄色区间时）
  - `score in [50, 70)` → L2 Telegram-critical
  - `score < 50` → L3 Telegram + 语音电话
  - **任意单维度 < 30** → 直接 L2（即使总分仍在绿色区间，某个维度严重异常也需告警）

- **时间策略**：
  - 定时报告：`evaluation_tier = F4`, `evaluation_interval = 3600s`
  - 实时监控：`evaluation_tier = F2`, `evaluation_interval = 60s`
  - `cooldown_seconds = 1800`（实时告警冷却 30min，避免反复响铃）
  - `recover_hysteresis = 连续 5 个周期（5min）分值回到上一级`
- **系统动作建议**：
  - 定时报告 → Telegram 推送到专用健康频道（格式化消息，包含颜色标识）
  - L1 → Telegram-info（主告警群）
  - L2 → Telegram-critical（主告警群）
  - L3 → Telegram + 语音 + 报告中标注"建议切换到 conservative 模式"

### 5.5 S-018 告警链路健康 / Canary 告警

- **规则编号**：S-018
- **规则名称**：告警链路健康 / Canary 告警失败
- **优先级**：P1
- **规则层级**：一级：平台统一主规则库
- **指标类型**：D — 系统自观测
- **风险对象**：告警通道完整性
- **业务目的**：验证"规则触发后告警是否真的发得出去"。这是风控闭环的最后一跳——前面的数据、指标、规则全都正常，但如果 Telegram Bot Token 过期、通道被限频、notifier 进程挂了，值班人员就收不到告警。S-018 通过 canary 机制主动探测告警通道的可用性。
- **Canary 机制设计**：
  - 每 15 分钟由 notifier 服务向专用健康频道发送一条测试消息
  - 测试消息格式：`🐤 Canary | {timestamp} | roundtrip: {ms}ms`
  - 通过 Telegram Bot API `getUpdates` 验证消息是否到达
  - 记录 roundtrip 时间和成功/失败状态
- **监控指标**：
  - IND-S-018 `canary_alert_success`
  - 内部指标：`alert_emit_success_pct`, `alert_delivery_latency_ms`, `last_successful_alert_age_ms`, `canary_alert_roundtrip_ms`
- **规则条件**：
  - canary 单次失败 → L1（可能是瞬时网络问题）
  - canary 连续 2 次失败（30min） → L2（告警通道可能已中断）
  - canary 连续 3 次失败（45min）或 `alert_emit_success_pct < 50%` → L3（告警通道确认中断）
  - `canary_alert_roundtrip_ms > 10000` → L1（告警投递严重延迟）
- **时间策略**：
  - `evaluation_tier = F1`（实时监控 canary 结果）
  - canary 探针频率：15min
  - `cooldown_seconds = 900`
  - `recover_hysteresis = 连续 2 次 canary 成功`
- **系统动作建议**：
  - L1：记录日志 + 标记告警通道降级
  - L2：尝试备用通道（如有配置），标记 hard cap 条件
  - L3：hard cap 生效（健康度上限 ≤40），同时尝试所有备用通道（邮件、备用 Bot 等）
- **特殊处理**：当主告警通道不可用时，S-018 的告警本身需要通过备用通道发送（避免"告警自己也发不出去"的死循环）
- **恢复条件**：canary 恢复成功，连续 2 次正常

---

## 6. 定时健康报告输出模板

每 1 小时推送到 Telegram **专用健康频道**的格式化消息。

### 6.1 绿色报告（一切正常）

```
🟢 风控系统健康报告 | 2026-04-02 15:00 UTC+8
━━━━━━━━━━━━━━━━━━━━━━━━━━
运行态：healthy
内部健康分：92/100  [■■■■■■■■■□] 优良

  数据采集  🟢 95   公共WS ✓ | 私有WS ✓ | 数据年龄 ✓ | Kafka lag: 12
  指标产出  🟢 90   计算延迟 ✓ | 产出完整 ✓ | Kafka lag: 8
  规则执行  🟢 94   引擎健康 ✓ | Watchdog ✓
  告警投递  🟢 96   Canary ✓ | 投递延迟 230ms ✓
  基础设施  🟡 82   Redis内存 73% ⚠ | MySQL ✓ | CPU ✓

外部依赖：API频控 45% ✓ | 所有symbol可交易 ✓

注意项：Redis 内存使用率 73%（warn 阈值 70%）
建议扩容层：无
━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 6.2 红色报告（危险状态）

```
🔴 风控系统健康报告 | 2026-04-02 16:00 UTC+8
━━━━━━━━━━━━━━━━━━━━━━━━━━
运行态：critical ← 立即关注！
内部健康分：38/100  [■■■■□□□□□□] 危险
Hard Cap 命中：S-004 系统失明（上限 40）

  数据采集  🔴 22   公共WS ✗ Binance断联 | 私有WS ✓ | Kafka lag: 5832 ⛔
  指标产出  🔴 35   计算延迟 2.3s ⛔ | 3个指标静默 ⛔
  规则执行  🟡 78   引擎健康 ✓ | Watchdog 延迟 1.2s ⚠
  告警投递  🟢 91   Canary ✓ | 投递延迟 450ms ✓
  基础设施  🟢 93   Redis ✓ | MySQL ✓ | CPU 45% ✓

外部依赖：API频控 88% ⛔ | BTC-USDT 暂停交易 ⚠

异常项（按严重度排序）：
  1. [数据采集] Binance 公共 WS 断联 45s，影响 10 个 symbol
  2. [数据采集] Kafka T0 consumer lag = 5832，T0 指标延迟 > 5s
  3. [指标产出] IND-L-001, IND-E-001, IND-M-005 产出静默 > 30s
  4. [外部] API 频控使用率 88%，已进入动态调速区间

建议动作：仅保留保命规则 + 全局高优先级告警（conservative 模式）
建议扩容层：indicator-engine（指标积压）
━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## 7. 与已有规则的关系

### 7.1 S-004（系统失明）vs S-017（健康度）

| 维度 | S-004 系统失明 | S-017 健康度 |
|------|---------------|-------------|
| 定位 | **紧急判定**：系统是否已经失明 | **全面体检**：系统各方面健康程度 |
| 输出 | bool（失明/正常） | 0~100 连续分值 + 运行态枚举 |
| 触发 | 事件驱动（上游变化即刻判定） | 定时（60s 实时 + 1h 报告） |
| 覆盖面 | 仅数据层（WS 连接 + 数据陈旧） | 全管道 5 个维度 |
| 告警 | 仅在失明时 P0 告警 | 分级告警 + 定时报告 |
| 关系 | S-004 是 S-017 数据采集层维度的一个输入，且触发 hard cap | S-017 包含但不替代 S-004 |

### 7.2 扩展已有规则（不改变规则编号）

| 规则 | 新增监控点 | 对应新指标 |
|------|-----------|-----------|
| S-001/S-002 | 新增 WS 连接池使用率监控 | 可复用 IND-S-015 模式 |
| S-008 | 新增规则配置加载耗时、生效规则实例数 | 内部 metrics 上报 |

### 7.3 指标库影响汇总

更新后指标库变化：75 → 85（+10 个 S-Class D 类指标）

| 变更 | 数量 | 说明 |
|------|------|------|
| 新增 metric | +8 | IND-S-014, IND-S-015, IND-S-015a, IND-S-015b, IND-S-015c, IND-S-015d, IND-S-016, IND-S-018 |
| 新增 signal | +1 | IND-S-017 (internal_health_score) |
| 新增 metric(vector) | +1 | IND-S-017a (dimension sub-scores) |
| metric 总数 | 70 → 78 | |
| signal 总数 | 5 → 6 | |
| **总计** | **75 → 85** | |

### 7.4 规则库影响汇总

更新后规则库变化：71 → 76（+5 条 S-Class）

| 变更 | 数量 | 说明 |
|------|------|------|
| 新增规则 | +5 | S-014, S-015, S-016, S-017, S-018 |
| **总计** | **71 → 76** | |
| MVP | 全部 5 条 | S-014 ✅, S-015 ✅, S-016 ✅, S-017 ✅, S-018 ✅ |

---

## 8. MVP 落地策略

### 8.1 落地顺序

**第一阶段：4 条底层规则 + 1 条汇总规则**

```
先做底层检测闭环：
  S-014 (Kafka积压) ─┐
  S-015 (资源压力) ──┤
  S-016 (指标静默) ──┼──→ S-017 (健康度评分 + 报告)
  S-018 (Canary)  ───┘
```

**第一阶段必须接入的输入**：
- Kafka lag（per consumer group）
- Redis memory / ops
- MySQL 连接池使用率
- 服务 CPU / Memory（关键进程：indicator-engine, rule-engine, notifier）
- indicator output age
- rule engine heartbeat (S-008)
- watchdog (S-009)
- notifier canary success

**第一阶段先不做的内容**：
- 不追求复杂趋势模型（kafka_lag_growth_rate 等 → V2）
- 不追求外部依赖独立数值评分（→ 运营 1-2 月后）
- 不追求自动降级执行（→ 稳定运行 2-3 月后）
- 不追求进程级深层指标（goroutine_count, gc_pause, fd_usage → V2）
- 不做 worker 按优先级拆分（→ 触发扩容阈值后）
- 不做 Kafka topic 分层隔离（→ 基础设施扩容阶段）
- 不做自适应报告频率（→ V2）

### 8.2 V2 扩展路径

| 阶段 | 内容 | 触发条件 |
|------|------|----------|
| V2.1 | 外部依赖独立数值评分 `external_dependency_score` | MVP 运行 1-2 月后 |
| V2.2 | 半自动降级（Telegram 确认后执行） | MVP 稳定运行 2-3 月 |
| V2.3 | 趋势指标（growth_rate 系列） | 误报率低于 5% |
| V2.4 | 进程深层指标（goroutine, gc, fd） | 出现过进程级故障 |
| V2.5 | Worker 按优先级拆分 | 单实例触发扩容阈值 |
| V2.6 | Kafka topic 分层（critical/standard/batch） | 出现资源争抢导致的高优任务延迟 |
| V2.7 | 自适应报告频率 | 健康报告噪声太大时 |

---

## 9. 评审记录

### 9.1 初版 → 最终版的关键变更

| 变更项 | 初版 | 最终版 | 变更原因 |
|--------|------|--------|----------|
| 规则数量 | 4 条 (S-014~S-017) | 5 条 (+S-018 canary) | 补齐告警链路盲区 |
| 评分体系 | 单一 0~100 总分（5维含外部接口） | 内部健康分 + 外部依赖文本摘要 | 避免内外混淆导致值班误判 |
| 评分公式 | 纯加权平均 | 加权平均 + hard cap | 防止关键故障被平均分掩盖 |
| TBD 处理 | 默认 100 分 | 从分母剔除 | 防止总分虚高 |
| 运行态 | 无 | healthy/internal_degraded/external_degraded/critical | 为后续降级铺路 |
| S-015 覆盖 | 仅中间件（Redis/MySQL/Kafka） | 中间件 + 应用进程（CPU/Memory） | 补齐应用层资源监控 |
| 指标数量 | 7 个 | 10 个 (+S-015c, S-015d, S-018) | 新增应用资源和 canary 指标 |
| 报告发送 | 主告警群，1h | 专用健康频道，1h；异常时主告警群 | 避免刷屏主告警群 |
| 降级策略 | 无 | 仅报告建议，不自动执行（MVP） | 避免误触降级 |

### 9.2 另一模型建议的采纳情况

| 建议 | 决定 | 说明 |
|------|------|------|
| S-018 canary | ✅ 采纳 | MVP 新增规则 |
| 内外部分拆 | ✅ 采纳（简化版） | internal 做数值，external 做文本摘要 |
| Hard cap | ✅ 采纳 | 5 条硬封顶条件 |
| 未实现项从分母剔除 | ✅ 采纳 | 修正评分公式 |
| 降级四态 | ✅ 采纳（通知模式） | 计算 + 报告，不自动执行 |
| 容器资源指标 | ✅ 部分采纳 | CPU + Memory 纳入，goroutine/gc/fd → V2 |
| Worker 按优先级拆 | ❌ MVP 不拆 | 单进程内部用 goroutine pool 隔离 |
| Kafka/Redis 分层隔离 | ❌ MVP 不做 | 归入基础设施扩容路径 |
| 趋势指标 | ❌ V2 | 先看瞬时值 |
| 报告频率自适应 | ❌ MVP 固定1h | 发到专用频道 |
