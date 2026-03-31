package mq

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// MessageHandler 消息处理回调
type MessageHandler func(ctx context.Context, msg kafka.Message) error

// Consumer Kafka 消息消费者
type Consumer struct {
	reader  *kafka.Reader
	handler MessageHandler
	logger  *zap.Logger
	topic   string
	group   string
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Brokers  []string
	Topic    string
	GroupID  string
	MinBytes int
	MaxBytes int
	MaxWait  time.Duration
}

// NewConsumer 创建 Kafka 消费者
func NewConsumer(cfg ConsumerConfig, handler MessageHandler, logger *zap.Logger) *Consumer {
	if cfg.MinBytes == 0 {
		cfg.MinBytes = 1
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 10 * 1024 * 1024 // 10MB
	}
	if cfg.MaxWait == 0 {
		cfg.MaxWait = 500 * time.Millisecond
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		Topic:    cfg.Topic,
		GroupID:  cfg.GroupID,
		MinBytes: cfg.MinBytes,
		MaxBytes: cfg.MaxBytes,
		MaxWait:  cfg.MaxWait,
	})

	logger.Info("Kafka consumer initialized",
		zap.String("topic", cfg.Topic),
		zap.String("group", cfg.GroupID))

	return &Consumer{
		reader:  reader,
		handler: handler,
		logger:  logger,
		topic:   cfg.Topic,
		group:   cfg.GroupID,
	}
}

// Start 启动消费循环（阻塞，直到 ctx 取消）
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("consumer started",
		zap.String("topic", c.topic),
		zap.String("group", c.group))

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer stopping",
				zap.String("topic", c.topic))
			return c.reader.Close()
		default:
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return c.reader.Close()
			}
			c.logger.Error("fetch message failed",
				zap.String("topic", c.topic),
				zap.Error(err))
			time.Sleep(time.Second) // back off on error
			continue
		}

		if err := c.handler(ctx, msg); err != nil {
			c.logger.Error("handle message failed",
				zap.String("topic", c.topic),
				zap.Int64("offset", msg.Offset),
				zap.Error(err))
			// 不 commit，下次重新消费
			continue
		}

		// 处理成功后 commit
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("commit message failed",
				zap.String("topic", c.topic),
				zap.Int64("offset", msg.Offset),
				zap.Error(err))
		}
	}
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	return c.reader.Close()
}

// ConsumerGroup 管理多个消费者的生命周期
type ConsumerGroup struct {
	consumers []*Consumer
	logger    *zap.Logger
}

// NewConsumerGroup 创建消费者组管理器
func NewConsumerGroup(logger *zap.Logger) *ConsumerGroup {
	return &ConsumerGroup{logger: logger}
}

// Add 添加消费者
func (g *ConsumerGroup) Add(c *Consumer) {
	g.consumers = append(g.consumers, c)
}

// StartAll 启动所有消费者（非阻塞，每个消费者一个 goroutine）
func (g *ConsumerGroup) StartAll(ctx context.Context) {
	for _, c := range g.consumers {
		go func(consumer *Consumer) {
			if err := consumer.Start(ctx); err != nil {
				g.logger.Error("consumer exited with error",
					zap.String("topic", consumer.topic),
					zap.Error(err))
			}
		}(c)
	}
	g.logger.Info(fmt.Sprintf("started %d consumers", len(g.consumers)))
}

// CloseAll 关闭所有消费者
func (g *ConsumerGroup) CloseAll() {
	for _, c := range g.consumers {
		c.Close()
	}
}
