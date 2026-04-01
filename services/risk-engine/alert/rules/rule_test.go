package rules

import (
	"context"
	"testing"

	"github.com/cex-risk/cex-risk/pkg/models"
	"github.com/cex-risk/cex-risk/pkg/store"
)

// mockRule 测试用的模拟规则
type mockRule struct {
	name   string
	code   string
	events []models.RiskEvent
	err    error
}

func (m *mockRule) Name() string { return m.name }
func (m *mockRule) Code() string { return m.code }
func (m *mockRule) Check(_ context.Context, _ *models.Account, _ *store.Store) ([]models.RiskEvent, error) {
	return m.events, m.err
}

// TestNewRuleRegistry 测试创建空注册表
func TestNewRuleRegistry(t *testing.T) {
	reg := NewRuleRegistry()
	if reg == nil {
		t.Fatal("NewRuleRegistry 不应返回 nil")
	}
	if reg.Len() != 0 {
		t.Fatalf("新注册表应为空，实际有 %d 条规则", reg.Len())
	}
}

// TestRegister_正常注册 测试正常注册规则
func TestRegister_正常注册(t *testing.T) {
	reg := NewRuleRegistry()
	rule := &mockRule{name: "提币权限检测", code: "P-001"}

	err := reg.Register(rule)
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("注册后应有 1 条规则，实际有 %d 条", reg.Len())
	}
}

// TestRegister_重复注册 测试重复注册同一编号应报错
func TestRegister_重复注册(t *testing.T) {
	reg := NewRuleRegistry()
	rule1 := &mockRule{name: "规则A", code: "P-001"}
	rule2 := &mockRule{name: "规则B", code: "P-001"}

	if err := reg.Register(rule1); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}

	err := reg.Register(rule2)
	if err == nil {
		t.Fatal("重复注册应返回错误")
	}
}

// TestRegister_nil规则 测试注册 nil 应报错
func TestRegister_nil规则(t *testing.T) {
	reg := NewRuleRegistry()
	err := reg.Register(nil)
	if err == nil {
		t.Fatal("注册 nil 规则应返回错误")
	}
}

// TestRegister_空编号 测试注册空编号应报错
func TestRegister_空编号(t *testing.T) {
	reg := NewRuleRegistry()
	rule := &mockRule{name: "无编号规则", code: ""}
	err := reg.Register(rule)
	if err == nil {
		t.Fatal("注册空编号规则应返回错误")
	}
}

// TestGet_存在的规则 测试获取已注册的规则
func TestGet_存在的规则(t *testing.T) {
	reg := NewRuleRegistry()
	rule := &mockRule{name: "提币权限检测", code: "P-001"}
	_ = reg.Register(rule)

	got := reg.Get("P-001")
	if got == nil {
		t.Fatal("应能获取已注册的规则")
	}
	if got.Code() != "P-001" {
		t.Fatalf("获取的规则编号不匹配: %s", got.Code())
	}
	if got.Name() != "提币权限检测" {
		t.Fatalf("获取的规则名称不匹配: %s", got.Name())
	}
}

// TestGet_不存在的规则 测试获取未注册的规则应返回 nil
func TestGet_不存在的规则(t *testing.T) {
	reg := NewRuleRegistry()
	got := reg.Get("NOT-EXIST")
	if got != nil {
		t.Fatal("获取不存在的规则应返回 nil")
	}
}

// TestList_多条规则 测试列出所有规则
func TestList_多条规则(t *testing.T) {
	reg := NewRuleRegistry()
	_ = reg.Register(&mockRule{name: "规则A", code: "P-001"})
	_ = reg.Register(&mockRule{name: "规则B", code: "P-002"})
	_ = reg.Register(&mockRule{name: "规则C", code: "P-003"})

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("应列出 3 条规则，实际 %d 条", len(list))
	}
}

// TestCodes 测试返回所有规则编号
func TestCodes(t *testing.T) {
	reg := NewRuleRegistry()
	_ = reg.Register(&mockRule{name: "规则A", code: "P-001"})
	_ = reg.Register(&mockRule{name: "规则B", code: "P-002"})

	codes := reg.Codes()
	if len(codes) != 2 {
		t.Fatalf("应有 2 个编号，实际 %d 个", len(codes))
	}

	// 验证编号都存在
	codeSet := make(map[string]bool)
	for _, c := range codes {
		codeSet[c] = true
	}
	if !codeSet["P-001"] || !codeSet["P-002"] {
		t.Fatalf("编号列表不完整: %v", codes)
	}
}

// TestMockRule_Check 测试模拟规则的 Check 方法
func TestMockRule_Check(t *testing.T) {
	events := []models.RiskEvent{
		{EventCode: "P-001", Level: "P1", Title: "检测到提币权限异常"},
	}
	rule := &mockRule{
		name:   "提币权限检测",
		code:   "P-001",
		events: events,
	}

	got, err := rule.Check(context.Background(), &models.Account{ID: "acc-1"}, nil)
	if err != nil {
		t.Fatalf("Check 不应返回错误: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("应返回 1 个事件，实际 %d 个", len(got))
	}
	if got[0].EventCode != "P-001" {
		t.Fatalf("事件编号不匹配: %s", got[0].EventCode)
	}
}

// TestDefaultRegistry 测试全局默认注册表存在且可用
func TestDefaultRegistry(t *testing.T) {
	if DefaultRegistry == nil {
		t.Fatal("DefaultRegistry 不应为 nil")
	}
}
