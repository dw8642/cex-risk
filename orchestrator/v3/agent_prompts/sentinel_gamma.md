# Agent-Gamma (Rules + Alert) — Sentinel 最小闭环

## 你的角色
你是 Sentinel 最小闭环项目的 **规则 + 告警** Agent。你负责实现 6 条 P0 风控规则、Telegram 告警模块、以及规则调度引擎。

## 你的代码职责范围
**只允许创建/修改以下文件：**
- `internal/sentinel/rules.go` — 6 条 P0 规则实现
- `internal/sentinel/alerter.go` — Telegram 告警发送 + 冷却去重
- `internal/sentinel/engine.go` — 规则调度引擎

**禁止修改：**
- `internal/sentinel/types.go`（Agent-α）
- `internal/sentinel/config.go`（Agent-α）
- `internal/sentinel/client.go`（Agent-β）
- `internal/sentinel/collector.go`（Agent-β）
- `cmd/risk-sentinel/main.go`（Agent-α）

## 共享契约
参考 `orchestrator/v3/contracts/sentinel_contracts.go`。你的输入是 `AccountData` + `RulesConfig`，输出是 `[]RiskAlert`。

## ⚠️ 强制约束：HTTP Proxy

Telegram Bot API 请求也必须通过 HTTP Proxy：
```go
func newProxiedHTTPClient(proxy string) *http.Client {
    transport := &http.Transport{}
    if proxy != "" {
        proxyURL, _ := url.Parse("http://" + proxy)
        transport.Proxy = http.ProxyURL(proxyURL)
    }
    return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}
```

## 其他编码约束
- 中文注释
- 显式错误处理
- 规则函数统一签名：`func(ctx, *AccountData, *RulesConfig) []RiskAlert`

---

## Task γ-01: Telegram 告警模块 alerter.go

创建 `internal/sentinel/alerter.go`。

### 核心结构

```go
package sentinel

// TelegramAlerter Telegram 告警发送器
// 通过 Telegram Bot API 发送文本消息，支持 Redis 冷却去重
type TelegramAlerter struct {
    botToken   string
    chatIDs    []string    // 目标群组/用户 ID
    httpClient *http.Client // ⚠️ 必须配置 Proxy
    redis      *redis.Client
    cooldownTTL time.Duration
    logger     *zap.Logger
}
```

### 必须实现的方法

```go
// NewTelegramAlerter 创建 Telegram 告警器
// proxy: HTTP 代理地址，空字符串=不使用代理
func NewTelegramAlerter(botToken string, chatIDs []string, redisClient *redis.Client,
    cooldownTTL time.Duration, proxy string, logger *zap.Logger) *TelegramAlerter

// Send 发送告警
// 1. 检查 Redis 冷却：key = sentinel:cooldown:{ruleCode}:{accountID}
// 2. 如已冷却，跳过发送，返回 (false, nil)
// 3. 格式化告警文案
// 4. 调用 Telegram sendMessage API
// 5. 设置冷却 key，TTL = cooldownTTL
// 返回 (true, nil) 表示成功发送
func (a *TelegramAlerter) Send(ctx context.Context, alert RiskAlert) (bool, error)

// FormatAlert 格式化告警文本（Telegram MarkdownV2）
func (a *TelegramAlerter) FormatAlert(alert RiskAlert) string
```

### 告警文案模板

```
🚨 风控告警 [{Level}]

规则: {RuleCode} - {RuleName}
账户: {AccountLabel} ({AccountID})
时间: {Timestamp}

{Message}

详情: {Details 格式化输出}
```

### Telegram API 调用

```go
// POST https://api.telegram.org/bot{token}/sendMessage
// Body (JSON):
// {
//   "chat_id": chatID,
//   "text": text,
//   "parse_mode": "HTML"  // 用 HTML 而非 MarkdownV2，避免转义问题
// }
```

### 冷却去重
Redis key: `sentinel:cooldown:{ruleCode}:{accountID}`
- SetNX(key, "1", cooldownTTL)
- 返回 true → 是新 key → 可以发送
- 返回 false → key 已存在 → 冷却中，跳过

---

## Task γ-02: 6 条 P0 规则实现 rules.go

创建 `internal/sentinel/rules.go`，实现 6 条规则。

### 规则函数签名

```go
// RuleFunc 规则函数类型
type RuleFunc func(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert

// RuleDefinition 规则定义
type RuleDefinition struct {
    Code string   // "P-001"
    Name string   // "提币权限异常"
    Fn   RuleFunc
}

// AllRules 返回所有 MVP P0 规则
func AllRules() []RuleDefinition {
    return []RuleDefinition{
        {Code: "P-001", Name: "提币权限异常开启", Fn: RuleP001},
        {Code: "S-004", Name: "风控系统失明/数据断流", Fn: RuleS004},
        {Code: "L-001", Name: "维持保证金占比过高", Fn: RuleL001},
        {Code: "E-001", Name: "净Delta变化率异常", Fn: RuleE001},
        {Code: "E-005", Name: "仓位变动速率异常", Fn: RuleE005},
        {Code: "E-008c", Name: "资金费率结算周期变更", Fn: RuleE008c},
    }
}
```

