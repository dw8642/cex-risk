// engine.go — 规则调度引擎
//
// 核心循环：Collector.CollectAll → 逐账户执行 Rules → Alerter.Send
// 支持 --dry-run 模式（仅打印，不发 Telegram）
// 支持规则热重载：定期从 MySQL 重新加载规则配置，无需重启
package sentinel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Engine 规则调度引擎
type Engine struct {
	collector    *Collector
	alerter      *TelegramAlerter
	rules        []RuleDefinition
	rulesCfg     *RulesConfig
	pollInterval time.Duration
	dryRun       bool
	logger       *zap.Logger

	// 热重载相关
	store          *SentinelStore // MySQL 连接，nil 则不热重载
	defaultRules   *RulesConfig   // TOML 原始默认值（作为 merge 基底）
	reloadInterval time.Duration  // 热重载间隔，0 则不热重载
	mu             sync.RWMutex   // 保护 rules 和 rulesCfg 的并发读写

	// 每条规则的上次评估时间（支持独立评估周期）
	lastRuleEval map[string]time.Time
}

// NewEngine 创建引擎
func NewEngine(collector *Collector, alerter *TelegramAlerter, rules []RuleDefinition,
	rulesCfg *RulesConfig, pollInterval time.Duration, logger *zap.Logger) *Engine {
	return &Engine{
		collector:    collector,
		alerter:      alerter,
		rules:        rules,
		rulesCfg:     rulesCfg,
		pollInterval: pollInterval,
		logger:       logger,
		lastRuleEval: make(map[string]time.Time),
	}
}

// SetDryRun 设置 dry-run 模式
func (e *Engine) SetDryRun(dryRun bool) {
	e.dryRun = dryRun
}

// EnableHotReload 启用规则热重载
// store: MySQL 连接（用于定期读取 risk_rules + sentinel_config）
// defaultRules: TOML 原始默认值（merge 基底）
// interval: 热重载间隔
func (e *Engine) EnableHotReload(store *SentinelStore, defaultRules *RulesConfig, interval time.Duration) {
	e.store = store
	e.defaultRules = defaultRules
	e.reloadInterval = interval
	e.logger.Info("规则热重载已启用", zap.Duration("reload_interval", interval))
}

// reloadRules 从 MySQL 重新加载规则和配置
func (e *Engine) reloadRules() {
	if e.store == nil {
		return
	}

	rules, rulesCfg, err := e.store.ReloadRulesFromDB(e.defaultRules)
	if err != nil {
		e.logger.Error("规则热重载失败，保持当前配置", zap.Error(err))
		return
	}

	e.mu.Lock()
	e.rules = rules
	e.rulesCfg = rulesCfg
	// 同步更新 alerter 的冷却时间
	e.alerter.UpdateCooldownTTL(time.Duration(rulesCfg.CooldownTTL) * time.Second)
	e.mu.Unlock()
}

// getRulesSnapshot 获取当前规则快照（线程安全）
func (e *Engine) getRulesSnapshot() ([]RuleDefinition, *RulesConfig) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.rules, e.rulesCfg
}

