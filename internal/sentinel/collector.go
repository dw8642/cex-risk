// collector.go — 数据采集器
//
// 管理多个账户的 BinanceRESTClient，周期性采集数据。
// 对每个账户并发采集，汇总返回 []AccountData 供 Engine 评估。
// 低频数据（权限、费率）按独立节奏刷新，不随每 tick 重复请求。
package sentinel

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Collector 数据采集器
type Collector struct {
	clients map[string]*BinanceRESTClient // key: account ID
	configs []AccountConfig
	redis   *redis.Client
	symbol  string // 监控的交易对
	logger  *zap.Logger

	// 上一轮仓位快照（内存保存，供 E-001/E-005 对比）
	prevPositions map[string][]PositionInfo

	// 低频数据缓存
	lastPermCheck    map[string]time.Time   // 上次权限检查时间
	lastFundingCheck time.Time              // 上次费率检查时间
	fundingCache     []FundingInfo          // 费率缓存（所有账户共享）
	lastPremiumCheck time.Time              // 上次 premiumIndex 检查时间
	premiumCache     map[string]FundingInfo // symbol → FundingInfo（含 fundingRate）

	// 价格滑动窗口（M-001 用，所有账户共享）
	priceHistory map[string][]PricePoint // symbol → 价格时间序列

	mu sync.Mutex // 保护 prevPositions 和缓存
}

// NewCollector 创建采集器
// 为每个账户创建对应的 BinanceRESTClient（根据 account_type 选择端点）
func NewCollector(cfg *SentinelConfig, redisClient *redis.Client, logger *zap.Logger) *Collector {
	clients := make(map[string]*BinanceRESTClient, len(cfg.Accounts))
	for _, acc := range cfg.Accounts {
		clients[acc.ID] = NewBinanceRESTClient(
			acc.APIKey,
			acc.SecretKey,
			cfg.Binance.ActiveRESTEndpoint(),
			cfg.Binance.PMRESTEndpoint(),
			acc.AccountType,
			acc.ID,
			cfg.System.HTTPProxy, // ⚠️ 所有请求走 Proxy
			cfg.System.DebugSignRequests,
			logger,
		)
	}

	return &Collector{
		clients:       clients,
		configs:       cfg.Accounts,
		redis:         redisClient,
		symbol:        cfg.Sentinel.Symbol,
		logger:        logger,
		prevPositions: make(map[string][]PositionInfo),
		lastPermCheck: make(map[string]time.Time),
		priceHistory:  make(map[string][]PricePoint),
	}
}

// CollectAll 采集所有账户的最新数据
// 对每个账户并发采集，汇总返回
func (c *Collector) CollectAll(ctx context.Context) []AccountData {
	results := make([]AccountData, len(c.configs))
	var wg sync.WaitGroup

	for i, acc := range c.configs {
		wg.Add(1)
		go func(idx int, a AccountConfig) {
			defer wg.Done()
			results[idx] = c.collectOne(ctx, a)
		}(i, acc)
	}
	wg.Wait()
	return results
}

