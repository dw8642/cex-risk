# 指标库 V1.0 — 风控系统指标定义与规则映射

> 本文档从 `rule_library_v3.0` 中提取并归一化，将规则的「监控参数」「数据来源」「计算逻辑」拆分为独立的指标定义。
> 规则库定义「判什么」（阈值 + 告警 + 动作），指标库定义「算什么」（数据源 + 计算 + 刷新频率）。

---

## 0. 总览

共 **72 个指标**，覆盖 **74 条规则**。

### 0.1 指标类型分布

| 类型 | 含义 | 数量 |
|------|------|------|
| A — 实时可读 (RT) | — | 23 |
| B — 窗口聚合 (AGG) | — | 14 |
| C — 跨源衍生 (DRV) | — | 10 |
| D — 系统自观测 (SYS) | — | 14 |
| E — 待量化 (TBD) | — | 11 |
| **合计** | | **72** |

### 0.2 分类分布

| 类别 | 指标数 | 对应规则类 |
|------|--------|----------|
| 清算 | 5 | L-清算 |
| 敞口 | 9 | E-敞口 |
| 系统 | 13 | S-系统 |
| 市场 | 8 | M-市场 |
| 行为 | 12 | B-行为 |
| 合规 | 4 | C-合规 |
| 权限 | 7 | P-权限 |
| 基线 | 5 | BASE-基线 |
| 策略 | 9 | STR-策略 |
| **合计** | **72** | |

### 0.3 衍生指标（有上游依赖）

共 **7** 个衍生指标：

| 衍生指标 | 上游依赖 | 消费规则 |
|----------|----------|----------|
| IND-E-004 总绝对敞口(USD) | IND-E-002 | E-004 |
| IND-E-007 风险预算使用率 | IND-E-001 + IND-E-004 | E-007 |
| IND-S-004 风控系统整体健康 | IND-S-001 + IND-S-002 + IND-S-003 | S-004 |
| IND-M-009 微观结构综合分 | IND-M-005 + IND-M-006 + IND-M-008 | M-009 |
| IND-B-008 对敲/刷量嫌疑分 | IND-B-005 | B-008 |
| IND-C-001 多账户关联合规分 | IND-B-011 + IND-B-009 | C-001 |
| IND-STR-007 方向集中度 | IND-B-002 | MAN-002 |

### 0.4 跨规则复用指标

共 **12** 个指标被 ≥2 条规则消费：

| 指标 | 消费规则 | 复用说明 |
|------|----------|----------|
| IND-E-003 对冲缺口(USD) | E-003, ARB-001(已降级) | E-003 平台级 + ARB-001 策略级（已降级为E-003参数） |
| IND-S-001 公共数据订阅健康 | S-001, S-004 | S-001 直接消费 + S-004 meta-rule 聚合 |
| IND-S-002 私有数据订阅健康 | S-002, S-004 | S-002 直接消费 + S-004 meta-rule 聚合 |
| IND-S-003 数据快照新鲜度 | S-003, S-004 | S-003 直接消费 + S-004 meta-rule 聚合 |
| IND-M-001 波动率 | M-001, M-003(已合并) | M-001 主检测 + M-003 联动动作（已合并至M-001） |
| IND-M-004 资金费率 | M-004, E-008 | M-004 费率异常检测 + E-008 累计出血计算 |
| IND-M-005 市场深度 | M-005, M-009 | M-005 深度骤降检测 + M-009 微观结构综合评分 |
| IND-M-006 买卖点差 | M-006, M-009 | M-006 点差扩张检测 + M-009 微观结构综合评分 |
| IND-M-008 毒性订单流指标 | M-008, M-009 | M-008 毒性流检测 + M-009 微观结构综合评分 |
| IND-B-002 单边成交比例 | B-002, MAN-002 | B-002 平台级通用 + MAN-002 策略专属方向集中度 |
| IND-B-005 高成交低仓变比 | B-005(已合并→B-008), B-008 | B-005 独立检测（已合并）+ B-008 作为子信号 |
| IND-B-011 多账户行为相似度 | B-011, C-001 | B-011 行为检测层 + C-001 合规判定层 |

---

## 1. L-清算 类指标（5 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-L-001 | 维持保证金率 maint_margin_rate | A | 实时/定时REST | L-001 |
| IND-L-002 | 爆仓距离 distance_pct | A | 实时/定时REST | L-002 |
| IND-L-003 | ADL等级 adl | A | 实时/定时REST | L-003 |
| IND-L-004 | 单边OI占比 oi_ratio | C | 定时REST | L-004 |
| IND-L-005 | 抵押物折后保证金率 collateral_margin_ratio | C | 定时REST | L-005 |

### IND-L-001 维持保证金率 maint_margin_rate

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所账户 REST API（maintMarginRatio 字段）
- **计算逻辑**：直接读取，无需计算
- **刷新频率**：实时/定时REST
- **输出单位**：%
- **消费规则**：L-001

