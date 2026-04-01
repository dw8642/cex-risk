# [3月24日]Risk梳理

# 一、背景

1. 3月24日前，交易引擎上线现货和合约的拉盘砸盘、吸筹出货、护盘策略，都是做市策略
2. 以之前香港团队风控文档的信息为基准，有效风控规则 22条
3. 去掉324之前不会支持的策略，如套利，剩余20条
4. 相关性较强的规则：
    
    
    | ORD-001  | BASE-002  | 都是在说单笔订单上下限 | 建议合并 |
    | --- | --- | --- | --- |
    | **VOL-002** | **MAN-A002** | 都是防单边，一个偏账户/市场成交量比例，一个偏「操控策略」的方向集中度 | 需要判断是否会重复告警 |

# 二、优先级和维度定义

## 2.1  优先级

| 优先级 | 说明 | 备注 | 策略数量 |
| --- | --- | --- | --- |
| P0 | 直接资金安全（错单、错价、敞口失控）；
系统/环境不可信时仍交易；
节奏失控（短时间大量建平仓）；
策略目标被违背仍继续执行（如拉盘偏离理想价、吸筹出货超总量） | 不做就会直接导致资金损失或系统不可控 | 14 |
| P1 | 对已有 P0 做细或补强；
仅影响某类策略/某类行为 | 明显降低风险、改善行为，但不做不会立刻爆雷 | 3 |
| P2 | 合规/运维（时间窗口）；
防交易所限频/识别（下单间隔、多账户分布）； | 合规、运维、体验类，可延后或按需 | 3 |

## 2.2  划分维度及维度优先级

| 维度 | 含义 | 数据源 | 重要性 |
| --- | --- | --- | --- |
| Order | 单笔订单或单笔成交 | 订单尺寸、价格、方向、时间、执行账户 | 最后一关：单笔不错，就不会产生错误仓位；实现简单、见效快 |
| Exchange | 某一家交易所的连接/API/状态 | 该交易所报错率、连接状态、限频 | 基础设施：API/连接异常时不降级，后续所有维度数据可能不可靠 |
| Account | 交易账户（多市场、多策略汇总） | 总敞口、总成交量、全账户订单 | 汇总层：总敞口、总成交量等，防多笔「合规单笔」叠加成账户级风险 |
| Market | 单个交易对/标的（如 BTCUSDT） | 该标的仓位、成交量、价格、订单簿 | 单标的层：单市场曝险、波动、单边比例等 |
| Strategy | 单个策略实例 | 该策略的仓位、成交、订单、配置参数 | 业务层：拉盘砸盘理想价、吸筹出货总量、做市网格等 |

## 2.3  按优先级和维度拆完，现有的20条规则优先级如下：

| 维度 | P0 （ 14个 ） | P1（3个） | P2（3个） |
| --- | --- | --- | --- |
| Order | ORD-001, BASE-002, ORD-002 | — | — |
| Exchange | SYS-001 | — | — |
| Account | POS-002, POS-003, VOL-001 | VOL-002 | — |
| Market | POS-001, MKT-001, LIQ-P001, LIQ-P002 | VOL-002 | — |
| Strategy | MAN-A001, ACC/DIS-001, LIQ-P003 | MAN-A002, ACC/DIS-002 | BASE-001, BASE-003, BASE-004 |

# 三、风控规则汇总

## 3.1  报警级别：

|  | 程度说明 | 触发条件 | 系统动作 | 文案 | 文案示例 |
| --- | --- | --- | --- | --- | --- |
| **L1报警** | 普通报警，仅需交易员关注，可稍后处理 | 风控项  超出阈值，策略状态没有改变 | 通知：TG 普通alert | 【告警】【Strategy  id】【RISK id】【告警内容】，请尽快处理 | 【告警】【Strategy 23】【RISK-ACCOUNT-006】多头OI占比 > 0.2，请知晓 |
| **L2 严重报警** | 紧急报警，需交易员尽快处理，可能影响资金/合规 | 风控项超出阈值，策略状态有变化 | 通知：TG 紧急alert
风控：降级或暂停 |  |  |
| **L3 电话报警** | 最高程度，可能影响策略及资金安全，需交易员立即介入 | 风控项超出阈值，策略状态有变化 | 通知：
  1. TG 紧急alert
  2. 电话
