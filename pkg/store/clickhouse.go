package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/models"
)

// ClickHouse 封装 ClickHouse 连接和写入操作
type ClickHouse struct {
	conn   driver.Conn
	logger *zap.Logger
}

// NewClickHouse 创建 ClickHouse 连接
func NewClickHouse(cfg *config.ClickHouseConfig, logger *zap.Logger) (*ClickHouse, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr()},
		Auth: clickhouse.Auth{
			Database: cfg.Name,
			Username: cfg.User,
			Password: cfg.Pass,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	logger.Info("ClickHouse connected", zap.String("addr", cfg.Addr()), zap.String("database", cfg.Name))
	return &ClickHouse{conn: conn, logger: logger}, nil
}

// Conn 返回底层连接
func (c *ClickHouse) Conn() driver.Conn {
	return c.conn
}

// Close 关闭连接
func (c *ClickHouse) Close() error {
	return c.conn.Close()
}

// Ping 健康检查
func (c *ClickHouse) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

// InsertTrade 写入成交到 ClickHouse
func (c *ClickHouse) InsertTrade(ctx context.Context, t *models.TradeEvent) error {
	return c.conn.Exec(ctx,
		`INSERT INTO trades (exchange_id, account_id, symbol, side, price, quantity, quote_qty, realized_pnl, commission, trade_id, order_id, trade_time, ingest_time, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ExchangeID, t.AccountID, t.Symbol, t.Side, t.Price, t.Quantity, t.QuoteQty, t.RealizedPnl, t.Commission, t.TradeID, t.OrderID, t.TradeTime, t.IngestTime, t.Source)
}

// InsertPositionSnapshot 写入仓位快照
func (c *ClickHouse) InsertPositionSnapshot(ctx context.Context, p *models.PositionSnapshot) error {
	return c.conn.Exec(ctx,
		`INSERT INTO position_snapshots (exchange_id, account_id, symbol, position_side, quantity, entry_price, mark_price, unrealized_pnl, leverage, margin_type, snapshot_time, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ExchangeID, p.AccountID, p.Symbol, p.PositionSide, p.Quantity, p.EntryPrice, p.MarkPrice, p.UnrealizedPnl, p.Leverage, p.MarginType, p.SnapshotTime, p.Source)
}

// InsertBalanceSnapshot 写入余额快照
func (c *ClickHouse) InsertBalanceSnapshot(ctx context.Context, b *models.BalanceSnapshot) error {
	return c.conn.Exec(ctx,
		`INSERT INTO balance_snapshots (exchange_id, account_id, asset, wallet_balance, available_balance, unrealized_pnl, margin_balance, maint_margin, snapshot_time, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ExchangeID, b.AccountID, b.Asset, b.WalletBalance, b.AvailableBalance, b.UnrealizedPnl, b.MarginBalance, b.MaintMargin, b.SnapshotTime, b.Source)
}

// BatchInsertTrades 批量写入成交（高吞吐场景）
func (c *ClickHouse) BatchInsertTrades(ctx context.Context, trades []models.TradeEvent) error {
	batch, err := c.conn.PrepareBatch(ctx,
		`INSERT INTO trades (exchange_id, account_id, symbol, side, price, quantity, quote_qty, realized_pnl, commission, trade_id, order_id, trade_time, ingest_time, source)`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, t := range trades {
		if err := batch.Append(t.ExchangeID, t.AccountID, t.Symbol, t.Side, t.Price, t.Quantity, t.QuoteQty, t.RealizedPnl, t.Commission, t.TradeID, t.OrderID, t.TradeTime, t.IngestTime, t.Source); err != nil {
			return fmt.Errorf("batch append: %w", err)
		}
	}

	return batch.Send()
}
