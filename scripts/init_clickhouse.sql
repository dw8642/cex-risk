-- ============================================================
-- CEX 风控系统 - ClickHouse 初始化脚本
-- 数据库: cex_risk
-- ============================================================

CREATE DATABASE IF NOT EXISTS cex_risk;

-- -----------------------------------------------------------
-- 1. 成交明细表（主存储，按天分区）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS cex_risk.trades
(
    `exchange_id`   LowCardinality(String),
    `account_id`    String,
    `symbol`        LowCardinality(String),
    `side`          Enum8('BUY' = 1, 'SELL' = 2),
    `price`         Decimal(20, 8),
    `quantity`      Decimal(20, 8),
    `quote_qty`     Decimal(20, 8),
    `realized_pnl`  Decimal(20, 8)  DEFAULT 0,
    `commission`    Decimal(20, 8)  DEFAULT 0,
    `trade_id`      String,
    `order_id`      String          DEFAULT '',
    `trade_time`    DateTime64(3),
    `ingest_time`   DateTime64(3)   DEFAULT now64(3),
    `source`        Enum8('ws' = 1, 'rest' = 2)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(trade_time)
ORDER BY (account_id, symbol, trade_time)
TTL toDateTime(trade_time) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- -----------------------------------------------------------
-- 2. 仓位快照表（定时对账快照）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS cex_risk.position_snapshots
(
    `exchange_id`       LowCardinality(String),
    `account_id`        String,
    `symbol`            LowCardinality(String),
    `position_side`     Enum8('LONG' = 1, 'SHORT' = 2, 'BOTH' = 3),
    `quantity`          Decimal(20, 8),
    `entry_price`       Decimal(20, 8),
    `mark_price`        Decimal(20, 8),
    `unrealized_pnl`    Decimal(20, 8),
    `leverage`          UInt16,
    `margin_type`       Enum8('cross' = 1, 'isolated' = 2),
    `snapshot_time`     DateTime64(3),
    `source`            Enum8('ws' = 1, 'rest' = 2)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(snapshot_time)
ORDER BY (account_id, symbol, snapshot_time)
TTL toDateTime(snapshot_time) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- -----------------------------------------------------------
-- 3. 余额快照表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS cex_risk.balance_snapshots
(
    `exchange_id`       LowCardinality(String),
    `account_id`        String,
    `asset`             LowCardinality(String),
    `wallet_balance`    Decimal(20, 8),
    `available_balance` Decimal(20, 8),
    `unrealized_pnl`    Decimal(20, 8),
    `margin_balance`    Decimal(20, 8),
    `maint_margin`      Decimal(20, 8),
    `snapshot_time`     DateTime64(3),
    `source`            Enum8('ws' = 1, 'rest' = 2)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(snapshot_time)
ORDER BY (account_id, asset, snapshot_time)
TTL toDateTime(snapshot_time) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- -----------------------------------------------------------
-- 4. 指标历史表（用于趋势分析和回溯）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS cex_risk.metrics_history
(
    `metric_key`    String,
    `account_id`    String,
    `symbol`        LowCardinality(String) DEFAULT '',
    `value`         Float64,
    `timestamp`     DateTime64(3),
    `labels`        Map(String, String)    DEFAULT map()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (metric_key, account_id, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- -----------------------------------------------------------
-- 5. 风险事件归档表（从 MySQL 同步过来做分析）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS cex_risk.risk_events_archive
(
    `id`            String,
    `event_code`    LowCardinality(String),
    `level`         Enum8('P0' = 0, 'P1' = 1, 'P2' = 2, 'P3' = 3),
    `status`        LowCardinality(String),
    `object_type`   LowCardinality(String),
    `object_id`     String,
    `project_id`    String  DEFAULT '',
    `title`         String,
    `details`       String,
    `created_at`    DateTime64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(created_at)
ORDER BY (event_code, created_at)
SETTINGS index_granularity = 8192;
