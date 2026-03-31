# CEX 风控系统 — 数据库表结构总览

> 汇总自：04_系统架构与开发方案.md、05_每日开发计划、scripts/init_mysql.sql、scripts/init_clickhouse.sql
> 生成时间：2026-03-31

---

## 一、总览

### 1.1 存储分工

| 存储引擎 | 定位 | 表数量 | 说明 |
|---------|------|--------|------|
| MySQL 8.0 | 配置 + 事务 + 审计 | 14 张 | 四层治理模型、规则配置、事件生命周期、控制动作、审计日志 |
| ClickHouse 23.12 | 时序 + 审计 + 分析 | 8 张 | 交易/仓位/余额快照、全量订单/成交/资金流水、指标历史 |
| Redis 7 | 实时状态缓存 | Key-Value | 实时仓位、余额、数据新鲜度向量、规则去重、限流令牌桶 |

### 1.2 版本交付节奏

| 版本 | MySQL 表 | ClickHouse 表 | 说明 |
|------|---------|--------------|------|
| 最小闭环 (D1-D3) | 9 张（已实现） | 5 张（已实现） | accounts, risk_rules, risk_events 等基础表 |
| MVP (D4-D13) | +3 张 | +3 张 | teams, strategies, rule_bindings, control_actions + orders, fills, ledger_entries |
| V1.0 (D14-D28) | +1 张 | — | webhook_configs |
| V2.0 (D29-D40) | +2 张 | — | backtest_tasks, strategy_baselines, arbitration_rules |

---

## 二、MySQL 表结构

### 2.1 四层治理模型（Project → Team → Strategy → Account）

#### projects — 做市项目（最顶层）

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 项目 ID | D1 |
| name | VARCHAR(128) NOT NULL | 项目名称 | D1 |
| description | TEXT | 项目描述 | D1 |
| risk_budget_daily_usd | DECIMAL(20,2) | 日风险预算 (USD) | D4 |
| allowed_symbols | JSON | 项目级允许交易对白名单 | D4 |
| status | ENUM('active','paused','archived') | 状态 | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |
| updated_at | TIMESTAMP | 更新时间 | D1 |

#### teams — 团队（内部组 / 外部合作伙伴）

> MVP D4 新增，替代原 partners 表

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 团队 ID | D4 |
| project_id | VARCHAR(36) NOT NULL | 所属项目 | D4 |
| name | VARCHAR(128) NOT NULL | 团队名称 | D4 |
| team_type | ENUM('internal','external') | 团队类型 | D4 |
| contact | VARCHAR(256) | 联系方式 | D4 |
| trust_level | ENUM('high','medium','low') | 信任等级（影响规则严格度） | D4 |
| risk_budget_ratio | DECIMAL(5,4) | 占项目风险预算比例 | D4 |
| status | ENUM('active','suspended','terminated') | 状态 | D4 |
| created_at | TIMESTAMP | 创建时间 | D4 |
| updated_at | TIMESTAMP | 更新时间 | D4 |

索引：`idx_project(project_id)`

#### strategies — 策略实例（可缺失层）

> MVP D4 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 策略 ID | D4 |
| team_id | VARCHAR(36) NOT NULL | 所属团队 | D4 |
| project_id | VARCHAR(36) NOT NULL | 所属项目 | D4 |
| strategy_type | ENUM('market_making','hedging','rebalancing','arbitrage','manual') | 策略类型 | D4 |
| name | VARCHAR(128) NOT NULL | 策略名称 | D4 |
| expected_position_range | JSON | 预期持仓范围 `{"min":-5,"max":5,"unit":"BTC"}` | D4 |
| expected_order_rate | JSON | 预期下单频率 `{"min":100,"max":500,"unit":"orders/min"}` | D4 |
| expected_otr_range | JSON | 预期挂撤单比 `{"min":3,"max":20}` | D4 |
| expected_spread_range | JSON | 预期报价价差 `{"min_bps":2,"max_bps":15}` | D4 |
| max_single_order_size | DECIMAL(20,8) | 单笔最大下单量 | D4 |
| allowed_symbols | JSON | 允许交易对 | D4 |
| allowed_exchanges | JSON | 允许交易所 | D4 |
| hedging_pair_id | VARCHAR(36) | 关联对冲策略 ID | D4 |
| status | ENUM('active','paused','stopped') | 状态 | D4 |
| created_at | TIMESTAMP | 创建时间 | D4 |
| updated_at | TIMESTAMP | 更新时间 | D4 |

