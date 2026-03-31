package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/pkg/config"
)

// Redis 封装 Redis 连接和风控系统常用操作
type Redis struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedis 创建 Redis 连接
func NewRedis(cfg *config.RedisConfig, logger *zap.Logger) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Username: cfg.User,
		Password: cfg.Pass,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	logger.Info("Redis connected", zap.String("addr", cfg.Addr()), zap.Int("db", cfg.DB))
	return &Redis{client: client, logger: logger}, nil
}

// Client 返回底层 redis.Client
func (r *Redis) Client() *redis.Client {
	return r.client
}

// Close 关闭连接
func (r *Redis) Close() error {
	return r.client.Close()
}

// Ping 健康检查
func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// ---- Key 命名规范 ----
// permission.{accountID}.withdraw_enabled  → "true"/"false"
// permission.{accountID}.ip_restrict       → "true"/"false"
// permission.{accountID}.ip_list           → Set
// permission.{accountID}.last_check_time   → Unix timestamp
// trade.volume.{accountID}.{window}        → Sorted Set (member=tradeID, score=timestamp)
// trade.symbols.{accountID}.{window}       → Set of symbols
// position.{accountID}.{symbol}            → Hash (qty, entry_price, mark_price, pnl, ...)
// balance.{accountID}.{asset}              → Hash (wallet, available, margin, maint_margin, ...)
// metrics.{accountID}.last_activity        → Unix timestamp
// alert.dedup.{eventCode}.{objectID}       → TTL key for dedup

// ---- Permission 操作 ----

func permKey(accountID, field string) string {
	return fmt.Sprintf("permission.%s.%s", accountID, field)
}

// SetPermission 写入权限检查结果
func (r *Redis) SetPermission(ctx context.Context, accountID string, field string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, permKey(accountID, field), value, ttl).Err()
}

// GetPermission 读取权限字段
func (r *Redis) GetPermission(ctx context.Context, accountID string, field string) (string, error) {
	return r.client.Get(ctx, permKey(accountID, field)).Result()
}

// SetPermissionIPList 写入 IP 白名单
func (r *Redis) SetPermissionIPList(ctx context.Context, accountID string, ips []string, ttl time.Duration) error {
	key := permKey(accountID, "ip_list")
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	if len(ips) > 0 {
		members := make([]interface{}, len(ips))
		for i, ip := range ips {
			members[i] = ip
		}
		pipe.SAdd(ctx, key, members...)
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// GetPermissionIPList 获取 IP 白名单
func (r *Redis) GetPermissionIPList(ctx context.Context, accountID string) ([]string, error) {
	return r.client.SMembers(ctx, permKey(accountID, "ip_list")).Result()
}

// ---- Trade Metrics 操作 ----

// AddTradeToWindow 添加成交到滑动窗口（Sorted Set）
func (r *Redis) AddTradeToWindow(ctx context.Context, accountID string, window string, tradeID string, timestamp float64) error {
	key := fmt.Sprintf("trade.volume.%s.%s", accountID, window)
	return r.client.ZAdd(ctx, key, redis.Z{Score: timestamp, Member: tradeID}).Err()
}

// TrimTradeWindow 裁剪滑动窗口，移除超时数据
func (r *Redis) TrimTradeWindow(ctx context.Context, accountID string, window string, minTimestamp float64) error {
	key := fmt.Sprintf("trade.volume.%s.%s", accountID, window)
	return r.client.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%f", minTimestamp)).Err()
}

// GetTradeWindowCount 获取窗口内成交数
func (r *Redis) GetTradeWindowCount(ctx context.Context, accountID string, window string) (int64, error) {
	key := fmt.Sprintf("trade.volume.%s.%s", accountID, window)
	return r.client.ZCard(ctx, key).Result()
}

// AddTradedSymbol 记录交易过的币对
func (r *Redis) AddTradedSymbol(ctx context.Context, accountID string, window string, symbol string, ttl time.Duration) error {
	key := fmt.Sprintf("trade.symbols.%s.%s", accountID, window)
	pipe := r.client.Pipeline()
	pipe.SAdd(ctx, key, symbol)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// GetTradedSymbols 获取窗口内交易过的所有币对
func (r *Redis) GetTradedSymbols(ctx context.Context, accountID string, window string) ([]string, error) {
	key := fmt.Sprintf("trade.symbols.%s.%s", accountID, window)
	return r.client.SMembers(ctx, key).Result()
}

// ---- Position 操作 ----

func positionKey(accountID, symbol string) string {
	return fmt.Sprintf("position.%s.%s", accountID, symbol)
}

// SetPosition 写入仓位快照到 Hash
func (r *Redis) SetPosition(ctx context.Context, accountID, symbol string, fields map[string]interface{}, ttl time.Duration) error {
	key := positionKey(accountID, symbol)
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// GetPosition 读取仓位 Hash
func (r *Redis) GetPosition(ctx context.Context, accountID, symbol string) (map[string]string, error) {
	return r.client.HGetAll(ctx, positionKey(accountID, symbol)).Result()
}

// ---- Balance 操作 ----

func balanceKey(accountID, asset string) string {
	return fmt.Sprintf("balance.%s.%s", accountID, asset)
}

// SetBalance 写入余额快照到 Hash
func (r *Redis) SetBalance(ctx context.Context, accountID, asset string, fields map[string]interface{}, ttl time.Duration) error {
	key := balanceKey(accountID, asset)
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// GetBalance 读取余额 Hash
func (r *Redis) GetBalance(ctx context.Context, accountID, asset string) (map[string]string, error) {
	return r.client.HGetAll(ctx, balanceKey(accountID, asset)).Result()
}

// ---- Last Activity ----

// SetLastActivity 更新账户最后活跃时间
func (r *Redis) SetLastActivity(ctx context.Context, accountID string, t time.Time) error {
	key := fmt.Sprintf("metrics.%s.last_activity", accountID)
	return r.client.Set(ctx, key, t.Unix(), 24*time.Hour).Err()
}

// GetLastActivity 获取账户最后活跃时间
func (r *Redis) GetLastActivity(ctx context.Context, accountID string) (time.Time, error) {
	key := fmt.Sprintf("metrics.%s.last_activity", accountID)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return time.Time{}, err
	}
	ts, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0), nil
}

// ---- Alert Dedup ----

// CheckAndSetAlertDedup 告警去重：如果 key 已存在则返回 false（重复），否则设置并返回 true
func (r *Redis) CheckAndSetAlertDedup(ctx context.Context, eventCode, objectID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf("alert.dedup.%s.%s", eventCode, objectID)
	return r.client.SetNX(ctx, key, "1", ttl).Result()
}

// ClearAlertDedup 清除告警去重标记（状态恢复正常后清除）
func (r *Redis) ClearAlertDedup(ctx context.Context, eventCode, objectID string) error {
	key := fmt.Sprintf("alert.dedup.%s.%s", eventCode, objectID)
	return r.client.Del(ctx, key).Err()
}
