-- ============================================================
-- Sentinel 哨兵模块 - MySQL 迁移脚本
-- 基于现有 cex_risk 库，扩展 accounts 表 + 完善 risk_rules 种子数据
-- ============================================================

USE `cex_risk`;

-- -----------------------------------------------------------
-- 1. 扩展 accounts 表：增加 API 凭据和账户类型
--    account_type: regular(经典合约) / portfolio_margin(统一账户)
--    api_key / secret_key: 哨兵直接使用的交易所凭据
--    兼容 MySQL 5.7+（不使用 IF NOT EXISTS）
-- -----------------------------------------------------------

-- 通过存储过程安全添加列（已存在则跳过）
DROP PROCEDURE IF EXISTS `_sentinel_migrate`;
DELIMITER $$
CREATE PROCEDURE `_sentinel_migrate`()
BEGIN
    -- account_type
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'accounts' AND COLUMN_NAME = 'account_type'
    ) THEN
        ALTER TABLE `accounts` ADD COLUMN `account_type` VARCHAR(32) NOT NULL DEFAULT 'regular'
            COMMENT '账户类型: regular(经典合约) / portfolio_margin(统一账户)' AFTER `market_type`;
    END IF;

    -- api_key
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'accounts' AND COLUMN_NAME = 'api_key'
    ) THEN
        ALTER TABLE `accounts` ADD COLUMN `api_key` VARCHAR(128) NOT NULL DEFAULT ''
            COMMENT 'API Key（哨兵直读）' AFTER `account_type`;
    END IF;

    -- secret_key
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'accounts' AND COLUMN_NAME = 'secret_key'
    ) THEN
        ALTER TABLE `accounts` ADD COLUMN `secret_key` VARCHAR(128) NOT NULL DEFAULT ''
            COMMENT 'Secret Key（哨兵直读）' AFTER `api_key`;
    END IF;

    -- symbol（监控交易对）
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'accounts' AND COLUMN_NAME = 'symbol'
    ) THEN
        ALTER TABLE `accounts` ADD COLUMN `symbol` VARCHAR(32) NOT NULL DEFAULT 'BTCUSDT'
            COMMENT '监控交易对，如 BTCUSDT' AFTER `secret_key`;
    END IF;
END$$
DELIMITER ;

CALL `_sentinel_migrate`();
DROP PROCEDURE IF EXISTS `_sentinel_migrate`;

-- -----------------------------------------------------------
-- 2. 更新 risk_rules 种子数据：覆盖 Sentinel P0 规则
--    config JSON 存储各规则的阈值参数，哨兵启动时读取
-- -----------------------------------------------------------

-- P-001: 提币权限异常（无阈值参数，纯布尔检测）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r001', 'P-001', '提币权限异常开启',
 '检测 API Key 提币权限是否被意外开启。做市 API Key 不应具备提币权限。',
 'permission', 'P0', 1,
 '{"alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- S-004: 风控失明/数据断流
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r011', 'S-004', '风控系统失明/数据断流',
 '数据采集失败导致风控系统对账户状态失去可见性。data_gap_threshold: 允许的最大数据断流秒数。',
 'system', 'P0', 1,
 '{"data_gap_threshold": 30, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- L-001: 保证金率过低
-- margin_ratio_threshold: 普通合约 maintMargin/marginBalance 阈值（0~1，越高越危险）
-- pm_unimmr_threshold: 统一账户 uniMMR 阈值（越小越危险，1.05为强平线）
-- pm_status_alert: 是否监控 accountStatus 异常
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r010', 'L-001', '维持保证金占比过高',
 '账户保证金率低于安全阈值（普通合约 maintMargin/marginBalance >= threshold; PM uniMMR <= threshold）',
 'liquidation', 'P0', 1,
 '{"margin_ratio_threshold": 0.5, "pm_unimmr_threshold": 1.5, "pm_status_alert": true, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- E-001: 净Delta变化率过高
-- delta_change_rate_threshold: 变化率阈值（0.3 = 30%）
-- delta_abs_threshold: 绝对变化兜底阈值（USD）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r009', 'E-001', '净Delta变化率异常',
 '账户净 Delta 暴露在单采集周期内变化超过阈值（双安全网：变化率 OR 绝对值）',
 'exposure', 'P0', 1,
 '{"delta_change_rate_threshold": 0.3, "delta_abs_threshold": 5000, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- E-005: 仓位变化速度异常
-- position_change_threshold: 单周期仓位名义价值累计变化阈值（USD）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r012', 'E-005', '仓位变动速率异常',
 '仓位名义价值在单采集周期内累计变化超过阈值',
 'exposure', 'P0', 1,
 '{"position_change_threshold": 50000, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- E-008c: 资金费率结算周期变化
-- expected_funding_interval: 预期结算间隔（毫秒），28800000 = 8小时
-- funding_interval_alert_cooldown: 告警冷却时间（秒），同一币种不重复告警
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r013', 'E-008c', '资金费率结算周期变更',
 '资金费率结算间隔偏离预期（如从8h变为4h/1h），可能影响持仓成本和套利策略',
 'exposure', 'P0', 1,
 '{"expected_funding_interval": 28800000, "funding_interval_alert_cooldown": 3600, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- L-002: 爆仓距离过近
