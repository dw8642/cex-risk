// ingestor.go — WS 实时数据采集器
//
// Ingestor 订阅交易所适配器的 4 个 channel（trades, positions, balances, accountUpdates），
// 将收到的事件序列化为 JSON 后写入对应的 Kafka topic。
// 每个监控账户对应一个独立的 Ingestor 实例。
//
// Kafka topic 映射：
//   tradeCh    → risk_trade_events
//   positionCh → risk_position_snapshots
//   balanceCh  → risk_balance_snapshots
//   accountCh  → risk_account_updates
package main

import (
	"context"
	"sync"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/exchange"
	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/mq"
	"go.uber.org/zap"
)

// Publisher Kafka 消息发布接口，便于测试时 mock
type Publisher interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
}

// 编译期检查 *mq.Producer 实现 Publisher 接口
var _ Publisher = (*mq.Producer)(nil)

// Ingestor 消费交易所 WS 推送，写入 Kafka
type Ingestor struct {
	adapter   exchange.Adapter
	producer  Publisher
	cfg       *config.Config
	logger    *zap.Logger
	accountID string

	wg     sync.WaitGroup
	stopCh chan struct{}
}

// NewIngestor 创建采集器
func NewIngestor(adapter exchange.Adapter, producer *mq.Producer, cfg *config.Config, logger *zap.Logger, accountID string) *Ingestor {
	return newIngestor(adapter, producer, cfg, logger, accountID)
}

// newIngestor 内部构造器，接受 Publisher 接口（用于测试 mock）
func newIngestor(adapter exchange.Adapter, producer Publisher, cfg *config.Config, logger *zap.Logger, accountID string) *Ingestor {
	return &Ingestor{
		adapter:   adapter,
		producer:  producer,
		cfg:       cfg,
		logger:    logger.With(zap.String("account", accountID), zap.String("component", "ingestor")),
		accountID: accountID,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动采集
func (i *Ingestor) Start(ctx context.Context) {
	// 订阅成交流
	tradeCh, err := i.adapter.SubscribeTrades(ctx, nil)
	if err != nil {
		i.logger.Error("subscribe trades failed", zap.Error(err))
	} else {
		i.wg.Add(1)
		go i.consumeTrades(ctx, tradeCh)
	}

	// 订阅仓位更新
	posCh, err := i.adapter.SubscribePositions(ctx)
	if err != nil {
		i.logger.Error("subscribe positions failed", zap.Error(err))
	} else {
		i.wg.Add(1)
		go i.consumePositions(ctx, posCh)
	}

	// 订阅余额更新
	balCh, err := i.adapter.SubscribeBalances(ctx)
	if err != nil {
		i.logger.Error("subscribe balances failed", zap.Error(err))
	} else {
		i.wg.Add(1)
		go i.consumeBalances(ctx, balCh)
	}

	// 订阅账户更新
	accCh, err := i.adapter.SubscribeAccountUpdates(ctx)
	if err != nil {
		i.logger.Error("subscribe account updates failed", zap.Error(err))
	} else {
		i.wg.Add(1)
		go i.consumeAccountUpdates(ctx, accCh)
	}

	i.logger.Info("ingestor started")
}

// Stop 停止采集
func (i *Ingestor) Stop() {
	close(i.stopCh)
	i.adapter.Close()
	i.wg.Wait()
	i.logger.Info("ingestor stopped")
}

// consumeTrades 消费成交事件，写入 Kafka
func (i *Ingestor) consumeTrades(ctx context.Context, ch <-chan models.TradeEvent) {
	defer i.wg.Done()
	topic := i.cfg.Kafka.Topics.TradeEvents

	for {
		select {
		case <-ctx.Done():
			return
		case <-i.stopCh:
			return
		case trade, ok := <-ch:
			if !ok {
				return
			}
			data, err := trade.Marshal()
			if err != nil {
				i.logger.Error("marshal trade failed", zap.Error(err))
				continue
			}
			if err := i.producer.Publish(ctx, topic, []byte(trade.AccountID), data); err != nil {
				i.logger.Error("publish trade failed", zap.Error(err))
			} else if i.cfg.System.Debug {
				i.logger.Debug("trade published",
					zap.String("symbol", trade.Symbol),
					zap.String("side", trade.Side),
					zap.Float64("price", trade.Price),
					zap.Float64("qty", trade.Quantity))
			}
		}
	}
}

// consumePositions 消费仓位快照，写入 Kafka
func (i *Ingestor) consumePositions(ctx context.Context, ch <-chan models.PositionSnapshot) {
	defer i.wg.Done()
	topic := i.cfg.Kafka.Topics.PositionSnapshots

	for {
		select {
		case <-ctx.Done():
			return
		case <-i.stopCh:
			return
		case pos, ok := <-ch:
			if !ok {
				return
			}
			data, err := pos.Marshal()
			if err != nil {
				i.logger.Error("marshal position failed", zap.Error(err))
				continue
			}
			if err := i.producer.Publish(ctx, topic, []byte(pos.AccountID), data); err != nil {
				i.logger.Error("publish position failed", zap.Error(err))
			} else if i.cfg.System.Debug {
				i.logger.Debug("position published",
					zap.String("symbol", pos.Symbol),
					zap.Float64("qty", pos.Quantity))
			}
		}
	}
}

// consumeBalances 消费余额快照，写入 Kafka
func (i *Ingestor) consumeBalances(ctx context.Context, ch <-chan models.BalanceSnapshot) {
	defer i.wg.Done()
	topic := i.cfg.Kafka.Topics.BalanceSnapshots

	for {
		select {
		case <-ctx.Done():
			return
		case <-i.stopCh:
			return
		case bal, ok := <-ch:
			if !ok {
				return
			}
			data, err := bal.Marshal()
			if err != nil {
				i.logger.Error("marshal balance failed", zap.Error(err))
				continue
			}
			if err := i.producer.Publish(ctx, topic, []byte(bal.AccountID), data); err != nil {
				i.logger.Error("publish balance failed", zap.Error(err))
			} else if i.cfg.System.Debug {
				i.logger.Debug("balance published",
					zap.String("asset", bal.Asset),
					zap.Float64("wallet", bal.WalletBalance))
			}
		}
	}
}

// consumeAccountUpdates 消费账户更新，写入 Kafka
func (i *Ingestor) consumeAccountUpdates(ctx context.Context, ch <-chan models.AccountUpdate) {
	defer i.wg.Done()
	topic := i.cfg.Kafka.Topics.AccountUpdates

	for {
		select {
		case <-ctx.Done():
			return
		case <-i.stopCh:
			return
		case update, ok := <-ch:
			if !ok {
				return
			}
			data, err := update.Marshal()
			if err != nil {
				i.logger.Error("marshal account update failed", zap.Error(err))
				continue
			}
			if err := i.producer.Publish(ctx, topic, []byte(update.AccountID), data); err != nil {
				i.logger.Error("publish account update failed", zap.Error(err))
			}
		}
	}
}
