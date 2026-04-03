# Day-1 最小闭环开发方案

## 0. 目标与约束

| 项 | 值 |
|---|---|
| 目标 | 今天跑通一个最小闭环：采集 → 规则判定 → Telegram 告警 |
| 交易所 | Binance Futures（1 个） |
| 币种 | BTCUSDT（1 个） |
| 账户 | 5 个（含普通合约 + 统一账户 Portfolio Margin） |
| Dashboard | 不需要 |
| 告警级别 | 全部统一 L2（Telegram 文本消息） |
| 规则数 | 7 条 P0（2 条已有 + 5 条新写） |
| 规则清单 | P-001, S-004, L-001, L-001pm, E-001, E-005, E-008c |

---

## 1. 架构设计：单进程闭环

**不走微服务管线**，用一个 `cmd/risk-sentinel/main.go` 单进程搞定，绕开 Kafka/ClickHouse 等重依赖，只依赖 **Redis**（缓存 + 冷却去重）。

```
┌──────────────────────────────────────────────────────────────┐
│                    risk-sentinel (单进程)                       │
│                                                                │
│  ┌───────────────┐    ┌──────────┐    ┌──────────┐            │
│  │   Collector    │───▶│  Redis   │◀───│  Rules   │            │
│  │ (REST轮询)     │    │ (缓存层)  │    │ (规则引擎) │            │
│  │               │    └──────────┘    └────┬─────┘            │
│  │  ┌──────────┐ │                         │                   │
│  │  │ Regular  │ │    每10s轮询             │ RiskEvent          │
│  │  │ /fapi/   │ │    5个账户               ▼                   │
│  │  ├──────────┤ │                    ┌──────────┐            │
│  │  │ PortMgn  │ │                    │ Alerter  │            │
│  │  │ /papi/   │ │                    │ (去重+TG) │            │
│  │  └──────────┘ │                    └──────────┘            │
│  └───────────────┘                                            │
│  Binance REST API                      Telegram Bot API       │
└──────────────────────────────────────────────────────────────┘

依赖：Redis (已有 docker-compose)
不需要：MySQL / Kafka / ClickHouse（今天不用）
```

### 为什么不走已有的 Ingestor + Kafka 管线？

已有管线是 WS 驱动的事件流架构，适合生产环境。但今天的目标是 **最快跑通闭环**：
- REST 轮询足够（10s 间隔，5 账户 × 1 API 调用 = 50次/分钟，远低于频控）
- 避免启动 Kafka/Zookeeper/MySQL 全套基础设施
- 单进程便于调试，出了问题一个日志全看完
- 规则跑通后，后续很容易把 Collector 替换为 Kafka Consumer

---

## 1.1 统一账户 (Portfolio Margin) 支持

### 背景

币安统一账户（Portfolio Margin）与普通合约账户使用不同的 API 前缀和数据结构。我们的 5 个账户中可能混合存在两种类型，需要自动适配。

### 账户类型检测

配置层指定 `account_type`，Collector 根据类型选择对应的 API 端点：

| 账户类型 | account_type | API 前缀 | 核心保证金指标 |
|---------|-------------|----------|-------------|
| 普通合约 | `regular` | `/fapi/v2/` | totalMaintMargin / totalMarginBalance |
| 统一账户 | `portfolio_margin` | `/papi/v1/` | `uniMMR`（统一维持保证金率） |

### API 端点映射

| 数据需求 | 普通合约 (`/fapi/`) | 统一账户 (`/papi/`) |
|---------|-------------------|-------------------|
| 账户信息 | `GET /fapi/v2/account` | `GET /papi/v1/account` |
| 仓位信息 | `GET /fapi/v2/positionRisk` | `GET /papi/v1/um/positionRisk` |
| 余额 | `/fapi/v2/account` (Assets 字段) | `GET /papi/v1/balance` |
| API 权限 | `GET /sapi/v1/account/apiRestrictions` | 同左（共用） |
| 资金费率 | `GET /fapi/v1/fundingInfo` | 同左（公共接口） |

### 统一账户关键字段