// RunOnce 执行一轮完整的采集→评估→告警
func (e *Engine) RunOnce(ctx context.Context) {
	start := time.Now()

	// 获取当前规则快照（热重载安全）
	rules, rulesCfg := e.getRulesSnapshot()

	// 1. 采集所有账户数据
	allData := e.collector.CollectAll(ctx)

	// 2. 逐账户评估规则
	var allAlerts []RiskAlert
	for i := range allData {
		data := &allData[i]

		if data.Error != nil {
			// 数据异常：仅执行 S-004（风控失明检测）
			for _, rule := range rules {
				if rule.Code == "S-004" {
					alerts := rule.Fn(data, rulesCfg)
					allAlerts = append(allAlerts, alerts...)
				}
			}
			continue
		}

		// 正常数据：执行所有规则（按各规则独立评估周期调度）
		for _, rule := range rules {
			// 检查规则是否到达评估周期
			if !e.isRuleDue(rule.Code, rulesCfg, start) {
				continue
			}
			if rule.Code == "P-001" {
				result := evalP001(data)
				e.logger.Debug("P-001 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Bool("enable_withdraw", result.EnableWithdraw),
					zap.Bool("ip_restrict", result.IPRestrict),
					zap.Bool("triggered", result.Triggered),
				)
			}

			if rule.Code == "L-001" {
				result := evalL001(data, rulesCfg)
				e.logger.Debug("L-001 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("account_type", result.AccountType),
					zap.String("skip_reason", result.SkipReason),
					zap.String("trigger_branch", result.TriggerBranch),
					zap.String("trigger_level", result.TriggerLevel),
					zap.Float64("margin_ratio", result.MarginRatio),
					zap.Float64("margin_ratio_threshold", result.MarginThreshold),
					zap.Float64("uniMMR", result.UniMMR),
					zap.Float64("uniMMR_threshold", result.UniMMRThreshold),
					zap.String("account_status", result.AccountStatus),
					zap.Float64("total_equity", result.TotalEquity),
					zap.Float64("maint_margin", result.MaintMargin),
					zap.Float64("margin_balance", result.MarginBalance),
					zap.Bool("triggered", result.Triggered),
					zap.String("trigger_reason", result.TriggerReason),
				)
			}

			if rule.Code == "E-001" {
				result := evalE001(data, rulesCfg)
				e.logger.Debug("E-001 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Float64("prev_delta", result.PrevDelta),
					zap.Float64("current_delta", result.CurrentDelta),
					zap.Float64("delta_change", result.DeltaChange),
					zap.Float64("abs_delta_change", result.AbsDeltaChange),
					zap.Float64("change_rate", result.ChangeRate),
					zap.Float64("abs_change_rate", result.AbsChangeRate),
					zap.Float64("delta_change_rate_threshold", rulesCfg.DeltaChangeRateThreshold),
					zap.Float64("delta_abs_threshold", rulesCfg.DeltaAbsThreshold),
					zap.Bool("rate_triggered", result.RateTriggered),
					zap.Bool("abs_triggered", result.AbsTriggered),
					zap.Bool("triggered", result.Triggered),
					zap.String("trigger_reason", result.TriggerReason),
				)
			}

			if rule.Code == "E-005" {
				result := evalE005(data, rulesCfg)
				e.logger.Debug("E-005 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Float64("total_change", result.TotalChange),
					zap.Float64("threshold", result.Threshold),
					zap.Int("current_count", result.CurrentCount),
					zap.Int("prev_count", result.PrevCount),
					zap.Int("changed_legs", result.ChangedLegs),
					zap.Int("closed_legs", result.ClosedLegs),
					zap.Bool("triggered", result.Triggered),
					zap.String("trigger_reason", result.TriggerReason),
				)
			}

			if rule.Code == "E-008c" {
				result := evalE008c(data, rulesCfg)
				e.logger.Debug("E-008c 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Int("mismatch_count", result.MismatchCount),
					zap.Float64("expected_hours", result.ExpectedHours),
					zap.String("distribution", result.Distribution),
					zap.String("preview", result.Preview),
					zap.Bool("triggered", result.Triggered),
				)
			}

			if rule.Code == "L-002" {
				result := evalL002(data, rulesCfg)
				e.logger.Debug("L-002 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Int("position_count", result.PositionCount),
					zap.Int("checked_count", result.CheckedCount),
					zap.Int("triggered_count", result.TriggeredCount),
					zap.Float64("min_distance_pct", result.MinDistancePct),
					zap.Float64("l2_threshold", result.L2Threshold),
					zap.Float64("l3_threshold", result.L3Threshold),
					zap.String("highest_level", result.HighestLevel),
					zap.String("closest_symbol", result.ClosestSymbol),
					zap.String("closest_side", result.ClosestSide),
					zap.Float64("closest_mark_price", result.ClosestMarkPrice),
					zap.Float64("closest_liq_price", result.ClosestLiqPrice),
					zap.Bool("triggered", result.Triggered),
				)
			}

			if rule.Code == "L-003" {
				result := evalL003(data, rulesCfg)
				e.logger.Debug("L-003 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Int("position_count", result.PositionCount),
					zap.Int("checked_count", result.CheckedCount),
					zap.Int("triggered_count", result.TriggeredCount),
					zap.Int("threshold", result.Threshold),
					zap.Int("max_adl_quantile", result.MaxADLQuantile),
					zap.String("top_symbol", result.TopSymbol),
					zap.String("top_side", result.TopSide),
					zap.Bool("triggered", result.Triggered),
				)
			}

			if rule.Code == "E-008" {
				result := evalE008(data, rulesCfg)
				e.logger.Debug("E-008 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Int("position_count", result.PositionCount),
					zap.Int("funding_info_count", result.FundingInfoCount),
					zap.Int("paying_count", result.PayingCount),
					zap.Int("triggered_count", result.TriggeredCount),
					zap.Float64("threshold", result.Threshold),
					zap.Float64("max_annualized_rate", result.MaxAnnualizedRate),
					zap.Float64("max_annualized_cost", result.MaxAnnualizedCost),
					zap.String("top_symbol", result.TopSymbol),
					zap.String("top_side", result.TopSide),
					zap.Bool("triggered", result.Triggered),
				)
			}

			if rule.Code == "S-014" {
				result := evalS014(data, rulesCfg)
				e.logger.Debug("S-014 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Int("total_requests", result.TotalRequests),
					zap.Int("error_count", result.ErrorCount),
					zap.Float64("error_rate", result.ErrorRate),
					zap.Float64("avg_latency_ms", result.AvgLatencyMs),
					zap.Float64("max_latency_ms", result.MaxLatencyMs),
					zap.Float64("latency_threshold", result.LatencyThreshold),
					zap.Float64("error_threshold", result.ErrorThreshold),
					zap.Bool("triggered", result.Triggered),
					zap.String("trigger_reason", result.TriggerReason),
				)
			}

			if rule.Code == "M-001" {
				result := evalM001(data, rulesCfg)
				e.logger.Debug("M-001 评估",
					zap.String("account_id", data.Account.AccountID),
					zap.String("account_label", data.Account.Label),
					zap.String("skip_reason", result.SkipReason),
					zap.Int("window_minutes", result.WindowMinutes),
					zap.Float64("threshold", result.Threshold),
					zap.Int("symbol_count", result.SymbolCount),
					zap.Int("checked_symbols", result.CheckedSymbols),
					zap.Int("triggered_count", result.TriggeredCount),
					zap.String("top_symbol", result.TopSymbol),
					zap.Float64("top_jump_pct", result.TopJumpPct),
					zap.String("top_direction", result.TopDirection),
					zap.Float64("prev_sample_price", result.PrevSamplePrice),
					zap.Float64("latest_sample_price", result.LatestSamplePrice),
					zap.Time("prev_sample_time", result.PrevSampleTime),
					zap.Time("latest_sample_time", result.LatestSampleTime),
					zap.Float64("start_price", result.StartPrice),
					zap.Float64("current_price", result.CurrentPrice),
					zap.String("preview", result.Preview),
					zap.Bool("triggered", result.Triggered),
				)
			}

			alerts := rule.Fn(data, rulesCfg)
			allAlerts = append(allAlerts, alerts...)

			// 记录本次评估时间（只在最后一个账户处理完后记录，避免重复）
			if i == len(allData)-1 {
				e.lastRuleEval[rule.Code] = start
			}
		}
	}

	// 3. 发送告警
	sentCount := 0
	cooledCount := 0
	for _, alert := range allAlerts {
		if e.dryRun {
			// Dry-run 模式：打印到 stdout
			fmt.Printf("[DRY-RUN] %s\n", FormatAlert(alert))
			sentCount++
			continue
		}

		sent, err := e.alerter.Send(ctx, alert)
		if err != nil {
			e.logger.Error("发送告警失败",
				zap.String("rule", alert.RuleCode),
				zap.String("account", alert.AccountLabel),
				zap.Error(err))
			continue
		}
		if sent {
			sentCount++
		} else {
			cooledCount++
		}
	}

	elapsed := time.Since(start)
	e.logger.Info("引擎周期完成",
		zap.Int("accounts", len(allData)),
		zap.Int("alerts_total", len(allAlerts)),
		zap.Int("alerts_sent", sentCount),
		zap.Int("alerts_cooled", cooledCount),
		zap.Duration("elapsed", elapsed))
}

