# CEX 风控系统 - 本地开发环境配置指南

## 一、前置依赖安装

### 1.1 Go 1.22+

**macOS (Homebrew)**
```bash
brew install go
# 或指定版本
brew install go@1.22
```

**Linux (Ubuntu/Debian)**
```bash
# 下载安装
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz

# 添加到 PATH (~/.bashrc 或 ~/.zshrc)
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

**验证**
```bash
go version
# 期望输出: go version go1.22.x ...
```

### 1.2 Docker Desktop

**macOS**: https://docs.docker.com/desktop/install/mac-install/
**Linux**: https://docs.docker.com/engine/install/ubuntu/
**Windows (WSL2)**: https://docs.docker.com/desktop/install/windows-install/

```bash
# 验证
docker --version        # Docker version 24.x+
docker compose version  # Docker Compose version v2.x+
```

> 确保 Docker Desktop 至少分配 4GB 内存（ClickHouse 和 Kafka 需要），建议 6GB+。

### 1.3 开发工具（可选但推荐）

```bash
# Go 代码质量工具
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest

# Kafka 调试工具（可选）
brew install kcat    # macOS
# apt install kafkacat  # Linux
```

---

## 二、项目初始化

### 2.1 克隆并安装依赖

```bash
cd ~/projects  # 或你的项目目录
git clone <your-repo-url> cex-risk
cd cex-risk

# 下载 Go 依赖
go mod download
go mod verify
```

### 2.2 启动中间件

```bash
# 一键启动所有中间件（MySQL + Redis + Kafka + ClickHouse）
make infra-up

# 查看状态
make infra-ps

# 期望输出：4 个容器全部 healthy
# cex-risk-mysql       running (healthy)
# cex-risk-redis       running (healthy)
# cex-risk-kafka       running (healthy)
# cex-risk-clickhouse  running (healthy)
```

首次启动时 MySQL 和 ClickHouse 会自动执行 `scripts/init_mysql.sql` 和 `scripts/init_clickhouse.sql`，建表和灌入种子数据。

### 2.3 创建 Kafka Topics

```bash
make kafka-create-topics

# 验证
make kafka-topics
# 期望看到 6 个 topic:
# risk_trade_events
# risk_position_snapshots
# risk_balance_snapshots
# risk_account_updates
# risk_risk_events
# risk_permission_checks
```

> Kafka 设置了 `auto.create.topics.enable=true`，服务启动时也会自动创建，但提前手动创建可以确保分区数正确。

### 2.4 健康检查

```bash
make health

# 期望输出:
# === 中间件健康检查 ===
# MySQL:      ✓ OK
# Redis:      ✓ OK
# Kafka:      ✓ OK
# ClickHouse: ✓ OK
# ======================
```

---

## 三、配置文件

项目使用 `config.toml` 作为统一配置文件。默认配置已经指向本地 Docker 中间件地址，一般无需修改。

关键配置段说明：

| 配置段 | 用途 | 默认值 |
|-------|------|-------|
| `[database]` | MySQL 连接 | 127.0.0.1:3306 |
| `[clickhouse]` | ClickHouse 连接 | 127.0.0.1:9000 |
| `[redis]` | Redis 连接 | 127.0.0.1:6379, db=2 |
| `[kafka]` | Kafka Broker | 127.0.0.1:9092 |
| `[binance]` | 交易所 API | 线上 Key（开发可用 testnet） |
| `[system]` | 系统设置 | http_proxy 用于翻墙 |

### 开发阶段的注意事项

1. **Binance API**: 如果只做功能开发不需要实盘数据，建议把 `use_testnet = true` 开启，使用测试网
2. **HTTP 代理**: `http_proxy = "127.0.0.1:7897"` 根据你本地代理端口调整
3. **Telegram / Twilio**: 开发阶段可以留空，通知模块不影响核心功能

---

## 四、编译与运行

### 4.1 编译

```bash
# 编译已实现的服务
make build

# 输出:
# bin/exchange-ingestor
# bin/healthcheck
```

### 4.2 运行服务

```bash
# 方式一：直接 go run
make run-ingestor

# 方式二：运行编译好的二进制
./bin/exchange-ingestor

