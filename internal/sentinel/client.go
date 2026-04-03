// client.go — 币安 REST API 轻量级客户端（仅用于 Sentinel 轮询）
//
// 支持两种账户类型：
//   - 普通合约 (regular): /fapi/v2/ 端点
//   - 统一账户 Portfolio Margin (portfolio_margin): /papi/v1/ 端点
//
// 所有 HTTP 请求通过可配置的 HTTP Proxy 发送
package sentinel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// BinanceRESTClient 币安 REST API 客户端
// 每个监控账户对应一个实例，根据 accountType 选择不同的 API 端点
type BinanceRESTClient struct {
	apiKey      string
	secretKey   string
	fapiBaseURL string       // 普通合约: https://fapi.binance.com
	papiBaseURL string       // PM: https://papi.binance.com
	httpClient  *http.Client // ⚠️ 已配置 Proxy
	accountType string       // "regular" | "portfolio_margin"
	accountID   string       // 风控系统内部 ID
	proxy       string
	debugSign   bool
	logger      *zap.Logger

	// API 健康指标跟踪（S-014 用）
	healthMu          sync.Mutex
	requestLatencies  []time.Duration // 滑动窗口内的请求延迟
	requestErrors     int             // 窗口内错误计数
	requestTotal      int             // 窗口内总请求数
	healthWindowStart time.Time       // 窗口起始时间
}

// NewBinanceRESTClient 创建 REST 客户端
// proxy: HTTP 代理地址（如 "127.0.0.1:7897"），空字符串=不使用代理
func NewBinanceRESTClient(apiKey, secretKey, fapiBaseURL, papiBaseURL, accountType, accountID, proxy string, debugSign bool, logger *zap.Logger) *BinanceRESTClient {
	return &BinanceRESTClient{
		apiKey:      apiKey,
		secretKey:   secretKey,
		fapiBaseURL: fapiBaseURL,
		papiBaseURL: papiBaseURL,
		httpClient:  newProxiedHTTPClient(proxy, 15*time.Second),
		accountType: accountType,
		accountID:   accountID,
		proxy:       proxy,
		debugSign:   debugSign,
		logger:      logger,
	}
}

// newProxiedHTTPClient 创建带 Proxy 的 HTTP 客户端
func newProxiedHTTPClient(proxy string, timeout time.Duration) *http.Client {
	transport := &http.Transport{}
	if proxy != "" {
		proxyURL, err := url.Parse("http://" + proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func previewBody(body []byte, limit int) string {
	if len(body) <= limit {
		return string(body)
	}
	return string(body[:limit]) + fmt.Sprintf("...<truncated %d bytes>", len(body)-limit)
}

// ==================== 统一入口 ====================

// GetAccountInfo 根据 accountType 自动选择正确的端点获取账户信息
func (c *BinanceRESTClient) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	if c.accountType == "portfolio_margin" {
		return c.getPMAccount(ctx)
	}
	return c.getFuturesAccount(ctx)
}

// GetPositions 根据 accountType 自动选择正确的端点获取仓位
func (c *BinanceRESTClient) GetPositions(ctx context.Context) ([]PositionInfo, error) {
	if c.accountType == "portfolio_margin" {
		return c.getPMPositions(ctx)
	}
	return c.getFuturesPositions(ctx)
}

// ==================== 普通合约端点 ====================

// getFuturesAccount 获取普通合约账户信息
// GET /fapi/v2/account (signed)
func (c *BinanceRESTClient) getFuturesAccount(ctx context.Context) (*AccountInfo, error) {
	data, err := c.signedGet(ctx, c.fapiBaseURL+"/fapi/v2/account", nil)
	if err != nil {
		return nil, fmt.Errorf("getFuturesAccount: %w", err)
	}

	var resp binanceFuturesAccountResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("getFuturesAccount parse: %w", err)
	}

	maintMargin := parseFloat64(resp.TotalMaintMargin)
	marginBalance := parseFloat64(resp.TotalMarginBalance)
	var marginRatio float64
	if marginBalance > 0 {
		marginRatio = maintMargin / marginBalance
	}

	return &AccountInfo{
		AccountID:          c.accountID,
		AccountType:        "regular",
		MarginRatio:        marginRatio,
		UniMMR:             0,
		AccountStatus:      "N/A",
		TotalEquity:        parseFloat64(resp.TotalWalletBalance),
		AvailableMargin:    parseFloat64(resp.AvailableBalance),
		TotalMaintMargin:   maintMargin,
		TotalMarginBalance: marginBalance,
		UpdatedAt:          time.Now(),
	}, nil
}

// getFuturesPositions 获取普通合约仓位
// GET /fapi/v2/positionRisk (signed)
func (c *BinanceRESTClient) getFuturesPositions(ctx context.Context) ([]PositionInfo, error) {
	data, err := c.signedGet(ctx, c.fapiBaseURL+"/fapi/v2/positionRisk", nil)
	if err != nil {
		return nil, fmt.Errorf("getFuturesPositions: %w", err)
	}

	var resp []binancePositionRiskResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("getFuturesPositions parse: %w", err)
	}

	return convertPositions(resp), nil
}