```go
// Portfolio Margin 账户响应 (/papi/v1/account)
type binancePMAccountResp struct {
    UniMMR             string `json:"uniMMR"`             // 统一维持保证金率，>1.2 健康，1.05 爆仓
    AccountEquity      string `json:"accountEquity"`      // 账户权益 (USD)
    ActualEquity       string `json:"actualEquity"`       // 实际权益
    AccountInitialMargin string `json:"accountInitialMargin"` // 初始保证金
    AccountMaintMargin string `json:"accountMaintMargin"` // 维持保证金
    AccountStatus      string `json:"accountStatus"`      // 账户状态（见下方）
}

// accountStatus 状态机（越往后越危险）：
// NORMAL → MARGIN_CALL → SUPPLY_MARGIN → REDUCE_ONLY →
// ACTIVE_LIQUIDATION → FORCE_LIQUIDATION → BANKRUPTED
```

### L-001 规则分支逻辑

| 账户类型 | 判定方式 | 阈值 |
|---------|---------|------|
| 普通合约 | `totalMaintMargin / totalMarginBalance >= threshold` | 默认 0.5 (50%) |
| 统一账户 | `uniMMR <= threshold`（注意方向相反，越小越危险） | 默认 1.5（对应约 66% 保证金占用） |

> 统一账户额外福利：`accountStatus` 字段天然是一个免费的风控信号。当状态不是 NORMAL 时，直接触发 L3 级别告警。这在 L-001 规则内以子检查实现，无需新增规则。

### Collector 适配器设计

```go
// AccountAdapter 统一接口，屏蔽普通/PM 差异
type AccountAdapter interface {
    GetAccountInfo(ctx context.Context) (*AccountInfo, error)   // 余额+保证金
    GetPositions(ctx context.Context) ([]PositionInfo, error)   // 仓位列表
}

// AccountInfo 统一输出结构
type AccountInfo struct {
    AccountType     string  // "regular" | "portfolio_margin"
    MarginRatio     float64 // 普通: maintMargin/marginBalance; PM: 1/uniMMR
    UniMMR          float64 // PM 专用，普通账户为 0
    AccountStatus   string  // PM 专用，普通账户为 "N/A"
    TotalEquity     float64 // 账户总权益 (USD)
    AvailableMargin float64 // 可用保证金
    MaintMargin     float64 // 维持保证金
    // ... 其他字段
}
```

Collector 初始化时根据 `account_type` 配置创建对应的 adapter，后续 Rules 层通过 `AccountInfo.AccountType` 字段做逻辑分支。

---

## 2. 实现的 7 条 P0 规则

| # | 编号 | 名称 | 状态 | 数据源 | 判定逻辑 | 预计工时 |
|---|------|------|------|--------|---------|---------|
| 1 | **P-001** | 提币权限异常开启 | ✅ 已实现 | REST /sapi/v1/account/apiRestrictions | enable_withdraw == true | 0 (重用) |
| 2 | **S-004** | 风控系统失明 | ✅ 已实现 (s013.go) | Redis freshness 时间戳 | data_gap > 30s | 10min (仅重命名) |
| 3 | **L-001** | 维持保证金占比过高 | 🔲 新写 | 普通: /fapi/v2/account; PM: /papi/v1/account | 普通: maintMargin/marginBalance >= 0.5; PM: uniMMR <= 1.5 **且** accountStatus != NORMAL 直接 L3 | 45min |
| 4 | **E-001** | 净 Delta 变化率异常 | 🔲 新写 | Redis 前后两次 position 快照差值 | abs(delta_change_rate) >= threshold | 40min |
| 5 | **E-005** | 仓位变动速率异常 | 🔲 新写 | Redis 前后两次 position 快照差值 | abs(Δ position_usd) / window >= threshold | 45min |
| 6 | **E-008c** | 资金费率结算周期变更 | 🔲 新写 | REST /fapi/v1/fundingInfo | current_interval != expected_interval | 30min |

> 总计：2 条重用 + 4 条新写 ≈ 3 小时规则开发（L-001 增加 PM 分支逻辑，工时+15min）

---

## 3. 需要补充的 Binance REST 数据

### 3.1 普通合约账户

现有 `binanceAccountResp` 缺少账户级字段，需补充：

```go
// binanceAccountResp 补充账户级保证金字段
type binanceAccountResp struct {
    TotalMaintMargin  string `json:"totalMaintMargin"`   // L-001 需要
    TotalMarginBalance string `json:"totalMarginBalance"` // L-001 需要
    TotalWalletBalance string `json:"totalWalletBalance"` // 备用
    AvailableBalance   string `json:"availableBalance"`   // 备用
    // ... 原有 Assets / Positions
}
```