// collectOne 采集单个账户
func (c *Collector) collectOne(ctx context.Context, acc AccountConfig) AccountData {
	client, ok := c.clients[acc.ID]
	if !ok {
		return AccountData{
			Config:      acc,
			CollectTime: time.Now(),
			Error:       fmt.Errorf("账户 %s 无对应客户端", acc.ID),
		}
	}

	now := time.Now()
	data := AccountData{
		Config:      acc,
		CollectTime: now,
	}

	// 1. 获取账户信息
	accInfo, err := client.GetAccountInfo(ctx)
	if err != nil {
		c.logger.Error("采集账户信息失败",
			zap.String("account", acc.ID),
			zap.String("label", acc.Label),
			zap.Error(err))
		data.Error = fmt.Errorf("账户信息采集失败: %w", err)
		return data
	}
	accInfo.Label = acc.Label
	data.Account = *accInfo

	// 2. 获取仓位
	positions, err := client.GetPositions(ctx)
	if err != nil {
		c.logger.Error("采集仓位失败",
			zap.String("account", acc.ID),
			zap.Error(err))
		data.Error = fmt.Errorf("仓位采集失败: %w", err)
		return data
	}
	data.Positions = positions

	// 3. 附上上一轮仓位快照
	c.mu.Lock()
	if prev, ok := c.prevPositions[acc.ID]; ok {
		data.PrevPositions = prev
	}
	// 更新当前快照为下一轮的 prev
	c.prevPositions[acc.ID] = positions
	c.mu.Unlock()

	// 4. 低频：API 权限检查（每 60s 一次）
	c.mu.Lock()
	lastPerm := c.lastPermCheck[acc.ID]
	c.mu.Unlock()

	if now.Sub(lastPerm) > 60*time.Second {
		withdraw, ipRestrict, err := client.GetAPIPermissions(ctx)
		if err != nil {
			c.logger.Warn("采集权限信息失败（非致命）",
				zap.String("account", acc.ID),
				zap.Error(err))
		} else {
			data.Account.EnableWithdraw = withdraw
			data.Account.IPRestrict = ipRestrict
			c.mu.Lock()
			c.lastPermCheck[acc.ID] = now
			c.mu.Unlock()
		}
	}

	// 5. 低频：资金费率信息（每 5min 一次，所有账户共享）
	c.mu.Lock()
	needFunding := now.Sub(c.lastFundingCheck) > 5*time.Minute
	c.mu.Unlock()

	if needFunding {
		fundingInfos, err := client.GetFundingInfo(ctx)
		if err != nil {
			c.logger.Warn("采集费率信息失败（非致命）",
				zap.Error(err))
		} else {
			c.mu.Lock()
			c.fundingCache = fundingInfos
			c.lastFundingCheck = now
			c.mu.Unlock()
		}
	}

	// 5b. 低频：Premium Index 资金费率（每 5min 一次，所有账户共享）
	c.mu.Lock()
	needPremium := now.Sub(c.lastPremiumCheck) > 5*time.Minute
	c.mu.Unlock()

	if needPremium {
		premiumMap, err := client.GetPremiumIndex(ctx)
		if err != nil {
			c.logger.Warn("采集 premiumIndex 失败（非致命）", zap.Error(err))
		} else {
			c.mu.Lock()
			c.premiumCache = premiumMap
			c.lastPremiumCheck = now
			c.mu.Unlock()
		}
	}

	// 合并 fundingCache + premiumCache → data.FundingInfos
	c.mu.Lock()
	merged := make([]FundingInfo, len(c.fundingCache))
	copy(merged, c.fundingCache)
	if c.premiumCache != nil {
		for i, fi := range merged {
			if pi, ok := c.premiumCache[fi.Symbol]; ok {
				merged[i].FundingRate = pi.FundingRate
				merged[i].NextFundingTime = pi.NextFundingTime
			}
		}
	}
	data.FundingInfos = merged
	c.mu.Unlock()

	// 5c. API 健康指标（S-014 用）
	data.APIHealth = client.GetHealthStats()

	// 5d. 价格滑动窗口更新（M-001 用）
	// 优先从 premiumIndex 取目标 symbol 的 markPrice，回退到持仓 markPrice
	c.mu.Lock()
	priceRecorded := make(map[string]bool)
	targetSymbol := acc.Symbol
	if targetSymbol == "" {
		targetSymbol = c.symbol
	}

	// 优先：从 premiumIndex 取目标 symbol 的价格，即使当前没有持仓也能记录价格历史
	if targetSymbol != "" && c.premiumCache != nil {
		if pi, ok := c.premiumCache[targetSymbol]; ok && pi.MarkPrice > 0 {
			c.recordPrice(targetSymbol, pi.MarkPrice, now)
			priceRecorded[targetSymbol] = true
		}
	}

	// 回退：记录所有持仓 symbol 的 markPrice
	for _, p := range positions {
		if p.MarkPrice <= 0 || priceRecorded[p.Symbol] {
			continue
		}
		c.recordPrice(p.Symbol, p.MarkPrice, now)
	}

	data.PriceHistory = make(map[string][]PricePoint, len(c.priceHistory))
	for sym, pts := range c.priceHistory {
		cp := make([]PricePoint, len(pts))
		copy(cp, pts)
		data.PriceHistory[sym] = cp
	}
	c.mu.Unlock()

	// 6. 写入 Redis（供外部消费者读取 + freshness 检测）
	c.writeRedis(ctx, acc.ID, &data)

	return data
}

// recordPrice 记录价格到滑动窗口（调用方需持有 c.mu 锁）
func (c *Collector) recordPrice(symbol string, price float64, now time.Time) {
	pp := PricePoint{Price: price, Time: now}
	history := c.priceHistory[symbol]
	history = append(history, pp)
	// 裁剪窗口：只保留最近 2 小时的数据（M-001 用 1h，留 buffer）
	cutoff := now.Add(-2 * time.Hour)
	start := 0
	for start < len(history) && history[start].Time.Before(cutoff) {
		start++
	}
	c.priceHistory[symbol] = history[start:]
}

// writeRedis 将采集数据写入 Redis
func (c *Collector) writeRedis(ctx context.Context, accountID string, data *AccountData) {
	if c.redis == nil {
		return
	}

	pipe := c.redis.Pipeline()

	// 账户信息
	if accJSON, err := json.Marshal(data.Account); err == nil {
		pipe.Set(ctx, fmt.Sprintf("sentinel:account:%s", accountID), accJSON, 60*time.Second)
	}

	// 仓位
	if posJSON, err := json.Marshal(data.Positions); err == nil {
		pipe.Set(ctx, fmt.Sprintf("sentinel:positions:%s", accountID), posJSON, 60*time.Second)
	}

	// Freshness 时间戳
	pipe.Set(ctx, fmt.Sprintf("sentinel:freshness:%s", accountID),
		strconv.FormatInt(time.Now().Unix(), 10), 120*time.Second)

	// 费率信息（按 symbol 存储）
	if len(data.FundingInfos) > 0 {
		if fundJSON, err := json.Marshal(data.FundingInfos); err == nil {
			pipe.Set(ctx, fmt.Sprintf("sentinel:funding_info:%s", c.symbol), fundJSON, 600*time.Second)
		}
	}

	if _, err := pipe.Exec(ctx); err != nil {
		c.logger.Warn("Redis 写入失败（非致命）", zap.Error(err))
	}
}
