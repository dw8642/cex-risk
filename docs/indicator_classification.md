# 风控规则指标分类 — 按数据获取方式

## 分类定义

| 类型 | 简称 | 含义 | 实现复杂度 | 示例 |
|------|------|------|-----------|------|
| **A: 实时可读** | RT | 交易所 API / WS 直接返回的字段，一次调用即得 | ⭐ 低 | margin_rate, adl, balance, funding_rate |
| **B: 窗口聚合** | AGG | 需要收集一段时间的数据，做时间窗口聚合/累加 | ⭐⭐ 中 | volume_1h, position_change_24h, cumulative_pnl |
| **C: 跨源衍生** | DRV | 需要组合多个数据源（跨账户/跨交易所/跨系统）计算 | ⭐⭐⭐ 中高 | net_delta, hedge_gap, reconciliation_diff |
| **D: 系统自观测** | SYS | 我们自己的基础设施需要产生的指标，不来自交易所 | ⭐⭐ 中（但需要先建设） | ws_connected, data_age, rule_engine_healthy |
| **E: 尚未量化** | TBD | 概念清晰但缺乏明确的可计算指标定义，需要进一步设计 | ⭐⭐⭐⭐ 高 | toxic_flow, microstructure_score, wash_trade_score |

> **为什么不只用三类？**
> - 「实时可读」和「窗口聚合」都来自交易所数据，但实现路径完全不同：前者读一次就行，后者需要时间序列存储 + 聚合逻辑。
> - 「系统自观测」(D) 数据源不是交易所而是我们自己的系统，是需要先建基础设施才能采集的指标。
> - 「跨源衍生」(C) 强调的是多数据源组合，复杂度在于数据对齐而非单一采集。

---

## 逐条规则分类

### 1. L-清算与生存类（5 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| L-001 | 维持保证金占比过高 | `maintMarginRatio` | **A** RT | 交易所 REST API 直接返回，值越高风险越大 |
| L-002 | 爆仓距离过近 | `liq_price`, `mark_price` → `distance_pct` | **A** RT | 两个字段都从 API 直接读取，distance_pct 为简单除法 |
| L-003 | ADL 风险升高 | `adl` (1-5) | **A** RT | 交易所 Position API 直接返回 |
| L-004 | 单边 OI 占比过高 | `position / oi`（分子按持仓模式分支） | **C** DRV | Position API ÷ OI 公共 API，跨源；需先判断 position_mode（单向→net，双向→long/short分别） |
| L-005 | 联合保证金/抵押物折价双杀 | `collateral_margin_ratio = Σ(qty×price×(1-haircut)) / margin_required` | **C** DRV | 抵押物价格 + 折扣率 + 账户资产，多源组合；⚠️ 条件待量化 |

### 2. E-敞口与对账类（9 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| E-001 | 净 Delta 过高 | `net_delta_usd`（现货+U本位分口径换算后聚合） | **C** DRV | 现货 qty×price + U本位 qty×price×multiplier，跨账户+跨品种+分层（账户/组/全局） |
| E-002 | 单市场暴露过大 | `position_usd = abs(qty) × mark_price` | **C** DRV | Position API + 行情源组合计算；Spot/Futures 阈值分开 |
| E-003 | 对冲缺口扩大 | `hedge_gap_usd`（→BASE层，MVP不开发） | **C** DRV | 多账户仓位 + 对冲映射关系，跨源；⏸️ 降级至 BASE，依赖策略元数据体系 |
| E-004 | 总绝对敞口上限 | `total_exposure_usd = Σ abs(position_usd)` | **C** DRV | 多层级聚合（账户/策略/团队/项目/全局），基于四级架构 |
| E-005 | 仓位变动速率异常 | `position_change_usd_1h/24h`（净变化，非绝对变化） | **B** AGG | 仓位快照时间序列；Spot/Futures分阈值，hedge mode可放宽 |
| E-006 | 账户可用资金过低 | `current_available`（Spot: free, Futures: availableBalance） | **A** RT | API 直接返回；Spot/Futures 字段口径不同，阈值分开 |
| E-007 | 风险预算综合监控 | `risk_budget_usage`（水位）+ `consumption_rate_pct`（速率） | **B** AGG | 水位：累亏/预算；速率：亏损增速/预算（%/h），双维度独立判定 |
| E-008 | Funding Rate Bleed | `annualized_funding_rate`（主）, `cumulative_funding_loss` | **B** AGG | 窗口内 funding 结算累计 → 年化换算（主判定），窗口可配（建议默认 3d） |
| E-009 | 资金周期对账差异 | `abs(reconciliation_diff)` | **C** DRV | 消费外部对账系统（Dashboard）结果，风控侧仅做绝对差值超限预警 |

