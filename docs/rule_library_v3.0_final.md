# 风控规则库正式版（V3.0 融合实施版）

## 0. 文档说明

### 0.1 文档定位

本文档是 CEX 风控系统的**规则库最终实施版**，在以下文档基础上融合产出：

| 来源文档 | 核心价值 | 本文档继承方式 |
|---------|---------|--------------|
| `risk_control_plan_v2.6.md` | 五层方法论、38 条主规则、仲裁机制 | **全量保留**，作为主规则库底座 |
| `風險控制規則.md` | 交易员原始需求 22 条规则 | **标准化收编**，映射到三级结构 |
| `[3月24日]Risk梳理.md` | P0/P1/P2 优先级、重复项分析、实现归属判断 | **完整继承**，作为优先级基准 |
| `rule_library_master_plan_v2.6_Version1.md` | 三级结构设计、融合结论、字段标准 | **采纳框架**，细化补充 |
| `rule_library_checklist_v2.6_Version1.md` | 规则确认流程 | **保留引用**，不重复 |

### 0.2 设计原则

1. **V2.6 主框架不推翻** — 五层方法论和 38 条主规则全量保留
2. **交易员规则不照搬** — 标准化收编到三级结构中
3. **现有代码实现不丢弃** — P-001、S-002（原 S-013）已实现，保持兼容
4. **3 月 24 日梳理结论优先** — 重复项合并、实现归属判断直接继承
5. **策略引擎依赖规则明确标注** — 不做不切实际的承诺
6. **策略专属规则延后实施** — 三级规则库（ARB/LIQ/MAN/ACCDIS）及完整策略状态机延后至 v1.0+，MVP 聚焦一级+二级规则
7. **收编规则优先开发** — 各类别中交易员原始需求收编的规则排在前面，优先实现

### 0.3 与代码实现的对应关系

| 代码位置 | 当前状态 | 说明 |
|---------|---------|------|
| `services/risk-engine/alert/rules/rule.go` | ✅ 已实现 | Rule 接口 + RuleRegistry |
| `services/risk-engine/alert/rules/p001.go` | ✅ 已实现 | P-001 提币权限异常检测 |
| `services/risk-engine/alert/rules/s013.go` | ✅ 已实现 | S-002 系统失明/数据断流（**需重命名为 s002.go**） |
| `pkg/models/risk.go` | ✅ 已实现 | RiskEvent / RuleConfig 模型 |
| `scripts/init_mysql.sql` | ✅ 10 条种子规则 | P-001,P-002,B-004,B-005,B-006,B-008,S-006,E-001,L-001（**需按新编号更新，原策略心跳移至 STG-001**） |

---

## 1. 三级规则库结构

