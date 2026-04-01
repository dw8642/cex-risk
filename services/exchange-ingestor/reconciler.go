// reconciler.go — REST 定时对账与权限检查
//
// Reconciler 通过 REST API 定时查询交易所数据，与 WS 实时流形成双重保障：
//   1. 仓位/余额对账: 定期拉取全量仓位和余额，写入 Kafka + Redis
//      解决 WS 可能丢消息或断线期间的数据缺失问题
//   2. 权限检查: 定期查询 API Key 权限状态，写入 Kafka + Redis
//      供 alert-engine 的 P-001（提币权限异常）规则实时判断
//
// 间隔配置（config.toml [risk_engine]）：
//   rest_reconcile_interval  = "30s"  — 仓位/余额对账间隔
//   permission_check_interval = "60s" — 权限检查间隔
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/exchange"
	"github.com/cex-risk/cex-risk/pkg/mq"
	"github.com/cex-risk/cex-risk/pkg/store"
	"go.uber.org/zap"
)

const permTTL = 5 * time.Minute // 权限数据 Redis TTL，略大于 permission_check_interval 防止间隙
const dataTTL = 2 * time.Minute // 仓位/余额数据 Redis TTL，略大于 rest_reconcile_interval

// ReconcilerRedis 对账器所需的 Redis 操作接口，便于测试 mock
type ReconcilerRedis interface {
	SetPosition(ctx context.Context, accountID, symbol string, fields map[string]interface{}, ttl time.Duration) error
	SetBalance(ctx context.Context, accountID, asset string, fields map[string]interface{}, ttl time.Duration) error
	SetLastActivity(ctx context.Context, accountID string, t time.Time) error
	SetPermission(ctx context.Context, accountID string, field string, value string, ttl time.Duration) error
	SetPermissionIPList(ctx context.Context, accountID string, ips []string, ttl time.Duration) error
}

// 编译期检查 *store.Redis 实现 ReconcilerRedis 接口
var _ ReconcilerRedis = (*store.Redis)(nil)

// Reconciler 定时 REST 对账 + 权限检查
type Reconciler struct {
	adapter   exchange.Adapter
	producer  Publisher
	redis     ReconcilerRedis
	cfg       *config.Config
	logger    *zap.Logger
	accountID string

	wg     sync.WaitGroup
	stopCh chan struct{}
}

// NewReconciler 创建对账器
func NewReconciler(adapter exchange.Adapter, producer *mq.Producer, redis *store.Redis, cfg *config.Config, logger *zap.Logger, accountID string) *Reconciler {
	return newReconciler(adapter, producer, redis, cfg, logger, accountID)
}

// newReconciler 内部构造器，接受接口类型（用于测试 mock）
func newReconciler(adapter exchange.Adapter, producer Publisher, redis ReconcilerRedis, cfg *config.Config, logger *zap.Logger, accountID string) *Reconciler {
	return &Reconciler{
		adapter:   adapter,
		producer:  producer,
		redis:     redis,
		cfg:       cfg,
		logger:    logger.With(zap.String("account", accountID), zap.String("component", "reconciler")),
		accountID: accountID,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动定时对账
func (r *Reconciler) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.reconcileLoop(ctx)

	r.wg.Add(1)
	go r.permissionCheckLoop(ctx)

	r.logger.Info("reconciler started",
		zap.String("reconcile_interval", r.cfg.RiskEngine.RESTReconcileInterval),
		zap.String("permission_interval", r.cfg.RiskEngine.PermissionCheckInterval))
}

// Stop 停止对账
func (r *Reconciler) Stop() {
	close(r.stopCh)
	r.wg.Wait()
	r.logger.Info("reconciler stopped")
}

// reconcileLoop 定时 REST 查询仓位和余额
func (r *Reconciler) reconcileLoop(ctx context.Context) {
	defer r.wg.Done()

	interval := r.cfg.RiskEngine.ParseRESTReconcileInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.doReconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.doReconcile(ctx)
		}
	}
}

