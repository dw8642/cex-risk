// rules.go — 6 条 P0 风控规则实现
//
// 规则清单（MVP P0）：
//
//	P-001: 提币权限异常开启
//	S-004: 风控系统失明/数据断流
//	L-001: 维持保证金占比过高（含 PM uniMMR + accountStatus）
//	E-001: 净 Delta 变化率异常
//	E-005: 仓位变动速率异常
//	E-008c: 资金费率结算周期变更
//
// 统一签名：func(data *AccountData, cfg *RulesConfig) []RiskAlert
package sentinel

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type e001EvalResult struct {
	CurrentDelta   float64
	PrevDelta      float64
	DeltaChange    float64
	AbsDeltaChange float64
	Denominator    float64
	ChangeRate     float64
	AbsChangeRate  float64
	RateTriggered  bool
	AbsTriggered   bool
	Triggered      bool
	SkipReason     string
	TriggerReason  string
}

type l001EvalResult struct {
	AccountType     string
	SkipReason      string
	Triggered       bool
	TriggerLevel    string
	TriggerBranch   string
	MarginRatio     float64
	MarginThreshold float64
	UniMMR          float64
	UniMMRThreshold float64
	AccountStatus   string
	TotalEquity     float64
	MaintMargin     float64
	MarginBalance   float64
	TriggerReason   string
}

type p001EvalResult struct {
	SkipReason     string
	EnableWithdraw bool
	IPRestrict     bool
	Triggered      bool
}

type s004EvalResult struct {
	Triggered   bool
	SkipReason  string
	ErrorReason string
}

type e005EvalResult struct {
	SkipReason    string
	TotalChange   float64
	Threshold     float64
	CurrentCount  int
	PrevCount     int
	Triggered     bool
	ChangedLegs   int
	ClosedLegs    int
	TriggerReason string
}

type e008cEvalResult struct {
	SkipReason    string
	Triggered     bool
	MismatchCount int
	ExpectedHours float64
	Distribution  string
	Preview       string
}

type l002EvalResult struct {
	SkipReason       string
	Triggered        bool
	PositionCount    int
	CheckedCount     int
	TriggeredCount   int
	MinDistancePct   float64
	L2Threshold      float64
	L3Threshold      float64
	HighestLevel     string
	ClosestSymbol    string
	ClosestSide      string
	ClosestMarkPrice float64
	ClosestLiqPrice  float64
}

type l003EvalResult struct {
	SkipReason     string
	Triggered      bool
	PositionCount  int
	CheckedCount   int
	TriggeredCount int
	Threshold      int
	MaxADLQuantile int
	TopSymbol      string
	TopSide        string
}

type e008EvalResult struct {
	SkipReason        string
	Triggered         bool
	PositionCount     int
	FundingInfoCount  int
	PayingCount       int
	TriggeredCount    int
	Threshold         float64
	MaxAnnualizedRate float64
	MaxAnnualizedCost float64
	TopSymbol         string
	TopSide           string
}

type s014EvalResult struct {
	SkipReason       string
	Triggered        bool
	TotalRequests    int
	ErrorCount       int
	ErrorRate        float64
	AvgLatencyMs     float64
	MaxLatencyMs     float64
	LatencyThreshold float64
	ErrorThreshold   float64
	TriggerReason    string
}

type m001EvalResult struct {
	SkipReason        string
	Triggered         bool
	WindowMinutes     int
	Threshold         float64
	SymbolCount       int
	CheckedSymbols    int
	TriggeredCount    int
	TopSymbol         string
	TopJumpPct        float64
	TopDirection      string
	PrevSamplePrice   float64
	LatestSamplePrice float64
	PrevSampleTime    time.Time
	LatestSampleTime  time.Time
	CurrentPrice      float64
	StartPrice        float64
	Preview           string
}

// AllRules 返回所有 MVP P0 规则
func AllRules() []RuleDefinition {
	return []RuleDefinition{
		{Code: "P-001", Name: "提币权限异常开启", Fn: RuleP001},
		{Code: "S-004", Name: "风控系统失明/数据断流", Fn: RuleS004},
		{Code: "S-014", Name: "交易所接口健康恶化", Fn: RuleS014},
		{Code: "L-001", Name: "维持保证金占比过高", Fn: RuleL001},
		{Code: "L-002", Name: "爆仓距离过近", Fn: RuleL002},
		{Code: "L-003", Name: "ADL风险升高", Fn: RuleL003},
		{Code: "E-001", Name: "净Delta变化率异常", Fn: RuleE001},
		{Code: "E-005", Name: "仓位变动速率异常", Fn: RuleE005},
		{Code: "E-008", Name: "资金费率侵蚀", Fn: RuleE008},
		{Code: "E-008c", Name: "资金费率结算周期变更", Fn: RuleE008c},
		{Code: "M-001", Name: "波动率突升", Fn: RuleM001},
	}
}

// ==================== P-001 提币权限异常 ====================

// RuleP001 检测 API Key 是否有提币权限
// 做市 API Key 不应具备提币权限，开启即告警
func RuleP001(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalP001(data)
	if result.SkipReason != "" || !result.Triggered {
		return nil // 数据异常由 S-004 处理
	}

	return []RiskAlert{{
		RuleCode:     "P-001",
		RuleName:     "提币权限异常开启",
		Level:        "L2",
		AccountID:    data.Account.AccountID,
		AccountLabel: data.Account.Label,
		Title:        fmt.Sprintf("账户 %s API Key 开启了提币权限", data.Account.Label),
		Message:      fmt.Sprintf("做市Key不应有提币权限，请确认是否误开。IP白名单：%v", data.Account.IPRestrict),
		Timestamp:    time.Now(),
	}}
}

