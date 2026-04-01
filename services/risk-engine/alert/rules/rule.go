// Package rules 定义风控规则引擎的基础框架。
// Rule 接口由各具体规则实现（如 P-001 提币权限检测），
// RuleRegistry 负责注册、查询和管理所有已加载的规则。
package rules

import (
	"context"
	"fmt"
	"sync"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

// Rule 风控规则接口 — 所有具体规则必须实现此接口
// Agent-C 将基于此接口实现各类风控检测规则（P-001 ~ P-xxx）
type Rule interface {
	// Name 返回规则的中文名称，用于日志和通知展示
	Name() string

	// Code 返回规则编号（如 "P-001"），全局唯一
	Code() string

	// Check 对指定账户执行风控检测
	// 返回检测到的风险事件列表；无风险时返回空切片和 nil error
	Check(ctx context.Context, account *models.Account, st *store.Store) ([]models.RiskEvent, error)
}

// RuleRegistry 规则注册表 — 管理所有已注册的风控规则
// 线程安全，支持并发注册和查询
type RuleRegistry struct {
	mu    sync.RWMutex
	rules map[string]Rule // key: rule code
}

// NewRuleRegistry 创建一个空的规则注册表
func NewRuleRegistry() *RuleRegistry {
	return &RuleRegistry{
		rules: make(map[string]Rule),
	}
}

// Register 注册一条规则到注册表
// 如果规则编号已存在，返回错误（防止重复注册覆盖）
func (r *RuleRegistry) Register(rule Rule) error {
	if rule == nil {
		return fmt.Errorf("规则不能为 nil")
	}
	code := rule.Code()
	if code == "" {
		return fmt.Errorf("规则编号不能为空")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rules[code]; exists {
		return fmt.Errorf("规则 %s 已注册，不允许重复注册", code)
	}
	r.rules[code] = rule
	return nil
}

// Get 根据规则编号获取规则，不存在时返回 nil
func (r *RuleRegistry) Get(code string) Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rules[code]
}

// List 返回所有已注册规则的有序列表（按编号排序）
func (r *RuleRegistry) List() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	return result
}

// Codes 返回所有已注册的规则编号
func (r *RuleRegistry) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	codes := make([]string, 0, len(r.rules))
	for code := range r.rules {
		codes = append(codes, code)
	}
	return codes
}

// Len 返回已注册规则的数量
func (r *RuleRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.rules)
}

// DefaultRegistry 全局默认规则注册表
// 各规则模块在 init() 中调用 DefaultRegistry.Register() 完成自动注册
var DefaultRegistry = NewRuleRegistry()