### IND-L-002 爆仓距离 distance_pct

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所 Position REST API（liq_price, mark_price）
- **计算逻辑**：abs(liq_price - mark_price) / mark_price
- **刷新频率**：实时/定时REST
- **输出单位**：%
- **消费规则**：L-002

### IND-L-003 ADL等级 adl

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所 Position REST API（adl 字段）
- **计算逻辑**：直接读取，离散档位
- **刷新频率**：实时/定时REST
- **输出单位**：档位
- **消费规则**：L-003

### IND-L-004 单边OI占比 oi_ratio

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：Position REST API + 公共OI API
- **计算逻辑**：abs(net_position) / market_total_oi（需区分单向/双向持仓模式）
- **刷新频率**：定时REST
- **输出单位**：%
- **消费规则**：L-004

### IND-L-005 抵押物折后保证金率 collateral_margin_ratio

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：账户 Asset REST API + 抵押物价格 Feed + 交易所折扣率配置
- **计算逻辑**：Σ(qty × price × (1-haircut_rate)) / total_margin_required
- **刷新频率**：定时REST
- **输出单位**：倍数
- **消费规则**：L-005

---

## 2. E-敞口 类指标（9 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-E-001 | 净Delta(USD) net_delta_usd | C | 定时REST | E-001 |
| IND-E-002 | 单市场暴露(USD) position_usd | C | 定时REST | E-002 |
| IND-E-003 | 对冲缺口(USD) hedge_gap_usd | C | 定时REST | E-003, ARB-001(已降级) |
| IND-E-004 | 总绝对敞口(USD) total_abs_exposure_usd | C | 定时REST | E-004 |
| IND-E-005 | 仓位变动速率 position_change_rate | B | 窗口聚合 | E-005 |
| IND-E-006 | 可用资金 available_balance | A | 实时/定时REST | E-006 |
| IND-E-007 | 风险预算使用率 risk_budget_usage | B | 窗口聚合 | E-007 |
| IND-E-008 | 资金费率累计出血 funding_bleed | B | 每8h/窗口聚合 | E-008 |
| IND-E-009 | 资金周期对账差异 fund_reconciliation_diff | C | 定时对账 | E-009 |

### IND-E-001 净Delta(USD) net_delta_usd

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：Position REST API（多账户持仓）+ Mark Price
- **计算逻辑**：Σ(position_qty × mark_price × direction_sign)，按账户/组聚合
- **刷新频率**：定时REST
- **输出单位**：USD
- **消费规则**：E-001

### IND-E-002 单市场暴露(USD) position_usd

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：Position REST API + Mark Price
- **计算逻辑**：abs(position_qty) × mark_price，按 exchange+symbol 维度
- **刷新频率**：定时REST
- **输出单位**：USD
- **消费规则**：E-002

### IND-E-003 对冲缺口(USD) hedge_gap_usd

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：Position REST API（多账户）+ Mark Price + 对冲映射配置
- **计算逻辑**：abs(leg_a_usd - leg_b_usd × expected_ratio)
- **刷新频率**：定时REST
- **输出单位**：USD
- **消费规则**：E-003, ARB-001(已降级)

### IND-E-004 总绝对敞口(USD) total_abs_exposure_usd

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：Position REST API + Mark Price
- **计算逻辑**：Σ abs(position_usd)，跨市场汇总
- **刷新频率**：定时REST
- **输出单位**：USD
- **消费规则**：E-004
- **上游依赖**：IND-E-002（单市场暴露(USD)）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

### IND-E-005 仓位变动速率 position_change_rate

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Position REST API（时序快照）
- **计算逻辑**：abs(position_t - position_t-n) / position_t-n，窗口聚合
- **刷新频率**：窗口聚合
- **输出单位**：%/min
- **消费规则**：E-005

### IND-E-006 可用资金 available_balance

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所账户 REST API
- **计算逻辑**：直接读取 availableBalance 字段
- **刷新频率**：实时/定时REST
- **输出单位**：USD
- **消费规则**：E-006

### IND-E-007 风险预算使用率 risk_budget_usage

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Position + Balance + 风控配置
- **计算逻辑**：当前风险占用 / 预设风险预算上限
- **刷新频率**：窗口聚合
- **输出单位**：%
- **消费规则**：E-007
- **上游依赖**：IND-E-001（净Delta(USD)）, IND-E-004（总绝对敞口(USD)）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

### IND-E-008 资金费率累计出血 funding_bleed

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：交易所 Funding Rate API + Position
- **计算逻辑**：Σ(funding_rate × position_value)，滚动窗口累计
- **刷新频率**：每8h/窗口聚合
- **输出单位**：USD
- **消费规则**：E-008