-- liq_distance_l2_threshold: L2 预警距离百分比（0.05 = 5%）
-- liq_distance_l3_threshold: L3 紧急距离百分比（0.02 = 2%）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r014', 'L-002', '爆仓距离过近',
 '仓位标记价格与强平价格距离低于安全阈值，距离越小爆仓风险越高',
 'liquidation', 'P0', 1,
 '{"liq_distance_l2_threshold": 0.05, "liq_distance_l3_threshold": 0.02, "alert_level": "L2/L3"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- L-003: ADL风险升高
-- adl_quantile_l2_threshold: ADL 等级 L2 阈值（币安 1-5，4=高风险）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r015', 'L-003', 'ADL风险升高',
 '仓位 ADL 等级过高，存在被交易所自动减仓风险。币安 ADL 1-5 档，越高越危险。',
 'liquidation', 'P1', 1,
 '{"adl_quantile_l2_threshold": 4, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- E-008: 资金费率侵蚀
-- funding_rate_annualized_l2: 年化费率 L2 阈值（0.25 = 25%）
-- funding_rate_cooldown: 告警冷却秒数
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r016', 'E-008', '资金费率侵蚀',
 '合约仓位被资金费率持续侵蚀，年化 funding 成本超阈值。基于当前 lastFundingRate 估算。',
 'exposure', 'P2', 1,
 '{"funding_rate_annualized_l2": 0.25, "funding_rate_cooldown": 1800, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- S-014: 交易所接口健康恶化（原 M-002）
-- api_latency_threshold_ms: 平均延迟阈值（毫秒）
-- api_error_rate_threshold: 错误率阈值（0.1 = 10%）
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r017', 'S-014', '交易所接口健康恶化',
 '交易所 REST API 平均延迟或错误率超阈值，接口健康状况恶化。',
 'system', 'P2', 1,
 '{"api_latency_threshold_ms": 2000, "api_error_rate_threshold": 0.1, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- 扩展 category ENUM（新增 market 类别，支持 M-Class 规则）
ALTER TABLE `risk_rules` MODIFY COLUMN `category` ENUM('permission','behavior','system','exposure','liquidation','market') NOT NULL;

-- M-001: 波动率突升
-- price_jump_window_minutes: 观察窗口（分钟）
-- price_jump_threshold: 价格跳变百分比阈值（0.10 = 10%）
-- price_jump_cooldown: 告警冷却秒数
INSERT INTO `risk_rules` (`id`, `rule_code`, `name`, `description`, `category`, `priority`, `enabled`, `config`) VALUES
('r018', 'M-001', '波动率突升',
 '市场价格在观察窗口内跳变超阈值。MVP 第一版基于 markPrice 滑动窗口实现价格跳变检测。',
 'market', 'P2', 1,
 '{"price_jump_window_minutes": 60, "price_jump_threshold": 0.10, "price_jump_cooldown": 600, "alert_level": "L2"}')
ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `enabled` = VALUES(`enabled`),
    `config` = VALUES(`config`);

-- -----------------------------------------------------------
-- 3. 全局告警配置表（冷却时间等）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sentinel_config` (
    `key`         VARCHAR(64) NOT NULL PRIMARY KEY,
    `value`       VARCHAR(256) NOT NULL,
    `description` VARCHAR(256) DEFAULT '',
    `updated_at`  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Sentinel 全局配置（可选覆盖 config.toml）';

INSERT INTO `sentinel_config` (`key`, `value`, `description`) VALUES
('cooldown_ttl', '300', '告警冷却时间（秒），同一 rule+account 组合在此时间内不重复告警'),
('poll_interval', '10s', '采集轮询间隔，支持 Go duration 格式如 10s/1m'),
('symbol', 'BTCUSDT', '默认监控交易对'),
('rule_reload_interval', '60s', '规则热重载间隔，定期从 MySQL 重新加载规则配置')
ON DUPLICATE KEY UPDATE `value` = VALUES(`value`);

-- -----------------------------------------------------------
-- 4. Sentinel 账户种子数据（示例，需替换真实 key）
-- -----------------------------------------------------------
INSERT INTO `accounts` (`id`, `project_id`, `exchange_id`, `market_type`, `account_type`, `api_key`, `secret_key`, `symbol`, `label`, `status`) VALUES
('acc-01', 'proj-demo-001', 'binance', 'futures', 'portfolio_margin', 'YOUR_API_KEY_1', 'YOUR_SECRET_1', 'BTCUSDT', '主力1-PM', 'active'),
('acc-02', 'proj-demo-001', 'binance', 'futures', 'regular',          'YOUR_API_KEY_2', 'YOUR_SECRET_2', 'BTCUSDT', '策略2',    'active'),
('acc-03', 'proj-demo-001', 'binance', 'futures', 'portfolio_margin', 'YOUR_API_KEY_3', 'YOUR_SECRET_3', 'ETHUSDT', '做市3-PM', 'active'),
('acc-04', 'proj-demo-001', 'binance', 'futures', 'regular',          'YOUR_API_KEY_4', 'YOUR_SECRET_4', 'BTCUSDT', '套利4',    'active'),
('acc-05', 'proj-demo-001', 'binance', 'futures', 'portfolio_margin', 'YOUR_API_KEY_5', 'YOUR_SECRET_5', 'ETHUSDT', '备用5-PM', 'active')
ON DUPLICATE KEY UPDATE
    `account_type` = VALUES(`account_type`),
    `api_key` = VALUES(`api_key`),
    `secret_key` = VALUES(`secret_key`),
    `symbol` = VALUES(`symbol`),
    `label` = VALUES(`label`);
