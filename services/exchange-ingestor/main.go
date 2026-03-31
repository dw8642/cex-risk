// exchange-ingestor 服务入口
//
// 职责：从交易所采集实时数据（WS + REST），写入 Kafka 和 Redis
//
// 启动流程：
//   1. 加载 config.toml 配置
//   2. 初始化 MySQL + Redis + Kafka Producer
//   3. 从 MySQL 读取所有 active 状态的监控账户
//   4. 为每个币安合约账户创建 Adapter → Ingestor + Reconciler
//   5. 监听 SIGINT/SIGTERM 信号，优雅关闭所有组件
//
// 使用方式：
//   CONFIG_PATH=../../config.toml go run .
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/exchange"
	_ "github.com/cex-risk/cex-risk/pkg/exchange" // init() 触发适配器注册
	"github.com/cex-risk/cex-risk/pkg/logger"
	"github.com/cex-risk/cex-risk/pkg/mq"
	"github.com/cex-risk/cex-risk/pkg/store"
	"go.uber.org/zap"
)

func main() {
	// 初始化 logger
	log := logger.New(true) // debug mode
	defer log.Sync()

	// 加载配置
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatal("load config failed", zap.Error(err))
	}
	log.Info("config loaded", zap.String("env", cfg.System.Env))

	// 初始化存储（ClickHouse 可选）
	st, err := store.NewStore(cfg, log, true)
	if err != nil {
		log.Fatal("init store failed", zap.Error(err))
	}
	defer st.Close()

	// 初始化 Kafka Producer
	topics := []string{
		cfg.Kafka.Topics.TradeEvents,
		cfg.Kafka.Topics.PositionSnapshots,
		cfg.Kafka.Topics.BalanceSnapshots,
		cfg.Kafka.Topics.AccountUpdates,
		cfg.Kafka.Topics.PermissionChecks,
	}
	producer := mq.NewProducer(cfg.Kafka.Brokers, topics, log)
	defer producer.Close()

	// 从 MySQL 读取活跃账户
	accounts, err := st.MySQL.GetActiveAccounts(context.Background())
	if err != nil {
		log.Fatal("get active accounts failed", zap.Error(err))
	}
	if len(accounts) == 0 {
		log.Fatal("no active accounts found — please seed the database first")
	}
	log.Info("active accounts loaded", zap.Int("count", len(accounts)))

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 为每个币安合约账户创建适配器和采集器
	var ingestors []*Ingestor
	var reconcilers []*Reconciler

	for _, acc := range accounts {
		if acc.ExchangeID != "binance" || acc.MarketType != "futures" {
			log.Info("skipping non-binance-futures account", zap.String("id", acc.ID))
			continue
		}

		// 创建适配器
		adapter, err := exchange.Create(
			"binance_futures",
			cfg.Binance.APIKey,
			cfg.Binance.SecretKey,
			exchange.WithWSEndpoint(cfg.Binance.ActiveWSEndpoint()),
			exchange.WithRESTEndpoint(cfg.Binance.ActiveRESTEndpoint()),
			exchange.WithHTTPProxy(cfg.System.HTTPProxy),
			exchange.WithAccountID(acc.ID),
			exchange.WithDebug(cfg.System.Debug),
		)
		if err != nil {
			log.Error("create adapter failed", zap.String("account", acc.ID), zap.Error(err))
			continue
		}

		// 连接交易所
		if err := adapter.Connect(ctx); err != nil {
			log.Error("adapter connect failed", zap.String("account", acc.ID), zap.Error(err))
			continue
		}
		log.Info("adapter connected", zap.String("account", acc.ID), zap.String("exchange", acc.ExchangeID))

		// 创建采集器
		ing := NewIngestor(adapter, producer, cfg, log, acc.ID)
		ing.Start(ctx)
		ingestors = append(ingestors, ing)

		// 创建对账器
		rec := NewReconciler(adapter, producer, st.Redis, cfg, log, acc.ID)
		rec.Start(ctx)
		reconcilers = append(reconcilers, rec)
	}

	if len(ingestors) == 0 {
		log.Fatal("no ingestors started — check account configuration")
	}

	log.Info("exchange-ingestor started",
		zap.Int("ingestors", len(ingestors)),
		zap.Int("reconcilers", len(reconcilers)))

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("shutdown signal received", zap.String("signal", sig.String()))

	// 优雅关闭
	cancel()
	for _, ing := range ingestors {
		ing.Stop()
	}
	for _, rec := range reconcilers {
		rec.Stop()
	}

	log.Info("exchange-ingestor stopped")
}
