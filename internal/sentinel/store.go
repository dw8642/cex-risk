// store.go — MySQL 数据访问层
//
// 从 MySQL 加载监控账户和风控规则配置
// 账户表 accounts: api_key, secret_key, exchange_id, account_type, label
// 规则表 risk_rules: rule_code, enabled, config(JSON 阈值参数)
package sentinel

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// SentinelStore MySQL 数据访问层
type SentinelStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewSentinelStore 创建 MySQL 连接
func NewSentinelStore(cfg *MySQLCfg, logger *zap.Logger) (*SentinelStore, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=true&loc=Local",
		cfg.User, cfg.Pass, cfg.Host, cfg.Port, cfg.Name, cfg.Charset)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	// 连接池配置
	db.SetMaxIdleConns(cfg.MaxIdleConn)
	db.SetMaxOpenConns(cfg.MaxOpenConn)
	db.SetConnMaxLifetime(time.Duration(cfg.MaxLifeTime) * time.Second)

	// 验证连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

	return &SentinelStore{db: db, logger: logger}, nil
}

// Close 关闭 MySQL 连接
func (s *SentinelStore) Close() error {
	return s.db.Close()
}

// LoadAccounts 从 accounts 表加载所有 active 账户
// 只加载 status='active' 且 api_key 非空的记录
func (s *SentinelStore) LoadAccounts() ([]AccountConfig, error) {
	query := `
		SELECT id, exchange_id, account_type, api_key, secret_key, label, COALESCE(symbol, '')
		FROM accounts
		WHERE status = 'active' AND api_key != '' AND secret_key != ''
		ORDER BY id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AccountConfig
	for rows.Next() {
		var a AccountConfig
		if err := rows.Scan(&a.ID, &a.Exchange, &a.AccountType, &a.APIKey, &a.SecretKey, &a.Label, &a.Symbol); err != nil {
			s.logger.Warn("扫描账户行失败", zap.Error(err))
			continue
		}
		// 去除可能的首尾空白（MySQL 存储或复制粘贴引入）
		a.APIKey = strings.TrimSpace(a.APIKey)
		a.SecretKey = strings.TrimSpace(a.SecretKey)
		// 默认值兜底
		if a.Exchange == "" {
			a.Exchange = "binance"
		}
		if a.AccountType == "" {
			a.AccountType = "regular"
		}
		accounts = append(accounts, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, nil
}

// RuleRow 从 MySQL 读取的规则行
type RuleRow struct {
	RuleCode    string
	Name        string
	Description string
	Category    string
	Priority    string
	Enabled     bool
	Config      map[string]interface{} // JSON 解析后的阈值参数
}

// LoadEnabledRules 从 risk_rules 表加载所有启用的规则
func (s *SentinelStore) LoadEnabledRules() ([]RuleRow, error) {
	query := `
		SELECT rule_code, name, description, category, priority, enabled, config
		FROM risk_rules
		WHERE enabled = 1
		ORDER BY rule_code
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query risk_rules: %w", err)
	}
	defer rows.Close()

	var rules []RuleRow
	for rows.Next() {
		var r RuleRow
		var configJSON sql.NullString
		if err := rows.Scan(&r.RuleCode, &r.Name, &r.Description, &r.Category, &r.Priority, &r.Enabled, &configJSON); err != nil {
			s.logger.Warn("扫描规则行失败", zap.Error(err))
			continue
		}

		// 解析 config JSON
		r.Config = make(map[string]interface{})
		if configJSON.Valid && configJSON.String != "" && configJSON.String != "{}" {
			if err := json.Unmarshal([]byte(configJSON.String), &r.Config); err != nil {
				s.logger.Warn("解析规则 config JSON 失败",
					zap.String("rule", r.RuleCode), zap.Error(err))
			}
		}

		rules = append(rules, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate risk_rules: %w", err)
	}

	return rules, nil
}

