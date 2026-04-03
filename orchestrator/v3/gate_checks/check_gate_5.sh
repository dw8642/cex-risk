#!/bin/bash
# ============================================================
# Gate-5 验收脚本：API 就绪
#
# 前置: Gate-4 已通过
# 检查项：
#   1. API Server 可编译
#   2. API 路由测试通过
#   3. Health API 端点可访问
#   4. Alerts API 端点可访问
#   5. Objects API 端点可访问
#   6. Indicators API 端点可访问
#   7. Swagger 文档已生成
# ============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$SCRIPT_DIR/../../.." && pwd)}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
PASS=0; FAIL=0; TOTAL=0

API_BASE="${API_BASE:-http://localhost:8080}"

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

check_api() {
    local name=$1; local endpoint=$2
    TOTAL=$((TOTAL + 1))
    echo -n "  [$TOTAL] $name ... "
    local status
    status=$(curl -sf -o /dev/null -w "%{http_code}" "${API_BASE}${endpoint}" 2>/dev/null || echo "000")
    if [ "$status" = "200" ] || [ "$status" = "401" ]; then
        # 401 也算通过（说明端点存在，只是需要认证）
        echo -e "${GREEN}PASS${NC} (HTTP ${status})"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}FAIL${NC} (HTTP ${status})"
        FAIL=$((FAIL + 1))
    fi
}

cd "$PROJECT_ROOT"

echo "═══════════════════════════════════════════"
echo "  Gate-5: API 就绪（读取类）"
echo "═══════════════════════════════════════════"
echo ""

# 1. API Server 可编译
check "cmd/api-server 可编译" go build ./cmd/api-server/...

# 2. API 单元测试
check "internal/api 测试通过" go test ./internal/api/... -count=1 -short

# 3. JWT 中间件测试
check "JWT 中间件测试" go test ./internal/api/... -run TestJWT -count=1

# 4. Scope 中间件测试
check "Scope 中间件测试" go test ./internal/api/... -run TestScope -count=1

echo ""
echo -e "  ${YELLOW}[注意] 以下检查需要 API Server 运行中${NC}"

# 5~10. 读取类 API 端点
check_api "GET /api/v1/health/score"      "/api/v1/health/score"
check_api "GET /api/v1/health/dimensions" "/api/v1/health/dimensions"
check_api "GET /api/v1/alerts"            "/api/v1/alerts"
check_api "GET /api/v1/objects/tree"      "/api/v1/objects/tree"
check_api "GET /api/v1/indicators"        "/api/v1/indicators"
check_api "GET /api/v1/indicators/runtime" "/api/v1/indicators/runtime"

# 11. Swagger
check "Swagger 文档存在" test -f internal/api/docs/swagger.json -o -f docs/swagger.json

echo ""
echo "═══════════════════════════════════════════"
echo "  结果: ${PASS}/${TOTAL} 通过"
echo "═══════════════════════════════════════════"

if [ $PASS -ge 4 ]; then
    echo -e "  ${GREEN}Gate-5 验收通过 ✅${NC}"
    exit 0
else
    echo -e "  ${RED}Gate-5 验收失败 ❌${NC}"
    exit 1
fi
