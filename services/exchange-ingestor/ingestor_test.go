// ingestor_test.go — Ingestor 单元测试
//
// 使用 mock 适配器和 mock Kafka producer 测试：
//   - 成交事件消费 → Kafka 发布
//   - 仓位快照消费 → Kafka 发布
//   - 余额快照消费 → Kafka 发布
//   - 账户更新消费 → Kafka 发布
//   - channel 关闭时 goroutine 正常退出
//   - 多事件批量消费
//   - 发布失败不中断消费循环
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/models"
	"go.uber.org/zap"
)

// ==================== Mock 实现 ====================

// mockPublisher 模拟 Kafka Producer，记录所有发布的消息
type mockPublisher struct {
	mu       sync.Mutex
	messages []publishedMsg
	failNext bool // 设为 true 时下次 Publish 返回错误
}

type publishedMsg struct {
	Topic string
	Key   []byte
	Value []byte
}

func (m *mockPublisher) Publish(_ context.Context, topic string, key []byte, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return fmt.Errorf("mock publish error")
	}
	m.messages = append(m.messages, publishedMsg{Topic: topic, Key: key, Value: value})
	return nil
}

func (m *mockPublisher) getMessages() []publishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]publishedMsg, len(m.messages))
	copy(cp, m.messages)
	return cp
}

// mockAdapter 模拟交易所适配器，提供可控的 channel
type mockAdapter struct {
	tradeCh    chan models.TradeEvent
	positionCh chan models.PositionSnapshot
	balanceCh  chan models.BalanceSnapshot
	accountCh  chan models.AccountUpdate

	positions   []models.PositionSnapshot
	balances    []models.BalanceSnapshot
	permissions *models.APIKeyPermissions
	posErr      error
	balErr      error
	permErr     error
}

func newMockAdapter() *mockAdapter {
	return &mockAdapter{
		tradeCh:    make(chan models.TradeEvent, 100),
		positionCh: make(chan models.PositionSnapshot, 100),
		balanceCh:  make(chan models.BalanceSnapshot, 100),
		accountCh:  make(chan models.AccountUpdate, 100),
	}
}

func (m *mockAdapter) Connect(_ context.Context) error                   { return nil }
func (m *mockAdapter) Close() error                                       { return nil }
func (m *mockAdapter) ExchangeID() string                                { return "mock" }
func (m *mockAdapter) MarketType() string                                { return "futures" }

func (m *mockAdapter) SubscribeTrades(_ context.Context, _ []string) (<-chan models.TradeEvent, error) {
	return m.tradeCh, nil
}
func (m *mockAdapter) SubscribePositions(_ context.Context) (<-chan models.PositionSnapshot, error) {
	return m.positionCh, nil
}
func (m *mockAdapter) SubscribeBalances(_ context.Context) (<-chan models.BalanceSnapshot, error) {
	return m.balanceCh, nil
}
func (m *mockAdapter) SubscribeAccountUpdates(_ context.Context) (<-chan models.AccountUpdate, error) {
	return m.accountCh, nil
}
func (m *mockAdapter) GetPositions(_ context.Context) ([]models.PositionSnapshot, error) {
	return m.positions, m.posErr
}
func (m *mockAdapter) GetBalances(_ context.Context) ([]models.BalanceSnapshot, error) {
	return m.balances, m.balErr
}
func (m *mockAdapter) GetAPIKeyPermissions(_ context.Context) (*models.APIKeyPermissions, error) {
	return m.permissions, m.permErr
}

// testConfig 返回测试用配置
func testConfig() *config.Config {
	cfg := &config.Config{}
	cfg.System.Debug = true
	cfg.Kafka.Topics.TradeEvents = "test_trades"
	cfg.Kafka.Topics.PositionSnapshots = "test_positions"
	cfg.Kafka.Topics.BalanceSnapshots = "test_balances"
	cfg.Kafka.Topics.AccountUpdates = "test_accounts"
	cfg.Kafka.Topics.PermissionChecks = "test_permissions"
	cfg.RiskEngine.RESTReconcileInterval = "1s"
	cfg.RiskEngine.PermissionCheckInterval = "1s"
	return cfg
}

// testLogger 返回静默 logger
func testLogger() *zap.Logger {
	return zap.NewNop()
}

// ==================== Ingestor 测试 ====================

