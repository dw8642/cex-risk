# 指标库 V2.0 — 风控系统指标定义与规则映射

> 本文档基于 `indicator_library_v1.0` 升级，参照 `indicator_library_structure_optimization_template.md` 优化。
> V2.0 主要变更：修正方向语义、拆分多输出指标、修正粒度/依赖关系、标记信号类指标、新增时间策略字段。
> E类(TBD待量化)指标已移除，待后续量化定义后再纳入。

---

## 0. 总览

共 **85 个指标**（metric: 78，signal: 6，metric-vector: 1），
覆盖 **68 条规则**（另有8条规则依赖E类TBD指标，见 §0.8 占位映射）。

### 0.1 层级分布

| 层级 | 含义 | 数量 |
|------|------|------|
| L0 — 原始观测 | — | 28 |
| L1 — 标准化基础 | — | 39 |
| L2 — 衍生指标 | — | 12 |
| L3 — 判定信号 | — | 6 |
| **合计** | | **85** |

### 0.2 指标类型分布

| 类型 | 含义 | 数量 |
|------|------|------|
| A — 实时可读(RT) | — | 25 |
| B — 窗口聚合(AGG) | — | 21 |
| C — 跨源衍生(DRV) | — | 11 |
| D — 系统自观测(SYS) | — | 28 |
| **合计** | | **85** |

### 0.3 分类分布

| 类别 | 指标数 | 对应规则类 |
|------|--------|----------|
| 清算 | 5 | L-清算 |
| 敞口 | 10 | E-敞口 |
| 系统 | 23 | S-系统 |
| 市场 | 8 | M-市场 |
| 行为 | 12 | B-行为 |
| 合规 | 2 | C-合规 |
| 权限 | 11 | P-权限 |
| 基线 | 5 | BASE-基线 |
| 策略 | 9 | STR-策略 |
| **合计** | **85** | |

### 0.4 衍生指标（有上游依赖）

共 **3** 个衍生指标：

| 衍生指标 | 上游依赖 | 消费规则 |
|----------|----------|----------|
| IND-E-004 总绝对敞口(USD) | IND-E-002 | E-004 |
| IND-E-007a 预算消耗速率 | IND-E-007 | E-007 |
| IND-S-004 系统失明标志 | IND-S-001 + IND-S-001a + IND-S-001b + IND-S-002 + IND-S-002a + IND-S-002b + IND-S-003 | S-004 |

### 0.5 跨规则复用指标

共 **11** 个指标被 ≥2 条规则消费：

| 指标 | 消费规则 |
|------|----------|
| IND-S-001 公共WS连接状态 | S-001, S-004 |
| IND-S-001a 公共WS心跳间隔 | S-001, S-004 |
| IND-S-001b 公共数据间隔 | S-001, S-004 |
| IND-S-002 私有WS连接状态 | S-002, S-004 |
| IND-S-002a 私有WS心跳间隔 | S-002, S-004 |
| IND-S-002b 私有数据间隔 | S-002, S-004 |
| IND-S-003 数据快照年龄 | S-003, S-004 |
| IND-M-004 资金费率 | M-004, E-008 |
| IND-M-005 市场深度 | M-005, M-009 |
| IND-M-006 买卖点差 | M-006, M-009 |
| IND-B-002 单边成交比例 | B-002, MAN-002 |

### 0.6 信号类指标（indicator_class = signal）

以下 **5 个指标**更接近规则判定输出而非基础量，单独标记为 `signal` 类：

| 指标ID | 名称 | 说明 |
|--------|------|------|
| IND-B-004 | unauthorized_trade_flag | 授权范围匹配判定 → bool flag |
| IND-B-007 | manual_order_flag | 来源白名单匹配判定 → bool flag |
| IND-P-005b | is_new_withdraw_address | 地址簿匹配判定 → bool flag |
| IND-P-006b | transfer_route_match | 路由白名单匹配判定 → bool flag |
| IND-S-004 | system_blind_flag | 多上游健康信号聚合判定 → bool flag |

> 这些指标留在库中是因为规则层直接消费且复用成本低于重新判断，但应避免在此层级继续新增「结论型」指标。

### 0.7 已移除的E类(TBD)指标

以下10个E类指标因尚未量化定义，暂不纳入指标库，待后续补充：

| 原ID | 原名称 | 对应规则 |
|------|--------|----------|
| IND-S-006 | 数据真相源一致性 | S-006 |
| IND-S-011 | 群组控制健康 | S-011 |
| IND-S-012 | 同源策略批量异常 | S-012 |
| IND-S-013 | API契约漂移 | S-013 |
| IND-M-008 | 毒性订单流指标 | M-008 |
| IND-M-009 | 微观结构综合分 | M-009 |
| IND-B-008 | 对敲/刷量嫌疑分 | B-008（注：B-008仍由IND-B-005供给部分数据） |
| IND-B-011 | 多账户行为相似度 | B-011 |
| IND-C-001 | 多账户关联合规分 | C-001 |
| IND-C-002 | 多账户规避检测 | C-002 |

### 0.8 TBD规则占位映射

以下8条规则的指标尚未纳入指标库，但在此明确占位，避免误认为映射已完整：

| 规则ID | 已有指标 | 缺失指标(TBD) | 状态 |
|--------|----------|---------------|------|
| S-006 | — | IND-S-006 数据真相源一致性 | TBD |
| S-011 | — | IND-S-011 群组控制健康 | TBD |
| S-012 | — | IND-S-012 同源策略批量异常 | TBD |
| S-013 | — | IND-S-013 API契约漂移 | TBD |
| M-008 | — | IND-M-008 毒性订单流指标 | TBD |
| M-009 | IND-M-005, IND-M-006 (部分) | IND-M-008 毒性订单流(TBD)、IND-M-009 综合分(TBD) | 部分已定义 |
| B-011 | — | IND-B-011 多账户行为相似度 | TBD |
| C-001 | — | IND-C-001 合规风险评分(依赖B-011+B-009) | TBD |
| C-002 | — | IND-C-002 多账户规避检测 | TBD |

### 0.9 指标更新时间分层建议

> `建议更新时间` 指指标缓存刷新/重算cadence，用于指导规则引擎调度；**不等于实际告警推送频率**。
> 告警频率由规则层的 `evaluation_interval`、`cooldown`、`recover_hysteresis` 单独控制。

| 时间层 | 含义 | 目标延迟 | 典型场景 |
|--------|------|----------|----------|
| T0 — 事件触发 | 有事件才算 | <500ms | 下单前检查、提现/划转、逐笔异常单 |
| T1 — 秒级状态 | 高危状态持续监控 | 1~5s | 清算近线、系统心跳、单腿暴露、策略心跳 |
| T2 — 滚动窗口 | 行为/市场统计滚动更新 | 30s~5min | 放量、单边成交、挂撤单比、波动突升 |
| T3 — 定时巡检 | 低频但重要的配置/状态检查 | 5min~1h | 权限漂移、账户限制、跨所偏离、资产类慢变量 |
| T4 — 批处理/对账 | 外部依赖或审计输出 | 1h~资金周期/T+1 | 对账差异、审计类聚合、外部 Dashboard |