### IND-E-009 资金周期对账差异 fund_reconciliation_diff

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：交易所划转/提币记录 + 内部账本
- **计算逻辑**：交易所余额 - 内部账本余额
- **刷新频率**：定时对账
- **输出单位**：USD
- **消费规则**：E-009

---

## 3. S-系统 类指标（13 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-S-001 | 公共数据订阅健康 public_feed_health | D | 实时心跳 | S-001, S-004 |
| IND-S-002 | 私有数据订阅健康 private_feed_health | D | 实时心跳 | S-002, S-004 |
| IND-S-003 | 数据快照新鲜度 data_freshness | D | 定时检测 | S-003, S-004 |
| IND-S-004 | 风控系统整体健康 system_blind_score | D | 实时 | S-004 |
| IND-S-005 | API频控使用率 api_rate_usage | D | 实时累计 | S-005 |
| IND-S-006 | 数据真相源一致性 data_consistency_diff | E | 定时 | S-006 |
| IND-S-007 | 指标计算延迟 indicator_calc_latency | D | 实时 | S-007 |
| IND-S-008 | 规则引擎运行状态 rule_engine_health | D | 实时 | S-008 |
| IND-S-009 | Watchdog主链路状态 watchdog_health | D | 实时 | S-009 |
| IND-S-010 | 交易所/Symbol可交易状态 tradeable_status | A | 实时/定时 | S-010 |
| IND-S-011 | 群组控制健康 group_control_health | E | 定时 | S-011 |
| IND-S-012 | 同源策略批量异常 batch_anomaly_ratio | E | 窗口聚合 | S-012 |
| IND-S-013 | API契约漂移 schema_diff | E | 定时 | S-013 |

### IND-S-001 公共数据订阅健康 public_feed_health

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：WebSocket 连接状态 + 心跳监测
- **计算逻辑**：last_msg_ts 延迟检测 + 连接状态布尔
- **刷新频率**：实时心跳
- **输出单位**：bool/ms
- **消费规则**：S-001, S-004

### IND-S-002 私有数据订阅健康 private_feed_health

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：WebSocket 私有连接状态 + 心跳监测
- **计算逻辑**：last_msg_ts 延迟检测 + 连接状态布尔
- **刷新频率**：实时心跳
- **输出单位**：bool/ms
- **消费规则**：S-002, S-004

### IND-S-003 数据快照新鲜度 data_freshness

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：各数据快照的时间戳
- **计算逻辑**：now() - snapshot_ts > max_age
- **刷新频率**：定时检测
- **输出单位**：ms/bool
- **消费规则**：S-003, S-004

### IND-S-004 风控系统整体健康 system_blind_score

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：IND-S-001 + IND-S-002 + IND-S-003 输出
- **计算逻辑**：Meta-rule：综合判定 public+private+freshness 是否构成"失明"
- **刷新频率**：实时
- **输出单位**：score/bool
- **消费规则**：S-004
- **上游依赖**：IND-S-001（公共数据订阅健康）, IND-S-002（私有数据订阅健康）, IND-S-003（数据快照新鲜度）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

### IND-S-005 API频控使用率 api_rate_usage

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：API 调用计数器 + 交易所限频配置
- **计算逻辑**：current_usage / rate_limit
- **刷新频率**：实时累计
- **输出单位**：%
- **消费规则**：S-005

### IND-S-006 数据真相源一致性 data_consistency_diff

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：REST 真相源 vs WS/内部快照
- **计算逻辑**：各维度(position/balance/order/price)差异比对
- **刷新频率**：定时
- **输出单位**：diff
- **消费规则**：S-006

### IND-S-007 指标计算延迟 indicator_calc_latency

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：指标引擎内部计时
- **计算逻辑**：indicator_output_ts - data_input_ts
- **刷新频率**：实时
- **输出单位**：ms
- **消费规则**：S-007

### IND-S-008 规则引擎运行状态 rule_engine_health

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：规则引擎心跳 + 异常日志
- **计算逻辑**：异常率/延迟/panic 检测
- **刷新频率**：实时
- **输出单位**：bool
- **消费规则**：S-008

### IND-S-009 Watchdog主链路状态 watchdog_health

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：Watchdog 心跳 + 端到端探针
- **计算逻辑**：主风控链路可达性检测
- **刷新频率**：实时
- **输出单位**：bool
- **消费规则**：S-009

### IND-S-010 交易所/Symbol可交易状态 tradeable_status

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所 exchangeInfo / Symbol Status API
- **计算逻辑**：直接读取 status 字段
- **刷新频率**：实时/定时
- **输出单位**：bool
- **消费规则**：S-010

### IND-S-011 群组控制健康 group_control_health

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：群组配置 + 控制动作执行日志
- **计算逻辑**：配置一致性 + 动作覆盖率 + 状态回写一致性
- **刷新频率**：定时
- **输出单位**：score
- **消费规则**：S-011

