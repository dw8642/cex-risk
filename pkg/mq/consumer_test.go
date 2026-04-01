package mq

import (
	"context"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// TestNewConsumer 验证消费者初始化及默认值
func TestNewConsumer(t *testing.T) {
	log := zap.NewNop()
	handler := MessageHandler(func(ctx context.Context, msg kafka.Message) error {
		return nil
	})

	c := NewConsumer(ConsumerConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "test_topic",
		GroupID: "test_group",
	}, handler, log)

	if c == nil {
		t.Fatal("NewConsumer 返回 nil")
	}
	if c.topic != "test_topic" {
		t.Errorf("期望 topic=test_topic，实际 %s", c.topic)
	}
	if c.group != "test_group" {
		t.Errorf("期望 group=test_group，实际 %s", c.group)
	}
	if c.reader == nil {
		t.Error("reader 不应为 nil")
	}
	c.Close()
}

// TestConsumerConfig_Defaults 验证 ConsumerConfig 默认值（零值表示会被 NewConsumer 设置）
func TestConsumerConfig_Defaults(t *testing.T) {
	cfg := ConsumerConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "t",
		GroupID: "g",
	}
	if cfg.MinBytes != 0 {
		t.Error("MinBytes 初始值应为 0")
	}
	if cfg.MaxBytes != 0 {
		t.Error("MaxBytes 初始值应为 0")
	}
	if cfg.MaxWait != 0 {
		t.Error("MaxWait 初始值应为 0")
	}
}

// TestConsumerConfig_CustomValues 验证自定义配置被正确传递
func TestConsumerConfig_CustomValues(t *testing.T) {
	log := zap.NewNop()
	handler := MessageHandler(func(ctx context.Context, msg kafka.Message) error {
		return nil
	})

	cfg := ConsumerConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    "custom_topic",
		GroupID:  "custom_group",
		MinBytes: 1024,
		MaxBytes: 5 * 1024 * 1024,
		MaxWait:  time.Second,
	}

	c := NewConsumer(cfg, handler, log)
	if c.topic != "custom_topic" {
		t.Errorf("期望 topic=custom_topic，实际 %s", c.topic)
	}
	c.Close()
}

// TestConsumerGroup 验证消费者组管理器初始化
func TestConsumerGroup(t *testing.T) {
	log := zap.NewNop()
	g := NewConsumerGroup(log)
	if g == nil {
		t.Fatal("NewConsumerGroup 返回 nil")
	}
	if len(g.consumers) != 0 {
		t.Errorf("初始消费者数应为 0，实际 %d", len(g.consumers))
	}
}

// TestConsumerGroup_Add 验证添加多个消费者
func TestConsumerGroup_Add(t *testing.T) {
	log := zap.NewNop()
	handler := MessageHandler(func(ctx context.Context, msg kafka.Message) error {
		return nil
	})

	g := NewConsumerGroup(log)

	c1 := NewConsumer(ConsumerConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "topic_1",
		GroupID: "g",
	}, handler, log)
	defer c1.Close()

	c2 := NewConsumer(ConsumerConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "topic_2",
		GroupID: "g",
	}, handler, log)
	defer c2.Close()

	g.Add(c1)
	g.Add(c2)

	if len(g.consumers) != 2 {
		t.Errorf("期望 2 个消费者，实际 %d", len(g.consumers))
	}
}