### 0.10 V2.0 变更摘要

| 变更项 | 说明 |
|--------|------|
| 指标总数 | 72(v1) → 75(v2)（+10拆分 -10移除E类 +3多输出拆分） |
| 修正direction_semantics | IND-L-005, M-005, B-002, P-001, P-002, P-006b, BASE-001 共7个 |
| 拆分多输出指标 | IND-E-007→+E-007a, M-001→+M-001a, M-002→+M-002a 共+3 |
| 修正S-003/S-004 | S-003输出改为ms(非bool), S-004改名system_blind_flag |
| 修正粒度 | E-002→account+exchange+symbol, S-010→exchange+symbol |
| 修正依赖 | STR-007移除对B-002的虚假依赖，降级为L1 |
| 标记signal类 | B-004, B-007, P-005b, P-006b, S-004 标记indicator_class=signal |
| 新增时间策略 | 全部指标补充suggested_update_time(T0-T4) |
| TBD占位映射 | 8条TBD规则明确占位，M-009标注部分已定义 |

---

## 1. L-清算 类指标（5 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-L-001 | 维持保证金率 maint_margin_rate | L0 | A | metric | account+symbol | 实时/定时REST | 1s（T1） | L-001 |
| IND-L-002 | 爆仓距离 distance_pct | L1 | A | metric | account+symbol | 实时/定时REST | 1s（T1） | L-002 |
| IND-L-003 | ADL等级 adl | L0 | A | metric | account+symbol | 实时/定时REST | 5s（T1） | L-003 |
| IND-L-004 | 单边OI占比 oi_ratio | L2 | C | metric | exchange+symbol | 定时REST | 1min（T2） | L-004 |
| IND-L-005 | 抵押物折后保证金率 collateral_margin_ratio | L2 | C | metric | account+symbol | 定时REST | 1min（T2） | L-005 |

### IND-L-001 维持保证金率 `maint_margin_rate`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取，无需计算
- **direction_semantics**：值越小越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：futures
- **time_mode**：point_in_time
- **source_of_truth**：交易所账户 REST API（maintMarginRatio 字段）
- **refresh / freshness_sla**：实时/定时REST / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：L-001

### IND-L-002 爆仓距离 `distance_pct`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：abs(liq_price - mark_price) / mark_price
- **direction_semantics**：值越小越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：futures
- **time_mode**：point_in_time
- **source_of_truth**：交易所 Position REST API（liq_price, mark_price）
- **refresh / freshness_sla**：实时/定时REST / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：L-002

### IND-L-003 ADL等级 `adl`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取，离散档位
- **direction_semantics**：值越大越危险
- **unit**：档位
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：futures
- **time_mode**：point_in_time
- **source_of_truth**：交易所 Position REST API（adl 字段）
- **refresh / freshness_sla**：实时/定时REST / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：L-003

### IND-L-004 单边OI占比 `oi_ratio`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：abs(net_position) / market_total_oi（需区分单向/双向持仓模式）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：futures
- **time_mode**：calendar_window
- **source_of_truth**：Position REST API + 公共OI API
- **refresh / freshness_sla**：定时REST / ≤60s
- **suggested_update_time**：1min（T2）
- **downstream_rules**：L-004

### IND-L-005 抵押物折后保证金率 `collateral_margin_ratio`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：Σ(qty × price × (1-haircut_rate)) / total_margin_required
- **direction_semantics**：值越小越危险
- **unit**：倍数
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：futures
- **time_mode**：calendar_window
- **source_of_truth**：账户 Asset REST API + 抵押物价格 Feed + 交易所折扣率配置
- **refresh / freshness_sla**：定时REST / ≤60s
- **suggested_update_time**：1min（T2）
- **downstream_rules**：L-005

---

## 2. E-敞口 类指标（10 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-E-001 | 净Delta(USD) net_delta_usd | L2 | C | metric | account_group | 定时REST | 1min（T2） | E-001 |
| IND-E-002 | 单市场暴露(USD) position_usd | L1 | C | metric | account+exchange+symbol | 定时REST | 30s（T2） | E-002 |
| IND-E-003 | 对冲缺口(USD) hedge_gap_usd | L2 | C | metric | hedge_pair | 定时REST | 30s（T2） | E-003 |
| IND-E-004 | 总绝对敞口(USD) total_abs_exposure_usd | L2 | C | metric | account_group | 定时REST | 1min（T2） | E-004 |
| IND-E-005 | 仓位变动额(USD) position_change_usd | L1 | B | metric | account | 窗口聚合(1h/24h) | 30s（T2） | E-005 |
| IND-E-006 | 可用资金 available_balance | L0 | A | metric | account | 实时/定时REST | 5s（T1） | E-006 |
| IND-E-007 | 风险预算使用率 risk_budget_usage | L2 | B | metric | strategy | 实时/窗口聚合 | 30s（T2） | E-007 |
| IND-E-007a | 预算消耗速率 budget_consumption_rate_pct | L2 | B | metric | strategy | 窗口聚合 | 30s（T2） | E-007 |
| IND-E-008 | 资金费率累计出血 funding_bleed | L1 | B | metric | account | 每8h/窗口聚合 | 5min（T2/T3） | E-008 |
| IND-E-009 | 资金周期对账差异 fund_reconciliation_diff | L2 | C | metric | account | 定时对账 | 1h/资金周期（T4） | E-009 |

### IND-E-001 净Delta(USD) `net_delta_usd`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：Σ(position_qty × mark_price × direction_sign)，按账户/组聚合
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account_group，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：Position REST API（多账户持仓）+ Mark Price
- **refresh / freshness_sla**：定时REST / ≤60s
- **suggested_update_time**：1min（T2）
- **downstream_rules**：E-001

### IND-E-002 单市场暴露(USD) `position_usd`

- **layer**：L1 — 标准化基础
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：abs(position_qty) × mark_price，按 exchange+symbol 维度
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+exchange+symbol，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：Position REST API + Mark Price
- **refresh / freshness_sla**：定时REST / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：E-002

### IND-E-003 对冲缺口(USD) `hedge_gap_usd`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：abs(leg_a_usd - leg_b_usd × expected_ratio)
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：hedge_pair，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：Position REST API（多账户）+ Mark Price + 对冲映射配置
- **refresh / freshness_sla**：定时REST / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：E-003

### IND-E-004 总绝对敞口(USD) `total_abs_exposure_usd`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：Σ abs(position_usd)，跨市场汇总
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account_group，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：Position REST API + Mark Price
- **refresh / freshness_sla**：定时REST / ≤60s
- **suggested_update_time**：1min（T2）
- **upstream_dependencies**：IND-E-002
- **downstream_rules**：E-004