### 3.2 统一账户 (Portfolio Margin)

新增响应结构：

```go
// binancePMAccountResp - /papi/v1/account 响应
type binancePMAccountResp struct {
    UniMMR               string `json:"uniMMR"`
    AccountEquity        string `json:"accountEquity"`
    ActualEquity         string `json:"actualEquity"`
    AccountInitialMargin string `json:"accountInitialMargin"`
    AccountMaintMargin   string `json:"accountMaintMargin"`
    AccountStatus        string `json:"accountStatus"`
}

// binancePMPositionResp - /papi/v1/um/positionRisk 响应
type binancePMPositionResp struct {
    Symbol           string `json:"symbol"`
    PositionAmt      string `json:"positionAmt"`
    EntryPrice       string `json:"entryPrice"`
    MarkPrice        string `json:"markPrice"`
    UnRealizedProfit string `json:"unRealizedProfit"`
    LiquidationPrice string `json:"liquidationPrice"`
    Leverage         string `json:"leverage"`
    PositionSide     string `json:"positionSide"`
}
```

### 3.3 新增 REST 端点汇总

| 端点 | 账户类型 | 用途 | 规则 |
|------|---------|------|------|
| `GET /fapi/v1/fundingInfo` | 公共 | 含 fundingInterval | E-008c |
| `GET /papi/v1/account` | PM | 账户信息 + uniMMR + accountStatus | L-001 |
| `GET /papi/v1/um/positionRisk` | PM | U本位仓位 | E-001, E-005 |
| `GET /papi/v1/balance` | PM | 各钱包余额 | 备用 |

需给 adapter 新增方法：
- `GetFundingInfo(ctx) → [{symbol, fundingInterval, ...}]`
- `GetPMAccountInfo(ctx) → binancePMAccountResp`
- `GetPMPositions(ctx) → []binancePMPositionResp`

---

## 4. 新增模块清单

```
cex-risk/
├── cmd/
│   └── risk-sentinel/
│       └── main.go              # 【新】单进程入口
├── internal/
│   └── sentinel/
│       ├── collector.go         # 【新】REST 轮询采集器（含 PM adapter 分支）
│       ├── engine.go            # 【新】规则调度引擎
│       ├── alerter.go           # 【新】Telegram 告警发送 + 冷却去重
│       └── types.go             # 【新】AccountInfo / PositionInfo 统一结构体
├── services/risk-engine/alert/rules/
│   ├── rule.go                  # 已有 (Rule 接口 + Registry)
│   ├── p001.go                  # 已有
│   ├── s013.go → s004.go        # 重命名
│   ├── l001.go                  # 【新】维持保证金占比（含 PM uniMMR + accountStatus）
│   ├── e001.go                  # 【新】净 Delta 变化率
│   ├── e005.go                  # 【新】仓位变动速率
│   └── e008c.go                 # 【新】结算周期变更
├── pkg/exchange/
│   ├── binance_futures.go       # 【改】补充 GetFundingInfo() + 账户级字段
│   └── binance_pm.go            # 【新】Portfolio Margin adapter
└── config.toml                  # 【改】添加 [[accounts]]（含 account_type）和 [rules]
```

---

## 5. 分阶段实施计划

### Phase 0 — 环境准备（30 min）

- [ ] `docker-compose up -d redis` （只起 Redis）
- [ ] 在 `config.toml` 中配置 5 个账户的 API Key
- [ ] 新增 `[accounts]` 配置段（替代 MySQL 查询，今天不用 MySQL）

```toml
# config.toml 新增
[[accounts]]
id = "acc-01"
exchange = "binance_futures"
account_type = "portfolio_margin"   # "regular" | "portfolio_margin"
api_key = "xxx"
secret_key = "xxx"
label = "主力1-PM"

[[accounts]]
id = "acc-02"
exchange = "binance_futures"
account_type = "regular"
api_key = "xxx"
secret_key = "xxx"
label = "策略2"

# ... 共 5 个，account_type 按实际情况填写
```

### Phase 1 — Telegram 告警模块（30 min）

`internal/sentinel/alerter.go`：
- `SendAlert(event RiskEvent)` → 格式化文案 → HTTP POST Telegram Bot API
- Redis 冷却去重：`cooldown:{rule_code}:{account_id}`，TTL 300s
- 文案格式：`【严重报警】【{rule_code}】{message}，请尽快处理`
- 使用现有 `config.toml` 中的 `telegram.bot_token` 和 `telegram.chat_ids`

