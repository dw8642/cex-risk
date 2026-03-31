# Day 1 开发记录 — 基础设施搭建 + 数据接入层

> 日期：2026-03-30
> 目标：搭建基础设施 + 实现币安合约数据接入，跑通「币安 WS → Kafka」链路

---

## 1. 交付物清单

| 文件 | 类型 | 说明 |
|------|------|------|
| `docker-compose.yml` | 基础设施 | MySQL 8.0 + Redis 7 + Kafka 3.6 (KRaft 模式) |
| `.env.example` | 配置模板 | 环境变量模板 |
| `pkg/exchange/binance_futures.go` | 核心代码 | 币安 USDT-M 合约适配器，实现 Adapter 接口 |
| `services/exchange-ingestor/main.go` | 服务入口 | 数据接入服务启动和编排 |
| `services/exchange-ingestor/ingestor.go` | 核心代码 | WS 实时数据→Kafka 采集器 |
| `services/exchange-ingestor/reconciler.go` | 核心代码 | REST 定时对账 + 权限检查 |
| `pkg/exchange/binance_futures_test.go` | 单元测试 | 12 个测试用例 |
| `tests/integration_test.go` | 集成测试 | MySQL/Redis/Kafka 连通性 + CRUD 验证 |

---

## 2. 基础设施 (docker-compose.yml)

### 容器编排

```
┌─────────────────────────────────────────┐
│  docker-compose.yml                     │
│                                         │
│  ┌─────────┐ ┌─────────┐ ┌──────────┐ │
│  │ MySQL   │ │ Redis   │ │ Kafka    │ │
│  │ 8.0     │ │ 7-alpine│ │ 3.6 KRaft│ │
│  │ :3306   │ │ :6379   │ │ :9092    │ │
│  └─────────┘ └─────────┘ └──────────┘ │
└─────────────────────────────────────────┘
```

### 设计决策

- **Kafka 使用 KRaft 模式**：无需 Zookeeper，减少容器数量，简化部署
- **服务暂不容器化**：开发阶段直接 `go run` 运行，方便调试，联调时再加入 compose
- **数据持久化**：三个服务均挂载 Docker volume，重启不丢数据

---

## 3. 币安合约适配器 (binance_futures.go)

### 架构设计

```
                          ┌──────────────────────────┐
                          │  BinanceFuturesAdapter    │
                          │                          │
  ┌────────────────┐      │  ┌──────────────────┐   │
  │ 币安 WS Server │ ◄────┼──│ readLoop()       │   │
  │ User Data      │      │  │ (goroutine)      │   │
  │ Stream         │      │  └──────┬───────────┘   │
  └────────────────┘      │         │                │
                          │         ▼                │
                          │  handleWSMessage()       │
                          │    ├── ORDER_TRADE_UPDATE │──► tradeCh (cap 1000)
                          │    ├── ACCOUNT_UPDATE     │──► positionCh (cap 500)
                          │    │                     │──► balanceCh (cap 200)
                          │    └── listenKeyExpired  │──► reconnect()
                          │                          │
  ┌────────────────┐      │  ┌──────────────────┐   │
  │ 币安 REST API  │ ◄────┼──│ signedGet()      │   │
  │ /fapi/v2/      │      │  │ (HMAC-SHA256)    │   │
  │ /sapi/v1/      │      │  └──────────────────┘   │
  └────────────────┘      │                          │
                          │  ┌──────────────────┐   │
                          │  │ keepAliveLoop()   │   │
                          │  │ (每30分钟续期)    │   │
                          │  └──────────────────┘   │
                          └──────────────────────────┘
```

### 实现的 Adapter 接口方法

| 方法 | 类型 | 说明 |
|------|------|------|
| `Connect()` | WS | 创建 listenKey → 连接 WS → 启动 readLoop + keepAliveLoop |
| `SubscribeTrades()` | WS | 返回成交事件 channel |
| `SubscribePositions()` | WS | 返回仓位变动 channel |
| `SubscribeBalances()` | WS | 返回余额变动 channel |
| `SubscribeAccountUpdates()` | WS | 返回原始账户更新 channel |
| `GetPositions()` | REST | 调用 `/fapi/v2/account` 查询仓位 |
| `GetBalances()` | REST | 调用 `/fapi/v2/account` 查询余额 |
| `GetAPIKeyPermissions()` | REST | 调用 `/sapi/v1/account/apiRestrictions` 查询权限 |
| `Close()` | - | 关闭 WS + 等待 goroutine 退出 + 关闭 channel |

### WS 事件处理

| 币安事件类型 | 处理方法 | 输出 |
|-------------|---------|------|
| `ORDER_TRADE_UPDATE` (x=TRADE) | `handleOrderTradeUpdate()` | → tradeCh |
| `ACCOUNT_UPDATE` | `handleAccountUpdate()` | → positionCh + balanceCh + accountCh |
| `listenKeyExpired` | 触发 `reconnect()` | 自动重连 |

### 重连策略

- 最多尝试 10 次
- 间隔递增：attempt * 2 秒
- 每次重连重新创建 listenKey 并建立新 WS 连接

### REST 签名

- 算法：HMAC-SHA256
- 参数：所有请求参数 + timestamp → 计算 signature
- Header：`X-MBX-APIKEY` 携带 API Key
- 支持 HTTP 代理（config.toml `[system] http_proxy`）

---

## 4. exchange-ingestor 服务

### 服务架构