### IND-E-005 仓位变动额(USD) `position_change_usd`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：abs(position_usd_t - position_usd_t-n)，支持1h/24h窗口
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Position REST API（时序快照）+ Mark Price
- **refresh / freshness_sla**：窗口聚合(1h/24h) / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：E-005

### IND-E-006 可用资金 `available_balance`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取 availableBalance 字段
- **direction_semantics**：值越小越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：交易所账户 REST API
- **refresh / freshness_sla**：实时/定时REST / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：E-006

### IND-E-007 风险预算使用率 `risk_budget_usage`

- **layer**：L2 — 衍生指标
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：current_loss = abs(Σ min(pnl, 0)); risk_budget_usage = current_loss / risk_budget
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Position PnL + 风控预算配置
- **refresh / freshness_sla**：实时/窗口聚合 / ≤1s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：E-007

### IND-E-007a 预算消耗速率 `budget_consumption_rate_pct`

- **layer**：L2 — 衍生指标
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：(current_loss_t - current_loss_t-n) / risk_budget，窗口内预算消耗增速
- **direction_semantics**：值越大越危险
- **unit**：%/h
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Position PnL 时序 + 风控预算配置
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **upstream_dependencies**：IND-E-007
- **downstream_rules**：E-007

### IND-E-008 资金费率累计出血 `funding_bleed`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：Σ(funding_rate × position_value)，滚动窗口累计
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：futures
- **time_mode**：rolling_window
- **source_of_truth**：交易所 Funding Rate API + Position
- **refresh / freshness_sla**：每8h/窗口聚合 / ≤30s
- **suggested_update_time**：5min（T2/T3）
- **downstream_rules**：E-008

### IND-E-009 资金周期对账差异 `fund_reconciliation_diff`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：消费外部Dashboard对账结果，非自行计算；对账差异 = Dashboard报告差额
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：外部对账 Dashboard 输出 + 交易所余额快照
- **refresh / freshness_sla**：定时对账 / ≤60s
- **suggested_update_time**：1h/资金周期（T4）
- **downstream_rules**：E-009

---

## 3. S-系统 类指标（23 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-S-001 | 公共WS连接状态 public_ws_connected | L0 | D | metric | exchange+feed_type | 实时心跳 | 1s（T1） | S-001, S-004 |
| IND-S-001a | 公共WS心跳间隔 public_ws_heartbeat_gap_ms | L0 | D | metric | exchange+feed_type | 实时心跳 | 1s（T1） | S-001, S-004 |
| IND-S-001b | 公共数据间隔 public_data_gap_ms | L0 | D | metric | exchange | 实时 | 1s（T1） | S-001, S-004 |
| IND-S-002 | 私有WS连接状态 private_ws_connected | L0 | D | metric | exchange+feed_type | 实时心跳 | 1s（T1） | S-002, S-004 |
| IND-S-002a | 私有WS心跳间隔 private_ws_heartbeat_gap_ms | L0 | D | metric | exchange+feed_type | 实时心跳 | 1s（T1） | S-002, S-004 |
| IND-S-002b | 私有数据间隔 private_data_gap_ms | L0 | D | metric | exchange | 实时 | 1s（T1） | S-002, S-004 |
| IND-S-003 | 数据快照年龄 data_age_ms | L0 | D | metric | exchange | 定时检测 | 30s（T2） | S-003, S-004 |
| IND-S-004 | 系统失明标志 system_blind_flag 🔶 | L3 | D | signal | global | 实时 | 1s（T1） | S-004 |
| IND-S-005 | API频控使用率 api_rate_usage | L1 | D | metric | exchange | 实时累计 | 1s（T1） | S-005 |
| IND-S-007 | 指标计算延迟 indicator_calc_latency | L1 | D | metric | exchange | 实时 | 5s（T1） | S-007 |
| IND-S-008 | 规则引擎运行状态 rule_engine_health | L1 | D | metric | exchange | 实时 | 5s（T1） | S-008 |
| IND-S-009 | Watchdog主链路状态 watchdog_health | L1 | D | metric | exchange | 实时 | 1s（T1） | S-009 |
| IND-S-010 | 交易所/Symbol可交易状态 tradeable_status | L0 | A | metric | exchange+symbol | 实时/定时 | 1min（T3） | S-010 |
| IND-S-014 | Kafka消费积压 kafka_consumer_lag | L0 | D | metric | consumer_group | 5s | 5s（T1） | S-014, S-017 |
| IND-S-015 | Redis内存使用率 redis_memory_usage_pct | L0 | D | metric | redis_instance | 30s | 30s（T2） | S-015, S-017 |
| IND-S-015a | Redis操作吞吐 redis_ops_per_sec | L0 | D | metric | redis_instance | 30s | 30s（T2） | S-015 |
| IND-S-015b | MySQL连接池使用率 mysql_conn_pool_usage_pct | L0 | D | metric | mysql_instance | 30s | 30s（T2） | S-015, S-017 |
| IND-S-015c | 服务CPU使用率 service_cpu_usage_pct | L0 | D | metric | service_name | 30s | 30s（T2） | S-015, S-017 |
| IND-S-015d | 服务内存使用率 service_memory_usage_pct | L0 | D | metric | service_name | 30s | 30s（T2） | S-015, S-017 |
| IND-S-016 | 指标产出年龄 indicator_output_age_ms | L1 | D | metric | indicator_id+grain | 10s | 10s（T1） | S-016, S-017 |
| IND-S-017 | 内部系统健康度评分 internal_health_score 🔶 | L3 | D | signal | global | 60s | 60s（T2） | S-017 |
| IND-S-017a | 维度健康子分 dimension_health_scores | L2 | D | metric | dimension(5维) | 60s | 60s（T2） | S-017 |
| IND-S-018 | Canary告警成功标志 canary_alert_success | L0 | D | metric | alert_channel | 15min | 15min（T3） | S-018, S-017 |

### IND-S-001 公共WS连接状态 `public_ws_connected`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：WebSocket连接状态布尔检测
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：exchange+feed_type，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：WebSocket 公共连接状态
- **refresh / freshness_sla**：实时心跳 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-001, S-004

### IND-S-001a 公共WS心跳间隔 `public_ws_heartbeat_gap_ms`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_heartbeat_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+feed_type，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：WebSocket 公共连接心跳监测
- **refresh / freshness_sla**：实时心跳 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-001, S-004

### IND-S-001b 公共数据间隔 `public_data_gap_ms`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_data_msg_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：WebSocket 公共数据流时间戳
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-001, S-004

### IND-S-002 私有WS连接状态 `private_ws_connected`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：WebSocket私有连接状态布尔检测
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：exchange+feed_type，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：WebSocket 私有连接状态
- **refresh / freshness_sla**：实时心跳 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-002, S-004

### IND-S-002a 私有WS心跳间隔 `private_ws_heartbeat_gap_ms`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_heartbeat_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+feed_type，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：WebSocket 私有连接心跳监测
- **refresh / freshness_sla**：实时心跳 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-002, S-004