// isRuleDue 判断规则是否到达独立评估周期
// 优先使用 RuleEvalIntervals[ruleCode]，未配置则回退到全局 pollInterval（即每轮都执行）
func (e *Engine) isRuleDue(ruleCode string, cfg *RulesConfig, now time.Time) bool {
	interval, hasInterval := cfg.RuleEvalIntervals[ruleCode]
	if !hasInterval || interval <= 0 {
		return true // 没配独立周期，每轮都执行
	}

	// 如果独立周期 <= 全局 pollInterval，等效于每轮执行
	if interval <= e.pollInterval {
		return true
	}

	lastEval, evaluated := e.lastRuleEval[ruleCode]
	if !evaluated {
		return true // 首次执行
	}

	return now.Sub(lastEval) >= interval
}

// Start 启动引擎主循环
// 阻塞运行，直到 ctx 被取消
func (e *Engine) Start(ctx context.Context) {
	rules, _ := e.getRulesSnapshot()
	e.logger.Info("Sentinel 引擎启动",
		zap.Duration("poll_interval", e.pollInterval),
		zap.Int("rules", len(rules)),
		zap.Bool("dry_run", e.dryRun),
		zap.Bool("hot_reload", e.store != nil && e.reloadInterval > 0))

	// 立即执行第一轮
	e.RunOnce(ctx)

	if e.dryRun {
		e.logger.Info("Dry-run 模式，单次执行完毕")
		return
	}

	// 启动规则热重载协程
	if e.store != nil && e.reloadInterval > 0 {
		go e.runReloadLoop(ctx)
	}

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("Sentinel 引擎停止", zap.Error(ctx.Err()))
			return
		case <-ticker.C:
			e.RunOnce(ctx)
		}
	}
}

// runReloadLoop 规则热重载循环
// 独立 goroutine，定期从 MySQL 重新加载 risk_rules + sentinel_config
func (e *Engine) runReloadLoop(ctx context.Context) {
	ticker := time.NewTicker(e.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.reloadRules()
		}
	}
}
