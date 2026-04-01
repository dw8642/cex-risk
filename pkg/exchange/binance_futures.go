// binance_futures.go — 币安 USDT-M 合约交易所适配器
//
// 实现 exchange.Adapter 接口，提供以下能力：
//   - WebSocket: 通过 User Data Stream 实时接收成交(ORDER_TRADE_UPDATE)和
//     仓位/余额变动(ACCOUNT_UPDATE)，自动维护 listenKey 续期和断线重连
//   - REST: 查询账户仓位/余额(/fapi/v2/account)、API Key 权限(/sapi/v1/account/apiRestrictions)
//   - 签名: HMAC-SHA256，支持 HTTP 代理（用于国内网络环境）
//
// 通过 init() 自动注册到 exchange.Registry["binance_futures"]，
// 调用方通过 exchange.Create("binance_futures", ...) 即可创建实例。
package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func init() {
	Register("binance_futures", NewBinanceFuturesAdapter)
}

// BinanceFuturesAdapter 币安 USDT-M 合约适配器
//
// 核心数据流：
//   WS User Data Stream → readLoop → handleWSMessage → tradeCh/positionCh/balanceCh/accountCh
//   REST /fapi/v2/account → GetPositions/GetBalances (由 Reconciler 定时调用)
//   REST /sapi/v1/account/apiRestrictions → GetAPIKeyPermissions (由 Reconciler 定时调用)
//
// Channel 容量设计：
//   tradeCh(1000)   — 成交事件量最大，高频交易时可能每秒数十条
//   positionCh(500) — 仓位变动较频繁
//   balanceCh(200)  — 余额变动相对低频
//   accountCh(500)  — 原始账户更新事件
type BinanceFuturesAdapter struct {
	apiKey    string
	secretKey string
	opts      *AdapterOptions
	logger    *zap.Logger

	restClient *http.Client    // 带代理和超时的 HTTP 客户端
	wsConn     *websocket.Conn // 当前活跃的 WS 连接
	wsMu       sync.Mutex      // 保护 wsConn 的并发读写

	listenKey string // 币安 User Data Stream 的 listenKey，60 分钟过期

	tradeCh    chan models.TradeEvent       // 实时成交事件输出
	positionCh chan models.PositionSnapshot // 仓位变动输出
	balanceCh  chan models.BalanceSnapshot  // 余额变动输出
	accountCh  chan models.AccountUpdate    // 原始账户更新输出

	stopCh chan struct{}    // 关闭信号
	wg     sync.WaitGroup  // 等待所有 goroutine 退出

	// 健壮性相关字段
	reconnectCount  atomic.Int64  // 累计重连次数
	reconnectMu     sync.Mutex    // 防止并发重连
	closed          atomic.Bool   // 标记是否已关闭，防止重复 Close
}