### IND-S-002b 私有数据间隔 `private_data_gap_ms`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_data_msg_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：WebSocket 私有数据流时间戳
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-002, S-004

### IND-S-003 数据快照年龄 `data_age_ms`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - snapshot_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：各数据快照的时间戳
- **refresh / freshness_sla**：定时检测 / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-003, S-004

### IND-S-004 系统失明标志 `system_blind_flag` `[signal]`

- **layer**：L3 — 判定信号
- **type**：D — 系统自观测(SYS)
- **indicator_class**：signal
- **mathematical_definition**：综合判定：任一公共/私有WS断连 OR 心跳超时 OR 数据间隔超时 OR 快照过旧 → system_blind
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：global，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：IND-S-001 + IND-S-002 + IND-S-003 输出
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **upstream_dependencies**：IND-S-001, IND-S-001a, IND-S-001b, IND-S-002, IND-S-002a, IND-S-002b, IND-S-003
- **downstream_rules**：S-004

### IND-S-005 API频控使用率 `api_rate_usage`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：current_usage / rate_limit
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：API 调用计数器 + 交易所限频配置
- **refresh / freshness_sla**：实时累计 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-005

### IND-S-007 指标计算延迟 `indicator_calc_latency`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：indicator_output_ts - data_input_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：指标引擎内部计时
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：S-007

### IND-S-008 规则引擎运行状态 `rule_engine_health`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：异常率/延迟/panic 检测
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：规则引擎心跳 + 异常日志
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：S-008

### IND-S-009 Watchdog主链路状态 `watchdog_health`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：主风控链路可达性检测
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：exchange，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Watchdog 心跳 + 端到端探针
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：S-009

### IND-S-010 交易所/Symbol可交易状态 `tradeable_status`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取 status 字段
- **direction_semantics**：false=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：交易所 exchangeInfo / Symbol Status API
- **refresh / freshness_sla**：实时/定时 / ≤1s
- **suggested_update_time**：1min（T3）
- **downstream_rules**：S-010

### IND-S-014 Kafka消费积压 `kafka_consumer_lag`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：Kafka consumer group 的 lag（= latest offset - committed offset），按 consumer group 分别计算
- **direction_semantics**：值越大越危险
- **unit**：消息数
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：consumer_group，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Kafka AdminClient API（ListConsumerGroupOffsets）
- **refresh / freshness_sla**：5s / ≤10s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：S-014, S-017

### IND-S-015 Redis内存使用率 `redis_memory_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：used_memory / maxmemory × 100%（来自 Redis INFO memory）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：redis_instance，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Redis INFO 命令
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

### IND-S-015a Redis操作吞吐 `redis_ops_per_sec`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：instantaneous_ops_per_sec（来自 Redis INFO stats）
- **direction_semantics**：值越大越需关注（接近性能上限时危险）
- **unit**：ops/s
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：redis_instance，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Redis INFO 命令
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015

### IND-S-015b MySQL连接池使用率 `mysql_conn_pool_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：active_connections / max_connections × 100%
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：mysql_instance，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：MySQL `SHOW STATUS` / 连接池 metrics
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

### IND-S-015c 服务CPU使用率 `service_cpu_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：应用进程 CPU 使用率（通过 runtime metrics 或 /proc/stat 采集）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：service_name，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Go runtime metrics / Prometheus client
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

### IND-S-015d 服务内存使用率 `service_memory_usage_pct`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：应用进程 RSS / 容器 memory limit × 100%（无容器时用系统总内存）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：service_name，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Go runtime metrics / cgroup memory stats
- **refresh / freshness_sla**：30s / ≤60s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：S-015, S-017

### IND-S-016 指标产出年龄 `indicator_output_age_ms`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_updated_at（读取 Redis 中指标 key 的最后更新时间戳）
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：indicator_id+entity_grain，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Redis 指标 key 的 updated_at 字段
- **refresh / freshness_sla**：10s / ≤15s
- **suggested_update_time**：10s（T1）
- **downstream_rules**：S-016, S-017
- **与 IND-S-003 / IND-S-007 的区别**：
  - IND-S-003 看**原始数据快照**年龄（数据层）
  - IND-S-007 看**指标计算延迟**（计算性能）
  - IND-S-016 看**指标输出**年龄（计算功能，结果是否更新）

### IND-S-017 内部系统健康度评分 `internal_health_score`

- **layer**：L3 — 判定信号
- **type**：D — 系统自观测(SYS)
- **indicator_class**：signal
- **mathematical_definition**：5 维加权汇总 + hard cap，详见 system_health_score_design.md §3
- **direction_semantics**：值越小越危险
- **unit**：分（0~100）
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：global，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：指标引擎内部聚合计算
- **refresh / freshness_sla**：60s / ≤120s
- **suggested_update_time**：60s（T2）
- **downstream_rules**：S-017
- **upstream_dependencies**：IND-S-001, S-001a, S-001b, S-002, S-002a, S-002b, S-003, S-007, S-008, S-009, S-014, S-015, S-015b, S-015c, S-015d, S-016, S-018

### IND-S-017a 维度健康子分 `dimension_health_scores`

- **layer**：L2 — 衍生指标
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：每个维度内子指标映射后加权平均
- **direction_semantics**：值越小越危险
- **unit**：分（0~100）
- **value_type**：float，**output_shape**：vector(5)（data_collection, indicator_output, rule_execution, alert_delivery, infrastructure）
- **entity_grain**：dimension(5维)，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：指标引擎内部计算
- **refresh / freshness_sla**：60s / ≤120s
- **suggested_update_time**：60s（T2）
- **downstream_rules**：S-017
- **upstream_dependencies**：与 IND-S-017 相同

### IND-S-018 Canary告警成功标志 `canary_alert_success`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：最近一次 canary 测试告警是否成功完成 roundtrip（发送 → 确认接收）
- **direction_semantics**：0=失败，1=成功
- **unit**：bool（0/1）
- **value_type**：int，**output_shape**：scalar
- **entity_grain**：alert_channel，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：notifier 服务内部记录
- **refresh / freshness_sla**：15min / ≤20min
- **suggested_update_time**：15min（T3）
- **downstream_rules**：S-018, S-017

---

## 4. M-市场 类指标（8 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-M-001 | 价格跳变率(1h) price_jump_pct_1h | L1 | B | metric | exchange+symbol | 窗口聚合 | 30s（T2） | M-001 |
| IND-M-001a | 实现波动率 realized_volatility | L1 | B | metric | exchange+symbol | 窗口聚合 | 30s（T2） | M-001 |
| IND-M-002 | API延迟 api_latency_ms | L1 | D | metric | exchange+symbol | 实时 | 5s（T1） | M-002 |
| IND-M-002a | API错误率 api_error_rate | L1 | B | metric | exchange+symbol | 窗口聚合 | 5s（T1） | M-002 |
| IND-M-004 | 资金费率 funding_rate | L0 | A | metric | exchange+symbol | 每8h/实时 | 1min或费率变更时（T3） | M-004, E-008 |
| IND-M-005 | 市场深度 depth_score | L1 | B | metric | exchange+symbol | 实时/窗口 | 30s（T2） | M-005, M-009 |
| IND-M-006 | 买卖点差 spread | L1 | A | metric | exchange+symbol | 实时 | 5s（T1） | M-006, M-009 |
| IND-M-007 | 跨所价格偏离 cross_exchange_deviation | L2 | C | metric | symbol(cross_exchange) | 实时 | 1min（T3） | M-007 |