索引：`idx_team(team_id)`, `idx_project(project_id)`

#### accounts — 交易所监控账户（最底层）

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 账户 ID | D1 |
| project_id | VARCHAR(36) NOT NULL | 所属项目 | D1 |
| team_id | VARCHAR(36) | 所属团队（可空，直接挂项目） | D4 扩展 |
| strategy_id | VARCHAR(36) | 所属策略（可空） | D4 扩展 |
| partner_id | VARCHAR(36) | 合作伙伴（D1 遗留，D4 后由 team_id 替代） | D1 |
| exchange_id | VARCHAR(32) | 交易所标识 (binance/okx/bybit) | D1 |
| market_type | ENUM('spot','futures','margin') | 市场类型 | D1 |
| label | VARCHAR(128) | 显示标签 | D1 |
| account_group | VARCHAR(36) | 账户组 ID（多账户治理） | D4 扩展 |
| status | ENUM('active','paused','disabled') | 状态 | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |
| updated_at | TIMESTAMP | 更新时间 | D1 |

索引：`idx_project`, `idx_team`, `idx_strategy`, `idx_exchange(exchange_id, market_type)`, `idx_group(account_group)`

### 2.2 API Key 与权限

#### api_key_configs — API Key 预期配置（不存明文）

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 配置 ID | D1 |
| account_id | VARCHAR(36) NOT NULL | 关联账户 | D1 |
| team_id | VARCHAR(36) | 持有该 Key 的团队 | D4 扩展 |
| key_label | VARCHAR(128) | 标签 | D1 |
| key_hash | VARCHAR(64) | API Key 的 SHA256 hash | D1 |
| expected_permissions | JSON NOT NULL | 预期权限 `{"spot":false,"futures":true,"withdraw":false}` | D1 |
| expected_ip_whitelist | JSON | 预期 IP 白名单 | D1 |
| allowed_symbols | JSON | 允许交易对 | D1 |
| status | ENUM('active','revoked','expired') | 状态 | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |
| updated_at | TIMESTAMP | 更新时间 | D1 |

索引：`idx_account(account_id)`, `idx_team(team_id)`

### 2.3 风控规则

#### risk_rules — 风控规则定义（全局唯一）

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 规则 ID | D1 |
| rule_code | VARCHAR(16) NOT NULL UNIQUE | 规则编号（P-001, S-013 等） | D1 |
| name | VARCHAR(256) NOT NULL | 规则名称 | D1 |
| description | TEXT | 规则描述 | D1 |
| category | ENUM('permission','behavior','system','market','compliance','exposure','liquidation') | 规则分类 | D1 (D4 扩展 category) |
| default_priority | ENUM('P0','P1','P2','P3') | 默认优先级 | D4 |
| default_config | JSON | 默认参数 | D4 |
| reversibility | ENUM('R0','R1','R2','R3') | 默认控制动作可逆性 | D4 |
| enabled | TINYINT(1) | 全局开关 | D1 |
| config | JSON | 规则参数（D1 简版，D4 后由 default_config 替代） | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |
| updated_at | TIMESTAMP | 更新时间 | D1 |

种子数据（10 条规则）：

| rule_code | 名称 | 分类 | 默认优先级 |
|-----------|------|------|-----------|
| P-001 | 提币权限异常开启 | permission | P0 |
| P-002 | 非授权交易范围开放 | permission | P0 |
| B-001 | 非授权交易 | behavior | P0 |
| B-003 | 高成交低净仓变化（疑似对敲） | behavior | P0 |
| B-004 | 异常滑点 | behavior | P0 |
| B-007 | 对敲/刷量嫌疑 | behavior | P0 |
| S-001 | 策略心跳超时 | system | P0 |
| S-007 | 数据真相源不一致 | system | P0 |
| E-001 | 净 Delta 过高 | exposure | P0 |
| L-001 | 维持保证金率过低 | liquidation | P0 |

#### rule_bindings — 规则绑定（四层任意层可绑定）