风控：拒单/暂停/降级
 | 【紧急告警】【Strategy  id】【RISK id】【告警内容】，已被风控降级，请尽快处理 | 【紧急告警】【Strategy 34】【RISK-ACCOUNT-007】爆仓价/当前价格 > 0.8，已被风控降级，请尽快处理 |

## 3.1  本期P0

### 3.1.1  3月6日之前完成的风控项

| 维度 | 编号 | 名称 | 目的 | 监控参数 | 触发报警条件 | 系统动作 | 报警级别 | Alert 报警文案 | 阈值配置项 | Alert 报警文案（不用了，部分参数可能还要参考，项目完成后可删掉） | 备注 |  |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Order | RISK-ORD-001 | 单笔订单尺寸限制 | 防 fat finger、异常大单 | order_qty, order_value_usd | order_qty > 阈值 或 order_value_usd > 阈值 | 报警 | **L1** | Client ID: 1
BigOrder : BTC-USDT 50k USDT | order_value_usd

LARGEORDER ALERT
LARGEORDER CALL  | **【报警】单笔挂单超额**

Level: WARNING
Content: order_value_usd 为1000usdt，超过阈值200usdt

策略：{strategy_id}
账户：{account_id}
标的：{symbol}
方向：{side}

请确认是否为意图单。
 |  |  |
| Account | RISK-ACC-003 | 合约维持保证金率 | 账户安全，防止账户爆仓 | maint_margin_rate | maint_margin_rate > 阈值 | 报警 + 降级/暂停 | **分级监控

L2：threshold_1
L3: threshold_2** | Client ID: 1
Binance MMR DANGER : 2.5% (min 3%) | MMR ALERT
MMR CALL | 【严重报警】维持保证金率超阈值，存在强平风险 

level: ALERT
Content: 维持保证金率：{maint_margin_rate}
高于阈值{threshold}
账户：{account_id}

请尽快补充保证金或减仓以降低风险。 

【紧急报警】维持保证金率超阈值，存在强平风险 

level: CRITICAL
Content: 维持保证金率：{maint_margin_rate}
高于阈值{threshold}
账户：{account_id}

当前系统已执行报警并降级/暂停。请尽快补充保证金或减仓以降低风险。 |  |  |
|  | RISK-ACC-004 | 账户可用资金 | 监控账户资金 | current_available,
min_available | current_available < 阈值 | 报警 | **L1** | Client ID: 1
AvailBal DANGER : 1000 < 5000 | AvailBal | 【报警】账户可用资金低于阈值 

Level: WARNING
Content: 可用资金为{current_available}usdt，超过阈值{threshold}
账户：{account_id}

请关注账户资金状况，必要时充值或减仓。  |  |  |
|  | RISK-ACC-005 | ADL风险 | 防合约持仓ADL | adl | adl > 阈值 | 报警 + 降级/暂停 | **L1： threshold_1
L2：threshold_2**
 | Client ID: 1
Binance UM ADL : 3 -> 4 | ADL ALERT
ADL CALL | 【报警】ADL 等级超阈值，存在自动减仓风险 

Level: WARNING
Content: 当前ADL为{adl}，超出阈值{threshold_1}
账户：{account_id}

请尽快降低仓位或补充保证金，避免被交易所自动减仓。

【严重报警】ADL 等级超阈值，存在自动减仓风险 

Level: ALERT
Content: 当前ADL为{adl}，超出阈值{threshold_1}
账户：{account_id}