### Phase 2 — REST 数据采集器（60 min）

`internal/sentinel/collector.go`：
- 加载 5 个账户配置 → 根据 `account_type` 为每个账户创建对应 adapter
- **普通合约 (regular)**：每 10s 调用 `/fapi/v2/account` + `/fapi/v2/positionRisk`
- **统一账户 (portfolio_margin)**：每 10s 调用 `/papi/v1/account` + `/papi/v1/um/positionRisk`
- **公共接口**（两种账户共用）：
  - `/fapi/v1/fundingInfo` → 结算周期（低频，5min 一次）
  - `/sapi/v1/account/apiRestrictions` → 权限（低频，60s 一次）
- 写入 Redis（统一 key 格式，规则层不感知账户类型差异）：
  - `sentinel:account:{id}` → JSON（统一结构 AccountInfo，含 account_type / margin_ratio / uniMMR / account_status）
  - `sentinel:positions:{id}:{symbol}` → JSON（仓位快照，统一字段）
  - `sentinel:position_prev:{id}:{symbol}` → 上一轮快照（E-001/E-005 对比用）
  - `sentinel:freshness:{id}` → 最新采集时间戳
  - `sentinel:funding_info:{symbol}` → 结算周期信息

### Phase 3 — 4 条新规则实现（2.5 h）

**L-001 维持保证金占比过高** (`l001.go`, ~100 LOC)
```go
// 数据：Redis sentinel:account:{id}（含 AccountInfo 统一结构）
//
// 分支 1 — 普通合约 (account_type == "regular"):
//   判定：totalMaintMargin / totalMarginBalance >= threshold (默认 0.5 = 50%)
//
// 分支 2 — 统一账户 (account_type == "portfolio_margin"):
//   判定 A：uniMMR <= threshold (默认 1.5，越小越危险)
//   判定 B：accountStatus != "NORMAL" → 直接升级为 L3 告警
//           MARGIN_CALL/SUPPLY_MARGIN → L2 + 额外提示
//           REDUCE_ONLY 及以上 → L3 紧急告警
//
// 输出：RiskEvent{Code:"L-001", Severity:"L2"/"L3", Message:"..."}
```

**E-001 净 Delta 变化率异常** (`e001.go`, ~80 LOC)
```go
// 数据：Redis sentinel:positions:{id}:BTCUSDT (当前) vs sentinel:position_prev:{id}:BTCUSDT (上一轮)
// 净 Delta = Σ(position_qty * markPrice)，多空带方向
// 判定：abs(delta_now - delta_prev) / max(abs(delta_prev), min_denominator) >= threshold (默认 0.3 = 30%)
// 双兜底：变化率 + 绝对变化量（abs_threshold 默认 5000 USD）
```

**E-005 仓位变动速率异常** (`e005.go`, ~80 LOC)
```go
// 数据：Redis sentinel:positions:{id}:BTCUSDT (当前) vs sentinel:position_prev:{id}:BTCUSDT (上一轮)
// 判定：abs(current_qty - prev_qty) * markPrice >= threshold (默认 10000 USD / 分钟)
// 窗口：采集间隔即窗口（10s），阈值按此比例缩放
```

**E-008c 结算周期变更** (`e008c.go`, ~60 LOC)
```go
// 数据：Redis sentinel:funding_info:BTCUSDT
// 判定：actual_interval != expected_interval (配置值，默认 8h = 28800000ms)
// 首次写入 expected_interval，后续检测变更
```

### Phase 4 — 主程序串接（30 min）

`cmd/risk-sentinel/main.go`：
```go
func main() {
    // 1. 加载配置
    cfg := config.Load("config.toml")

    // 2. 初始化 Redis
    rdb := redis.NewClient(...)

    // 3. 初始化 Telegram Alerter
    alerter := sentinel.NewAlerter(cfg.Telegram, rdb)

    // 4. 初始化 Collector（5个账户）
    collector := sentinel.NewCollector(cfg.Accounts, rdb)

    // 5. 注册规则
    rules := []rules.Rule{P001, S004, L001, E001, E005, E008C}

    // 6. 主循环
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        collector.Poll(ctx)       // 采集数据写 Redis
        for _, account := range accounts {
            for _, rule := range rules {
                events := rule.Check(ctx, account, store)
                for _, evt := range events {
                    alerter.Send(evt)  // 去重 + 发 Telegram
                }
            }
        }
    }
}
```