### IND-S-012 同源策略批量异常 batch_anomaly_ratio

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：策略状态聚合 + 告警聚合 + 策略元数据
- **计算逻辑**：同源（模板/版本/团队）策略中异常比例
- **刷新频率**：窗口聚合
- **输出单位**：%
- **消费规则**：S-012

### IND-S-013 API契约漂移 schema_diff

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：API 响应 schema 比对
- **计算逻辑**：字段变化检测（新增/缺失/类型变化）
- **刷新频率**：定时
- **输出单位**：bool/list
- **消费规则**：S-013

---

## 4. M-市场 类指标（8 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-M-001 | 波动率 volatility | B | 窗口聚合 | M-001, M-003(已合并) |
| IND-M-002 | 交易所接口延迟 exchange_latency | D | 实时 | M-002 |
| IND-M-004 | 资金费率 funding_rate | A | 每8h/实时 | M-004, E-008 |
| IND-M-005 | 市场深度 depth_score | B | 实时/窗口 | M-005, M-009 |
| IND-M-006 | 买卖点差 spread | A | 实时 | M-006, M-009 |
| IND-M-007 | 跨所价格偏离 cross_exchange_deviation | C | 实时 | M-007 |
| IND-M-008 | 毒性订单流指标 toxic_flow | E | 窗口聚合 | M-008, M-009 |
| IND-M-009 | 微观结构综合分 microstructure_score | E | 窗口聚合 | M-009 |

### IND-M-001 波动率 volatility

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：K线/Trades 数据
- **计算逻辑**：窗口内价格标准差/ATR/实现波动率
- **刷新频率**：窗口聚合
- **输出单位**：%
- **消费规则**：M-001, M-003(已合并)

### IND-M-002 交易所接口延迟 exchange_latency

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：API 探针 + WS 延迟监测
- **计算逻辑**：round-trip latency 统计
- **刷新频率**：实时
- **输出单位**：ms
- **消费规则**：M-002

### IND-M-004 资金费率 funding_rate

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所 Funding Rate API
- **计算逻辑**：直接读取
- **刷新频率**：每8h/实时
- **输出单位**：bps
- **消费规则**：M-004, E-008

### IND-M-005 市场深度 depth_score

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Orderbook WS/REST
- **计算逻辑**：各档位累计量/加权深度指标
- **刷新频率**：实时/窗口
- **输出单位**：USD/score
- **消费规则**：M-005, M-009

### IND-M-006 买卖点差 spread

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Orderbook（best_ask - best_bid）
- **计算逻辑**：(best_ask - best_bid) / mid_price
- **刷新频率**：实时
- **输出单位**：bps
- **消费规则**：M-006, M-009

### IND-M-007 跨所价格偏离 cross_exchange_deviation

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：多交易所 Mark Price / Mid Price
- **计算逻辑**：max(abs(price_i - price_j)) / avg_price
- **刷新频率**：实时
- **输出单位**：%
- **消费规则**：M-007

### IND-M-008 毒性订单流指标 toxic_flow

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：Trades + Orderbook
- **计算逻辑**：VPIN/毒性流检测算法（待定义）
- **刷新频率**：窗口聚合
- **输出单位**：score
- **消费规则**：M-008, M-009

### IND-M-009 微观结构综合分 microstructure_score

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：IND-M-005 + IND-M-006 + IND-M-008
- **计算逻辑**：综合加权评分
- **刷新频率**：窗口聚合
- **输出单位**：score
- **消费规则**：M-009
- **上游依赖**：IND-M-005（市场深度）, IND-M-006（买卖点差）, IND-M-008（毒性订单流指标）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

---

## 5. B-行为 类指标（12 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-B-001 | 异常成交放量 volume_anomaly | B | 窗口聚合 | B-001 |
| IND-B-002 | 单边成交比例 side_ratio | B | 窗口聚合 | B-002, MAN-002 |
| IND-B-003 | 单笔订单规模 order_size | A | 实时/逐笔 | B-003 |
| IND-B-004 | 非授权交易标记 unauthorized_trade_flag | A | 实时/逐笔 | B-004 |
| IND-B-005 | 高成交低仓变比 volume_position_change_ratio | B | 窗口聚合 | B-005(已合并→B-008), B-008 |
| IND-B-006 | 执行滑点 slippage | C | 逐笔 | B-006 |
| IND-B-007 | 手工单异常标记 manual_order_flag | A | 实时 | B-007 |
| IND-B-008 | 对敲/刷量嫌疑分 wash_trade_score | E | 窗口聚合 | B-008 |
| IND-B-009 | 自成交检测 self_trade_flag | B | 逐笔/窗口 | B-009 |
| IND-B-010 | 挂撤单比 OTR | B | 窗口聚合 | B-010 |
| IND-B-011 | 多账户行为相似度 behavior_similarity | E | 窗口聚合 | B-011, C-001 |
| IND-B-012 | 单腿暴露 leg_risk | E | 实时 | B-012 |