// ==================== 统一账户 (Portfolio Margin) 端点 ====================

// getPMAccount 获取统一账户信息
// GET /papi/v1/account (signed)
func (c *BinanceRESTClient) getPMAccount(ctx context.Context) (*AccountInfo, error) {
	data, err := c.signedGet(ctx, c.papiBaseURL+"/papi/v1/account", nil)
	if err != nil {
		return nil, fmt.Errorf("getPMAccount: %w", err)
	}

	var resp binancePMAccountResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("getPMAccount parse: %w", err)
	}

	uniMMR := parseFloat64(resp.UniMMR)
	equity := parseFloat64(resp.AccountEquity)
	maintMargin := parseFloat64(resp.AccountMaintMargin)

	// 将 uniMMR 转换为可比的 marginRatio（越大越危险）
	// uniMMR 越小越危险，所以 marginRatio = 1/uniMMR（近似）
	var marginRatio float64
	if uniMMR > 0 {
		marginRatio = 1.0 / uniMMR
	}

	return &AccountInfo{
		AccountID:        c.accountID,
		AccountType:      "portfolio_margin",
		MarginRatio:      marginRatio,
		UniMMR:           uniMMR,
		AccountStatus:    resp.AccountStatus,
		TotalEquity:      equity,
		AvailableMargin:  equity - maintMargin,
		TotalMaintMargin: maintMargin,
		UpdatedAt:        time.Now(),
	}, nil
}

// getPMPositions 获取统一账户 U本位合约仓位
// GET /papi/v1/um/positionRisk (signed)
func (c *BinanceRESTClient) getPMPositions(ctx context.Context) ([]PositionInfo, error) {
	data, err := c.signedGet(ctx, c.papiBaseURL+"/papi/v1/um/positionRisk", nil)
	if err != nil {
		return nil, fmt.Errorf("getPMPositions: %w", err)
	}

	var resp []binancePositionRiskResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("getPMPositions parse: %w", err)
	}

	return convertPositions(resp), nil
}

// ==================== 公共端点 ====================

// GetAPIPermissions 获取 API Key 权限
// GET /sapi/v1/account/apiRestrictions (signed)
// sapi 走现货 API 域名，需要将 fapi 替换为 api
func (c *BinanceRESTClient) GetAPIPermissions(ctx context.Context) (enableWithdraw bool, ipRestrict bool, err error) {
	spotBase := strings.Replace(c.fapiBaseURL, "fapi", "api", 1)
	data, err := c.signedGet(ctx, spotBase+"/sapi/v1/account/apiRestrictions", nil)
	if err != nil {
		return false, false, fmt.Errorf("getAPIPermissions: %w", err)
	}

	var resp binanceAPIRestrictionsResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return false, false, fmt.Errorf("getAPIPermissions parse: %w", err)
	}

	return resp.EnableWithdrawals, resp.IPRestrict, nil
}

// GetFundingInfo 获取资金费率信息（公共端点，无需签名）
// GET /fapi/v1/fundingInfo
func (c *BinanceRESTClient) GetFundingInfo(ctx context.Context) ([]FundingInfo, error) {
	reqURL := c.fapiBaseURL + "/fapi/v1/fundingInfo"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("getFundingInfo request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getFundingInfo http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("getFundingInfo read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("getFundingInfo status %d: %s", resp.StatusCode, string(body))
	}

	var items []binanceFundingInfoResp
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("getFundingInfo parse: %w", err)
	}

	var result []FundingInfo
	for _, item := range items {
		result = append(result, FundingInfo{
			Symbol:          item.Symbol,
			FundingInterval: int64(item.FundingIntervalHours) * 3600 * 1000, // 小时→毫秒
		})
	}
	return result, nil
}

