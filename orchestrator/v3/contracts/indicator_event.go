// Package contracts 定义 Agent 间数据交换的接口契约。
package contracts

// ============================================================
// 契约 2: 指标层(Agent-I) → 规则层(Agent-R)
// Kafka Topic: indicator.events
// Redis Key: ind:{indicator_id}:{grain}
// ============================================================

// IndicatorEvent 是指标引擎产出到 Kafka 的标准事件格式。
// Agent-I 产出，Agent-R 消费。
type IndicatorEvent struct {
	// IndicatorID 指标唯一标识（如 "IND-S-001"）
	IndicatorID string `json:"indicator_id"`

	// Value 指标计算值
	Value float64 `json:"value"`

	// Grain 粒度键，格式: {exchange}:{symbol}:{account_id}
	// 部分字段可缺省，如全局指标: global
	// 示例: binance:BTC-USDT:acct001
	Grain string `json:"grain"`

	// Timestamp 计算时间戳（毫秒）
	Timestamp int64 `json:"timestamp"`

	// IsSignal 是否为 signal 类型（布尔事件而非数值指标）
	// signal 类型的 Value 约定：1.0 = 触发, 0.0 = 未触发
	IsSignal bool `json:"is_signal"`
}

// IndicatorRedisValue 是写入 Redis 的指标值格式。
// Key: ind:{indicator_id}:{grain}
type IndicatorRedisValue struct {
	// Value 当前指标值
	Value float64 `json:"value"`

	// Ts 最后更新时间戳（秒）
	Ts int64 `json:"ts"`

	// Fresh 是否新鲜（超过指标 TTL 即为 false）
	Fresh bool `json:"fresh"`
}

// Kafka Topic 命名
const (
	// TopicIndicatorEvents 指标事件 Topic
	TopicIndicatorEvents = "indicator.events"
)

// Redis Key 命名约定（指标层写入）
const (
	// RedisIndicatorKeyPattern 指标值 Key
	// 格式: ind:{indicator_id}:{grain}
	// 示例: ind:IND-S-001:binance:BTC-USDT
	RedisIndicatorKeyPattern = "ind:%s:%s"
)

// 指标时间分层
const (
	TierT0 = "T0" // tick 级（每笔推送立即计算）
	TierT1 = "T1" // 秒级（1~5s 聚合窗口）
	TierT2 = "T2" // 分钟级（1min 聚合窗口）
	TierT3 = "T3" // 小时级
	TierT4 = "T4" // 每日
)

// 指标计算层级（串行保证）
const (
	PhaseL0 = "L0" // 原始数据直接映射
	PhaseL1 = "L1" // 单数据源简单计算
	PhaseL2 = "L2" // 跨数据源或跨指标衍生
	PhaseL3 = "L3" // 复合评分（依赖 L2）
)