### IND-B-001 异常成交放量 volume_anomaly

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Trades / Fills 聚合
- **计算逻辑**：window_volume / baseline_volume
- **刷新频率**：窗口聚合
- **输出单位**：倍数
- **消费规则**：B-001

### IND-B-002 单边成交比例 side_ratio

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Trades / Fills 聚合
- **计算逻辑**：buy_volume / total_volume 或 sell_volume / total_volume
- **刷新频率**：窗口聚合
- **输出单位**：%
- **消费规则**：B-002, MAN-002

### IND-B-003 单笔订单规模 order_size

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order Request / Fills
- **计算逻辑**：order_qty × price
- **刷新频率**：实时/逐笔
- **输出单位**：USD
- **消费规则**：B-003

### IND-B-004 非授权交易标记 unauthorized_trade_flag

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order + 授权配置
- **计算逻辑**：order 与授权范围匹配检测
- **刷新频率**：实时/逐笔
- **输出单位**：bool
- **消费规则**：B-004

### IND-B-005 高成交低仓变比 volume_position_change_ratio

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Trades + Position Snapshot
- **计算逻辑**：window_volume / abs(position_change)
- **刷新频率**：窗口聚合
- **输出单位**：倍数
- **消费规则**：B-005(已合并→B-008), B-008

### IND-B-006 执行滑点 slippage

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：Order + Fills + Orderbook
- **计算逻辑**：(exec_price - expected_price) / expected_price
- **刷新频率**：逐笔
- **输出单位**：bps
- **消费规则**：B-006

### IND-B-007 手工单异常标记 manual_order_flag

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order 来源标签
- **计算逻辑**：order_source 非策略引擎且满足异常条件
- **刷新频率**：实时
- **输出单位**：bool
- **消费规则**：B-007

### IND-B-008 对敲/刷量嫌疑分 wash_trade_score

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：Fills + Position + 账户组归属
- **计算逻辑**：多维模式组合评分（高成交低仓变+对称成交+高频小额往返）
- **刷新频率**：窗口聚合
- **输出单位**：score
- **消费规则**：B-008
- **上游依赖**：IND-B-005（高成交低仓变比）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

### IND-B-009 自成交检测 self_trade_flag

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Fills（匹配 maker/taker 账户）
- **计算逻辑**：同一归属账户组内的对手方成交检测
- **刷新频率**：逐笔/窗口
- **输出单位**：bool/count
- **消费规则**：B-009

### IND-B-010 挂撤单比 OTR

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Order 创建+取消记录
- **计算逻辑**：cancel_count / (cancel_count + fill_count)
- **刷新频率**：窗口聚合
- **输出单位**：ratio
- **消费规则**：B-010

### IND-B-011 多账户行为相似度 behavior_similarity

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：多账户 Fills + 账户组归属
- **计算逻辑**：时间/方向/价格/仓位路径多维相似度
- **刷新频率**：窗口聚合
- **输出单位**：score
- **消费规则**：B-011, C-001

### IND-B-012 单腿暴露 leg_risk

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：Order/Fill 多腿关联 + hedge_group_id
- **计算逻辑**：open_leg_count < expected && oldest_leg_age > timeout && unhedged > threshold
- **刷新频率**：实时
- **输出单位**：USD/ms
- **消费规则**：B-012

---

## 6. C-合规 类指标（4 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-C-001 | 多账户关联合规分 compliance_risk_score | E | 窗口聚合 | C-001 |
| IND-C-002 | 多账户规避检测 evasion_pattern_flag | E | 窗口聚合 | C-002 |
| IND-C-003 | 做市资格状态 mm_qualification | A | 定时 | C-003 |
| IND-C-004 | 账户限制状态 account_restriction | A | 定时/事件 | C-004 |

### IND-C-001 多账户关联合规分 compliance_risk_score

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：B-011/B-009 告警输出 + 账户关联关系
- **计算逻辑**：基于行为相似度+自成交+协同模式的合规风险评分
- **刷新频率**：窗口聚合
- **输出单位**：score
- **消费规则**：C-001
- **上游依赖**：IND-B-011（多账户行为相似度）, IND-B-009（自成交检测）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

### IND-C-002 多账户规避检测 evasion_pattern_flag

- **指标类型**：E — 待量化 (TBD)
- **数据来源**：多账户聚合数据 + 平台约束配置
- **计算逻辑**：账户组整体是否拆分规避单一限制
- **刷新频率**：窗口聚合
- **输出单位**：bool/score
- **消费规则**：C-002

### IND-C-003 做市资格状态 mm_qualification

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所做市状态 API + 做市项目考核
- **计算逻辑**：直接读取资格状态+考核指标
- **刷新频率**：定时
- **输出单位**：status
- **消费规则**：C-003