// GetServerTime 获取 Binance 服务器时间与本地时钟偏差。
// 当前统一使用 futures time 端点，便于排查 signed futures request 的 timestamp 问题。
func (c *BinanceRESTClient) GetServerTime(ctx context.Context) (serverTime time.Time, localTime time.Time, skew time.Duration, err error) {
	reqURL := c.fapiBaseURL + "/fapi/v1/time"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("getServerTime request: %w", err)
	}

	localBefore := time.Now()
	resp, err := c.httpClient.Do(req)
	localAfter := time.Now()
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("getServerTime http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("getServerTime read: %w", err)
	}
	if c.debugSign && c.logger != nil {
		c.logger.Info("Binance 时间接口响应",
			zap.String("account_id", c.accountID),
			zap.String("endpoint", reqURL),
			zap.Int("status_code", resp.StatusCode),
			zap.String("body_preview", previewBody(body, 256)),
		)
	}
	if resp.StatusCode != 200 {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("getServerTime status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("getServerTime parse: %w", err)
	}

	serverTime = time.UnixMilli(payload.ServerTime)
	localTime = localBefore.Add(localAfter.Sub(localBefore) / 2)
	skew = localTime.Sub(serverTime)
	return serverTime, localTime, skew, nil
}

// GetPremiumIndex 获取溢价指数（含 lastFundingRate）
// GET /fapi/v1/premiumIndex（公共端点，无需签名）
func (c *BinanceRESTClient) GetPremiumIndex(ctx context.Context) (map[string]FundingInfo, error) {
	reqURL := c.fapiBaseURL + "/fapi/v1/premiumIndex"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("getPremiumIndex request: %w", err)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	c.recordLatency(time.Since(start), err != nil)
	if err != nil {
		return nil, fmt.Errorf("getPremiumIndex http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("getPremiumIndex read: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("getPremiumIndex status %d: %s", resp.StatusCode, string(body))
	}

	var items []binancePremiumIndexResp
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("getPremiumIndex parse: %w", err)
	}

	result := make(map[string]FundingInfo, len(items))
	for _, item := range items {
		result[item.Symbol] = FundingInfo{
			Symbol:          item.Symbol,
			FundingRate:     parseFloat64(item.LastFundingRate),
			NextFundingTime: item.NextFundingTime,
			MarkPrice:       parseFloat64(item.MarkPrice),
		}
	}
	return result, nil
}

// recordLatency 记录 API 请求延迟和错误（S-014 用）
func (c *BinanceRESTClient) recordLatency(latency time.Duration, isError bool) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	now := time.Now()
	// 每 5 分钟重置窗口
	if c.healthWindowStart.IsZero() || now.Sub(c.healthWindowStart) > 5*time.Minute {
		c.requestLatencies = c.requestLatencies[:0]
		c.requestErrors = 0
		c.requestTotal = 0
		c.healthWindowStart = now
	}

	c.requestLatencies = append(c.requestLatencies, latency)
	c.requestTotal++
	if isError {
		c.requestErrors++
	}
}

// GetHealthStats 获取当前窗口的 API 健康统计
func (c *BinanceRESTClient) GetHealthStats() *APIHealthStats {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	if c.requestTotal == 0 {
		return nil
	}

	var totalMs, maxMs float64
	for _, lat := range c.requestLatencies {
		ms := float64(lat.Milliseconds())
		totalMs += ms
		if ms > maxMs {
			maxMs = ms
		}
	}

	return &APIHealthStats{
		TotalRequests: c.requestTotal,
		ErrorCount:    c.requestErrors,
		AvgLatencyMs:  totalMs / float64(len(c.requestLatencies)),
		MaxLatencyMs:  maxMs,
		WindowStart:   c.healthWindowStart,
	}
}

// ==================== 签名与 HTTP ====================