### 3. S-系统类（13 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| S-001 | 公共数据订阅断联/延迟 | 连接健康(`connected`, `heartbeat_gap`, `reconnect_count`, `latency`) + 业务鲜活(`data_gap_ms`, `critical_symbol_update`) | **D** SYS | 全量观测分层告警；三层架构：连接→数据类型（主）→关键标的白名单 |
| S-002 | 私有数据订阅断联 | `ws_private_connected`, `listen_key_expired`, `private_event_latency_ms`; 连接层+业务层 | **D** SYS | 双层监控（连接层+业务层）；REST回退决定L2/L3严重度 |
| S-003 | 数据陈旧/快照过期 | `public_data_age_ms`, `private_snapshot_age_ms`, `position_snapshot_age_ms` | **D** SYS | 事件驱动型私有流不以age判陈旧；优先对定时刷新的快照任务判定 |
| S-004 | 风控系统整体失明 | `blindness_ratio`（关键源异常比例）= down_count / total_count | **D** SYS | 元监控：消费 S-001/S-002/S-003 输出，组合判定（比例+核心交易所全断+多类型联动） |
| S-005 | 风控只读API频控 | `rate_limit_usage_pct`, `remaining_requests` | **D** SYS | 只监控风控自身只读Key，从REST响应头采集；交易API频控需通过遥测另接 |
| S-006 | 数据真相源不一致 | `position/balance/order_state/price_consistency_diff` | **E** TBD | 拆维度比对（仓位/余额/订单/价格），不建议第一版用综合score；L1局部/L2核心 |
| S-007 | 指标计算延迟过高 | `pipeline_processing_latency_ms`, `metric_calc_latency_ms`, `calc_queue_backlog` | **D** SYS | 全局端到端+局部单指标分级；L1局部/L2全局持续 |
| S-008 | 规则引擎运行异常 | `rule_engine_healthy`, `rule_engine_heartbeat_age_ms`, `rule_eval_error_rate`, `rule_engine_restart_count` | **D** SYS | 多条件结构化判定；L2 |
| S-009 | Watchdog 主链路异常 | `watchdog_healthy`, `pipeline_e2e_latency_ms`, `watchdog_probe_age_ms` | **D** SYS | 独立看门狗端到端探测；L3；恢复需连续3周期 |
| S-010 | 交易所/Symbol 不可交易 | `symbol_trading_enabled`, `exchange_status` | **A** RT | 交易所 exchangeInfo API；L2单symbol/L3交易所级 |
| S-011 | 群组控制失效 | `group_config_consistency`, `group_action_coverage_ratio`, `group_state_writeback_consistency` | **E** TBD | 前置依赖群组控制能力（config+dispatch+writeback）；L2 |
| S-012 | 同源策略批量异常 | `batch_anomaly_count`, `batch_anomaly_ratio`, `same_source_dimension` | **E** TBD | 同源维度：模板/版本/团队/依赖；L1 |
| S-013 | API 契约漂移 | `schema_diff_detected`, `schema_diff_type`, `affected_field_list` | **E** TBD | schema_diff_type: 新增/缺失/类型变化/嵌套/枚举；关键字段缺失优先级更高；L2 |

### 4. M-市场类（9 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| M-001 | 波动率突升 | `price_jump_pct_1h`(MVP), `volatility_1h`(后续) | **B** AGG | MVP先做价格跳变；阈值按交易对类别分层；涨跌不对称需业务理由 |
| M-002 | 交易所接口健康恶化 | `api_latency_ms`, `api_error_rate` | **D** SYS | HTTP client 采集；后续按公共/私有/交易接口分类配置 |
| ~~M-003~~ | ~~波动参与度收敛~~ → 合并至 M-001 | — | — | 联动控制动作，非独立规则 |
| M-004 | 资金费率异常波动 | `funding_rate`, `funding_rate_ma` | **A** RT | 交易所 API 直接返回；**只合约**；MVP按绝对阈值 |
| M-005 | 深度骤降 | `depth_change_pct`, `bid/ask/total_depth` | **B** AGG | 需先定义"深度"口径（±x bps / 前N档 / 可成交金额） |
| M-006 | 点差急剧扩张 | `spread_bps` | **A** RT | 主来源优先用 orderbook；后续可增强为相对基线扩张 |
| M-007 | 跨所价格偏离 | `cross_exchange_price_deviation`, `instrument_mapping_id` | **C** DRV | 只比较同类市场（现货对现货、合约对合约） |
| M-008 | 毒性订单流/逆向选择 | `toxic_flow_indicator` | **E** TBD | 主要对做市/被动参与型策略有意义；第一版可先用朴素代理指标 |
| M-009 | 微观结构恶化 | `microstructure_score` | **E** TBD | ⏸️ 降级为Dashboard视图；前置依赖M-005/M-006/M-008 |