### IND-M-001 价格跳变率(1h) `price_jump_pct_1h`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：abs(price_now - price_1h_ago) / price_1h_ago
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：K线/Trades/Mark Price 时序数据
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：M-001

### IND-M-001a 实现波动率 `realized_volatility`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：窗口内价格收益率标准差（可选ATR）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：K线/Trades/Mark Price 时序数据
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：M-001

### IND-M-002 API延迟 `api_latency_ms`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：round-trip latency统计（p50/p99）
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：API 探针 + WS 延迟监测 + API错误计数
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：M-002

### IND-M-002a API错误率 `api_error_rate`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：error_count / total_count（滚动窗口内）
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：API错误计数器
- **refresh / freshness_sla**：窗口聚合 / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：M-002

### IND-M-004 资金费率 `funding_rate`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取
- **direction_semantics**：中性（绝对值越大越异常）
- **unit**：bps
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：futures
- **time_mode**：point_in_time
- **source_of_truth**：交易所 Funding Rate API
- **refresh / freshness_sla**：每8h/实时 / ≤1s
- **suggested_update_time**：1min或费率变更时（T3）
- **downstream_rules**：M-004, E-008

### IND-M-005 市场深度 `depth_score`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：各档位累计量/加权深度指标
- **direction_semantics**：值越小越危险
- **unit**：USD/score
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Orderbook WS/REST
- **refresh / freshness_sla**：实时/窗口 / ≤1s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：M-005, M-009

### IND-M-006 买卖点差 `spread`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：(best_ask - best_bid) / mid_price
- **direction_semantics**：值越大越危险
- **unit**：bps
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：exchange+symbol，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Orderbook（best_ask - best_bid）
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：5s（T1）
- **downstream_rules**：M-006, M-009

### IND-M-007 跨所价格偏离 `cross_exchange_deviation`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：max(abs(price_i - price_j)) / avg_price
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：symbol(cross_exchange)，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：多交易所 Mark Price / Mid Price
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1min（T3）
- **downstream_rules**：M-007

---

## 5. B-行为 类指标（12 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-B-001 | 异常成交放量 volume_anomaly | L1 | B | metric | account+symbol | 窗口聚合 | 30s（T2） | B-001 |
| IND-B-002 | 单边成交比例 directional_imbalance | L1 | B | metric | account+symbol | 窗口聚合 | 30s（T2） | B-002, MAN-002 |
| IND-B-003 | 单笔订单规模 order_size | L1 | A | metric | account+symbol | 实时/逐笔 | 事件触发（T0） | B-003 |
| IND-B-004 | 非授权交易标记 unauthorized_trade_flag 🔶 | L3 | A | signal | account+symbol | 实时/逐笔 | 事件触发（T0） | B-004 |
| IND-B-005 | 高成交低仓变比 volume_position_change_ratio | L1 | B | metric | account+symbol | 窗口聚合 | 30s（T2） | B-008 |
| IND-B-006 | 执行滑点 slippage | L2 | C | metric | account+symbol | 逐笔 | 事件触发（T0） | B-006 |
| IND-B-007 | 手工单异常标记 manual_order_flag 🔶 | L3 | A | signal | account+symbol | 实时 | 事件触发（T0/T1） | B-007 |
| IND-B-009 | 自成交检测 self_trade_flag | L1 | B | metric | account_group | 逐笔/窗口 | 1min（T3） | B-009 |
| IND-B-010 | 挂撤单比 OTR order_to_trade_ratio | L1 | B | metric | account+symbol | 窗口聚合 | 30s（T2） | B-010 |
| IND-B-012 | 未对冲腿数量 open_leg_count | L1 | B | metric | account+symbol | 实时 | 1s（T1） | B-012 |
| IND-B-012a | 最久未对冲腿时长 oldest_unhedged_leg_age_ms | L1 | B | metric | account+symbol | 实时 | 1s（T1） | B-012 |
| IND-B-012b | 未对冲敞口(USD) unhedged_exposure_usd | L2 | C | metric | account+symbol | 实时 | 1s（T1） | B-012 |

### IND-B-001 异常成交放量 `volume_anomaly`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：window_volume / baseline_volume
- **direction_semantics**：值越大越危险
- **unit**：倍数
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Trades / Fills 聚合
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：B-001

### IND-B-002 单边成交比例 `directional_imbalance`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：abs(buy_volume - sell_volume) / total_volume，方向不平衡比例
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Trades / Fills 聚合
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：B-002, MAN-002

### IND-B-003 单笔订单规模 `order_size`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：order_qty × price
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order Request / Fills
- **refresh / freshness_sla**：实时/逐笔 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：B-003

### IND-B-004 非授权交易标记 `unauthorized_trade_flag` `[signal]`

- **layer**：L3 — 判定信号
- **type**：A — 实时可读(RT)
- **indicator_class**：signal
- **mathematical_definition**：order 与授权范围匹配检测
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order + 授权配置
- **refresh / freshness_sla**：实时/逐笔 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：B-004

### IND-B-005 高成交低仓变比 `volume_position_change_ratio`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：window_volume / abs(position_change)
- **direction_semantics**：值越大越危险
- **unit**：倍数
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Trades + Position Snapshot
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：B-008

### IND-B-006 执行滑点 `slippage`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：(exec_price - expected_price) / expected_price
- **direction_semantics**：值越大越危险
- **unit**：bps
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order + Fills + Orderbook
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：B-006

### IND-B-007 手工单异常标记 `manual_order_flag` `[signal]`

- **layer**：L3 — 判定信号
- **type**：A — 实时可读(RT)
- **indicator_class**：signal
- **mathematical_definition**：order_source NOT IN allowed_source_list → flag; 结合audit_log交叉验证
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Order来源标签(order_source) + 内部审计日志(audit_log) + 允许来源白名单(allowed_source_list)
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：事件触发（T0/T1）
- **downstream_rules**：B-007

### IND-B-009 自成交检测 `self_trade_flag`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：同一归属账户组内的对手方成交检测
- **direction_semantics**：true=异常
- **unit**：bool/count
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account_group，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Fills（匹配 maker/taker 账户）
- **refresh / freshness_sla**：逐笔/窗口 / ≤500ms
- **suggested_update_time**：1min（T3）
- **downstream_rules**：B-009

