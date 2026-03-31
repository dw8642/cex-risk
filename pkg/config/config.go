package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config 风控系统全局配置，兼容现有 toml 格式
type Config struct {
	System     SystemConfig     `toml:"system"`
	Database   DatabaseConfig   `toml:"database"`
	ClickHouse ClickHouseConfig `toml:"clickhouse"`
	Redis      RedisConfig      `toml:"redis"`
	Kafka      KafkaConfig      `toml:"kafka"`
	Binance    BinanceConfig    `toml:"binance"`
	Telegram   TelegramConfig   `toml:"telegram"`
	Twilio     TwilioConfig     `toml:"twilio"`
	RiskEngine RiskEngineConfig `toml:"risk_engine"`
}

type SystemConfig struct {
	Debug     bool   `toml:"debug"`
	Env       string `toml:"env"`
	HTTPPort  string `toml:"http_port"`
	HTTPProxy string `toml:"http_proxy"`
}

type DatabaseConfig struct {
	Host        string `toml:"host"`
	Port        string `toml:"port"`
	User        string `toml:"user"`
	Pass        string `toml:"pass"`
	Name        string `toml:"name"`
	Charset     string `toml:"charset"`
	Prefix      string `toml:"prefix"`
	MaxIdleConn int    `toml:"max_idle_conn"`
	MaxOpenConn int    `toml:"max_open_conn"`
	MaxLifeTime int    `toml:"max_life_time"`
}

// DSN 生成 MySQL DSN 连接字符串
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=true&loc=Local",
		d.User, d.Pass, d.Host, d.Port, d.Name, d.Charset)
}

// MaxLifeTimeDuration 返回连接最大生命周期
func (d *DatabaseConfig) MaxLifeTimeDuration() time.Duration {
	return time.Duration(d.MaxLifeTime) * time.Second
}

type ClickHouseConfig struct {
	Host string `toml:"host"`
	Port string `toml:"port"`
	User string `toml:"user"`
	Pass string `toml:"pass"`
	Name string `toml:"name"`
}

// Addr 返回 ClickHouse 连接地址
func (c *ClickHouseConfig) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

type RedisConfig struct {
	Host     string `toml:"host"`
	Port     string `toml:"port"`
	User     string `toml:"user"`
	Pass     string `toml:"pass"`
	DB       int    `toml:"db"`
	PoolSize int    `toml:"pool_size"`
}

// Addr 返回 Redis 连接地址
func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", r.Host, r.Port)
}

type KafkaConfig struct {
	Brokers []string         `toml:"brokers"`
	Topics  KafkaTopicConfig `toml:"topics"`
}

type KafkaTopicConfig struct {
	TradeEvents       string `toml:"trade_events"`
	PositionSnapshots string `toml:"position_snapshots"`
	BalanceSnapshots  string `toml:"balance_snapshots"`
	AccountUpdates    string `toml:"account_updates"`
	RiskEvents        string `toml:"risk_events"`
	PermissionChecks  string `toml:"permission_checks"`
}

type BinanceConfig struct {
	APIKey          string `toml:"api_key"`
	SecretKey       string `toml:"secret_key"`
	FuturesWS       string `toml:"futures_ws"`
	FuturesREST     string `toml:"futures_rest"`
	UseTestnet      bool   `toml:"use_testnet"`
	TestnetFuturesWS   string `toml:"testnet_futures_ws"`
	TestnetFuturesREST string `toml:"testnet_futures_rest"`
}

// ActiveWSEndpoint 根据是否使用测试网返回 WS 地址
func (b *BinanceConfig) ActiveWSEndpoint() string {
	if b.UseTestnet {
		return b.TestnetFuturesWS
	}
	return b.FuturesWS
}

// ActiveRESTEndpoint 根据是否使用测试网返回 REST 地址
func (b *BinanceConfig) ActiveRESTEndpoint() string {
	if b.UseTestnet {
		return b.TestnetFuturesREST
	}
	return b.FuturesREST
}