> MVP D4 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 绑定 ID | D4 |
| rule_code | VARCHAR(16) NOT NULL | 绑定哪条规则 | D4 |
| scope_type | ENUM('global','project','team','strategy','account') | 绑定层级 | D4 |
| scope_id | VARCHAR(36) | 对应层级 ID，global 时为空 | D4 |
| enabled | TINYINT(1) | 该层级是否启用 | D4 |
| priority | ENUM('P0','P1','P2','P3') | 覆盖默认优先级 | D4 |
| config | JSON | 该层级的参数覆盖（阈值等） | D4 |
| created_at | TIMESTAMP | 创建时间 | D4 |
| updated_at | TIMESTAMP | 更新时间 | D4 |

唯一约束：`UNIQUE(rule_code, scope_type, scope_id)`

解析优先级：`Account > Strategy > Team > Project > Global`（就近生效、上层兜底）

### 2.4 风险事件与控制

#### risk_events — 风险事件记录

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 事件 ID | D1 |
| event_code | VARCHAR(16) NOT NULL | 触发规则编号 | D1 |
| level | ENUM('P0','P1','P2','P3') NOT NULL | 事件级别 | D1 |
| status | ENUM('open','ack','handling','resolved','false_positive') | 事件状态 | D1 |
| object_type | VARCHAR(32) NOT NULL | 触发对象类型 | D1 |
| object_id | VARCHAR(36) NOT NULL | 触发对象 ID | D1 |
| project_id | VARCHAR(36) | 关联项目 | D1 |
| team_id | VARCHAR(36) | 关联团队 | D4 扩展 |
| strategy_id | VARCHAR(36) | 关联策略 | D4 扩展 |
| account_id | VARCHAR(36) | 关联账户 | D4 扩展 |
| title | VARCHAR(256) NOT NULL | 事件标题 | D1 |
| details | JSON | 事件详情 | D1 |
| freshness_snapshot | JSON | 触发时的数据新鲜度向量 | D4 扩展 |
| action_taken | VARCHAR(32) | 已执行的控制动作 | D4 扩展 |
| action_reversibility | ENUM('R0','R1','R2','R3') | 动作可逆性 | D4 扩展 |
| ack_by | VARCHAR(64) | 确认人 | D1 |
| ack_at | TIMESTAMP | 确认时间 | D1 |
| resolved_at | TIMESTAMP | 解决时间 | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |

索引：`idx_status`, `idx_level`, `idx_event_code`, `idx_project`, `idx_team`, `idx_account`, `idx_created(DESC)`

#### control_actions — 控制动作执行记录

> MVP D8 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 动作 ID | D8 |
| event_id | VARCHAR(36) NOT NULL | 触发事件 | D8 |
| action_type | ENUM('revoke_key','pause_strategy','cancel_orders','reduce_position','alert_only') | 动作类型 | D8 |
| reversibility | ENUM('R0','R1','R2','R3') NOT NULL | 可逆性等级 | D8 |
| target_type | VARCHAR(32) NOT NULL | 作用对象类型 | D8 |
| target_id | VARCHAR(36) NOT NULL | 作用对象 ID | D8 |
| status | ENUM('pending','approved','executing','success','failed','rolled_back') | 执行状态 | D8 |
| approved_by | VARCHAR(64) | R0 动作需人工审批人 | D8 |
| detail | JSON | 执行详情 | D8 |
| created_at | TIMESTAMP | 创建时间 | D8 |
| executed_at | TIMESTAMP | 执行时间 | D8 |

索引：`idx_event(event_id)`, `idx_status(status)`

控制动作分级：

| 等级 | 含义 | 动作示例 | 审批 |
|------|------|---------|------|
| R3 | 纯观测 | 告警通知 | 无需 |
| R2 | 可逆-自动 | 暂停策略、减仓 | 无需 |
| R1 | 可逆-有代价 | 撤单、平仓 | 无需 |
| R0 | 不可逆 | 吊销 API Key | 人工审批（P0-Critical 可旁路） |

### 2.5 通知与审计

#### notification_logs — 通知发送记录

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 记录 ID | D1 |
| event_id | VARCHAR(36) NOT NULL | 关联事件 | D1 |
| channel | ENUM('telegram_msg','telegram_voice','twilio','email') | 通知渠道 | D1 |
| recipient | VARCHAR(128) NOT NULL | 接收人 | D1 |
| status | ENUM('pending','sent','delivered','failed') | 发送状态 | D1 |
| error_msg | TEXT | 失败原因 | D1 |
| sent_at | TIMESTAMP | 发送时间 | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |

索引：`idx_event`, `idx_status`

