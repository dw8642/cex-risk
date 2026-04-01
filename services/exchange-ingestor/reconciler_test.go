// reconciler_test.go — Reconciler 单元测试
//
// 使用 mock 适配器、mock Kafka producer、mock Redis 测试：
//   - REST 仓位对账 → Kafka + Redis
//   - REST 余额对账 → Kafka + Redis
//   - 权限检查 → Kafka + Redis
//   - REST 查询失败时的错误处理
//   - 定时循环启停
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cex-risk/cex-risk/pkg/models"
)

// mockRedis 模拟 Redis 存储，记录所有写入操作
type mockRedis struct {
	mu            sync.Mutex
	positions     map[string]map[string]interface{} // key=accountID:symbol
	balances      map[string]map[string]interface{} // key=accountID:asset
	permissions   map[string]string                 // key=accountID:field
	ipLists       map[string][]string               // key=accountID
	lastActivity  map[string]time.Time              // key=accountID
}

func newMockRedis() *mockRedis {
	return &mockRedis{
		positions:    make(map[string]map[string]interface{}),
		balances:     make(map[string]map[string]interface{}),
		permissions:  make(map[string]string),
		ipLists:      make(map[string][]string),
		lastActivity: make(map[string]time.Time),
	}
}

func (r *mockRedis) SetPosition(_ context.Context, accountID, symbol string, fields map[string]interface{}, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.positions[accountID+":"+symbol] = fields
	return nil
}

func (r *mockRedis) SetBalance(_ context.Context, accountID, asset string, fields map[string]interface{}, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.balances[accountID+":"+asset] = fields
	return nil
}

func (r *mockRedis) SetLastActivity(_ context.Context, accountID string, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastActivity[accountID] = t
	return nil
}

func (r *mockRedis) SetPermission(_ context.Context, accountID string, field string, value string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissions[accountID+":"+field] = value
	return nil
}

func (r *mockRedis) SetPermissionIPList(_ context.Context, accountID string, ips []string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ipLists[accountID] = ips
	return nil
}

// ==================== Reconciler 测试 ====================

// TestReconcilerDoReconcile 测试一次仓位/余额对账
func TestReconcilerDoReconcile(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()

	// 模拟 REST 返回数据
	adapter.positions = []models.PositionSnapshot{
		{
			ExchangeID: "binance", AccountID: "acc-010", Symbol: "BTCUSDT",
			PositionSide: "BOTH", Quantity: 0.5, EntryPrice: 42000.0,
			MarkPrice: 43000.0, UnrealizedPnl: 500.0, Leverage: 10,
			MarginType: "cross", SnapshotTime: time.Now(), Source: "rest",
		},
	}
	adapter.balances = []models.BalanceSnapshot{
		{
			ExchangeID: "binance", AccountID: "acc-010", Asset: "USDT",
			WalletBalance: 50000.0, AvailableBalance: 45000.0,
			UnrealizedPnl: 500.0, MarginBalance: 50500.0,
			MaintMargin: 2000.0, SnapshotTime: time.Now(), Source: "rest",
		},
	}

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-010")
	rec.doReconcile(context.Background())

	// 验证 Kafka 发布
	msgs := pub.getMessages()
	posCount, balCount := 0, 0
	for _, m := range msgs {
		switch m.Topic {
		case "test_positions":
			posCount++
			var decoded models.PositionSnapshot
			if err := json.Unmarshal(m.Value, &decoded); err != nil {
				t.Fatalf("unmarshal position: %v", err)
			}
			if decoded.Symbol != "BTCUSDT" {
				t.Errorf("Symbol = %q, want BTCUSDT", decoded.Symbol)
			}
		case "test_balances":
			balCount++
			var decoded models.BalanceSnapshot
			if err := json.Unmarshal(m.Value, &decoded); err != nil {
				t.Fatalf("unmarshal balance: %v", err)
			}
			if decoded.Asset != "USDT" {
				t.Errorf("Asset = %q, want USDT", decoded.Asset)
			}
		}
	}
	if posCount != 1 {
		t.Errorf("position messages = %d, want 1", posCount)
	}
	if balCount != 1 {
		t.Errorf("balance messages = %d, want 1", balCount)
	}

	// 验证 Redis 写入
	redis.mu.Lock()
	defer redis.mu.Unlock()

	posFields, ok := redis.positions["acc-010:BTCUSDT"]
	if !ok {
		t.Error("position not written to Redis")
	} else {
		if posFields["leverage"] != 10 {
			t.Errorf("leverage = %v, want 10", posFields["leverage"])
		}
	}

	balFields, ok := redis.balances["acc-010:USDT"]
	if !ok {
		t.Error("balance not written to Redis")
	} else {
		if balFields["wallet_balance"] != "50000.000000" {
			t.Errorf("wallet_balance = %v", balFields["wallet_balance"])
		}
	}

	if _, ok := redis.lastActivity["acc-010"]; !ok {
		t.Error("last activity not updated in Redis")
	}
}

