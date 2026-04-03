# Agent-Alpha (Foundation + Integration) — Sentinel 最小闭环

## 你的角色
你是 Sentinel 最小闭环项目的 **基座 + 集成** Agent。你负责：
1. 创建所有 Agent 依赖的共享类型和配置定义
2. 更新 config.toml 添加 sentinel 专用配置
3. 最终将所有模块串接到 main.go 并验证编译

## 你的代码职责范围
**只允许修改以下文件：**
- `internal/sentinel/types.go` — 共享数据结构
- `internal/sentinel/config.go` — 配置解析
- `cmd/risk-sentinel/main.go` — 主程序入口
- `config.toml` — 添加配置段

**禁止修改：**
- `internal/sentinel/client.go`（Agent-β）
- `internal/sentinel/collector.go`（Agent-β）
- `internal/sentinel/rules.go`（Agent-γ）
- `internal/sentinel/alerter.go`（Agent-γ）
- `internal/sentinel/engine.go`（Agent-γ）

## 共享契约
参考 `orchestrator/v3/contracts/sentinel_contracts.go` 中的完整定义。

## 强制约束
- **HTTP Proxy**: 所有 http.Client 必须支持 `cfg.System.HTTPProxy` 代理配置
- **中文注释**: 所有公共函数必须有中文文档注释
- **显式错误处理**: 禁止 `_ = someFunc()`，所有错误必须检查和包装

---

## Task α-01: 共享类型定义 types.go + config.go

### types.go — 所有 Agent 共享的数据结构

创建 `internal/sentinel/types.go`，包含以下类型（字段定义严格遵循契约）：

```go
package sentinel

// AccountInfo 统一账户信息（屏蔽普通/PM 差异）
type AccountInfo struct {
    AccountID        string
    AccountType      string   // "regular" | "portfolio_margin"
    Label            string
    MarginRatio      float64  // 统一语义：越大越危险
    UniMMR           float64  // PM 专用（越小越危险），普通=0
    AccountStatus    string   // PM 专用，普通="N/A"
    TotalEquity      float64
    AvailableMargin  float64
    TotalMaintMargin float64
    TotalMarginBalance float64
    EnableWithdraw   bool
    IPRestrict       bool
    UpdatedAt        time.Time
}

// PositionInfo 统一仓位信息
type PositionInfo struct {
    Symbol        string
    PositionSide  string
    Quantity      float64
    EntryPrice    float64
    MarkPrice     float64
    UnrealizedPnl float64
    Leverage      int
    NotionalValue float64  // abs(Quantity * MarkPrice)
}

// FundingInfo 资金费率结算信息
type FundingInfo struct {
    Symbol          string
    FundingInterval int64    // ms
    FundingRate     float64
    NextFundingTime int64    // Unix ms
}

// AccountData 单个账户的完整采集快照
type AccountData struct {
    Config        AccountConfig   // 来自配置
    Account       AccountInfo     // 当前账户状态
    Positions     []PositionInfo  // 当前仓位
    PrevPositions []PositionInfo  // 上一轮仓位
    FundingInfos  []FundingInfo   // 资金费率
    CollectTime   time.Time
    Error         error           // 采集失败时非 nil
}

// RiskAlert 风险告警
type RiskAlert struct {
    RuleCode     string
    RuleName     string
    Level        string  // "L2"
    AccountID    string
    AccountLabel string
    Title        string
    Message      string
    Details      map[string]interface{}
    Timestamp    time.Time
}
```

### config.go — Sentinel 专用配置解析

创建 `internal/sentinel/config.go`，使用 `github.com/BurntSushi/toml` 解析：

```go
package sentinel

type SentinelConfig struct {
    System   SystemCfg       `toml:"system"`
    Redis    RedisCfg        `toml:"redis"`
    Binance  BinanceCfg      `toml:"binance"`
    Telegram TelegramCfg     `toml:"telegram"`
    Sentinel SentinelOpts    `toml:"sentinel"`
    Accounts []AccountConfig `toml:"accounts"`
    Rules    RulesConfig     `toml:"rules"`
}

type SystemCfg struct {
    Debug     bool   `toml:"debug"`
    Env       string `toml:"env"`
    HTTPProxy string `toml:"http_proxy"`  // ⚠️ 关键：HTTP 代理地址
}

type RedisCfg struct {
    Host string `toml:"host"`
    Port string `toml:"port"`
    // ... user/pass/db/pool_size
}

type BinanceCfg struct {
    FuturesREST        string `toml:"futures_rest"`
    UseTestnet         bool   `toml:"use_testnet"`
    TestnetFuturesREST string `toml:"testnet_futures_rest"`
}

type TelegramCfg struct {
    BotToken string   `toml:"bot_token"`
    ChatIDs  []string `toml:"chat_ids"`
}

type SentinelOpts struct {
    PollInterval string `toml:"poll_interval"` // "10s"
    Symbol       string `toml:"symbol"`        // "BTCUSDT"
}

type AccountConfig struct {
    ID          string `toml:"id"`
    Exchange    string `toml:"exchange"`
    AccountType string `toml:"account_type"` // "regular" | "portfolio_margin"
    APIKey      string `toml:"api_key"`
    SecretKey   string `toml:"secret_key"`
    Label       string `toml:"label"`
}

type RulesConfig struct {
    MarginRatioThreshold     float64 `toml:"margin_ratio_threshold"`
    PMUniMMRThreshold        float64 `toml:"pm_unimmr_threshold"`
    PMStatusAlert            bool    `toml:"pm_status_alert"`
    DeltaChangeRateThreshold float64 `toml:"delta_change_rate_threshold"`
    DeltaAbsThreshold        float64 `toml:"delta_abs_threshold"`
    PositionChangeThreshold  float64 `toml:"position_change_threshold"`
    ExpectedFundingInterval  int64   `toml:"expected_funding_interval"`
    DataGapThreshold         int     `toml:"data_gap_threshold"`
    CooldownTTL              int     `toml:"cooldown_ttl"`
}
```

