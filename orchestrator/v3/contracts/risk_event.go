// Package contracts 定义 Agent 间数据交换的接口契约。
package contracts

// ============================================================
// 契约 3: 规则层(Agent-R) → 告警/API(Agent-W)
// Kafka Topic: risk.events
// MySQL Table: risk_events
// ============================================================

// RiskEvent 是规则引擎产出的风险事件标准格式。
// Agent-R 产出，Agent-W API 读取，Alert Service 消费。
type RiskEvent struct {
	// EventID 事件唯一 ID（UUID v4）
	EventID string `json:"event_id"`

	// RuleCode 触发的规则编码（如 "S-014"）
	RuleCode string `json:"rule_code"`

	// Severity 严重级别: L1(信息) | L2(警告) | L3(严重)
	Severity string `json:"severity"`

	// Priority 响应优先级: P0(立即) | P1(紧急) | P2(标准) | P3(低)
	Priority string `json:"priority"`

	// ScopeType 作用域类型: global | project | team | strategy | account
	ScopeType string `json:"scope_type"`

	// ScopeID 作用域 ID（如 account_id, project_id）
	ScopeID string `json:"scope_id"`

	// TriggerValue 触发时的指标值
	TriggerValue float64 `json:"trigger_value"`

	// Threshold 对应的阈值
	Threshold float64 `json:"threshold"`

	// Message 人类可读的告警消息（中文）
	Message string `json:"message"`

	// Status 事件状态: open | ack | handling | resolved | false_positive
	Status string `json:"status"`

	// CreatedAt 事件创建时间戳（毫秒）
	CreatedAt int64 `json:"created_at"`

	// ResolvedAt 事件解决时间戳（毫秒），未解决时为 0
	ResolvedAt int64 `json:"resolved_at,omitempty"`

	// AckedBy 确认人（用户 ID）
	AckedBy string `json:"acked_by,omitempty"`

	// AlertLevel 实际触发的告警级别: L1 | L2 | L3
	// 由规则引擎根据 TriggerValue 与多级阈值判定
	AlertLevel string `json:"alert_level"`

	// Audience 告警受众: ops | trader | risk | all
	// 运行时由通知路由模块根据 rule_code × alert_level 查询
	// 优先使用 DB 自定义配置，回退到代码默认映射
	Audience string `json:"audience"`
}

// 严重级别常量
const (
	SeverityL1 = "L1" // 信息
	SeverityL2 = "L2" // 警告
	SeverityL3 = "L3" // 严重
)

// 响应优先级常量
const (
	PriorityP0 = "P0" // 立即响应（系统失明/保命约束触发）
	PriorityP1 = "P1" // 紧急（清算风险/权限异常）
	PriorityP2 = "P2" // 标准（阈值越限）
	PriorityP3 = "P3" // 低（信息性通知）
)

// 告警级别常量
const (
	AlertLevelL1 = "L1" // Dashboard 展示
	AlertLevelL2 = "L2" // Dashboard + Telegram 文本
	AlertLevelL3 = "L3" // Dashboard + Telegram 文本 + Telegram 语音
)

// 告警受众常量
const (
	AudienceOps    = "ops"    // 系统运维（DevOps / SRE）
	AudienceTrader = "trader" // 交易员（策略负责人 / 执行团队）
	AudienceRisk   = "risk"   // 风控负责人（老板 / 风控主管）
	AudienceAll    = "all"    // 全员（致命风险）
)

// 事件状态常量
const (
	StatusOpen          = "open"
	StatusAcknowledged  = "ack"
	StatusHandling      = "handling"
	StatusResolved      = "resolved"
	StatusFalsePositive = "false_positive"
)

// 作用域类型常量
const (
	ScopeGlobal   = "global"
	ScopeProject  = "project"
	ScopeTeam     = "team"
	ScopeStrategy = "strategy"
	ScopeAccount  = "account"
)

// Kafka Topic
const (
	TopicRiskEvents = "risk.events"
)

// Redis Key（Cooldown 去重）
const (
	// RedisCooldownKeyPattern 告警去重 Key（基础版）
	// 格式: cooldown:{rule_code}:{scope_type}:{scope_id}
	// TTL: 规则配置的 cooldown 时间（默认 300s）
	RedisCooldownKeyPattern = "cooldown:%s:%s:%s"

	// RedisCooldownKeyPatternV2 告警去重 Key（含受众+级别，独立冷却）
	// 格式: cooldown:{rule_code}:{scope_type}:{scope_id}:{audience}:{level}
	// 保证同一事件在不同受众和不同级别之间独立冷却
	RedisCooldownKeyPatternV2 = "cooldown:%s:%s:%s:%s:%s"
)
