// integration_test.go — Day1 集成测试
//
// 测试前提：docker compose up -d 已启动 MySQL + Redis + Kafka
// 运行方式：CONFIG_PATH=../config.toml go test -v -tags=integration -count=1 ./tests/
//
// 测试覆盖：
//   1. MySQL 连通性 + 表结构验证 + 种子数据验证
//   2. Redis 连通性 + 权限/仓位/余额 读写验证
//   3. Kafka 连通性 + 消息发布/消费验证
//   4. Store 层 CRUD 操作验证

//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/mq"
	"github.com/cex-risk/cex-risk/pkg/store"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

var (
	testCfg    *config.Config
	testLogger *zap.Logger
)

func createKafkaTopic(ctx context.Context, broker, topic string) error {
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", controller.Host+":"+fmt.Sprint(controller.Port))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	return controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
}

func init() {
	testLogger, _ = zap.NewDevelopment()
	var err error
	testCfg, err = config.LoadFromEnv()
	if err != nil {
		panic("load config failed: " + err.Error())
	}
}

// ==================== MySQL 集成测试 ====================

func TestMySQL_Connection(t *testing.T) {
	mysql, err := store.NewMySQL(&testCfg.Database, testLogger)
	if err != nil {
		t.Fatalf("MySQL connection failed: %v", err)
	}
	defer mysql.Close()

	ctx := context.Background()
	if err := mysql.Ping(ctx); err != nil {
		t.Fatalf("MySQL ping failed: %v", err)
	}
}

func TestMySQL_SeedData(t *testing.T) {
	mysql, err := store.NewMySQL(&testCfg.Database, testLogger)
	if err != nil {
		t.Fatalf("MySQL connection failed: %v", err)
	}
	defer mysql.Close()

	ctx := context.Background()

	// 验证规则种子数据
	rules, err := mysql.GetAllRules(ctx)
	if err != nil {
		t.Fatalf("GetAllRules failed: %v", err)
	}
	if len(rules) < 10 {
		t.Errorf("expected >= 10 rules, got %d", len(rules))
	}

	// 验证启用的规则（P-001, P-002）
	enabled, err := mysql.GetEnabledRules(ctx)
	if err != nil {
		t.Fatalf("GetEnabledRules failed: %v", err)
	}
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled rules, got %d", len(enabled))
	}

	// 验证演示账户
	accounts, err := mysql.GetActiveAccounts(ctx)
	if err != nil {
		t.Fatalf("GetActiveAccounts failed: %v", err)
	}
	if len(accounts) < 1 {
		t.Errorf("expected >= 1 active account, got %d", len(accounts))
	}
}

func TestMySQL_RiskEventCRUD(t *testing.T) {
	mysql, err := store.NewMySQL(&testCfg.Database, testLogger)
	if err != nil {
		t.Fatalf("MySQL connection failed: %v", err)
	}
	defer mysql.Close()

	ctx := context.Background()
	eventID := uuid.New().String()

	// 插入风险事件
	event := &models.RiskEvent{
		ID:         eventID,
		EventCode:  "P-001",
		Level:      "P0",
		Status:     "open",
		ObjectType: "account",
		ObjectID:   "acc-demo-001",
		ProjectID:  "proj-demo-001",
		Title:      "集成测试-提币权限异常",
		Details:    map[string]interface{}{"test": true, "message": "integration test event"},
		CreatedAt:  time.Now(),
	}

	if err := mysql.InsertRiskEvent(ctx, event); err != nil {
		t.Fatalf("InsertRiskEvent failed: %v", err)
	}

	// 读取事件
	got, err := mysql.GetRiskEventByID(ctx, eventID)
	if err != nil {
		t.Fatalf("GetRiskEventByID failed: %v", err)
	}
	if got.EventCode != "P-001" {
		t.Errorf("EventCode = %q, want P-001", got.EventCode)
	}
	if got.Level != "P0" {
		t.Errorf("Level = %q, want P0", got.Level)
	}
	if got.Title != "集成测试-提币权限异常" {
		t.Errorf("Title = %q", got.Title)
	}

	// 清理测试数据
	_, _ = mysql.DB().ExecContext(ctx, "DELETE FROM risk_events WHERE id = ?", eventID)
}

