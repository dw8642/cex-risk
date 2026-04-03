// Package contracts 定义 Agent 间数据交换的接口契约。
// 这些结构体是 Agent 间解耦的关键——每个 Agent 必须严格遵守此处定义的格式。
// 修改此文件需要 Orchestrator 审批。
package contracts

import "encoding/json"

// ============================================================
// 契约 1: 数据层(Agent-D) → 指标层(Agent-I)
// Kafka Topic: public.{exchange}.{data_type} / private.{exchange}.{data_type}
// ============================================================

// MarketDataMessage 是数据层发布到 Kafka 的标准消息格式。
// Agent-D 产出，Agent-I 消费。
type MarketDataMessage struct {
	// Exchange 交易所标识（如 "binance"）
	Exchange string `json:"exchange"`

	// Symbol 交易对（如 "BTC-USDT"）
	Symbol string `json:"symbol"`

	// DataType 数据类型，枚举值：
	//   公共: orderbook, trade, mark_price, ticker, funding_rate, open_interest, exchange_info
	//   私有: order, position, balance, account_update
	DataType string `json:"data_type"`

	// Data 原始数据（各类型结构不同，由消费者按 DataType 解析）
	Data json.RawMessage `json:"data"`

	// ExchangeTS 交易所端时间戳（毫秒）
	ExchangeTS int64 `json:"exchange_ts"`

	// ReceivedAt 本地接收时间戳（毫秒），用于计算传输延迟
	ReceivedAt int64 `json:"received_at"`

	// Freshness 数据新鲜度判定：fresh | stale | unknown
	Freshness string `json:"freshness"`
}

// Kafka Topic 命名约定
const (
	// TopicPublicPrefix 公共数据 Topic 前缀
	// 完整格式: public.{exchange}.{data_type}
	// 示例: public.binance.orderbook
	TopicPublicPrefix = "public"

	// TopicPrivatePrefix 私有数据 Topic 前缀
	// 完整格式: private.{exchange}.{data_type}
	// 示例: private.binance.orders
	TopicPrivatePrefix = "private"
)

// Redis Key 命名约定（数据层写入）
const (
	// RedisRawKeyPattern 原始数据 Key
	// 格式: raw:{exchange}:{data_type}:{symbol}
	// 示例: raw:binance:orderbook:BTC-USDT
	RedisRawKeyPattern = "raw:%s:%s:%s"

	// RedisFreshnessKeyPattern 新鲜度时间戳 Key
	// 格式: freshness:{exchange}:{data_type}:{symbol}
	// Value: Unix 时间戳（秒）
	RedisFreshnessKeyPattern = "freshness:%s:%s:%s"
)