# 运行健康检查工具
make run-health
```

### 4.3 观察数据流

启动 exchange-ingestor 后可以验证数据流转：

```bash
# 1. 查看 Kafka 消息（需要 kcat）
kcat -b localhost:9092 -t risk_trade_events -C -o end

# 或者用 docker 内的 kafka 工具
docker exec cex-risk-kafka kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic risk_trade_events \
  --from-latest

# 2. 查看 ClickHouse 中的数据
make ch-cli
# 然后执行:
# SELECT count() FROM trades;
# SELECT * FROM trades ORDER BY trade_time DESC LIMIT 5;

# 3. 查看 Redis 缓存
make redis-cli
# 然后执行:
# KEYS *
# GET <key>

# 4. 查看 MySQL 中的配置和事件
make mysql-cli
# 然后执行:
# SELECT * FROM risk_rules WHERE is_enabled=1;
# SELECT * FROM risk_events ORDER BY created_at DESC LIMIT 10;
```

---

## 五、测试

```bash
# 运行全部测试
make test

# 仅单元测试（不需要中间件）
make test-unit

# 集成测试（需要中间件运行中）
make test-integration

# 生成覆盖率报告
make test-cover
# 浏览器打开 coverage.html 查看
```

---

## 六、日常开发流程

```
1. make infra-up          # 启动中间件（每天开工一次）
2. make health            # 确认中间件正常
3. 编写代码
4. make test              # 跑测试
5. make lint              # 代码检查
6. make run-ingestor      # 本地运行验证
7. make infra-down        # 收工停止中间件
```

### 重置环境

如果需要从头开始（清空所有数据）：
```bash
make infra-reset
make kafka-create-topics
```

如果只需要重置 MySQL 数据：
```bash
make db-reset
```

---

## 七、目录结构速览

```
cex-risk/
├── cmd/                         # 辅助命令入口
│   └── healthcheck/main.go
├── config.toml                  # 统一配置
├── docker-compose.yml           # 中间件编排
├── Makefile                     # 开发命令集
├── docs/                        # 文档
├── frontend/                    # 前端（待实现）
├── pkg/                         # 公共库
│   ├── config/                  #   配置加载
│   ├── exchange/                #   交易所适配器
│   ├── logger/                  #   日志
│   ├── models/                  #   数据模型
│   ├── mq/                      #   Kafka 生产/消费
│   └── store/                   #   存储层（MySQL+Redis+CH）
├── scripts/                     # 初始化脚本
│   ├── init_mysql.sql
│   └── init_clickhouse.sql
├── services/                    # 微服务
│   ├── exchange-ingestor/       #   数据采集（已实现）
│   ├── risk-engine/             #   风控引擎（待实现）
│   ├── api-server/              #   API 服务（待实现）
│   └── telegram-notifier/       #   通知服务（待实现）
└── tests/                       # 集成测试
```

---

## 八、常见问题

**Q: Docker 容器启动失败，端口被占用？**
检查并释放端口：
```bash
lsof -i :3306   # MySQL
lsof -i :6379   # Redis
lsof -i :9092   # Kafka
lsof -i :9000   # ClickHouse Native
lsof -i :8123   # ClickHouse HTTP
```

**Q: ClickHouse 初始化脚本没有执行？**
ClickHouse 只在首次创建容器时执行 `docker-entrypoint-initdb.d` 下的脚本。如果容器已存在但表没建，手动执行：
```bash
make ch-init
```

**Q: Kafka topic 分区数不对？**
先删后建：
```bash
docker exec cex-risk-kafka kafka-topics.sh --bootstrap-server localhost:9092 \
  --delete --topic risk_trade_events
make kafka-create-topics
```

> 如果你使用的是当前仓库内置的 Kafka 容器实现，容器内脚本路径为 `/opt/kafka/bin/`；对应 Makefile 已经处理，无需手动修改。

**Q: go mod download 很慢？**
设置 Go 代理：
```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

**Q: Binance WebSocket 连不上？**
确认代理设置正确（config.toml 中的 `http_proxy`），或在终端设置：
```bash
export HTTP_PROXY=http://127.0.0.1:7897
export HTTPS_PROXY=http://127.0.0.1:7897
```