func TestMySQL_TradeLog(t *testing.T) {
	mysql, err := store.NewMySQL(&testCfg.Database, testLogger)
	if err != nil {
		t.Fatalf("MySQL connection failed: %v", err)
	}
	defer mysql.Close()

	ctx := context.Background()
	trade := &models.TradeEvent{
		ExchangeID: "binance",
		AccountID:  "acc-demo-001",
		Symbol:     "BTCUSDT",
		Side:       "BUY",
		Price:      50000.0,
		Quantity:   0.01,
		QuoteQty:   500.0,
		TradeID:    "test-" + uuid.New().String()[:8],
		OrderID:    "test-order-001",
		TradeTime:  time.Now(),
		IngestTime: time.Now(),
		Source:     "ws",
	}

	if err := mysql.InsertTradeLog(ctx, trade); err != nil {
		t.Fatalf("InsertTradeLog failed: %v", err)
	}

	// 清理
	_, _ = mysql.DB().ExecContext(ctx, "DELETE FROM trades_log WHERE trade_id = ?", trade.TradeID)
}

// ==================== Redis 集成测试 ====================

func TestRedis_Connection(t *testing.T) {
	rds, err := store.NewRedis(&testCfg.Redis, testLogger)
	if err != nil {
		t.Fatalf("Redis connection failed: %v", err)
	}
	defer rds.Close()

	ctx := context.Background()
	if err := rds.Ping(ctx); err != nil {
		t.Fatalf("Redis ping failed: %v", err)
	}
}

func TestRedis_PermissionReadWrite(t *testing.T) {
	rds, err := store.NewRedis(&testCfg.Redis, testLogger)
	if err != nil {
		t.Fatalf("Redis connection failed: %v", err)
	}
	defer rds.Close()

	ctx := context.Background()
	accountID := "test-acc-" + uuid.New().String()[:8]
	ttl := 30 * time.Second

	// 写入权限
	if err := rds.SetPermission(ctx, accountID, "withdraw_enabled", "true", ttl); err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}

	// 读取权限
	val, err := rds.GetPermission(ctx, accountID, "withdraw_enabled")
	if err != nil {
		t.Fatalf("GetPermission failed: %v", err)
	}
	if val != "true" {
		t.Errorf("withdraw_enabled = %q, want true", val)
	}

	// IP 白名单
	ips := []string{"1.2.3.4", "5.6.7.8"}
	if err := rds.SetPermissionIPList(ctx, accountID, ips, ttl); err != nil {
		t.Fatalf("SetPermissionIPList failed: %v", err)
	}

	gotIPs, err := rds.GetPermissionIPList(ctx, accountID)
	if err != nil {
		t.Fatalf("GetPermissionIPList failed: %v", err)
	}
	if len(gotIPs) != 2 {
		t.Errorf("expected 2 IPs, got %d", len(gotIPs))
	}
}

func TestRedis_PositionReadWrite(t *testing.T) {
	rds, err := store.NewRedis(&testCfg.Redis, testLogger)
	if err != nil {
		t.Fatalf("Redis connection failed: %v", err)
	}
	defer rds.Close()

	ctx := context.Background()
	accountID := "test-acc-" + uuid.New().String()[:8]
	ttl := 30 * time.Second

	fields := map[string]interface{}{
		"quantity":    "0.5",
		"entry_price": "42000.00",
		"mark_price":  "43000.00",
		"leverage":    10,
	}
	if err := rds.SetPosition(ctx, accountID, "BTCUSDT", fields, ttl); err != nil {
		t.Fatalf("SetPosition failed: %v", err)
	}

	got, err := rds.GetPosition(ctx, accountID, "BTCUSDT")
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}
	if got["quantity"] != "0.5" {
		t.Errorf("quantity = %q, want 0.5", got["quantity"])
	}
	if got["entry_price"] != "42000.00" {
		t.Errorf("entry_price = %q", got["entry_price"])
	}
}