// signedGet 带 HMAC-SHA256 签名的 GET 请求
func (c *BinanceRESTClient) signedGet(ctx context.Context, fullURL string, extra url.Values) ([]byte, error) {
	params := url.Values{}
	for k, v := range extra {
		params[k] = v
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("recvWindow", "5000")
	payload := params.Encode()
	signature := c.sign(payload)
	params.Set("signature", signature)

	reqURL := fullURL + "?" + params.Encode()
	if c.debugSign && c.logger != nil {
		c.logger.Info("Binance 签名请求调试",
			zap.String("account_id", c.accountID),
			zap.String("account_type", c.accountType),
			zap.String("endpoint", fullURL),
			zap.String("payload", payload),
			zap.String("signature_prefix", signature[:8]),
			zap.Bool("proxy_enabled", c.proxy != ""),
			zap.String("proxy", c.proxy),
		)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", c.apiKey)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		c.recordLatency(latency, true)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.recordLatency(latency, true)
		return nil, err
	}
	isErr := resp.StatusCode != 200
	c.recordLatency(latency, isErr)

	if c.debugSign && c.logger != nil {
		c.logger.Info("Binance 签名请求响应",
			zap.String("account_id", c.accountID),
			zap.String("endpoint", fullURL),
			zap.Int("status_code", resp.StatusCode),
			zap.String("body_preview", previewBody(body, 256)),
		)
	}

	if isErr {
		return nil, fmt.Errorf("binance %s status %d: %s", fullURL, resp.StatusCode, string(body))
	}
	return body, nil
}

// sign HMAC-SHA256 签名
func (c *BinanceRESTClient) sign(payload string) string {
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// ==================== 响应结构 ====================

type binanceFuturesAccountResp struct {
	TotalMaintMargin   string `json:"totalMaintMargin"`
	TotalMarginBalance string `json:"totalMarginBalance"`
	TotalWalletBalance string `json:"totalWalletBalance"`
	AvailableBalance   string `json:"availableBalance"`
	Assets             []struct {
		Asset            string `json:"asset"`
		WalletBalance    string `json:"walletBalance"`
		AvailableBalance string `json:"availableBalance"`
		MaintMargin      string `json:"maintMargin"`
		MarginBalance    string `json:"marginBalance"`
	} `json:"assets"`
	Positions []struct {
		Symbol           string `json:"symbol"`
		PositionAmt      string `json:"positionAmt"`
		EntryPrice       string `json:"entryPrice"`
		MarkPrice        string `json:"markPrice"`
		UnrealizedProfit string `json:"unrealizedProfit"`
		Leverage         string `json:"leverage"`
		PositionSide     string `json:"positionSide"`
		Notional         string `json:"notional"`
	} `json:"positions"`
}

type binancePMAccountResp struct {
	UniMMR               string `json:"uniMMR"`
	AccountEquity        string `json:"accountEquity"`
	ActualEquity         string `json:"actualEquity"`
	AccountInitialMargin string `json:"accountInitialMargin"`
	AccountMaintMargin   string `json:"accountMaintMargin"`
	AccountStatus        string `json:"accountStatus"`
}

type binancePositionRiskResp struct {
	Symbol           string `json:"symbol"`
	PositionAmt      string `json:"positionAmt"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnRealizedProfit string `json:"unRealizedProfit"`
	LiquidationPrice string `json:"liquidationPrice"`
	Leverage         string `json:"leverage"`
	PositionSide     string `json:"positionSide"`
	Notional         string `json:"notional"`
	ADLQuantile      int    `json:"adlQuantile"` // ADL 等级 1-5
}

type binancePremiumIndexResp struct {
	Symbol          string `json:"symbol"`
	MarkPrice       string `json:"markPrice"`
	LastFundingRate string `json:"lastFundingRate"`
	NextFundingTime int64  `json:"nextFundingTime"`
}

type binanceFundingInfoResp struct {
	Symbol               string `json:"symbol"`
	FundingIntervalHours int    `json:"fundingIntervalHours"`
}

type binanceAPIRestrictionsResp struct {
	EnableWithdrawals bool `json:"enableWithdrawals"`
	IPRestrict        bool `json:"ipRestrict"`
	EnableFutures     bool `json:"enableFutures"`
}

// ==================== 工具函数 ====================

func parseFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// convertPositions 将币安仓位响应转换为统一 PositionInfo
// 只返回有仓位的（positionAmt != 0）
func convertPositions(resp []binancePositionRiskResp) []PositionInfo {
	var positions []PositionInfo
	for _, p := range resp {
		qty := parseFloat64(p.PositionAmt)
		if qty == 0 {
			continue
		}
		markPrice := parseFloat64(p.MarkPrice)
		positions = append(positions, PositionInfo{
			Symbol:           p.Symbol,
			PositionSide:     p.PositionSide,
			Quantity:         qty,
			EntryPrice:       parseFloat64(p.EntryPrice),
			MarkPrice:        markPrice,
			UnrealizedPnl:    parseFloat64(p.UnRealizedProfit),
			Leverage:         parseInt(p.Leverage),
			NotionalValue:    math.Abs(qty * markPrice),
			LiquidationPrice: parseFloat64(p.LiquidationPrice),
			ADLQuantile:      p.ADLQuantile,
		})
	}
	return positions
}
