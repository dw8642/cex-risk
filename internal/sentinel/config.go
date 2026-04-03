// Package sentinel 实现最小闭环风控哨兵 — 单进程 REST 轮询 + 规则引擎 + Telegram 告警
package sentinel

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// SentinelConfig 哨兵专用配置，从 config.toml 解析
type SentinelConfig struct {
	System   SystemCfg       `toml:"system"`
	Database MySQLCfg        `toml:"database"` // 复用现有 [database] 段
	Redis    RedisCfg        `toml:"redis"`
	Binance  BinanceCfg      `toml:"binance"`
	Telegram TelegramCfg     `toml:"telegram"`
	Sentinel SentinelOpts    `toml:"sentinel"`
	Accounts []AccountConfig `toml:"accounts"` // TOML 回退，MySQL 优先
	Rules    RulesConfig     `toml:"rules"`    // TOML 回退，MySQL 优先
}

type SystemCfg struct {
	Debug             bool   `toml:"debug"`
	Env               string `toml:"env"`
	HTTPProxy         string `toml:"http_proxy"`
	DebugSignRequests bool   `toml:"debug_sign_requests"`
}

// MySQLCfg MySQL 连接配置，对应 config.toml 的 [database] 段
type MySQLCfg struct {
	Host        string `toml:"host"`
	Port        string `toml:"port"`
	User        string `toml:"user"`
	Pass        string `toml:"pass"`
	Name        string `toml:"name"`
	Charset     string `toml:"charset"`
	MaxIdleConn int    `toml:"max_idle_conn"`
	MaxOpenConn int    `toml:"max_open_conn"`
	MaxLifeTime int    `toml:"max_life_time"` // 秒
}

// HasConfig 是否配置了 MySQL
func (m *MySQLCfg) HasConfig() bool {
	return m.Host != "" && m.Name != ""
}

type RedisCfg struct {
	Host     string `toml:"host"`
	Port     string `toml:"port"`
	User     string `toml:"user"`
	Pass     string `toml:"pass"`
	DB       int    `toml:"db"`
	PoolSize int    `toml:"pool_size"`
}

func (r *RedisCfg) Addr() string {
	return fmt.Sprintf("%s:%s", r.Host, r.Port)
}

type BinanceCfg struct {
	FuturesREST        string `toml:"futures_rest"`
	UseTestnet         bool   `toml:"use_testnet"`
	TestnetFuturesREST string `toml:"testnet_futures_rest"`
}

// ActiveRESTEndpoint 返回当前使用的 REST 基础 URL
func (b *BinanceCfg) ActiveRESTEndpoint() string {
	if b.UseTestnet {
		return b.TestnetFuturesREST
	}
	return b.FuturesREST
}

// PMRESTEndpoint 返回 Portfolio Margin REST 基础 URL
// PM 接口走 papi.binance.com，测试网暂无 PM 支持
func (b *BinanceCfg) PMRESTEndpoint() string {
	if b.UseTestnet {
		// 测试网无 PM，回退到 fapi
		return b.TestnetFuturesREST
	}
	return "https://papi.binance.com"
}

type TelegramCfg struct {
	BotToken string   `toml:"bot_token"`
	ChatIDs  []string `toml:"chat_ids"`
}

type SentinelOpts struct {
	PollInterval string `toml:"poll_interval"` // "10s"
	Symbol       string `toml:"symbol"`        // "BTCUSDT"
}

