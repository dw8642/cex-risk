package models

import "time"

// Project 做市项目
type Project struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Status      string    `json:"status" db:"status"` // active / paused / archived
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Partner 执行合作伙伴
type Partner struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Contact   string    `json:"contact" db:"contact"`
	RiskLevel string    `json:"risk_level" db:"risk_level"` // low / medium / high / critical
	Status    string    `json:"status" db:"status"`         // active / suspended / terminated
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Account 交易所监控账户
type Account struct {
	ID         string    `json:"id" db:"id"`
	ProjectID  string    `json:"project_id" db:"project_id"`
	PartnerID  string    `json:"partner_id" db:"partner_id"`
	ExchangeID string    `json:"exchange_id" db:"exchange_id"`
	MarketType string    `json:"market_type" db:"market_type"` // spot / futures / margin
	Label      string    `json:"label" db:"label"`
	Status     string    `json:"status" db:"status"` // active / paused / disabled
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// APIKeyConfig API Key 预期配置（不存放明文 key）
type APIKeyConfig struct {
	ID                    string   `json:"id" db:"id"`
	AccountID             string   `json:"account_id" db:"account_id"`
	KeyLabel              string   `json:"key_label" db:"key_label"`
	KeyHash               string   `json:"key_hash" db:"key_hash"`
	ExpectedPermissions   map[string]bool `json:"expected_permissions"`
	ExpectedIPWhitelist   []string        `json:"expected_ip_whitelist"`
	AllowedSymbols        []string        `json:"allowed_symbols"`
	Status                string   `json:"status" db:"status"` // active / revoked / expired
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

// IsWithdrawAllowed 预期是否允许提币
func (a *APIKeyConfig) IsWithdrawAllowed() bool {
	if a.ExpectedPermissions == nil {
		return false
	}
	return a.ExpectedPermissions["withdraw"]
}

// IsSymbolAllowed 检查某个币对是否在允许列表中
func (a *APIKeyConfig) IsSymbolAllowed(symbol string) bool {
	if len(a.AllowedSymbols) == 0 {
		return true // 未配置限制则全部允许
	}
	for _, s := range a.AllowedSymbols {
		if s == symbol {
			return true
		}
	}
	return false
}