### IND-B-010 挂撤单比 `order_to_trade_ratio`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：(new_order_count_window + cancel_count_window) / max(fill_count_window, 1)
- **direction_semantics**：值越大越危险
- **unit**：ratio
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Order 创建+取消记录
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：B-010

### IND-B-012 未对冲腿数量 `open_leg_count`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：hedge_group中尚未匹配对冲的腿数
- **direction_semantics**：值越大越危险
- **unit**：count
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Order/Fill 多腿关联 + hedge_group_id
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：B-012

### IND-B-012a 最久未对冲腿时长 `oldest_unhedged_leg_age_ms`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：now() - oldest_open_leg_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Order/Fill 多腿关联 + hedge_group_id
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：B-012

### IND-B-012b 未对冲敞口(USD) `unhedged_exposure_usd`

- **layer**：L2 — 衍生指标
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：Σ(open_leg_qty × mark_price)
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account+symbol，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Open legs position × Mark Price
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：B-012

---

## 6. C-合规 类指标（2 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-C-003 | 做市资格状态 mm_qualification | L0 | A | metric | account_group | 定时 | 15min（T3） | C-003 |
| IND-C-004 | 账户限制状态 account_restriction | L0 | A | metric | account_group | 定时/事件 | 5min或事件触发（T3） | C-004 |

### IND-C-003 做市资格状态 `mm_qualification`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取资格状态+考核指标
- **direction_semantics**：值越大越危险
- **unit**：status
- **value_type**：enum，**output_shape**：scalar
- **entity_grain**：account_group，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：交易所做市状态 API + 做市项目考核
- **refresh / freshness_sla**：定时 / ≤60s
- **suggested_update_time**：15min（T3）
- **downstream_rules**：C-003

### IND-C-004 账户限制状态 `account_restriction`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取限制类型+错误码监测
- **direction_semantics**：值越大越危险
- **unit**：status
- **value_type**：enum，**output_shape**：scalar
- **entity_grain**：account_group，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：交易所账户状态 API + API 权限检测
- **refresh / freshness_sla**：定时/事件 / ≤60s
- **suggested_update_time**：5min或事件触发（T3）
- **downstream_rules**：C-004

---

## 7. P-权限 类指标（11 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-P-001 | 提币权限状态 withdraw_enabled | L0 | A | metric | account | 定时 | 15min（T3） | P-001 |
| IND-P-002 | 授权范围匹配 permission_scope_match | L0 | A | metric | account | 定时/事件 | 5min或事件触发（T3） | P-002 |
| IND-P-003 | Key生命周期状态 key_lifecycle | L1 | D | metric | account | 定时 | 1h（T3） | P-003 |
| IND-P-004 | 权限扩散度 permission_spread | L1 | D | metric | account | 定时 | 15min（T3） | P-004 |
| IND-P-005 | 提币单笔金额 withdraw_amount | L0 | D | metric | account | 实时/事件 | 事件触发（T0） | P-005 |
| IND-P-005a | 提币频率(24h) withdraw_frequency_24h | L1 | B | metric | account | 窗口聚合 | 1min（T2） | P-005 |
| IND-P-005b | 新提币地址标记 is_new_withdraw_address 🔶 | L3 | A | signal | account | 实时/事件 | 事件触发（T0） | P-005 |
| IND-P-006 | 划转单笔金额 transfer_amount | L0 | C | metric | account | 实时/事件 | 事件触发（T0） | P-006 |
| IND-P-006a | 划转频率(24h) transfer_frequency_24h | L1 | B | metric | account | 窗口聚合 | 1min（T2） | P-006 |
| IND-P-006b | 划转路由白名单匹配 transfer_route_match 🔶 | L3 | A | signal | account | 实时/事件 | 事件触发（T0） | P-006 |
| IND-P-007 | IP白名单变更 ip_whitelist_change | L0 | D | metric | account | 定时/事件 | 5min或事件触发（T3） | P-007 |

### IND-P-001 提币权限状态 `withdraw_enabled`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：直接读取 withdraw_enabled 布尔值
- **direction_semantics**：false=异常（权限应为enabled，false表示被禁用或异常）
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：交易所 API Key 权限查询
- **refresh / freshness_sla**：定时 / ≤60s
- **suggested_update_time**：15min（T3）
- **downstream_rules**：P-001

### IND-P-002 授权范围匹配 `permission_scope_match`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：Key 实际权限 vs 授权范围模型匹配
- **direction_semantics**：false=异常（权限范围不匹配）
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：API Key 权限配置 + 授权范围配置
- **refresh / freshness_sla**：定时/事件 / ≤60s
- **suggested_update_time**：5min或事件触发（T3）
- **downstream_rules**：P-002

### IND-P-003 Key生命周期状态 `key_lifecycle`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：key_age_days, last_rotation_days, risk_level 综合
- **direction_semantics**：值越大越危险
- **unit**：days/level
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：API Key 管理系统
- **refresh / freshness_sla**：定时 / ≤60s
- **suggested_update_time**：1h（T3）
- **downstream_rules**：P-003

### IND-P-004 权限扩散度 `permission_spread`

- **layer**：L1 — 标准化基础
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：高危权限持有者数/比例
- **direction_semantics**：值越大越危险
- **unit**：count/ratio
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：calendar_window
- **source_of_truth**：API Key 权限台账 + 账户归属
- **refresh / freshness_sla**：定时 / ≤60s
- **suggested_update_time**：15min（T3）
- **downstream_rules**：P-004

### IND-P-005 提币单笔金额 `withdraw_amount`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：单笔提币金额(USD)
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：交易所提币 API/WS
- **refresh / freshness_sla**：实时/事件 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：P-005

### IND-P-005a 提币频率(24h) `withdraw_frequency_24h`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：24h窗口内提币次数
- **direction_semantics**：值越大越危险
- **unit**：count
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：交易所提币记录
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：1min（T2）
- **downstream_rules**：P-005

### IND-P-005b 新提币地址标记 `is_new_withdraw_address` `[signal]`

- **layer**：L3 — 判定信号
- **type**：A — 实时可读(RT)
- **indicator_class**：signal
- **mathematical_definition**：withdraw_address NOT IN known_address_list
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：交易所提币记录 + 地址簿白名单
- **refresh / freshness_sla**：实时/事件 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：P-005

### IND-P-006 划转单笔金额 `transfer_amount`

- **layer**：L0 — 原始观测
- **type**：C — 跨源衍生(DRV)
- **indicator_class**：metric
- **mathematical_definition**：单笔划转金额(USD)
- **direction_semantics**：值越大越危险
- **unit**：USD
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：交易所划转 API
- **refresh / freshness_sla**：实时/事件 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：P-006

### IND-P-006a 划转频率(24h) `transfer_frequency_24h`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：24h窗口内划转次数
- **direction_semantics**：值越大越危险
- **unit**：count
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：交易所划转记录
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：1min（T2）
- **downstream_rules**：P-006

