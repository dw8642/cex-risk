# ==============================================================
# CEX Risk Control System - Development Makefile
# ==============================================================

.PHONY: help infra infra-up infra-down infra-reset infra-logs infra-ps \
        db-init db-reset ch-cli mysql-cli redis-cli kafka-topics \
        build run test lint clean health

# 默认目标
help: ## 显示帮助信息
	@echo ""
	@echo "  CEX 风控系统 - 开发命令"
	@echo "  ======================="
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ---------------------------------------------------------------
# 基础设施（Docker Compose）
# ---------------------------------------------------------------
infra-up: ## 启动所有中间件（MySQL + Redis + Kafka + ClickHouse）
	docker compose up -d
	@echo "等待服务健康..."
	@sleep 5
	@$(MAKE) health

infra-down: ## 停止所有中间件
	docker compose down

infra-reset: ## 重置所有中间件（删除数据卷，重新初始化）
	docker compose down -v
	docker compose up -d
	@echo "等待服务重新初始化..."
	@sleep 10
	@$(MAKE) health

infra-logs: ## 查看中间件日志（可加 SVC=mysql 指定服务）
	docker compose logs -f $(SVC)

infra-ps: ## 查看中间件运行状态
	docker compose ps

# ---------------------------------------------------------------
# 数据库操作
# ---------------------------------------------------------------
db-init: ## 手动执行 MySQL 初始化脚本
	docker exec -i cex-risk-mysql mysql -uroot -p9Cn50al8W4F7dooaNIhv cex_risk < scripts/init_mysql.sql

db-reset: ## 重置 MySQL 数据（drop + recreate）
	docker exec -i cex-risk-mysql mysql -uroot -p9Cn50al8W4F7dooaNIhv -e "DROP DATABASE IF EXISTS cex_risk; CREATE DATABASE cex_risk CHARACTER SET utf8mb4;"
	docker exec -i cex-risk-mysql mysql -uroot -p9Cn50al8W4F7dooaNIhv cex_risk < scripts/init_mysql.sql
	@echo "MySQL 数据库已重置"

ch-init: ## 手动执行 ClickHouse 初始化脚本
	docker exec -i cex-risk-clickhouse clickhouse-client --multiquery < scripts/init_clickhouse.sql

mysql-cli: ## 进入 MySQL 命令行
	docker exec -it cex-risk-mysql mysql -uroot -p9Cn50al8W4F7dooaNIhv cex_risk

redis-cli: ## 进入 Redis 命令行
	docker exec -it cex-risk-redis redis-cli -n 2

ch-cli: ## 进入 ClickHouse 命令行
	docker exec -it cex-risk-clickhouse clickhouse-client -d cex_risk

kafka-topics: ## 列出所有 Kafka Topics
	docker exec cex-risk-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list

kafka-create-topics: ## 创建项目所需的 Kafka Topics
	@for topic in risk_trade_events risk_position_snapshots risk_balance_snapshots \
		risk_account_updates risk_risk_events risk_permission_checks; do \
		echo "Creating topic: $$topic"; \
		docker exec cex-risk-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
			--create --topic $$topic --partitions 3 --replication-factor 1 --if-not-exists; \
	done
	@echo "所有 Topics 创建完成"

# ---------------------------------------------------------------
# Go 构建与运行
# ---------------------------------------------------------------
build: ## 编译所有服务
	go build -o bin/exchange-ingestor ./services/exchange-ingestor/
	go build -o bin/healthcheck ./cmd/healthcheck/
	@echo "编译完成 -> bin/"

build-all: ## 编译所有服务（包括尚未实现的）
	@mkdir -p bin
	@for svc in exchange-ingestor; do \
		echo "Building $$svc..."; \
		go build -o bin/$$svc ./services/$$svc/ || exit 1; \
	done
	go build -o bin/healthcheck ./cmd/healthcheck/
	@echo "全部编译完成"

run-ingestor: ## 运行 Exchange Ingestor 服务
	go run ./services/exchange-ingestor/

run-health: ## 运行健康检查
	go run ./cmd/healthcheck/

# ---------------------------------------------------------------
# 测试
# ---------------------------------------------------------------
test: ## 运行所有测试
	go test ./... -v -count=1