系统已报警并降级/暂停。请尽快降低仓位或补充保证金，避免被交易所自动减仓。 |  |  |
| （合约还没开放）
dex 做不了
aster拿不到oi数据 | RISK-ACC-006 | 单边OI占比 |  | oi
total_position | total_position / oi > 阈值 | 报警 | **L1** | Client ID: 1
OI long ratio DANGER : 85% > 80% | OI RATIO | 【报警】单边持仓占比过高 
Level: WARNING
Content: {side}头持仓OI占比{ratio_pct}%，超出阈值{threshold}
仓位：{total_position}
标的 OI：{oi}

建议关注集中度风险，酌情分散或减仓。 |  |  |
|  | RISK-ACC-007 | 爆仓价 | 账户安全 | liq_price
mark_price | 爆仓价/当前价格 > 阈值 | 报警 + 降级/暂停 | **L2 ：**threshold_1
**L3：**threshold_2 | Client ID: 1
LiqPx DANGER : 95000 (1.2% to mark) | LIQPXDIS ALERT
LIQPXDIS CALL | 【严重报警】爆仓价接近当前价格 

Level: ALERT
Content: 爆仓价{liq_price}与当前标记价格{mark_price}距离{distance_pct}%，低于阈值{threshold_1}
账户：{account_id}

系统已报警并降级。建议尽快充值保证金或减仓。 

【电话报警】爆仓价极度接近，紧急 
Level: CRITICAL
Content: 爆仓价{liq_price}与当前标记价格{mark_price}距离{distance_pct}%，低于阈值{threshold_2}
账户：{account_id}
账户：{account_id}
爆仓价：{liq_price}
标记价：{mark_price}
已暂停并触发电话报警。请立即处理,充值保证金或减仓，避免强平。
 |  |  |
|  | RISK-POS-004 | 净持仓量 | 防止策略单向敞口暴露 | delta size | delta size > 阈值 | 报警 | **L2 ：**threshold_1
**L3：**threshold_2 | BINANCE SPOT UAI DELTA: 5000 > 2000 | DELTA ALERT
DELTA CALL |  |  |  |
| Market | RISK-MKT-001 | 市场异常波动保护 | 市场异常波动时自动收敛风险 | price_jump_pct_1h, ~~price_jump_pct_24h（~~按标的） | 任一项 > 阈值 | 拉阔挂单、降参与度并降级
报警 | **L1 ：[lower，upper]** | Client ID: 1
1hChg DANGER : ETH-USDT -4.1% > 4% cap | 1HCHG LOWER
1HCHG UPPER | 【报警】代币价格波动 
Level: WARNING
Content: {symbol}价格，过去一小时，涨幅超过20%/ 跌幅超过10%

建议关注代币价格变化 |  |  |
|  | RISK-MKT-002 | 资金费率波动 | 防止资金费率超出阈值，影响合约持仓收益 | funding_rate | funding_rate > 阈值上限、或< 阈值下限 | 报警 | **L1 ：[lower，upper]** | Client ID: 1
Binance UM FundRate DANGER : 0.05% > 0.03% | FUND LOWER
FUND UPPER |  |  |  |

 

### 3.1.2  3月13日之前完成的风控项

| 维度 | 编号 | 名称 | 目的 | 监控参数 | 触发报警条件 | 系统动作 | 报警级别 | Alert报警文案 | 阈值配置项 | 阈值由谁来定 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Order | RISK-ORD-002 | 订单价格偏离限制 | 防错价、过度激进挂单 | price_deviation（挂单价与市价偏离）
拒单率：目前还无法统计 | price_deviation > 阈值 | 必要时降级 | 通知 L1 | Warning:
Account ID: 1 UAI Price Deviation DANGER : 50% > 20% | PRICE DEVIATION | 风控做(完成) |
| Exchange | RISK-SYS-001 | 系统与数据异常保护

和Daniel确认 | 交易所/系统不稳定时保护资金 | api_error_rate_1h, api_error_rate_24h | api_error_rate > 阈值 | 降级；严重时 PAUSED
推送 | L2：超出阈值
L3：被paused | Alert:
Binance API Error Rate DANGER : 1h 10% > 5%

