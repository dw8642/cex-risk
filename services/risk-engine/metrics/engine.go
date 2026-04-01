package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/mq"
	"github.com/cex-risk/cex-risk/pkg/store"
)

const (
	// consumerGroupID 指标引擎消费者组 ID
	consumerGroupID = "risk-metrics-engine"

	// positionTTL 仓位数据 Redis 过期时间（2 小时）
	positionTTL = 2 * time.Hour
	// balanceTTL 余额数据 Redis 过期时间（2 小时）
	balanceTTL = 2 * time.Hour
	// freshnessTTL freshness 键过期时间（24 小时）
	freshnessTTL = 24 * time.Hour
)

// Engine 指标引擎 — 消费 Kafka 消息并更新 Redis 实时指标
// 负责：
//   - 消费 exchange.trades → 更新滑动窗口成交指标
//   - 消费 exchange.positions → 更新 position:{account}:{symbol} Hash
//   - 消费 exchange.balances → 更新 balance:{account}:{asset} Hash
//   - 维护 freshness:{account}:{dim} 数据新鲜度时间戳
type Engine struct {
	redis         *store.Redis
	cfg           *config.Config
	consumerGroup *mq.ConsumerGroup
	logger        *zap.Logger
	metricsWindow time.Duration // 成交统计滑动窗口大小
}

// NewEngine 创建指标引擎实例
func NewEngine(redis *store.Redis, cfg *config.Config, logger *zap.Logger) (*Engine, error) {
	if redis == nil {
		return nil, fmt.Errorf("redis 不能为 nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config 不能为 nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger 不能为 nil")
	}

	e := &Engine{
		redis:         redis,
		cfg:           cfg,
		logger:        logger,
		metricsWindow: cfg.RiskEngine.ParseMetricsWindow(),
	}

	// 创建消费者组，为每个 topic 创建独立的消费者
	group := mq.NewConsumerGroup(logger)

	// 成交事件消费者
	tradeConsumer := mq.NewConsumer(mq.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topics.TradeEvents,
		GroupID: consumerGroupID,
	}, e.handleTradeEvent, logger)
	group.Add(tradeConsumer)

	// 仓位快照消费者
	positionConsumer := mq.NewConsumer(mq.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topics.PositionSnapshots,
		GroupID: consumerGroupID,
	}, e.handlePositionSnapshot, logger)
	group.Add(positionConsumer)

	// 余额快照消费者
	balanceConsumer := mq.NewConsumer(mq.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topics.BalanceSnapshots,
		GroupID: consumerGroupID,
	}, e.handleBalanceSnapshot, logger)
	group.Add(balanceConsumer)

	e.consumerGroup = group
	return e, nil
}

// Start 启动指标引擎，并行消费所有 topic（非阻塞）
func (e *Engine) Start(ctx context.Context) {
	e.logger.Info("指标引擎启动",
		zap.Duration("metrics_window", e.metricsWindow),
		zap.String("consumer_group", consumerGroupID))
	e.consumerGroup.StartAll(ctx)
}

// Stop 停止指标引擎，关闭所有消费者
func (e *Engine) Stop() {
	e.logger.Info("指标引擎停止")
	e.consumerGroup.CloseAll()
}

// handleTradeEvent 处理成交事件消息
// 解析 TradeEvent → 更新滑动窗口成交计数、交易币对集合、最后活跃时间
func (e *Engine) handleTradeEvent(ctx context.Context, msg kafka.Message) error {
	var trade models.TradeEvent
	if err := json.Unmarshal(msg.Value, &trade); err != nil {
		return fmt.Errorf("反序列化 TradeEvent 失败: %w", err)
	}

	accountID := trade.AccountID
	window := e.metricsWindow.String()
	tradeTS := float64(trade.TradeTime.UnixMilli())

	// 添加成交到滑动窗口
	if err := e.redis.AddTradeToWindow(ctx, accountID, window, trade.TradeID, tradeTS); err != nil {
		return fmt.Errorf("写入成交窗口失败 account=%s trade=%s: %w", accountID, trade.TradeID, err)
	}

	// 裁剪过期数据
	cutoff := float64(time.Now().Add(-e.metricsWindow).UnixMilli())
	if err := e.redis.TrimTradeWindow(ctx, accountID, window, cutoff); err != nil {
		return fmt.Errorf("裁剪成交窗口失败 account=%s: %w", accountID, err)
	}

	// 记录交易币对
	if err := e.redis.AddTradedSymbol(ctx, accountID, window, trade.Symbol, e.metricsWindow); err != nil {
		return fmt.Errorf("记录交易币对失败 account=%s symbol=%s: %w", accountID, trade.Symbol, err)
	}

	// 更新最后活跃时间
	if err := e.redis.SetLastActivity(ctx, accountID, trade.TradeTime); err != nil {
		return fmt.Errorf("更新活跃时间失败 account=%s: %w", accountID, err)
	}

	// 更新 freshness 时间戳
	if err := e.setFreshness(ctx, accountID, "trade", trade.TradeTime); err != nil {
		return fmt.Errorf("更新 trade freshness 失败 account=%s: %w", accountID, err)
	}

	e.logger.Debug("处理成交事件",
		zap.String("account", accountID),
		zap.String("symbol", trade.Symbol),
		zap.String("trade_id", trade.TradeID),
		zap.Float64("price", trade.Price),
		zap.Float64("qty", trade.Quantity))

	return nil
}

