// Package rules — S-013 系统失明/数据断流检测规则
//
// 规则编号：S-013
// 规则分类：system（系统类）
// 触发条件：任何数据维度超过阈值（默认 30s）未更新
// 预期行为：生成 P0 级别 RiskEvent（系统失明是最高优先级）
//
// 数据断流意味着风控系统对该账户"失明"，无法感知任何风险变化。
// 这是最危险的状态 — 在失明期间发生的任何异常都将被漏检。
// 因此系统失明事件为 P0 最高优先级，需立即处理。
package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

// DefaultFreshnessThreshold 默认数据新鲜度阈值：30 秒
// 超过此时间未收到数据更新，视为数据断流
const DefaultFreshnessThreshold = 30 * time.Second

// S013Rule 系统失明/数据断流检测规则
// 检查 Redis 中账户的最后活跃时间戳，判断数据流是否中断
type S013Rule struct {
	// Threshold 数据新鲜度阈值，可外部配置。为零值时使用默认 30s
	Threshold time.Duration
	// NowFunc 获取当前时间的函数，便于测试注入
	NowFunc func() time.Time
}

// Name 返回规则中文名称
func (r *S013Rule) Name() string {
	return "系统失明/数据断流检测"
}

// Code 返回规则编号
func (r *S013Rule) Code() string {
	return "S-013"
}

// threshold 返回实际使用的阈值，零值时回退到默认值
func (r *S013Rule) threshold() time.Duration {
	if r.Threshold > 0 {
		return r.Threshold
	}
	return DefaultFreshnessThreshold
}

// now 返回当前时间，支持测试注入
func (r *S013Rule) now() time.Time {
	if r.NowFunc != nil {
		return r.NowFunc()
	}
	return time.Now()
}

// Check 对指定账户执行数据断流检测
// 从 Redis 读取 metrics.{accountID}.last_activity 时间戳，
// 如果距今超过阈值（默认 30s），说明该账户的数据流已中断，
// 生成 P0 级别风险事件。
//
// 特殊情况：
// - Redis 中无此 key（redis.Nil）：视为从未收到过数据，同样触发告警
// - Redis 连接异常：返回错误，由上层决定是否降级处理
func (r *S013Rule) Check(ctx context.Context, account *models.Account, st *store.Store) ([]models.RiskEvent, error) {
	if account == nil {
		return nil, fmt.Errorf("S-013: account 不能为 nil")
	}
	if st == nil || st.Redis == nil {
		return nil, fmt.Errorf("S-013: store 或 Redis 连接不可用")
	}

	lastActivity, err := st.Redis.GetLastActivity(ctx, account.ID)
	if err != nil {
		if err == redis.Nil {
			// 从未收到过数据 → 同样视为失明
			return r.buildEvent(account, time.Time{}, "从未收到数据更新"), nil
		}
		return nil, fmt.Errorf("S-013: 读取账户 %s 最后活跃时间失败: %w", account.ID, err)
	}

	// 计算数据空窗时长
	gap := r.now().Sub(lastActivity)
	thresh := r.threshold()

	if gap <= thresh {
		// 数据新鲜，无风险
		return nil, nil
	}

	// 数据断流 → 生成 P0 级别风险事件
	msg := fmt.Sprintf("数据断流 %.0f 秒，阈值 %.0f 秒", gap.Seconds(), thresh.Seconds())
	return r.buildEvent(account, lastActivity, msg), nil
}

// buildEvent 构造系统失明风险事件
func (r *S013Rule) buildEvent(account *models.Account, lastActivity time.Time, reason string) []models.RiskEvent {
	details := map[string]interface{}{
		"account_id":  account.ID,
		"exchange_id": account.ExchangeID,
		"threshold_s": r.threshold().Seconds(),
		"rule_code":   "S-013",
		"reason":      reason,
	}
	// 如果有最后活跃时间，记录到详情中
	if !lastActivity.IsZero() {
		details["last_activity"] = lastActivity.Unix()
		details["gap_seconds"] = r.now().Sub(lastActivity).Seconds()
	}

	event := models.RiskEvent{
		EventCode:  "S-013",
		Level:      "P0",
		Status:     "open",
		ObjectType: "system",
		ObjectID:   account.ID,
		ProjectID:  account.ProjectID,
		Title:      fmt.Sprintf("账户 %s 数据断流告警", account.ID),
		Details:    details,
		CreatedAt:  r.now(),
	}

	return []models.RiskEvent{event}
}

func init() {
	_ = DefaultRegistry.Register(&S013Rule{})
}