// NewBinanceFuturesAdapter 创建币安合约适配器
func NewBinanceFuturesAdapter(apiKey, secretKey string, optFns ...Option) Adapter {
	opts := ApplyOptions(optFns)
	if opts.WSEndpoint == "" {
		opts.WSEndpoint = "wss://fstream.binance.com"
	}
	if opts.RESTEndpoint == "" {
		opts.RESTEndpoint = "https://fapi.binance.com"
	}

	logger, _ := zap.NewDevelopment()

	transport := &http.Transport{}
	if opts.HTTPProxy != "" {
		proxyURL, err := url.Parse("http://" + opts.HTTPProxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &BinanceFuturesAdapter{
		apiKey:    apiKey,
		secretKey: secretKey,
		opts:      opts,
		logger:    logger,
		restClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		tradeCh:    make(chan models.TradeEvent, 1000),
		positionCh: make(chan models.PositionSnapshot, 500),
		balanceCh:  make(chan models.BalanceSnapshot, 200),
		accountCh:  make(chan models.AccountUpdate, 500),
		stopCh:     make(chan struct{}),
	}
}

func (b *BinanceFuturesAdapter) ExchangeID() string { return "binance" }
func (b *BinanceFuturesAdapter) MarketType() string  { return "futures" }

// Connect 建立 WS 连接，启动 listenKey 维护
func (b *BinanceFuturesAdapter) Connect(ctx context.Context) error {
	// 1. 创建 listenKey
	key, err := b.createListenKey(ctx)
	if err != nil {
		return fmt.Errorf("create listenKey: %w", err)
	}
	b.listenKey = key
	b.logger.Info("listenKey created", zap.String("key", key[:8]+"..."))

	// 2. 连接 WebSocket
	wsURL := fmt.Sprintf("%s/ws/%s", b.opts.WSEndpoint, b.listenKey)
	dialer := websocket.DefaultDialer
	if b.opts.HTTPProxy != "" {
		proxyURL, _ := url.Parse("http://" + b.opts.HTTPProxy)
		dialer = &websocket.Dialer{
			Proxy:            http.ProxyURL(proxyURL),
			HandshakeTimeout: 15 * time.Second,
		}
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws connect: %w", err)
	}
	b.wsConn = conn
	b.logger.Info("WebSocket connected", zap.String("endpoint", b.opts.WSEndpoint))

	// 3. 启动 WS 消息读取循环
	b.wg.Add(1)
	go b.readLoop()

	// 4. 启动 listenKey 续期
	b.wg.Add(1)
	go b.keepAliveLoop()

	return nil
}

// SubscribeTrades 返回成交事件 channel
func (b *BinanceFuturesAdapter) SubscribeTrades(ctx context.Context, symbols []string) (<-chan models.TradeEvent, error) {
	return b.tradeCh, nil
}

// SubscribePositions 返回仓位快照 channel
func (b *BinanceFuturesAdapter) SubscribePositions(ctx context.Context) (<-chan models.PositionSnapshot, error) {
	return b.positionCh, nil
}

// SubscribeBalances 返回余额快照 channel
func (b *BinanceFuturesAdapter) SubscribeBalances(ctx context.Context) (<-chan models.BalanceSnapshot, error) {
	return b.balanceCh, nil
}

// SubscribeAccountUpdates 返回账户更新 channel
func (b *BinanceFuturesAdapter) SubscribeAccountUpdates(ctx context.Context) (<-chan models.AccountUpdate, error) {
	return b.accountCh, nil
}

// GetPositions REST 查询当前仓位
func (b *BinanceFuturesAdapter) GetPositions(ctx context.Context) ([]models.PositionSnapshot, error) {
	data, err := b.signedGet(ctx, "/fapi/v2/account", nil)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	var resp binanceAccountResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse account: %w", err)
	}

	now := time.Now()
	var positions []models.PositionSnapshot
	for _, p := range resp.Positions {
		qty := parseFloat(p.PositionAmt)
		if qty == 0 {
			continue // 跳过空仓
		}
		positions = append(positions, models.PositionSnapshot{
			ExchangeID:    "binance",
			AccountID:     b.opts.AccountID,
			Symbol:        p.Symbol,
			PositionSide:  p.PositionSide,
			Quantity:      qty,
			EntryPrice:    parseFloat(p.EntryPrice),
			MarkPrice:     parseFloat(p.MarkPrice),
			UnrealizedPnl: parseFloat(p.UnrealizedProfit),
			Leverage:      parseInt(p.Leverage),
			MarginType:    p.MarginType,
			SnapshotTime:  now,
			Source:        "rest",
		})
	}
	return positions, nil
}

// GetBalances REST 查询当前余额
func (b *BinanceFuturesAdapter) GetBalances(ctx context.Context) ([]models.BalanceSnapshot, error) {
	data, err := b.signedGet(ctx, "/fapi/v2/account", nil)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	var resp binanceAccountResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse account: %w", err)
	}

	now := time.Now()
	var balances []models.BalanceSnapshot
	for _, a := range resp.Assets {
		wb := parseFloat(a.WalletBalance)
		if wb == 0 {
			continue
		}
		balances = append(balances, models.BalanceSnapshot{
			ExchangeID:       "binance",
			AccountID:        b.opts.AccountID,
			Asset:            a.Asset,
			WalletBalance:    wb,
			AvailableBalance: parseFloat(a.AvailableBalance),
			UnrealizedPnl:    parseFloat(a.UnrealizedProfit),
			MarginBalance:    parseFloat(a.MarginBalance),
			MaintMargin:      parseFloat(a.MaintMargin),
			SnapshotTime:     now,
			Source:           "rest",
		})
	}
	return balances, nil
}