### IND-P-006b 划转路由白名单匹配 `transfer_route_match` `[signal]`

- **layer**：L3 — 判定信号
- **type**：A — 实时可读(RT)
- **indicator_class**：signal
- **mathematical_definition**：transfer_route IN allowed_topology
- **direction_semantics**：false=异常（划转路由不在白名单内）
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：交易所划转记录 + 合法路由拓扑配置
- **refresh / freshness_sla**：实时/事件 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：P-006

### IND-P-007 IP白名单变更 `ip_whitelist_change`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：变更事件检测（新增/删除/替换/清空）
- **direction_semantics**：值越大越危险
- **unit**：event
- **value_type**：enum，**output_shape**：scalar
- **entity_grain**：account，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：API Key 配置变更事件 + 白名单快照
- **refresh / freshness_sla**：定时/事件 / ≤60s
- **suggested_update_time**：5min或事件触发（T3）
- **downstream_rules**：P-007

---

## 8. BASE-基线 类指标（5 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-BASE-001 | 策略时间窗口 time_window_check | L0 | A | metric | strategy | 实时 | 事件触发（T0） | BASE-001 |
| IND-BASE-002 | 订单规模 order_size_check | L1 | A | metric | strategy | 逐笔 | 事件触发（T0） | BASE-002 |
| IND-BASE-003 | 价格偏离度 price_deviation | L1 | A | metric | strategy | 逐笔 | 事件触发（T0） | BASE-003 |
| IND-BASE-004 | 下单间隔/节奏 order_interval | L1 | B | metric | strategy | 逐笔/窗口 | 30s（T2） | BASE-004 |
| IND-BASE-005 | 账户/IP分发集中度 dispatch_concentration | L1 | B | metric | strategy | 窗口聚合 | 1min（T2/T3） | BASE-005 |

### IND-BASE-001 策略时间窗口 `time_window_check`

- **layer**：L0 — 原始观测
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：current_time IN allowed_window
- **direction_semantics**：false=异常（不在允许交易时间窗口内）
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Strategy Config + 系统时钟
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：BASE-001

### IND-BASE-002 订单规模 `order_size_check`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：order_qty/value vs min/max 配置
- **direction_semantics**：值越大越危险
- **unit**：USD/qty
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order Request + Strategy Config
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：BASE-002

### IND-BASE-003 价格偏离度 `price_deviation`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：abs(order_price - mid_price) / mid_price
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order Request + Mid/Mark Price
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：BASE-003

### IND-BASE-004 下单间隔/节奏 `order_interval`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：相邻 order/exec 时间间隔 vs 允许范围
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Order/Trades 时间戳序列 + Strategy Config
- **refresh / freshness_sla**：逐笔/窗口 / ≤500ms
- **suggested_update_time**：30s（T2）
- **downstream_rules**：BASE-004

### IND-BASE-005 账户/IP分发集中度 `dispatch_concentration`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：account/ip concentration ratio
- **direction_semantics**：值越大越危险
- **unit**：ratio
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Order/Trades + 执行器 IP 分发记录
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：1min（T2/T3）
- **downstream_rules**：BASE-005

---

## 9. STR-策略 类指标（9 个）

| 指标ID | 指标名称 | 层级 | 类型 | 类别 | 粒度 | 刷新频率 | 建议更新时间 | 消费规则 |
|--------|----------|------|------|------|------|----------|--------------|----------|
| IND-STR-001 | 策略心跳 heartbeat | L0 | D | metric | strategy | 实时 | 1s（T1） | STG-001 |
| IND-STR-002 | 套利净收益 arb_net_spread | L1 | A | metric | strategy | 实时 | 1s（T1） | ARB-002 |
| IND-STR-003 | 挂单中价偏离 order_mid_cross | L1 | A | metric | strategy | 逐笔 | 事件触发（T0） | LIQ-001 |
| IND-STR-004 | 网格结构参数 grid_structure | L1 | A | metric | strategy | 逐笔 | 事件触发（T0） | LIQ-002 |
| IND-STR-005 | 单tick资本投放率 tick_capital_ratio | L1 | A | metric | strategy | 逐tick | 逐tick（T0/T1） | LIQ-003 |
| IND-STR-006 | 理想价格偏离 ideal_price_dev | L1 | A | metric | strategy | 逐笔 | 事件触发（T0） | MAN-001 |
| IND-STR-007 | 方向集中度 direction_concentration | L1 | B | metric | strategy | 窗口聚合 | 30s（T2） | MAN-002 |
| IND-STR-008 | 累计执行进度 execution_progress | L1 | B | metric | strategy | 累计 | 1min（T2/T3） | ACCDIS-001 |
| IND-STR-009 | 执行价格带 price_band_check | L1 | A | metric | strategy | 逐笔 | 事件触发（T0） | ACCDIS-002 |

### IND-STR-001 策略心跳 `heartbeat`

- **layer**：L0 — 原始观测
- **type**：D — 系统自观测(SYS)
- **indicator_class**：metric
- **mathematical_definition**：now() - last_heartbeat_ts
- **direction_semantics**：值越大越危险
- **unit**：ms
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：策略进程上报心跳
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：STG-001

### IND-STR-002 套利净收益 `arb_net_spread`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：(sellPrice - buyPrice) / midPrice（扣费后）
- **direction_semantics**：中性
- **unit**：bps
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：point_in_time
- **source_of_truth**：Orderbook（Ask/Bid）+ 手续费率配置
- **refresh / freshness_sla**：实时 / ≤1s
- **suggested_update_time**：1s（T1）
- **downstream_rules**：ARB-002

### IND-STR-003 挂单中价偏离 `order_mid_cross`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：BUY 且 execPrice > midPrice / SELL 且 execPrice < midPrice
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order + Orderbook mid_price
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：LIQ-001

### IND-STR-004 网格结构参数 `grid_structure`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：grid levels/spread/tolerance vs 配置
- **direction_semantics**：值越大越危险
- **unit**：config
- **value_type**：enum，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Strategy Config + 实际挂单分布
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：LIQ-002

### IND-STR-005 单tick资本投放率 `tick_capital_ratio`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：Σ limitOrderPlaced.size / (maxCapitalAllocationPct × baseTotal)
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Order + Inventory
- **refresh / freshness_sla**：逐tick / ≤500ms
- **suggested_update_time**：逐tick（T0/T1）
- **downstream_rules**：LIQ-003

### IND-STR-006 理想价格偏离 `ideal_price_dev`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：(exec_price - ideal_price) / ideal_price
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Trades + 策略理想价格（TWAP）
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：MAN-001

### IND-STR-007 方向集中度 `direction_concentration`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：Σbuy / Σsell（窗口内）
- **direction_semantics**：值越大越危险
- **unit**：ratio
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Trades 聚合
- **refresh / freshness_sla**：窗口聚合 / ≤30s
- **suggested_update_time**：30s（T2）
- **downstream_rules**：MAN-002

