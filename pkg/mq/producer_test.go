package mq

import (
	"context"
	"sort"
	"testing"

	"go.uber.org/zap"
)

// TestNewProducer 验证生产者初始化
func TestNewProducer(t *testing.T) {
	log := zap.NewNop()
	brokers := []string{"localhost:9092"}
	topics := []string{"topic_a", "topic_b", "topic_c"}

	p := NewProducer(brokers, topics, log)
	if p == nil {
		t.Fatal("NewProducer 返回 nil")
	}
	if len(p.writers) != 3 {
		t.Errorf("期望 3 个 writer，实际 %d", len(p.writers))
	}
	if len(p.brokers) != 1 {
		t.Errorf("期望 1 个 broker，实际 %d", len(p.brokers))
	}
}

// TestNewProducer_Empty 验证空 topic 列表
func TestNewProducer_Empty(t *testing.T) {
	log := zap.NewNop()
	p := NewProducer([]string{"localhost:9092"}, nil, log)
	if len(p.writers) != 0 {
		t.Errorf("期望 0 个 writer，实际 %d", len(p.writers))
	}
}

// TestProducerTopics 验证 Topics() 返回已注册的 topic
func TestProducerTopics(t *testing.T) {
	log := zap.NewNop()
	expected := []string{"risk_events", "risk_trades"}
	p := NewProducer([]string{"localhost:9092"}, expected, log)

	got := p.Topics()
	sort.Strings(got)
	sort.Strings(expected)

	if len(got) != len(expected) {
		t.Fatalf("期望 %d 个 topic，实际 %d", len(expected), len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("topic[%d] 期望 %s，实际 %s", i, expected[i], got[i])
		}
	}
}

// TestPublish_UnknownTopic 验证向未注册 topic 发送消息返回错误
func TestPublish_UnknownTopic(t *testing.T) {
	log := zap.NewNop()
	p := NewProducer([]string{"localhost:9092"}, []string{"known_topic"}, log)

	err := p.Publish(context.Background(), "unknown_topic", []byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("期望 unknown topic 返回错误，实际 nil")
	}
}

// TestPublishBatch_UnknownTopic 验证批量发送到未注册 topic 返回错误
func TestPublishBatch_UnknownTopic(t *testing.T) {
	log := zap.NewNop()
	p := NewProducer([]string{"localhost:9092"}, []string{"known_topic"}, log)

	err := p.PublishBatch(context.Background(), "unknown_topic", nil)
	if err == nil {
		t.Fatal("期望 unknown topic 返回错误，实际 nil")
	}
}

// TestProducerClose_NoWriters 验证空 producer 关闭不报错
func TestProducerClose_NoWriters(t *testing.T) {
	log := zap.NewNop()
	p := NewProducer([]string{"localhost:9092"}, nil, log)
	if err := p.Close(); err != nil {
		t.Errorf("空 producer 关闭出错: %v", err)
	}
}