#### audit_logs — 不可变审计日志

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | BIGINT AUTO_INCREMENT PK | 自增 ID | D1 |
| timestamp | TIMESTAMP(3) | 操作时间（毫秒） | D1 |
| actor | VARCHAR(64) NOT NULL | 操作者 (system/user/api) | D1 |
| action | VARCHAR(64) NOT NULL | 操作类型 | D1 |
| resource | VARCHAR(128) NOT NULL | 操作对象 | D1 |
| detail | JSON | 详情 | D1 |
| ip | VARCHAR(45) | 操作 IP | D1 |

索引：`idx_timestamp`, `idx_actor`, `idx_resource`

设计要点：仅 INSERT，禁止 UPDATE/DELETE，定期 hash chain 校验（每 1000 条计算 SHA256 链）。

### 2.6 演示/过渡表

#### trades_log — 成交明细（演示版，MySQL 暂替 ClickHouse）

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | BIGINT AUTO_INCREMENT PK | 自增 ID | D1 |
| exchange_id | VARCHAR(32) NOT NULL | 交易所 | D1 |
| account_id | VARCHAR(36) NOT NULL | 账户 | D1 |
| symbol | VARCHAR(32) NOT NULL | 交易对 | D1 |
| side | ENUM('BUY','SELL') | 方向 | D1 |
| price | DECIMAL(20,8) | 价格 | D1 |
| quantity | DECIMAL(20,8) | 数量 | D1 |
| quote_qty | DECIMAL(20,8) | 成交额 | D1 |
| realized_pnl | DECIMAL(20,8) | 已实现盈亏 | D1 |
| commission | DECIMAL(20,8) | 手续费 | D1 |
| trade_id | VARCHAR(64) | 交易所原始 trade ID | D1 |
| order_id | VARCHAR(64) | 订单 ID | D1 |
| trade_time | TIMESTAMP(3) | 成交时间 | D1 |
| ingest_time | TIMESTAMP(3) | 采集时间 | D1 |
| source | ENUM('ws','rest') | 数据来源 | D1 |

索引：`idx_account_time`, `idx_symbol_time`, `idx_trade_id`

#### partners — 合作伙伴（D1 版本，D4 后由 teams 替代）

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 伙伴 ID | D1 |
| name | VARCHAR(128) NOT NULL | 名称 | D1 |
| contact | VARCHAR(256) | 联系方式 | D1 |
| risk_level | ENUM('low','medium','high','critical') | 风险等级 | D1 |
| status | ENUM('active','suspended','terminated') | 状态 | D1 |
| created_at | TIMESTAMP | 创建时间 | D1 |
| updated_at | TIMESTAMP | 更新时间 | D1 |

### 2.7 V1.0+ 扩展表

#### webhook_configs — Webhook 回调配置

> V1.0 D35 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 配置 ID | D35 |
| url | VARCHAR(512) NOT NULL | 回调地址 | D35 |
| secret | VARCHAR(128) | HMAC 签名密钥 | D35 |
| event_filter | JSON | 事件过滤条件（级别/规则/项目） | D35 |
| enabled | TINYINT(1) | 是否启用 | D35 |
| retry_count | INT DEFAULT 3 | 重试次数 | D35 |
| created_at | TIMESTAMP | 创建时间 | D35 |
| updated_at | TIMESTAMP | 更新时间 | D35 |

### 2.8 V2.0 扩展表

#### backtest_tasks — 回测任务

> V2.0 D29 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 任务 ID | D29 |
| rule_code | VARCHAR(16) NOT NULL | 回测规则 | D29 |
| params | JSON NOT NULL | 回测参数 | D29 |
| time_range_start | TIMESTAMP | 回测起始时间 | D29 |
| time_range_end | TIMESTAMP | 回测结束时间 | D29 |
| status | ENUM('pending','running','done','failed') | 任务状态 | D29 |
| result | JSON | 回测结果（命中率/误报率等） | D29 |
| created_at | TIMESTAMP | 创建时间 | D29 |
| finished_at | TIMESTAMP | 完成时间 | D29 |

#### strategy_baselines — 策略行为基线

> V2.0 D32 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 记录 ID | D32 |
| strategy_id | VARCHAR(36) NOT NULL | 策略 ID | D32 |
| metric_name | VARCHAR(64) NOT NULL | 指标名 (order_rate/otr/avg_spread/maker_ratio/avg_order_size) | D32 |
| p5 | DECIMAL(20,8) | 第 5 百分位 | D32 |
| p50 | DECIMAL(20,8) | 中位数 | D32 |
| p95 | DECIMAL(20,8) | 第 95 百分位 | D32 |
| calculated_at | TIMESTAMP | 计算时间 | D32 |

