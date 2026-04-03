// Package contracts 定义 Agent 间数据交换的接口契约。
package contracts

// ============================================================
// 契约 4: 健康治理(Agent-R) → Dashboard API(Agent-W)
// Redis Key: health:score / health:state / health:dimensions
// ============================================================

// HealthScore 是健康度双分数模型的输出格式。
// Agent-R health/scorer.go 产出，Agent-W API 读取。
type HealthScore struct {
	// InternalHealthScore 内部健康度评分 (0~100)
	// 五维加权: data_quality(25%) + system_stability(25%) +
	//           exchange_connectivity(20%) + account_health(15%) + alert_load(15%)
	InternalHealthScore float64 `json:"internal_health_score"`

	// ExternalDependencySummary 外部依赖摘要
	ExternalDependencySummary ExternalDependency `json:"external_dependency_summary"`

	// State 运行态: healthy | internal_degraded | external_degraded | critical
	State string `json:"state"`

	// Dimensions 五维度明细
	Dimensions []HealthDimension `json:"dimensions"`

	// HardCapTriggered 是否触发 Hard Cap
	HardCapTriggered bool `json:"hard_cap_triggered"`

	// HardCapReason Hard Cap 触发原因（为空表示未触发）
	HardCapReason string `json:"hard_cap_reason,omitempty"`

	// UpdatedAt 最后更新时间戳（毫秒）
	UpdatedAt int64 `json:"updated_at"`
}

// ExternalDependency 外部依赖状态摘要
type ExternalDependency struct {
	// ExchangeAPI 交易所 API 连通性
	ExchangeAPI string `json:"exchange_api"` // ok | degraded | down

	// TelegramBot Telegram 通知通道状态
	TelegramBot string `json:"telegram_bot"` // ok | degraded | down

	// ThirdPartyData 第三方数据源状态
	ThirdPartyData string `json:"third_party_data"` // ok | degraded | down
}

// HealthDimension 单个健康维度详情
type HealthDimension struct {
	// Name 维度名称
	Name string `json:"name"`

	// Weight 权重 (0~1)
	Weight float64 `json:"weight"`

	// Score 该维度得分 (0~100)
	Score float64 `json:"score"`

	// Items 该维度下的检查项明细
	Items []HealthCheckItem `json:"items"`
}

// HealthCheckItem 单个检查项
type HealthCheckItem struct {
	// Name 检查项名称
	Name string `json:"name"`

	// Status 状态: ok | warning | critical
	Status string `json:"status"`

	// Value 当前值
	Value string `json:"value"`

	// Detail 补充说明
	Detail string `json:"detail,omitempty"`
}

// 运行态常量
const (
	StateHealthy           = "healthy"
	StateInternalDegraded  = "internal_degraded"
	StateExternalDegraded  = "external_degraded"
	StateCritical          = "critical"
)

// Hard Cap 触发条件
const (
	HardCapDataBlind       = "任意数据维度 > 60s 无更新"
	HardCapAllRulesFail    = "全部规则评估失败"
	HardCapTelegramDown    = "Telegram 通道连续 30min 不可达"
	HardCapDBUnreachable   = "MySQL/Redis 不可连接"
	HardCapScoreBelowFloor = "加权分 < 20（已在危险区）"
)

// 健康度维度名称
const (
	DimDataQuality          = "data_quality"
	DimSystemStability      = "system_stability"
	DimExchangeConnectivity = "exchange_connectivity"
	DimAccountHealth        = "account_health"
	DimAlertLoad            = "alert_load"
)

// Redis Key（健康度写入）
const (
	RedisHealthScore      = "health:score"       // HealthScore JSON
	RedisHealthState      = "health:state"        // 当前状态字符串
	RedisHealthDimensions = "health:dimensions"   // []HealthDimension JSON
	RedisHealthHistory    = "health:history:%d"   // 历史记录, %d = 时间戳
)
