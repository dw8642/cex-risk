# Agent-Beta (Data Layer) — Sentinel 最小闭环

## 你的角色
你是 Sentinel 最小闭环项目的 **数据层** Agent。你负责实现 Binance REST API 客户端（同时支持普通合约和统一账户 Portfolio Margin）以及数据采集器。

## 你的代码职责范围
**只允许创建/修改以下文件：**
- `internal/sentinel/client.go` — Binance REST 客户端
- `internal/sentinel/collector.go` — 数据采集器

**禁止修改：**
- `internal/sentinel/types.go`（Agent-α）
- `internal/sentinel/config.go`（Agent-α）
- `internal/sentinel/rules.go`（Agent-γ）
- `internal/sentinel/alerter.go`（Agent-γ）
- `internal/sentinel/engine.go`（Agent-γ）
- `cmd/risk-sentinel/main.go`（Agent-α）

## 共享契约
参考 `orchestrator/v3/contracts/sentinel_contracts.go`。你的输入是 `AccountConfig`，输出是 `AccountData`。

## ⚠️ 强制约束：HTTP Proxy

**所有 REST API 请求必须通过可配置的 HTTP 代理！** 这是硬性要求。

```go
// 创建带 Proxy 的 http.Client
func newProxiedHTTPClient(proxy string, timeout time.Duration) *http.Client {
    transport := &http.Transport{}
    if proxy != "" {
        proxyURL, err := url.Parse("http://" + proxy)
        if err == nil {
            transport.Proxy = http.ProxyURL(proxyURL)
        }
    }
    return &http.Client{Transport: transport, Timeout: timeout}
}
```

代理地址从 `SentinelConfig.System.HTTPProxy` 获取，典型值 `127.0.0.1:7897`。

## 其他编码约束
- 中文注释
- 显式错误处理，禁止 `_ = someFunc()`
- 所有 float 字段从 JSON string 解析：使用 `strconv.ParseFloat`

---

## Task β-01: Binance REST 客户端 client.go

创建 `internal/sentinel/client.go`，实现轻量级 REST-only 客户端（不含 WebSocket）。

### 核心结构

```go
package sentinel

// BinanceRESTClient 币安 REST API 客户端（仅用于 Sentinel 轮询）
// 支持两种账户类型：普通合约 (/fapi/) 和 统一账户 (/papi/)
// 所有请求通过 HTTP Proxy 发送
type BinanceRESTClient struct {
    apiKey     string
    secretKey  string
    baseURL    string // 普通: https://fapi.binance.com, PM: https://papi.binance.com
    pmBaseURL  string // PM 专用: https://papi.binance.com
    httpClient *http.Client
    accountType string // "regular" | "portfolio_margin"
}
```

### 必须实现的方法

**1. 构造函数**
```go
// NewBinanceRESTClient 创建 REST 客户端
// proxy: HTTP 代理地址（如 "127.0.0.1:7897"），空字符串=不使用代理
func NewBinanceRESTClient(apiKey, secretKey, baseURL, pmBaseURL, accountType, proxy string) *BinanceRESTClient
```

**2. 签名方法**（复用已有模式，HMAC-SHA256）
```go
func (c *BinanceRESTClient) sign(payload string) string
func (c *BinanceRESTClient) signedGet(ctx context.Context, fullURL string, extra url.Values) ([]byte, error)
```

**3. 普通合约端点**
```go
// GetFuturesAccount 获取普通合约账户信息
// GET /fapi/v2/account (signed)
// 返回 AccountInfo（填充 MarginRatio, TotalEquity, TotalMaintMargin, TotalMarginBalance 等）
func (c *BinanceRESTClient) GetFuturesAccount(ctx context.Context) (*AccountInfo, error)

// GetFuturesPositions 获取普通合约仓位
// GET /fapi/v2/positionRisk (signed)
// 只返回 positionAmt != 0 的仓位
func (c *BinanceRESTClient) GetFuturesPositions(ctx context.Context) ([]PositionInfo, error)
```

