package models

import (
	"encoding/json"
	"time"
)

// TradeEvent 成交事件 — 从交易所 WS/REST 接入的原始成交数据
type TradeEvent struct {
	ExchangeID  string    `json:"exchange_id"`
	AccountID   string    `json:"account_id"`
	Symbol      string    `json:"symbol"`
	Side        string    `json:"side"` // BUY / SELL
	Price       float64   `json:"price"`
	Quantity    float64   `json:"quantity"`
	QuoteQty    float64   `json:"quote_qty"`
	RealizedPnl float64   `json:"realized_pnl"`
	Commission  float64   `json:"commission"`
	TradeID     string    `json:"trade_id"`
	OrderID     string    `json:"order_id"`
	TradeTime   time.Time `json:"trade_time"`
	IngestTime  time.Time `json:"ingest_time"`
	Source      string    `json:"source"` // ws / rest
}

// PositionSnapshot 仓位快照
type PositionSnapshot struct {
	ExchangeID    string    `json:"exchange_id"`
	AccountID     string    `json:"account_id"`
	Symbol        string    `json:"symbol"`
	PositionSide  string    `json:"position_side"` // LONG / SHORT / BOTH
	Quantity      float64   `json:"quantity"`
	EntryPrice    float64   `json:"entry_price"`
	MarkPrice     float64   `json:"mark_price"`
	UnrealizedPnl float64   `json:"unrealized_pnl"`
	Leverage      int       `json:"leverage"`
	MarginType    string    `json:"margin_type"` // cross / isolated
	SnapshotTime  time.Time `json:"snapshot_time"`
	Source        string    `json:"source"`
}

// BalanceSnapshot 余额快照
type BalanceSnapshot struct {
	ExchangeID       string    `json:"exchange_id"`
	AccountID        string    `json:"account_id"`
	Asset            string    `json:"asset"`
	WalletBalance    float64   `json:"wallet_balance"`
	AvailableBalance float64   `json:"available_balance"`
	UnrealizedPnl    float64   `json:"unrealized_pnl"`
	MarginBalance    float64   `json:"margin_balance"`
	MaintMargin      float64   `json:"maint_margin"`
	SnapshotTime     time.Time `json:"snapshot_time"`
	Source           string    `json:"source"`
}

// APIKeyPermissions 从交易所 REST API 查询到的实际权限
type APIKeyPermissions struct {
	ExchangeID       string    `json:"exchange_id"`
	AccountID        string    `json:"account_id"`
	EnableSpot       bool      `json:"enable_spot"`
	EnableFutures    bool      `json:"enable_futures"`
	EnableWithdraw   bool      `json:"enable_withdraw"`
	EnableInternalTransfer bool `json:"enable_internal_transfer"`
	EnableMargin     bool      `json:"enable_margin"`
	IPRestrict       bool      `json:"ip_restrict"`
	IPList           []string  `json:"ip_list"`
	TradingSymbols   []string  `json:"trading_symbols,omitempty"` // 实际发生交易的币对
	CheckTime        time.Time `json:"check_time"`
}

// AccountUpdate 账户更新事件（余额变动、仓位变动等推送）
type AccountUpdate struct {
	ExchangeID string          `json:"exchange_id"`
	AccountID  string          `json:"account_id"`
	EventType  string          `json:"event_type"` // ACCOUNT_UPDATE / ORDER_TRADE_UPDATE / etc
	EventTime  time.Time       `json:"event_time"`
	RawData    json.RawMessage `json:"raw_data"`
}

// Marshal 序列化为 JSON bytes，用于写入 Kafka
func (t *TradeEvent) Marshal() ([]byte, error) {
	return json.Marshal(t)
}

func (p *PositionSnapshot) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

func (b *BalanceSnapshot) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

func (a *APIKeyPermissions) Marshal() ([]byte, error) {
	return json.Marshal(a)
}

func (a *AccountUpdate) Marshal() ([]byte, error) {
	return json.Marshal(a)
}