#### arbitration_rules — 仲裁规则配置

> V2.0 D34 新增

| 字段 | 类型 | 说明 | 版本 |
|------|------|------|------|
| id | VARCHAR(36) PK | 规则 ID | D34 |
| state_priority | JSON NOT NULL | 状态优先级权重 | D34 |
| conflict_strategy | VARCHAR(32) NOT NULL | 冲突解决策略 | D34 |
| config | JSON | 配置详情 | D34 |
| created_at | TIMESTAMP | 创建时间 | D34 |
| updated_at | TIMESTAMP | 更新时间 | D34 |

---

## 三、ClickHouse 表结构

所有 ClickHouse 表使用 MergeTree 引擎，按时间分区，配置 TTL 自动过期。

### 3.1 最小闭环版（已实现，scripts/init_clickhouse.sql）

#### trades — 成交明细（主存储）

| 字段 | 类型 | 说明 |
|------|------|------|
| exchange_id | LowCardinality(String) | 交易所 |
| account_id | String | 账户 ID |
| symbol | LowCardinality(String) | 交易对 |
| side | Enum8('BUY'=1, 'SELL'=2) | 方向 |
| price | Decimal(20,8) | 价格 |
| quantity | Decimal(20,8) | 数量 |
| quote_qty | Decimal(20,8) | 成交额 |
| realized_pnl | Decimal(20,8) | 已实现盈亏 |
| commission | Decimal(20,8) | 手续费 |
| trade_id | String | 交易所成交 ID |
| order_id | String | 订单 ID |
| trade_time | DateTime64(3) | 成交时间 |
| ingest_time | DateTime64(3) | 采集时间 |
| source | Enum8('ws'=1, 'rest'=2) | 数据来源 |

分区：`toYYYYMMDD(trade_time)` | 排序：`(account_id, symbol, trade_time)` | TTL：90 天

#### position_snapshots — 仓位快照

| 字段 | 类型 | 说明 |
|------|------|------|
| exchange_id | LowCardinality(String) | 交易所 |
| account_id | String | 账户 ID |
| symbol | LowCardinality(String) | 交易对 |
| position_side | Enum8('LONG'=1, 'SHORT'=2, 'BOTH'=3) | 持仓方向 |
| quantity | Decimal(20,8) | 持仓量 |
| entry_price | Decimal(20,8) | 入场价 |
| mark_price | Decimal(20,8) | 标记价 |
| unrealized_pnl | Decimal(20,8) | 未实现盈亏 |
| leverage | UInt16 | 杠杆倍数 |
| margin_type | Enum8('cross'=1, 'isolated'=2) | 保证金模式 |
| snapshot_time | DateTime64(3) | 快照时间 |
| source | Enum8('ws'=1, 'rest'=2) | 数据来源 |

分区：`toYYYYMMDD(snapshot_time)` | 排序：`(account_id, symbol, snapshot_time)` | TTL：30 天

#### balance_snapshots — 余额快照

| 字段 | 类型 | 说明 |
|------|------|------|
| exchange_id | LowCardinality(String) | 交易所 |
| account_id | String | 账户 ID |
| asset | LowCardinality(String) | 资产类型 |
| wallet_balance | Decimal(20,8) | 钱包余额 |
| available_balance | Decimal(20,8) | 可用余额 |
| unrealized_pnl | Decimal(20,8) | 未实现盈亏 |
| margin_balance | Decimal(20,8) | 保证金余额 |
| maint_margin | Decimal(20,8) | 维持保证金 |
| snapshot_time | DateTime64(3) | 快照时间 |
| source | Enum8('ws'=1, 'rest'=2) | 数据来源 |

分区：`toYYYYMMDD(snapshot_time)` | 排序：`(account_id, asset, snapshot_time)` | TTL：30 天

#### metrics_history — 指标历史（趋势分析 + 回溯）

| 字段 | 类型 | 说明 |
|------|------|------|
| metric_key | String | 指标名 |
| account_id | String | 账户 ID |
| symbol | LowCardinality(String) | 交易对（可空） |
| value | Float64 | 指标值 |
| timestamp | DateTime64(3) | 时间 |
| labels | Map(String, String) | 附加标签 |

