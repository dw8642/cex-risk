package metrics

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

// newTestRedis 创建基于 miniredis 的测试 Redis 实例
func newTestRedis(t *testing.T) (*store.Redis, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	rds := store.NewRedisFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	return rds, mr
}

// newTestConfig 创建测试用配置
func newTestConfig() *config.Config {
	return &config.Config{
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Topics: config.KafkaTopicConfig{
				TradeEvents:       "risk_trade_events",
				PositionSnapshots: "risk_position_snapshots",
				BalanceSnapshots:  "risk_balance_snapshots",
			},
		},
		RiskEngine: config.RiskEngineConfig{
			MetricsWindow: "5m",
		},
	}
}

// mustMarshal JSON 序列化，测试中失败则 fatal
func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return data
}

// --- NewEngine 测试 ---

func TestNewEngine_参数校验(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()
	cfg := newTestConfig()
	log := zap.NewNop()

	// redis 为 nil
	_, err := NewEngine(nil, cfg, log)
	if err == nil {
		t.Error("期望 redis=nil 时返回错误")
	}

	// config 为 nil
	_, err = NewEngine(rds, nil, log)
	if err == nil {
		t.Error("期望 config=nil 时返回错误")
	}

	// logger 为 nil
	_, err = NewEngine(rds, cfg, nil)
	if err == nil {
		t.Error("期望 logger=nil 时返回错误")
	}

	// 正常创建
	e, err := NewEngine(rds, cfg, log)
	if err != nil {
		t.Fatalf("正常创建失败: %v", err)
	}
	if e.metricsWindow != 5*time.Minute {
		t.Errorf("metricsWindow = %v, want 5m", e.metricsWindow)
	}
}

// --- handleTradeEvent 测试 ---

func TestHandleTradeEvent_正常处理(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()
	cfg := newTestConfig()

	e := &Engine{
		redis:         rds,
		cfg:           cfg,
		logger:        zap.NewNop(),
		metricsWindow: 5 * time.Minute,
	}

	now := time.Now().Truncate(time.Second)
	trade := models.TradeEvent{
		ExchangeID: "binance",
		AccountID:  "acc001",
		Symbol:     "BTCUSDT",
		Side:       "BUY",
		Price:      50000.0,
		Quantity:   0.1,
		QuoteQty:   5000.0,
		TradeID:    "t-123",
		OrderID:    "o-456",
		TradeTime:  now,
		IngestTime: now,
		Source:     "ws",
	}

	msg := kafka.Message{Value: mustMarshal(t, trade)}
	ctx := context.Background()

	if err := e.handleTradeEvent(ctx, msg); err != nil {
		t.Fatalf("handleTradeEvent 失败: %v", err)
	}

	// 验证滑动窗口成交计数
	count, err := rds.GetTradeWindowCount(ctx, "acc001", "5m0s")
	if err != nil {
		t.Fatalf("GetTradeWindowCount 失败: %v", err)
	}
	if count != 1 {
		t.Errorf("成交窗口计数 = %d, want 1", count)
	}

	// 验证交易币对
	symbols, err := rds.GetTradedSymbols(ctx, "acc001", "5m0s")
	if err != nil {
		t.Fatalf("GetTradedSymbols 失败: %v", err)
	}
	if len(symbols) != 1 || symbols[0] != "BTCUSDT" {
		t.Errorf("交易币对 = %v, want [BTCUSDT]", symbols)
	}

	// 验证最后活跃时间
	activity, err := rds.GetLastActivity(ctx, "acc001")
	if err != nil {
		t.Fatalf("GetLastActivity 失败: %v", err)
	}
	if activity.Unix() != now.Unix() {
		t.Errorf("活跃时间 = %v, want %v", activity, now)
	}

	// 验证 freshness
	fresh, err := GetFreshness(ctx, rds, "acc001", "trade")
	if err != nil {
		t.Fatalf("GetFreshness 失败: %v", err)
	}
	if fresh.Unix() != now.Unix() {
		t.Errorf("freshness = %v, want %v", fresh, now)
	}
}

