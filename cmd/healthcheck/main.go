package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/cex-risk/cex-risk/pkg/config"
	"github.com/cex-risk/cex-risk/pkg/logger"
	"github.com/cex-risk/cex-risk/pkg/store"
)

func main() {
	// 加载配置
	cfgPath := "config.toml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Config load failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Config loaded from %s (env=%s)\n", cfgPath, cfg.System.Env)

	log := logger.New(cfg.System.Debug)
	defer log.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	allOK := true

	// --- MySQL ---
	fmt.Print("\n📦 MySQL ... ")
	mysql, err := store.NewMySQL(&cfg.Database, log)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		allOK = false
	} else {
		defer mysql.Close()
		// 检查 cex_risk 库是否存在
		var dbName string
		err = mysql.DB().QueryRowContext(ctx, "SELECT DATABASE()").Scan(&dbName)
		if err != nil {
			fmt.Printf("❌ query failed: %v\n", err)
			allOK = false
		} else {
			fmt.Printf("✅ connected (database=%s)\n", dbName)

			// 检查关键表是否存在
			tables := []string{"projects", "accounts", "partners", "api_key_configs", "risk_rules", "risk_events", "trades_log", "audit_logs", "notification_logs"}
			missingTables := []string{}
			for _, t := range tables {
				var count int
				err := mysql.DB().QueryRowContext(ctx,
					"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
					cfg.Database.Name, t).Scan(&count)
				if err != nil || count == 0 {
					missingTables = append(missingTables, t)
				}
			}
			if len(missingTables) > 0 {
				fmt.Printf("   ⚠️  Missing tables: %s\n", strings.Join(missingTables, ", "))
				fmt.Printf("   👉 Run: mysql -u%s -p < scripts/init_mysql.sql\n", cfg.Database.User)
				allOK = false
			} else {
				fmt.Printf("   ✅ All %d tables exist\n", len(tables))

				// 检查种子数据
				var ruleCount int
				mysql.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM risk_rules").Scan(&ruleCount)
				var accCount int
				mysql.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts").Scan(&accCount)
				fmt.Printf("   📊 Rules: %d, Accounts: %d\n", ruleCount, accCount)
			}
		}
	}

	// --- Redis ---
	fmt.Print("\n📦 Redis ... ")
	rds, err := store.NewRedis(&cfg.Redis, log)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		allOK = false
	} else {
		defer rds.Close()
		info, _ := rds.Client().Info(ctx, "server").Result()
		// 提取版本号
		version := "unknown"
		for _, line := range strings.Split(info, "\n") {
			if strings.HasPrefix(line, "redis_version:") {
				version = strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
				break
			}
		}
		fmt.Printf("✅ connected (version=%s, db=%d)\n", version, cfg.Redis.DB)
	}

	// --- ClickHouse ---
	fmt.Print("\n📦 ClickHouse ... ")
	ch, err := store.NewClickHouse(&cfg.ClickHouse, log)
	if err != nil {
		fmt.Printf("⚠️  not available (optional): %v\n", err)
		fmt.Println("   ℹ️  演示版可用 MySQL 暂替，不影响功能")
	} else {
		defer ch.Close()
		// 检查 cex_risk 库是否存在
		var count uint64
		err = ch.Conn().QueryRow(ctx, "SELECT count() FROM system.databases WHERE name = ?", cfg.ClickHouse.Name).Scan(&count)
		if err != nil {
			fmt.Printf("✅ connected, but query failed: %v\n", err)
		} else if count == 0 {
			fmt.Printf("✅ connected, database '%s' not found\n", cfg.ClickHouse.Name)
			fmt.Println("   👉 Run: clickhouse-client < scripts/init_clickhouse.sql")
		} else {
			fmt.Printf("✅ connected (database=%s)\n", cfg.ClickHouse.Name)
		}
	}

	// --- Kafka ---
	fmt.Print("\n📦 Kafka ... ")
	if len(cfg.Kafka.Brokers) == 0 {
		fmt.Println("❌ no brokers configured")
		allOK = false
	} else {
		conn, err := kafka.DialContext(ctx, "tcp", cfg.Kafka.Brokers[0])
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			allOK = false
		} else {
			defer conn.Close()
			brokers, err := conn.Brokers()
			if err != nil {
				fmt.Printf("✅ connected, but can't list brokers: %v\n", err)
			} else {
				fmt.Printf("✅ connected (%d broker(s))\n", len(brokers))
				for _, b := range brokers {
					fmt.Printf("   📡 Broker %d: %s:%d\n", b.ID, b.Host, b.Port)
				}
			}

			// 列出风控相关 topics
			partitions, err := conn.ReadPartitions()
			if err == nil {
				riskTopics := []string{}
				for _, p := range partitions {
					if strings.HasPrefix(p.Topic, "risk_") && p.ID == 0 {
						riskTopics = append(riskTopics, p.Topic)
					}
				}
				if len(riskTopics) > 0 {
					fmt.Printf("   📋 Risk topics: %s\n", strings.Join(riskTopics, ", "))
				} else {
					fmt.Println("   ℹ️  No risk_* topics yet (will be auto-created)")
				}
			}
		}
	}

	// --- 汇总 ---
	fmt.Println("\n" + strings.Repeat("=", 50))
	if allOK {
		fmt.Println("✅ All infrastructure ready!")
	} else {
		fmt.Println("⚠️  Some issues found, please fix above")
		os.Exit(1)
	}
}
