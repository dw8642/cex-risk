package rules

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

func TestS013Rule_基本属性(t *testing.T) {
	r := &S013Rule{}
	if r.Name() != "系统失明/数据断流检测" {
		t.Errorf("Name() = %q, want %q", r.Name(), "系统失明/数据断流检测")
	}
	if r.Code() != "S-013" {
		t.Errorf("Code() = %q, want %q", r.Code(), "S-013")
	}
}

func TestS013Rule_Check_数据新鲜无告警(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{
		ID:         "acc-001",
		ProjectID:  "proj-001",
		ExchangeID: "binance",
	}

	now := time.Now()
	// 设置最后活跃时间为 10 秒前（小于默认阈值 30s）
	mr.Set(fmt.Sprintf("metrics.%s.last_activity", account.ID), fmt.Sprintf("%d", now.Add(-10*time.Second).Unix()))

	r := &S013Rule{
		NowFunc: func() time.Time { return now },
	}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("期望 0 个事件（数据新鲜）, 实际 %d", len(events))
	}
}

func TestS013Rule_Check_数据断流触发P0告警(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{
		ID:         "acc-002",
		ProjectID:  "proj-001",
		ExchangeID: "binance",
	}

	now := time.Now()
	// 设置最后活跃时间为 60 秒前（大于默认阈值 30s）
	mr.Set(fmt.Sprintf("metrics.%s.last_activity", account.ID), fmt.Sprintf("%d", now.Add(-60*time.Second).Unix()))

	r := &S013Rule{
		NowFunc: func() time.Time { return now },
	}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件, 实际 %d", len(events))
	}

	ev := events[0]
	if ev.EventCode != "S-013" {
		t.Errorf("EventCode = %q, want %q", ev.EventCode, "S-013")
	}
	if ev.Level != "P0" {
		t.Errorf("Level = %q, want %q", ev.Level, "P0")
	}
	if ev.ObjectType != "system" {
		t.Errorf("ObjectType = %q, want %q", ev.ObjectType, "system")
	}
	if ev.ObjectID != "acc-002" {
		t.Errorf("ObjectID = %q, want %q", ev.ObjectID, "acc-002")
	}
	if ev.Status != "open" {
		t.Errorf("Status = %q, want %q", ev.Status, "open")
	}
}

func TestS013Rule_Check_从未收到数据触发告警(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{
		ID:         "acc-003",
		ProjectID:  "proj-001",
		ExchangeID: "okx",
	}

	// 不设置任何 key → redis.Nil → 从未收到数据

	r := &S013Rule{}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件（从未收到数据）, 实际 %d", len(events))
	}
	if events[0].Level != "P0" {
		t.Errorf("Level = %q, want %q", events[0].Level, "P0")
	}
}

func TestS013Rule_Check_自定义阈值(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{ID: "acc-004", ProjectID: "proj-001", ExchangeID: "binance"}

	now := time.Now()
	// 设置最后活跃时间为 15 秒前
	mr.Set(fmt.Sprintf("metrics.%s.last_activity", account.ID), fmt.Sprintf("%d", now.Add(-15*time.Second).Unix()))

	// 默认阈值 30s → 不触发
	r1 := &S013Rule{NowFunc: func() time.Time { return now }}
	events, err := r1.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("默认阈值 30s: 期望 0 个事件, 实际 %d", len(events))
	}

	// 自定义阈值 10s → 触发
	r2 := &S013Rule{
		Threshold: 10 * time.Second,
		NowFunc:   func() time.Time { return now },
	}
	events, err = r2.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("自定义阈值 10s: 期望 1 个事件, 实际 %d", len(events))
	}
}

func TestS013Rule_Check_边界值恰好等于阈值(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{ID: "acc-005", ProjectID: "proj-001", ExchangeID: "binance"}

	// 使用整秒对齐的时间，避免 Unix 秒级精度导致的偏差
	now := time.Unix(time.Now().Unix(), 0)
	// 设置最后活跃时间恰好等于阈值（30s 前）
	mr.Set(fmt.Sprintf("metrics.%s.last_activity", account.ID), fmt.Sprintf("%d", now.Add(-30*time.Second).Unix()))

	r := &S013Rule{NowFunc: func() time.Time { return now }}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	// 恰好等于阈值：gap == threshold → gap <= threshold → 不触发
	if len(events) != 0 {
		t.Errorf("恰好等于阈值: 期望 0 个事件, 实际 %d", len(events))
	}
}

func TestS013Rule_Check_account为nil返回错误(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	r := &S013Rule{}
	_, err := r.Check(ctx, nil, st)
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
}

func TestS013Rule_Check_store为nil返回错误(t *testing.T) {
	ctx := context.Background()
	account := &models.Account{ID: "acc-006"}
	r := &S013Rule{}
	_, err := r.Check(ctx, account, nil)
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
}

func TestS013Rule_Check_Redis无连接返回错误(t *testing.T) {
	ctx := context.Background()
	account := &models.Account{ID: "acc-007"}
	r := &S013Rule{}
	st := &store.Store{Redis: nil}
	_, err := r.Check(ctx, account, st)
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
}

func TestS013Rule_Check_事件详情包含关键字段(t *testing.T) {
	st, mr := newTestRedisStore(t)
	defer mr.Close()

	ctx := context.Background()
	account := &models.Account{ID: "acc-008", ProjectID: "proj-002", ExchangeID: "bybit"}

	now := time.Now()
	mr.Set(fmt.Sprintf("metrics.%s.last_activity", account.ID), fmt.Sprintf("%d", now.Add(-60*time.Second).Unix()))

	r := &S013Rule{NowFunc: func() time.Time { return now }}
	events, err := r.Check(ctx, account, st)
	if err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件, 实际 %d", len(events))
	}

	details := events[0].Details
	if details["account_id"] != "acc-008" {
		t.Errorf("details.account_id = %v, want %q", details["account_id"], "acc-008")
	}
	if details["exchange_id"] != "bybit" {
		t.Errorf("details.exchange_id = %v, want %q", details["exchange_id"], "bybit")
	}
	if details["threshold_s"] != float64(30) {
		t.Errorf("details.threshold_s = %v, want %v", details["threshold_s"], 30.0)
	}
	if _, ok := details["last_activity"]; !ok {
		t.Error("details 缺少 last_activity 字段")
	}
	if _, ok := details["gap_seconds"]; !ok {
		t.Error("details 缺少 gap_seconds 字段")
	}
}

func TestS013Rule_已注册到DefaultRegistry(t *testing.T) {
	rule := DefaultRegistry.Get("S-013")
	if rule == nil {
		t.Fatal("S-013 未注册到 DefaultRegistry")
	}
	if rule.Code() != "S-013" {
		t.Errorf("Code() = %q, want %q", rule.Code(), "S-013")
	}
}