// ==================== S-004 风控系统失明 ====================

// RuleS004 检测数据采集是否断流
// 当 AccountData.Error 非 nil 时表示采集失败，风控系统对该账户"失明"
func RuleS004(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalS004(data)
	if result.SkipReason != "" || !result.Triggered {
		return nil // 数据正常
	}

	return []RiskAlert{{
		RuleCode:     "S-004",
		RuleName:     "风控系统失明/数据断流",
		Level:        "L2",
		AccountID:    data.Config.ID,
		AccountLabel: data.Config.Label,
		Title:        fmt.Sprintf("账户 %s 数据采集失败", data.Config.Label),
		Message:      fmt.Sprintf("错误：%s\n数据恢复前该账户处于盲区状态。", data.Error.Error()),
		Timestamp:    time.Now(),
	}}
}

func evalP001(data *AccountData) p001EvalResult {
	if data.Error != nil {
		return p001EvalResult{SkipReason: "data_error"}
	}
	result := p001EvalResult{
		EnableWithdraw: data.Account.EnableWithdraw,
		IPRestrict:     data.Account.IPRestrict,
	}
	if !data.Account.EnableWithdraw {
		result.SkipReason = "withdraw_disabled"
		return result
	}
	result.Triggered = true
	return result
}

func evalS004(data *AccountData) s004EvalResult {
	if data.Error == nil {
		return s004EvalResult{SkipReason: "no_error"}
	}
	return s004EvalResult{Triggered: true, ErrorReason: data.Error.Error()}
}

// ==================== L-001 维持保证金占比过高 ====================

// RuleL001 检测保证金风险
//
// 分支 A — 普通合约: totalMaintMargin / totalMarginBalance >= threshold (0.5)
// 分支 B — 统一账户:
//
//	B1: uniMMR <= threshold (1.5)，越小越危险
//	B2: accountStatus != NORMAL → 直接告警
func RuleL001(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalL001(data, cfg)
	if result.SkipReason != "" {
		return nil
	}

	var alerts []RiskAlert
	acc := data.Account

	if acc.AccountType == "portfolio_margin" {
		// B1: uniMMR 阈值检测
		if acc.UniMMR > 0 && acc.UniMMR <= cfg.PMUniMMRThreshold {
			alerts = append(alerts, RiskAlert{
				RuleCode:     "L-001",
				RuleName:     "维持保证金占比过高",
				Level:        "L2",
				AccountID:    acc.AccountID,
				AccountLabel: acc.Label,
				Title:        fmt.Sprintf("账户 %s uniMMR 偏低 %.4f（阈值 %.4f）", acc.Label, acc.UniMMR, cfg.PMUniMMRThreshold),
				Message: fmt.Sprintf("权益 $%.2f｜维持保证金 $%.2f｜状态 %s\n强平线 1.05，请确认是否需要补保证金或减仓。",
					acc.TotalEquity, acc.TotalMaintMargin, acc.AccountStatus),
				Timestamp: time.Now(),
			})
		}

		// B2: accountStatus 异常检测
		if cfg.PMStatusAlert && acc.AccountStatus != "NORMAL" && acc.AccountStatus != "N/A" && acc.AccountStatus != "" {
			level := "L2"
			switch acc.AccountStatus {
			case "REDUCE_ONLY", "ACTIVE_LIQUIDATION", "FORCE_LIQUIDATION", "BANKRUPTED":
				level = "L3" // 紧急
			}
			alerts = append(alerts, RiskAlert{
				RuleCode:     "L-001",
				RuleName:     "统一账户状态异常",
				Level:        level,
				AccountID:    acc.AccountID,
				AccountLabel: acc.Label,
				Title:        fmt.Sprintf("账户 %s 状态 %s", acc.Label, acc.AccountStatus),
				Message:      fmt.Sprintf("uniMMR %.4f｜权益 $%.2f", acc.UniMMR, acc.TotalEquity),
				Timestamp:    time.Now(),
			})
		}
	} else {
		// A: 普通合约
		if acc.TotalMarginBalance > 0 {
			ratio := acc.TotalMaintMargin / acc.TotalMarginBalance
			if ratio >= cfg.MarginRatioThreshold {
				alerts = append(alerts, RiskAlert{
					RuleCode:     "L-001",
					RuleName:     "维持保证金占比过高",
					Level:        "L2",
					AccountID:    acc.AccountID,
					AccountLabel: acc.Label,
					Title:        fmt.Sprintf("账户 %s 保证金占比 %.1f%%（阈值 %.1f%%）", acc.Label, ratio*100, cfg.MarginRatioThreshold*100),
					Message:      fmt.Sprintf("维持保证金 $%.2f｜余额 $%.2f", acc.TotalMaintMargin, acc.TotalMarginBalance),
					Timestamp:    time.Now(),
				})
			}
		}
	}

	return alerts
}