func TestRedis_AlertDedup(t *testing.T) {
	rds, err := store.NewRedis(&testCfg.Redis, testLogger)
	if err != nil {
		t.Fatalf("Redis connection failed: %v", err)
	}
	defer rds.Close()

	ctx := context.Background()
	ttl := 10 * time.Second

	// 首次应返回 true（非重复）
	isNew, err := rds.CheckAndSetAlertDedup(ctx, "P-001", "test-dedup-obj", ttl)
	if err != nil {
		t.Fatalf("CheckAndSetAlertDedup failed: %v", err)
	}
	if !isNew {
		t.Error("first call should return true (new)")
	}

	// 再次调用应返回 false（重复）
	isNew2, err := rds.CheckAndSetAlertDedup(ctx, "P-001", "test-dedup-obj", ttl)
	if err != nil {
		t.Fatalf("CheckAndSetAlertDedup second call failed: %v", err)
	}
	if isNew2 {
		t.Error("second call should return false (duplicate)")
	}

	// 清除后应该又可以
	if err := rds.ClearAlertDedup(ctx, "P-001", "test-dedup-obj"); err != nil {
		t.Fatalf("ClearAlertDedup failed: %v", err)
	}
	isNew3, _ := rds.CheckAndSetAlertDedup(ctx, "P-001", "test-dedup-obj", ttl)
	if !isNew3 {
		t.Error("after clear, should return true again")
	}
}

// ==================== Kafka 集成测试 ====================

func TestKafka_ProduceAndConsume(t *testing.T) {
	testTopic := "test_integration_" + uuid.New().String()[:8]
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := createKafkaTopic(ctx, testCfg.Kafka.Brokers[0], testTopic); err != nil {
		t.Fatalf("create Kafka topic failed: %v", err)
	}

	// 创建 Producer
	producer := mq.NewProducer(testCfg.Kafka.Brokers, []string{testTopic}, testLogger)
	defer producer.Close()

	// 发布消息
	trade := models.TradeEvent{
		ExchangeID: "binance",
		AccountID:  "acc-test",
		Symbol:     "BTCUSDT",
		Side:       "BUY",
		Price:      50000,
		Quantity:   0.01,
		TradeTime:  time.Now(),
		IngestTime: time.Now(),
		Source:     "test",
	}
	data, _ := json.Marshal(trade)

	if err := producer.Publish(ctx, testTopic, []byte("acc-test"), data); err != nil {
		t.Fatalf("Kafka publish failed: %v", err)
	}

	// 消费消息验证
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  testCfg.Kafka.Brokers,
		Topic:    testTopic,
		GroupID:  "test-group-" + uuid.New().String()[:8],
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  5 * time.Second,
	})
	defer reader.Close()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("Kafka read failed: %v", err)
	}

	var received models.TradeEvent
	if err := json.Unmarshal(msg.Value, &received); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if received.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %q, want BTCUSDT", received.Symbol)
	}
	if received.Price != 50000 {
		t.Errorf("Price = %f, want 50000", received.Price)
	}
}

// ==================== Store 统一层集成测试 ====================

func TestStore_FullInit(t *testing.T) {
	st, err := store.NewStore(testCfg, testLogger, true) // ClickHouse 可选
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer st.Close()

	// 健康检查
	ctx := context.Background()
	health := st.HealthCheck(ctx)

	if health["mysql"] != "ok" {
		t.Errorf("MySQL health = %q", health["mysql"])
	}
	if health["redis"] != "ok" {
		t.Errorf("Redis health = %q", health["redis"])
	}
}
