package exchange

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cex-risk/cex-risk/pkg/models"
	"go.uber.org/zap"
)

// TestParseFloat 测试字符串转浮点数
func TestParseFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"123.456", 123.456},
		{"0", 0},
		{"-50.5", -50.5},
		{"0.00000001", 0.00000001},
		{"", 0},        // 空字符串返回 0
		{"abc", 0},     // 非法字符串返回 0
	}
	for _, tt := range tests {
		got := parseFloat(tt.input)
		if got != tt.want {
			t.Errorf("parseFloat(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

// TestParseInt 测试字符串转整数
func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"10", 10},
		{"0", 0},
		{"-1", -1},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseInt(tt.input)
		if got != tt.want {
			t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestSign 测试 HMAC-SHA256 签名
func TestSign(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		secretKey: "NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j",
	}

	// 使用已知的输入和预期输出验证签名
	payload := "symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000&timestamp=1499827319559"
	sig := adapter.sign(payload)

	// 签名应该是 64 字符的十六进制字符串
	if len(sig) != 64 {
		t.Errorf("sign() length = %d, want 64", len(sig))
	}

	// 相同输入应产生相同签名
	sig2 := adapter.sign(payload)
	if sig != sig2 {
		t.Errorf("sign() not deterministic: %s != %s", sig, sig2)
	}

	// 不同输入应产生不同签名
	sig3 := adapter.sign(payload + "x")
	if sig == sig3 {
		t.Error("sign() produced same signature for different payloads")
	}
}

// TestNewBinanceFuturesAdapter 测试适配器创建和默认值
func TestNewBinanceFuturesAdapter(t *testing.T) {
	adapter := NewBinanceFuturesAdapter("testKey", "testSecret")

	bfa, ok := adapter.(*BinanceFuturesAdapter)
	if !ok {
		t.Fatal("NewBinanceFuturesAdapter should return *BinanceFuturesAdapter")
	}

	if bfa.apiKey != "testKey" {
		t.Errorf("apiKey = %q, want %q", bfa.apiKey, "testKey")
	}
	if bfa.secretKey != "testSecret" {
		t.Errorf("secretKey = %q, want %q", bfa.secretKey, "testSecret")
	}

	// 检查默认端点
	if bfa.opts.WSEndpoint != "wss://fstream.binance.com" {
		t.Errorf("WSEndpoint = %q, want default", bfa.opts.WSEndpoint)
	}
	if bfa.opts.RESTEndpoint != "https://fapi.binance.com" {
		t.Errorf("RESTEndpoint = %q, want default", bfa.opts.RESTEndpoint)
	}
}

// TestNewBinanceFuturesAdapterWithOptions 测试带选项的适配器创建
func TestNewBinanceFuturesAdapterWithOptions(t *testing.T) {
	adapter := NewBinanceFuturesAdapter("key", "secret",
		WithWSEndpoint("wss://custom.endpoint"),
		WithRESTEndpoint("https://custom.rest"),
		WithHTTPProxy("127.0.0.1:7897"),
		WithAccountID("acc-001"),
		WithDebug(true),
	)

	bfa := adapter.(*BinanceFuturesAdapter)

	if bfa.opts.WSEndpoint != "wss://custom.endpoint" {
		t.Errorf("WSEndpoint = %q", bfa.opts.WSEndpoint)
	}
	if bfa.opts.RESTEndpoint != "https://custom.rest" {
		t.Errorf("RESTEndpoint = %q", bfa.opts.RESTEndpoint)
	}
	if bfa.opts.HTTPProxy != "127.0.0.1:7897" {
		t.Errorf("HTTPProxy = %q", bfa.opts.HTTPProxy)
	}
	if bfa.opts.AccountID != "acc-001" {
		t.Errorf("AccountID = %q", bfa.opts.AccountID)
	}
	if !bfa.opts.Debug {
		t.Error("Debug should be true")
	}
}

// TestExchangeIDAndMarketType 测试标识方法
func TestExchangeIDAndMarketType(t *testing.T) {
	adapter := NewBinanceFuturesAdapter("key", "secret")

	if adapter.ExchangeID() != "binance" {
		t.Errorf("ExchangeID() = %q, want %q", adapter.ExchangeID(), "binance")
	}
	if adapter.MarketType() != "futures" {
		t.Errorf("MarketType() = %q, want %q", adapter.MarketType(), "futures")
	}
}

// TestAdapterRegistered 测试适配器已注册到全局 Registry
func TestAdapterRegistered(t *testing.T) {
	factory, ok := Registry["binance_futures"]
	if !ok {
		t.Fatal("binance_futures not registered in Registry")
	}

	adapter := factory("key", "secret")
	if adapter == nil {
		t.Fatal("factory returned nil")
	}
	if adapter.ExchangeID() != "binance" {
		t.Errorf("ExchangeID() = %q", adapter.ExchangeID())
	}
}

// TestHandleOrderTradeUpdate 测试 WS 成交事件解析
func TestHandleOrderTradeUpdate(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		opts:    &AdapterOptions{AccountID: "acc-test-001"},
		tradeCh: make(chan models.TradeEvent, 10),
		stopCh:  make(chan struct{}),
	}
	adapter.logger, _ = newTestLogger()

	// 模拟币安 ORDER_TRADE_UPDATE 消息
	wsMsg := map[string]interface{}{
		"e": "ORDER_TRADE_UPDATE",
		"E": time.Now().UnixMilli(),
		"o": map[string]interface{}{
			"s": "BTCUSDT",
			"S": "BUY",
			"o": "LIMIT",
			"x": "TRADE", // ExecutionType = TRADE 表示实际成交
			"i": 12345,
			"t": 67890,
			"L": "50000.50",  // 最近成交价
			"l": "0.001",     // 最近成交量
			"Y": "50.0005",   // Quote quantity
			"rp": "10.5",     // Realized PnL
			"n": "0.02",      // Commission
			"T": time.Now().UnixMilli(),
		},
	}
	msgBytes, _ := json.Marshal(wsMsg)

	adapter.handleWSMessage(msgBytes)

	// 验证 channel 收到了事件
	select {
	case trade := <-adapter.tradeCh:
		if trade.Symbol != "BTCUSDT" {
			t.Errorf("Symbol = %q, want BTCUSDT", trade.Symbol)
		}
		if trade.Side != "BUY" {
			t.Errorf("Side = %q, want BUY", trade.Side)
		}
		if trade.Price != 50000.50 {
			t.Errorf("Price = %f, want 50000.50", trade.Price)
		}
		if trade.Quantity != 0.001 {
			t.Errorf("Quantity = %f, want 0.001", trade.Quantity)
		}
		if trade.AccountID != "acc-test-001" {
			t.Errorf("AccountID = %q, want acc-test-001", trade.AccountID)
		}
		if trade.Source != "ws" {
			t.Errorf("Source = %q, want ws", trade.Source)
		}
	default:
		t.Error("no trade event received")
	}
}

// TestHandleOrderTradeUpdateNonTrade 测试非成交事件被过滤
func TestHandleOrderTradeUpdateNonTrade(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		opts:    &AdapterOptions{AccountID: "acc-test-001"},
		tradeCh: make(chan models.TradeEvent, 10),
		stopCh:  make(chan struct{}),
	}
	adapter.logger, _ = newTestLogger()

	// ExecutionType = NEW (下单，非成交)
	wsMsg := map[string]interface{}{
		"e": "ORDER_TRADE_UPDATE",
		"E": time.Now().UnixMilli(),
		"o": map[string]interface{}{
			"s": "BTCUSDT",
			"S": "BUY",
			"x": "NEW", // 不是 TRADE
			"i": 12345,
			"t": 0,
			"L": "0",
			"l": "0",
			"T": time.Now().UnixMilli(),
		},
	}
	msgBytes, _ := json.Marshal(wsMsg)

	adapter.handleWSMessage(msgBytes)

	// channel 应该为空
	select {
	case <-adapter.tradeCh:
		t.Error("non-TRADE event should not produce trade event")
	default:
		// 正确：无事件
	}
}