func TestHandleTradeEvent_多笔成交累计(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()
	cfg := newTestConfig()

	e := &Engine{
		redis:         rds,
		cfg:           cfg,
		logger:        zap.NewNop(),
		metricsWindow: 5 * time.Minute,
	}
	ctx := context.Background()
	now := time.Now()

	// 发送 3 笔不同币对的成交
	trades := []models.TradeEvent{
		{AccountID: "acc001", Symbol: "BTCUSDT", TradeID: "t-1", TradeTime: now, IngestTime: now, Source: "ws"},
		{AccountID: "acc001", Symbol: "ETHUSDT", TradeID: "t-2", TradeTime: now.Add(time.Second), IngestTime: now, Source: "ws"},
		{AccountID: "acc001", Symbol: "BTCUSDT", TradeID: "t-3", TradeTime: now.Add(2 * time.Second), IngestTime: now, Source: "ws"},
	}

	for _, trade := range trades {
		msg := kafka.Message{Value: mustMarshal(t, trade)}
		if err := e.handleTradeEvent(ctx, msg); err != nil {
			t.Fatalf("handleTradeEvent 失败: %v", err)
		}
	}

	// 验证 3 笔成交
	count, err := rds.GetTradeWindowCount(ctx, "acc001", "5m0s")
	if err != nil {
		t.Fatalf("GetTradeWindowCount 失败: %v", err)
	}
	if count != 3 {
		t.Errorf("成交窗口计数 = %d, want 3", count)
	}

	// 验证 2 个不同币对
	symbols, err := rds.GetTradedSymbols(ctx, "acc001", "5m0s")
	if err != nil {
		t.Fatalf("GetTradedSymbols 失败: %v", err)
	}
	if len(symbols) != 2 {
		t.Errorf("交易币对数 = %d, want 2", len(symbols))
	}
}

func TestHandleTradeEvent_反序列化失败(t *testing.T) {
	e := &Engine{logger: zap.NewNop(), metricsWindow: 5 * time.Minute}
	msg := kafka.Message{Value: []byte("invalid json")}
	err := e.handleTradeEvent(context.Background(), msg)
	if err == nil {
		t.Error("期望 JSON 非法时返回错误")
	}
}

// --- handlePositionSnapshot 测试 ---

func TestHandlePositionSnapshot_正常处理(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()
	cfg := newTestConfig()

	e := &Engine{
		redis:         rds,
		cfg:           cfg,
		logger:        zap.NewNop(),
		metricsWindow: 5 * time.Minute,
	}

	now := time.Now().Truncate(time.Second)
	pos := models.PositionSnapshot{
		ExchangeID:    "binance",
		AccountID:     "acc001",
		Symbol:        "BTCUSDT",
		PositionSide:  "LONG",
		Quantity:      1.5,
		EntryPrice:    48000.0,
		MarkPrice:     50000.0,
		UnrealizedPnl: 3000.0,
		Leverage:      10,
		MarginType:    "cross",
		SnapshotTime:  now,
		Source:        "ws",
	}

	msg := kafka.Message{Value: mustMarshal(t, pos)}
	ctx := context.Background()

	if err := e.handlePositionSnapshot(ctx, msg); err != nil {
		t.Fatalf("handlePositionSnapshot 失败: %v", err)
	}

	// 验证 Redis Hash — HGETALL position:acc001:BTCUSDT
	fields, err := rds.GetPosition(ctx, "acc001", "BTCUSDT")
	if err != nil {
		t.Fatalf("GetPosition 失败: %v", err)
	}

	// 检查关键字段
	checks := map[string]string{
		"exchange_id":    "binance",
		"symbol":         "BTCUSDT",
		"position_side":  "LONG",
		"quantity":       "1.5",
		"entry_price":    "48000",
		"mark_price":     "50000",
		"unrealized_pnl": "3000",
		"leverage":       "10",
		"margin_type":    "cross",
		"source":         "ws",
	}
	for k, want := range checks {
		if got := fields[k]; got != want {
			t.Errorf("position[%s] = %q, want %q", k, got, want)
		}
	}

	// 验证 freshness
	fresh, err := GetFreshness(ctx, rds, "acc001", "position")
	if err != nil {
		t.Fatalf("GetFreshness 失败: %v", err)
	}
	if fresh.Unix() != now.Unix() {
		t.Errorf("freshness = %v, want %v", fresh, now)
	}
}

func TestHandlePositionSnapshot_反序列化失败(t *testing.T) {
	e := &Engine{logger: zap.NewNop(), metricsWindow: 5 * time.Minute}
	msg := kafka.Message{Value: []byte("{bad")}
	err := e.handlePositionSnapshot(context.Background(), msg)
	if err == nil {
		t.Error("期望 JSON 非法时返回错误")
	}
}