// doReconcile 执行一次仓位/余额对账
func (r *Reconciler) doReconcile(ctx context.Context) {
	// 查询仓位
	positions, err := r.adapter.GetPositions(ctx)
	if err != nil {
		r.logger.Error("rest get positions failed", zap.Error(err))
	} else {
		topic := r.cfg.Kafka.Topics.PositionSnapshots
		for _, pos := range positions {
			data, err := pos.Marshal()
			if err != nil {
				continue
			}
			if err := r.producer.Publish(ctx, topic, []byte(pos.AccountID), data); err != nil {
				r.logger.Error("publish rest position failed", zap.Error(err))
			}
			// 写入 Redis
			fields := map[string]interface{}{
				"quantity":       fmt.Sprintf("%f", pos.Quantity),
				"entry_price":   fmt.Sprintf("%f", pos.EntryPrice),
				"mark_price":    fmt.Sprintf("%f", pos.MarkPrice),
				"unrealized_pnl": fmt.Sprintf("%f", pos.UnrealizedPnl),
				"leverage":      pos.Leverage,
				"margin_type":   pos.MarginType,
				"position_side": pos.PositionSide,
				"updated_at":    pos.SnapshotTime.Unix(),
			}
			if err := r.redis.SetPosition(ctx, pos.AccountID, pos.Symbol, fields, dataTTL); err != nil {
				r.logger.Error("redis set position failed", zap.Error(err))
			}
		}
		r.logger.Debug("reconcile positions done", zap.Int("count", len(positions)))
	}

	// 查询余额
	balances, err := r.adapter.GetBalances(ctx)
	if err != nil {
		r.logger.Error("rest get balances failed", zap.Error(err))
	} else {
		topic := r.cfg.Kafka.Topics.BalanceSnapshots
		for _, bal := range balances {
			data, err := bal.Marshal()
			if err != nil {
				continue
			}
			if err := r.producer.Publish(ctx, topic, []byte(bal.AccountID), data); err != nil {
				r.logger.Error("publish rest balance failed", zap.Error(err))
			}
			fields := map[string]interface{}{
				"wallet_balance":    fmt.Sprintf("%f", bal.WalletBalance),
				"available_balance": fmt.Sprintf("%f", bal.AvailableBalance),
				"unrealized_pnl":   fmt.Sprintf("%f", bal.UnrealizedPnl),
				"margin_balance":    fmt.Sprintf("%f", bal.MarginBalance),
				"maint_margin":      fmt.Sprintf("%f", bal.MaintMargin),
				"updated_at":        bal.SnapshotTime.Unix(),
			}
			if err := r.redis.SetBalance(ctx, bal.AccountID, bal.Asset, fields, dataTTL); err != nil {
				r.logger.Error("redis set balance failed", zap.Error(err))
			}
		}
		r.logger.Debug("reconcile balances done", zap.Int("count", len(balances)))
	}

	// 更新最后活跃时间
	if err := r.redis.SetLastActivity(ctx, r.accountID, time.Now()); err != nil {
		r.logger.Error("redis set last activity failed", zap.Error(err))
	}
}

// permissionCheckLoop 定时检查 API Key 权限
func (r *Reconciler) permissionCheckLoop(ctx context.Context) {
	defer r.wg.Done()

	interval := r.cfg.RiskEngine.ParsePermissionCheckInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.doPermissionCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.doPermissionCheck(ctx)
		}
	}
}

// doPermissionCheck 执行一次权限检查
func (r *Reconciler) doPermissionCheck(ctx context.Context) {
	perm, err := r.adapter.GetAPIKeyPermissions(ctx)
	if err != nil {
		r.logger.Error("rest get permissions failed", zap.Error(err))
		return
	}

	// 写入 Kafka
	topic := r.cfg.Kafka.Topics.PermissionChecks
	data, err := perm.Marshal()
	if err != nil {
		r.logger.Error("marshal permissions failed", zap.Error(err))
		return
	}
	if err := r.producer.Publish(ctx, topic, []byte(perm.AccountID), data); err != nil {
		r.logger.Error("publish permissions failed", zap.Error(err))
	}

	// 写入 Redis（SetPermission 接受 string value + TTL）
	boolStr := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	if err := r.redis.SetPermission(ctx, perm.AccountID, "withdraw_enabled", boolStr(perm.EnableWithdraw), permTTL); err != nil {
		r.logger.Error("redis set withdraw_enabled failed", zap.Error(err))
	}
	if err := r.redis.SetPermission(ctx, perm.AccountID, "spot_enabled", boolStr(perm.EnableSpot), permTTL); err != nil {
		r.logger.Error("redis set spot_enabled failed", zap.Error(err))
	}
	if err := r.redis.SetPermission(ctx, perm.AccountID, "futures_enabled", boolStr(perm.EnableFutures), permTTL); err != nil {
		r.logger.Error("redis set futures_enabled failed", zap.Error(err))
	}
	if err := r.redis.SetPermission(ctx, perm.AccountID, "internal_transfer_enabled", boolStr(perm.EnableInternalTransfer), permTTL); err != nil {
		r.logger.Error("redis set internal_transfer_enabled failed", zap.Error(err))
	}
	if err := r.redis.SetPermission(ctx, perm.AccountID, "ip_restrict", boolStr(perm.IPRestrict), permTTL); err != nil {
		r.logger.Error("redis set ip_restrict failed", zap.Error(err))
	}
	if len(perm.IPList) > 0 {
		if err := r.redis.SetPermissionIPList(ctx, perm.AccountID, perm.IPList, permTTL); err != nil {
			r.logger.Error("redis set ip_list failed", zap.Error(err))
		}
	}

	r.logger.Debug("permission check done",
		zap.Bool("withdraw", perm.EnableWithdraw),
		zap.Bool("futures", perm.EnableFutures),
		zap.Bool("spot", perm.EnableSpot),
		zap.Bool("ip_restrict", perm.IPRestrict))
}