**4. 统一账户 (Portfolio Margin) 端点**
```go
// GetPMAccount 获取统一账户信息
// GET /papi/v1/account (signed)
// 关键字段：uniMMR, accountEquity, accountMaintMargin, accountStatus
func (c *BinanceRESTClient) GetPMAccount(ctx context.Context) (*AccountInfo, error)

// GetPMPositions 获取统一账户 U本位合约仓位
// GET /papi/v1/um/positionRisk (signed)
func (c *BinanceRESTClient) GetPMPositions(ctx context.Context) ([]PositionInfo, error)
```

**5. 公共端点**（两种账户共用）
```go
// GetAPIPermissions 获取 API Key 权限
// GET /sapi/v1/account/apiRestrictions (signed)
// 注意：sapi 走现货 API 域名，需要 baseURL 替换 fapi→api
func (c *BinanceRESTClient) GetAPIPermissions(ctx context.Context) (enableWithdraw bool, ipRestrict bool, err error)

// GetFundingInfo 获取资金费率信息
// GET /fapi/v1/fundingInfo (公共端点，无需签名)
// 注意：无需签名，但仍需通过 Proxy
func (c *BinanceRESTClient) GetFundingInfo(ctx context.Context, symbol string) ([]FundingInfo, error)
```

**6. 统一入口方法**
```go
// GetAccountInfo 根据 accountType 自动选择正确的端点
func (c *BinanceRESTClient) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
    if c.accountType == "portfolio_margin" {
        return c.GetPMAccount(ctx)
    }
    return c.GetFuturesAccount(ctx)
}

// GetPositions 根据 accountType 自动选择正确的端点
func (c *BinanceRESTClient) GetPositions(ctx context.Context) ([]PositionInfo, error) {
    if c.accountType == "portfolio_margin" {
        return c.GetPMPositions(ctx)
    }
    return c.GetFuturesPositions(ctx)
}
```

### Binance API 响应结构参考

```go
// --- 普通合约 /fapi/v2/account ---
type binanceFuturesAccountResp struct {
    TotalMaintMargin  string `json:"totalMaintMargin"`
    TotalMarginBalance string `json:"totalMarginBalance"`
    TotalWalletBalance string `json:"totalWalletBalance"`
    AvailableBalance   string `json:"availableBalance"`
    Assets    []binanceFuturesAsset    `json:"assets"`
    Positions []binanceFuturesPosition `json:"positions"`
}

// --- 普通合约 /fapi/v2/positionRisk ---
type binanceFuturesPositionRisk struct {
    Symbol           string `json:"symbol"`
    PositionAmt      string `json:"positionAmt"`
    EntryPrice       string `json:"entryPrice"`
    MarkPrice        string `json:"markPrice"`
    UnRealizedProfit string `json:"unRealizedProfit"`
    LiquidationPrice string `json:"liquidationPrice"`
    Leverage         string `json:"leverage"`
    PositionSide     string `json:"positionSide"`
    Notional         string `json:"notional"`
}

// --- PM /papi/v1/account ---
type binancePMAccountResp struct {
    UniMMR               string `json:"uniMMR"`
    AccountEquity        string `json:"accountEquity"`
    ActualEquity         string `json:"actualEquity"`
    AccountInitialMargin string `json:"accountInitialMargin"`
    AccountMaintMargin   string `json:"accountMaintMargin"`
    AccountStatus        string `json:"accountStatus"`
}

// --- PM /papi/v1/um/positionRisk ---
// 结构同 binanceFuturesPositionRisk

// --- /fapi/v1/fundingInfo ---
type binanceFundingInfoResp struct {
    Symbol            string `json:"symbol"`
    AdjustedFundingRateCap  string `json:"adjustedFundingRateCap"`
    AdjustedFundingRateFloor string `json:"adjustedFundingRateFloor"`
    FundingIntervalHours     int   `json:"fundingIntervalHours"`
    // 注意：fundingIntervalHours 是小时数（如 8），需转换为 ms: * 3600 * 1000
}

// --- /sapi/v1/account/apiRestrictions ---
type binanceAPIRestrictionsResp struct {
    EnableWithdrawals  bool `json:"enableWithdrawals"`
    IPRestrict         bool `json:"ipRestrict"`
    EnableFutures      bool `json:"enableFutures"`
}
```