// BuildRulesConfig 从 MySQL 规则行构建 RulesConfig
// 将各规则的 config JSON 中的阈值合并到统一的 RulesConfig 结构
// 优先级: MySQL risk_rules.config > TOML [rules] > 硬编码默认值
func BuildRulesConfig(rows []RuleRow, defaults *RulesConfig) *RulesConfig {
	cfg := *defaults // 复制默认值作为基底
	if cfg.RuleEvalIntervals == nil {
		cfg.RuleEvalIntervals = make(map[string]time.Duration)
	}

	for _, r := range rows {
		// 通用：解析 evaluation_interval（所有规则共用逻辑）
		if v, ok := r.Config["evaluation_interval"]; ok {
			if s, ok := v.(string); ok && s != "" {
				if d, err := time.ParseDuration(s); err == nil && d > 0 {
					cfg.RuleEvalIntervals[r.RuleCode] = d
				}
			}
		}

		switch r.RuleCode {
		case "L-001":
			if v, ok := getFloat(r.Config, "margin_ratio_threshold"); ok {
				cfg.MarginRatioThreshold = v
			}
			if v, ok := getFloat(r.Config, "pm_unimmr_threshold"); ok {
				cfg.PMUniMMRThreshold = v
			}
			if v, ok := r.Config["pm_status_alert"].(bool); ok {
				cfg.PMStatusAlert = v
			}

		case "E-001":
			if v, ok := getFloat(r.Config, "delta_change_rate_threshold"); ok {
				cfg.DeltaChangeRateThreshold = v
			}
			if v, ok := getFloat(r.Config, "delta_abs_threshold"); ok {
				cfg.DeltaAbsThreshold = v
			}

		case "E-005":
			if v, ok := getFloat(r.Config, "position_change_threshold"); ok {
				cfg.PositionChangeThreshold = v
			}

		case "E-008c":
			if v, ok := getFloat(r.Config, "expected_funding_interval"); ok {
				cfg.ExpectedFundingInterval = int64(v)
			}
			if v, ok := getFloat(r.Config, "funding_interval_alert_cooldown"); ok {
				cfg.FundingIntervalAlertCooldown = int(v)
			}

		case "S-004":
			if v, ok := getFloat(r.Config, "data_gap_threshold"); ok {
				cfg.DataGapThreshold = int(v)
			}

		case "L-002":
			if v, ok := getFloat(r.Config, "liq_distance_l2_threshold"); ok {
				cfg.LiqDistanceL2Threshold = v
			}
			if v, ok := getFloat(r.Config, "liq_distance_l3_threshold"); ok {
				cfg.LiqDistanceL3Threshold = v
			}

		case "L-003":
			if v, ok := getFloat(r.Config, "adl_quantile_l2_threshold"); ok {
				cfg.ADLQuantileL2Threshold = int(v)
			}

		case "E-008":
			if v, ok := getFloat(r.Config, "funding_rate_annualized_l2"); ok {
				cfg.FundingRateAnnualizedL2 = v
			}
			if v, ok := getFloat(r.Config, "funding_rate_cooldown"); ok {
				cfg.FundingRateCooldown = int(v)
			}

		case "S-014":
			if v, ok := getFloat(r.Config, "api_latency_threshold_ms"); ok {
				cfg.APILatencyThresholdMs = v
			}
			if v, ok := getFloat(r.Config, "api_error_rate_threshold"); ok {
				cfg.APIErrorRateThreshold = v
			}

		case "M-001":
			if v, ok := getFloat(r.Config, "price_jump_window_minutes"); ok {
				cfg.PriceJumpWindowMinutes = int(v)
			}
			if v, ok := getFloat(r.Config, "price_jump_threshold"); ok {
				cfg.PriceJumpThreshold = v
			}
			if v, ok := getFloat(r.Config, "price_jump_cooldown"); ok {
				cfg.PriceJumpCooldown = int(v)
			}
		}
	}

	cfg.ApplyDefaults()

	return &cfg
}

// LoadSentinelGlobalConfig 从 sentinel_config 表加载全局配置
// 返回 key→value 映射，调用方负责合并到 RulesConfig/SentinelOpts
func (s *SentinelStore) LoadSentinelGlobalConfig() (map[string]string, error) {
	query := `SELECT ` + "`key`" + `, value FROM sentinel_config`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query sentinel_config: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			s.logger.Warn("扫描 sentinel_config 行失败", zap.Error(err))
			continue
		}
		result[k] = v
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sentinel_config: %w", err)
	}
	return result, nil
}

// ApplyGlobalConfig 将 sentinel_config 表中的全局配置合并到 RulesConfig
func ApplyGlobalConfig(cfg *RulesConfig, globalCfg map[string]string) {
	if v, ok := globalCfg["cooldown_ttl"]; ok {
		if n := parseInt(v); n > 0 {
			cfg.CooldownTTL = n
		}
	}
}

// ReloadRulesFromDB 从 MySQL 重新加载规则配置（用于热重载）
// 返回更新后的规则列表和配置，调用方负责替换 Engine 中的引用
func (s *SentinelStore) ReloadRulesFromDB(defaults *RulesConfig) ([]RuleDefinition, *RulesConfig, error) {
	// 1. 加载启用的规则
	dbRuleRows, err := s.LoadEnabledRules()
	if err != nil {
		return nil, nil, fmt.Errorf("reload rules: %w", err)
	}

	// 2. 构建阈值配置
	rulesCfg := BuildRulesConfig(dbRuleRows, defaults)

	// 3. 加载全局配置覆盖
	globalCfg, err := s.LoadSentinelGlobalConfig()
	if err != nil {
		s.logger.Warn("热重载: 加载全局配置失败，使用默认值", zap.Error(err))
	} else {
		ApplyGlobalConfig(rulesCfg, globalCfg)
	}

	// 4. 按 enabled 状态过滤规则
	rules := FilterRulesByDB(AllRules(), dbRuleRows)

	s.logger.Info("规则热重载完成",
		zap.Int("enabled_rules", len(rules)),
		zap.Int("db_rule_rows", len(dbRuleRows)),
		zap.Int("cooldown_ttl", rulesCfg.CooldownTTL))

	return rules, rulesCfg, nil
}

// FilterRulesByDB 根据 MySQL 中启用的规则过滤代码侧的规则列表
// 只保留 MySQL risk_rules 表中 enabled=1 的规则
func FilterRulesByDB(allRules []RuleDefinition, dbRows []RuleRow) []RuleDefinition {
	enabledSet := make(map[string]bool, len(dbRows))
	for _, r := range dbRows {
		enabledSet[r.RuleCode] = true
	}

	var filtered []RuleDefinition
	for _, r := range allRules {
		if enabledSet[r.Code] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// getFloat 从 map[string]interface{} 中安全提取 float64
// JSON 数字默认解析为 float64
func getFloat(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}