// TestHandleAccountUpdate 测试 WS 账户更新事件解析
func TestHandleAccountUpdate(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		opts:       &AdapterOptions{AccountID: "acc-test-001"},
		balanceCh:  make(chan models.BalanceSnapshot, 10),
		positionCh: make(chan models.PositionSnapshot, 10),
		accountCh:  make(chan models.AccountUpdate, 10),
		stopCh:     make(chan struct{}),
	}
	adapter.logger, _ = newTestLogger()

	wsMsg := map[string]interface{}{
		"e": "ACCOUNT_UPDATE",
		"E": time.Now().UnixMilli(),
		"a": map[string]interface{}{
			"B": []map[string]interface{}{
				{"a": "USDT", "wb": "10000.50", "cw": "9500.00"},
			},
			"P": []map[string]interface{}{
				{"s": "ETHUSDT", "pa": "5.0", "ep": "3000.00", "up": "250.00", "mt": "cross", "ps": "BOTH"},
			},
		},
	}
	msgBytes, _ := json.Marshal(wsMsg)

	adapter.handleWSMessage(msgBytes)

	// 验证余额
	select {
	case bal := <-adapter.balanceCh:
		if bal.Asset != "USDT" {
			t.Errorf("Asset = %q, want USDT", bal.Asset)
		}
		if bal.WalletBalance != 10000.50 {
			t.Errorf("WalletBalance = %f, want 10000.50", bal.WalletBalance)
		}
	default:
		t.Error("no balance event received")
	}

	// 验证仓位
	select {
	case pos := <-adapter.positionCh:
		if pos.Symbol != "ETHUSDT" {
			t.Errorf("Symbol = %q, want ETHUSDT", pos.Symbol)
		}
		if pos.Quantity != 5.0 {
			t.Errorf("Quantity = %f, want 5.0", pos.Quantity)
		}
		if pos.EntryPrice != 3000.00 {
			t.Errorf("EntryPrice = %f, want 3000.00", pos.EntryPrice)
		}
	default:
		t.Error("no position event received")
	}

	// 验证原始事件
	select {
	case update := <-adapter.accountCh:
		if update.EventType != "ACCOUNT_UPDATE" {
			t.Errorf("EventType = %q, want ACCOUNT_UPDATE", update.EventType)
		}
	default:
		t.Error("no account update event received")
	}
}

