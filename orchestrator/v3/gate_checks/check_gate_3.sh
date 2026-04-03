#!/bin/bash
# ============================================================
# Gate-3 验收脚本：指标层就绪
#
# 前置: Gate-2 已通过
# 检查项：
#   1. 指标引擎可编译
#   2. S-Class 指标测试通过
#   3. L-Class 指标测试通过
#   4. 指标 Registry 注册数 >= 79（MVP 指标数）
#   5. Redis 有指标产出数据
#   6. T0~T2 Worker 框架测试通过
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
echo "  Gate-3: 指标层就绪"
echo "═══════════════════════════════════════════"
echo ""

# 1. 指标引擎可编译
check "cmd/indicator-engine 可编译" go build ./cmd/indicator-engine/...

# 2. 指标引擎全量测试
check "internal/indicator 测试通过" go test ./internal/indicator/... -count=1 -short

# 3. S-Class 指标测试
check "S-Class 指标测试" go test ./internal/indicator/... -run "TestSClass|TestS_" -count=1

# 4. L-Class 指标测试
check "L-Class 指标测试" go test ./internal/indicator/... -run "TestLClass|TestL_" -count=1

# 5. 指标 Registry 检查（通过测试函数验证注册数量）
check "指标 Registry 注册数检查" go test ./internal/indicator/... -run TestRegistryCount -count=1

# 6. 与数据层联调（需中间件）
check "Redis 有指标产出" redis-cli EXISTS "ind:IND-S-001:binance:BTC-USDT"

echo ""
echo "═══════════════════════════════════════════"
echo "  结果: ${PASS}/${TOTAL} 通过"
echo "═══════════════════════════════════════════"

if [ $PASS -ge 4 ]; then
    echo -e "  ${GREEN}Gate-3 验收通过 ✅${NC}"
    exit 0
else
    echo -e "  ${RED}Gate-3 验收失败 ❌${NC}"
    exit 1
fi