Alert:
Strategy pump Binance  futures UAI PAUSED:Binance API Error Rate DANGER : 1h 50% | api error rate | 策略本身 |
| Account | RISK-POS-002 | 全账户总曝险控制
还没做 | 防多策略叠加导致整体曝险失控 | total_exposure_usd = Σ abs(position_usd) | total_exposure_usd > 阈值 | ~~降一级~~  
报警 | L2 | Alert:
UAI total exposure  : 100M $ > 50M $ | exposure_usd | 风控做(未做)
所有账户所有市场 |
|  | RISK-POS-003 | 仓位变化速率限制
还没做 | 防短时间快速吸筹或出货 | position_change_usd_1h, ~~position_change_usd_24h~~ | 任一项 > 阈值 | ~~降一级~~
报警 | L1 | Warning:
UAI POSITION CHANGE : 1h 350,000 $ > 300,000 $ | position_change_usd_1h | 风控做(未做) |
|  | RISK-VOL-001 | 成交量节奏控制还没做 | 防短时间成交量异常放大 | volume_usd_1h, ~~volume_usd_24h~~ | 任一项 > 阈值 | ~~降一级~~
报警 | L1 | Warning:
UAI VOLUME  : 1h 20M $ > 15M $ | volume_usd_1h | 风控做(未做) |
| Market | RISK-POS-001 | 单市场仓位曝险控制（delta） | 防单市场仓位过集中 | position_usd = abs(position_qty)×mark_price（按标的）
单个交易所所有账户持仓 | position_usd > 阈值 | ~~降一级~~
报警 | L1 | Warning:UAI Binance POSITION 600,000 $ > 500,000 $ | single exchange position | 风控做(完成) |
|  | RISK-LIQ-P001 | 网格控制 1
做不了，交易引擎直接下单 | 挂单不跨中间价 | order.execPrice, order.side, midPrice（该交易所+标的） | BUY 且 execPrice > mid 或 SELL 且 execPrice < mid | 降一级 | L*2* | Alert:
 UAI Binance futures  : BUY 30,000 $ @ 100.5 > mid 100.0 |  | 策略本身
风控做不到 |
|  | RISK-LIQ-P002 | 网格控制 2
需要交易引擎配合，如果需要，可以晚一点再做 | 网格层级、spread、价差在设定范围内 | gridLevels, 第一档与 mid 的 spread, 价差, midSpreadBetweenGridsPct | 任一项超配置 | 降一级 | L2 | Alert:
UAI Binance futures GRID levels : 20 > 12 | gridLevels
minSpreadFromMidPct
gridSpacingPct | 策略本身
风控做不到
需要网格订单薄 |
| Strategy | RISK-MAN-A001 | 价格操控风险控制
已做完，拉砸盘策略上线时，再评估 | 拉/砸盘时价格偏离理想价则暂停 | exec_price, ideal_price（如 TWAP）, 方向 | 拉升时 exec_price > ideal_price 或 下拉时 exec_price < ideal_price | 降一级 | L2 | Alert::
Account ID: 1
PxDev DANGER : 0.5% vs ideal (pump) | IDEAL PRICE DEVIATION | 风控做 |
|  | RISK-ACC/DIS-001 | 交易总上限控制 | 吸筹/出货总量不超计划
还没做 | totalQuantity, totalTradingAmount | totalQuantity > totalTradingAmount | PAUSED | L2 | Alert:
Strategy pump Binance  futures UAI PAUSED : UAI ACCUMULATION  traded 110% > 100% plan
 |  | 风控做 |
|  | RISK-LIQ-P003 | 最大订单量 | 每 tick 挂单量不超分配比例

