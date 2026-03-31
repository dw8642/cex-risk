package exchange

import (
	"context"

	"github.com/cex-risk/cex-risk/pkg/models"
)

// Adapter 交易所适配器接口
// 所有交易所接入必须实现此接口，保证多交易所扩展能力
type Adapter interface {
	// Connect 建立连接（WS + REST client 初始化）
	Connect(ctx context.Context) error

	// SubscribeTrades 订阅成交流
	SubscribeTrades(ctx context.Context, symbols []string) (<-chan models.TradeEvent, error)

	// SubscribePositions 订阅仓位更新
	SubscribePositions(ctx context.Context) (<-chan models.PositionSnapshot, error)

	// SubscribeBalances 订阅余额更新
	SubscribeBalances(ctx context.Context) (<-chan models.BalanceSnapshot, error)

	// SubscribeAccountUpdates 订阅账户更新事件（统一推送流）
	SubscribeAccountUpdates(ctx context.Context) (<-chan models.AccountUpdate, error)

	// GetPositions REST 查询当前仓位（对账用）
	GetPositions(ctx context.Context) ([]models.PositionSnapshot, error)

	// GetBalances REST 查询当前余额（对账用）
	GetBalances(ctx context.Context) ([]models.BalanceSnapshot, error)

	// GetAPIKeyPermissions REST 查询 API Key 权限（P-001 检测用）
	GetAPIKeyPermissions(ctx context.Context) (*models.APIKeyPermissions, error)

	// Close 关闭连接
	Close() error

	// ExchangeID 返回交易所标识
	ExchangeID() string

	// MarketType 返回市场类型 (spot/futures/margin)
	MarketType() string
}

// AdapterFactory 适配器工厂函数类型
type AdapterFactory func(apiKey, secretKey string, opts ...Option) Adapter

// Option 适配器选项
type Option func(*AdapterOptions)

// AdapterOptions 通用选项
type AdapterOptions struct {
	WSEndpoint   string
	RESTEndpoint string
	HTTPProxy    string
	AccountID    string // 风控系统内部的账户 ID
	Debug        bool
}

// WithWSEndpoint 设置 WS 地址
func WithWSEndpoint(url string) Option {
	return func(o *AdapterOptions) {
		o.WSEndpoint = url
	}
}

// WithRESTEndpoint 设置 REST 地址
func WithRESTEndpoint(url string) Option {
	return func(o *AdapterOptions) {
		o.RESTEndpoint = url
	}
}

// WithHTTPProxy 设置 HTTP 代理
func WithHTTPProxy(proxy string) Option {
	return func(o *AdapterOptions) {
		o.HTTPProxy = proxy
	}
}

// WithAccountID 设置风控系统内部账户 ID
func WithAccountID(id string) Option {
	return func(o *AdapterOptions) {
		o.AccountID = id
	}
}

// WithDebug 设置调试模式
func WithDebug(debug bool) Option {
	return func(o *AdapterOptions) {
		o.Debug = debug
	}
}

// ApplyOptions 应用选项
func ApplyOptions(opts []Option) *AdapterOptions {
	o := &AdapterOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Registry 交易所适配器注册表
var Registry = make(map[string]AdapterFactory)

// Register 注册适配器工厂
func Register(exchangeID string, factory AdapterFactory) {
	Registry[exchangeID] = factory
}

// Create 根据交易所 ID 创建适配器
func Create(exchangeID, apiKey, secretKey string, opts ...Option) (Adapter, error) {
	factory, ok := Registry[exchangeID]
	if !ok {
		return nil, &UnsupportedExchangeError{ExchangeID: exchangeID}
	}
	return factory(apiKey, secretKey, opts...), nil
}

// UnsupportedExchangeError 不支持的交易所
type UnsupportedExchangeError struct {
	ExchangeID string
}

func (e *UnsupportedExchangeError) Error() string {
	return "unsupported exchange: " + e.ExchangeID
}
