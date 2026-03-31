package mq

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Producer Kafka 消息生产者
type Producer struct {
	writers map[string]*kafka.Writer // topic -> writer
	brokers []string
	logger  *zap.Logger
}

// NewProducer 创建 Kafka 生产者，按 topic 初始化 writer
func NewProducer(brokers []string, topics []string, logger *zap.Logger) *Producer {
	p := &Producer{
		writers: make(map[string]*kafka.Writer),
		brokers: brokers,
		logger:  logger,
	}

	for _, topic := range topics {
		p.writers[topic] = &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			BatchSize:    100,
			BatchTimeout: 10 * time.Millisecond,
			WriteTimeout: 10 * time.Second,
			RequiredAcks: kafka.RequireOne,
			Async:        false,
		}
	}

	logger.Info("Kafka producer initialized",
		zap.Strings("brokers", brokers),
		zap.Int("topics", len(topics)))

	return p
}

// Publish 发布消息到指定 topic
func (p *Producer) Publish(ctx context.Context, topic string, key []byte, value []byte) error {
	w, ok := p.writers[topic]
	if !ok {
		return fmt.Errorf("unknown topic: %s", topic)
	}

	msg := kafka.Message{
		Key:   key,
		Value: value,
		Time:  time.Now(),
	}

	if err := w.WriteMessages(ctx, msg); err != nil {
		p.logger.Error("kafka publish failed",
			zap.String("topic", topic),
			zap.Error(err))
		return fmt.Errorf("kafka publish to %s: %w", topic, err)
	}

	return nil
}

// PublishBatch 批量发布消息
func (p *Producer) PublishBatch(ctx context.Context, topic string, messages []kafka.Message) error {
	w, ok := p.writers[topic]
	if !ok {
		return fmt.Errorf("unknown topic: %s", topic)
	}

	if err := w.WriteMessages(ctx, messages...); err != nil {
		p.logger.Error("kafka batch publish failed",
			zap.String("topic", topic),
			zap.Int("count", len(messages)),
			zap.Error(err))
		return fmt.Errorf("kafka batch publish to %s: %w", topic, err)
	}

	return nil
}

// Close 关闭所有 writer
func (p *Producer) Close() error {
	var lastErr error
	for topic, w := range p.writers {
		if err := w.Close(); err != nil {
			p.logger.Error("kafka writer close failed",
				zap.String("topic", topic),
				zap.Error(err))
			lastErr = err
		}
	}
	return lastErr
}

// Topics 返回已注册的所有 topic
func (p *Producer) Topics() []string {
	topics := make([]string, 0, len(p.writers))
	for t := range p.writers {
		topics = append(topics, t)
	}
	return topics
}
