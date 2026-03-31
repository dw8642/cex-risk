package models

import (
	"encoding/json"
	"time"
)

// RiskEvent 风险事件 — 规则引擎检测到风险后生成
type RiskEvent struct {
	ID         string                 `json:"id" db:"id"`
	EventCode  string                 `json:"event_code" db:"event_code"`
	Level      string                 `json:"level" db:"level"`         // P0 / P1 / P2 / P3
	Status     string                 `json:"status" db:"status"`       // open / ack / handling / resolved / false_positive
	ObjectType string                 `json:"object_type" db:"object_type"` // account / partner / system
	ObjectID   string                 `json:"object_id" db:"object_id"`
	ProjectID  string                 `json:"project_id" db:"project_id"`
	Title      string                 `json:"title" db:"title"`
	Details    map[string]interface{} `json:"details" db:"details"`
	AckBy      string                 `json:"ack_by,omitempty" db:"ack_by"`
	AckAt      *time.Time             `json:"ack_at,omitempty" db:"ack_at"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty" db:"resolved_at"`
	CreatedAt  time.Time              `json:"created_at" db:"created_at"`
}

func (r *RiskEvent) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

func UnmarshalRiskEvent(data []byte) (*RiskEvent, error) {
	var e RiskEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// IsP0 是否为最高优先级
func (r *RiskEvent) IsP0() bool {
	return r.Level == "P0"
}

// RuleConfig 规则配置 — 对应 risk_rules 表
type RuleConfig struct {
	ID          string                 `json:"id" db:"id"`
	RuleCode    string                 `json:"rule_code" db:"rule_code"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	Category    string                 `json:"category" db:"category"` // permission / behavior / system / exposure / liquidation
	Priority    string                 `json:"priority" db:"priority"`
	Enabled     bool                   `json:"enabled" db:"enabled"`
	Config      map[string]interface{} `json:"config" db:"config"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// NotificationLog 通知记录
type NotificationLog struct {
	ID        string     `json:"id" db:"id"`
	EventID   string     `json:"event_id" db:"event_id"`
	Channel   string     `json:"channel" db:"channel"` // telegram_msg / telegram_voice / twilio / email
	Recipient string     `json:"recipient" db:"recipient"`
	Status    string     `json:"status" db:"status"` // pending / sent / delivered / failed
	ErrorMsg  string     `json:"error_msg,omitempty" db:"error_msg"`
	SentAt    *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// AuditLog 审计日志（不可变）
type AuditLog struct {
	ID        int64                  `json:"id" db:"id"`
	Timestamp time.Time              `json:"timestamp" db:"timestamp"`
	Actor     string                 `json:"actor" db:"actor"`     // system / user / api
	Action    string                 `json:"action" db:"action"`
	Resource  string                 `json:"resource" db:"resource"`
	Detail    map[string]interface{} `json:"detail,omitempty" db:"detail"`
	IP        string                 `json:"ip,omitempty" db:"ip"`
}
