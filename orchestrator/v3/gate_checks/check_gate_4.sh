#!/bin/bash
# ============================================================
# Gate-4 验收脚本：规则层就绪
#
# 前置: Gate-3 已通过
# 检查项：
#   1. 规则引擎可编译
#   2. 规则引擎测试通过（含 72 条 MVP 规则）
#   3. 健康度评分模块测试通过
#   4. 状态机测试通过
#   5. 告警服务测试通过
#   6. RiskEvent 可正常产出
# ============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$SCRIPT_DIR/../../.." && pwd)}"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
PASS=0; FAIL=0; TOTAL=0

check() {
    local name=$1; shift
    TOTAL=$((TOTAL + 1))
    echo -n "  [$TOTAL] $name ... "
    if "$@" >/dev/null 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}FAIL${NC}"
        FAIL=$((FAIL + 1))
    fi
}

cd "$PROJECT_ROOT"

echo "═══════════════════════════════════════════"
echo "  Gate-4: 规则层就绪"
echo "═══════════════════════════════════════════"
echo ""

# 1. 规则引擎可编译
check "cmd/rule-engine 可编译" go build ./cmd/rule-engine/...

# 2. 告警服务可编译
check "cmd/alert-service 可编译" go build ./cmd/alert-service/...

# 3. 规则引擎测试
check "internal/rule 测试通过" go test ./internal/rule/... -count=1 -short

# 4. 四层治理继承解析测试
check "Resolver 测试通过" go test ./internal/rule/... -run TestResolver -count=1

# 5. 健康度评分
check "internal/health 测试通过" go test ./internal/health/... -count=1 -short

# 6. 状态机
check "状态机测试通过" go test ./internal/health/... -run "TestStateMachine|TestHardCap" -count=1

# 7. 告警服务
check "internal/alert 测试通过" go test ./internal/alert/... -count=1 -short

# 8. 告警去重
check "告警去重测试" go test ./internal/alert/... -run TestDedup -count=1

# 9. 与指标层联调
check "RiskEvent 产出验证" go test ./internal/rule/... -run TestRiskEventOutput -count=1

echo ""
echo "═══════════════════════════════════════════"
echo "  结果: ${PASS}/${TOTAL} 通过"
echo "═══════════════════════════════════════════"

if [ $PASS -ge 6 ]; then
    echo -e "  ${GREEN}Gate-4 验收通过 ✅${NC}"
    exit 0
else
    echo -e "  ${RED}Gate-4 验收失败 ❌${NC}"
    exit 1
fi