提供以下辅助方法：
- `LoadSentinelConfig(path string) (*SentinelConfig, error)` — 加载 + 填充默认值
- `(RulesConfig).ApplyDefaults()` — 填充零值字段的默认值
- `(AccountConfig).IsPortfolioMargin() bool`
- `(BinanceCfg).ActiveRESTEndpoint() string` — 根据 use_testnet 返回
- `(BinanceCfg).PMRESTEndpoint() string` — 返回 PM API 地址 `https://papi.binance.com`
- `(SentinelOpts).ParsePollInterval() time.Duration`
- `(RedisCfg).Addr() string`

**默认值表：**
| 字段 | 默认值 |
|------|--------|
| MarginRatioThreshold | 0.5 |
| PMUniMMRThreshold | 1.5 |
| PMStatusAlert | true |
| DeltaChangeRateThreshold | 0.3 |
| DeltaAbsThreshold | 5000 |
| PositionChangeThreshold | 50000 |
| ExpectedFundingInterval | 28800000 |
| DataGapThreshold | 30 |
| CooldownTTL | 300 |
| PollInterval | "10s" |
| Symbol | "BTCUSDT" |

---

## Task α-02: 更新 config.toml

在现有 `config.toml` **末尾**追加以下配置段：

```toml
# ============================================================
# Sentinel 最小闭环配置
# ============================================================

[sentinel]
poll_interval = "10s"
symbol = "BTCUSDT"

[[accounts]]
id = "acc-01"
exchange = "binance_futures"
account_type = "portfolio_margin"
api_key = ""
secret_key = ""
label = "主力1-PM"

[[accounts]]
id = "acc-02"
exchange = "binance_futures"
account_type = "regular"
api_key = ""
secret_key = ""
label = "策略2"

[[accounts]]
id = "acc-03"
exchange = "binance_futures"
account_type = "portfolio_margin"
api_key = ""
secret_key = ""
label = "做市3-PM"

[[accounts]]
id = "acc-04"
exchange = "binance_futures"
account_type = "regular"
api_key = ""
secret_key = ""
label = "套利4"

[[accounts]]
id = "acc-05"
exchange = "binance_futures"
account_type = "portfolio_margin"
api_key = ""
secret_key = ""
label = "备用5-PM"

[rules]
margin_ratio_threshold = 0.5
pm_unimmr_threshold = 1.5
pm_status_alert = true
delta_change_rate_threshold = 0.3
delta_abs_threshold = 5000.0
position_change_threshold = 50000.0
expected_funding_interval = 28800000
data_gap_threshold = 30
cooldown_ttl = 300
```

---

## Task α-03-prep: main.go 骨架

创建 `cmd/risk-sentinel/main.go` 最小骨架，确保 Wave 1 期间编译通过：

```go
package main

import (
    "fmt"
    "os"
    "github.com/cex-risk/cex-risk/internal/sentinel"
)

func main() {
    configPath := "config.toml"
    if len(os.Args) > 2 && os.Args[1] == "--config" {
        configPath = os.Args[2]
    }

    cfg, err := sentinel.LoadSentinelConfig(configPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Sentinel 启动: %d 个账户, 轮询间隔 %s, 币种 %s\n",
        len(cfg.Accounts), cfg.Sentinel.PollInterval, cfg.Sentinel.Symbol)

    // TODO: Wave 2 阶段填充完整逻辑
    _ = cfg
}
```

---

## Task α-03: main.go 完整集成

Wave 1 完成后，将 collector + engine + alerter 串接到 main.go：

```go
func main() {
    // 1. 解析命令行参数（--config, --dry-run）
    // 2. 加载配置 LoadSentinelConfig()
    // 3. 初始化 Redis 连接
    // 4. 创建 BinanceRESTClient (per account, with proxy)
    // 5. 创建 Collector
    // 6. 创建 Alerter (Telegram, with proxy)
    // 7. 创建 Engine (collector + rules + alerter)
    // 8. 注册 signal handler (SIGINT/SIGTERM → graceful shutdown)
    // 9. 启动 engine.Start(ctx) 阻塞循环
}
```

支持 `--dry-run` 模式：仅执行一次采集+规则评估，打印结果到 stdout，不发送 Telegram。

---

## Task α-04: 编译验证

执行以下检查，全部通过后 gate-s2 打开：
```bash
go build ./internal/sentinel/...
go vet ./internal/sentinel/...
go build -o /tmp/risk-sentinel ./cmd/risk-sentinel/
/tmp/risk-sentinel --config config.toml --dry-run
```

修复所有编译错误（可能需要调整 import 路径、类型对齐等）。