// GetAPIKeyPermissions REST 查询 API Key 权限
//
// 注意：币安合约 API (fapi) 没有直接查询 API Key 权限的接口，
// 需要通过现货 API (api) 的 /sapi/v1/account/apiRestrictions 来查询。
// 这里通过字符串替换将 fapi 端点转换为 api 端点。
// 返回的权限信息包括：现货、合约、提币、内部转账、保证金交易等开关状态。
func (b *BinanceFuturesAdapter) GetAPIKeyPermissions(ctx context.Context) (*models.APIKeyPermissions, error) {
	spotBase := strings.Replace(b.opts.RESTEndpoint, "fapi", "api", 1)

	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("signature", b.sign(params.Encode()))

	reqURL := fmt.Sprintf("%s/sapi/v1/account/apiRestrictions?%s", spotBase, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", b.apiKey)

	resp, err := b.restClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api restrictions request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api restrictions status %d: %s", resp.StatusCode, string(body))
	}

	var restrictions binanceAPIRestrictions
	if err := json.Unmarshal(body, &restrictions); err != nil {
		return nil, fmt.Errorf("parse restrictions: %w", err)
	}

	return &models.APIKeyPermissions{
		ExchangeID:             "binance",
		AccountID:              b.opts.AccountID,
		EnableSpot:             restrictions.EnableSpotAndMarginTrading,
		EnableFutures:          restrictions.EnableFutures,
		EnableWithdraw:         restrictions.EnableWithdrawals,
		EnableInternalTransfer: restrictions.EnableInternalTransfer,
		EnableMargin:           restrictions.EnableMargin,
		IPRestrict:             restrictions.IPRestrict,
		CheckTime:              time.Now(),
	}, nil
}

// Close 关闭连接（幂等安全，可重复调用）
// 关闭顺序：
//   1. 标记 closed，防止重复关闭和重连
//   2. 通知 stopCh，让 readLoop 和 keepAliveLoop 退出
//   3. 关闭 WS 连接
//   4. 等待所有 goroutine 退出（带超时保护，防止永久阻塞）
//   5. 关闭输出 channel
func (b *BinanceFuturesAdapter) Close() error {
	// 幂等保护：只执行一次
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	b.logger.Info("开始关闭适配器",
		zap.Int64("累计重连次数", b.reconnectCount.Load()))

	close(b.stopCh)

	b.wsMu.Lock()
	if b.wsConn != nil {
		// 发送 WS Close 帧，给对端优雅关闭的机会
		b.wsConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"))
		b.wsConn.Close()
		b.wsConn = nil
	}
	b.wsMu.Unlock()

	// 带超时等待所有 goroutine 退出，防止 goroutine 泄漏导致永久阻塞
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		b.logger.Info("所有 goroutine 已退出")
	case <-time.After(10 * time.Second):
		b.logger.Error("等待 goroutine 退出超时（10s），可能存在 goroutine 泄漏")
	}

	close(b.tradeCh)
	close(b.positionCh)
	close(b.balanceCh)
	close(b.accountCh)

	b.logger.Info("适配器已关闭")
	return nil
}

// ==================== 内部方法 ====================

// readLoop WebSocket 消息读取循环
// 在读取失败时触发重连，若重连也失败则退出循环（等待外部重启）
func (b *BinanceFuturesAdapter) readLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.stopCh:
			return
		default:
		}

		b.wsMu.Lock()
		conn := b.wsConn
		b.wsMu.Unlock()
		if conn == nil {
			b.logger.Warn("readLoop: wsConn 为 nil，退出读取循环")
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-b.stopCh:
				return
			default:
				b.logger.Error("ws 读取失败，尝试重连",
					zap.Error(err),
					zap.Int64("累计重连次数", b.reconnectCount.Load()))
				if !b.reconnect() {
					b.logger.Error("重连失败，readLoop 退出")
					return
				}
				continue
			}
		}

		b.handleWSMessage(msg)
	}
}