### PM AccountStatus 状态说明
状态越往后越危险，任何非 NORMAL 状态都应告警：
- `NORMAL` — 正常
- `MARGIN_CALL` — 追保提醒
- `SUPPLY_MARGIN` — 需要补充保证金
- `REDUCE_ONLY` — 只能减仓
- `ACTIVE_LIQUIDATION` — 正在清算
- `FORCE_LIQUIDATION` — 强制清算
- `BANKRUPTED` — 穿仓

---

## Task β-02: 数据采集器 collector.go

创建 `internal/sentinel/collector.go`，实现周期性数据采集。

### 核心结构

```go
// Collector 数据采集器
// 管理多个账户的 REST 客户端，周期性采集数据
type Collector struct {
    clients  map[string]*BinanceRESTClient // key: account ID
    configs  []AccountConfig
    redis    *redis.Client
    symbol   string // 监控的交易对
    logger   *zap.Logger
    prevData map[string][]PositionInfo // 内存中保存上一轮仓位快照
}
```

### 必须实现的方法

```go
// NewCollector 创建采集器
// 为每个账户创建对应的 BinanceRESTClient（根据 account_type 选择端点）
func NewCollector(cfg *SentinelConfig, redisClient *redis.Client, logger *zap.Logger) *Collector

// CollectAll 采集所有账户的最新数据
// 对每个账户并发采集（goroutine），汇总返回
// 采集失败的账户：AccountData.Error 非 nil，Engine 层负责处理
func (c *Collector) CollectAll(ctx context.Context) []AccountData

// collectOne 采集单个账户（内部方法）
// 1. GetAccountInfo → AccountInfo
// 2. GetPositions → []PositionInfo
// 3. GetAPIPermissions（每 60s 一次，非每次都查）
// 4. GetFundingInfo（每 5min 一次，公共接口只查一次即可）
// 5. 组装 AccountData，附上 PrevPositions
// 6. 更新 prevData 和 Redis freshness
func (c *Collector) collectOne(ctx context.Context, acc AccountConfig) AccountData
```

### Redis 写入规范
每次采集成功后写入 Redis（供其他可能的消费者读取）：
```
sentinel:account:{id}           → JSON(AccountInfo), TTL 60s
sentinel:positions:{id}         → JSON([]PositionInfo), TTL 60s
sentinel:freshness:{id}         → Unix timestamp string, TTL 120s
sentinel:funding_info:{symbol}  → JSON([]FundingInfo), TTL 600s
```

### 频率控制
| 数据类型 | 频率 | 说明 |
|---------|------|------|
| 账户信息 + 仓位 | 每 tick（10s） | 核心数据 |
| API 权限 | 每 60s | 低频，减少请求 |
| 资金费率 | 每 5min | 公共接口，所有账户共享一份 |

使用时间戳判断是否需要刷新低频数据：
```go
type Collector struct {
    // ...
    lastPermCheck  map[string]time.Time // 上次权限检查时间
    lastFundingCheck time.Time          // 上次费率检查时间
}
```

### 并发采集
`CollectAll` 对 5 个账户并发采集：
```go
func (c *Collector) CollectAll(ctx context.Context) []AccountData {
    results := make([]AccountData, len(c.configs))
    var wg sync.WaitGroup
    for i, acc := range c.configs {
        wg.Add(1)
        go func(idx int, a AccountConfig) {
            defer wg.Done()
            results[idx] = c.collectOne(ctx, a)
        }(i, acc)
    }
    wg.Wait()
    return results
}
```