// handlePositionSnapshot 处理仓位快照消息
// 解析 PositionSnapshot → 写入 Redis Hash position:{account}:{symbol}
func (e *Engine) handlePositionSnapshot(ctx context.Context, msg kafka.Message) error {
	var pos models.PositionSnapshot
	if err := json.Unmarshal(msg.Value, &pos); err != nil {
		return fmt.Errorf("反序列化 PositionSnapshot 失败: %w", err)
	}

	accountID := pos.AccountID
	fields := map[string]interface{}{
		"exchange_id":    pos.ExchangeID,
		"symbol":         pos.Symbol,
		"position_side":  pos.PositionSide,
		"quantity":       strconv.FormatFloat(pos.Quantity, 'f', -1, 64),
		"entry_price":    strconv.FormatFloat(pos.EntryPrice, 'f', -1, 64),
		"mark_price":     strconv.FormatFloat(pos.MarkPrice, 'f', -1, 64),
		"unrealized_pnl": strconv.FormatFloat(pos.UnrealizedPnl, 'f', -1, 64),
		"leverage":       strconv.Itoa(pos.Leverage),
		"margin_type":    pos.MarginType,
		"snapshot_time":  pos.SnapshotTime.Format(time.RFC3339),
		"source":         pos.Source,
	}

	if err := e.redis.SetPosition(ctx, accountID, pos.Symbol, fields, positionTTL); err != nil {
		return fmt.Errorf("写入仓位快照失败 account=%s symbol=%s: %w", accountID, pos.Symbol, err)
	}

	// 更新最后活跃时间和 freshness
	if err := e.redis.SetLastActivity(ctx, accountID, pos.SnapshotTime); err != nil {
		return fmt.Errorf("更新活跃时间失败 account=%s: %w", accountID, err)
	}
	if err := e.setFreshness(ctx, accountID, "position", pos.SnapshotTime); err != nil {
		return fmt.Errorf("更新 position freshness 失败 account=%s: %w", accountID, err)
	}

	e.logger.Debug("处理仓位快照",
		zap.String("account", accountID),
		zap.String("symbol", pos.Symbol),
		zap.Float64("qty", pos.Quantity),
		zap.Float64("mark_price", pos.MarkPrice))

	return nil
}

// handleBalanceSnapshot 处理余额快照消息
// 解析 BalanceSnapshot → 写入 Redis Hash balance:{account}:{asset}
func (e *Engine) handleBalanceSnapshot(ctx context.Context, msg kafka.Message) error {
	var bal models.BalanceSnapshot
	if err := json.Unmarshal(msg.Value, &bal); err != nil {
		return fmt.Errorf("反序列化 BalanceSnapshot 失败: %w", err)
	}

	accountID := bal.AccountID
	fields := map[string]interface{}{
		"exchange_id":       bal.ExchangeID,
		"asset":             bal.Asset,
		"wallet_balance":    strconv.FormatFloat(bal.WalletBalance, 'f', -1, 64),
		"available_balance": strconv.FormatFloat(bal.AvailableBalance, 'f', -1, 64),
		"unrealized_pnl":    strconv.FormatFloat(bal.UnrealizedPnl, 'f', -1, 64),
		"margin_balance":    strconv.FormatFloat(bal.MarginBalance, 'f', -1, 64),
		"maint_margin":      strconv.FormatFloat(bal.MaintMargin, 'f', -1, 64),
		"snapshot_time":     bal.SnapshotTime.Format(time.RFC3339),
		"source":            bal.Source,
	}

	if err := e.redis.SetBalance(ctx, accountID, bal.Asset, fields, balanceTTL); err != nil {
		return fmt.Errorf("写入余额快照失败 account=%s asset=%s: %w", accountID, bal.Asset, err)
	}

	// 更新最后活跃时间和 freshness
	if err := e.redis.SetLastActivity(ctx, accountID, bal.SnapshotTime); err != nil {
		return fmt.Errorf("更新活跃时间失败 account=%s: %w", accountID, err)
	}
	if err := e.setFreshness(ctx, accountID, "balance", bal.SnapshotTime); err != nil {
		return fmt.Errorf("更新 balance freshness 失败 account=%s: %w", accountID, err)
	}

	e.logger.Debug("处理余额快照",
		zap.String("account", accountID),
		zap.String("asset", bal.Asset),
		zap.Float64("wallet", bal.WalletBalance),
		zap.Float64("available", bal.AvailableBalance))

	return nil
}

// setFreshness 更新数据新鲜度时间戳
// key 格式: freshness:{accountID}:{dim}，dim 为 trade / position / balance
func (e *Engine) setFreshness(ctx context.Context, accountID, dim string, t time.Time) error {
	key := fmt.Sprintf("freshness.%s.%s", accountID, dim)
	return e.redis.Client().Set(ctx, key, t.Unix(), freshnessTTL).Err()
}

// GetFreshness 获取数据新鲜度时间戳（供外部查询）
func GetFreshness(ctx context.Context, r *store.Redis, accountID, dim string) (time.Time, error) {
	key := fmt.Sprintf("freshness.%s.%s", accountID, dim)
	val, err := r.Client().Get(ctx, key).Result()
	if err != nil {
		return time.Time{}, fmt.Errorf("读取 freshness %s 失败: %w", key, err)
	}
	ts, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析 freshness 时间戳失败 key=%s val=%s: %w", key, val, err)
	}
	return time.Unix(ts, 0), nil
}
