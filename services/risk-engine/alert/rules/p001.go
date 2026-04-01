// Package rules — P-001 提币权限异常检测规则
//
// 规则编号：P-001
// 规则分类：permission（权限类）
// 触发条件：API Key 当前具有提币权限（enable_withdraw = true）
// 预期行为：生成 P1 级别 RiskEvent，通知管理员确认
//
// 做市 API Key 通常不应具备提币权限。如果检测到提币权限开启，
// 说明可能存在配置错误或账户被篡改的风险。
package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

// P001Rule 提币权限异常检测规则
// 检查 Redis 中缓存的 API Key 实际权限，如果 enable_withdraw 为 true 则触发告警
type P001Rule struct{}

// Name 返回规则中文名称
func (r *P001Rule) Name() string {
	return "提币权限异常检测"
}

// Code 返回规则编号
func (r *P001Rule) Code() string {
	return "P-001"
}

// Check 对指定账户执行提币权限检测
// 从 Redis 读取 permission.{accountID}.enable_withdraw 字段，
// 如果值为 "true"，说明该 API Key 具有提币权限，生成 P1 级别风险事件。
// 如果 Redis 中无此 key（redis.Nil），视为权限数据尚未采集，跳过不报错。
func (r *P001Rule) Check(ctx context.Context, account *models.Account, st *store.Store) ([]models.RiskEvent, error) {
	if account == nil {
		return nil, fmt.Errorf("P-001: account 不能为 nil")
	}
	if st == nil || st.Redis == nil {
		return nil, fmt.Errorf("P-001: store 或 Redis 连接不可用")
	}

	// 从 Redis 读取实际的提币权限状态
	val, err := st.Redis.GetPermission(ctx, account.ID, "enable_withdraw")
	if err != nil {
		// key 不存在 → 权限数据尚未采集，不视为异常
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("P-001: 读取账户 %s 提币权限失败: %w", account.ID, err)
	}

	// 权限值不为 "true" → 正常，无风险
	if val != "true" {
		return nil, nil
	}

	// 提币权限开启 → 生成 P1 级别风险事件
	event := models.RiskEvent{
		EventCode:  "P-001",
		Level:      "P1",
		Status:     "open",
		ObjectType: "account",
		ObjectID:   account.ID,
		ProjectID:  account.ProjectID,
		Title:      fmt.Sprintf("账户 %s 检测到提币权限开启", account.ID),
		Details: map[string]interface{}{
			"account_id":      account.ID,
			"exchange_id":     account.ExchangeID,
			"enable_withdraw": true,
			"rule_code":       "P-001",
			"message":         "做市 API Key 不应具备提币权限，请立即确认是否为预期配置",
		},
		CreatedAt: time.Now(),
	}

	return []models.RiskEvent{event}, nil
}

func init() {
	_ = DefaultRegistry.Register(&P001Rule{})
}