分区：`toYYYYMMDD(timestamp)` | 排序：`(metric_key, account_id, timestamp)` | TTL：90 天

#### risk_events_archive — 风险事件归档（从 MySQL 同步）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | String | 事件 ID |
| event_code | LowCardinality(String) | 规则编号 |
| level | Enum8('P0'=0, 'P1'=1, 'P2'=2, 'P3'=3) | 事件级别 |
| status | LowCardinality(String) | 事件状态 |
| object_type | LowCardinality(String) | 对象类型 |
| object_id | String | 对象 ID |
| project_id | String | 项目 ID |
| title | String | 标题 |
| details | String | 详情 |
| created_at | DateTime64(3) | 创建时间 |

分区：`toYYYYMM(created_at)` | 排序：`(event_code, created_at)` | TTL：无（永久保留）

### 3.2 MVP 扩展（D4 新增，审计 + Evolve 数据基座）

#### orders — 订单全量记录

> 来自 WS USER_DATA ORDER_TRADE_UPDATE，每次状态变更都写入（不聚合）

| 字段 | 类型 | 说明 |
|------|------|------|
| event_time | DateTime64(3) | 交易所事件时间 |
| received_at | DateTime64(3) | 风控系统收到时间 |
| account_id | String | 账户 ID |
| project_id | String | 项目 ID |
| team_id | String | 团队 ID |
| strategy_id | String | 策略 ID |
| exchange_id | LowCardinality(String) | 交易所 |
| symbol | LowCardinality(String) | 交易对 |
| order_id | String | 交易所订单 ID |
| client_order_id | String | 客户端订单 ID |
| side | Enum8('BUY'=1, 'SELL'=2) | 方向 |
| order_type | LowCardinality(String) | 订单类型 (LIMIT/MARKET/STOP_MARKET/...) |
| time_in_force | LowCardinality(String) | 有效期 (GTC/IOC/FOK/GTD) |
| status | LowCardinality(String) | 状态 (NEW/PARTIALLY_FILLED/FILLED/CANCELED/...) |
| price | Decimal64(8) | 委托价 |
| orig_qty | Decimal64(8) | 委托量 |
| executed_qty | Decimal64(8) | 已成交量 |
| cumulative_quote_qty | Decimal64(8) | 累计成交额 |
| avg_price | Decimal64(8) | 均价 |
| reduce_only | UInt8 | 只减仓标记 |
| close_position | UInt8 | 关闭仓位标记 |
| position_side | LowCardinality(String) | 持仓方向 (BOTH/LONG/SHORT) |
| stop_price | Decimal64(8) | 止损价 |
| working_type | LowCardinality(String) | 触发方式 (MARK_PRICE/CONTRACT_PRICE) |
| commission | Decimal64(8) | 手续费 |
| commission_asset | LowCardinality(String) | 手续费资产 |
| is_maker | UInt8 | 是否 maker |
| update_id | UInt64 | 交易所推送序号（用于连续性检查） |

分区：`toYYYYMM(event_time)` | 排序：`(account_id, symbol, event_time, order_id)` | TTL：2 年

#### fills — 成交全量记录

> 来自 WS USER_DATA fills / trade updates

| 字段 | 类型 | 说明 |
|------|------|------|
| event_time | DateTime64(3) | 成交时间 |
| received_at | DateTime64(3) | 收到时间 |
| account_id | String | 账户 ID |
| project_id | String | 项目 ID |
| team_id | String | 团队 ID |
| strategy_id | String | 策略 ID |
| exchange_id | LowCardinality(String) | 交易所 |
| symbol | LowCardinality(String) | 交易对 |
| trade_id | String | 交易所成交 ID |
| order_id | String | 关联订单 ID |
| side | Enum8('BUY'=1, 'SELL'=2) | 方向 |
| price | Decimal64(8) | 成交价 |
| qty | Decimal64(8) | 成交量 |
| quote_qty | Decimal64(8) | 成交额 |
| realized_pnl | Decimal64(8) | 本笔实现盈亏 |
| commission | Decimal64(8) | 手续费 |
| commission_asset | LowCardinality(String) | 手续费资产 |
| position_side | LowCardinality(String) | 持仓方向 |
| is_maker | UInt8 | 是否 maker |
| buyer_order_id | String | 买方订单 ID |
| seller_order_id | String | 卖方订单 ID |

分区：`toYYYYMM(event_time)` | 排序：`(account_id, symbol, event_time, trade_id)` | TTL：2 年