// handleWSMessage 处理 WS 推送消息
func (b *BinanceFuturesAdapter) handleWSMessage(msg []byte) {
	var base struct {
		EventType string `json:"e"`
		EventTime int64  `json:"E"`
	}
	if err := json.Unmarshal(msg, &base); err != nil {
		b.logger.Warn("ws unmarshal base failed", zap.Error(err))
		return
	}

	switch base.EventType {
	case "ORDER_TRADE_UPDATE":
		b.handleOrderTradeUpdate(msg, base.EventTime)
	case "ACCOUNT_UPDATE":
		b.handleAccountUpdate(msg, base.EventTime)
	case "listenKeyExpired":
		b.logger.Warn("listenKey 已过期，异步触发重连",
			zap.Int64("累计重连次数", b.reconnectCount.Load()))
		// 使用 goroutine 异步重连，避免阻塞 readLoop
		// reconnectMu 保证不会和 readLoop 的重连并发执行
		go b.reconnect()
	default:
		if b.opts.Debug {
			b.logger.Debug("ws event", zap.String("type", base.EventType))
		}
	}
}

// handleOrderTradeUpdate 处理成交推送
func (b *BinanceFuturesAdapter) handleOrderTradeUpdate(msg []byte, eventTime int64) {
	var update struct {
		Order struct {
			Symbol        string `json:"s"`
			Side          string `json:"S"`
			OrderType     string `json:"o"`
			ExecutionType string `json:"x"` // TRADE = 成交
			OrderID       int64  `json:"i"`
			TradeID       int64  `json:"t"`
			Price         string `json:"L"` // 最近成交价
			Quantity      string `json:"l"` // 最近成交量
			QuoteQty      string `json:"Y"` // Quote quantity
			RealizedPnl   string `json:"rp"`
			Commission    string `json:"n"`
			TradeTime     int64  `json:"T"`
		} `json:"o"`
	}
	if err := json.Unmarshal(msg, &update); err != nil {
		b.logger.Warn("parse ORDER_TRADE_UPDATE failed", zap.Error(err))
		return
	}

	o := update.Order
	if o.ExecutionType != "TRADE" {
		return // 只关注实际成交
	}

	trade := models.TradeEvent{
		ExchangeID:  "binance",
		AccountID:   b.opts.AccountID,
		Symbol:      o.Symbol,
		Side:        o.Side,
		Price:       parseFloat(o.Price),
		Quantity:    parseFloat(o.Quantity),
		QuoteQty:    parseFloat(o.QuoteQty),
		RealizedPnl: parseFloat(o.RealizedPnl),
		Commission:  parseFloat(o.Commission),
		TradeID:     strconv.FormatInt(o.TradeID, 10),
		OrderID:     strconv.FormatInt(o.OrderID, 10),
		TradeTime:   time.UnixMilli(o.TradeTime),
		IngestTime:  time.Now(),
		Source:       "ws",
	}

	select {
	case b.tradeCh <- trade:
	default:
		b.logger.Warn("trade channel full, dropping event")
	}
}

// handleAccountUpdate 处理账户更新推送（仓位 + 余额变动）
func (b *BinanceFuturesAdapter) handleAccountUpdate(msg []byte, eventTime int64) {
	var update struct {
		Account struct {
			Balances []struct {
				Asset            string `json:"a"`
				WalletBalance    string `json:"wb"`
				CrossWalletBalance string `json:"cw"`
			} `json:"B"`
			Positions []struct {
				Symbol       string `json:"s"`
				PositionAmt  string `json:"pa"`
				EntryPrice   string `json:"ep"`
				UnrealizedPnl string `json:"up"`
				MarginType   string `json:"mt"`
				PositionSide string `json:"ps"`
			} `json:"P"`
		} `json:"a"`
	}
	if err := json.Unmarshal(msg, &update); err != nil {
		b.logger.Warn("parse ACCOUNT_UPDATE failed", zap.Error(err))
		return
	}

	now := time.Now()

	// 推送余额变动
	for _, bal := range update.Account.Balances {
		snapshot := models.BalanceSnapshot{
			ExchangeID:    "binance",
			AccountID:     b.opts.AccountID,
			Asset:         bal.Asset,
			WalletBalance: parseFloat(bal.WalletBalance),
			SnapshotTime:  now,
			Source:        "ws",
		}
		select {
		case b.balanceCh <- snapshot:
		default:
			b.logger.Warn("balance channel full")
		}
	}

	// 推送仓位变动
	for _, pos := range update.Account.Positions {
		snapshot := models.PositionSnapshot{
			ExchangeID:    "binance",
			AccountID:     b.opts.AccountID,
			Symbol:        pos.Symbol,
			PositionSide:  pos.PositionSide,
			Quantity:      parseFloat(pos.PositionAmt),
			EntryPrice:    parseFloat(pos.EntryPrice),
			UnrealizedPnl: parseFloat(pos.UnrealizedPnl),
			MarginType:    pos.MarginType,
			SnapshotTime:  now,
			Source:        "ws",
		}
		select {
		case b.positionCh <- snapshot:
		default:
			b.logger.Warn("position channel full")
		}
	}

	// 推送原始账户更新事件
	select {
	case b.accountCh <- models.AccountUpdate{
		ExchangeID: "binance",
		AccountID:  b.opts.AccountID,
		EventType:  "ACCOUNT_UPDATE",
		EventTime:  time.UnixMilli(eventTime),
		RawData:    json.RawMessage(msg),
	}:
	default:
	}
}