func evalL001(data *AccountData, cfg *RulesConfig) l001EvalResult {
	if data.Error != nil {
		return l001EvalResult{SkipReason: "data_error"}
	}

	acc := data.Account
	result := l001EvalResult{
		AccountType:     acc.AccountType,
		MarginThreshold: cfg.MarginRatioThreshold,
		UniMMRThreshold: cfg.PMUniMMRThreshold,
		UniMMR:          acc.UniMMR,
		AccountStatus:   acc.AccountStatus,
		TotalEquity:     acc.TotalEquity,
		MaintMargin:     acc.TotalMaintMargin,
		MarginBalance:   acc.TotalMarginBalance,
	}

	if acc.AccountType == "portfolio_margin" {
		if acc.UniMMR > 0 && acc.UniMMR <= cfg.PMUniMMRThreshold {
			result.Triggered = true
			result.TriggerLevel = "L2"
			result.TriggerBranch = "pm_unimmr"
			result.TriggerReason = fmt.Sprintf("uniMMR %.4f ≤ %.4f", acc.UniMMR, cfg.PMUniMMRThreshold)
			return result
		}

		if cfg.PMStatusAlert && acc.AccountStatus != "NORMAL" && acc.AccountStatus != "N/A" && acc.AccountStatus != "" {
			result.Triggered = true
			result.TriggerBranch = "pm_account_status"
			result.TriggerLevel = "L2"
			if acc.AccountStatus == "REDUCE_ONLY" || acc.AccountStatus == "ACTIVE_LIQUIDATION" || acc.AccountStatus == "FORCE_LIQUIDATION" || acc.AccountStatus == "BANKRUPTED" {
				result.TriggerLevel = "L3"
			}
			result.TriggerReason = fmt.Sprintf("accountStatus=%s", acc.AccountStatus)
			return result
		}

		result.SkipReason = "pm_below_threshold"
		return result
	}

	if acc.TotalMarginBalance <= 0 {
		result.SkipReason = "margin_balance_zero"
		return result
	}

	result.MarginRatio = acc.TotalMaintMargin / acc.TotalMarginBalance
	if result.MarginRatio >= cfg.MarginRatioThreshold {
		result.Triggered = true
		result.TriggerLevel = "L2"
		result.TriggerBranch = "regular_margin_ratio"
		result.TriggerReason = fmt.Sprintf("margin_ratio %.4f ≥ %.4f", result.MarginRatio, cfg.MarginRatioThreshold)
		return result
	}

	result.SkipReason = "regular_below_threshold"
	return result
}

// ==================== E-001 净 Delta 变化率异常 ====================