```
┌─────────────────────────────────────────────────────────────┐
│                  一级：平台统一主规则库                        │
│  覆盖所有账户、策略、交易所                                    │
│  编号前缀：P / B / S / M / C / E / L                        │
│  实现归属：risk_platform                                     │
│  数据来源：Redis 实时指标 + MySQL 配置 + ClickHouse 历史      │
│  排序原则：各类内交易员收编规则（★）排前，优先开发              │
├─────────────────────────────────────────────────────────────┤
│                  二级：执行基础规则库                          │
│  覆盖所有 Bot / 执行器的基础约束                              │
│  编号前缀：BASE                                              │
│  实现归属：risk_platform / shared                            │
│  数据来源：订单流 + 策略配置                                  │
├─────────────────────────────────────────────────────────────┤
│                  三级：策略专属规则库 ⛔ POST-MVP             │
│  覆盖特定策略类型的执行风险                                    │
│  编号前缀：ARB / LIQ / MAN / ACCDIS                         │
│  实现归属：shared / strategy_engine                          │
│  数据来源：策略内部参数 + 平台数据                            │
│  ⚠️ 延后至 v1.0+，与策略引擎同步实施                         │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 一级：平台统一主规则库

> **编号规则**：各类别中 ★ 标记的为交易员原始需求收编规则，排在前面优先开发。

### 2.1 权限类（P-Class）— 7 条

对应识别层"权限状态风险"。检测 API Key 权限是否超出最小必要范围。

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **P-001** ★ | 提币权限异常开启 | P1 | RISK-POS（权限检查） | A | L2 | ✅ 已实现 |
| **P-002** | 非授权交易范围开放 | P2 | A | L1 | 🔲 种子配置 |
| **P-003** | 高危 API Key 未及时停用 | P1 | D | L2 | 🔲 待开发 |
| **P-004** | 多账户权限过度扩散风险 | P2 | D | L1 | 🔲 待开发 |
| **P-005** | 大额/异常提币实时监控 | P0 | D | L3 | 🔲 待开发 |
| **P-006** | 异常资金划转监控 | P1 | E | L2 | 🔲 待开发 |
| **P-007** | API Key IP 白名单变更监控 | P1 | D | L2 | 🔲 新增 |

**P-007 新增说明**：交易员实际使用中存在 IP 白名单被修改但未察觉的场景，属于权限安全基础防线。

---

### 2.2 行为类（B-Class）— 12 条

对应识别层"行为状态风险"。阈值大量依赖策略元数据基线。

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **B-001** ★ | 异常成交放量 | P2 | RISK-VOL-001 | B | L1 | 🔲 待开发 |
| **B-002** ★ | 单边成交比例异常 | P2 | RISK-VOL-003, MAN-A002 | B | L1 | 🔲 待开发 |
| **B-003** ★ | 单笔订单规模异常 | P1 | RISK-ORD-001 | A | L1 | 🔲 待开发 |
| **B-004** | 非授权交易 | P1 | A | L2 | 🔲 种子配置 |
| **B-005** | 高成交低净仓变化 | P2 | B | L1 | 🔲 种子配置 |
| **B-006** | 异常滑点/执行质量劣化 | P2 | C | L1 | 🔲 种子配置 |
| **B-007** | 手工单异常 | P2 | A | L1 | 🔲 待开发 |
| **B-008** | 对敲/刷量嫌疑（基础版） | P2 | E | L1 | 🔲 种子配置 |
| **B-009** | Wash Trade / Self-Trade | P2 | B | L2 | 🔲 待开发 |
| **B-010** | 挂撤单比例（OTR）异常 | P2 | B | L1 | 🔲 待开发 |
| **B-011** | 多账户高相似执行行为 | P2 | E | L1 | 🔲 待开发 |
| **B-012** | 单腿暴露（Leg Risk）监控 | P2 | E | L2 | 🔲 待开发 |

**B-001 优化说明**：继承 RISK-VOL-001，增加 1h/24h 双窗口节奏检测。
- 指标：`volume_usd_1h`, `volume_usd_24h`
- 触发条件：任一项 > 阈值
- 来源：RISK-VOL-001 成交量节奏控制

**B-002 新增说明**：合并 RISK-VOL-003（单边成交比例限制）和 RISK-MAN-A002（操纵策略方向集中度），抽象为平台统一规则。
- 指标：`directional_ratio_1h`（1h 内买/卖成交量比例）
- 范围：账户 + 市场
- 触发条件：`directional_ratio > 阈值`
- 系统动作：L1 告警 + 限制同向交易

**B-003 新增说明**：将 RISK-ORD-001（单笔订单尺寸限制）提升到平台级，作为 fat finger 防护。
- 指标：`order_qty`, `order_value_usd`
- 触发条件：`order_value_usd > 阈值`
- 告警文案参考 Risk 梳理：`【报警】单笔挂单超额 order_value_usd 为1000usdt，超过阈值200usdt`
- 系统动作：L1 告警（不拒单，拒单由 BASE-002 负责）

---

### 2.3 系统与接入类（S-Class）— 13 条

对应识别层"系统状态风险"。

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **S-001** ★ | 公共数据订阅断联/延迟 | P0 | RISK-SYS-001 | D | L1/L2/L3 分级 | 🔲 待开发 |
| **S-002** ★ | 风控系统整体失明风险 | P0 | RISK-SYS-001 | D | L3 | ✅ 已实现 |
| **S-003** ★ | 风控只读 API 频控使用率过高/耗尽 | P1 | RISK-SYS-001 | D | L1/L2 分级 | 🔲 待开发 |
| **S-004** ★ | 私有数据订阅断联/延迟 | P0 | RISK-SYS-001 | D | L2/L3 分级 | 🔲 **新增** |
| **S-005** ★ | 数据陈旧/快照过期检测 | P0 | RISK-SYS-001 | D | L2/L3 分级 | 🔲 **新增** |
| **S-006** | 数据真相源不一致 | P2 | E | L1 | 🔲 种子配置 |
| **S-007** | 指标计算延迟过高 | P2 | D | L1 | 🔲 待开发 |
| **S-008** | 规则引擎运行异常 | P1 | D | L2 | 🔲 待开发 |
| **S-009** | Watchdog 主风控链路异常 | P0 | D | L3 | 🔲 待开发 |
| **S-010** | 交易所/Symbol 临时不可交易 | P1 | A | L2 | 🔲 待开发 |
| **S-011** | 群组控制失效 | P1 | E | L2 | 🔲 待开发 |
| **S-012** | 同源策略批量异常 | P2 | E | L1 | 🔲 待开发 |
| **S-013** | API 契约漂移与静默变更 | P1 | E | L2 | 🔲 待开发 |

**S-001 说明**：公共数据（orderbook / trades / ticker）WebSocket 订阅健康监控。原 RISK-SYS-001 拆分细化。
- **核心设计原则：全量观测，分层告警**——全量 WS 订阅都记录健康状态，但不对所有 symbol 等权逐条报警
- MVP 监控三层：连接层（连接存活/心跳/重连）→ 数据类型层（交易所×数据类型整体健康，本条主层级）→ 关键标的层（核心 symbol 白名单补充监控）
- 监控参数分两类：A.连接健康（connected, heartbeat_gap, reconnect_count, latency）+ B.业务数据鲜活（last_business_update_ts, data_gap_ms, critical_symbol_last_update）
- 解决"连接活着但数据卡住"的静默断流问题——只看连接层会漏报
- 告警分级：L1 数据类型/核心标的预警 → L2 持续断联/延迟/频繁重连 → L3 多类型同时断联全面失明
- 恢复：WS 恢复连接 且 业务数据恢复更新，连续 2 个检测周期确认
- 告警文案：`【报警】{exchange} {data_type} WS 断联 {gap}s，已重连 {count} 次未恢复`

**S-002 说明**：风控系统整体失明风险 — S-001/S-004/S-005 的上层元规则（meta-rule），全局健康聚合判定。
- 定位：不直接监控 WS 连接，而是消费 S-001/S-004/S-005 的告警状态做组合判定
- 组合触发条件（任一满足）：A. 关键源异常比例 >= 50%；B. 核心交易所公共+私有全断；C. 同一交易所 >= N 类数据同时异常
- 指标：`blindness_ratio = critical_source_down_count / critical_source_count`
- 直接 L3（系统失明不设缓冲） → Telegram 语音电话
- 系统约束：进入失明模式后应禁止依赖不可信数据的高级自动动作，MVP 预留失明标志位
- 恢复：关键源恢复 + 数据新鲜度恢复，连续 3 个检测周期稳定确认（比普通规则更严格）
- 告警文案：`【电话报警】风控系统整体失明！{down_count}/{total_count} 关键数据源异常，所有自动判定不可信`

**S-004 新增说明**：私有数据（账户余额 / 仓位 / 订单更新）WebSocket 订阅健康监控。账户适用：现货 + 合约。
- 与 S-001 的区别：私有数据断联 = "半失明/局部失明"，看不到自己的订单、仓位、余额状态，危险等级高于公共数据断联
- **监控分两层**——区分"链路真断"与"只是暂时没有业务事件"：
  - **连接层**（链路存活）：ws_private_connected, ws_private_heartbeat_age_ms, listen_key_expired, listen_key_remaining_ttl_s
  - **业务层**（需前置条件：账户当前有活跃挂单或持仓）：private_event_latency_ms——空闲账户无事件不触发
- **REST 回退与严重度关系**（核心判定前置项）：
  - 无 REST 回退 → 连接层断联即 L3（真正盲飞）
  - 有 REST 回退且正常 → 连接层断联为 L2（降级运行）
  - 有 REST 回退但 REST 也失败 → L3（完全盲飞）
- 粒度：每交易所 × 每账户
- 系统动作：L2 → 标记数据"降级态" + Telegram 高频预警；L3 → 标记"不可信" + 电话告警 + 若具备控制链路则暂停策略，否则提升至人工最高级告警
- 恢复：WS 恢复 + listenKey 正常 + 完成一次状态对齐/补拉 + 连续 3 个检测周期稳定
- 告警文案：L3 `【电话报警】{exchange} 账户 {account_id} 私有WS断联 {duration}s + REST失败，仓位数据不可信`

**S-005 新增说明**：数据陈旧检测 — 即使 WebSocket 连接在线，也可能长时间未收到更新。
- 场景：WS 连接没断但交易所推送卡住（静默故障），或者 REST 轮询数据长时间未刷新
- 指标：
  - `public_data_age_ms = now() - last_orderbook_update_ts`（公共数据年龄）
  - `private_data_age_ms = now() - last_balance_update_ts`（私有数据年龄）
  - `position_snapshot_age_ms = now() - last_position_sync_ts`（仓位快照年龄）
- 触发条件：
  - L2：`public_data_age_ms > 10s`（行情陈旧，报价可能过时）
  - L2：`private_data_age_ms > 30s`（账户数据陈旧，敞口计算不准）
  - L3：`position_snapshot_age_ms > 60s`（仓位快照过期，必须强制刷新）
- 系统动作：L2 标记数据不可信 + 降级；L3 暂停 + 强制 REST 全量刷新
- 告警文案：`【报警】Binance BTC/USDT orderbook 数据已 {age}s 未更新，标记为陈旧`
- 与 S-001/S-004 的区别：S-001/S-004 检测"连接断了没有"，S-005 检测"连接在但数据对不对"

**S-003 说明**：风控只读 API 频控使用率过高/耗尽 — 只监控风控自身只读 Key，不监控交易执行链路。
- 架构约束：风控使用只读 Key（查仓位/余额/订单），交易使用交易 Key，两者隔离、不同频控池
- 从 API 响应头解析 rate limit：`rate_limit_usage_pct`, `remaining_requests`, `reset_time`
- L1 >= 70% 预警 / L2 >= 90% 危险 → 进入配额保命模式（降低非关键轮询，优先保障仓位、余额）
- 后续如需监控交易 API，通过交易引擎遥测上报方式接入，风控不直接接触交易 Key
- 告警文案：`【预警】{exchange} 风控只读API频控使用率 {usage_pct}%（剩余 {remaining} 请求）`

---

### 2.4 市场与微观结构类（M-Class）— 9 条

对应识别层"市场状态风险"。市场不可控，控制动作以 R2/R3 为主。

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **M-001** ★ | 波动率突升 | P2 | RISK-MKT-001 | B | L1 | 🔲 待开发 |
| **M-002** ★ | 交易所接口健康恶化 | P2 | RISK-SYS-001 | D | L1 | 🔲 待开发 |
| **M-003** ★ | 市场异常波动参与度收敛 | P2 | RISK-MKT-001 | E | L1 | 🔲 待开发 |
| **M-004** ★ | 资金费率异常波动 | P2 | RISK-MKT-002 | A | L1 | 🔲 待开发 |
| **M-005** | 深度骤降 | P2 | B | L1 | 🔲 待开发 |
| **M-006** | 点差急剧扩张 | P2 | A | L1 | 🔲 待开发 |
| **M-007** | 跨所价格偏离 | P2 | C | L1 | 🔲 待开发 |
| **M-008** | 毒性订单流/逆向选择恶化 | P2 | E | L1 | 🔲 待开发 |
| **M-009** | 微观结构恶化 | P2 | E | L1 | 🔲 待开发 |

**M-001 优化说明**：继承 RISK-MKT-001，增加 1h 窗口的价格跳变检测。
- 指标：`price_jump_pct_1h`
- 告警文案参考：`{symbol}价格过去一小时涨幅超过20%/跌幅超过10%`

**M-003 新增说明**：RISK-MKT-001 原始规则中"扩大挂单范围、降低参与度"的动作拆为独立规则。当 M-001 触发时，M-003 联动控制参与度。

**M-004 新增说明**：继承 RISK-MKT-002 资金费率异常。
- 指标：`funding_rate`
- 触发条件：`funding_rate > 上限阈值` 或 `< 下限阈值`
- 来源：Risk 梳理中 RISK-MKT-002
- 告警文案参考：`Binance UM FundRate DANGER : 0.05% > 0.03%`

---

### 2.5 平台合规类（C-Class）— 4 条

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **C-001** | 多账户关联执行合规风险 | P2 | E | L1 | 🔲 待开发 |
| **C-002** | 多账户规避平台限制风险 | P2 | E | L1 | 🔲 待开发 |
| **C-003** | 做市资格/激励资格受损 | P2 | A | L1 | 🔲 待开发 |
| **C-004** | 账户被平台限制/降权 | P1 | A | L2 | 🔲 待开发 |

---

### 2.6 敞口与对账类（E-Class）— 9 条

对应识别层"敞口状态风险"。既有实时监控也有周期性对账。

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **E-001** ★ | 净 Delta 过高 | P2 | RISK-POS-004 | C | L1/L2 分级 | 🔲 待开发 |
| **E-002** ★ | 单市场暴露过大 | P1 | RISK-POS-001 | C | L1/L2 分级 | 🔲 待开发 |
| **E-003** ★ | 对冲缺口扩大 | P2 | RISK-ARB-001 | C | L1/L2 分级 | ⏸️ MVP不开发（→BASE层） |
| **E-004** ★ | 总绝对敞口上限 | P1 | RISK-POS-002 | C | L1/L2 分级 | 🔲 待开发 |
| **E-005** ★ | 仓位变动速率异常 | P1 | RISK-POS-003 | B | L1/L2/L3 分级 | 🔲 待开发 |
| **E-006** ★ | 账户可用资金过低 | P2 | RISK-ACC-004 | A | L1/L2 分级 | 🔲 待开发 |
| **E-007** | 风险预算综合监控（水位+速率） | P2 | — | B | L1/L2 分级 | 🔲 待开发 |
| **E-008** | Funding Rate Bleed（资金费率慢性消耗） | P2 | — | B | L1/L2 分级 | 🔲 待开发 |
| **E-009** | 资金周期对账差异 | P2 | — | C | L1（预留L2） | 🔲 待开发 |

**E-001 说明**：继承 RISK-POS-004 净 Delta 方向性敞口。
- 适用范围：现货 + U 本位合约（币本位合约暂不纳入）
- 统一 USD 换算口径：现货 `qty × mark_price`；U 本位合约 `qty × mark_price × multiplier`（qty 带方向）
- 计算层级：账户级 / 账户组级 / 全局级，三个层级独立判定，阈值分别配置
- 条件：`abs(net_delta_usd) >= threshold`
- 告警输出必须包含方向（净多/净空），不能只报绝对值
- 恢复：回落至阈值以下，连续 2 个窗口确认
- 告警文案：`【预警】{scope} 净Delta {abs_delta_usd} USD（净{delta_side}），超阈值 {threshold} USD`

**E-002 说明**：继承 RISK-POS-001，改名为「单市场暴露过大」（原"集中风险"，实际监控绝对暴露非占比）。
- 适用范围：现货 + U 本位合约，单账户 + 单市场（交易对）
- 指标：`position_usd = abs(position_qty) × mark_price`
- 条件：`position_usd >= per_market_limit`
- 阈值按产品类型分开：`spot_per_market_limit` / `futures_per_market_limit`
- MVP 先做单账户 + 单市场；聚合版（同交易所所有账户）后续扩展
- 告警文案：`【预警】{exchange} {account} {symbol} 仓位暴露 {position_usd} USD，超阈值 {threshold} USD（{product_type}）`

**E-003 说明**：继承 RISK-ARB-001 对冲缺口（⏸️ MVP 不开发，降级至 BASE 层）。
- 层级调整：一级 → BASE 层，对冲缺口强依赖策略上下文（对冲对定义、腿配置），不适合平台统一规则
- 前置条件：必须已配置对冲映射关系（hedging_pair_id），未配置的策略不参与计算
- 适用范围：合约为主（支持现货+合约组合对冲）
- 核心指标：`hedge_gap_usd = abs(leg_a_usd - leg_b_usd × expected_ratio)`
- 后续需先建设策略元数据体系和对冲映射配置能力

**E-004 说明**：继承 RISK-POS-002，统一为多层级总绝对敞口上限。
- 适用范围：现货 + U 本位合约
- 核心指标：`total_exposure_usd = Σ abs(position_usd)`（不做多空抵消，与 E-001 互补）
- 聚合维度（基于四级架构：项目→团队→策略→账户）：各层级独立判定，阈值分别配置
- MVP 先做账户级 + 全局级，中间层级随架构配置完善逐步上线
- 告警文案：`【预警】{scope} 总绝对敞口 {exposure_usd} USD，超阈值 {threshold} USD`

**E-005 说明**：继承 RISK-POS-003，检测短时间仓位急剧变化。
- 适用范围：现货 + U 本位合约，单账户 + 单市场
- 指标定义：`position_change_usd = (current_net - previous_net) × mark_price`（净变化，非绝对变化）
- 双窗口：1h + 24h，条件 `abs(change) >= threshold`
- 阈值按 Spot/Futures 分开，hedge mode 可放宽
- 告警输出包含变化方向（增多/减多/增空/减空）
- MVP 做账户级，策略级后续可选，不做更高层级（速率信号聚合后稀释）
- 告警文案：`【预警】{exchange} {account} {symbol} 仓位1h变动 {change_usd} USD（{change_side}），超阈值`

**E-006 说明**：继承 RISK-ACC-004 账户流动资金缓冲监控。
- 适用范围：现货 + U 本位合约，单账户
- 字段口径按账户类型不同：现货取 `free`/`availableBalance`，合约取 `availableBalance`（扣除已占用保证金）
- 条件：`current_available <= min_available`，Spot/Futures 阈值分开配置
- MVP 先用绝对值阈值，后续可扩展相对阈值（占总权益比例）
- 告警文案：`【预警】{exchange} {account} 可用资金 {current_available} USDT，低于警戒线（{account_type}）`

**E-007 说明**：风险预算综合监控 — 双维度监控预算健康度（水位 + 消耗速率）。
- 适用范围：现货 + U 本位合约
- `current_loss` 口径：风险窗口内累计亏损 `abs(Σ min(pnl, 0))`，只计亏损，含已实现+未实现
- 水位维度：`risk_budget_usage = current_loss / risk_budget`，L1 70% / L2 100%（待确认）
- 速率维度：`budget_consumption_rate_pct = delta_loss / risk_budget / hours`（%/h），前置动态指标
- 两个维度独立判定、独立告警、独立恢复
- 恢复：水位 — 使用率回落连续 2 窗口；速率 — 消耗速率回落连续 2 速率窗口
- 告警文案（水位）：`【预警】{scope} 风险预算使用率 {usage_pct}%（{risk_window}窗口累亏 {current_loss} USD / 预算 {budget} USD）`
- 告警文案（速率）：`【预警】{scope} 预算消耗速率 {rate_pct}%/h，照此速率 {eta_hours}h 后将耗尽预算`

**E-008 说明**：Funding Rate Bleed — 监控合约仓位被资金费率持续侵蚀的风险。
- 适用范围：仅合约（U 本位），单账户 + 单市场（交易对）
- 本条关注"已经吃了多少"（累计损失），不覆盖前瞻信号（当前费率方向不利等，后续另建）
- 数据来源：Funding Rate 结算记录（REST API）+ Position REST（仓位大小），REST 为最终裁定
- 统计窗口可配置（rolling 24h / 3d / 7d），建议默认 3d
- 累计 funding 损失：`cumulative_funding_loss = Σ(funding_rate × position_size × mark_price)`（窗口内，只计支出方向）
- 年化换算（主判定指标）：`annualized_funding_rate = (cumulative_funding_loss / position_notional) / window_days × 365`
- 条件：`annualized_funding_rate >= threshold`（年化成本越高越危险）
- L1：年化费率偏高（预警） → Telegram-info 低频预警
- L2：年化费率严重偏高（持仓被快速消耗） → Telegram-critical 高频预警
- 恢复：窗口滑动后年化费率自然下降（历史结算出窗），或仓位关闭/减仓后不再产生新支出，连续 2 个结算周期确认
- 告警文案：`【预警】{exchange} {account} {symbol} Funding年化费率 {annualized_rate}%（{window}窗口累计 {loss} USD），超警戒线 {threshold}%`

**E-009 说明**：资金周期对账差异 — 消费外部账户系统（Dashboard）的对账结果，差异超限时预警。
- 适用范围：现货 + 合约，单账户
- 定位：风控系统不自行做对账，只读取外部对账系统的结果做预警；后续版本可能演进为自主对账
- 数据来源：外部账户系统（Dashboard）对账结果接口（API / MQ / 共享数据库）
- 条件：`abs(reconciliation_diff) >= max_recon_diff`（绝对差值，覆盖正负两个方向）
- 阈值按 账户 + 币种级可配
- L1：差异超限（预警） → Telegram-info 低频预警
- [预留] L2：大额差异 / 连续多周期差异（后续版本扩展）
- 恢复：外部系统下一个对账周期差异回落，连续 2 个对账周期确认
- 告警文案：`【预警】{exchange} {account} 对账差异 {abs_diff} {currency}（交易所 {exchange_balance} vs 内部账 {internal_balance}），超阈值 {threshold} {currency}`

---

### 2.7 清算与生存类（L-Class）— 5 条

优先级最高。在仲裁引擎中永远排第一位。

| 编号 | 名称 | 默认等级 | 来源规则 | 指标 | 告警级别 | 实现状态 |
|------|------|------|------|------|------|------|
| **L-001** ★ | 维持保证金占比过高 | P0 | RISK-ACC-003 | A | L2/L3 分级 | 🔲 种子配置 |
| **L-002** ★ | 爆仓距离过近 | P0 | RISK-ACC-007 | A | L2/L3 分级 | 🔲 待开发 |
| **L-003** ★ | ADL 风险升高 | P1 | RISK-ACC-005 | A | L1/L2 分级 | 🔲 待开发 |
| **L-004** ★ | 单边 OI 占比过高 | P2 | RISK-ACC-006 | C | L1 | 🔲 待开发 |
| **L-005** | 联合保证金/抵押物折价双杀 | P1 | — | C | L1/L2 分级 | ⏸️ MVP不开发（条件待量化） |

**L-001 优化说明**：继承 RISK-ACC-003，改名为「维持保证金占比过高」（原名"过低"易误解）。
- 指标：Binance `maintMarginRatio`，值越高说明距强平越近
- 主判定源：账户 REST 接口，最终裁定以 REST 为准
- 适用范围：仅合约（Futures），Spot 无保证金机制不适用
- L2（ALERT）：`maint_margin_rate >= threshold_1`（初始建议 5%，账户级可配），连续 2 窗口命中
- L3（CRITICAL）：`maint_margin_rate >= threshold_2`（初始建议 8%，账户级可配），数据 Fresh 即时触发，否则二次 REST 确认
- 告警文案参考：`【严重报警】{exchange} {account} 维持保证金占比 {value}% 超阈值 {threshold}%，距强平风险较近`

**L-002 说明**：继承 RISK-ACC-007 爆仓价距离。
- 指标：`liq_price`, `mark_price`
- 触发条件：`abs(liq_price - mark_price) / mark_price < threshold`
- L2：距离 < threshold_1（如 5%）
- L3：距离 < threshold_2（如 2%）→ 暂停 + 电话告警
- 告警文案：`【电话报警】爆仓价极度接近 liq_price与mark_price距离{distance_pct}%`

**L-003 说明**：继承 RISK-ACC-005 ADL 风险。
- 指标：`adl` 等级（1-5）
- L1：`adl > threshold_1`（如 3）
- L2：`adl > threshold_2`（如 4）→ 降级/暂停
- 告警文案：`【报警】ADL 等级超阈值，存在自动减仓风险`

**L-004 说明**：继承 RISK-ACC-006 单边 OI 占比。
- 适用范围：仅合约（Futures），单市场（交易对）+ 按持仓模式分支
- 核心逻辑：分子取值依赖账户持仓模式（`position_mode`）
  - 单向持仓：`ratio = abs(net_position) / oi`，按净持仓方向告警
  - 双向持仓：`long_ratio = long_position / oi`，`short_ratio = short_position / oi`，任一方向超阈值即触发
- 主判定源：Position REST API + OI 公共 API
- 告警文案：`【预警】{exchange} {account} {symbol} {side}头持仓占市场OI {ratio_pct}%，超阈值 {threshold}%`
- 备注：依赖交易所提供 OI 数据；双向模式下多空可能同时超限，分别告警

**L-005 说明**：联合保证金/抵押物折价双杀（⏸️ MVP 不开发，条件待量化）。
- 适用范围：仅合约（联合保证金 / Portfolio Margin 模式），仅对存在非稳定币抵押物的账户生效
- 核心判定比率：`collateral_margin_ratio = effective_collateral_value / total_margin_required`
  - `effective_collateral_value = Σ(qty × price × (1 - haircut_rate))`
- 双杀判定方向：抵押物价格下跌 + 折扣率恶化 → 有效保证金急剧缩水
- 条件方向：`<=`（比率越低越危险）
- ⚠️ 条件未完成量化：静态安全线和双杀速率阈值均待交易员确认后方可排期
- 后续路径：先用交易所 `uniMMR` 快速上线（路径A），再叠加分项计算（路径B）
- 告警文案：`【严重预警】{exchange} {account} 联合保证金有效抵押率 {ratio}x 跌破安全线`

---

## 3. 二级：执行基础规则库（BASE）

覆盖所有 Bot / 执行器都应遵守的基础约束。将策略侧 guardrail 标准化，避免规则散落在策略代码中。全部为交易员收编规则 ★。

| 编号 | 名称 | 优先级 | 来源规则 | 实现归属 | 指标 | 告警级别 |
|------|------|------|------|------|------|------|
| **BASE-001** ★ | 策略运行时间窗口检查 | P2 | RISK-BASE-001 | risk_platform | A | L1 |
| **BASE-002** ★ | 单笔订单规模/支出范围检查 | P0 | RISK-ORD-001 + RISK-BASE-002 | risk_platform | A | L2 |
| **BASE-003** ★ | 下单价格偏离限制 | P0 | RISK-ORD-002 | risk_platform | A | L1 |
| **BASE-004** ★ | 下单间隔与节奏检查 | P2 | RISK-BASE-003 | shared | B | L1 |
| **BASE-005** ★ | 执行账户/IP 分发异常检查 | P2 | RISK-BASE-004 | shared | B | L1 |

### BASE-001 策略运行时间窗口检查
- 指标类型：**A** (RT 实时可读)
- 目的：检查 Bot 是否在 Config 设定的时间范围内运行
- 指标：`current_time`, `start_time`, `end_time`
- 规则条件：`current_time` 不在 `[start_time, end_time]` 内
- 系统动作：PAUSED
- 备注：时间窗口配置在策略表中

### BASE-002 单笔订单规模/支出范围检查
- 指标类型：**A** (RT 实时可读)
- 目的：防 fat finger、异常大单（合并 RISK-ORD-001 + RISK-BASE-002）
- 指标：`order_qty`, `order_value_usd`
- 规则条件：`order_qty > $MaxOrderQty` 或 `order_value_usd > $MaxOrderValue`
- 系统动作：**拒绝订单**；持续触发则降级
- 与 B-003 的区别：B-003 是事后告警，BASE-002 是事前拦截（前置守卫）
- 阈值以 Base token 为单位，由策略配置

### BASE-003 下单价格偏离限制
- 指标类型：**A** (RT 实时可读)
- 目的：防错价、过度激进挂单
- 指标：`price_deviation = abs(order_price - market_mid_price) / market_mid_price`
- 规则条件：`price_deviation > $MaxPriceDeviation`
- 系统动作：拒绝订单 + 告警
- 告警文案参考：`Warning: Account ID: 1 UAI Price Deviation DANGER : 50% > 20%`

### BASE-004 下单间隔与节奏检查
- 指标类型：**B** (AGG 窗口聚合)
- 目的：控制下单间隔，降低被交易所识别为高频的风险
- 指标：相邻两笔 `execTime` 间隔
- 规则条件：`trades[n].execTime - trades[n-1].execTime < tick`（tick = tickIntervalMs × randomDelayJitter）
- 系统动作：降一级
- 实现归属：shared — 需策略引擎提供 tick 参数

### BASE-005 执行账户/IP 分发异常检查
- 指标类型：**B** (AGG 窗口聚合)
- 目的：避免间隔内挂单集中在同一账户/IP，被交易所识别
- 指标：X 间隔内 `trades.executorIp` 或 `account_id` 分布
- 规则条件：同一 IP/账户挂单过多，未满足分散要求
- 系统动作：L1 告警
- 实现归属：shared — 需等 IP 方案确定
- 告警文案参考：`Warning: ACCOUNT id: 15 UAI open order concentration: 70% > 60%`

---

## 4. 三级：策略专属规则库 ⛔ POST-MVP（v1.0+ 实施）

> **重要说明**：三级策略专属规则库整体延后至 MVP 之后的版本实施。
> 原因：策略专属规则强依赖策略引擎内部参数和执行上下文，在 MVP 阶段策略引擎尚未定型，
> 过早实现会导致频繁返工。MVP 阶段优先保证一级平台规则和二级 BASE 规则的完整覆盖。
>
> 相关的**策略状态机**（NORMAL→DEGRADED→RESTRICTED→PAUSED）同样延后至 v1.0+ 实施，
> MVP 阶段使用简化的二态模型（active / paused）。

### 4.0 策略通用规则（STG）

> 适用于所有策略类型的通用风控规则，与策略引擎同步实施。

| 编号 | 名称 | 优先级 | 来源规则 | 实现归属 | 指标 | 告警级别 |
|------|------|------|------|------|------|------|
| **STG-001** | 策略心跳超时 | P1 | strategy_engine / shared | — | L2 |

**STG-001**：
- 目的：检测策略进程/Bot 是否正常运行
- 指标：`last_heartbeat_ts`（策略上报的最近一次心跳时间）
- 触发条件：`now() - last_heartbeat_ts > heartbeat_timeout`（如 30s）
- 系统动作：L2 告警 + 暂停该策略
- 告警文案：`【严重报警】策略 {strategy_id} 心跳超时 {duration}s，已暂停`
- 实现归属：strategy_engine 上报心跳，risk_platform 检测超时
- 备注：心跳间隔和超时阈值由策略配置决定，不同策略类型可不同

---

### 4.1 套利策略（ARB）

| 编号 | 名称 | 优先级 | 来源规则 | 实现归属 |
|------|------|-------|------|------|---------|--------|
| **ARB-001** ★ | 套利未对冲敞口风险 | P2 | RISK-ARB-001 | — | shared |
| **ARB-002** ★ | 套利价差净收益检查 | P2 | RISK-CroArb-001 | — | strategy_engine |

**ARB-001**：
- 指标：`unhedged_exposure_usd`（跨交易所套利单边成交的未对冲金额）
- 触发条件：`unhedged_exposure_usd > $MaxUnhedgedExposure`
- 系统动作：限制套利策略 + 降级
- 实现归属：shared（平台监控，执行限制需策略侧配合）

**ARB-002**：
- 指标：`buyPrice = Ask × (1+feeRateBps)`, `sellPrice = bid × (1-feeRateBps)`
- 触发条件：`(sellPrice - buyPrice) / midPrice < minSpreadBps`
- 系统动作：DEGRADED
- 实现归属：strategy_engine（更像执行前置判断）

### 4.2 流动性/网格策略（LIQ）

| 编号 | 名称 | 优先级 | 来源规则 | 实现归属 |
|------|------|-------|------|------|---------|--------|
| **LIQ-001** ★ | 挂单跨越中价检查 | P0 | RISK-LIQ-P001 | — | strategy_engine / shared |
| **LIQ-002** ★ | 网格结构偏离检查 | P1 | RISK-LIQ-P002 | — | strategy_engine / shared |
| **LIQ-003** ★ | 每 tick 最大资本投放控制 | P1 | RISK-LIQ-P003 | — | strategy_engine / shared |

**LIQ-001**：
- 指标：`order.execPrice`, `midPrice`, `order.side`
- 触发条件：买单 `execPrice > midPrice` 或 卖单 `execPrice < midPrice`
- 系统动作：降一级
- 备注：3 月 24 日梳理结论 — "做不了，交易引擎直接下单"，风控只能做事后检测

**LIQ-002**：
- 参数：`maxGridLevels`, `minSpreadFromMidPct`, `orderPriceTolerancePct`, `midSpreadBetweenGridsPct`
- 触发条件：任一项超配置
- 备注：需要网格订单簿数据，依赖交易引擎配合

**LIQ-003**：
- 指标：`∑ limitOrderPlaced.size`, `maxCapitalAllocationPct`, `currentInventory.baseTotal`
- 触发条件：`∑size > maxCapitalAllocationPct × baseTotal`
- 告警文案参考：`Alert: UAI TICK SIZE : 12% of inventory > 10% cap`
- 备注：依赖每轮挂单上下文

### 4.3 市值管理/操纵执行策略（MAN）

| 编号 | 名称 | 优先级 | 来源规则 | 实现归属 |
|------|------|-------|------|------|---------|--------|
| **MAN-001** ★ | 理想价格偏离控制 | P1 | RISK-MAN-A001 | — | shared |
| **MAN-002** ★ | 操纵策略方向集中度控制 | P1 | RISK-MAN-A002 | — | shared |

**MAN-001**：
- 指标：`exec_price`, `ideal_price`（TWAP 算法得出）
- 触发条件：拉升时 `exec_price > ideal_price`，下拉时 `exec_price < ideal_price`
- 系统动作：降一级
- 告警文案参考：`Account ID: 1 PxDev DANGER : 0.5% vs ideal (pump)`
- 实现归属：shared — 需引擎提供 ideal_price

**MAN-002**：
- 指标：X 间隔内 `∑trades.side["buy"] / ∑trades.side["sell"]`
- 触发条件：比例 > `reverseTradeProbability`
- 系统动作：降一级
- 与 B-002 的区别：B-002 是平台级通用规则，MAN-002 是操纵策略专属

### 4.4 吸筹/出货策略（ACCDIS）

| 编号 | 名称 | 优先级 | 来源规则 | 实现归属 |
|------|------|-------|------|------|---------|--------|
| **ACCDIS-001** ★ | 周期累计成交总额/总量上限 | P0 | RISK-ACC/DIS-001 | — | risk_platform |
| **ACCDIS-002** ★ | 策略价格带偏离检查 | P1 | RISK-ACC/DIS-002 | — | shared |

**ACCDIS-001**：
- 指标：`totalQuantity`, `totalTradingAmount`
- 触发条件：`totalQuantity > totalTradingAmount`（计划总量）
- 系统动作：PAUSED
- 告警文案参考：`Strategy pump PAUSED : UAI ACCUMULATION traded 110% > 100% plan`

**ACCDIS-002**：
- 指标：`exec_price`, `priceLimit`
- 触发条件：`exec_price > priceLimit`
- 系统动作：DEGRADED

---

## 5. 规则总数统计与来源映射

### 5.1 总数统计

| 层级 | 类别 | 数量 | 其中★收编 | V2.6 原有 | 新增/优化 | MVP 范围 |
|------|------|------|----------|----------|----------|---------|
| 一级 | P-Class | 7 | 1 | 6 | +1 | ✅ |
| 一级 | B-Class | 12 | 3 | 8 | +4 | ✅ |
| 一级 | S-Class | 13 | 5 | 8 | +5 | ✅ |
| 一级 | M-Class | 9 | 4 | 5 | +4 | ✅ |
| 一级 | C-Class | 4 | 0 | 4 | 0 | ✅ |
| 一级 | E-Class | 12 | 7 | 8 | +4 | ✅ |
| 一级 | L-Class | 5 | 4 | 3 | +2 | ✅ |
| **一级小计** | — | **62** | **24** | **42** | **+20** | **✅ MVP** |
| 二级 | BASE | 5 | 5 | 0 | +5 | ✅ |
| **二级小计** | — | **5** | **5** | **0** | **+5** | **✅ MVP** |
| 三级 | STG（通用） | 1 | 0 | 0 | +1 | ⛔ POST-MVP |
| 三级 | ARB | 2 | 2 | 0 | +2 | ⛔ POST-MVP |
| 三级 | LIQ | 3 | 3 | 0 | +3 | ⛔ POST-MVP |
| 三级 | MAN | 2 | 2 | 0 | +2 | ⛔ POST-MVP |
| 三级 | ACCDIS | 2 | 2 | 0 | +2 | ⛔ POST-MVP |
| **三级小计** | — | **10** | **9** | **0** | **+10** | **⛔ POST-MVP** |
| **总计** | — | **77** | **38** | **42** | **+35** | **67 MVP + 10 POST-MVP** |

### 5.2 交易员原始规则映射表（29 条 → 全部收编）

| 原始规则 | 收编去向（新编号） | 收编方式 |
|---------|-------------------|---------|
| RISK-POS-001 | **E-002** 单市场仓位集中风险 | 新增 |
| RISK-POS-002 | **E-004** 总绝对敞口上限 | 新增 |
| RISK-POS-003 | **E-005** 仓位变动速率异常 | 新增 |
| RISK-POS-004 | **E-001** 净 Delta 过高 | 已有，优化 |
| RISK-VOL-001 | **B-001** 异常成交放量 | 已有，优化 |
| RISK-VOL-003 | **B-002** 单边成交比例异常 | 新增 |
| RISK-ORD-001 | **B-003** + **BASE-002** | 新增 + 新增（合并 BASE-002） |
| RISK-ORD-002 | **B-006** + **BASE-003** | 已有 + 新增 |
| RISK-ARB-001 | **ARB-001** | 新增（POST-MVP） |
| RISK-MKT-001 | **M-001** + **M-003** | 已有 + 新增 |
| RISK-MKT-002 | **M-004** 资金费率异常 | 新增 |
| RISK-SYS-001 | **S-001** + **S-002** + **S-003** | 新增 + 已实现 + 新增 |
| RISK-BASE-001 | **BASE-001** | 新增 |
| RISK-BASE-002 | **BASE-002**（合并 ORD-001） | 新增 |
| RISK-BASE-003 | **BASE-004** | 新增 |
| RISK-BASE-004 | **BASE-005** | 新增 |
| RISK-MAN-A001 | **MAN-001** | 新增（POST-MVP） |
| RISK-MAN-A002 | **MAN-002** + **B-002** | 新增（POST-MVP） + 新增 |
| RISK-LIQ-P001 | **LIQ-001** | 新增（POST-MVP） |
| RISK-LIQ-P002 | **LIQ-002** | 新增（POST-MVP） |
| RISK-LIQ-P003 | **LIQ-003** | 新增（POST-MVP） |
| RISK-ACC/DIS-001 | **ACCDIS-001** | 新增（POST-MVP） |
| RISK-ACC/DIS-002 | **ACCDIS-002** | 新增（POST-MVP） |
| RISK-CroArb-001 | **ARB-002** | 新增（POST-MVP） |
| RISK-ACC-003 | **L-001** 维持保证金占比过高 | 已有，优化 |
| RISK-ACC-004 | **E-006** 账户可用资金过低 | 新增 |
| RISK-ACC-005 | **L-003** ADL 风险 | 新增 |
| RISK-ACC-006 | **L-004** 单边 OI 占比 | 新增 |
| RISK-ACC-007 | **L-002** 爆仓距离过近 | 新增 |

**结论：交易员原始 22 条规则 + Risk 梳理补充的 7 条 = 29 条全部收编，无一遗漏。**

### 5.3 新旧编号映射表（代码迁移用）

> 本次重新编号后，以下旧编号需要在代码、配置、种子数据中更新。

| 旧编号 | 新编号 | 名称 | 变更说明 |
|-------|-------|------|---------|
| P-004 | **P-003** | 高危 API Key 未及时停用 | 填补间隔 |
| P-005 | **P-004** | 多账户权限过度扩散风险 | 填补间隔 |
| P-006 | **P-005** | 大额/异常提币实时监控 | 填补间隔 |
| P-007 | **P-006** | 异常资金划转监控 | 填补间隔 |
| P-008 | **P-007** | API Key IP 白名单变更监控 | 填补间隔 |
| B-002 | **B-001** | 异常成交放量 | ★收编提前 |
| B-014 | **B-002** | 单边成交比例异常 | ★收编提前 |
| B-015 | **B-003** | 单笔订单规模异常 | ★收编提前 |
| B-001 | **B-004** | 非授权交易 | 收编后顺延 |
| B-003 | **B-005** | 高成交低净仓变化 | 收编后顺延 |
| B-004 | **B-006** | 异常滑点/执行质量劣化 | 收编后顺延 |
| B-005 | **B-007** | 手工单异常 | 收编后顺延 |
| B-007 | **B-008** | 对敲/刷量嫌疑 | 收编后顺延 |
| B-010 | **B-009** | Wash Trade / Self-Trade | 收编后顺延 |
| B-011 | **B-010** | 挂撤单比例异常 | 收编后顺延 |
| B-012 | **B-011** | 多账户高相似执行行为 | 收编后顺延 |
| B-013 | **B-012** | 单腿暴露监控 | 收编后顺延 |
| S-008 | **S-001** | 公共数据订阅断联/延迟 | ★收编拆分细化（原"数据接入链路中断"） |
| S-013 | **S-002** | 风控系统整体失明风险 | ★收编提前（**需改代码 s013.go → s002.go**） |
| S-016 | **S-003** | 风控只读API频控使用率过高/耗尽 | ★收编提前（范围收缩为只读Key） |
| (新增) | **S-004** | 私有数据订阅断联/延迟 | ★**新增**（从 S-001 拆分，私有数据独立监控） |
| (新增) | **S-005** | 数据陈旧/快照过期检测 | ★**新增**（连接在但数据不更新的静默故障） |
| S-001 | **STG-001** | 策略心跳超时 | 移至三级策略通用规则（POST-MVP） |
| S-007 | **S-006** | 数据真相源不一致 | 收编后顺延 |
| S-009 | **S-007** | 指标计算延迟过高 | 收编后顺延 |
| S-010 | **S-008** | 规则引擎运行异常 | 收编后顺延 |
| S-014 | **S-009** | Watchdog 主风控链路异常 | 收编后顺延 |
| S-017 | **S-010** | 交易所/Symbol 临时不可交易 | 收编后顺延 |
| S-018 | **S-011** | 群组控制失效 | 收编后顺延 |
| S-019 | **S-012** | 同源策略批量异常 | 收编后顺延 |
| S-020 | **S-013** | API 契约漂移与静默变更 | 收编后顺延 |
| M-005 | **M-002** | 交易所接口健康恶化 | ★收编提前 |
| M-009 | **M-003** | 市场异常波动参与度收敛 | ★收编提前 |
| M-010 | **M-004** | 资金费率异常波动 | ★收编提前 |
| M-002 | **M-005** | 深度骤降 | 收编后顺延 |
| M-003 | **M-006** | 点差急剧扩张 | 收编后顺延 |
| M-004 | **M-007** | 跨所价格偏离 | 收编后顺延 |
| M-007 | **M-008** | 毒性订单流/逆向选择恶化 | 收编后顺延 |
| M-008 | **M-009** | 微观结构恶化 | 收编后顺延 |
| E-002A | **E-002** | 单市场仓位集中风险 | ★收编 + 去掉 A 后缀 |
| E-005A | **E-004** | 总绝对敞口上限 | ★收编提前 + 去掉 A 后缀 |
| E-016 | **E-005** | 仓位变动速率异常 | ★收编提前 |
| E-017 | **E-006** | 账户可用资金过低 | ★收编提前 |
| E-005+E-006 | **E-007** | 风险预算综合监控 | 收编后合并（水位+速率） |
| E-014 | **E-009** | 资金周期对账差异 | 收编后顺延 |
| L-004 | **L-003** | ADL 风险升高 | ★收编提前 |
| L-005 | **L-002** | 爆仓距离过近 | ★收编提前 |
| L-007 | **L-004** | 单边 OI 占比过高 | ★收编提前 |
| L-006 | **L-005** | 联合保证金/抵押物折价双杀 | 收编后顺延 |

**需更新的代码文件**：
- `services/risk-engine/alert/rules/s013.go` → 重命名为 `s002.go`，内部 `Code()` 返回 `"S-002"`
- `scripts/init_mysql.sql` → 种子规则编号全部按新编号更新

---

## 6. 实施优先级排期

### 6.1 P0 — 最小闭环必须（v0.1.0-poc ~ v0.2.0-mvp）

**已实现：**
- P-001 提币权限异常（✅ 代码 + 测试）
- S-002 风控系统整体失明（✅ 代码 + 测试，原 S-013）

**立即实现（MVP 阶段）：**

| 规则 | 说明 | 数据依赖 | 实现复杂度 |
|------|------|---------|-----------|
| L-001 ★ | 维持保证金占比过高 | REST API maintMarginRatio | 中 |
| L-002 ★ | 爆仓距离过近 | REST API liq_price + mark_price | 中 |
| E-001 ★ | 净 Delta 过高 | Redis position 数据 | 低 |
| E-002 ★ | 单市场仓位集中 | Redis position 数据 | 低 |
| E-004 ★ | 全账户总敞口 | Redis position 聚合 | 低 |
| E-005 ★ | 仓位变动速率 | Redis position 差值 | 中 |
| B-003 ★ | 单笔订单规模异常 | Kafka trade_events | 低 |
| BASE-002 ★ | 单笔订单拦截 | 订单流前置 | 中 |
| BASE-003 ★ | 价格偏离拦截 | 订单流 + 行情 | 中 |
| S-003 ★ | 风控只读API频控 | REST response headers | 中 |
| S-004 ★ | 私有数据订阅断联 | WebSocket 私有连接状态 | 中 |
| S-005 ★ | 数据陈旧/快照过期 | last_update 时间戳 | 低 |

> 注：P0 阶段 14 条规则中 **全部为★收编规则**，体现交易员需求优先。

### 6.2 P1 — MVP 增强（v0.2.0 ~ v1.0.0）

- B-001, B-002, B-004, B-005, B-006, B-008
- S-006, S-008, S-010
- M-001, M-003, M-004
- E-006, E-007, E-008
- L-003, L-004
- BASE-001, BASE-004

### 6.3 P2 — 二期增强（v1.0.0 ~ v2.0.0）

- P-003, P-004, P-007, B-007, B-009, B-010, B-011, B-012
- C-001 ~ C-004, S-007, S-011, S-012, S-013
- M-002, M-005, M-006, M-007, M-008, M-009
- E-003, E-009
- L-005, BASE-005

### 6.4 策略专属规则 — POST-MVP（v1.0.0+ 与策略引擎同步实施）

> 以下规则依赖策略引擎内部参数，需等策略引擎架构定型后再实施。
> 同时实施完整四态策略状态机（NORMAL→DEGRADED→RESTRICTED→PAUSED）。

- **策略通用**：STG-001（策略心跳超时）
- **套利策略**：ARB-001, ARB-002
- **流动性/网格策略**：LIQ-001, LIQ-002, LIQ-003
- **市值管理策略**：MAN-001, MAN-002
- **吸筹/出货策略**：ACCDIS-001, ACCDIS-002

### 6.5 P3 — 研究/画像/学习类

- 图谱化串通分析
- 学习型阈值自动校准
- 高级微观结构风险评分
- Partner 风险画像

---

## 7. 告警级别与通知渠道对照

### 7.1 MVP 阶段（唯一通道：Telegram）

| 级别 | 通道 | 说明 | 系统动作（MVP） |
|------|------|------|----------------|
| **L1** | Telegram-info | 低频预警，常规风险提示 | 仅通知 |
| **L2** | Telegram-critical | 高频预警，需重点关注 | 仅通知（自动化动作后续迭代） |
| **L3** | Telegram 语音电话 | 最高级告警，通过 Telegram 语音通话 | 仅通知（自动化动作后续迭代） |

> MVP 阶段所有规则触发后均为 Telegram 通知，不执行自动化干预动作（暂停/降级/限制/拒单）。
> 共 17 条规则包含 [后续] 自动化动作，详见 checklist。

### 7.2 后续迭代（增加自动化动作）

| 级别 | 通道 | 系统动作（后续） |
|------|------|----------------|
| **L1** | Telegram-info | 仅告警，策略状态不变 |
| **L2** | Telegram-critical | 告警 + 降级/暂停 |
| **L3** | Telegram 语音电话 | 告警 + 降级/暂停 + 语音电话 |

告警文案格式：
- L1：`【告警】【Strategy {id}】【{RISK_CODE}】{内容}，请关注`
- L2：`【严重报警】【Strategy {id}】【{RISK_CODE}】{内容}，请尽快处理`
- L3：`【紧急告警】【Strategy {id}】【{RISK_CODE}】{内容}，请立即处理`

---

## 7A. 指标类型分类（按数据获取方式）

### 7A.1 分类定义

| 类型 | 简称 | 含义 | 数量 | MVP 实现难度 |
|------|------|------|------|-------------|
| **A** | RT 实时可读 | 交易所 API/WS 直接返回，一次调用即得 | 19 | ⭐ 低 |
| **B** | AGG 窗口聚合 | 需收集时间序列数据，做窗口聚合/累加计算 | 16 | ⭐⭐ 中 |
| **C** | DRV 跨源衍生 | 需组合多个数据源（跨账户/跨交易所/跨系统）计算 | 9 | ⭐⭐⭐ 中高 |
| **D** | SYS 系统自观测 | 我方基础设施产生的指标，非来自交易所 | 16 | ⭐⭐ 中（需先建设） |
| **E** | TBD 尚未量化 | 概念清晰但缺乏明确可计算的指标定义，需进一步设计 | 12 | ⭐⭐⭐⭐ 高 |

> 10 条 POST-MVP 规则暂未分类。

### 7A.2 逐规则分类总览

#### L-清算（5 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| L-001 | 维持保证金占比过高 | **A** | maintMarginRatio | 交易所 REST API 直接返回，值越高风险越大 |
| L-002 | 爆仓距离过近 | **A** | liq_price, mark_price | API 实时字段，简单除法 |
| L-003 | ADL 风险升高 | **A** | adl (1-5) | Position API 直接返回 |
| L-004 | 单边 OI 占比过高 | **C** | position / oi | 持仓÷市场OI，跨源 |
| L-005 | 联合保证金/抵押物折价 | **C** | collateral_value, haircut | 多源组合计算 |

#### E-敞口与对账（9 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| E-001 | 净 Delta 过高 | **C** | net_delta_usd | 跨账户+跨品种汇总 |
| E-002 | 单市场暴露过大 | **C** | position_usd | Position API + Mark Price 组合，Spot/Futures 阈值分开 |
| E-003 | 对冲缺口扩大 | **C** | hedge_gap_usd | 多账户+对冲映射 |
| E-004 | 总绝对敞口上限 | **C** | total_exposure_usd | 多层级聚合（账户/策略/团队/项目/全局） |
| E-005 | 仓位变动速率异常 | **B** | position_change_1h/24h | 仓位快照时间序列；Spot/Futures 分阈值，hedge mode 可放宽 |
| E-006 | 账户可用资金过低 | **A** | available_balance | API 直接返回；Spot/Futures 字段口径不同 |
| E-007 | 风险预算综合监控 | **B** | risk_budget_usage（水位）+ consumption_rate_pct（速率） | 水位：累亏/预算；速率：亏损增速/预算（%/h），双维度独立判定 |
| E-008 | Funding Rate Bleed | **B** | annualized_funding_rate（主）, cumulative_funding_loss | 窗口内 funding 结算累计 → 年化换算，建议默认 3d 窗口 |
| E-009 | 资金周期对账差异 | **C** | abs(reconciliation_diff) | 消费外部对账系统结果，绝对差值判定 |

#### S-系统（13 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| S-001 | 公共数据断联/延迟 | **D** | 连接健康(connected, heartbeat_gap, reconnect) + 业务鲜活(data_gap_ms, critical_symbol_update) | 全量观测分层告警；三层：连接→数据类型→关键标的 |
| S-002 | 整体失明风险 | **D** | blindness_ratio（关键源异常比例） | 元监控：消费 S-001/S-004/S-005 输出做组合判定 |
| S-003 | 风控只读API频控 | **D** | rate_limit_usage_pct, remaining_requests | 只监控风控自身只读Key，从REST响应头采集 |
| S-004 | 私有数据断联 | **D** | ws_private_connected | 私有 WS 管理器 |
| S-005 | 数据陈旧/过期 | **D** | data_age_ms | 各模块更新时间 |
| S-006 | 数据真相源不一致 | **E** | consistency_score | 比对逻辑待定义 |
| S-007 | 指标计算延迟 | **D** | calc_latency_ms | 管道延迟埋点 |
| S-008 | 规则引擎异常 | **D** | engine_healthy | 引擎自检/心跳 |
| S-009 | Watchdog 链路异常 | **D** | watchdog_healthy | watchdog 组件 |
| S-010 | Symbol 不可交易 | **A** | trading_enabled | exchangeInfo API |
| S-011 | 群组控制失效 | **E** | group_control | 逻辑待实现 |
| S-012 | 同源策略批量异常 | **E** | batch_anomaly | 定义待明确 |
| S-013 | API 契约漂移 | **E** | schema_diff | 检测方式待设计 |

#### M-市场（9 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| M-001 | 波动率突升 | **B** | price_jump_1h | 价格时间序列窗口 |
| M-002 | 接口健康恶化 | **D** | api_latency, error_rate | HTTP client 采集 |
| M-003 | 波动参与度收敛 | **E** | participation_rate | 概念待定义 |
| M-004 | 资金费率异常 | **A** | funding_rate | API 直接返回 |
| M-005 | 深度骤降 | **B** | depth_change_pct | orderbook 快照序列 |
| M-006 | 点差急剧扩张 | **A** | spread_bps | 实时 orderbook |
| M-007 | 跨所价格偏离 | **C** | cross_exchange_dev | 多交易所报价比对 |
| M-008 | 毒性订单流 | **E** | toxic_flow | 学术指标待研究 |
| M-009 | 微观结构恶化 | **E** | microstructure_score | 综合评分待定义 |

#### B-行为（12 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| B-001 | 异常成交放量 | **B** | volume_1h/24h | 窗口聚合 |
| B-002 | 单边成交比例 | **B** | directional_ratio | 方向累加取比 |
| B-003 | 单笔订单规模 | **A** | order_value_usd | 实时检查 |
| B-004 | 非授权交易 | **A** | 白名单比对 | 实时可判 |
| B-005 | 高成交低净仓 | **B** | vol_vs_pos_change | 窗口对比 |
| B-006 | 异常滑点 | **C** | slippage_bps | orderbook+成交 |
| B-007 | 手工单异常 | **A** | order_source | 实时可判 |
| B-008 | 对敲/刷量嫌疑 | **E** | wash_trade_score | 模式待定义 |
| B-009 | Self-Trade | **B** | self_trade_detected | fills 配对分析 |
| B-010 | OTR 异常 | **B** | order_to_trade_ratio | 窗口聚合 |
| B-011 | 多账户相似行为 | **E** | similarity_score | 算法待定义 |
| B-012 | Leg Risk | **E** | leg_risk_exposure | 逻辑待设计 |

#### C-合规（4 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| C-001 | 多账户关联合规 | **E** | 关联行为特征 | 算法待定义 |
| C-002 | 规避限制风险 | **E** | 规避行为特征 | 定义待明确 |
| C-003 | 做市资格受损 | **A** | mm_status | 交易所 API |
| C-004 | 账户被限制 | **A** | account_restricted | 交易所 API |

#### P-权限（7 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| P-001 | 提币权限异常 | **A** | withdraw_enabled | Key 权限查询 |
| P-002 | 非授权范围开放 | **A** | Key 权限比对 | Key 权限查询 |
| P-003 | 高危 Key 未停用 | **D** | Key 状态 | 生命周期管理 |
| P-004 | IP 白名单变更 | **D** | IP 白名单 | 快照比对 |
| P-005 | Key 权限变更 | **D** | Key 权限差异 | 快照比对 |
| P-006 | Key 泄露/异常 | **E** | 使用模式 | 检测待定义 |
| P-007 | 提币地址变更 | **D** | 地址白名单 | 快照比对 |

#### BASE（5 条）

| 规则 | 名称 | 类型 | 核心指标 | 说明 |
|------|------|------|---------|------|
| BASE-001 | 时间窗口检查 | **A** | current_time vs config | 实时判断 |
| BASE-002 | 单笔规模检查 | **A** | order_qty/value | 实时判断 |
| BASE-003 | 价格偏离限制 | **A** | price_deviation | 实时可算 |
| BASE-004 | 下单间隔节奏 | **B** | tick_interval | 最近执行时间 |
| BASE-005 | IP 分发异常 | **B** | 同IP/账户占比 | 窗口统计 |

### 7A.3 实现优先级建议

- **Phase 1（A 类）**：19 条实时可读规则先上线，数据直接可用
- **Phase 2（B 类）**：16 条窗口聚合规则，需引入 ClickHouse 时序存储
- **Phase 3（D 类）**：16 条系统自观测规则，需先建 WS 管理器/watchdog/埋点等基础设施
- **Phase 4（C 类）**：9 条跨源衍生规则，需多账户数据汇聚和跨所对齐
- **Phase 5（E 类）**：12 条尚未量化规则，需先完成指标定义和算法设计

---

## 8. 策略状态机 ⛔ POST-MVP（v1.0+ 实施）

> **延后说明**：完整的四态策略状态机延后至 v1.0+ 与三级策略专属规则库同步实施。
> MVP 阶段使用**简化二态模型**：`active` / `paused`，由一级/二级规则触发暂停即可。

### 8.1 MVP 阶段：简化二态模型

```
active ←→ paused
```

- 规则触发 P0 级事件 → 直接 `paused`
- 人工确认后恢复为 `active`
- 不引入 DEGRADED / RESTRICTED 中间态，降低系统复杂度

### 8.2 v1.0+ 完整四态模型（POST-MVP）

```
NORMAL → DEGRADED → RESTRICTED → PAUSED
  ↑          ↑           ↑          │
  └──────────┴───────────┴──────────┘
           （人工恢复 或 自动恢复）