```
┌─────────────────────────────────────────────────────┐
│  exchange-ingestor                                   │
│                                                      │
│  main.go (服务编排)                                   │
│    │                                                 │
│    ├── 加载 config.toml                              │
│    ├── 初始化 MySQL + Redis + Kafka Producer          │
│    ├── 查询 active 账户列表                           │
│    │                                                 │
│    ├── 为每个账户创建:                                 │
│    │   ├── BinanceFuturesAdapter  ──┐               │
│    │   ├── Ingestor (WS → Kafka)    ├── goroutines  │
│    │   └── Reconciler (REST定时)    ┘               │
│    │                                                 │
│    └── 监听 SIGINT/SIGTERM → 优雅关闭                 │
│                                                      │
│  数据流向:                                            │
│    WS 实时推送 ──► Ingestor ──► Kafka topics         │
│    REST 定时查询 ──► Reconciler ──► Kafka + Redis    │
└─────────────────────────────────────────────────────┘
```

### Ingestor — WS 实时数据采集

每个监控账户一个 Ingestor 实例，启动 4 个消费 goroutine：

| Channel | Kafka Topic | 说明 |
|---------|-------------|------|
| tradeCh | `risk_trade_events` | 实时成交 |
| positionCh | `risk_position_snapshots` | 仓位变动 |
| balanceCh | `risk_balance_snapshots` | 余额变动 |
| accountCh | `risk_account_updates` | 原始账户更新 |

### Reconciler — REST 定时对账

两个独立的定时循环：

| 循环 | 间隔 | 数据写入 | 用途 |
|------|------|---------|------|
| `reconcileLoop` | 30s | Kafka + Redis (仓位/余额 Hash) | WS 数据对账，防丢消息 |
| `permissionCheckLoop` | 60s | Kafka + Redis (权限 String) | P-001 规则实时判断依据 |

### Redis Key 设计

```
permission.{accountID}.withdraw_enabled   → "true"/"false"  (TTL 5m)
permission.{accountID}.spot_enabled       → "true"/"false"  (TTL 5m)
permission.{accountID}.futures_enabled    → "true"/"false"  (TTL 5m)
permission.{accountID}.ip_restrict        → "true"/"false"  (TTL 5m)
permission.{accountID}.ip_list            → Set             (TTL 5m)
position.{accountID}.{symbol}             → Hash            (TTL 2m)
balance.{accountID}.{asset}               → Hash            (TTL 2m)
metrics.{accountID}.last_activity         → Unix timestamp  (TTL 24h)
```

---

## 5. 测试覆盖

### 单元测试 (12 项，全部通过)

| 测试 | 覆盖内容 |
|------|---------|
| `TestParseFloat` | 字符串→浮点数解析，含边界值 |
| `TestParseInt` | 字符串→整数解析 |
| `TestSign` | HMAC-SHA256 签名正确性和确定性 |
| `TestNewBinanceFuturesAdapter` | 适配器创建 + 默认值 |
| `TestNewBinanceFuturesAdapterWithOptions` | 自定义选项生效 |
| `TestExchangeIDAndMarketType` | 标识方法返回值 |
| `TestAdapterRegistered` | init() 自动注册到全局 Registry |
| `TestHandleOrderTradeUpdate` | WS 成交事件解析 → tradeCh |
| `TestHandleOrderTradeUpdateNonTrade` | 非成交事件被过滤 |
| `TestHandleAccountUpdate` | WS 账户更新 → balanceCh + positionCh + accountCh |
| `TestParseAccountResponse` | REST 账户响应 JSON 解析 |
| `TestParseAPIRestrictions` | REST 权限响应 JSON 解析 |

运行命令：
```bash
go test ./pkg/exchange/ -v -count=1
```

### 集成测试 (需 Docker Compose)

| 测试 | 覆盖内容 |
|------|---------|
| `TestMySQL_Connection` | MySQL 连通性 |
| `TestMySQL_SeedData` | 种子数据验证（规则、账户） |
| `TestMySQL_RiskEventCRUD` | 风险事件增删查 |
| `TestMySQL_TradeLog` | 成交记录写入 |
| `TestRedis_Connection` | Redis 连通性 |
| `TestRedis_PermissionReadWrite` | 权限 + IP 白名单读写 |
| `TestRedis_PositionReadWrite` | 仓位 Hash 读写 |
| `TestRedis_AlertDedup` | 告警去重逻辑 |
| `TestKafka_ProduceAndConsume` | Kafka 消息发布+消费端到端 |
| `TestStore_FullInit` | Store 统一层初始化 + 健康检查 |

运行命令：
```bash
docker compose up -d
CONFIG_PATH=../config.toml go test -v -tags=integration -count=1 ./tests/
```

---

## 6. 新增依赖

| 包 | 版本 | 用途 |
|---|------|------|
| `github.com/gorilla/websocket` | v1.4.2 | 币安 WS 连接 |

---

## 7. 启动方式

```bash
# 1. 启动基础设施
cd cex-risk
docker compose up -d

# 2. 验证基础设施
cd cmd/healthcheck && go run main.go

# 3. 配置 API Key
# 编辑 config.toml [binance] 填入 api_key 和 secret_key

# 4. 启动数据接入
cd services/exchange-ingestor
CONFIG_PATH=../../config.toml go run .

# 5. 验证 Kafka 数据流
# 使用 kafka console consumer 查看 risk_trade_events topic
```

---

## 8. 已知限制和后续改进

| 项目 | 当前状态 | 后续计划 |
|------|---------|---------|
| 多账户 API Key | 所有账户共用同一个 API Key | 支持每账户独立 Key |
| WS 断线恢复 | 重连后可能丢失断线期间的事件 | 重连后通过 REST 补齐 |
| 错误监控 | 只有日志输出 | 接入 Prometheus metrics |
| 测试网支持 | config 支持但未实测 | 添加测试网集成测试 |
