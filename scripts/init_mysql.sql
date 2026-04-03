-- ============================================================
-- CEX 风控系统 - MySQL 初始化脚本
-- 数据库: cex_risk
-- ============================================================

CREATE DATABASE IF NOT EXISTS `cex_risk` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `cex_risk`;

-- -----------------------------------------------------------
-- 1. 项目表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `projects` (
    `id`          VARCHAR(36) NOT NULL PRIMARY KEY,
    `name`        VARCHAR(128) NOT NULL,
    `description` TEXT,
    `status`      ENUM('active','paused','archived') NOT NULL DEFAULT 'active',
    `created_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='做市项目';

-- -----------------------------------------------------------
-- 2. 合作伙伴表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `partners` (
    `id`          VARCHAR(36) NOT NULL PRIMARY KEY,
    `name`        VARCHAR(128) NOT NULL,
    `contact`     VARCHAR(256) DEFAULT '',
    `risk_level`  ENUM('low','medium','high','critical') NOT NULL DEFAULT 'low',
    `status`      ENUM('active','suspended','terminated') NOT NULL DEFAULT 'active',
    `created_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='执行合作伙伴';

-- -----------------------------------------------------------
-- 3. 监控账户表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `accounts` (
    `id`            VARCHAR(36) NOT NULL PRIMARY KEY,
    `project_id`    VARCHAR(36) NOT NULL,
    `partner_id`    VARCHAR(36) DEFAULT NULL,
    `exchange_id`   VARCHAR(32) NOT NULL DEFAULT 'binance',
    `market_type`   ENUM('spot','futures','margin') NOT NULL DEFAULT 'futures',
    `account_type`  ENUM('regular','pm') NOT NULL DEFAULT 'regular' COMMENT '账户类型: regular=普通合约, pm=组合保证金',
    `api_key`       VARCHAR(128) NOT NULL DEFAULT '' COMMENT '交易所 API Key',
    `secret_key`    VARCHAR(128) NOT NULL DEFAULT '' COMMENT '交易所 Secret Key',
    `symbol`        VARCHAR(32) NOT NULL DEFAULT 'BTCUSDT' COMMENT '监控交易对，如 BTCUSDT',
    `label`         VARCHAR(128) NOT NULL DEFAULT '',
    `status`        ENUM('active','paused','disabled') NOT NULL DEFAULT 'active',
    `created_at`    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_project` (`project_id`),
    INDEX `idx_partner` (`partner_id`),
    INDEX `idx_exchange` (`exchange_id`, `market_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='交易所监控账户';

-- -----------------------------------------------------------
-- 4. API Key 配置表（不存明文 key，只存预期权限配置）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `api_key_configs` (
    `id`                VARCHAR(36) NOT NULL PRIMARY KEY,
    `account_id`        VARCHAR(36) NOT NULL,
    `key_label`         VARCHAR(128) NOT NULL DEFAULT '',
    `key_hash`          VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'API Key 的 SHA256 hash，用于标识',
    `expected_permissions` JSON NOT NULL COMMENT '预期权限配置，如 {"spot":false,"futures":true,"withdraw":false}',
    `expected_ip_whitelist` JSON DEFAULT NULL COMMENT '预期 IP 白名单',
    `allowed_symbols`   JSON DEFAULT NULL COMMENT '允许交易的币对列表',
    `status`            ENUM('active','revoked','expired') NOT NULL DEFAULT 'active',
    `created_at`        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_account` (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='API Key 预期配置（不存明文）';

-- -----------------------------------------------------------
-- 5. 风控规则表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `risk_rules` (
    `id`          VARCHAR(36) NOT NULL PRIMARY KEY,
    `rule_code`   VARCHAR(16) NOT NULL UNIQUE,
    `name`        VARCHAR(256) NOT NULL,
    `description` TEXT,
    `category`    ENUM('permission','behavior','system','exposure','liquidation','market') NOT NULL,
    `priority`    ENUM('P0','P1','P2','P3') NOT NULL DEFAULT 'P0',
    `enabled`     TINYINT(1) NOT NULL DEFAULT 1,
    `config`      JSON DEFAULT NULL COMMENT '规则参数配置',
    `created_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='风控规则定义';

-- -----------------------------------------------------------
-- 6. 风险事件表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `risk_events` (
    `id`          VARCHAR(36) NOT NULL PRIMARY KEY,
    `event_code`  VARCHAR(16) NOT NULL,
    `level`       ENUM('P0','P1','P2','P3') NOT NULL,
    `status`      ENUM('open','ack','handling','resolved','false_positive') NOT NULL DEFAULT 'open',
    `object_type` VARCHAR(32) NOT NULL COMMENT '关联对象类型: account/partner/system',
    `object_id`   VARCHAR(36) NOT NULL COMMENT '关联对象 ID',
    `project_id`  VARCHAR(36) DEFAULT NULL,
    `title`       VARCHAR(256) NOT NULL,
    `details`     JSON DEFAULT NULL,
    `ack_by`      VARCHAR(64) DEFAULT NULL COMMENT '确认人',
    `ack_at`      TIMESTAMP NULL DEFAULT NULL,
    `resolved_at` TIMESTAMP NULL DEFAULT NULL,
    `created_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_status` (`status`),
    INDEX `idx_level` (`level`),
    INDEX `idx_object` (`object_type`, `object_id`),
    INDEX `idx_event_code` (`event_code`),
    INDEX `idx_created` (`created_at` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='风险事件记录';

-- -----------------------------------------------------------
-- 7. 通知记录表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `notification_logs` (
    `id`          VARCHAR(36) NOT NULL PRIMARY KEY,
    `event_id`    VARCHAR(36) NOT NULL,
    `channel`     ENUM('telegram_msg','telegram_voice','twilio','email') NOT NULL,
    `recipient`   VARCHAR(128) NOT NULL,
    `status`      ENUM('pending','sent','delivered','failed') NOT NULL DEFAULT 'pending',
    `error_msg`   TEXT DEFAULT NULL,
    `sent_at`     TIMESTAMP NULL DEFAULT NULL,
    `created_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_event` (`event_id`),
    INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='通知发送记录';

-- -----------------------------------------------------------
-- 8. 成交明细表（演示版用 MySQL 暂替 ClickHouse）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `trades_log` (
    `id`          BIGINT AUTO_INCREMENT PRIMARY KEY,
    `exchange_id` VARCHAR(32) NOT NULL,
    `account_id`  VARCHAR(36) NOT NULL,
    `symbol`      VARCHAR(32) NOT NULL,
    `side`        ENUM('BUY','SELL') NOT NULL,
    `price`       DECIMAL(20,8) NOT NULL,
    `quantity`    DECIMAL(20,8) NOT NULL,
    `quote_qty`   DECIMAL(20,8) NOT NULL DEFAULT 0,
    `realized_pnl` DECIMAL(20,8) DEFAULT 0,
    `commission`  DECIMAL(20,8) DEFAULT 0,
    `trade_id`    VARCHAR(64) DEFAULT NULL COMMENT '交易所原始 trade ID',
    `order_id`    VARCHAR(64) DEFAULT NULL,
    `trade_time`  TIMESTAMP(3) NOT NULL,
    `ingest_time` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `source`      ENUM('ws','rest') NOT NULL DEFAULT 'ws',
    INDEX `idx_account_time` (`account_id`, `trade_time`),
    INDEX `idx_symbol_time` (`symbol`, `trade_time`),
    INDEX `idx_trade_id` (`exchange_id`, `trade_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='成交明细（演示版）';

-- -----------------------------------------------------------
-- 9. 审计日志表（不可变）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `audit_logs` (
    `id`          BIGINT AUTO_INCREMENT PRIMARY KEY,
    `timestamp`   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `actor`       VARCHAR(64) NOT NULL COMMENT '操作者: system/user/api',
    `action`      VARCHAR(64) NOT NULL COMMENT '操作类型',
    `resource`    VARCHAR(128) NOT NULL COMMENT '操作对象',
    `detail`      JSON DEFAULT NULL,
    `ip`          VARCHAR(45) DEFAULT NULL,
    INDEX `idx_timestamp` (`timestamp`),
    INDEX `idx_actor` (`actor`),
    INDEX `idx_resource` (`resource`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='不可变审计日志';

-- ============================================================
-- 种子数据
-- ============================================================

-- 风控规则定义
-- ① 非 Sentinel 规则（暂未实现，enabled=0 占位）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r002', 'P-002', '非授权交易范围开放',  '检测账户在非授权交易对上发生了交易活动',                    'permission',  'P0', 0, '{"check_interval":"10s"}'),
('r003', 'B-001', '非授权交易',          '检测到未经授权的下单行为',                                 'behavior',    'P0', 0, '{}'),
('r004', 'B-003', '高成交低净仓变化',    '成交量与净仓位变化严重不匹配，疑似对敲',                    'behavior',    'P0', 0, '{}'),
('r005', 'B-004', '异常滑点',            '实际成交价格与市价偏差超过阈值',                           'behavior',    'P0', 0, '{"max_slippage_bps": 50}'),
('r006', 'B-007', '对敲/刷量嫌疑',       '短时间内出现方向相反、价格接近的对手盘成交',               'behavior',    'P0', 0, '{"window":"5m","score_threshold":70}'),
('r007', 'S-001', '策略心跳超时',        '策略进程心跳超时未上报',                                   'system',      'P0', 0, '{"timeout":"60s"}'),
('r008', 'S-007', '数据真相源不一致',    'WebSocket 实时数据与 REST 对账数据出现不一致',              'system',      'P0', 0, '{"tolerance_pct": 1}')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`), `config`=VALUES(`config`);

-- ② Sentinel 已实现规则（enabled=1）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
-- P-001: 提币权限异常（纯布尔检测，无阈值参数）
('r001', 'P-001', '提币权限异常开启',
 '检测 API Key 提币权限是否被意外开启。做市 API Key 不应具备提币权限。',
 'permission', 'P0', 1,
 '{"evaluation_interval": "60s", "alert_level": "L2"}'),

-- S-004: 风控失明/数据断流
('r011', 'S-004', '风控系统失明/数据断流',
 '数据采集失败导致风控系统对账户状态失去可见性。',
 'system', 'P0', 1,
 '{"evaluation_interval": "10s", "data_gap_threshold": 30, "alert_level": "L2"}'),

-- S-014: 交易所接口健康恶化（原 M-002）
('r017', 'S-014', '交易所接口健康恶化',
 '交易所 REST API 平均延迟或错误率超阈值。',
 'system', 'P2', 1,
 '{"evaluation_interval": "10s", "api_latency_threshold_ms": 2000, "api_error_rate_threshold": 0.1, "alert_level": "L2"}'),

-- L-001: 维持保证金占比过高
('r010', 'L-001', '维持保证金占比过高',
 '账户保证金率低于安全阈值（普通合约 maintMargin/marginBalance >= threshold; PM uniMMR <= threshold）',
 'liquidation', 'P0', 1,
 '{"evaluation_interval": "10s", "margin_ratio_threshold": 0.5, "pm_unimmr_threshold": 1.5, "pm_status_alert": true, "alert_level": "L2"}'),

-- L-002: 爆仓距离过近
('r014', 'L-002', '爆仓距离过近',
 '仓位标记价格与强平价格距离低于安全阈值。',
 'liquidation', 'P0', 1,
 '{"evaluation_interval": "10s", "liq_distance_l2_threshold": 0.05, "liq_distance_l3_threshold": 0.02, "alert_level": "L2/L3"}'),

-- L-003: ADL 风险升高
('r015', 'L-003', 'ADL风险升高',
 '仓位 ADL 等级过高，存在被交易所自动减仓风险。币安 ADL 1-5 档。',
 'liquidation', 'P1', 1,
 '{"evaluation_interval": "10s", "adl_quantile_l2_threshold": 4, "alert_level": "L2"}'),

-- E-001: 净 Delta 变化率异常
('r009', 'E-001', '净Delta变化率异常',
 '账户净 Delta 暴露在单采集周期内变化超过阈值（双安全网：变化率 OR 绝对值）',
 'exposure', 'P0', 1,
 '{"evaluation_interval": "30s", "delta_change_rate_threshold": 0.3, "delta_abs_threshold": 5000, "alert_level": "L2"}'),

-- E-005: 仓位变动速率异常
('r012', 'E-005', '仓位变动速率异常',
 '仓位名义价值在单采集周期内累计变化超过阈值',
 'exposure', 'P0', 1,
 '{"evaluation_interval": "30s", "position_change_threshold": 50000, "alert_level": "L2"}'),

-- E-008: 资金费率侵蚀
('r016', 'E-008', '资金费率侵蚀',
 '合约仓位被资金费率持续侵蚀，年化 funding 成本超阈值。',
 'exposure', 'P2', 1,
 '{"evaluation_interval": "5m", "funding_rate_annualized_l2": 0.25, "funding_rate_cooldown": 1800, "alert_level": "L2"}'),

-- E-008c: 资金费率结算周期变更
('r013', 'E-008c', '资金费率结算周期变更',
 '资金费率结算间隔偏离预期（如从8h变为4h/1h）',
 'exposure', 'P0', 1,
 '{"evaluation_interval": "5m", "expected_funding_interval": 28800000, "funding_interval_alert_cooldown": 3600, "alert_level": "L2"}'),

-- M-001: 波动率突升
('r018', 'M-001', '波动率突升',
 '市场价格在观察窗口内跳变超阈值。基于 markPrice 滑动窗口检测。',
 'market', 'P2', 1,
 '{"evaluation_interval": "60s", "price_jump_window_minutes": 60, "price_jump_threshold": 0.10, "price_jump_cooldown": 600, "alert_level": "L2"}')

ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `category` = VALUES(`category`),
    `priority` = VALUES(`priority`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- 演示项目
INSERT INTO `projects` (`id`, `name`, `description`, `status`) VALUES
('proj-demo-001', '演示项目Alpha', 'CEX做市风控演示项目', 'active')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

-- 演示合作伙伴
INSERT INTO `partners` (`id`, `name`, `contact`, `risk_level`, `status`) VALUES
('partner-demo-001', 'DemoPartner-A', 'partner-a@example.com', 'low', 'active')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

-- 演示账户（需要填入真实的 API Key / Secret Key）
INSERT INTO `accounts` (`id`, `project_id`, `partner_id`, `exchange_id`, `market_type`, `account_type`, `api_key`, `secret_key`, `symbol`, `label`, `status`) VALUES
('acc-demo-001', 'proj-demo-001', 'partner-demo-001', 'binance', 'futures', 'regular', '', '', 'BTCUSDT', '演示合约账户-01', 'active')
ON DUPLICATE KEY UPDATE `label`=VALUES(`label`), `account_type`=VALUES(`account_type`), `symbol`=VALUES(`symbol`);

-- 演示 API Key 配置（预期权限：只开合约交易，不开提币和现货）
INSERT INTO `api_key_configs` (`id`, `account_id`, `key_label`, `key_hash`, `expected_permissions`, `expected_ip_whitelist`, `allowed_symbols`, `status`) VALUES
('key-demo-001', 'acc-demo-001', 'Demo-Futures-ReadTrade', '',
 '{"spot": false, "futures": true, "withdraw": false, "internal_transfer": false}',
 '["YOUR_SERVER_IP"]',
 '["BTCUSDT", "ETHUSDT"]',
 'active')
ON DUPLICATE KEY UPDATE `key_label`=VALUES(`key_label`);