### 5. B-行为类（12 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| B-001 | 异常成交放量 | `volume_usd_1h/24h` | **B** AGG | 绝对上限+相对基线倍数双兜底：`max(expected*multiplier, max_volume)` |
| B-002 | 单边成交比例异常 | `directional_ratio_1h = abs(buy-sell)/max(total, epsilon)` | **B** AGG | 前置 `min_total_volume` 门槛避免小样本误报；按策略类型分层 |
| B-003 | 单笔订单规模异常 | `order_value_usd` | **A** RT | Order创建事件（事后告警），Fill辅助审计不重复触发；与BASE-002事前拦截配合 |
| B-004 | 非授权交易 | `symbol_whitelist_match`, `direction_whitelist_match` | **A** RT | 订单/成交 vs 授权配置白名单，实时可判 |
| ~~B-005~~ | ~~高成交低净仓变化~~ → 合并至 B-008 | — | — | MVP子信号纳入B-008 |
| B-006 | 异常滑点/执行质量劣化 | `single_fill_slippage_bps` + `avg/p95_slippage_bps_window` | **C** DRV | 单笔+窗口双层；参考价口径需按市场类型定义；L1单笔/L2持续劣化 |
| B-007 | 手工单异常 | `order_source`, `source_whitelist_match`, `order_audit_match` | **A** RT | 订单来源治理（来源白名单+审计日志匹配）；L1首次/L2连续高影响 |
| B-008 | 对敲/刷量嫌疑 | `suspicious_pattern_flags`, `pattern_score` | **E** TBD | 模式识别型；排除已授权正常做市/套利场景+min成交额门槛 |
| B-009 | Wash Trade / Self-Trade | `self_trade_count/volume/ratio_window`, `related_account_match_count` | **B** AGG | 两型：同账户确认型（可确认自成交）+ 关联账户疑似型（短时间+相近价格配对）|
| B-010 | 挂撤单比例(OTR)异常 | `otr = (new_order + cancel) / max(fill, 1)` | **B** AGG | 前置 `min_order_count_window` 门槛；按策略类型/交易所分层；L1/L2分级 |
| B-011 | 多账户高相似执行行为 | `time/direction/price/position_path_similarity_score` | **E** TBD | 先限定已知关联账户组范围；多维相似特征不建议一开始黑盒总分 |
| B-012 | 单腿暴露(Leg Risk) | `open_leg_count`, `oldest_open_leg_age_ms`, `unhedged_exposure_usd` | **E** TBD | 强依赖多腿关联模型（hedge_group_id / strategy_order_group_id） |

### 6. C-合规类（4 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| C-001 | 多账户关联执行合规 | `linked_account_group`, `group_behavior_flags`, `compliance_risk_score` | **E** TBD | B-011/B-009的合规升级层；审计型规则；L1线索提示 |
| C-002 | 多账户规避限制风险 | `evasion_target_type`, `group_limit_usage`, `account_split_pattern_flags` | **E** TBD | 治理/审计型；需先定义"规避对象"（OTR/仓位上限/做市资格/API频控） |
| C-003 | 做市资格/激励受损 | `mm_qualification_status`, `mm_score`, `quoted_time_ratio`, `program_otr` | **A** RT | 结果态（资格失效）+前兆态（考核接近红线）；L1临近/L2已失效 |
| C-004 | 账户被限制/降权 | `account_restricted`, `restriction_type_list`, `restriction_error_rate` | **A** RT | 明示限制+能力退化（限制错误码持续出现）；L1退化/L2明确限制 |

### 7. P-权限类（7 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| P-001 | 提币权限异常开启 | `withdraw_enabled` | **A** RT | API Key 权限查询；全账户类型/所有生产API Key |
| P-002 | 非授权交易范围开放 | `permission_scope_match`, 授权范围模型 | **A** RT | API Key 权限 vs 授权范围模型（symbol/市场类型/方向/持仓模式） |
| P-003 | 高危 API Key 未及时停用 | `key_age_days`, `last_rotation_days`, `key_risk_level` | **D** SYS | 全账户类型；高危Key长期存活/未轮转/闲置未停用 |
| P-004 | 多账户权限过度扩散风险 | `permission_spread_count`, `high_risk_permission_holder_count` | **D** SYS | 高风险权限在多个账户/Key过度扩散；周期审计 |
| P-005 | 大额/异常提币实时监控 | `withdraw_amount`, `is_new_withdraw_address`, `daily_withdraw_amount` | **D** SYS | L2大额提币/L3高风险异常模式（新地址+大额+非常用时间+高频） |
| P-006 | 异常资金划转监控 | `transfer_amount`, `transfer_frequency`, `route_whitelist_match` | **C** DRV | 金额+频率+路径三类检测；需合法划转拓扑配置 |
| P-007 | API Key IP 白名单变更监控 | `ip_whitelist_change_detected`, `ip_whitelist_change_type`, `ip_whitelist_size_diff` | **D** SYS | 变更事件分类（新增/删除/全量替换/清空/扩大网段） |