func (s *SentinelOpts) ParsePollInterval() time.Duration {
	d, err := time.ParseDuration(s.PollInterval)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// AccountConfig 单个监控账户配置
type AccountConfig struct {
	ID          string `toml:"id"`
	Exchange    string `toml:"exchange"`
	AccountType string `toml:"account_type"` // "regular" | "portfolio_margin"
	APIKey      string `toml:"api_key"`
	SecretKey   string `toml:"secret_key"`
	Label       string `toml:"label"`
	Symbol      string `toml:"symbol"` // 监控交易对，如 "BTCUSDT"
}

// IsPortfolioMargin 是否为统一账户
func (a *AccountConfig) IsPortfolioMargin() bool {
	return a.AccountType == "portfolio_margin"
}

// RulesConfig 规则阈值配置
type RulesConfig struct {
	// L-001
	MarginRatioThreshold float64 `toml:"margin_ratio_threshold"` // 普通合约，默认 0.5
	PMUniMMRThreshold    float64 `toml:"pm_unimmr_threshold"`    // 统一账户 uniMMR，默认 1.5
	PMStatusAlert        bool    `toml:"pm_status_alert"`        // 监控 accountStatus，默认 true

	// E-001
	DeltaChangeRateThreshold float64 `toml:"delta_change_rate_threshold"` // 默认 0.3
	DeltaAbsThreshold        float64 `toml:"delta_abs_threshold"`         // 默认 5000 USD

	// E-005
	PositionChangeThreshold float64 `toml:"position_change_threshold"` // 默认 50000 USD/min

	// E-008c
	ExpectedFundingInterval      int64 `toml:"expected_funding_interval"`       // 默认 28800000 ms (8h)
	FundingIntervalAlertCooldown int   `toml:"funding_interval_alert_cooldown"` // 默认 3600s (1h)

	// S-004
	DataGapThreshold int `toml:"data_gap_threshold"` // 默认 30s

	// L-002 爆仓距离
	LiqDistanceL2Threshold float64 `toml:"liq_distance_l2_threshold"` // L2 预警线，默认 0.05 (5%)
	LiqDistanceL3Threshold float64 `toml:"liq_distance_l3_threshold"` // L3 严重线，默认 0.02 (2%)

	// L-003 ADL 风险
	ADLQuantileL2Threshold int `toml:"adl_quantile_l2_threshold"` // L2 阈值，默认 4

	// E-008 资金费率侵蚀
	FundingRateAnnualizedL2 float64 `toml:"funding_rate_annualized_l2"` // L2 年化费率阈值，默认 0.25 (25%)
	FundingRateCooldown     int     `toml:"funding_rate_cooldown"`      // 冷却秒数，默认 1800

	// S-014 交易所接口健康
	APILatencyThresholdMs float64 `toml:"api_latency_threshold_ms"` // 平均延迟阈值，默认 2000ms
	APIErrorRateThreshold float64 `toml:"api_error_rate_threshold"` // 错误率阈值，默认 0.1 (10%)

	// M-001 波动率突升
	PriceJumpWindowMinutes int     `toml:"price_jump_window_minutes"` // 价格跳变观察窗口（分钟），默认 60
	PriceJumpThreshold     float64 `toml:"price_jump_threshold"`      // 价格跳变阈值（百分比），默认 0.10 (10%)
	PriceJumpCooldown      int     `toml:"price_jump_cooldown"`       // 告警冷却秒数，默认 600

	// 冷却
	CooldownTTL int `toml:"cooldown_ttl"` // 默认 300s

	// 每条规则的独立评估周期（从 risk_rules.config.evaluation_interval 解析）
	// key: rule_code, value: 评估间隔
	// 未配置的规则回退到全局 poll_interval
	RuleEvalIntervals map[string]time.Duration `toml:"-"`
}

// ApplyDefaults 填充默认值
func (r *RulesConfig) ApplyDefaults() {
	if r.RuleEvalIntervals == nil {
		r.RuleEvalIntervals = make(map[string]time.Duration)
	}
	if r.MarginRatioThreshold == 0 {
		r.MarginRatioThreshold = 0.5
	}
	if r.PMUniMMRThreshold == 0 {
		r.PMUniMMRThreshold = 1.5
	}
	if !r.PMStatusAlert {
		r.PMStatusAlert = true
	}
	if r.DeltaChangeRateThreshold == 0 {
		r.DeltaChangeRateThreshold = 0.3
	}
	if r.DeltaAbsThreshold == 0 {
		r.DeltaAbsThreshold = 5000
	}
	if r.PositionChangeThreshold == 0 {
		r.PositionChangeThreshold = 50000
	}
	if r.ExpectedFundingInterval == 0 {
		r.ExpectedFundingInterval = 28800000
	}
	if r.FundingIntervalAlertCooldown == 0 {
		r.FundingIntervalAlertCooldown = 3600
	}
	if r.DataGapThreshold == 0 {
		r.DataGapThreshold = 30
	}
	if r.LiqDistanceL2Threshold == 0 {
		r.LiqDistanceL2Threshold = 0.05
	}
	if r.LiqDistanceL3Threshold == 0 {
		r.LiqDistanceL3Threshold = 0.02
	}
	if r.ADLQuantileL2Threshold == 0 {
		r.ADLQuantileL2Threshold = 4
	}
	if r.FundingRateAnnualizedL2 == 0 {
		r.FundingRateAnnualizedL2 = 0.25
	}
	if r.FundingRateCooldown == 0 {
		r.FundingRateCooldown = 1800
	}
	if r.APILatencyThresholdMs == 0 {
		r.APILatencyThresholdMs = 2000
	}
	if r.APIErrorRateThreshold == 0 {
		r.APIErrorRateThreshold = 0.1
	}
	if r.PriceJumpWindowMinutes == 0 {
		r.PriceJumpWindowMinutes = 60
	}
	if r.PriceJumpThreshold == 0 {
		r.PriceJumpThreshold = 0.10
	}
	if r.PriceJumpCooldown == 0 {
		r.PriceJumpCooldown = 600
	}
	if r.CooldownTTL == 0 {
		r.CooldownTTL = 300
	}
}

// LoadSentinelConfig 从 TOML 文件加载哨兵配置
func LoadSentinelConfig(path string) (*SentinelConfig, error) {
	var cfg SentinelConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load sentinel config %s: %w", path, err)
	}
	cfg.Rules.ApplyDefaults()
	if cfg.Sentinel.PollInterval == "" {
		cfg.Sentinel.PollInterval = "10s"
	}
	if cfg.Sentinel.Symbol == "" {
		cfg.Sentinel.Symbol = "BTCUSDT"
	}
	return &cfg, nil
}