test-unit: ## 仅运行单元测试（排除 integration tag）
	go test ./... -v -count=1 -short

test-integration: ## 运行集成测试（需要中间件运行中）
	CONFIG_PATH=$(PWD)/config.toml go test ./tests/ -v -count=1 -tags=integration

test-cover: ## 运行测试并生成覆盖率报告
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告: coverage.html"

# ---------------------------------------------------------------
# 代码质量
# ---------------------------------------------------------------
lint: ## 运行 golangci-lint
	golangci-lint run ./...

fmt: ## 格式化代码
	gofmt -w .
	goimports -w .

vet: ## 运行 go vet
	go vet ./...

# ---------------------------------------------------------------
# 健康检查
# ---------------------------------------------------------------
health: ## 检查所有中间件的连通性
	@echo "=== 中间件健康检查 ==="
	@echo -n "MySQL:      " && docker exec cex-risk-mysql mysqladmin ping -uroot -p9Cn50al8W4F7dooaNIhv 2>/dev/null | grep -q alive && echo "✓ OK" || echo "✗ FAIL"
	@echo -n "Redis:      " && docker exec cex-risk-redis redis-cli ping 2>/dev/null | grep -q PONG && echo "✓ OK" || echo "✗ FAIL"
	@echo -n "Kafka:      " && docker exec cex-risk-kafka /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 && echo "✓ OK" || echo "✗ FAIL"
	@echo -n "ClickHouse: " && docker exec cex-risk-clickhouse clickhouse-client --query "SELECT 1" >/dev/null 2>&1 && echo "✓ OK" || echo "✗ FAIL"
	@echo "======================"

# ---------------------------------------------------------------
# Orchestrator（多 Agent 调度）
# ---------------------------------------------------------------
orch-init: ## 初始化 Agent worktree（默认 A+B）
	@./orchestrator/orchestrator.sh init $(AGENTS)

orch-plan: ## 查看某天计划（用法: make orch-plan DAY=D1）
	@./orchestrator/orchestrator.sh plan $(DAY)

orch-run: ## 执行某天全部任务（用法: make orch-run DAY=D1）
	@./orchestrator/orchestrator.sh run $(DAY)

orch-run-task: ## 执行单个任务（用法: make orch-run-task TASK=D1-A1）
	@./orchestrator/orchestrator.sh run-task $(TASK)

orch-status: ## 查看所有 Agent 工作状态
	@./orchestrator/orchestrator.sh status

orch-merge: ## 合并某个 Agent 到 main（用法: make orch-merge AGENT=a）
	@./orchestrator/orchestrator.sh merge $(AGENT)

orch-sync: ## 同步 main 到所有 Agent worktree
	@./orchestrator/orchestrator.sh sync

orch-merge-all: ## 每日收工：合并所有 Agent（用法: make orch-merge-all DAY=D1）
	@./orchestrator/orchestrator.sh merge-all $(DAY)

orch-gate: ## 执行验收检查（用法: make orch-gate DAY=D1）
	@./orchestrator/orchestrator.sh gate $(DAY)

# ---------------------------------------------------------------
# 测试报告
# ---------------------------------------------------------------
test-report-unit: ## 运行单元测试并生成报告
	@./orchestrator/test-report.sh unit

test-report-integration: ## 运行集成测试并生成报告
	@./orchestrator/test-report.sh integration

test-report-benchmark: ## 运行压力测试并生成报告
	@./orchestrator/test-report.sh benchmark

test-report-all: ## 运行全部测试并生成报告
	@./orchestrator/test-report.sh all

# ---------------------------------------------------------------
# CI 辅助
# ---------------------------------------------------------------
push-all: ## 推送所有分支到 GitHub
	@./orchestrator/ci-helper.sh push-all

gen-docs: ## 从代码生成文档
	@./orchestrator/ci-helper.sh gen-docs

changelog: ## 生成 CHANGELOG
	@./orchestrator/ci-helper.sh changelog

pre-commit: ## 提交前检查（编译+测试+lint+格式化）
	@./orchestrator/ci-helper.sh pre-commit

# ---------------------------------------------------------------
# 清理
# ---------------------------------------------------------------
clean: ## 清理编译产物
	rm -rf bin/ coverage.out coverage.html
	go clean -cache -testcache
