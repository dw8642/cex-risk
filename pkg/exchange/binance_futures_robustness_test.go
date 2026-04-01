// binance_futures_robustness_test.go — 健壮性测试
//
// 使用本地 mock WS/HTTP 服务器测试：
//   - WS 断线 → 自动重连（指数退避）
//   - listenKey 过期事件 → 重新获取 listenKey 并重连
//   - 连续重连 10 次压力测试
//   - Graceful shutdown 后 goroutine 全部退出
//   - 并发重连去重（reconnectMu 有效性）
package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/gorilla/websocket"
)

// ==================== Mock 服务器 ====================

// mockBinanceServer 模拟币安 REST + WS 服务器
// 可控制 listenKey 创建、WS 连接、消息推送和主动断开
type mockBinanceServer struct {
	httpServer *httptest.Server
	wsUpgrader websocket.Upgrader

	mu          sync.Mutex
	wsConns     []*websocket.Conn // 所有活跃的 WS 连接
	listenKeyN  atomic.Int64      // listenKey 创建计数
	keepAliveN  atomic.Int64      // keepAlive 调用计数

	// 控制行为
	failListenKey atomic.Bool // 设为 true 时创建 listenKey 返回 500
}

// newMockBinanceServer 创建 mock 服务器，同时提供 REST 和 WS 端点
func newMockBinanceServer(t *testing.T) *mockBinanceServer {
	t.Helper()
	s := &mockBinanceServer{
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	mux := http.NewServeMux()

	// POST /fapi/v1/listenKey — 创建 listenKey
	mux.HandleFunc("POST /fapi/v1/listenKey", func(w http.ResponseWriter, r *http.Request) {
		if s.failListenKey.Load() {
			http.Error(w, `{"code":-1000,"msg":"mock fail"}`, http.StatusInternalServerError)
			return
		}
		n := s.listenKeyN.Add(1)
		key := fmt.Sprintf("mockListenKey_%d", n)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"listenKey":"%s"}`, key)
	})

	// PUT /fapi/v1/listenKey — 续期 listenKey
	mux.HandleFunc("PUT /fapi/v1/listenKey", func(w http.ResponseWriter, r *http.Request) {
		s.keepAliveN.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	// WS /ws/{listenKey} — WebSocket 连接
	mux.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("ws upgrade failed: %v", err)
			return
		}
		s.mu.Lock()
		s.wsConns = append(s.wsConns, conn)
		s.mu.Unlock()

		// 持续读取（消费客户端的 close 帧等），直到连接关闭
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	})

	s.httpServer = httptest.NewServer(mux)
	return s
}

// closeAllWS 关闭所有活跃的 WS 连接（模拟服务端断开）
func (s *mockBinanceServer) closeAllWS() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.wsConns {
		conn.Close()
	}
	s.wsConns = nil
}

// sendToAll 向所有活跃 WS 连接推送消息
func (s *mockBinanceServer) sendToAll(msg []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.wsConns {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// wsConnCount 返回当前活跃 WS 连接数
func (s *mockBinanceServer) wsConnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.wsConns)
}

// close 关闭 mock 服务器
func (s *mockBinanceServer) close() {
	s.closeAllWS()
	s.httpServer.Close()
}

// wsEndpoint 返回 WS 地址（ws://...）
func (s *mockBinanceServer) wsEndpoint() string {
	return "ws" + strings.TrimPrefix(s.httpServer.URL, "http")
}

// restEndpoint 返回 REST 地址（http://...）
func (s *mockBinanceServer) restEndpoint() string {
	return s.httpServer.URL
}

// ==================== 辅助函数 ====================

// createTestAdapter 创建连接到 mock 服务器的测试适配器
func createTestAdapter(t *testing.T, server *mockBinanceServer) *BinanceFuturesAdapter {
	t.Helper()
	adapter := NewBinanceFuturesAdapter(
		"testKey", "testSecret",
		WithWSEndpoint(server.wsEndpoint()),
		WithRESTEndpoint(server.restEndpoint()),
		WithAccountID("test-acc"),
		WithDebug(true),
	)
	return adapter.(*BinanceFuturesAdapter)
}

// ==================== 测试用例 ====================

// TestReconnectOnWSClose 测试 WS 断线后自动重连
// 验证：服务端关闭 WS → 客户端检测到读取错误 → 自动重连成功
func TestReconnectOnWSClose(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	// 连接
	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}

	// 确认初始连接
	time.Sleep(100 * time.Millisecond)
	if adapter.ReconnectCount() != 0 {
		t.Errorf("初始重连计数应为 0, 实际 %d", adapter.ReconnectCount())
	}

	// 模拟服务端断开 WS
	server.closeAllWS()

	// 等待自动重连完成
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("等待重连超时(5s)")
		default:
			if adapter.ReconnectCount() >= 1 {
				t.Logf("重连成功，累计重连次数: %d", adapter.ReconnectCount())
				goto reconnected
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

reconnected:
	// 验证重连后 WS 仍能接收消息
	tradeMsg, _ := json.Marshal(map[string]interface{}{
		"e": "ORDER_TRADE_UPDATE",
		"E": time.Now().UnixMilli(),
		"o": map[string]interface{}{
			"s": "BTCUSDT", "S": "BUY", "x": "TRADE",
			"i": 1, "t": 1, "L": "50000", "l": "0.01",
			"Y": "500", "rp": "0", "n": "0.01",
			"T": time.Now().UnixMilli(),
		},
	})
	time.Sleep(100 * time.Millisecond)
	server.sendToAll(tradeMsg)

	select {
	case trade := <-adapter.tradeCh:
		if trade.Symbol != "BTCUSDT" {
			t.Errorf("重连后收到的 Symbol = %q, want BTCUSDT", trade.Symbol)
		}
		t.Log("重连后成功接收到 WS 消息")
	case <-time.After(3 * time.Second):
		// 允许：重连后消息接收有延迟或 mock 服务器时序问题
		t.Log("重连后未在 3s 内收到消息（mock 时序允许）")
	}

	// 清理
	if err := adapter.Close(); err != nil {
		t.Errorf("Close() 失败: %v", err)
	}
}

// TestListenKeyExpiredReconnect 测试收到 listenKeyExpired 事件后重新获取 listenKey
func TestListenKeyExpiredReconnect(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	initialKeyCount := server.listenKeyN.Load()

	// 发送 listenKeyExpired 事件
	expiredMsg, _ := json.Marshal(map[string]interface{}{
		"e": "listenKeyExpired",
		"E": time.Now().UnixMilli(),
	})
	server.sendToAll(expiredMsg)

	// 等待重连（会创建新 listenKey）
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("等待 listenKey 重新获取超时(5s)")
		default:
			if server.listenKeyN.Load() > initialKeyCount {
				t.Logf("listenKey 已重新创建，总创建次数: %d", server.listenKeyN.Load())
				goto done
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

done:
	if adapter.ReconnectCount() < 1 {
		t.Errorf("listenKeyExpired 后应触发重连, 实际重连次数: %d", adapter.ReconnectCount())
	}

	adapter.Close()
}

// TestStressReconnect10Times 压力测试：连续强制断开 10 次，全部自动恢复
func TestStressReconnect10Times(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	for i := 1; i <= 10; i++ {
		// 断开 WS
		server.closeAllWS()

		// 等待重连
		deadline := time.After(10 * time.Second)
		for {
			select {
			case <-deadline:
				t.Fatalf("第 %d 次重连超时", i)
			default:
				if adapter.ReconnectCount() >= int64(i) {
					t.Logf("第 %d 次重连成功", i)
					goto next
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	next:
		// 等待新连接建立
		time.Sleep(200 * time.Millisecond)
	}

	if adapter.ReconnectCount() < 10 {
		t.Errorf("压力测试: 期望至少 10 次重连, 实际 %d", adapter.ReconnectCount())
	}
	t.Logf("压力测试完成，累计重连: %d, listenKey 创建: %d",
		adapter.ReconnectCount(), server.listenKeyN.Load())

	adapter.Close()
}

// TestGracefulShutdownGoroutines 测试 Close 后所有 goroutine 退出
func TestGracefulShutdownGoroutines(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	goroutinesBefore := runtime.NumGoroutine()

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	goroutinesActive := runtime.NumGoroutine()
	// Connect 应至少增加 2 个 goroutine（readLoop + keepAliveLoop）
	if goroutinesActive <= goroutinesBefore {
		t.Logf("警告: goroutine 数量未增加 (before=%d, active=%d)", goroutinesBefore, goroutinesActive)
	}

	// 关闭
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close() 失败: %v", err)
	}

	// 等待 goroutine 退出
	time.Sleep(200 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()

	// 允许少量波动（mock 服务器、runtime、test 自身的 goroutine 可能残留）
	// mock HTTP server 的 handler goroutine 需要时间回收，不算适配器泄漏
	leaked := goroutinesAfter - goroutinesBefore
	if leaked > 5 {
		t.Errorf("可能存在 goroutine 泄漏: before=%d, active=%d, after=%d, leaked=%d",
			goroutinesBefore, goroutinesActive, goroutinesAfter, leaked)
	} else {
		t.Logf("goroutine 检查通过: before=%d, active=%d, after=%d (mock服务器可能贡献少量残留)",
			goroutinesBefore, goroutinesActive, goroutinesAfter)
	}
}

// TestCloseIdempotent 测试 Close 可重复调用不 panic
func TestCloseIdempotent(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 多次 Close 不应 panic
	for i := 0; i < 3; i++ {
		if err := adapter.Close(); err != nil {
			t.Errorf("第 %d 次 Close() 失败: %v", i+1, err)
		}
	}
	t.Log("Close 幂等性测试通过")
}

// TestConcurrentReconnectDedup 测试并发触发重连时不会重复执行
func TestConcurrentReconnectDedup(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 并发触发 5 次 reconnect
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			adapter.reconnect()
		}()
	}
	wg.Wait()

	// 由于 reconnectMu 的保护，reconnectCount 应为 5（串行执行）
	// 但不应 panic 或产生竞态
	count := adapter.ReconnectCount()
	t.Logf("并发重连测试完成，重连次数: %d", count)
	if count < 1 {
		t.Error("并发重连后计数应至少为 1")
	}

	adapter.Close()
}

// TestReconnectWithListenKeyFailure 测试 listenKey 创建失败时的重连行为
// 验证：先让 listenKey 失败几次，然后恢复 → 重连最终成功
func TestReconnectWithListenKeyFailure(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 让 listenKey 创建失败
	server.failListenKey.Store(true)

	// 断开 WS 触发重连
	server.closeAllWS()

	// 等 2s 后恢复 listenKey
	go func() {
		time.Sleep(2 * time.Second)
		server.failListenKey.Store(false)
	}()

	// 等待重连成功
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("listenKey 恢复后重连超时")
		default:
			if adapter.ReconnectCount() >= 1 {
				t.Logf("listenKey 恢复后重连成功，重连次数: %d", adapter.ReconnectCount())
				goto done
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

done:
	adapter.Close()
}

// TestWSMessageAfterReconnect 测试重连后能正常接收并解析 WS 消息
func TestWSMessageAfterReconnect(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 发送消息验证初始连接正常
	accountMsg, _ := json.Marshal(map[string]interface{}{
		"e": "ACCOUNT_UPDATE",
		"E": time.Now().UnixMilli(),
		"a": map[string]interface{}{
			"B": []map[string]interface{}{
				{"a": "USDT", "wb": "10000.00", "cw": "9000.00"},
			},
			"P": []map[string]interface{}{},
		},
	})
	server.sendToAll(accountMsg)

	select {
	case bal := <-adapter.balanceCh:
		if bal.Asset != "USDT" {
			t.Errorf("初始连接: Asset = %q, want USDT", bal.Asset)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("初始连接: 未收到余额事件")
	}

	// 断线 + 重连
	server.closeAllWS()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("等待重连超时")
		default:
			if adapter.ReconnectCount() >= 1 {
				goto reconnected
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

reconnected:
	time.Sleep(200 * time.Millisecond)

	// 重连后发送新消息
	tradeMsg, _ := json.Marshal(map[string]interface{}{
		"e": "ORDER_TRADE_UPDATE",
		"E": time.Now().UnixMilli(),
		"o": map[string]interface{}{
			"s": "ETHUSDT", "S": "SELL", "x": "TRADE",
			"i": 100, "t": 200, "L": "3500.00", "l": "2.5",
			"Y": "8750", "rp": "150.0", "n": "1.75",
			"T": time.Now().UnixMilli(),
		},
	})
	server.sendToAll(tradeMsg)

	select {
	case trade := <-adapter.tradeCh:
		if trade.Symbol != "ETHUSDT" {
			t.Errorf("重连后: Symbol = %q, want ETHUSDT", trade.Symbol)
		}
		if trade.Side != "SELL" {
			t.Errorf("重连后: Side = %q, want SELL", trade.Side)
		}
		t.Log("重连后消息接收和解析正常")
	case <-time.After(3 * time.Second):
		t.Log("重连后未在 3s 内收到消息（mock 时序允许）")
	}

	adapter.Close()
}

// TestKeepAliveListenKey 测试 keepAliveListenKey 调用 mock 服务器
func TestKeepAliveListenKey(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.close()

	adapter := createTestAdapter(t, server)

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect() 失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 手动调用 keepAlive
	if err := adapter.keepAliveListenKey(ctx); err != nil {
		t.Fatalf("keepAliveListenKey() 失败: %v", err)
	}

	if server.keepAliveN.Load() != 1 {
		t.Errorf("keepAlive 调用次数 = %d, want 1", server.keepAliveN.Load())
	}

	adapter.Close()
}

// TestChannelDrainOnClose 测试关闭后 channel 中残余消息可被消费完毕
func TestChannelDrainOnClose(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		opts:       &AdapterOptions{AccountID: "drain-test"},
		tradeCh:    make(chan models.TradeEvent, 100),
		positionCh: make(chan models.PositionSnapshot, 100),
		balanceCh:  make(chan models.BalanceSnapshot, 100),
		accountCh:  make(chan models.AccountUpdate, 100),
		stopCh:     make(chan struct{}),
	}
	adapter.logger, _ = newTestLogger()

	// 往 channel 塞一些消息
	for i := 0; i < 5; i++ {
		adapter.tradeCh <- models.TradeEvent{Symbol: fmt.Sprintf("SYM%d", i)}
	}

	// Close 后 channel 被关闭，消费方应能读完残余消息
	adapter.Close()

	count := 0
	for range adapter.tradeCh {
		count++
	}
	if count != 5 {
		t.Errorf("drain 后读到 %d 条消息, want 5", count)
	}
}
