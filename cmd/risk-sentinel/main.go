// risk-sentinel — 最小闭环风控哨兵
//
// 数据来源优先级: MySQL > config.toml
//   - 账户列表: 优先从 MySQL accounts 表加载（active + 有 API key）
//   - 规则配置: 优先从 MySQL risk_rules 表加载（enabled=1），阈值从 config JSON 合并
//   - 基础设施配置（Redis/Telegram/Proxy）: 始终从 config.toml
//
// 用法:
//
//	go run ./cmd/risk-sentinel --config config.toml
//	go run ./cmd/risk-sentinel --config config.toml --dry-run
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/internal/sentinel"
)

func maskSecret(s string) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func shortFingerprint(s string) string {
	if s == "" {
		return "<empty>"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

func endpointByAccountType(cfg *sentinel.SentinelConfig, accountType string) string {
	if accountType == "portfolio_margin" {
		return cfg.Binance.PMRESTEndpoint()
	}
	return cfg.Binance.ActiveRESTEndpoint()
}

func main() {
	// 命令行参数
	configPath := flag.String("config", "config.toml", "配置文件路径")
	dryRun := flag.Bool("dry-run", false, "试运行模式：仅采集+评估，打印告警到 stdout，不发送 Telegram")
	noProxy := flag.Bool("no-proxy", false, "禁用 config.toml 中的 HTTP 代理，直接请求 Binance")
	debugSign := flag.Bool("debug-sign", false, "打印 Binance 签名请求调试信息（不泄露 secret）")
	checkTimeSkew := flag.Bool("check-time-skew", false, "先检查本地与 Binance 服务器时间偏差")
	selfCheckBinance := flag.Bool("self-check-binance", false, "仅对首个账户执行一次 Binance signed request 自检后退出")
	testTelegram := flag.Bool("test-telegram", false, "发送一条测试消息到 Telegram，验证 bot_token/chat_id/proxy 链路后退出")
	flag.Parse()

	// 初始化 logger
	var logger *zap.Logger
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 logger 失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 加载配置
	cfg, err := sentinel.LoadSentinelConfig(*configPath)
	if err != nil {
		logger.Fatal("加载配置失败", zap.Error(err))
	}
	if *noProxy {
		cfg.System.HTTPProxy = ""
	}
	if *debugSign {
		cfg.System.DebugSignRequests = true
	}

	logger.Info("配置加载完成",
		zap.String("env", cfg.System.Env),
		zap.String("proxy", cfg.System.HTTPProxy),
		zap.Bool("mysql_configured", cfg.Database.HasConfig()),
		zap.Bool("dry_run", *dryRun),
		zap.Bool("no_proxy", *noProxy),
		zap.Bool("debug_sign", cfg.System.DebugSignRequests),
		zap.Bool("check_time_skew", *checkTimeSkew),
		zap.Bool("self_check_binance", *selfCheckBinance),
		zap.Bool("test_telegram", *testTelegram))

	// =========================================================
	// --test-telegram：发送测试消息后退出
	// =========================================================
	if *testTelegram {
		logger.Info("Telegram 链路测试",
			zap.String("proxy", cfg.System.HTTPProxy),
			zap.Int("chat_ids", len(cfg.Telegram.ChatIDs)))

		alerter := sentinel.NewTelegramAlerter(
			cfg.Telegram.BotToken,
			cfg.Telegram.ChatIDs,
			nil, // 不需要 Redis
			0,   // 不需要冷却
			cfg.System.HTTPProxy,
			logger,
		)

		testAlert := sentinel.RiskAlert{
			RuleCode:     "TEST",
			RuleName:     "Telegram 链路测试",
			Level:        "L0",
			AccountID:    "test",
			AccountLabel: "测试账户",
			Timestamp:    time.Now(),
			Message:      "✅ Sentinel Telegram 告警链路正常\n\n此消息由 --test-telegram 触发，可忽略。",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		sent, err := alerter.Send(ctx, testAlert)
		if err != nil {
			logger.Fatal("Telegram 发送失败", zap.Error(err))
		}
		if sent {
			logger.Info("✅ Telegram 测试消息发送成功")
		} else {
			logger.Warn("Telegram 消息被冷却抑制（不应发生在测试模式）")
		}
		return
	}

	// =========================================================
	// MySQL 初始化：加载账户和规则
	// =========================================================
	var store *sentinel.SentinelStore
	accounts := cfg.Accounts     // TOML 回退
	rulesCfg := &cfg.Rules       // TOML 回退
	rules := sentinel.AllRules() // 代码内全部规则
	accountSource := "config.toml"

	if cfg.Database.HasConfig() {
		store, err = sentinel.NewSentinelStore(&cfg.Database, logger)
		if err != nil {
			logger.Warn("MySQL 连接失败，回退到 config.toml 账户配置", zap.Error(err))
		} else {
			defer store.Close()
			logger.Info("MySQL 连接成功",
				zap.String("host", cfg.Database.Host),
				zap.String("db", cfg.Database.Name))

			// 从 MySQL 加载账户
			dbAccounts, err := store.LoadAccounts()
			if err != nil {
				logger.Warn("MySQL 加载账户失败，回退到 config.toml", zap.Error(err))
			} else if len(dbAccounts) > 0 {
				accounts = dbAccounts
				accountSource = "mysql.accounts"
				logger.Info("从 MySQL 加载账户",
					zap.Int("count", len(accounts)))
			} else {
				logger.Warn("MySQL 无 active 账户，回退到 config.toml",
					zap.Int("toml_accounts", len(cfg.Accounts)))
			}

			// 从 MySQL 加载规则
			dbRuleRows, err := store.LoadEnabledRules()
			if err != nil {
				logger.Warn("MySQL 加载规则失败，使用默认规则配置", zap.Error(err))
			} else if len(dbRuleRows) > 0 {
				// 用 MySQL 规则阈值覆盖默认值
				rulesCfg = sentinel.BuildRulesConfig(dbRuleRows, &cfg.Rules)
				// 按 MySQL enabled 状态过滤规则
				rules = sentinel.FilterRulesByDB(sentinel.AllRules(), dbRuleRows)
				logger.Info("从 MySQL 加载规则配置",
					zap.Int("enabled_rules", len(rules)),
					zap.Int("db_rule_rows", len(dbRuleRows)))

				// 打印各规则的阈值参数（方便确认 MySQL 配置已生效）
				for _, r := range dbRuleRows {
					logger.Info("  规则配置",
						zap.String("rule_code", r.RuleCode),
						zap.String("name", r.Name),
						zap.Bool("enabled", r.Enabled),
						zap.Any("config", r.Config))
				}
			}

			// 从 sentinel_config 表加载全局配置（cooldown_ttl 等）
			globalCfg, err := store.LoadSentinelGlobalConfig()
			if err != nil {
				logger.Warn("MySQL 加载全局配置失败，使用 TOML 默认值", zap.Error(err))
			} else if len(globalCfg) > 0 {
				sentinel.ApplyGlobalConfig(rulesCfg, globalCfg)
				logger.Info("从 MySQL 加载全局配置",
					zap.Int("cooldown_ttl", rulesCfg.CooldownTTL),
					zap.Any("global_config", globalCfg))
			}
		}
	}

	// 检查是否有账户可监控
	if len(accounts) == 0 {
		logger.Fatal("无可监控账户（MySQL 和 config.toml 均为空）")
	}

	logger.Info("监控账户",
		zap.Int("count", len(accounts)),
		zap.String("symbol", cfg.Sentinel.Symbol))

	// 打印账户概览
	for _, a := range accounts {
		logger.Info("  账户",
			zap.String("id", a.ID),
			zap.String("label", a.Label),
			zap.String("type", a.AccountType),
			zap.String("exchange", a.Exchange),
			zap.String("source", accountSource),
			zap.String("rest_endpoint", endpointByAccountType(cfg, a.AccountType)),
			zap.String("api_key_masked", maskSecret(a.APIKey)),
			zap.Int("api_key_len", len(a.APIKey)),
			zap.String("api_key_fp", shortFingerprint(a.APIKey)),
			zap.String("secret_key_masked", maskSecret(a.SecretKey)),
			zap.Int("secret_key_len", len(a.SecretKey)),
			zap.String("secret_key_fp", shortFingerprint(a.SecretKey)))
	}

	if *selfCheckBinance || *checkTimeSkew {
		first := accounts[0]
		logger.Info("执行 Binance 诊断",
			zap.String("account_id", first.ID),
			zap.String("account_label", first.Label),
			zap.String("account_type", first.AccountType),
			zap.String("endpoint", endpointByAccountType(cfg, first.AccountType)),
		)
		client := sentinel.NewBinanceRESTClient(
			first.APIKey,
			first.SecretKey,
			cfg.Binance.ActiveRESTEndpoint(),
			cfg.Binance.PMRESTEndpoint(),
			first.AccountType,
			first.ID,
			cfg.System.HTTPProxy,
			cfg.System.DebugSignRequests,
			logger,
		)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if *checkTimeSkew || *selfCheckBinance {
			serverTime, localTime, skew, err := client.GetServerTime(ctx)
			if err != nil {
				logger.Error("Binance 时间偏差检查失败", zap.Error(err))
			} else {
				logger.Info("Binance 时间偏差检查",
					zap.Time("local_time", localTime),
					zap.Time("server_time", serverTime),
					zap.Int64("skew_ms", skew.Milliseconds()),
				)
				if skew > time.Second || skew < -time.Second {
					logger.Warn("本地与 Binance 服务器时间偏差较大，可能导致签名失败",
						zap.Int64("skew_ms", skew.Milliseconds()))
				}
			}
		}
		if !*selfCheckBinance {
			return
		}
		info, err := client.GetAccountInfo(ctx)
		if err != nil {
			logger.Fatal("Binance signed request 自检失败", zap.Error(err))
		}
		logger.Info("Binance signed request 自检成功",
			zap.String("account_id", info.AccountID),
			zap.String("account_type", info.AccountType),
			zap.Float64("margin_ratio", info.MarginRatio),
			zap.Float64("total_equity", info.TotalEquity),
		)
		return
	}

	// =========================================================
	// 基础设施初始化
	// =========================================================

	// 覆盖 cfg.Accounts 为最终账户列表（可能来自 MySQL）
	cfg.Accounts = accounts

	// 初始化 Redis
	var redisClient *redis.Client
	if cfg.Redis.Host != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr(),
			Username: cfg.Redis.User,
			Password: cfg.Redis.Pass,
			DB:       cfg.Redis.DB,
			PoolSize: cfg.Redis.PoolSize,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := redisClient.Ping(ctx).Err(); err != nil {
			logger.Warn("Redis 连接失败（告警冷却去重将不可用）", zap.Error(err))
			redisClient = nil
		} else {
			logger.Info("Redis 连接成功", zap.String("addr", cfg.Redis.Addr()))
		}
		cancel()
	}

	// 创建 Collector
	collector := sentinel.NewCollector(cfg, redisClient, logger)

	// 创建 Alerter（带 Proxy）
	cooldownTTL := time.Duration(rulesCfg.CooldownTTL) * time.Second
	alerter := sentinel.NewTelegramAlerter(
		cfg.Telegram.BotToken,
		cfg.Telegram.ChatIDs,
		redisClient,
		cooldownTTL,
		cfg.System.HTTPProxy,
		logger,
	)

	// 创建 Engine
	pollInterval := cfg.Sentinel.ParsePollInterval()
	engine := sentinel.NewEngine(
		collector,
		alerter,
		rules,
		rulesCfg,
		pollInterval,
		logger,
	)
	engine.SetDryRun(*dryRun)

	// 启用规则热重载（MySQL 可用时）
	if store != nil {
		reloadInterval := 60 * time.Second // 默认 60s
		if globalCfg, err := store.LoadSentinelGlobalConfig(); err == nil {
			if v, ok := globalCfg["rule_reload_interval"]; ok {
				if d, err := time.ParseDuration(v); err == nil {
					reloadInterval = d
				}
			}
		}
		engine.EnableHotReload(store, &cfg.Rules, reloadInterval)
	}

	// =========================================================
	// 启动
	// =========================================================
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("收到退出信号", zap.String("signal", sig.String()))
		cancel()
	}()

	logger.Info("Sentinel 启动",
		zap.Duration("poll_interval", pollInterval),
		zap.Int("rules", len(rules)),
		zap.Int("accounts", len(accounts)))

	engine.Start(ctx)

	logger.Info("Sentinel 已停止")
}