// keepAliveLoop 每 30 分钟续期 listenKey（PUT /fapi/v1/listenKey）
// 如果续期失败则触发完整重连（创建新 listenKey + 新 WS 连接）
func (b *BinanceFuturesAdapter) keepAliveLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := b.keepAliveListenKey(ctx)
			cancel()
			if err != nil {
				b.logger.Error("listenKey 续期失败，触发重连",
					zap.Error(err),
					zap.Int64("累计重连次数", b.reconnectCount.Load()))
				b.reconnect()
			} else {
				b.logger.Debug("listenKey 续期成功",
					zap.String("key", b.listenKey[:8]+"..."))
			}
		}
	}
}

// reconnect 重连 WebSocket
// 重连策略：指数退避（1s → 2s → 4s → 8s → ... → 30s），最多尝试 10 次
// 每次重连都会重新创建 listenKey 并建立新的 WS 连接
// 使用 reconnectMu 防止并发重连（readLoop 和 keepAliveLoop 可能同时触发）
// 返回 true 表示重连成功，false 表示所有尝试均失败
func (b *BinanceFuturesAdapter) reconnect() bool {
	// 防止并发重连
	b.reconnectMu.Lock()
	defer b.reconnectMu.Unlock()

	// 已关闭则不再重连
	if b.closed.Load() {
		return false
	}

	count := b.reconnectCount.Add(1)
	b.logger.Info("开始 WS 重连", zap.Int64("第N次重连", count))

	b.wsMu.Lock()
	if b.wsConn != nil {
		b.wsConn.Close()
		b.wsConn = nil
	}
	b.wsMu.Unlock()

	backoff := time.Second // 初始退避 1s
	const maxBackoff = 30 * time.Second

	for i := 0; i < 10; i++ {
		// 检查是否在重连期间收到了关闭信号
		select {
		case <-b.stopCh:
			b.logger.Info("重连中途收到关闭信号，放弃重连")
			return false
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		key, err := b.createListenKey(ctx)
		if err != nil {
			cancel()
			b.logger.Warn("重连: 创建 listenKey 失败",
				zap.Int("尝试次数", i+1),
				zap.Int("最大尝试", 10),
				zap.Duration("退避时间", backoff),
				zap.Error(err))
			time.Sleep(backoff)
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		b.wsMu.Lock()
		b.listenKey = key
		b.wsMu.Unlock()

		wsURL := fmt.Sprintf("%s/ws/%s", b.opts.WSEndpoint, b.listenKey)
		dialer := websocket.DefaultDialer
		if b.opts.HTTPProxy != "" {
			proxyURL, _ := url.Parse("http://" + b.opts.HTTPProxy)
			dialer = &websocket.Dialer{
				Proxy:            http.ProxyURL(proxyURL),
				HandshakeTimeout: 15 * time.Second,
			}
		}

		conn, _, err := dialer.DialContext(ctx, wsURL, nil)
		cancel()
		if err != nil {
			b.logger.Warn("重连: WS 拨号失败",
				zap.Int("尝试次数", i+1),
				zap.Int("最大尝试", 10),
				zap.Duration("退避时间", backoff),
				zap.Error(err))
			time.Sleep(backoff)
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		b.wsMu.Lock()
		b.wsConn = conn
		b.wsMu.Unlock()

		b.logger.Info("WS 重连成功",
			zap.Int("本次尝试次数", i+1),
			zap.Int64("累计重连次数", count),
			zap.String("listenKey", key[:8]+"..."))
		return true
	}

	b.logger.Error("WS 重连失败，已耗尽所有尝试",
		zap.Int("最大尝试次数", 10),
		zap.Int64("累计重连次数", count))
	return false
}

// ReconnectCount 返回累计重连次数（用于监控和测试）
func (b *BinanceFuturesAdapter) ReconnectCount() int64 {
	return b.reconnectCount.Load()
}

// ==================== REST 辅助方法 ====================

// createListenKey 创建 User Data Stream listenKey
func (b *BinanceFuturesAdapter) createListenKey(ctx context.Context) (string, error) {
	reqURL := fmt.Sprintf("%s/fapi/v1/listenKey", b.opts.RESTEndpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-MBX-APIKEY", b.apiKey)

	resp, err := b.restClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("create listenKey status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ListenKey string `json:"listenKey"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.ListenKey, nil
}

// keepAliveListenKey 续期 listenKey
func (b *BinanceFuturesAdapter) keepAliveListenKey(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/fapi/v1/listenKey", b.opts.RESTEndpoint)
	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-MBX-APIKEY", b.apiKey)

	resp, err := b.restClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keepAlive status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// signedGet 带 HMAC-SHA256 签名的 GET 请求
// 币安认证接口要求所有请求携带 timestamp + signature 参数，
// signature = HMAC-SHA256(queryString, secretKey)
func (b *BinanceFuturesAdapter) signedGet(ctx context.Context, path string, extra url.Values) ([]byte, error) {
	params := url.Values{}
	if extra != nil {
		for k, v := range extra {
			params[k] = v
		}
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("signature", b.sign(params.Encode()))

	reqURL := fmt.Sprintf("%s%s?%s", b.opts.RESTEndpoint, path, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", b.apiKey)

	resp, err := b.restClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("binance %s status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

// sign HMAC-SHA256 签名
func (b *BinanceFuturesAdapter) sign(payload string) string {
	h := hmac.New(sha256.New, []byte(b.secretKey))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// ==================== 币安 REST 响应结构 ====================

// binanceAccountResp 币安 GET /fapi/v2/account 响应结构
// 包含账户所有资产余额和仓位信息，数值字段均为字符串类型需解析
type binanceAccountResp struct {
	Assets []struct {
		Asset            string `json:"asset"`
		WalletBalance    string `json:"walletBalance"`
		AvailableBalance string `json:"availableBalance"`
		UnrealizedProfit string `json:"unrealizedProfit"`
		MarginBalance    string `json:"marginBalance"`
		MaintMargin      string `json:"maintMargin"`
	} `json:"assets"`
	Positions []struct {
		Symbol           string `json:"symbol"`
		PositionSide     string `json:"positionSide"`
		PositionAmt      string `json:"positionAmt"`
		EntryPrice       string `json:"entryPrice"`
		MarkPrice        string `json:"markPrice"`
		UnrealizedProfit string `json:"unrealizedProfit"`
		Leverage         string `json:"leverage"`
		MarginType       string `json:"marginType"`
	} `json:"positions"`
}

// binanceAPIRestrictions 币安 GET /sapi/v1/account/apiRestrictions 响应结构
// 描述 API Key 的各项权限开关状态
type binanceAPIRestrictions struct {
	IPRestrict                    bool `json:"ipRestrict"`
	EnableWithdrawals             bool `json:"enableWithdrawals"`
	EnableInternalTransfer        bool `json:"enableInternalTransfer"`
	EnableMargin                  bool `json:"enableMargin"`
	EnableSpotAndMarginTrading    bool `json:"enableSpotAndMarginTrading"`
	EnableFutures                 bool `json:"enableFutures"`
}

// ==================== 工具函数 ====================

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