// RuleE001 检测净 Delta 暴露的变化速率
//
// 计算：
//
//	net_delta = Σ(quantity * markPrice) — 多头+，空头-
//	delta_change = net_delta_current - net_delta_prev
//	delta_change_rate = delta_change / max(abs(net_delta_prev), DeltaAbsThreshold)
//
// 判定（双安全网）：
//
//	abs(delta_change_rate) >= 0.3  OR  abs(delta_change) >= 5000 USD
func RuleE001(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalE001(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	return []RiskAlert{{
		RuleCode:     "E-001",
		RuleName:     "净Delta变化率异常",
		Level:        "L2",
		AccountID:    data.Account.AccountID,
		AccountLabel: data.Account.Label,
		Title:        fmt.Sprintf("账户 %s 净Delta变化过快", data.Account.Label),
		Message: fmt.Sprintf("$%.0f → $%.0f（变化 $%.0f）\n%s",
			result.PrevDelta, result.CurrentDelta, result.DeltaChange, result.TriggerReason),
		Timestamp: time.Now(),
	}}
}

func evalE001(data *AccountData, cfg *RulesConfig) e001EvalResult {
	if data.Error != nil || len(data.PrevPositions) == 0 {
		if data.Error != nil {
			return e001EvalResult{SkipReason: "data_error"}
		}
		return e001EvalResult{SkipReason: "no_prev_positions"}
	}

	currentDelta := calcNetDelta(data.Positions)
	prevDelta := calcNetDelta(data.PrevPositions)

	deltaChange := currentDelta - prevDelta
	absDeltaChange := math.Abs(deltaChange)

	// 计算变化率（分母用 max(abs(prev), DeltaAbsThreshold) 防止除零和放大）
	denominator := math.Max(math.Abs(prevDelta), cfg.DeltaAbsThreshold)
	changeRate := deltaChange / denominator

	absChangeRate := math.Abs(changeRate)

	// 双安全网判定
	rateTriggered := absChangeRate >= cfg.DeltaChangeRateThreshold
	absTriggered := absDeltaChange >= cfg.DeltaAbsThreshold

	result := e001EvalResult{
		CurrentDelta:   currentDelta,
		PrevDelta:      prevDelta,
		DeltaChange:    deltaChange,
		AbsDeltaChange: absDeltaChange,
		Denominator:    denominator,
		ChangeRate:     changeRate,
		AbsChangeRate:  absChangeRate,
		RateTriggered:  rateTriggered,
		AbsTriggered:   absTriggered,
	}

	if !rateTriggered && !absTriggered {
		result.SkipReason = "below_threshold"
		return result
	}

	triggerReason := ""
	if rateTriggered && absTriggered {
		triggerReason = fmt.Sprintf("变化率 %.1f%% ≥ %.1f%% 且绝对值 $%.0f ≥ $%.0f",
			absChangeRate*100, cfg.DeltaChangeRateThreshold*100,
			absDeltaChange, cfg.DeltaAbsThreshold)
	} else if rateTriggered {
		triggerReason = fmt.Sprintf("变化率 %.1f%% ≥ %.1f%%",
			absChangeRate*100, cfg.DeltaChangeRateThreshold*100)
	} else {
		triggerReason = fmt.Sprintf("绝对变化 $%.0f ≥ $%.0f（兜底）",
			absDeltaChange, cfg.DeltaAbsThreshold)
	}

	result.Triggered = true
	result.TriggerReason = triggerReason
	return result
}

// calcNetDelta 计算净 Delta（多头为正，空头为负）
func calcNetDelta(positions []PositionInfo) float64 {
	var netDelta float64
	for _, p := range positions {
		// Quantity 已经带方向（正=多，负=空），直接乘以 markPrice
		netDelta += p.Quantity * p.MarkPrice
	}
	return netDelta
}

// ==================== E-005 仓位变动速率异常 ====================

// RuleE005 检测仓位名义价值的变动速度
//
// 对每个 symbol 计算 abs(current_notional - prev_notional)，
// 汇总 total_change，超过阈值则告警
func RuleE005(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalE005(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	return []RiskAlert{{
		RuleCode:     "E-005",
		RuleName:     "仓位变动速率异常",
		Level:        "L2",
		AccountID:    data.Account.AccountID,
		AccountLabel: data.Account.Label,
		Title:        fmt.Sprintf("账户 %s 仓位变动 $%.0f（阈值 $%.0f）", data.Account.Label, result.TotalChange, cfg.PositionChangeThreshold),
		Message:      fmt.Sprintf("仓位数 %d→%d", len(data.PrevPositions), len(data.Positions)),
		Timestamp:    time.Now(),
	}}
}

func evalE005(data *AccountData, cfg *RulesConfig) e005EvalResult {
	if data.Error != nil {
		return e005EvalResult{SkipReason: "data_error"}
	}
	if len(data.PrevPositions) == 0 {
		return e005EvalResult{SkipReason: "no_prev_positions"}
	}

	prevMap := make(map[string]float64)
	for _, p := range data.PrevPositions {
		key := p.Symbol + "|" + p.PositionSide
		prevMap[key] = p.NotionalValue
	}

	var totalChange float64
	currentMap := make(map[string]float64)
	changedLegs := 0
	for _, p := range data.Positions {
		key := p.Symbol + "|" + p.PositionSide
		currentMap[key] = p.NotionalValue
		prev := prevMap[key]
		diff := math.Abs(p.NotionalValue - prev)
		if diff > 0 {
			changedLegs++
		}
		totalChange += diff
	}

	closedLegs := 0
	for key, prevNotional := range prevMap {
		if _, ok := currentMap[key]; !ok {
			totalChange += prevNotional
			closedLegs++
		}
	}

	result := e005EvalResult{
		TotalChange:  totalChange,
		Threshold:    cfg.PositionChangeThreshold,
		CurrentCount: len(data.Positions),
		PrevCount:    len(data.PrevPositions),
		ChangedLegs:  changedLegs,
		ClosedLegs:   closedLegs,
	}
	if totalChange < cfg.PositionChangeThreshold {
		result.SkipReason = "below_threshold"
		return result
	}
	result.Triggered = true
	result.TriggerReason = fmt.Sprintf("total_change %.2f ≥ %.2f", totalChange, cfg.PositionChangeThreshold)
	return result
}

// ==================== E-008c 资金费率结算周期变更 ====================

type fundingMismatch struct {
	Symbol         string
	ActualHours    float64
	ExpectedHours  float64
	ActualInterval int64
}

// RuleE008c 检测资金费率结算周期是否发生变更
//
// 交易所可能将结算周期从 8h 改为 4h 或 1h，这会显著影响资金费成本。
// 任何偏离预期周期的变更都应告警。
func RuleE008c(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalE008c(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	var mismatches []fundingMismatch
	var lines []string
	for _, fi := range data.FundingInfos {
		if fi.FundingInterval == 0 {
			continue // 无数据
		}

		if fi.FundingInterval != cfg.ExpectedFundingInterval {
			actualHours := float64(fi.FundingInterval) / 3600000.0
			expectedHours := float64(cfg.ExpectedFundingInterval) / 3600000.0
			mismatches = append(mismatches, fundingMismatch{
				Symbol:         fi.Symbol,
				ActualHours:    actualHours,
				ExpectedHours:  expectedHours,
				ActualInterval: fi.FundingInterval,
			})
			lines = append(lines, fmt.Sprintf("- %s: %.0fh → 预期 %.0fh", fi.Symbol, actualHours, expectedHours))
		}
	}

	if len(mismatches) == 0 {
		return nil
	}

	sort.Slice(mismatches, func(i, j int) bool {
		if mismatches[i].ActualHours == mismatches[j].ActualHours {
			return mismatches[i].Symbol < mismatches[j].Symbol
		}
		return mismatches[i].ActualHours < mismatches[j].ActualHours
	})

	buckets := make(map[string]int)
	for _, item := range mismatches {
		bucket := fmt.Sprintf("%.0f小时", item.ActualHours)
		buckets[bucket]++
	}
	bucketKeys := make([]string, 0, len(buckets))
	for k := range buckets {
		bucketKeys = append(bucketKeys, k)
	}
	sort.Strings(bucketKeys)

	var bucketSummary []string
	for _, k := range bucketKeys {
		bucketSummary = append(bucketSummary, fmt.Sprintf("%s %d个", k, buckets[k]))
	}

	const previewLimit = 20
	previewLines := lines
	if len(previewLines) > previewLimit {
		previewLines = append([]string{}, lines[:previewLimit]...)
		previewLines = append(previewLines, fmt.Sprintf("- 其余 %d 个币种已省略", len(lines)-previewLimit))
	}

	return []RiskAlert{{
		RuleCode:     "E-008c",
		RuleName:     "资金费率结算周期变更",
		Level:        "L2",
		AccountID:    data.Account.AccountID,
		AccountLabel: data.Account.Label,
		Title:        fmt.Sprintf("账户 %s %d个币种资金费周期变更", data.Account.Label, len(mismatches)),
		Message: fmt.Sprintf("预期 %.0fh｜分布：%s\n%s",
			float64(cfg.ExpectedFundingInterval)/3600000.0,
			strings.Join(bucketSummary, "，"),
			strings.Join(previewLines, "\n"),
		),
		CooldownTTLSeconds: cfg.FundingIntervalAlertCooldown,
		Timestamp:          time.Now(),
	}}
}

func evalE008c(data *AccountData, cfg *RulesConfig) e008cEvalResult {
	if data.Error != nil {
		return e008cEvalResult{SkipReason: "data_error"}
	}
	if len(data.FundingInfos) == 0 {
		return e008cEvalResult{SkipReason: "no_funding_infos"}
	}

	buckets := make(map[string]int)
	var lines []string
	count := 0
	for _, fi := range data.FundingInfos {
		if fi.FundingInterval == 0 {
			continue
		}
		if fi.FundingInterval != cfg.ExpectedFundingInterval {
			actualHours := float64(fi.FundingInterval) / 3600000.0
			bucket := fmt.Sprintf("%.0f小时", actualHours)
			buckets[bucket]++
			count++
			if len(lines) < 5 {
				lines = append(lines, fmt.Sprintf("%s %.0fh", fi.Symbol, actualHours))
			}
		}
	}
	if count == 0 {
		return e008cEvalResult{SkipReason: "no_mismatch"}
	}
	bucketKeys := make([]string, 0, len(buckets))
	for k := range buckets {
		bucketKeys = append(bucketKeys, k)
	}
	sort.Strings(bucketKeys)
	var parts []string
	for _, k := range bucketKeys {
		parts = append(parts, fmt.Sprintf("%s %d个", k, buckets[k]))
	}
	return e008cEvalResult{
		Triggered:     true,
		MismatchCount: count,
		ExpectedHours: float64(cfg.ExpectedFundingInterval) / 3600000.0,
		Distribution:  strings.Join(parts, "，"),
		Preview:       strings.Join(lines, ", "),
	}
}

// ==================== L-002 爆仓距离过近 ====================

// RuleL002 检测仓位与强平价格的距离
//
// 计算：distance_pct = abs(liq_price - mark_price) / mark_price
// 判定：distance_pct <= threshold → 告警
//   - L2: distance_pct <= 5%（默认）
//   - L3: distance_pct <= 2%（默认）
func RuleL002(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalL002(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	var alerts []RiskAlert
	for _, p := range data.Positions {
		if p.LiquidationPrice <= 0 || p.MarkPrice <= 0 {
			continue // 无强平价格（如全仓模式下可能为0）
		}

		distancePct := math.Abs(p.LiquidationPrice-p.MarkPrice) / p.MarkPrice

		var level string
		if distancePct <= cfg.LiqDistanceL3Threshold {
			level = "L3"
		} else if distancePct <= cfg.LiqDistanceL2Threshold {
			level = "L2"
		} else {
			continue
		}

		alerts = append(alerts, RiskAlert{
			RuleCode:     "L-002",
			RuleName:     "爆仓距离过近",
			Level:        level,
			AccountID:    data.Account.AccountID,
			AccountLabel: data.Account.Label,
			Title:        fmt.Sprintf("账户 %s %s %s 爆仓距离 %.2f%%", data.Account.Label, p.Symbol, p.PositionSide, distancePct*100),
			Message: fmt.Sprintf("强平价 %.4f｜标记价 %.4f｜杠杆 %dx｜名义 $%.0f",
				p.LiquidationPrice, p.MarkPrice, p.Leverage, p.NotionalValue),
			Timestamp: time.Now(),
		})
	}

	return alerts
}

func evalL002(data *AccountData, cfg *RulesConfig) l002EvalResult {
	if data.Error != nil {
		return l002EvalResult{SkipReason: "data_error"}
	}
	result := l002EvalResult{
		PositionCount:  len(data.Positions),
		L2Threshold:    cfg.LiqDistanceL2Threshold,
		L3Threshold:    cfg.LiqDistanceL3Threshold,
		MinDistancePct: -1,
	}
	for _, p := range data.Positions {
		if p.LiquidationPrice <= 0 || p.MarkPrice <= 0 {
			continue
		}
		result.CheckedCount++
		distancePct := math.Abs(p.LiquidationPrice-p.MarkPrice) / p.MarkPrice
		if result.MinDistancePct < 0 || distancePct < result.MinDistancePct {
			result.MinDistancePct = distancePct
			result.ClosestSymbol = p.Symbol
			result.ClosestSide = p.PositionSide
			result.ClosestMarkPrice = p.MarkPrice
			result.ClosestLiqPrice = p.LiquidationPrice
		}
		if distancePct <= cfg.LiqDistanceL3Threshold {
			result.Triggered = true
			result.TriggeredCount++
			result.HighestLevel = "L3"
		} else if distancePct <= cfg.LiqDistanceL2Threshold {
			result.Triggered = true
			result.TriggeredCount++
			if result.HighestLevel == "" {
				result.HighestLevel = "L2"
			}
		}
	}
	if result.CheckedCount == 0 {
		result.SkipReason = "no_liq_price"
		return result
	}
	if !result.Triggered {
		result.SkipReason = "all_above_threshold"
	}
	return result
}

// ==================== L-003 ADL 风险升高 ====================

// RuleL003 检测 ADL（自动减仓）风险等级
//
// 币安 ADL 等级 1-5，数值越大越危险（5=最高优先被减仓）
// 判定：adlQuantile >= threshold → 告警
func RuleL003(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalL003(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	var alerts []RiskAlert
	for _, p := range data.Positions {
		if p.ADLQuantile <= 0 {
			continue // 无 ADL 数据
		}

		if p.ADLQuantile < cfg.ADLQuantileL2Threshold {
			continue
		}

		alerts = append(alerts, RiskAlert{
			RuleCode:     "L-003",
			RuleName:     "ADL风险升高",
			Level:        "L2",
			AccountID:    data.Account.AccountID,
			AccountLabel: data.Account.Label,
			Title:        fmt.Sprintf("账户 %s %s %s ADL等级 %d", data.Account.Label, p.Symbol, p.PositionSide, p.ADLQuantile),
			Message: fmt.Sprintf("阈值 %d｜名义 $%.0f｜杠杆 %dx\n存在被交易所自动减仓风险",
				cfg.ADLQuantileL2Threshold, p.NotionalValue, p.Leverage),
			Timestamp: time.Now(),
		})
	}

	return alerts
}

func evalL003(data *AccountData, cfg *RulesConfig) l003EvalResult {
	if data.Error != nil {
		return l003EvalResult{SkipReason: "data_error"}
	}
	result := l003EvalResult{PositionCount: len(data.Positions), Threshold: cfg.ADLQuantileL2Threshold}
	for _, p := range data.Positions {
		if p.ADLQuantile <= 0 {
			continue
		}
		result.CheckedCount++
		if p.ADLQuantile > result.MaxADLQuantile {
			result.MaxADLQuantile = p.ADLQuantile
			result.TopSymbol = p.Symbol
			result.TopSide = p.PositionSide
		}
		if p.ADLQuantile >= cfg.ADLQuantileL2Threshold {
			result.Triggered = true
			result.TriggeredCount++
		}
	}
	if result.CheckedCount == 0 {
		result.SkipReason = "no_adl_data"
		return result
	}
	if !result.Triggered {
		result.SkipReason = "below_threshold"
	}
	return result
}

// ==================== E-008 资金费率侵蚀 ====================

// RuleE008 检测资金费率对仓位的年化侵蚀成本
//
// 简化 MVP 实现：使用当前 lastFundingRate 估算年化成本
// annualized_rate = abs(funding_rate) * (365 * 24 / funding_interval_hours) * 方向系数
// 当持仓方向与 fundingRate 方向一致时（多头+正费率 或 空头+负费率），需支付资金费
func RuleE008(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalE008(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	// 构建 symbol → FundingInfo 映射
	fundingMap := make(map[string]FundingInfo)
	for _, fi := range data.FundingInfos {
		if fi.FundingRate != 0 {
			fundingMap[fi.Symbol] = fi
		}
	}

	var alerts []RiskAlert
	for _, p := range data.Positions {
		fi, ok := fundingMap[p.Symbol]
		if !ok || fi.FundingRate == 0 {
			continue
		}

		// 判断是否需支付资金费：
		// 多头（qty>0）+ 正费率 → 多头付空头
		// 空头（qty<0）+ 负费率 → 空头付多头
		isPaying := (p.Quantity > 0 && fi.FundingRate > 0) || (p.Quantity < 0 && fi.FundingRate < 0)
		if !isPaying {
			continue // 收费方，不告警
		}

		// 年化费率估算：每次结算费率 × 每年结算次数
		intervalHours := float64(fi.FundingInterval) / 3600000.0
		if intervalHours <= 0 {
			intervalHours = 8 // 默认 8h
		}
		settlementsPerYear := 365.0 * 24.0 / intervalHours
		annualizedRate := math.Abs(fi.FundingRate) * settlementsPerYear

		if annualizedRate < cfg.FundingRateAnnualizedL2 {
			continue
		}

		// 估算年化 USD 成本
		annualizedCost := annualizedRate * p.NotionalValue

		alerts = append(alerts, RiskAlert{
			RuleCode:     "E-008",
			RuleName:     "资金费率侵蚀",
			Level:        "L2",
			AccountID:    data.Account.AccountID,
			AccountLabel: data.Account.Label,
			Title:        fmt.Sprintf("账户 %s %s 资金费年化 %.1f%%", data.Account.Label, p.Symbol, annualizedRate*100),
			Message: fmt.Sprintf("当前费率 %.4f%%｜结算周期 %.0fh｜名义 $%.0f｜年化成本 $%.0f",
				fi.FundingRate*100, intervalHours, p.NotionalValue, annualizedCost),
			CooldownTTLSeconds: cfg.FundingRateCooldown,
			Timestamp:          time.Now(),
		})
	}

	return alerts
}

func evalE008(data *AccountData, cfg *RulesConfig) e008EvalResult {
	if data.Error != nil {
		return e008EvalResult{SkipReason: "data_error"}
	}
	if len(data.FundingInfos) == 0 {
		return e008EvalResult{SkipReason: "no_funding_infos"}
	}
	if len(data.Positions) == 0 {
		return e008EvalResult{SkipReason: "no_positions"}
	}

	fundingMap := make(map[string]FundingInfo)
	for _, fi := range data.FundingInfos {
		if fi.FundingRate != 0 {
			fundingMap[fi.Symbol] = fi
		}
	}
	result := e008EvalResult{PositionCount: len(data.Positions), FundingInfoCount: len(data.FundingInfos), Threshold: cfg.FundingRateAnnualizedL2}
	for _, p := range data.Positions {
		fi, ok := fundingMap[p.Symbol]
		if !ok || fi.FundingRate == 0 {
			continue
		}
		isPaying := (p.Quantity > 0 && fi.FundingRate > 0) || (p.Quantity < 0 && fi.FundingRate < 0)
		if !isPaying {
			continue
		}
		result.PayingCount++
		intervalHours := float64(fi.FundingInterval) / 3600000.0
		if intervalHours <= 0 {
			intervalHours = 8
		}
		annualizedRate := math.Abs(fi.FundingRate) * (365.0 * 24.0 / intervalHours)
		annualizedCost := annualizedRate * p.NotionalValue
		if annualizedRate > result.MaxAnnualizedRate {
			result.MaxAnnualizedRate = annualizedRate
			result.MaxAnnualizedCost = annualizedCost
			result.TopSymbol = p.Symbol
			result.TopSide = p.PositionSide
		}
		if annualizedRate >= cfg.FundingRateAnnualizedL2 {
			result.Triggered = true
			result.TriggeredCount++
		}
	}
	if result.PayingCount == 0 {
		result.SkipReason = "no_paying_positions"
		return result
	}
	if !result.Triggered {
		result.SkipReason = "below_threshold"
	}
	return result
}

// ==================== S-014 交易所接口健康恶化 ====================

// RuleS014 检测交易所 API 延迟和错误率
//
// 当平均延迟超阈值 或 错误率超阈值 → 告警
func RuleS014(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalS014(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}
	h := data.APIHealth
	var reasons []string
	if h.AvgLatencyMs >= cfg.APILatencyThresholdMs {
		reasons = append(reasons, fmt.Sprintf("平均延迟 %.0fms ≥ %.0fms", h.AvgLatencyMs, cfg.APILatencyThresholdMs))
	}
	errorRate := float64(h.ErrorCount) / float64(h.TotalRequests)
	if errorRate >= cfg.APIErrorRateThreshold {
		reasons = append(reasons, fmt.Sprintf("错误率 %.1f%% ≥ %.1f%%", errorRate*100, cfg.APIErrorRateThreshold*100))
	}

	return []RiskAlert{{
		RuleCode:     "S-014",
		RuleName:     "交易所接口健康恶化",
		Level:        "L2",
		AccountID:    data.Account.AccountID,
		AccountLabel: data.Account.Label,
		Title:        fmt.Sprintf("账户 %s API 健康恶化", data.Account.Label),
		Message: fmt.Sprintf("%s\n请求 %d 次｜错误 %d 次｜最大延迟 %.0fms",
			strings.Join(reasons, "；"), h.TotalRequests, h.ErrorCount, h.MaxLatencyMs),
		Timestamp: time.Now(),
	}}
}

func evalS014(data *AccountData, cfg *RulesConfig) s014EvalResult {
	if data.Error != nil {
		return s014EvalResult{SkipReason: "data_error"}
	}
	if data.APIHealth == nil {
		return s014EvalResult{SkipReason: "no_api_health"}
	}
	h := data.APIHealth
	result := s014EvalResult{
		TotalRequests:    h.TotalRequests,
		ErrorCount:       h.ErrorCount,
		AvgLatencyMs:     h.AvgLatencyMs,
		MaxLatencyMs:     h.MaxLatencyMs,
		LatencyThreshold: cfg.APILatencyThresholdMs,
		ErrorThreshold:   cfg.APIErrorRateThreshold,
	}
	if h.TotalRequests < 3 {
		result.SkipReason = "insufficient_samples"
		return result
	}
	result.ErrorRate = float64(h.ErrorCount) / float64(h.TotalRequests)
	var reasons []string
	if h.AvgLatencyMs >= cfg.APILatencyThresholdMs {
		reasons = append(reasons, fmt.Sprintf("avg_latency %.2f ≥ %.2f", h.AvgLatencyMs, cfg.APILatencyThresholdMs))
	}
	if result.ErrorRate >= cfg.APIErrorRateThreshold {
		reasons = append(reasons, fmt.Sprintf("error_rate %.4f ≥ %.4f", result.ErrorRate, cfg.APIErrorRateThreshold))
	}
	if len(reasons) == 0 {
		result.SkipReason = "below_threshold"
		return result
	}
	result.Triggered = true
	result.TriggerReason = strings.Join(reasons, "; ")
	return result
}

// ==================== M-001 波动率突升 ====================

// RuleM001 检测市场价格短时跳变
//
// MVP 实现：基于 markPrice 滑动窗口，计算窗口内最早价格到当前价格的变化率
//
//	price_jump_pct = abs(current_price - window_start_price) / window_start_price
//
// 判定：price_jump_pct >= threshold → L2 告警
//
// 监控范围：账户配置的 symbol + 所有持仓的 symbol
// 通过 cooldown key（rule + symbol）自动去重。
func RuleM001(data *AccountData, cfg *RulesConfig) []RiskAlert {
	result := evalM001(data, cfg)
	if result.SkipReason != "" || !result.Triggered {
		return nil
	}

	windowDuration := time.Duration(cfg.PriceJumpWindowMinutes) * time.Minute
	now := data.CollectTime

	// 收集要检查的 symbol：账户配置的 + 所有持仓的
	symbols := make(map[string]float64) // symbol → current markPrice
	if data.Config.Symbol != "" {
		symbols[data.Config.Symbol] = 0 // 先占位，后面取价格
	}
	for _, p := range data.Positions {
		if p.MarkPrice > 0 {
			symbols[p.Symbol] = p.MarkPrice
		}
	}

	var alerts []RiskAlert
	for symbol, currentPrice := range symbols {
		history, ok := data.PriceHistory[symbol]
		if !ok || len(history) < 2 {
			continue
		}

		// 如果 currentPrice 为 0（账户配置的 symbol 但无持仓），取最新采样点
		if currentPrice <= 0 {
			currentPrice = history[len(history)-1].Price
		}

		// 找到窗口起始价格：窗口内最早的采样点
		windowStart := now.Add(-windowDuration)
		var startPrice float64
		for _, pt := range history {
			if !pt.Time.Before(windowStart) {
				startPrice = pt.Price
				break
			}
		}
		if startPrice <= 0 {
			startPrice = history[len(history)-1].Price
		}

		jumpPct := math.Abs(currentPrice-startPrice) / startPrice

		if jumpPct < cfg.PriceJumpThreshold {
			continue
		}

		direction := "↑"
		if currentPrice < startPrice {
			direction = "↓"
		}

		alerts = append(alerts, RiskAlert{
			RuleCode:     "M-001",
			RuleName:     "波动率突升",
			Level:        "L2",
			AccountID:    data.Account.AccountID,
			AccountLabel: data.Account.Label,
			Title:        fmt.Sprintf("%s %dmin 价格跳变 %s%.1f%%", symbol, cfg.PriceJumpWindowMinutes, direction, jumpPct*100),
			Message: fmt.Sprintf("%.4f → %.4f｜阈值 %.1f%%",
				startPrice, currentPrice, cfg.PriceJumpThreshold*100),
			CooldownKey:        fmt.Sprintf("sentinel:cooldown:M-001:%s", symbol), // 按 symbol 去重，不按 account
			CooldownTTLSeconds: cfg.PriceJumpCooldown,
			Timestamp:          time.Now(),
		})
	}

	return alerts
}

func evalM001(data *AccountData, cfg *RulesConfig) m001EvalResult {
	if data.Error != nil {
		return m001EvalResult{SkipReason: "data_error"}
	}
	if len(data.PriceHistory) == 0 {
		return m001EvalResult{SkipReason: "no_price_history"}
	}

	windowDuration := time.Duration(cfg.PriceJumpWindowMinutes) * time.Minute
	now := data.CollectTime
	symbols := make(map[string]float64)
	if data.Config.Symbol != "" {
		symbols[data.Config.Symbol] = 0
	}
	for _, p := range data.Positions {
		if p.MarkPrice > 0 {
			symbols[p.Symbol] = p.MarkPrice
		}
	}

	result := m001EvalResult{
		WindowMinutes: cfg.PriceJumpWindowMinutes,
		Threshold:     cfg.PriceJumpThreshold,
		SymbolCount:   len(symbols),
	}
	var preview []string
	for symbol, currentPrice := range symbols {
		history, ok := data.PriceHistory[symbol]
		if !ok || len(history) < 2 {
			continue
		}
		result.CheckedSymbols++
		prevSample := history[len(history)-2]
		latestSample := history[len(history)-1]
		if currentPrice <= 0 {
			currentPrice = latestSample.Price
		}
		windowStart := now.Add(-windowDuration)
		var startPrice float64
		for _, pt := range history {
			if !pt.Time.Before(windowStart) {
				startPrice = pt.Price
				break
			}
		}
		if startPrice <= 0 {
			startPrice = latestSample.Price
		}
		jumpPct := math.Abs(currentPrice-startPrice) / startPrice
		direction := "up"
		if currentPrice < startPrice {
			direction = "down"
		}
		if len(preview) < 5 {
			preview = append(preview, fmt.Sprintf("%s %.4f%%", symbol, jumpPct*100))
		}
		if result.TopSymbol == "" || jumpPct > result.TopJumpPct {
			result.TopJumpPct = jumpPct
			result.TopSymbol = symbol
			result.TopDirection = direction
			result.PrevSamplePrice = prevSample.Price
			result.LatestSamplePrice = latestSample.Price
			result.PrevSampleTime = prevSample.Time
			result.LatestSampleTime = latestSample.Time
			result.CurrentPrice = currentPrice
			result.StartPrice = startPrice
		}
		if jumpPct >= cfg.PriceJumpThreshold {
			result.Triggered = true
			result.TriggeredCount++
		}
	}
	result.Preview = strings.Join(preview, ", ")
	if result.CheckedSymbols == 0 {
		result.SkipReason = "insufficient_price_samples"
		return result
	}
	if !result.Triggered {
		result.SkipReason = "below_threshold"
	}
	return result
}
