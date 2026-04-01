package rules

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

// newTestRedisStore 创建基于 miniredis 的测试 Store，无需真实 Redis
func newTestRedisStore(t *testing.T) (*store.Store, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	rds := store.NewRedisFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	return &store.Store{Redis: rds}, mr
}

func TestP001Rule_基本属性(t *testing.T) {
	r := &P001Rule{}
	if r.Name() != "提币权限异常检测" {
		t.Errorf("Name() = %q, want %q", r.Name(), "提币权限异常检测")
	}
	if r.Code() != "P-001" {
		t.Errorf("Code() = %q, want %q", r.Code(), "P-001")
	}
}

func TestP001Rule_Check_权限开启触发告警(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{
		ID:         "acc-001",
		ProjectID:  "proj-001",
		ExchangeID: "binance",
	}

	// 设置提币权限为 true
	mr.Set("permission.acc-001.enable_withdraw", "true")

	r := &P001Rule{}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件, 实际 %d", len(events))
	}

	ev := events[0]
	if ev.EventCode != "P-001" {
		t.Errorf("EventCode = %q, want %q", ev.EventCode, "P-001")
	}
	if ev.Level != "P1" {
		t.Errorf("Level = %q, want %q", ev.Level, "P1")
	}
	if ev.ObjectType != "account" {
		t.Errorf("ObjectType = %q, want %q", ev.ObjectType, "account")
	}
	if ev.ObjectID != "acc-001" {
		t.Errorf("ObjectID = %q, want %q", ev.ObjectID, "acc-001")
	}
	if ev.Status != "open" {
		t.Errorf("Status = %q, want %q", ev.Status, "open")
	}
}

func TestP001Rule_Check_权限关闭无告警(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{ID: "acc-002", ProjectID: "proj-001"}

	// 设置提币权限为 false
	mr.Set("permission.acc-002.enable_withdraw", "false")

	r := &P001Rule{}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("期望 0 个事件, 实际 %d", len(events))
	}
}

func TestP001Rule_Check_权限数据未采集不报错(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{ID: "acc-003", ProjectID: "proj-001"}

	// 不设置任何 key → redis.Nil

	r := &P001Rule{}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("期望 0 个事件（权限未采集应跳过）, 实际 %d", len(events))
	}
}

func TestP001Rule_Check_account为nil返回错误(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	r := &P001Rule{}
	_, err := r.Check(ctx, nil, st)
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
}

func TestP001Rule_Check_store为nil返回错误(t *testing.T) {
	ctx := context.Background()
	account := &models.Account{ID: "acc-004"}
	r := &P001Rule{}

	_, err := r.Check(ctx, account, nil)
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
}

func TestP001Rule_Check_Redis无连接返回错误(t *testing.T) {
	ctx := context.Background()
	account := &models.Account{ID: "acc-005"}
	r := &P001Rule{}

	st := &store.Store{Redis: nil}
	_, err := r.Check(ctx, account, st)
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
}

func TestP001Rule_已注册到DefaultRegistry(t *testing.T) {
	rule := DefaultRegistry.Get("P-001")
	if rule == nil {
		t.Fatal("P-001 未注册到 DefaultRegistry")
	}
	if rule.Code() != "P-001" {
		t.Errorf("Code() = %q, want %q", rule.Code(), "P-001")
	}
}