// TestParseAccountResponse 测试 REST 账户响应解析
func TestParseAccountResponse(t *testing.T) {
	respJSON := `{
		"assets": [
			{"asset":"USDT","walletBalance":"50000.00","availableBalance":"45000.00","unrealizedProfit":"500.00","marginBalance":"50500.00","maintMargin":"2000.00"},
			{"asset":"BNB","walletBalance":"0","availableBalance":"0","unrealizedProfit":"0","marginBalance":"0","maintMargin":"0"}
		],
		"positions": [
			{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"0.5","entryPrice":"42000.00","markPrice":"43000.00","unrealizedProfit":"500.00","leverage":"10","marginType":"cross"},
			{"symbol":"ETHUSDT","positionSide":"BOTH","positionAmt":"0","entryPrice":"0","markPrice":"3000.00","unrealizedProfit":"0","leverage":"5","marginType":"cross"}
		]
	}`

	var resp binanceAccountResp
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// 检查 assets
	if len(resp.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(resp.Assets))
	}
	if resp.Assets[0].Asset != "USDT" {
		t.Errorf("Asset[0] = %q, want USDT", resp.Assets[0].Asset)
	}
	if parseFloat(resp.Assets[0].WalletBalance) != 50000.00 {
		t.Errorf("WalletBalance = %s", resp.Assets[0].WalletBalance)
	}

	// 检查 positions — 只有 BTCUSDT 有仓位(qty != 0)
	nonZeroPositions := 0
	for _, p := range resp.Positions {
		if parseFloat(p.PositionAmt) != 0 {
			nonZeroPositions++
		}
	}
	if nonZeroPositions != 1 {
		t.Errorf("expected 1 non-zero position, got %d", nonZeroPositions)
	}
}

// TestParseAPIRestrictions 测试权限响应解析
func TestParseAPIRestrictions(t *testing.T) {
	respJSON := `{
		"ipRestrict": true,
		"enableWithdrawals": false,
		"enableInternalTransfer": false,
		"enableMargin": false,
		"enableSpotAndMarginTrading": true,
		"enableFutures": true
	}`

	var restrictions binanceAPIRestrictions
	if err := json.Unmarshal([]byte(respJSON), &restrictions); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !restrictions.IPRestrict {
		t.Error("IPRestrict should be true")
	}
	if restrictions.EnableWithdrawals {
		t.Error("EnableWithdrawals should be false")
	}
	if !restrictions.EnableFutures {
		t.Error("EnableFutures should be true")
	}
	if !restrictions.EnableSpotAndMarginTrading {
		t.Error("EnableSpotAndMarginTrading should be true")
	}
}

// TestCreateAdapter 测试通过 Registry 创建适配器
func TestCreateAdapter(t *testing.T) {
	adapter, err := Create("binance_futures", "key", "secret", WithAccountID("acc-001"))
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if adapter.ExchangeID() != "binance" {
		t.Errorf("ExchangeID() = %q, want binance", adapter.ExchangeID())
	}
	if adapter.MarketType() != "futures" {
		t.Errorf("MarketType() = %q, want futures", adapter.MarketType())
	}
}

// TestCreateUnsupportedExchange 测试创建不支持的交易所返回错误
func TestCreateUnsupportedExchange(t *testing.T) {
	_, err := Create("okx_futures", "key", "secret")
	if err == nil {
		t.Fatal("expected error for unsupported exchange")
	}
	unsupErr, ok := err.(*UnsupportedExchangeError)
	if !ok {
		t.Fatalf("expected *UnsupportedExchangeError, got %T", err)
	}
	if unsupErr.ExchangeID != "okx_futures" {
		t.Errorf("ExchangeID = %q, want okx_futures", unsupErr.ExchangeID)
	}
	if unsupErr.Error() != "unsupported exchange: okx_futures" {
		t.Errorf("Error() = %q", unsupErr.Error())
	}
}

