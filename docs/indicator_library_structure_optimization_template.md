# 指标库结构优化建议模板

> 用途：用于把 `indicator_library_v1.0.md` 从“规则导出型指标表”升级为“可复用、可治理、可实现”的指标体系文档。
> 
> 目标：统一指标层级、字段规范、依赖关系、版本管理与规则映射方式，减少口径漂移与实现歧义。

---

# 1. 设计目标

指标库应明确回答 4 个问题：

1. **这个指标是什么**
2. **这个指标从哪里来**
3. **这个指标怎么算**
4. **这个指标被谁消费**

规则库负责：

- 判定条件
- 阈值
- 告警等级
- 动作
- 恢复条件

指标库负责：

- 数据源
- 粒度
- 计算逻辑
- 刷新方式
- 依赖关系
- 输出结构
- 版本与治理

---

# 2. 推荐的指标分层

建议将指标统一分为 4 层。

## L0 — 原始观测指标（Raw Observation）

直接来自交易所、内部服务、系统探针，不带业务判断。

典型例子：

- `maintMarginRatio`
- `mark_price`
- `best_bid`
- `best_ask`
- `available_balance`
- `api_error_rate`
- `last_update_ts`

特点：

- 尽量不做业务推理
- 来源单一
- 可复用性最强

## L1 — 标准化基础指标（Normalized Base Metric）

基于 L0 做统一口径转换，但不包含阈值判断。

典型例子：

- `distance_to_liq_pct`
- `position_usd`
- `spread_bps`
- `order_value_usd`
- `data_age_ms`
- `single_fill_slippage_bps`

特点：

- 统一单位
- 统一方向语义
- 跨规则复用的核心层

## L2 — 衍生指标（Derived Metric）

由多个 L0/L1 指标组合而来，但仍然不直接等同于规则判定。

典型例子：

- `total_abs_exposure_usd`
- `risk_budget_usage`
- `budget_consumption_rate_pct`
- `microstructure_score`
- `hedge_gap_usd`

特点：

- 有明确上游依赖
- 适合做指标血缘管理

## L3 — 判定信号（Signal / Flag）

已非常接近规则层的指标输出，通常是 bool、enum、score、flag。

典型例子：

- `unauthorized_trade_flag`
- `manual_order_flag`
- `schema_diff_detected`
- `system_blind_flag`
- `wash_trade_score`

特点：

- 可存在，但不能过多
- 要明确这是“信号层”，不是原子指标层
- 规则库尽量消费 L1/L2，少直接依赖黑盒 L3

---

# 3. 每个指标建议必备字段

建议每条指标定义至少包含以下字段。

## 3.1 基本信息

- `indicator_id`：唯一标识，如 `IND-E-001`
- `indicator_name_cn`：中文名
- `indicator_name_en`：英文名 / code name
- `layer`：L0 / L1 / L2 / L3
- `type`：A / B / C / D / E
- `category`：L / E / S / M / B / C / P / BASE / STR
- `status`：draft / confirmed / deprecated
- `version`：如 `v1.0`

## 3.2 语义定义

- `business_definition`：业务含义
- `mathematical_definition`：数学定义 / 公式
- `direction_semantics`：值变大是更危险 / 更安全 / 中性
- `unit`：USD / % / bps / ms / count / bool / enum
- `value_type`：float / int / bool / enum / object / list
- `output_shape`：scalar / map / time_series / object

## 3.3 维度定义

- `entity_grain`：
  - account
  - account+symbol
  - exchange+symbol
  - strategy
  - account_group
  - global
- `market_scope`：spot / futures / both / conditional
- `time_mode`：point_in_time / rolling_window / calendar_window / event_driven
- `window_definition`：若是窗口指标，明确 1m / 5m / 1h / rolling 24h 等
- `grouping_keys`：主键维度，如 `exchange, account, symbol`

## 3.4 数据来源

- `source_of_truth`：主判定源
- `fallback_source`：降级备用源
- `source_systems`：数据来自哪些系统
- `upstream_fields`：依赖的原始字段
- `update_trigger`：REST 拉取 / WS 事件 / 定时任务 / 内部埋点

## 3.5 计算与新鲜度

- `calculation_logic`：明确公式
- `normalization_rules`：单位换算 / 方向统一 / 精度处理
- `staleness_sla`：允许数据多旧
- `missing_data_policy`：缺数时返回什么
- `late_data_policy`：迟到数据如何处理

## 3.6 治理字段

- `owner_team`
- `implementation_owner`
- `storage_location`
- `compute_location`
- `retention_policy`
- `cardinality_risk`
- `testability`
- `backfillability`

## 3.7 映射关系

- `upstream_dependencies`
- `downstream_rules`
- `downstream_indicators`
- `related_metrics`

---

# 4. 推荐的文档结构

建议按下面结构组织指标库。

## 4.1 文档头部

- 版本说明
- 覆盖范围说明
- 与规则库对应关系说明
- 指标层级定义
- 状态说明

## 4.2 全局索引表