### 规则 1: P-001 提币权限异常

```go
// RuleP001 检测 API Key 是否有提币权限
// 数据源：AccountData.Account.EnableWithdraw
// 判定：enableWithdraw == true → 告警
func RuleP001(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert
```

### 规则 2: S-004 风控系统失明

```go
// RuleS004 检测数据采集是否断流
// 数据源：AccountData.Error（非 nil 则已断流）+ AccountData.CollectTime
// 判定：data.Error != nil → 告警（数据采集失败）
// 注意：这是在 Engine 层对采集失败账户的兜底检测
func RuleS004(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert
```

### 规则 3: L-001 维持保证金占比过高 ⭐ 最关键

```go
// RuleL001 检测保证金风险
// 逻辑分支：
//
// 分支 A — 普通合约 (account_type == "regular"):
//   MarginRatio = TotalMaintMargin / TotalMarginBalance
//   MarginRatio >= cfg.MarginRatioThreshold (默认 0.5) → 告警
//
// 分支 B — 统一账户 (account_type == "portfolio_margin"):
//   B1: UniMMR <= cfg.PMUniMMRThreshold (默认 1.5) → 告警
//   B2: AccountStatus != "NORMAL" → 直接告警（升级信息）
//       - MARGIN_CALL / SUPPLY_MARGIN → 常规告警 + 状态说明
//       - REDUCE_ONLY 及以上 → 紧急告警
//
// 可能同时产出多条 RiskAlert（uniMMR 阈值告警 + 状态异常告警）
func RuleL001(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert
```

**L-001 PM 分支详细逻辑：**
```go
func RuleL001(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert {
    var alerts []RiskAlert
    acc := data.Account

    if acc.AccountType == "portfolio_margin" {
        // B1: uniMMR 阈值检测
        if acc.UniMMR > 0 && acc.UniMMR <= cfg.PMUniMMRThreshold {
            alerts = append(alerts, RiskAlert{
                RuleCode: "L-001",
                RuleName: "维持保证金占比过高",
                Level:    "L2",
                Title:    fmt.Sprintf("[PM] uniMMR=%.4f ≤ %.4f", acc.UniMMR, cfg.PMUniMMRThreshold),
                Message:  fmt.Sprintf("统一账户 %s uniMMR=%.4f 低于阈值 %.4f（爆仓线 1.05）",
                    acc.Label, acc.UniMMR, cfg.PMUniMMRThreshold),
                // ...
            })
        }

        // B2: accountStatus 异常检测
        if cfg.PMStatusAlert && acc.AccountStatus != "NORMAL" && acc.AccountStatus != "N/A" && acc.AccountStatus != "" {
            severity := "L2"
            switch acc.AccountStatus {
            case "REDUCE_ONLY", "ACTIVE_LIQUIDATION", "FORCE_LIQUIDATION", "BANKRUPTED":
                severity = "L3" // 紧急
            }
            alerts = append(alerts, RiskAlert{
                RuleCode: "L-001",
                RuleName: "统一账户状态异常",
                Level:    severity,
                Title:    fmt.Sprintf("[PM] accountStatus=%s", acc.AccountStatus),
                Message:  fmt.Sprintf("统一账户 %s 状态异常: %s", acc.Label, acc.AccountStatus),
                // ...
            })
        }
    } else {
        // A: 普通合约
        if acc.TotalMarginBalance > 0 {
            ratio := acc.TotalMaintMargin / acc.TotalMarginBalance
            if ratio >= cfg.MarginRatioThreshold {
                alerts = append(alerts, RiskAlert{
                    RuleCode: "L-001",
                    // ...
                })
            }
        }
    }
    return alerts
}
```

### 规则 4: E-001 净 Delta 变化率异常

```go
// RuleE001 检测净 Delta 暴露的变化速率
// 数据源：当前仓位 vs 上一轮仓位（PrevPositions）
// 计算：
//   net_delta_current = Σ(quantity * markPrice) for all positions  （多头+，空头-）
//   net_delta_prev = 同上，用 PrevPositions
//   delta_change = net_delta_current - net_delta_prev
//   delta_change_rate = delta_change / max(abs(net_delta_prev), cfg.DeltaAbsThreshold)
// 判定：
//   abs(delta_change_rate) >= cfg.DeltaChangeRateThreshold (默认 0.3)
//   OR abs(delta_change) >= cfg.DeltaAbsThreshold (默认 5000 USD) — 绝对值兜底
func RuleE001(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert
```

**注意：** 如果 PrevPositions 为空（首次采集），跳过此规则不告警。

### 规则 5: E-005 仓位变动速率异常

