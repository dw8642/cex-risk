// Package sentinel 实现最小闭环风控哨兵
//
// 架构：单进程 REST 轮询 → Redis → 规则引擎 → Telegram 告警
// 支持：币安普通合约 (/fapi/) + 统一账户 Portfolio Margin (/papi/)
package sentinel

import "time"

// AccountInfo 统一账户信息（屏蔽普通合约/PM 差异）
// Collector 负责将不同 API 响应转化为此统一结构
type AccountInfo struct {
	AccountID   string // 配置中的 ID
	AccountType string // "regular" | "portfolio_margin"
	Label       string // 人类可读标签

	// 保证金指标
	MarginRatio        float64 // 普通: totalMaintMargin/totalMarginBalance; PM: 转换后的可比值
	UniMMR             float64 // PM 专用原始值（越小越危险），普通账户为 0
	AccountStatus      string  // PM 专用（NORMAL/MARGIN_CALL/...），普通账户为 "N/A"
	TotalEquity        float64 // 账户总权益 USD
	AvailableMargin    float64 // 可用保证金
	TotalMaintMargin   float64 // 维持保证金
	TotalMarginBalance float64 // 保证金余额（普通合约）

	// 权限（P-001 用）
	EnableWithdraw bool
	IPRestrict     bool

	UpdatedAt time.Time
}

// PositionInfo 统一仓位信息
type PositionInfo struct {
	Symbol           string
	PositionSide     string  // LONG / SHORT / BOTH
	Quantity         float64 // 数量（正=多，负=空）
	EntryPrice       float64
	MarkPrice        float64
	UnrealizedPnl    float64
	Leverage         int
	NotionalValue    float64 // abs(Quantity * MarkPrice)
	LiquidationPrice float64 // 强平价格（L-002 用）
	ADLQuantile      int     // ADL 等级 1-5（L-003 用），0=无数据
}

// FundingInfo 资金费率结算信息
type FundingInfo struct {
	Symbol          string
	FundingInterval int64 // 结算周期（毫秒），如 28800000 = 8h
	FundingRate     float64
	NextFundingTime int64   // Unix ms
	MarkPrice       float64 // premiumIndex 返回的标记价格（M-001 用）
}

// APIHealthStats 交易所 API 健康指标（S-014 用）
type APIHealthStats struct {
	TotalRequests int       // 窗口内总请求数
	ErrorCount    int       // 窗口内错误数
	AvgLatencyMs  float64   // 窗口内平均延迟 (ms)
	MaxLatencyMs  float64   // 窗口内最大延迟 (ms)
	WindowStart   time.Time // 统计窗口起始
}

// PricePoint 价格采样点（M-001 滑动窗口用）
type PricePoint struct {
	Price float64
	Time  time.Time
}

// AccountData 单个账户的完整采集快照
// Collector 每 tick 产出一份，传给 Engine
type AccountData struct {
	Config        AccountConfig           // 来自配置
	Account       AccountInfo             // 当前账户状态
	Positions     []PositionInfo          // 当前仓位
	PrevPositions []PositionInfo          // 上一轮仓位（E-001/E-005 用）
	FundingInfos  []FundingInfo           // 资金费率信息
	APIHealth     *APIHealthStats         // API 健康指标（S-014 用）
	PriceHistory  map[string][]PricePoint // symbol → 价格滑动窗口（M-001 用）
	CollectTime   time.Time
	Error         error // 采集失败时非 nil，Engine 跳过该账户并告警
}

// RiskAlert 风险告警事件
// Rules 产出 → Engine 收集 → Alerter 发送
type RiskAlert struct {
	RuleCode           string // "P-001", "L-001", ...
	RuleName           string // "提币权限异常", ...
	Level              string // "L2"（MVP 统一）
	AccountID          string
	AccountLabel       string
	Title              string
	Message            string                 // Telegram 消息正文
	Details            map[string]interface{} // 结构化详情
	CooldownKey        string                 // 可选：自定义冷却去重 key（市场级规则用 symbol 维度）
	CooldownTTLSeconds int                    // 可选：单条告警覆盖默认冷却周期
	Timestamp          time.Time
}

// RuleFunc 规则函数签名
// 输入：单个账户的完整数据 + 规则配置
// 输出：触发的告警列表（无告警则返回空切片）
type RuleFunc func(data *AccountData, cfg *RulesConfig) []RiskAlert

// RuleDefinition 规则定义
type RuleDefinition struct {
	Code string // "P-001"
	Name string // "提币权限异常"
	Fn   RuleFunc
}