### IND-STR-008 累计执行进度 `execution_progress`

- **layer**：L1 — 标准化基础
- **type**：B — 窗口聚合(AGG)
- **indicator_class**：metric
- **mathematical_definition**：executed_total / planned_total
- **direction_semantics**：值越大越危险
- **unit**：%
- **value_type**：float，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：rolling_window
- **source_of_truth**：Trades 累计 + 策略计划配置
- **refresh / freshness_sla**：累计 / ≤30s
- **suggested_update_time**：1min（T2/T3）
- **downstream_rules**：ACCDIS-001

### IND-STR-009 执行价格带 `price_band_check`

- **layer**：L1 — 标准化基础
- **type**：A — 实时可读(RT)
- **indicator_class**：metric
- **mathematical_definition**：exec_price vs priceLimit
- **direction_semantics**：true=异常
- **unit**：bool
- **value_type**：bool，**output_shape**：scalar
- **entity_grain**：strategy，**market_scope**：both
- **time_mode**：event_driven
- **source_of_truth**：Trades + 策略价格带配置
- **refresh / freshness_sla**：逐笔 / ≤500ms
- **suggested_update_time**：事件触发（T0）
- **downstream_rules**：ACCDIS-002

---

## 10. 规则→指标映射表（含TBD占位）

| 规则ID | 消费指标 | 状态 |
|--------|----------|------|
| ACCDIS-001 | IND-STR-008 | ✅ |
| ACCDIS-002 | IND-STR-009 | ✅ |
| ARB-002 | IND-STR-002 | ✅ |
| B-001 | IND-B-001 | ✅ |
| B-002 | IND-B-002 | ✅ |
| B-003 | IND-B-003 | ✅ |
| B-004 | IND-B-004 | ✅ |
| B-006 | IND-B-006 | ✅ |
| B-007 | IND-B-007 | ✅ |
| B-008 | IND-B-005 | ⚠️ 部分(IND-B-008 TBD) |
| B-009 | IND-B-009 | ✅ |
| B-010 | IND-B-010 | ✅ |
| B-011 | — | ⚠️ TBD (IND-B-011) |
| B-012 | IND-B-012, IND-B-012a, IND-B-012b | ✅ |
| BASE-001 | IND-BASE-001 | ✅ |
| BASE-002 | IND-BASE-002 | ✅ |
| BASE-003 | IND-BASE-003 | ✅ |
| BASE-004 | IND-BASE-004 | ✅ |
| BASE-005 | IND-BASE-005 | ✅ |
| C-001 | — | ⚠️ TBD (IND-C-001, 依赖B-011+B-009) |
| C-002 | — | ⚠️ TBD (IND-C-002) |
| C-003 | IND-C-003 | ✅ |
| C-004 | IND-C-004 | ✅ |
| E-001 | IND-E-001 | ✅ |
| E-002 | IND-E-002 | ✅ |
| E-003 | IND-E-003 | ✅ |
| E-004 | IND-E-004 | ✅ |
| E-005 | IND-E-005 | ✅ |
| E-006 | IND-E-006 | ✅ |
| E-007 | IND-E-007, IND-E-007a | ✅ |
| E-008 | IND-E-008, IND-M-004 | ✅ |
| E-009 | IND-E-009 | ✅ |
| L-001 | IND-L-001 | ✅ |
| L-002 | IND-L-002 | ✅ |
| L-003 | IND-L-003 | ✅ |
| L-004 | IND-L-004 | ✅ |
| L-005 | IND-L-005 | ✅ |
| LIQ-001 | IND-STR-003 | ✅ |
| LIQ-002 | IND-STR-004 | ✅ |
| LIQ-003 | IND-STR-005 | ✅ |
| M-001 | IND-M-001, IND-M-001a | ✅ |
| M-002 | IND-M-002, IND-M-002a | ✅ |
| M-004 | IND-M-004 | ✅ |
| M-005 | IND-M-005 | ✅ |
| M-006 | IND-M-006 | ✅ |
| M-007 | IND-M-007 | ✅ |
| M-008 | — | ⚠️ TBD (IND-M-008) |
| M-009 | IND-M-005, IND-M-006 | ⚠️ 部分(IND-M-008 TBD, IND-M-009 TBD) |
| MAN-001 | IND-STR-006 | ✅ |
| MAN-002 | IND-B-002, IND-STR-007 | ✅ |
| P-001 | IND-P-001 | ✅ |
| P-002 | IND-P-002 | ✅ |
| P-003 | IND-P-003 | ✅ |
| P-004 | IND-P-004 | ✅ |
| P-005 | IND-P-005, IND-P-005a, IND-P-005b | ✅ |
| P-006 | IND-P-006, IND-P-006a, IND-P-006b | ✅ |
| P-007 | IND-P-007 | ✅ |
| S-001 | IND-S-001, IND-S-001a, IND-S-001b | ✅ |
| S-002 | IND-S-002, IND-S-002a, IND-S-002b | ✅ |
| S-003 | IND-S-003 | ✅ |
| S-004 | IND-S-001, IND-S-001a, IND-S-001b, IND-S-002, IND-S-002a, IND-S-002b, IND-S-003, IND-S-004 | ✅ |
| S-005 | IND-S-005 | ✅ |
| S-006 | — | ⚠️ TBD (IND-S-006) |
| S-007 | IND-S-007 | ✅ |
| S-008 | IND-S-008 | ✅ |
| S-009 | IND-S-009 | ✅ |
| S-010 | IND-S-010 | ✅ |
| S-011 | — | ⚠️ TBD (IND-S-011) |
| S-012 | — | ⚠️ TBD (IND-S-012) |
| S-013 | — | ⚠️ TBD (IND-S-013) |
| S-014 | IND-S-014 | ✅ |
| S-015 | IND-S-015, IND-S-015a, IND-S-015b, IND-S-015c, IND-S-015d | ✅ |
| S-016 | IND-S-016 | ✅ |
| S-017 | IND-S-017, IND-S-017a | ✅ |
| S-018 | IND-S-018 | ✅ |
| STG-001 | IND-STR-001 | ✅ |

---

## 11. 命名规范

- **ID**：`IND-{CATEGORY}-{NUMBER}[a-z]`（子指标加后缀a/b/c）
- **code name**：小写+下划线，描述「量」而非「结论」
- **bool/flag**：仅外部返回bool或确为事件信号时使用，且必须标记 `indicator_class=signal`
- **indicator_class**：`metric`(基础量/可复用计算口径) 或 `signal`(判定信号/接近规则输出)

## 12. 总原则

**指标库优先沉淀「可复用的计算口径」，而不是「某条规则的判定结果」；能抽成基础量，就不要过早固化成 flag。**

已有的6个signal类指标中，5个为历史保留，1个（IND-S-017 internal_health_score）为健康度治理子系统的 L3 聚合信号。后续不应再新增 signal 类指标。新指标一律输出数值量，判定留给规则层。