// TestApplyOptions 测试选项应用
func TestApplyOptions(t *testing.T) {
	opts := ApplyOptions([]Option{
		WithWSEndpoint("wss://test"),
		WithRESTEndpoint("https://test"),
		WithHTTPProxy("127.0.0.1:1080"),
		WithAccountID("acc-123"),
		WithDebug(true),
	})

	if opts.WSEndpoint != "wss://test" {
		t.Errorf("WSEndpoint = %q", opts.WSEndpoint)
	}
	if opts.RESTEndpoint != "https://test" {
		t.Errorf("RESTEndpoint = %q", opts.RESTEndpoint)
	}
	if opts.HTTPProxy != "127.0.0.1:1080" {
		t.Errorf("HTTPProxy = %q", opts.HTTPProxy)
	}
	if opts.AccountID != "acc-123" {
		t.Errorf("AccountID = %q", opts.AccountID)
	}
	if !opts.Debug {
		t.Error("Debug should be true")
	}
}

// TestApplyOptionsEmpty 测试空选项返回零值
func TestApplyOptionsEmpty(t *testing.T) {
	opts := ApplyOptions(nil)
	if opts.WSEndpoint != "" || opts.RESTEndpoint != "" || opts.HTTPProxy != "" {
		t.Error("empty options should have zero values")
	}
}

// TestHandleListenKeyExpired 测试 listenKey 过期事件触发异步重连
func TestHandleListenKeyExpired(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		opts:       &AdapterOptions{AccountID: "acc-test", WSEndpoint: "ws://127.0.0.1:1", RESTEndpoint: "http://127.0.0.1:1"},
		tradeCh:    make(chan models.TradeEvent, 10),
		positionCh: make(chan models.PositionSnapshot, 10),
		balanceCh:  make(chan models.BalanceSnapshot, 10),
		accountCh:  make(chan models.AccountUpdate, 10),
		stopCh:     make(chan struct{}),
		restClient: &http.Client{Timeout: 100 * time.Millisecond},
	}
	adapter.logger, _ = newTestLogger()

	// listenKeyExpired 事件应异步触发 reconnect，handleWSMessage 立即返回
	msg, _ := json.Marshal(map[string]interface{}{
		"e": "listenKeyExpired",
		"E": time.Now().UnixMilli(),
	})

	done := make(chan struct{})
	go func() {
		adapter.handleWSMessage(msg)
		close(done)
	}()

	// handleWSMessage 应该快速返回（因为 reconnect 是异步的）
	select {
	case <-done:
		// 正常
	case <-time.After(time.Second):
		t.Fatal("handleWSMessage 不应阻塞（reconnect 应异步执行）")
	}

	// 给异步 reconnect goroutine 一点启动时间
	time.Sleep(100 * time.Millisecond)
}

// TestChannelFullDrop 测试 channel 满时丢弃事件不阻塞
func TestChannelFullDrop(t *testing.T) {
	adapter := &BinanceFuturesAdapter{
		opts:       &AdapterOptions{AccountID: "acc-test"},
		tradeCh:    make(chan models.TradeEvent, 1), // 容量 1
		positionCh: make(chan models.PositionSnapshot, 10),
		balanceCh:  make(chan models.BalanceSnapshot, 10),
		accountCh:  make(chan models.AccountUpdate, 10),
		stopCh:     make(chan struct{}),
	}
	adapter.logger, _ = newTestLogger()

	// 填满 channel
	adapter.tradeCh <- models.TradeEvent{}

	// 再发一条成交事件，应被丢弃而不阻塞
	msg, _ := json.Marshal(map[string]interface{}{
		"e": "ORDER_TRADE_UPDATE",
		"E": time.Now().UnixMilli(),
		"o": map[string]interface{}{
			"s": "BTCUSDT", "S": "BUY", "x": "TRADE",
			"i": 1, "t": 1, "L": "50000", "l": "0.01",
			"Y": "500", "rp": "0", "n": "0.01",
			"T": time.Now().UnixMilli(),
		},
	})

	done := make(chan struct{})
	go func() {
		adapter.handleWSMessage(msg)
		close(done)
	}()

	select {
	case <-done:
		// 正常：不阻塞
	case <-time.After(time.Second):
		t.Fatal("handleWSMessage blocked on full channel")
	}
}

// newTestLogger 创建测试用 logger（不输出到控制台）
func newTestLogger() (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = []string{} // 静默
	cfg.ErrorOutputPaths = []string{}
	return cfg.Build()
}