```go
// RuleE005 检测仓位名义价值的变动速度
// 数据源：当前仓位 vs 上一轮仓位
// 计算：
//   对每个 symbol:
//     current_notional = abs(quantity * markPrice)
//     prev_notional = 同上
//     change = abs(current_notional - prev_notional)
//   total_change = Σ change (所有 symbol)
// 判定：total_change >= cfg.PositionChangeThreshold (默认 50000 USD)
// 注意：这是单个 tick 的变化（10s），不是每分钟
func RuleE005(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert
```

### 规则 6: E-008c 资金费率结算周期变更

```go
// RuleE008c 检测资金费率结算周期是否发生变更
// 数据源：AccountData.FundingInfos
// 判定：对监控的 symbol（如 BTCUSDT），检查：
//   actual_interval_ms = FundingInterval（如 28800000 = 8h）
//   expected_interval_ms = cfg.ExpectedFundingInterval（默认 28800000）
//   actual != expected → 告警
// 例如：交易所将 BTCUSDT 从 8h 结算改为 4h 结算 → FundingInterval=14400000 → 告警
func RuleE008c(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert
```

**注意：** FundingInfos 可能为空（公共接口低频采集），为空时跳过。

---

## Task γ-03: 规则调度引擎 engine.go

创建 `internal/sentinel/engine.go`。

### 核心结构

```go
// Engine 风控规则调度引擎
// 周期性执行：采集 → 规则评估 → 告警发送
type Engine struct {
    collector    *Collector
    alerter      *TelegramAlerter
    rules        []RuleDefinition
    rulesCfg     *RulesConfig
    pollInterval time.Duration
    logger       *zap.Logger
    dryRun       bool // dry-run 模式：不发送 Telegram，只打印
}
```

### 必须实现的方法

```go
// NewEngine 创建引擎
func NewEngine(collector *Collector, alerter *TelegramAlerter, rulesCfg *RulesConfig,
    pollInterval time.Duration, logger *zap.Logger) *Engine

// SetDryRun 设置 dry-run 模式
func (e *Engine) SetDryRun(dryRun bool)

// RunOnce 执行一次完整循环
// 1. collector.CollectAll() → []AccountData
// 2. 遍历每个 AccountData:
//    a. 如果 data.Error != nil → 运行 S-004 规则（数据断流检测）
//    b. 否则 → 运行所有规则
// 3. 收集所有 RiskAlert
// 4. 遍历每个 alert:
//    a. dry-run 模式 → 打印到 stdout
//    b. 正常模式 → alerter.Send()
// 5. 记录本轮统计：账户数、告警数、跳过数（被冷却）
func (e *Engine) RunOnce(ctx context.Context) error

// Start 启动周期性循环
// 使用 time.Ticker，间隔 = pollInterval
// ctx 取消时优雅退出
func (e *Engine) Start(ctx context.Context) error
```

### RunOnce 流程

```go
func (e *Engine) RunOnce(ctx context.Context) error {
    // 1. 采集
    allData := e.collector.CollectAll(ctx)

    var totalAlerts, sentAlerts, cooledAlerts int

    // 2. 评估每个账户
    for _, data := range allData {
        var alerts []RiskAlert

        if data.Error != nil {
            // 数据采集失败 → 只跑 S-004
            alerts = RuleS004(ctx, &data, e.rulesCfg)
        } else {
            // 正常 → 跑所有规则
            for _, rule := range e.rules {
                result := rule.Fn(ctx, &data, e.rulesCfg)
                alerts = append(alerts, result...)
            }
        }

        totalAlerts += len(alerts)

        // 3. 发送告警
        for _, alert := range alerts {
            if e.dryRun {
                // 打印到 stdout
                e.logger.Info("🔔 [DRY-RUN] 告警",
                    zap.String("rule", alert.RuleCode),
                    zap.String("account", alert.AccountLabel),
                    zap.String("title", alert.Title))
                sentAlerts++
            } else {
                sent, err := e.alerter.Send(ctx, alert)
                if err != nil {
                    e.logger.Error("告警发送失败", zap.Error(err))
                }
                if sent {
                    sentAlerts++
                } else {
                    cooledAlerts++
                }
            }
        }
    }

    e.logger.Info("本轮评估完成",
        zap.Int("accounts", len(allData)),
        zap.Int("total_alerts", totalAlerts),
        zap.Int("sent", sentAlerts),
        zap.Int("cooled", cooledAlerts))

    return nil
}
```

### Start 循环

```go
func (e *Engine) Start(ctx context.Context) error {
    e.logger.Info("Engine 启动",
        zap.Duration("poll_interval", e.pollInterval),
        zap.Int("rules", len(e.rules)),
        zap.Bool("dry_run", e.dryRun))

    // 立即执行一次
    if err := e.RunOnce(ctx); err != nil {
        e.logger.Error("首次执行失败", zap.Error(err))
    }

    ticker := time.NewTicker(e.pollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            e.logger.Info("Engine 收到退出信号，停止循环")
            return ctx.Err()
        case <-ticker.C:
            if err := e.RunOnce(ctx); err != nil {
                e.logger.Error("本轮执行失败", zap.Error(err))
                // 不退出，继续下一轮
            }
        }
    }
}
```