type TelegramConfig struct {
	BotToken    string  `toml:"bot_token"`
	ChatIDs     []int64 `toml:"chat_ids"`
	AppID       int     `toml:"app_id"`
	AppHash     string  `toml:"app_hash"`
	Phone       string  `toml:"phone"`
	SessionName string  `toml:"session_name"`
}

type TwilioConfig struct {
	AccountSID string `toml:"account_sid"`
	AuthToken  string `toml:"auth_token"`
	FromNumber string `toml:"from_number"`
}

type RiskEngineConfig struct {
	AlertCheckInterval      string `toml:"alert_check_interval"`
	RESTReconcileInterval   string `toml:"rest_reconcile_interval"`
	PermissionCheckInterval string `toml:"permission_check_interval"`
	MetricsWindow           string `toml:"metrics_window"`
}

// ParseAlertCheckInterval 解析告警检查间隔
func (r *RiskEngineConfig) ParseAlertCheckInterval() time.Duration {
	d, err := time.ParseDuration(r.AlertCheckInterval)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// ParseRESTReconcileInterval 解析 REST 对账间隔
func (r *RiskEngineConfig) ParseRESTReconcileInterval() time.Duration {
	d, err := time.ParseDuration(r.RESTReconcileInterval)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// ParsePermissionCheckInterval 解析权限检查间隔
func (r *RiskEngineConfig) ParsePermissionCheckInterval() time.Duration {
	d, err := time.ParseDuration(r.PermissionCheckInterval)
	if err != nil {
		return 60 * time.Second
	}
	return d
}

// ParseMetricsWindow 解析指标窗口
func (r *RiskEngineConfig) ParseMetricsWindow() time.Duration {
	d, err := time.ParseDuration(r.MetricsWindow)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// Load 从 TOML 文件加载配置
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}

	// 设置默认值
	cfg.applyDefaults()

	return &cfg, nil
}

// LoadFromEnv 优先从环境变量 CONFIG_PATH 读取配置路径，否则用默认路径
func LoadFromEnv() (*Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config.toml"
	}
	return Load(path)
}

func (c *Config) applyDefaults() {
	if c.System.HTTPPort == "" {
		c.System.HTTPPort = "8080"
	}
	if c.Database.Charset == "" {
		c.Database.Charset = "utf8mb4"
	}
	if c.Database.MaxIdleConn == 0 {
		c.Database.MaxIdleConn = 10
	}
	if c.Database.MaxOpenConn == 0 {
		c.Database.MaxOpenConn = 100
	}
	if c.Database.MaxLifeTime == 0 {
		c.Database.MaxLifeTime = 600
	}
	if c.Redis.PoolSize == 0 {
		c.Redis.PoolSize = 10
	}
	if c.RiskEngine.AlertCheckInterval == "" {
		c.RiskEngine.AlertCheckInterval = "10s"
	}
	if c.RiskEngine.RESTReconcileInterval == "" {
		c.RiskEngine.RESTReconcileInterval = "30s"
	}
	if c.RiskEngine.PermissionCheckInterval == "" {
		c.RiskEngine.PermissionCheckInterval = "60s"
	}
	if c.RiskEngine.MetricsWindow == "" {
		c.RiskEngine.MetricsWindow = "5m"
	}

	// Kafka topics 默认值
	if c.Kafka.Topics.TradeEvents == "" {
		c.Kafka.Topics.TradeEvents = "risk_trade_events"
	}
	if c.Kafka.Topics.PositionSnapshots == "" {
		c.Kafka.Topics.PositionSnapshots = "risk_position_snapshots"
	}
	if c.Kafka.Topics.BalanceSnapshots == "" {
		c.Kafka.Topics.BalanceSnapshots = "risk_balance_snapshots"
	}
	if c.Kafka.Topics.AccountUpdates == "" {
		c.Kafka.Topics.AccountUpdates = "risk_account_updates"
	}
	if c.Kafka.Topics.RiskEvents == "" {
		c.Kafka.Topics.RiskEvents = "risk_risk_events"
	}
	if c.Kafka.Topics.PermissionChecks == "" {
		c.Kafka.Topics.PermissionChecks = "risk_permission_checks"
	}
}