// TestReconcilerDoPermissionCheck 测试一次权限检查
func TestReconcilerDoPermissionCheck(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()

	adapter.permissions = &models.APIKeyPermissions{
		ExchangeID:             "binance",
		AccountID:              "acc-011",
		EnableSpot:             true,
		EnableFutures:          true,
		EnableWithdraw:         false,
		EnableInternalTransfer: false,
		EnableMargin:           false,
		IPRestrict:             true,
		IPList:                 []string{"1.2.3.4", "5.6.7.8"},
		CheckTime:              time.Now(),
	}

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-011")
	rec.doPermissionCheck(context.Background())

	// 验证 Kafka 发布权限数据
	msgs := pub.getMessages()
	permCount := 0
	for _, m := range msgs {
		if m.Topic == "test_permissions" {
			permCount++
			if string(m.Key) != "acc-011" {
				t.Errorf("key = %q, want acc-011", string(m.Key))
			}
		}
	}
	if permCount != 1 {
		t.Errorf("permission messages = %d, want 1", permCount)
	}

	// 验证 Redis 写入
	redis.mu.Lock()
	defer redis.mu.Unlock()

	if redis.permissions["acc-011:withdraw_enabled"] != "false" {
		t.Errorf("withdraw_enabled = %q, want false", redis.permissions["acc-011:withdraw_enabled"])
	}
	if redis.permissions["acc-011:futures_enabled"] != "true" {
		t.Errorf("futures_enabled = %q, want true", redis.permissions["acc-011:futures_enabled"])
	}
	if redis.permissions["acc-011:spot_enabled"] != "true" {
		t.Errorf("spot_enabled = %q, want true", redis.permissions["acc-011:spot_enabled"])
	}
	if redis.permissions["acc-011:ip_restrict"] != "true" {
		t.Errorf("ip_restrict = %q, want true", redis.permissions["acc-011:ip_restrict"])
	}
	if redis.permissions["acc-011:internal_transfer_enabled"] != "false" {
		t.Errorf("internal_transfer_enabled = %q, want false", redis.permissions["acc-011:internal_transfer_enabled"])
	}

	// 验证 IP 白名单
	ips, ok := redis.ipLists["acc-011"]
	if !ok {
		t.Error("IP list not written to Redis")
	} else if len(ips) != 2 {
		t.Errorf("IP list length = %d, want 2", len(ips))
	}
}

// TestReconcilerRESTError 测试 REST 查询失败的错误处理
func TestReconcilerRESTError(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()

	// 模拟 REST 错误
	adapter.posErr = fmt.Errorf("connection refused")
	adapter.balErr = fmt.Errorf("timeout")
	adapter.permErr = fmt.Errorf("unauthorized")

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-012")

	// 不应 panic
	rec.doReconcile(context.Background())
	rec.doPermissionCheck(context.Background())

	// Kafka 不应有消息（全部失败）
	msgs := pub.getMessages()
	if len(msgs) != 0 {
		t.Errorf("published %d messages on error, want 0", len(msgs))
	}
}

// TestReconcilerEmptyPositions 测试空仓位/余额的处理
func TestReconcilerEmptyPositions(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()

	// 空仓位和余额
	adapter.positions = []models.PositionSnapshot{}
	adapter.balances = []models.BalanceSnapshot{}

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-013")
	rec.doReconcile(context.Background())

	// 只有 lastActivity 应该更新
	redis.mu.Lock()
	defer redis.mu.Unlock()
	if _, ok := redis.lastActivity["acc-013"]; !ok {
		t.Error("last activity should be updated even with empty data")
	}

	msgs := pub.getMessages()
	if len(msgs) != 0 {
		t.Errorf("published %d messages for empty data, want 0", len(msgs))
	}
}