需要交易引擎配合 | ∑limitOrderPlaced.size, maxCapitalAllocationPct, currentInventory.baseTotal | ∑size > maxCapitalAllocationPct × baseTotal | 降一级 | L2 | Alert:
UAI TICK SIZE : 12% of inventory > 10% cap | TICK SIZE CAP PERCENT | 策略本身
需要网格订单薄 |
|  | RISK-BASE-004 | 订单账户检查 | 避免间隔内挂单集中在同一账户/IP

等IP方案确定后一起做 | X 间隔内 trades.executorIp（或 account_id）, maxDistributedAccounts | 同一 IP/账户挂单过多，未满足分散要求
多久的订单？ | ~~降一级
报警~~ | L1 | Warning:
ACCOUNT id: 15
UAI open order concentration:  70% > 60% | single account open order concentration | 风控检查 |
| buffer |  |  |  |  |  |  |  | buffer |  |  |

待确认：

1. 由谁来定风控规则阈值：一些风控规则，是根据策略配置监控，交易员配在策略里，就OK吗？

## 3.2  P1和P2的部分，后面做，此处仅供参考和讨论

| 维度 | 编号 | 名称 | 目的 | 监控参数 | 触发报警条件 | 系统动作 |
| --- | --- | --- | --- | --- | --- | --- |
| Exchange/Account | RISK-VOL-002 | 单边成交比例限制 | 防长时间单边吸筹或出货 | directional_ratio_1h, directional_ratio_24h（可按账户或按标的） | directional_ratio > 阈值 | 限制同向交易并降级 |
| Strategy | RISK-MAN-A002 | 订单风险控制 | 减单边订单集中、避免制造市场假象 | X 间隔内 trades.side（buy/sell 笔数或量）, reverseTradeProbability | ∑buy/∑sell > reverseTradeProbability | 降一级 |
| Strategy | RISK-ACC/DIS-002 | 交易价格控制 | 吸筹/出货在指定价格内完成 | exec_price, priceLimit | exec_price > priceLimit | DEGRADED |

| 维度 | 编号 | 名称 | 目的 | 监控参数 | 触发报警条件 | 系统动作 |  |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Strategy | RISK-BASE-001 | 时间范围检查 | Bot 仅在配置时间窗口内运行 | current_time, start_time, end_time | current_time 不在 [start_time, end_time] 内 | PAUSED |  |
| Strategy | RISK-BASE-003 | 下单间隔检查 | 控制下单间隔，降低被交易所识别为高频的风险 | trades[].execTime, tickIntervalMs, randomDelayJitter（tick） | 相邻两笔 execTime 间隔 < tick | 降一级 |  |

# 四、策略状态机

![画布 3.png](%5B3%E6%9C%8824%E6%97%A5%5DRisk%E6%A2%B3%E7%90%86/%E7%94%BB%E5%B8%83_3.png)

待确认：风控判断当前风险降低或消失，策略状态会回退吗？

# 附录：风控规则来源

1. **系統風控規則政策（Rule Policy v1.0）：**
[https://www.notion.so/emojidao/Rule-Policy-v1-0-2eacfcb53d828021b5fad017f322706d?source=copy_link](https://www.notion.so/2eacfcb53d828021b5fad017f322706d?pvs=21)
2. **系統風控規則政策_v1.1:** [https://docs.google.com/document/d/1h1wfsk4TYbZKnk_ZCqkmkXdF6grwI9m6bapFTTVEU54/edit?tab=t.0](https://docs.google.com/document/d/1h1wfsk4TYbZKnk_ZCqkmkXdF6grwI9m6bapFTTVEU54/edit?tab=t.0)
3. **系統風控規則政策_v1.2:** [https://docs.google.com/document/d/1h1wfsk4TYbZKnk_ZCqkmkXdF6grwI9m6bapFTTVEU54/edit?tab=t.4wbf7s0n9e9](https://docs.google.com/document/d/1h1wfsk4TYbZKnk_ZCqkmkXdF6grwI9m6bapFTTVEU54/edit?tab=t.4wbf7s0n9e9)