建议提供 4 张索引表：

1. 按类别索引
2. 按层级索引（L0/L1/L2/L3）
3. 按类型索引（A/B/C/D/E）
4. 按复用度索引（被多少规则消费）

## 4.3 指标详情区

每个指标按统一模板展开。

## 4.4 血缘与复用区

- 指标 → 指标依赖表
- 规则 → 指标映射表
- 高复用指标列表
- 废弃 / 合并指标列表

## 4.5 附录

- 命名规范
- 单位规范
- 常见计算口径
- 待确认问题清单

---

# 5. 推荐的命名规范

## 5.1 ID 规范

格式建议：

- `IND-{CATEGORY}-{NUMBER}`

例如：

- `IND-L-001`
- `IND-S-005`
- `IND-BASE-003`
- `IND-STR-007`

## 5.2 code name 规范

建议统一使用：

- 小写
- 下划线
- 尽量描述“量”，少描述“结论”

优先：

- `position_usd`
- `spread_bps`
- `data_age_ms`
- `risk_budget_usage`

慎用：

- `risk_flag`
- `danger_score`
- `bad_state`

## 5.3 bool / flag 规范

只有在以下场景才建议直接定义 bool：

- 外部系统本身就返回 bool
- 确实是事件类信号
- 规则层复用成本明显低于重新判断

否则优先输出基础量，而不是直接输出 flag。

---

# 6. 推荐的拆分原则

## 6.1 一个指标尽量只表达一件事

不建议：

- `withdraw_monitor`
- `transfer_monitor`
- `public_feed_health`

因为这些是“监控包”，不是单指标。

建议拆成：

- `withdraw_amount`
- `withdraw_frequency_24h`
- `is_new_withdraw_address`
- `public_ws_connected`
- `public_ws_heartbeat_gap_ms`
- `public_data_gap_ms`

## 6.2 阈值判断尽量留在规则层

不建议在指标定义中写：

- `value > threshold`
- `count < expected`
- `age > timeout`

这些应保留在规则层。

指标层只负责提供：

- `value`
- `count`
- `expected`
- `age_ms`

## 6.3 组合型指标必须声明上游依赖

例如：

- `risk_budget_usage`
- `microstructure_score`
- `compliance_risk_score`

必须显式写明：

- 上游指标
- 依赖版本
- 聚合方式
- 失效传播方式

---

# 7. 建议新增的工程治理字段模板

建议在每条指标详情后增加一个治理小节。

模板如下：

```text
- owner_team:
- implementation_owner:
- source_of_truth:
- fallback_source:
- compute_location:
- storage_location:
- freshness_sla:
- missing_data_policy:
- cardinality_risk:
- backfillability:
- version:
```

---

# 8. 推荐的单指标模板

下面给出推荐模板。

## 指标卡模板

### IND-XXX 指标中文名 code_name

- **layer**：L0 / L1 / L2 / L3
- **type**：A / B / C / D / E
- **category**：L / E / S / M / B / C / P / BASE / STR
- **business_definition**：
- **mathematical_definition**：
- **direction_semantics**：值越大越危险 / 值越小越危险 / 中性
- **unit**：
- **value_type**：
- **output_shape**：
- **entity_grain**：
- **market_scope**：
- **time_mode**：
- **window_definition**：
- **source_of_truth**：
- **fallback_source**：
- **source_systems**：
- **upstream_fields**：
- **calculation_logic**：
- **normalization_rules**：
- **refresh_frequency**：
- **freshness_sla**：
- **missing_data_policy**：
- **upstream_dependencies**：
- **downstream_rules**：
- **downstream_indicators**：
- **owner_team**：
- **implementation_owner**：
- **storage_location**：
- **compute_location**：
- **retention_policy**：
- **version**：
- **status**：draft / confirmed / deprecated
- **notes**：

---

# 9. 现有指标库改造的优先顺序建议

## 第一优先级：修正规则库与指标库口径不一致

优先检查：

- 仓位变动速率类
- 对账类
- OTR 类
- 波动率 / 价格跳变类
- 单边成交比例类
- 手工单 / 来源审计类

## 第二优先级：拆分打包型指标

优先拆：

- 系统健康类
- 提币监控类
- 划转监控类
- 群组控制类

## 第三优先级：建立层级标签

给全部指标打上：

- L0
- L1
- L2
- L3

## 第四优先级：补治理字段

至少补：

- owner
- source_of_truth
- freshness_sla
- version
- compute_location

---

# 10. 最终目标状态

当指标库完成结构优化后，应具备以下特征：

1. 规则库和指标库口径一致
2. 原子指标、衍生指标、信号指标层次清楚
3. 高复用指标有明确主定义
4. 所有指标都可追踪上游和下游
5. 指标公式可版本化
6. 新规则新增时优先复用已有指标，而不是重新发明

---

# 11. 一句话总原则

**指标库应优先沉淀“可复用的计算口径”，而不是沉淀“某条规则的判定结果”；能抽成基础量，就不要过早固化成 flag。**