```

- 每条规则触发时，系统模式**降低一个等级**
- 不同模式使用**不同的阈值**（同一条规则，DEGRADED 模式的阈值比 NORMAL 更严格）
- 恢复条件需逐条规则确认：自动恢复（连续 N 个窗口正常）还是人工恢复
- 完整状态机依赖三级策略专属规则库的实现

---

## 9. 规则字段标准（正式版）

每条规则进入配置系统前，必须包含以下字段：

1. **规则编号**（rule_code）
2. **规则名称**（name）
3. **规则层级**（level: main / base / strategy）
4. **来源规则编号**（source_rule_ids）
5. **规则分类**（category: permission / behavior / system / market / compliance / exposure / liquidation）
6. **风险对象**（object_type: account / market / strategy / exchange / system）
7. **业务目的**（description）
8. **适用范围**（scope: 账户+市场 / 账户 / 全局 / 策略）
9. **数据来源**（data_source: redis / kafka / mysql / rest_api）
10. **指标定义**（metrics）
11. **时间窗口**（window: 实时 / 1h / 24h / 自定义）
12. **规则条件**（condition）
13. **阈值参数**（thresholds — 由 risk_rules.config JSON 存储）
14. **告警等级**（alert_level: L1 / L2 / L3）
15. **默认响应等级**（default_priority: P0 ~ P3）
16. **系统动作**（action: alert_only / degrade / restrict / pause / reject_order / phone_call）
17. **冷却/去重**（cooldown: 如 5m 内同规则同对象不重复告警）
18. **恢复条件**（recovery: auto_N_windows / manual）
19. **实现归属**（owner: risk_platform / strategy_engine / shared）
20. **当前实现状态**（status: implemented / seed / pending / research）
21. **是否收编规则**（from_trader: true / false）
22. **备注**（notes）

---

## 10. 与现有代码的兼容性说明

### 10.1 Rule 接口兼容

当前 `Rule` 接口签名：
```go
type Rule interface {
    Name() string
    Code() string
    Check(ctx context.Context, account *models.Account, st *store.Store) ([]models.RiskEvent, error)
}
```

新增规则只需实现此接口，在 `init()` 中注册到 `DefaultRegistry` 即可。三级规则库结构不影响接口定义，仅通过编号前缀（P/B/S/M/C/E/L/BASE/STG/ARB/LIQ/MAN/ACCDIS）区分。

### 10.2 RuleConfig 表兼容

`risk_rules` 表的 `category` 字段建议扩展：
- 现有：`permission / behavior / system / exposure / liquidation`
- 新增：`market / compliance / base / arb / liq / man / accdis`

`config` JSON 字段存储各规则的阈值参数，结构由各规则自定义。

### 10.3 RiskEvent 兼容

`RiskEvent.EventCode` 直接使用规则编号（如 `E-002`, `BASE-002`, `LIQ-001`），无需修改模型。

### 10.4 重命名迁移清单

本次重新编号涉及已有代码的修改：

| 文件 | 修改内容 |
|------|---------|
| `services/risk-engine/alert/rules/s013.go` | 重命名为 `s002.go`，`Code()` 返回 `"S-002"` |
| `scripts/init_mysql.sql` | 种子规则编号更新（见 Section 5.3 映射表） |

### 10.5 建议的目录结构

```
services/risk-engine/alert/rules/
├── rule.go                 # Rule 接口 + Registry（已有）
├── p001.go                 # P-001 提币权限（已有）
├── s002.go                 # S-002 系统失明（已有 s013.go，需重命名）
├── b001.go                 # B-001 异常成交放量 ★
├── b003.go                 # B-003 单笔订单规模 ★
├── e001.go                 # E-001 净 Delta ★
├── e002.go                 # E-002 单市场仓位 ★
├── e004.go                 # E-004 全账户总敞口 ★
├── e005.go                 # E-005 仓位变动速率 ★
├── l001.go                 # L-001 MMR ★
├── l002.go                 # L-002 爆仓距离 ★
├── s003.go                 # S-003 风控只读API频控 ★
├── base_002.go             # BASE-002 订单拦截 ★
├── base_003.go             # BASE-003 价格偏离 ★
└── ...
```

---

## 11. 与 GPT 版本的差异说明

本文档在 `rule_library_master_plan_v2.6_Version1.md`（GPT 版本）基础上做了以下调整：

1. **新增 Risk 梳理中遗漏的规则**：GPT 版本未收编 RISK-ACC-003（MMR）、RISK-ACC-004（可用资金）、RISK-ACC-005（ADL）、RISK-ACC-006（OI 占比）、RISK-ACC-007（爆仓价）、RISK-POS-004（净持仓量/Delta）、RISK-MKT-002（资金费率）。本版全部补充。
2. **增加告警级别对照表**：继承 Risk 梳理的 L1/L2/L3 + 文案模板，与 V2.6 的 P0-P3 响应等级双轨并行。
3. **增加策略状态机**：继承 Risk 梳理的 NORMAL→DEGRADED→RESTRICTED→PAUSED 状态机。
4. **增加代码兼容性说明**：明确与当前 Go 代码的 Rule 接口、RuleConfig 表、RiskEvent 模型的兼容方式。
5. **细化实施优先级**：从"P0/P1/P2"三档细化为按版本里程碑（poc/mvp/v1.0/v2.0）对齐的排期。
6. **修正规则总数**：GPT 版本统计为约 60 条，本版精确统计为 75 条（含三级规则库全部规则）。
7. **策略专属规则拆分延后**：三级策略专属规则库（ARB/LIQ/MAN/ACCDIS 共 9 条）及完整四态策略状态机整体延后至 v1.0+，MVP 阶段聚焦一级（61 条）+ 二级（5 条）= 66 条规则，使用简化二态模型（active/paused）。
8. **收编规则优先排序与重新编号**：各类别内交易员原始需求收编的规则（★）统一排到前面，优先开发。全部编号重新排列，无间隔。详见 Section 5.3 新旧编号映射表。