### IND-C-004 账户限制状态 account_restriction

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所账户状态 API + API 权限检测
- **计算逻辑**：直接读取限制类型+错误码监测
- **刷新频率**：定时/事件
- **输出单位**：status
- **消费规则**：C-004

---

## 7. P-权限 类指标（7 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-P-001 | 提币权限状态 withdraw_enabled | A | 定时 | P-001 |
| IND-P-002 | 授权范围匹配 permission_scope_match | A | 定时/事件 | P-002 |
| IND-P-003 | Key生命周期状态 key_lifecycle | D | 定时 | P-003 |
| IND-P-004 | 权限扩散度 permission_spread | D | 定时 | P-004 |
| IND-P-005 | 提币实时监控 withdraw_monitor | D | 实时/事件 | P-005 |
| IND-P-006 | 资金划转监控 transfer_monitor | C | 实时/事件 | P-006 |
| IND-P-007 | IP白名单变更 ip_whitelist_change | D | 定时/事件 | P-007 |

### IND-P-001 提币权限状态 withdraw_enabled

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：交易所 API Key 权限查询
- **计算逻辑**：直接读取 withdraw_enabled 布尔值
- **刷新频率**：定时
- **输出单位**：bool
- **消费规则**：P-001

### IND-P-002 授权范围匹配 permission_scope_match

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：API Key 权限配置 + 授权范围配置
- **计算逻辑**：Key 实际权限 vs 授权范围模型匹配
- **刷新频率**：定时/事件
- **输出单位**：bool
- **消费规则**：P-002

### IND-P-003 Key生命周期状态 key_lifecycle

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：API Key 管理系统
- **计算逻辑**：key_age_days, last_rotation_days, risk_level 综合
- **刷新频率**：定时
- **输出单位**：days/level
- **消费规则**：P-003

### IND-P-004 权限扩散度 permission_spread

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：API Key 权限台账 + 账户归属
- **计算逻辑**：高危权限持有者数/比例
- **刷新频率**：定时
- **输出单位**：count/ratio
- **消费规则**：P-004

### IND-P-005 提币实时监控 withdraw_monitor

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：交易所提币 API/WS + 地址簿
- **计算逻辑**：金额/频率/新地址检测
- **刷新频率**：实时/事件
- **输出单位**：USD/count
- **消费规则**：P-005

### IND-P-006 资金划转监控 transfer_monitor

- **指标类型**：C — 跨源衍生 (DRV)
- **数据来源**：交易所划转 API + 合法拓扑配置
- **计算逻辑**：金额/频率/路由白名单匹配
- **刷新频率**：实时/事件
- **输出单位**：USD/count
- **消费规则**：P-006

### IND-P-007 IP白名单变更 ip_whitelist_change

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：API Key 配置变更事件 + 白名单快照
- **计算逻辑**：变更事件检测（新增/删除/替换/清空）
- **刷新频率**：定时/事件
- **输出单位**：event
- **消费规则**：P-007

---

## 8. BASE-基线 类指标（5 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-BASE-001 | 策略时间窗口 time_window_check | A | 实时 | BASE-001 |
| IND-BASE-002 | 订单规模 order_size_check | A | 逐笔 | BASE-002 |
| IND-BASE-003 | 价格偏离度 price_deviation | A | 逐笔 | BASE-003 |
| IND-BASE-004 | 下单间隔/节奏 order_interval | B | 逐笔/窗口 | BASE-004 |
| IND-BASE-005 | 账户/IP分发集中度 dispatch_concentration | B | 窗口聚合 | BASE-005 |

### IND-BASE-001 策略时间窗口 time_window_check

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Strategy Config + 系统时钟
- **计算逻辑**：current_time IN allowed_window
- **刷新频率**：实时
- **输出单位**：bool
- **消费规则**：BASE-001

### IND-BASE-002 订单规模 order_size_check

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order Request + Strategy Config
- **计算逻辑**：order_qty/value vs min/max 配置
- **刷新频率**：逐笔
- **输出单位**：USD/qty
- **消费规则**：BASE-002

### IND-BASE-003 价格偏离度 price_deviation

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order Request + Mid/Mark Price
- **计算逻辑**：abs(order_price - mid_price) / mid_price
- **刷新频率**：逐笔
- **输出单位**：%
- **消费规则**：BASE-003

### IND-BASE-004 下单间隔/节奏 order_interval

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Order/Trades 时间戳序列 + Strategy Config
- **计算逻辑**：相邻 order/exec 时间间隔 vs 允许范围
- **刷新频率**：逐笔/窗口
- **输出单位**：ms
- **消费规则**：BASE-004

### IND-BASE-005 账户/IP分发集中度 dispatch_concentration

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Order/Trades + 执行器 IP 分发记录
- **计算逻辑**：account/ip concentration ratio
- **刷新频率**：窗口聚合
- **输出单位**：ratio
- **消费规则**：BASE-005

---

## 9. STR-策略 类指标（9 个）

