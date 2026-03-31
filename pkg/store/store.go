package store

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/cex-risk/cex-risk/pkg/config"
)

// Store 统一数据存储管理器，聚合 MySQL + Redis + ClickHouse
type Store struct {
	MySQL      *MySQL
	Redis      *Redis
	ClickHouse *ClickHouse
	logger     *zap.Logger
}

// NewStore 创建完整的存储连接
// clickhouseOptional 为 true 时 ClickHouse 连接失败不会阻断启动（演示版 MySQL 暂替）
func NewStore(cfg *config.Config, logger *zap.Logger, clickhouseOptional bool) (*Store, error) {
	s := &Store{logger: logger}

	// MySQL（必须）
	mysql, err := NewMySQL(&cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("init mysql: %w", err)
	}
	s.MySQL = mysql

	// Redis（必须）
	rds, err := NewRedis(&cfg.Redis, logger)
	if err != nil {
		mysql.Close()
		return nil, fmt.Errorf("init redis: %w", err)
	}
	s.Redis = rds

	// ClickHouse（可选，演示版用 MySQL 暂替）
	ch, err := NewClickHouse(&cfg.ClickHouse, logger)
	if err != nil {
		if clickhouseOptional {
			logger.Warn("ClickHouse not available, using MySQL fallback for trades storage", zap.Error(err))
		} else {
			mysql.Close()
			rds.Close()
			return nil, fmt.Errorf("init clickhouse: %w", err)
		}
	} else {
		s.ClickHouse = ch
	}

	return s, nil
}

// Close 关闭所有连接
func (s *Store) Close() {
	if s.MySQL != nil {
		s.MySQL.Close()
	}
	if s.Redis != nil {
		s.Redis.Close()
	}
	if s.ClickHouse != nil {
		s.ClickHouse.Close()
	}
	s.logger.Info("All store connections closed")
}

// HealthCheck 检查所有连接健康状态
func (s *Store) HealthCheck(ctx context.Context) map[string]string {
	status := make(map[string]string)

	if err := s.MySQL.Ping(ctx); err != nil {
		status["mysql"] = fmt.Sprintf("error: %v", err)
	} else {
		status["mysql"] = "ok"
	}

	if err := s.Redis.Ping(ctx); err != nil {
		status["redis"] = fmt.Sprintf("error: %v", err)
	} else {
		status["redis"] = "ok"
	}

	if s.ClickHouse != nil {
		if err := s.ClickHouse.Ping(ctx); err != nil {
			status["clickhouse"] = fmt.Sprintf("error: %v", err)
		} else {
			status["clickhouse"] = "ok"
		}
	} else {
		status["clickhouse"] = "not_configured"
	}

	return status
}

// HasClickHouse 是否有 ClickHouse 可用
func (s *Store) HasClickHouse() bool {
	return s.ClickHouse != nil
}
