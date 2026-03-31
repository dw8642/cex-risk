package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 创建日志实例
func New(debug bool) *zap.Logger {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		panic("init logger: " + err.Error())
	}
	return logger
}