// TestIngestorConsumeTrades 测试成交事件从 channel 消费并发布到 Kafka
func TestIngestorConsumeTrades(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-001")

	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	// 发送一条成交事件
	trade := models.TradeEvent{
		ExchangeID: "binance",
		AccountID:  "acc-001",
		Symbol:     "BTCUSDT",
		Side:       "BUY",
		Price:      50000.0,
		Quantity:   0.01,
		TradeID:    "12345",
		TradeTime:  time.Now(),
		IngestTime: time.Now(),
		Source:     "ws",
	}
	adapter.tradeCh <- trade

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	msgs := pub.getMessages()
	found := false
	for _, m := range msgs {
		if m.Topic == "test_trades" {
			found = true
			if string(m.Key) != "acc-001" {
				t.Errorf("key = %q, want acc-001", string(m.Key))
			}
			var decoded models.TradeEvent
			if err := json.Unmarshal(m.Value, &decoded); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Symbol != "BTCUSDT" {
				t.Errorf("Symbol = %q, want BTCUSDT", decoded.Symbol)
			}
			if decoded.Price != 50000.0 {
				t.Errorf("Price = %f, want 50000.0", decoded.Price)
			}
		}
	}
	if !found {
		t.Error("no trade message published to Kafka")
	}

	cancel()
	ing.Stop()
}

// TestIngestorConsumePositions 测试仓位快照消费
func TestIngestorConsumePositions(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-002")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	pos := models.PositionSnapshot{
		ExchangeID:    "binance",
		AccountID:     "acc-002",
		Symbol:        "ETHUSDT",
		PositionSide:  "LONG",
		Quantity:      10.0,
		EntryPrice:    3000.0,
		UnrealizedPnl: 500.0,
		SnapshotTime:  time.Now(),
		Source:        "ws",
	}
	adapter.positionCh <- pos

	time.Sleep(100 * time.Millisecond)

	msgs := pub.getMessages()
	found := false
	for _, m := range msgs {
		if m.Topic == "test_positions" {
			found = true
			var decoded models.PositionSnapshot
			if err := json.Unmarshal(m.Value, &decoded); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Symbol != "ETHUSDT" {
				t.Errorf("Symbol = %q, want ETHUSDT", decoded.Symbol)
			}
			if decoded.Quantity != 10.0 {
				t.Errorf("Quantity = %f, want 10.0", decoded.Quantity)
			}
		}
	}
	if !found {
		t.Error("no position message published")
	}

	cancel()
	ing.Stop()
}

// TestIngestorConsumeBalances 测试余额快照消费
func TestIngestorConsumeBalances(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-003")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	bal := models.BalanceSnapshot{
		ExchangeID:       "binance",
		AccountID:        "acc-003",
		Asset:            "USDT",
		WalletBalance:    50000.0,
		AvailableBalance: 40000.0,
		SnapshotTime:     time.Now(),
		Source:           "ws",
	}
	adapter.balanceCh <- bal

	time.Sleep(100 * time.Millisecond)

	msgs := pub.getMessages()
	found := false
	for _, m := range msgs {
		if m.Topic == "test_balances" {
			found = true
			var decoded models.BalanceSnapshot
			if err := json.Unmarshal(m.Value, &decoded); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Asset != "USDT" {
				t.Errorf("Asset = %q, want USDT", decoded.Asset)
			}
			if decoded.WalletBalance != 50000.0 {
				t.Errorf("WalletBalance = %f, want 50000.0", decoded.WalletBalance)
			}
		}
	}
	if !found {
		t.Error("no balance message published")
	}

	cancel()
	ing.Stop()
}

// TestIngestorConsumeAccountUpdates 测试账户更新事件消费
func TestIngestorConsumeAccountUpdates(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-004")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	update := models.AccountUpdate{
		ExchangeID: "binance",
		AccountID:  "acc-004",
		EventType:  "ACCOUNT_UPDATE",
		EventTime:  time.Now(),
		RawData:    json.RawMessage(`{"test": true}`),
	}
	adapter.accountCh <- update

	time.Sleep(100 * time.Millisecond)

	msgs := pub.getMessages()
	found := false
	for _, m := range msgs {
		if m.Topic == "test_accounts" {
			found = true
			if string(m.Key) != "acc-004" {
				t.Errorf("key = %q, want acc-004", string(m.Key))
			}
		}
	}
	if !found {
		t.Error("no account update message published")
	}

	cancel()
	ing.Stop()
}

// TestIngestorMultipleEvents 测试批量消费多条事件
func TestIngestorMultipleEvents(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-005")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	// 发送 10 条成交事件
	for i := 0; i < 10; i++ {
		adapter.tradeCh <- models.TradeEvent{
			ExchangeID: "binance",
			AccountID:  "acc-005",
			Symbol:     "BTCUSDT",
			Side:       "BUY",
			Price:      50000.0 + float64(i),
			Quantity:   0.01,
			TradeTime:  time.Now(),
			IngestTime: time.Now(),
			Source:     "ws",
		}
	}

	time.Sleep(200 * time.Millisecond)

	msgs := pub.getMessages()
	tradeCount := 0
	for _, m := range msgs {
		if m.Topic == "test_trades" {
			tradeCount++
		}
	}
	if tradeCount != 10 {
		t.Errorf("published %d trades, want 10", tradeCount)
	}

	cancel()
	ing.Stop()
}