// --- handleBalanceSnapshot 测试 ---

func TestHandleBalanceSnapshot_正常处理(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()
	cfg := newTestConfig()

	e := &Engine{
		redis:         rds,
		cfg:           cfg,
		logger:        zap.NewNop(),
		metricsWindow: 5 * time.Minute,
	}

	now := time.Now().Truncate(time.Second)
	bal := models.BalanceSnapshot{
		ExchangeID:       "binance",
		AccountID:        "acc001",
		Asset:            "USDT",
		WalletBalance:    100000.0,
		AvailableBalance: 80000.0,
		UnrealizedPnl:    3000.0,
		MarginBalance:    95000.0,
		MaintMargin:      5000.0,
		SnapshotTime:     now,
		Source:           "rest",
	}

	msg := kafka.Message{Value: mustMarshal(t, bal)}
	ctx := context.Background()

	if err := e.handleBalanceSnapshot(ctx, msg); err != nil {
		t.Fatalf("handleBalanceSnapshot 失败: %v", err)
	}

	// 验证 Redis Hash — HGETALL balance:acc001:USDT
	fields, err := rds.GetBalance(ctx, "acc001", "USDT")
	if err != nil {
		t.Fatalf("GetBalance 失败: %v", err)
	}

	checks := map[string]string{
		"exchange_id":       "binance",
		"asset":             "USDT",
		"wallet_balance":    "100000",
		"available_balance": "80000",
		"unrealized_pnl":    "3000",
		"margin_balance":    "95000",
		"maint_margin":      "5000",
		"source":            "rest",
	}
	for k, want := range checks {
		if got := fields[k]; got != want {
			t.Errorf("balance[%s] = %q, want %q", k, got, want)
		}
	}

	// 验证 freshness
	fresh, err := GetFreshness(ctx, rds, "acc001", "balance")
	if err != nil {
		t.Fatalf("GetFreshness 失败: %v", err)
	}
	if fresh.Unix() != now.Unix() {
		t.Errorf("freshness = %v, want %v", fresh, now)
	}
}

func TestHandleBalanceSnapshot_多资产(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()
	cfg := newTestConfig()

	e := &Engine{
		redis:         rds,
		cfg:           cfg,
		logger:        zap.NewNop(),
		metricsWindow: 5 * time.Minute,
	}
	ctx := context.Background()
	now := time.Now()

	// 写入两个不同资产的余额
	assets := []models.BalanceSnapshot{
		{AccountID: "acc001", Asset: "USDT", WalletBalance: 100000, SnapshotTime: now, Source: "rest"},
		{AccountID: "acc001", Asset: "BTC", WalletBalance: 2.5, SnapshotTime: now, Source: "rest"},
	}

	for _, bal := range assets {
		msg := kafka.Message{Value: mustMarshal(t, bal)}
		if err := e.handleBalanceSnapshot(ctx, msg); err != nil {
			t.Fatalf("handleBalanceSnapshot 失败: %v", err)
		}
	}

	// 验证两个资产独立存储
	usdt, err := rds.GetBalance(ctx, "acc001", "USDT")
	if err != nil {
		t.Fatalf("GetBalance USDT 失败: %v", err)
	}
	if usdt["wallet_balance"] != "100000" {
		t.Errorf("USDT wallet = %q, want 100000", usdt["wallet_balance"])
	}

	btc, err := rds.GetBalance(ctx, "acc001", "BTC")
	if err != nil {
		t.Fatalf("GetBalance BTC 失败: %v", err)
	}
	if btc["wallet_balance"] != "2.5" {
		t.Errorf("BTC wallet = %q, want 2.5", btc["wallet_balance"])
	}
}

func TestHandleBalanceSnapshot_反序列化失败(t *testing.T) {
	e := &Engine{logger: zap.NewNop(), metricsWindow: 5 * time.Minute}
	msg := kafka.Message{Value: []byte("not-json")}
	err := e.handleBalanceSnapshot(context.Background(), msg)
	if err == nil {
		t.Error("期望 JSON 非法时返回错误")
	}
}

// --- GetFreshness 测试 ---

func TestGetFreshness_不存在的键(t *testing.T) {
	rds, mr := newTestRedis(t)
	defer mr.Close()

	_, err := GetFreshness(context.Background(), rds, "noexist", "trade")
	if err == nil {
		t.Error("期望键不存在时返回错误")
	}
}