| 指标ID | 指标名称 | 类型 | 刷新频率 | 消费规则 |
|--------|----------|------|----------|----------|
| IND-STR-001 | 策略心跳 heartbeat | D | 实时 | STG-001 |
| IND-STR-002 | 套利净收益 arb_net_spread | A | 实时 | ARB-002 |
| IND-STR-003 | 挂单中价偏离 order_mid_cross | A | 逐笔 | LIQ-001 |
| IND-STR-004 | 网格结构参数 grid_structure | A | 逐笔 | LIQ-002 |
| IND-STR-005 | 单tick资本投放率 tick_capital_ratio | A | 逐tick | LIQ-003 |
| IND-STR-006 | 理想价格偏离 ideal_price_dev | A | 逐笔 | MAN-001 |
| IND-STR-007 | 方向集中度 direction_concentration | B | 窗口聚合 | MAN-002 |
| IND-STR-008 | 累计执行进度 execution_progress | B | 累计 | ACCDIS-001 |
| IND-STR-009 | 执行价格带 price_band_check | A | 逐笔 | ACCDIS-002 |

### IND-STR-001 策略心跳 heartbeat

- **指标类型**：D — 系统自观测 (SYS)
- **数据来源**：策略进程上报心跳
- **计算逻辑**：now() - last_heartbeat_ts
- **刷新频率**：实时
- **输出单位**：ms
- **消费规则**：STG-001

### IND-STR-002 套利净收益 arb_net_spread

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Orderbook（Ask/Bid）+ 手续费率配置
- **计算逻辑**：(sellPrice - buyPrice) / midPrice（扣费后）
- **刷新频率**：实时
- **输出单位**：bps
- **消费规则**：ARB-002

### IND-STR-003 挂单中价偏离 order_mid_cross

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order + Orderbook mid_price
- **计算逻辑**：BUY 且 execPrice > midPrice / SELL 且 execPrice < midPrice
- **刷新频率**：逐笔
- **输出单位**：bool
- **消费规则**：LIQ-001

### IND-STR-004 网格结构参数 grid_structure

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Strategy Config + 实际挂单分布
- **计算逻辑**：grid levels/spread/tolerance vs 配置
- **刷新频率**：逐笔
- **输出单位**：config
- **消费规则**：LIQ-002

### IND-STR-005 单tick资本投放率 tick_capital_ratio

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Order + Inventory
- **计算逻辑**：Σ limitOrderPlaced.size / (maxCapitalAllocationPct × baseTotal)
- **刷新频率**：逐tick
- **输出单位**：%
- **消费规则**：LIQ-003

### IND-STR-006 理想价格偏离 ideal_price_dev

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Trades + 策略理想价格（TWAP）
- **计算逻辑**：(exec_price - ideal_price) / ideal_price
- **刷新频率**：逐笔
- **输出单位**：%
- **消费规则**：MAN-001

### IND-STR-007 方向集中度 direction_concentration

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Trades 聚合
- **计算逻辑**：Σbuy / Σsell（窗口内）
- **刷新频率**：窗口聚合
- **输出单位**：ratio
- **消费规则**：MAN-002
- **上游依赖**：IND-B-002（单边成交比例）
- **⚠️ 衍生指标**：需等上游指标就绪后计算

### IND-STR-008 累计执行进度 execution_progress

- **指标类型**：B — 窗口聚合 (AGG)
- **数据来源**：Trades 累计 + 策略计划配置
- **计算逻辑**：executed_total / planned_total
- **刷新频率**：累计
- **输出单位**：%
- **消费规则**：ACCDIS-001

### IND-STR-009 执行价格带 price_band_check

- **指标类型**：A — 实时可读 (RT)
- **数据来源**：Trades + 策略价格带配置
- **计算逻辑**：exec_price vs priceLimit
- **刷新频率**：逐笔
- **输出单位**：bool
- **消费规则**：ACCDIS-002

---

## 10. 规则 → 指标映射表

