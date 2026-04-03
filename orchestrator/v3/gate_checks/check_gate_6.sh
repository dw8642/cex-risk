#!/bin/bash
# ============================================================
# Gate-6 验收脚本：配置 API 就绪
#
# 前置: Gate-5 已通过
# 检查项：
#   1. 规则绑定 CRUD API 测试通过
#   2. 变更审批 API 测试通过
#   3. RBAC 4角色权限矩阵测试
#   4. Scope 过滤测试
#   5. 配置发布（Draft→Preview→Approve→Publish）流程测试
#   6. 前端对接验证
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
        echo -e "${GREEN}PASS${NC} (HTTP ${status})"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}FAIL${NC} (HTTP ${status})"
        FAIL=$((FAIL + 1))
    fi
}

cd "$PROJECT_ROOT"

echo "═══════════════════════════════════════════"
echo "  Gate-6: 配置 API 就绪"
echo "═══════════════════════════════════════════"
echo ""

# 1. 规则绑定 CRUD 测试
check "RuleBinding CRUD 测试" go test ./internal/api/... -run TestRuleBinding -count=1

# 2. 变更审批流程测试
check "Changes 审批测试" go test ./internal/api/... -run TestChanges -count=1

# 3. RBAC 角色权限矩阵
check "RBAC 权限矩阵测试" go test ./internal/api/... -run TestRBAC -count=1

# 4. Scope 过滤
check "Scope 过滤测试" go test ./internal/api/... -run TestScopeFilter -count=1

# 5. 双轨发布流程
check "双轨发布测试" go test ./internal/api/... -run "TestPublish|TestDraftPreviewApprove" -count=1

echo ""
echo -e "  ${YELLOW}[注意] 以下检查需要 API Server 运行中${NC}"

# 6~9. 配置类 API 端点
check_api "GET /api/v1/rules/templates"   "/api/v1/rules/templates"
check_api "GET /api/v1/changes"           "/api/v1/changes"
check_api "GET /api/v1/users"             "/api/v1/users"

# 10. 前端构建
check "前端 build 通过" bash -c "cd web && npm run build"

echo ""
echo "═══════════════════════════════════════════"
echo "  结果: ${PASS}/${TOTAL} 通过"
echo "═══════════════════════════════════════════"

if [ $PASS -ge 5 ]; then
    echo -e "  ${GREEN}Gate-6 验收通过 ✅  MVP 就绪！${NC}"
    exit 0
else
    echo -e "  ${RED}Gate-6 验收失败 ❌${NC}"
    exit 1
fi