#### ledger_entries — 资金流水

> 来自 WS ACCOUNT_UPDATE 中的 balance changes

| 字段 | 类型 | 说明 |
|------|------|------|
| event_time | DateTime64(3) | 事件时间 |
| received_at | DateTime64(3) | 收到时间 |
| account_id | String | 账户 ID |
| project_id | String | 项目 ID |
| exchange_id | LowCardinality(String) | 交易所 |
| event_reason | LowCardinality(String) | 变动原因 (DEPOSIT/WITHDRAW/ORDER/FUNDING_FEE/...) |
| asset | LowCardinality(String) | 资产类型 |
| balance_delta | Decimal64(8) | 变动金额 |
| wallet_balance | Decimal64(8) | 变动后余额 |
| cross_wallet_balance | Decimal64(8) | 全仓余额 |

分区：`toYYYYMM(event_time)` | 排序：`(account_id, asset, event_time)` | TTL：2 年

---

## 四、Redis Key 设计

| Key 模式 | 类型 | 说明 | TTL |
|---------|------|------|-----|
| `position:{account_id}:{symbol}` | Hash | 实时仓位快照 | 无（覆盖更新） |
| `balance:{account_id}:{asset}` | Hash | 实时余额 | 无 |
| `freshness:{account_id}:{dim}` | String | 数据新鲜度时间戳 (dim=trade/position/balance/order) | 无 |
| `metrics:{account_id}:last_activity` | String | 最后活跃时间 | 无 |
| `trade:volume:{account_id}:{symbol}` | SortedSet | 滑动窗口成交量 | 自动清理旧成员 |
| `dedup:risk_event:{event_hash}` | String | 事件去重 | 配置化 TTL |
| `ratelimit:{account_id}:api_weight` | String | API 频率限制当前权重 | 1 分钟 |
| `blast_radius:{action_type}` | List | 令牌桶滑动窗口 | 5 分钟 |

---

## 五、Kafka Topics

| Topic | 生产者 | 消费者 | 说明 |
|-------|--------|--------|------|
| risk_trade_events | Exchange Ingestor | Metrics Engine | 成交事件 |
| risk_position_snapshots | Exchange Ingestor | Metrics Engine | 仓位快照 |
| risk_balance_snapshots | Exchange Ingestor | Metrics Engine | 余额快照 |
| risk_account_updates | Exchange Ingestor | Risk Engine | 账户更新 |
| risk_risk_events | Alert Engine | Notifier, Executor, API WS | 风险事件 |
| risk_permission_checks | REST Reconciler | Alert Engine | 权限检查结果 |
| private_orders | Exchange Ingestor | Private Data Sink | 全量订单（D4+） |
| private_fills | Exchange Ingestor | Private Data Sink | 全量成交（D4+） |
| private_ledger | Exchange Ingestor | Private Data Sink | 资金流水（D4+） |

---

## 六、数据流向图

```
                          ┌──────────────┐
                          │  Binance WS  │
                          └──────┬───────┘
                                 │
                    ┌────────────▼────────────┐
                    │   Exchange Ingestor     │
                    └─┬──────┬──────┬────────┘
                      │      │      │
            ┌─────────▼┐ ┌──▼──┐ ┌─▼──────────┐
            │  Kafka    │ │Redis│ │  Kafka      │
            │ exchange.*│ │(新鲜│ │ private.*   │
            └─────┬─────┘ │度)  │ └──────┬──────┘
                  │       └──┬──┘        │
        ┌─────────▼───────┐  │  ┌────────▼────────┐
        │  Metrics Engine │──┘  │ Private Data Sink│
        │  (写 Redis)     │     │ (写 ClickHouse)  │
        └─────────┬───────┘     └─────────────────┘
                  │
        ┌─────────▼──────────┐
        │   Alert Engine     │
        │ Resolver→Rules→    │
        │ Dedup→Arbitrator   │
        └──┬─────────┬───────┘
           │         │
    ┌──────▼──┐  ┌───▼──────────┐
    │ MySQL   │  │ Kafka        │
    │risk_    │  │risk_risk_    │
    │events   │  │events        │
    └─────────┘  └──┬────┬──────┘
                    │    │
          ┌─────────▼┐ ┌─▼──────────────┐
          │ Notifier │ │Control Executor│
          │(Telegram)│ │(R0-R3 动作)    │
          └──────────┘ └────────────────┘
```