// TestReconcilerStartStop 测试定时循环启停
func TestReconcilerStartStop(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()
	cfg.RiskEngine.RESTReconcileInterval = "100ms"
	cfg.RiskEngine.PermissionCheckInterval = "100ms"

	adapter.positions = []models.PositionSnapshot{
		{ExchangeID: "binance", AccountID: "acc-014", Symbol: "BTCUSDT",
			Quantity: 1.0, SnapshotTime: time.Now(), Source: "rest"},
	}
	adapter.balances = []models.BalanceSnapshot{}
	adapter.permissions = &models.APIKeyPermissions{
		ExchangeID: "binance", AccountID: "acc-014",
		EnableFutures: true, CheckTime: time.Now(),
	}

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-014")
	ctx, cancel := context.WithCancel(context.Background())

	rec.Start(ctx)

	// 等待几个周期
	time.Sleep(350 * time.Millisecond)

	cancel()

	// Stop 不应阻塞
	done := make(chan struct{})
	go func() {
		rec.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() blocked after context cancel")
	}

	// 应该有多次对账
	msgs := pub.getMessages()
	if len(msgs) < 2 {
		t.Errorf("published %d messages, want >= 2 (multiple reconcile cycles)", len(msgs))
	}
}

// TestReconcilerPermissionNoIPList 测试无 IP 白名单时不写入
func TestReconcilerPermissionNoIPList(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()

	adapter.permissions = &models.APIKeyPermissions{
		ExchangeID:    "binance",
		AccountID:     "acc-015",
		EnableFutures: true,
		IPRestrict:    false,
		IPList:        nil, // 无 IP 白名单
		CheckTime:     time.Now(),
	}

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-015")
	rec.doPermissionCheck(context.Background())

	redis.mu.Lock()
	defer redis.mu.Unlock()

	// 不应写入 IP 白名单
	if _, ok := redis.ipLists["acc-015"]; ok {
		t.Error("should not write empty IP list to Redis")
	}

	// 但权限字段应写入
	if redis.permissions["acc-015:ip_restrict"] != "false" {
		t.Errorf("ip_restrict = %q, want false", redis.permissions["acc-015:ip_restrict"])
	}
}

// TestReconcilerMultiplePositions 测试多仓位对账
func TestReconcilerMultiplePositions(t *testing.T) {
	adapter := newMockAdapter()
	pub := &mockPublisher{}
	redis := newMockRedis()
	cfg := testConfig()

	adapter.positions = []models.PositionSnapshot{
		{ExchangeID: "binance", AccountID: "acc-016", Symbol: "BTCUSDT",
			Quantity: 0.5, EntryPrice: 42000.0, SnapshotTime: time.Now(), Source: "rest"},
		{ExchangeID: "binance", AccountID: "acc-016", Symbol: "ETHUSDT",
			Quantity: 10.0, EntryPrice: 3000.0, SnapshotTime: time.Now(), Source: "rest"},
		{ExchangeID: "binance", AccountID: "acc-016", Symbol: "SOLUSDT",
			Quantity: 100.0, EntryPrice: 150.0, SnapshotTime: time.Now(), Source: "rest"},
	}
	adapter.balances = []models.BalanceSnapshot{
		{ExchangeID: "binance", AccountID: "acc-016", Asset: "USDT",
			WalletBalance: 50000.0, SnapshotTime: time.Now(), Source: "rest"},
		{ExchangeID: "binance", AccountID: "acc-016", Asset: "BNB",
			WalletBalance: 10.0, SnapshotTime: time.Now(), Source: "rest"},
	}

	rec := newReconciler(adapter, pub, redis, cfg, testLogger(), "acc-016")
	rec.doReconcile(context.Background())

	// 验证 Kafka: 3 仓位 + 2 余额
	msgs := pub.getMessages()
	posCount, balCount := 0, 0
	for _, m := range msgs {
		switch m.Topic {
		case "test_positions":
			posCount++
		case "test_balances":
			balCount++
		}
	}
	if posCount != 3 {
		t.Errorf("position messages = %d, want 3", posCount)
	}
	if balCount != 2 {
		t.Errorf("balance messages = %d, want 2", balCount)
	}

	// 验证 Redis: 所有仓位和余额都写入
	redis.mu.Lock()
	defer redis.mu.Unlock()
	if len(redis.positions) != 3 {
		t.Errorf("redis positions = %d, want 3", len(redis.positions))
	}
	if len(redis.balances) != 2 {
		t.Errorf("redis balances = %d, want 2", len(redis.balances))
	}
}