// TestIngestorPublishFailure 测试 Kafka 发布失败不中断消费循环
func TestIngestorPublishFailure(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-006")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	// 第一条事件设置发布失败
	pub.mu.Lock()
	pub.failNext = true
	pub.mu.Unlock()

	adapter.tradeCh <- models.TradeEvent{
		ExchangeID: "binance", AccountID: "acc-006", Symbol: "BTCUSDT",
		Side: "BUY", Price: 50000.0, Source: "ws",
	}

	// 第二条事件应正常发布
	time.Sleep(50 * time.Millisecond)
	adapter.tradeCh <- models.TradeEvent{
		ExchangeID: "binance", AccountID: "acc-006", Symbol: "ETHUSDT",
		Side: "SELL", Price: 3000.0, Source: "ws",
	}

	time.Sleep(100 * time.Millisecond)

	msgs := pub.getMessages()
	if len(msgs) != 1 {
		t.Errorf("published %d messages, want 1 (first failed)", len(msgs))
	}
	if len(msgs) > 0 {
		var decoded models.TradeEvent
		json.Unmarshal(msgs[0].Value, &decoded)
		if decoded.Symbol != "ETHUSDT" {
			t.Errorf("Symbol = %q, want ETHUSDT (the successful one)", decoded.Symbol)
		}
	}

	cancel()
	ing.Stop()
}

// TestIngestorChannelClose 测试 channel 关闭后 goroutine 正常退出
func TestIngestorChannelClose(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-007")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	// 关闭所有 channel — 模拟适配器关闭
	close(adapter.tradeCh)
	close(adapter.positionCh)
	close(adapter.balanceCh)
	close(adapter.accountCh)

	// 等待 goroutine 退出
	done := make(chan struct{})
	go func() {
		ing.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("goroutines did not exit after channel close")
	}

	cancel()
}

// TestIngestorGracefulShutdown 测试 context cancel 触发优雅退出
func TestIngestorGracefulShutdown(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-008")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	// 先发一条事件确认正常运行
	adapter.tradeCh <- models.TradeEvent{
		ExchangeID: "binance", AccountID: "acc-008", Symbol: "BTCUSDT",
		Side: "BUY", Price: 50000.0, Source: "ws",
	}
	time.Sleep(50 * time.Millisecond)

	// cancel context 触发退出
	cancel()

	done := make(chan struct{})
	go func() {
		ing.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 正常
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return after context cancel")
	}
}

// TestIngestorMixedEvents 测试同时消费多种事件类型
func TestIngestorMixedEvents(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	cfg := testConfig()

	ing := newIngestor(adapter, pub, cfg, testLogger(), "acc-009")
	ctx, cancel := context.WithCancel(context.Background())
	ing.Start(ctx)

	// 同时发送不同类型的事件
	adapter.tradeCh <- models.TradeEvent{
		ExchangeID: "binance", AccountID: "acc-009", Symbol: "BTCUSDT",
		Side: "BUY", Price: 50000.0, Source: "ws",
	}
	adapter.positionCh <- models.PositionSnapshot{
		ExchangeID: "binance", AccountID: "acc-009", Symbol: "BTCUSDT",
		Quantity: 0.5, Source: "ws",
	}
	adapter.balanceCh <- models.BalanceSnapshot{
		ExchangeID: "binance", AccountID: "acc-009", Asset: "USDT",
		WalletBalance: 10000.0, Source: "ws",
	}
	adapter.accountCh <- models.AccountUpdate{
		ExchangeID: "binance", AccountID: "acc-009", EventType: "ACCOUNT_UPDATE",
	}

	time.Sleep(200 * time.Millisecond)

	msgs := pub.getMessages()
	topics := map[string]int{}
	for _, m := range msgs {
		topics[m.Topic]++
	}

	if topics["test_trades"] != 1 {
		t.Errorf("trades = %d, want 1", topics["test_trades"])
	}
	if topics["test_positions"] != 1 {
		t.Errorf("positions = %d, want 1", topics["test_positions"])
	}
	if topics["test_balances"] != 1 {
		t.Errorf("balances = %d, want 1", topics["test_balances"])
	}
	if topics["test_accounts"] != 1 {
		t.Errorf("accounts = %d, want 1", topics["test_accounts"])
	}

	cancel()
	ing.Stop()
}
