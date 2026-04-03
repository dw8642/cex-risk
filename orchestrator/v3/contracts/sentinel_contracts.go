// Package contracts — Sentinel 最小闭环多 Agent 共享契约
//
// 此文件定义了 Agent-α / Agent-β / Agent-γ 之间的接口和数据结构约定。
// 不参与编译（放在 contracts/ 下仅作参考），实际实现在 internal/sentinel/ 中。
//
// ⚠️  所有 Agent 必须严格遵守此契约，不得修改字段名、类型或方法签名。
package contracts

import (
	"context"
	"time"
)

// ============================================================
// 1. 配置类型 — Agent-α 在 config.go 中实现
// ============================================================

// AccountConfig 单个监控账户配置（从 config.toml [[accounts]] 解析）
type AccountConfig struct {
	ID          string // 唯一标识，如 "acc-01"
	Exchange    string // "binance_futures"
	AccountType string // "regular" | "portfolio_margin"
	APIKey      string
	SecretKey   string
	Label       string // 人类可读标签，如 "主力1-PM"
}

// RulesConfig 规则阈值配置（从 config.toml [rules] 解析）
type RulesConfig struct {
	MarginRatioThreshold     float64 // L-001 普通合约，默认 0.5
	PMUniMMRThreshold        float64 // L-001 统一账户 uniMMR，默认 1.5
	PMStatusAlert            bool    // L-001 监控 accountStatus，默认 true
	DeltaChangeRateThreshold float64 // E-001 默认 0.3
	DeltaAbsThreshold        float64 // E-001 默认 5000 USD
	PositionChangeThreshold  float64 // E-005 默认 50000 USD/min
	ExpectedFundingInterval  int64   // E-008c 默认 28800000 ms (8h)
	DataGapThreshold         int     // S-004 默认 30s
	CooldownTTL              int     // 默认 300s
}

// ============================================================
// 2. 数据类型 — Agent-α 在 types.go 中定义
// ============================================================

// AccountInfo 统一账户信息（屏蔽普通/PM差异）
// Agent-β 的 Collector 负责填充此结构
type AccountInfo struct {
	AccountID     string  // 配置中的 ID
	AccountType   string  // "regular" | "portfolio_margin"
	Label         string  // 人类可读标签

	// 保证金指标（统一语义：ratio 越大越危险）
	MarginRatio   float64 // 普通: totalMaintMargin/totalMarginBalance; PM: 转换为可比值
	UniMMR        float64 // PM 专用原始值（越小越危险），普通账户为 0
	AccountStatus string  // PM 专用（NORMAL/MARGIN_CALL/...），普通账户为 "N/A"

	// 余额
	TotalEquity     float64 // 账户总权益 USD
	AvailableMargin float64 // 可用保证金
	TotalMaintMargin  float64 // 维持保证金
	TotalMarginBalance float64 // 保证金余额

	// 权限（P-001 用）
	EnableWithdraw bool
	IPRestrict     bool

	UpdatedAt time.Time
}

// PositionInfo 统一仓位信息
type PositionInfo struct {
	Symbol        string
	PositionSide  string  // LONG / SHORT / BOTH
	Quantity      float64 // 数量（正=多，负=空）
	EntryPrice    float64
	MarkPrice     float64
	UnrealizedPnl float64
	Leverage      int
	NotionalValue float64 // abs(Quantity * MarkPrice) — 名义价值 USD
}

// FundingInfo 资金费率结算信息
type FundingInfo struct {
	Symbol          string
	FundingInterval int64 // 结算周期（毫秒），如 28800000 = 8h
	FundingRate     float64
	NextFundingTime int64 // Unix ms
}

// AccountData 单个账户的完整采集快照
// Collector 每 tick 产出一份，传给 Engine
type AccountData struct {
	Config        AccountConfig
	Account       AccountInfo
	Positions     []PositionInfo // 当前仓位
	PrevPositions []PositionInfo // 上一轮仓位（E-001/E-005 用）
	FundingInfos  []FundingInfo  // 资金费率信息
	CollectTime   time.Time
	Error         error // 采集失败时非 nil，Engine 跳过该账户并告警
}

// RiskAlert 风险告警事件
// Rules 产出 → Engine 收集 → Alerter 发送
type RiskAlert struct {
	RuleCode     string                 // "P-001", "L-001", ...
	RuleName     string                 // "提币权限异常", ...
	Level        string                 // "L2"（MVP 统一）
	AccountID    string
	AccountLabel string
	Title        string
	Message      string                 // Telegram 消息正文
	Details      map[string]interface{} // 结构化详情
	Timestamp    time.Time
}

// ============================================================
// 3. 接口契约 — 各 Agent 实现
// ============================================================

// Collector 数据采集器接口（Agent-β 实现）
type Collector interface {
	// CollectAll 采集所有账户的最新数据
	// 返回每个账户的 AccountData，采集失败的账户 Error 字段非 nil
	CollectAll(ctx context.Context) []AccountData
}

// RuleFunc 规则函数签名（Agent-γ 实现）
// 输入：单个账户的完整数据 + 规则配置
// 输出：触发的告警列表（无告警则返回空切片）
type RuleFunc func(ctx context.Context, data *AccountData, cfg *RulesConfig) []RiskAlert

// Alerter 告警发送接口（Agent-γ 实现）
type Alerter interface {
	// Send 发送告警，内部处理冷却去重
	// 返回是否实际发送（false = 被冷却抑制）
	Send(ctx context.Context, alert RiskAlert) (bool, error)
}

// Engine 规则调度引擎接口（Agent-γ 实现）
type Engine interface {
	// RunOnce 执行一次完整的采集→评估→告警循环
	RunOnce(ctx context.Context) error

	// Start 启动周期性循环（阻塞直到 ctx 取消）
	Start(ctx context.Context) error
}

// ============================================================
// 4. HTTP Proxy 约束
// ============================================================
//
// 【强制要求】所有访问交易所 REST API 的 http.Client 必须配置代理：
//
//   proxy := cfg.System.HTTPProxy  // 从 config.toml [system] http_proxy 读取
//   if proxy != "" {
//       proxyURL, _ := url.Parse("http://" + proxy)
//       transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
//       client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
//   }
//
// 涉及的模块：
//   - client.go: BinanceRESTClient 的所有 REST 调用（/fapi/, /papi/, /sapi/）
//   - alerter.go: Telegram Bot API 调用（https://api.telegram.org/）
//
// 配置示例（config.toml）：
//   [system]
//   http_proxy = "127.0.0.1:7897"

// ============================================================
// 5. Redis Key 命名规范
// ============================================================
//
// sentinel:account:{accountID}           → JSON(AccountInfo)
// sentinel:positions:{accountID}         → JSON([]PositionInfo)
// sentinel:positions_prev:{accountID}    → JSON([]PositionInfo) — 上一轮快照
// sentinel:funding_info:{symbol}         → JSON([]FundingInfo)
// sentinel:freshness:{accountID}         → Unix timestamp string
// sentinel:cooldown:{ruleCode}:{accountID} → "1", TTL = CooldownTTL