### 8. BASE（5 条）

| 规则 | 名称 | 核心指标 | 类型 | 说明 |
|------|------|---------|------|------|
| BASE-001 | 策略运行时间窗口检查 | `current_time` vs `start_time`/`end_time`/`time_zone` | **A** RT | 支持跨日窗口；L2 |
| BASE-002 | 单笔订单规模/支出范围检查 | `order_qty`, `order_value_usd`, `contract_qty` | **A** RT | 数量/名义金额/合约张数三类阈值；事前拦截型；L1单次/L2连续 |
| BASE-003 | 下单价格偏离限制 | `price_deviation = abs(order_price - market_mid_price) / market_mid_price` | **A** RT | 参考价口径按现货/合约配置；B-006的上游预防规则 |
| BASE-004 | 下单间隔与节奏检查 | `tickIntervalMs`, `jitter_range`, `order_create_time / execTime` | **B** AGG | 合法范围=tickInterval±jitter_range；优先监控下单节奏；L1轻度/L2持续超频 |
| BASE-005 | 执行账户/IP 分发异常检查 | `account_concentration_ratio`, `ip_concentration_ratio` | **B** AGG | 窗口内集中度统计；IP维度需executorIp可稳定落库 |

---

## 汇总统计

| 类型 | 数量 | 占比 | MVP 实现难度 |
|------|------|------|-------------|
| **A: 实时可读** (RT) | 19 | 25% | ✅ 最先实现，数据直接可用 |
| **B: 窗口聚合** (AGG) | 16 | 21% | ✅ 需要时间序列存储，但逻辑清晰 |
| **C: 跨源衍生** (DRV) | 9 | 12% | ⚠️ 需要多源数据对齐，架构复杂度中高 |
| **D: 系统自观测** (SYS) | 16 | 21% | ⚠️ 需要先建基础设施（连接管理、埋点、watchdog） |
| **E: 尚未量化** (TBD) | 12 | 16% | ❌ 需要进一步设计指标定义 |
| STR-策略（未分类） | 10 | 13% | ⛔ 暂不分类 |
| **总计** | **77** (+5 overlap) | | |

> 注：部分 STR-策略 规则（STG/ARB/LIQ/MAN/ACCDIS）因 checklist 中无详细指标描述，未纳入分类。

---

## MVP 实现优先级建议

### Phase 1：先把「A: 实时可读」19 条上线
数据直接从交易所 API/WS 获取，不需要额外存储和计算管道。包括：
- L-001/002/003（清算三剑客）
- E-002/007（仓位、余额）
- S-010（Symbol状态）
- M-004/006（funding rate、spread）
- B-003/004/007（单笔检查）
- C-003/004（资格、限制）
- P-001/002（权限检查）
- BASE-001/002/003（前置拦截基础）

### Phase 2：加上「B: 窗口聚合」16 条
需要引入时间序列存储（ClickHouse 已规划），实现滑动窗口聚合。包括：
- E-005/007/008（仓位变动速率、风险预算综合、Funding Rate）
- M-001/005（波动率、深度变化）
- B-001/002/005/009/010（成交量、OTR 等）
- BASE-004/005（节奏、分发）

### Phase 3：「D: 系统自观测」16 条
需要先建设基础设施：WS 连接管理器暴露 metrics、HTTP client 采集延迟/错误率、watchdog 组件等。包括：
- S-001~009（系统类核心）
- M-002（API 健康）
- P-003~005/007（Key 管理）

### Phase 4：「C: 跨源衍生」9 条
需要多账户数据汇聚、跨交易所价格对齐、内外对账等。包括：
- L-004/005（OI占比、抵押物）
- E-001/002/003/004（净Delta、单市场暴露、对冲缺口、总绝对敞口）
- E-009（对账差异）
- M-007（跨所价格偏离）
- B-006（滑点 — 需 orderbook + 成交）

### Phase 5：「E: 尚未量化」12 条 — 后续迭代
需要先研究和设计指标定义，再实现。包括：
- S-006/011/012/013（数据一致性、群组控制、批量异常、API漂移）
- ~~M-003~~(→M-001)/008/~~009~~(→Dashboard)（毒性流；参与度合并至M-001；微观结构降级为Dashboard）
- B-008/011/012（对敲、相似行为、Leg Risk）
- C-001/002（多账户合规）
- P-006（Key 泄露）
