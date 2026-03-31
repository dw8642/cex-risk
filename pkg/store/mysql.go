package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/models"
)

// MySQL 封装 MySQL 连接和常用操作
type MySQL struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewMySQL 创建 MySQL 连接
func NewMySQL(cfg *config.DatabaseConfig, logger *zap.Logger) (*MySQL, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	db.SetMaxIdleConns(cfg.MaxIdleConn)
	db.SetMaxOpenConns(cfg.MaxOpenConn)
	db.SetConnMaxLifetime(cfg.MaxLifeTimeDuration())

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

	logger.Info("MySQL connected", zap.String("database", cfg.Name))
	return &MySQL{db: db, logger: logger}, nil
}

// DB 返回底层 sql.DB，用于需要直接操作的场景
func (m *MySQL) DB() *sql.DB {
	return m.db
}

// Close 关闭连接
func (m *MySQL) Close() error {
	return m.db.Close()
}

// Ping 健康检查
func (m *MySQL) Ping(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

// ---- Account 操作 ----

// GetActiveAccounts 获取所有活跃账户
func (m *MySQL) GetActiveAccounts(ctx context.Context) ([]models.Account, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT id, project_id, COALESCE(partner_id,''), exchange_id, market_type, label, status, created_at, updated_at FROM accounts WHERE status = 'active'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.PartnerID, &a.ExchangeID, &a.MarketType, &a.Label, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// GetAccountByID 根据 ID 获取账户
func (m *MySQL) GetAccountByID(ctx context.Context, id string) (*models.Account, error) {
	var a models.Account
	err := m.db.QueryRowContext(ctx,
		"SELECT id, project_id, COALESCE(partner_id,''), exchange_id, market_type, label, status, created_at, updated_at FROM accounts WHERE id = ?", id).
		Scan(&a.ID, &a.ProjectID, &a.PartnerID, &a.ExchangeID, &a.MarketType, &a.Label, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ---- API Key Config 操作 ----

// GetAPIKeyConfigsByAccount 获取账户的 API Key 预期配置
func (m *MySQL) GetAPIKeyConfigsByAccount(ctx context.Context, accountID string) ([]models.APIKeyConfig, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT id, account_id, key_label, key_hash, expected_permissions, expected_ip_whitelist, allowed_symbols, status, created_at, updated_at FROM api_key_configs WHERE account_id = ? AND status = 'active'",
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []models.APIKeyConfig
	for rows.Next() {
		var c models.APIKeyConfig
		var permJSON, ipJSON, symbolsJSON []byte
		if err := rows.Scan(&c.ID, &c.AccountID, &c.KeyLabel, &c.KeyHash, &permJSON, &ipJSON, &symbolsJSON, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if len(permJSON) > 0 {
			_ = json.Unmarshal(permJSON, &c.ExpectedPermissions)
		}
		if len(ipJSON) > 0 {
			_ = json.Unmarshal(ipJSON, &c.ExpectedIPWhitelist)
		}
		if len(symbolsJSON) > 0 {
			_ = json.Unmarshal(symbolsJSON, &c.AllowedSymbols)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// ---- Risk Event 操作 ----

// InsertRiskEvent 写入风险事件
func (m *MySQL) InsertRiskEvent(ctx context.Context, e *models.RiskEvent) error {
	detailsJSON, _ := json.Marshal(e.Details)
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO risk_events (id, event_code, level, status, object_type, object_id, project_id, title, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.EventCode, e.Level, e.Status, e.ObjectType, e.ObjectID, e.ProjectID, e.Title, detailsJSON, e.CreatedAt)
	return err
}

// GetRiskEvents 分页查询风险事件
func (m *MySQL) GetRiskEvents(ctx context.Context, limit, offset int, filters map[string]string) ([]models.RiskEvent, int, error) {
	where := "1=1"
	args := []interface{}{}
	if v, ok := filters["status"]; ok && v != "" {
		where += " AND status = ?"
		args = append(args, v)
	}
	if v, ok := filters["level"]; ok && v != "" {
		where += " AND level = ?"
		args = append(args, v)
	}
	if v, ok := filters["event_code"]; ok && v != "" {
		where += " AND event_code = ?"
		args = append(args, v)
	}

	// 总数
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM risk_events WHERE "+where, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 分页数据
	query := fmt.Sprintf("SELECT id, event_code, level, status, object_type, object_id, COALESCE(project_id,''), title, details, ack_by, ack_at, resolved_at, created_at FROM risk_events WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?", where)
	args = append(args, limit, offset)

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []models.RiskEvent
	for rows.Next() {
		var e models.RiskEvent
		var detailsJSON []byte
		var ackBy sql.NullString
		var ackAt, resolvedAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.EventCode, &e.Level, &e.Status, &e.ObjectType, &e.ObjectID, &e.ProjectID, &e.Title, &detailsJSON, &ackBy, &ackAt, &resolvedAt, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &e.Details)
		}
		if ackBy.Valid {
			e.AckBy = ackBy.String
		}
		if ackAt.Valid {
			t := ackAt.Time
			e.AckAt = &t
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time
			e.ResolvedAt = &t
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}

// GetRiskEventByID 根据 ID 获取事件详情
func (m *MySQL) GetRiskEventByID(ctx context.Context, id string) (*models.RiskEvent, error) {
	var e models.RiskEvent
	var detailsJSON []byte
	var ackBy sql.NullString
	var ackAt, resolvedAt sql.NullTime

	err := m.db.QueryRowContext(ctx,
		"SELECT id, event_code, level, status, object_type, object_id, COALESCE(project_id,''), title, details, ack_by, ack_at, resolved_at, created_at FROM risk_events WHERE id = ?", id).
		Scan(&e.ID, &e.EventCode, &e.Level, &e.Status, &e.ObjectType, &e.ObjectID, &e.ProjectID, &e.Title, &detailsJSON, &ackBy, &ackAt, &resolvedAt, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	if len(detailsJSON) > 0 {
		_ = json.Unmarshal(detailsJSON, &e.Details)
	}
	if ackBy.Valid {
		e.AckBy = ackBy.String
	}
	if ackAt.Valid {
		t := ackAt.Time
		e.AckAt = &t
	}
	if resolvedAt.Valid {
		t := resolvedAt.Time
		e.ResolvedAt = &t
	}
	return &e, nil
}

// CountOpenEvents 统计各级别活跃事件数
func (m *MySQL) CountOpenEvents(ctx context.Context) (map[string]int, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT level, COUNT(*) FROM risk_events WHERE status IN ('open','ack','handling') GROUP BY level")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]int{"P0": 0, "P1": 0, "P2": 0, "P3": 0}
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, err
		}
		result[level] = count
	}
	return result, rows.Err()
}

// CountTodayEvents 统计今日事件数
func (m *MySQL) CountTodayEvents(ctx context.Context) (int, error) {
	var count int
	err := m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM risk_events WHERE DATE(created_at) = CURDATE()").Scan(&count)
	return count, err
}

// ---- Risk Rules 操作 ----

// GetAllRules 获取所有规则
func (m *MySQL) GetAllRules(ctx context.Context) ([]models.RuleConfig, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT id, rule_code, name, COALESCE(description,''), category, priority, enabled, config, created_at, updated_at FROM risk_rules ORDER BY rule_code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.RuleConfig
	for rows.Next() {
		var r models.RuleConfig
		var configJSON []byte
		if err := rows.Scan(&r.ID, &r.RuleCode, &r.Name, &r.Description, &r.Category, &r.Priority, &r.Enabled, &configJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &r.Config)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// GetEnabledRules 获取启用的规则
func (m *MySQL) GetEnabledRules(ctx context.Context) ([]models.RuleConfig, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT id, rule_code, name, COALESCE(description,''), category, priority, enabled, config, created_at, updated_at FROM risk_rules WHERE enabled = 1 ORDER BY rule_code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.RuleConfig
	for rows.Next() {
		var r models.RuleConfig
		var configJSON []byte
		if err := rows.Scan(&r.ID, &r.RuleCode, &r.Name, &r.Description, &r.Category, &r.Priority, &r.Enabled, &configJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &r.Config)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// ---- Trade Log 操作 ----

// InsertTradeLog 写入成交记录（MySQL 暂存版）
func (m *MySQL) InsertTradeLog(ctx context.Context, t *models.TradeEvent) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO trades_log (exchange_id, account_id, symbol, side, price, quantity, quote_qty, realized_pnl, commission, trade_id, order_id, trade_time, ingest_time, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ExchangeID, t.AccountID, t.Symbol, t.Side, t.Price, t.Quantity, t.QuoteQty, t.RealizedPnl, t.Commission, t.TradeID, t.OrderID, t.TradeTime, t.IngestTime, t.Source)
	return err
}

// ---- Notification Log 操作 ----

// InsertNotificationLog 写入通知记录
func (m *MySQL) InsertNotificationLog(ctx context.Context, n *models.NotificationLog) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO notification_logs (id, event_id, channel, recipient, status, error_msg, sent_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.EventID, n.Channel, n.Recipient, n.Status, n.ErrorMsg, n.SentAt, n.CreatedAt)
	return err
}

// ---- Audit Log 操作 ----

// InsertAuditLog 写入审计日志（只增不改）
func (m *MySQL) InsertAuditLog(ctx context.Context, a *models.AuditLog) error {
	detailJSON, _ := json.Marshal(a.Detail)
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO audit_logs (timestamp, actor, action, resource, detail, ip)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.Timestamp, a.Actor, a.Action, a.Resource, detailJSON, a.IP)
	return err
}

// ---- Overview 统计 ----

// GetActiveAccountCount 获取活跃监控账户数
func (m *MySQL) GetActiveAccountCount(ctx context.Context) (int, error) {
	var count int
	err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE status = 'active'").Scan(&count)
	return count, err
}

// GetRuleHitCounts 获取各规则命中数
func (m *MySQL) GetRuleHitCounts(ctx context.Context) (map[string]int, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT event_code, COUNT(*) FROM risk_events GROUP BY event_code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var code string
		var count int
		if err := rows.Scan(&code, &count); err != nil {
			return nil, err
		}
		result[code] = count
	}
	return result, rows.Err()
}