### Phase 5 — 端到端测试（30 min）

- [ ] 启动 `risk-sentinel`，观察 REST 采集日志
- [ ] 手动触发告警测试（如设置极低的保证金阈值）
- [ ] 验证 Telegram 群收到告警消息
- [ ] 验证冷却去重生效（相同告警 5min 内不重复）
- [ ] 压力测试：5 个账户 × 6 条规则 = 30 次判定 / 10s，确认无超时

---

## 6. 时间线（约 6 小时）

```
09:00 ─ Phase 0 环境准备                  30 min
09:30 ─ Phase 1 Telegram 告警模块          30 min
10:00 ─ Phase 2 REST 采集器 + PM adapter   60 min  (+15min PM适配)
11:00 ─ Phase 3 规则实现 (4条, L-001含PM)  2.5 h  (+30min PM分支)
13:30 ─ 午休
14:15 ─ Phase 4 主程序串接                 30 min
14:45 ─ Phase 5 端到端测试 + 修bug         1 h
15:45 ─ ✅ 闭环跑通
```

---

## 7. 阈值初始配置

| 规则 | 参数 | 默认值 | 说明 |
|------|------|--------|------|
| L-001 | margin_ratio_threshold | 0.5 (50%) | 普通合约：维持保证金 / 保证金余额 |
| L-001 | pm_unimmr_threshold | 1.5 | 统一账户：uniMMR ≤ 此值告警（1.05 爆仓） |
| L-001 | pm_status_alert | true | 统一账户：accountStatus 非 NORMAL 时告警 |
| E-001 | delta_change_rate_threshold | 0.3 (30%) | 净Delta变化率 |
| E-001 | delta_abs_threshold | 5000 (USD) | 净Delta绝对变化量兜底 |
| E-005 | position_change_threshold | 50000 (USD) | 每分钟仓位变动上限 |
| E-008c | expected_funding_interval | 28800000 (ms) | 预期结算周期 8h |
| S-004 | data_gap_threshold | 30 (s) | 数据断流超时 |
| P-001 | — | enable_withdraw == true | 无阈值，二值判定 |
| 冷却 | cooldown_ttl | 300 (s) | 同一告警 5 分钟内不重复 |

> 阈值全部写在 `config.toml` 的 `[rules]` 段中，后续迁移到 MySQL 可配置。

---

## 8. 今天不做的事

| 项 | 原因 | 后续计划 |
|----|------|---------|
| Dashboard | 需要前后端联调，至少 2 天 | Sprint 2 |
| Kafka 管线 | 闭环不需要，REST 轮询够用 | 后续替换 Collector |
| MySQL 持久化 | 今天事件只走 Telegram，不入库 | Sprint 2 |
| ClickHouse 历史 | 同上 | Sprint 2 |
| WS 实时流 | REST 10s 够用，WS 调试复杂 | Sprint 2 |
| L3 语音通话 | 统一 L2 文本 | Sprint 2 |
| 多币种 | 今天只做 BTCUSDT | Sprint 2 扩展 |
| 受众分组 | 统一发一个群 | Sprint 2 |

---

## 9. 文件改动清单（给 Claude Agent 的执行清单）

```
【新建 9 个文件】
cmd/risk-sentinel/main.go
internal/sentinel/collector.go
internal/sentinel/engine.go
internal/sentinel/alerter.go
internal/sentinel/types.go                    — AccountInfo / PositionInfo 统一结构
pkg/exchange/binance_pm.go                    — Portfolio Margin adapter
services/risk-engine/alert/rules/l001.go
services/risk-engine/alert/rules/e001.go
services/risk-engine/alert/rules/e005.go
services/risk-engine/alert/rules/e008c.go

【修改 3 个文件】
pkg/exchange/binance_futures.go  — 补充 GetFundingInfo() + 账户级字段
services/risk-engine/alert/rules/s013.go → 重命名为 s004.go，Code() 返回 "S-004"
config.toml — 新增 [[accounts]]（含 account_type）和 [rules] 配置段

【测试文件（可选，时间允许再写）】
services/risk-engine/alert/rules/l001_test.go
services/risk-engine/alert/rules/e001_test.go
services/risk-engine/alert/rules/e005_test.go
```