| 规则ID | 规则名称 | 消费指标 |
|--------|----------|----------|
| L-001 | 维持保证金占比过高 | IND-L-001 |
| L-002 | 爆仓距离过近 | IND-L-002 |
| L-003 | ADL 风险升高 | IND-L-003 |
| L-004 | 单边 OI 占比过高 | IND-L-004 |
| L-005 | 联合保证金/抵押物折价双杀 | IND-L-005 |
| E-001 | 净 Delta 过高 | IND-E-001 |
| E-002 | 单市场暴露过大（原"单市场仓位集中风险"，实际监控绝对暴露而非集中度占比） | IND-E-002 |
| E-003 | 对冲缺口扩大 | IND-E-003 |
| E-004 | 总绝对敞口上限 | IND-E-004 |
| E-005 | 仓位变动速率异常 | IND-E-005 |
| E-006 | 账户可用资金过低 | IND-E-006 |
| E-007 | 风险预算综合监控 | IND-E-007 |
| E-008 | Funding Rate Bleed（资金费率慢性消耗） | IND-E-008, IND-M-004 |
| E-009 | 资金周期对账差异 | IND-E-009 |
| S-001 | 公共数据订阅断联/延迟 | IND-S-001 |
| S-002 | 私有数据订阅断联/延迟 | IND-S-002 |
| S-003 | 数据陈旧/快照过期检测 | IND-S-003 |
| S-004 | 风控系统整体失明风险 | IND-S-001, IND-S-002, IND-S-003, IND-S-004 |
| S-005 | 风控只读 API 频控使用率过高/耗尽 | IND-S-005 |
| S-006 | 数据真相源不一致 | IND-S-006 |
| S-007 | 指标计算延迟过高 | IND-S-007 |
| S-008 | 规则引擎运行异常 | IND-S-008 |
| S-009 | Watchdog 主风控链路异常 | IND-S-009 |
| S-010 | 交易所/Symbol 临时不可交易 | IND-S-010 |
| S-011 | 群组控制失效 | IND-S-011 |
| S-012 | 同源策略批量异常 | IND-S-012 |
| S-013 | API 契约漂移与静默变更 | IND-S-013 |
| M-001 | 波动率突升 | IND-M-001 |
| M-002 | 交易所接口健康恶化 | IND-M-002 |
| M-003 | 市场异常波动参与度收敛 | IND-M-001 |
| M-004 | 资金费率异常波动 | IND-M-004 |
| M-005 | 深度骤降 | IND-M-005 |
| M-006 | 点差急剧扩张 | IND-M-006 |
| M-007 | 跨所价格偏离 | IND-M-007 |
| M-008 | 毒性订单流/逆向选择恶化 | IND-M-008 |
| M-009 | 微观结构恶化 | IND-M-005, IND-M-006, IND-M-008, IND-M-009 |
| B-001 | 异常成交放量 | IND-B-001 |
| B-002 | 单边成交比例异常 | IND-B-002 |
| B-003 | 单笔订单规模异常 | IND-B-003 |
| B-004 | 非授权交易 | IND-B-004 |
| B-005 | 高成交低净仓变化 | IND-B-005 |
| B-006 | 异常滑点/执行质量劣化 | IND-B-006 |
| B-007 | 手工单异常 | IND-B-007 |
| B-008 | 对敲/刷量嫌疑（基础版） | IND-B-005, IND-B-008 |
| B-009 | Wash Trade / Self-Trade | IND-B-009 |
| B-010 | 挂撤单比例（OTR）异常 | IND-B-010 |
| B-011 | 多账户高相似执行行为 | IND-B-011 |
| B-012 | 单腿暴露（Leg Risk）监控 | IND-B-012 |
| C-001 | 多账户关联执行合规风险 | IND-B-011, IND-C-001 |
| C-002 | 多账户规避平台限制风险 | IND-C-002 |
| C-003 | 做市资格/激励资格受损 | IND-C-003 |
| C-004 | 账户被平台限制/降权 | IND-C-004 |
| P-001 | 提币权限异常开启 | IND-P-001 |
| P-002 | 非授权交易范围开放 | IND-P-002 |
| P-003 | 高危 API Key 未及时停用 | IND-P-003 |
| P-004 | 多账户权限过度扩散风险 | IND-P-004 |
| P-005 | 大额/异常提币实时监控 | IND-P-005 |
| P-006 | 异常资金划转监控 | IND-P-006 |
| P-007 | API Key IP 白名单变更监控 | IND-P-007 |
| BASE-001 | 策略运行时间窗口检查 | IND-BASE-001 |
| BASE-002 | 单笔订单规模/支出范围检查 | IND-BASE-002 |
| BASE-003 | 下单价格偏离限制 | IND-BASE-003 |
| BASE-004 | 下单间隔与节奏检查 | IND-BASE-004 |
| BASE-005 | 执行账户/IP 分发异常检查 | IND-BASE-005 |
| ACCDIS-001 | 周期累计成交总额/总量上限 | IND-STR-008 |
| ACCDIS-002 | 策略价格带偏离检查 | IND-STR-009 |
| ARB-001 | 套利未对冲敞口风险 | IND-E-003 |
| ARB-002 | 套利价差净收益检查 | IND-STR-002 |
| LIQ-001 | 挂单跨越中价检查 | IND-STR-003 |
| LIQ-002 | 网格结构偏离检查 | IND-STR-004 |
| LIQ-003 | 每 tick 最大资本投放控制 | IND-STR-005 |
| MAN-001 | 理想价格偏离控制 | IND-STR-006 |
| MAN-002 | 操纵策略方向集中度控制 | IND-B-002, IND-STR-007 |
| STG-001 | 策略心跳超时 | IND-STR-001 |

---

*生成时间：2026-04-02 | 版本：V1.0 草稿*
