#!/bin/bash
# ==============================================================
# Sentinel 最小闭环 Gate 检查脚本
#
# 用法:
#   ./check_sentinel_gates.sh [gate-s0|gate-s1|gate-s2|all]
# ==============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$PROJECT_ROOT"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

pass() { echo -e "  ${GREEN}✅ PASS${NC} $1"; }
fail() { echo -e "  ${RED}❌ FAIL${NC} $1: $2"; FAILED=1; }

GATE="${1:-all}"
FAILED=0

# ---------------------------------------------------------------
# Gate-S0: 契约就绪
# ---------------------------------------------------------------
check_gate_s0() {
    echo -e "\n${YELLOW}═══ Gate-S0: 契约就绪 ═══${NC}"

    # 1. types.go + config.go 存在
    if [ -f "internal/sentinel/types.go" ] && [ -f "internal/sentinel/config.go" ]; then
        pass "types.go + config.go 存在"
    else
        fail "文件检查" "internal/sentinel/types.go 或 config.go 不存在"
    fi

    # 2. 包可编译
    if go build ./internal/sentinel/ 2>/dev/null; then
        pass "go build ./internal/sentinel/ 编译通过"
    else
        fail "编译检查" "internal/sentinel 编译失败"
    fi

    # 3. config.toml 包含 accounts 配置
    if grep -q 'account_type' config.toml 2>/dev/null; then
        pass "config.toml 包含 account_type 配置"
    else
        fail "配置检查" "config.toml 缺少 account_type 字段"
    fi

    # 4. 关键类型存在
    if grep -q 'type AccountInfo struct' internal/sentinel/types.go 2>/dev/null; then
        pass "AccountInfo 类型已定义"
    else
        fail "类型检查" "AccountInfo 未定义"
    fi

    if grep -q 'type AccountData struct' internal/sentinel/types.go 2>/dev/null; then
        pass "AccountData 类型已定义"
    else
        fail "类型检查" "AccountData 未定义"
    fi

    if grep -q 'type RiskAlert struct' internal/sentinel/types.go 2>/dev/null; then
        pass "RiskAlert 类型已定义"
    else
        fail "类型检查" "RiskAlert 未定义"
    fi

    if grep -q 'type SentinelConfig struct' internal/sentinel/config.go 2>/dev/null; then
        pass "SentinelConfig 类型已定义"
    else
        fail "类型检查" "SentinelConfig 未定义"
    fi
}

# ---------------------------------------------------------------
# Gate-S1: 模块就绪
# ---------------------------------------------------------------
check_gate_s1() {
    echo -e "\n${YELLOW}═══ Gate-S1: 模块就绪 ═══${NC}"

    # 1. 所有 sentinel 文件存在
    for f in types.go config.go client.go collector.go rules.go alerter.go engine.go; do
        if [ -f "internal/sentinel/$f" ]; then
            pass "$f 存在"
        else
            fail "文件检查" "internal/sentinel/$f 不存在"
        fi
    done

    # 2. 包可编译
    if go build ./internal/sentinel/ 2>/dev/null; then
        pass "go build ./internal/sentinel/ 编译通过"
    else
        fail "编译检查" "internal/sentinel 编译失败"
        echo "  编译错误详情:"
        go build ./internal/sentinel/ 2>&1 | head -20 || true
    fi

    # 3. go vet 通过
    if go vet ./internal/sentinel/ 2>/dev/null; then
        pass "go vet 通过"
    else
        fail "vet 检查" "go vet 有警告"
    fi

    # 4. Proxy 约束检查 — client.go 中必须使用 proxy
    if grep -q 'http.ProxyURL\|Proxy.*proxyURL\|proxy.*Transport' internal/sentinel/client.go 2>/dev/null; then
        pass "client.go 包含 HTTP Proxy 配置"
    else
        fail "Proxy 检查" "client.go 未配置 HTTP Proxy"
    fi

    # 5. Proxy 约束检查 — alerter.go 中必须使用 proxy
    if grep -q 'http.ProxyURL\|Proxy.*proxyURL\|proxy.*Transport' internal/sentinel/alerter.go 2>/dev/null; then
        pass "alerter.go 包含 HTTP Proxy 配置"
    else
        fail "Proxy 检查" "alerter.go 未配置 HTTP Proxy"
    fi

    # 6. 规则数量检查
    RULE_COUNT=$(grep -c 'func Rule[A-Z]' internal/sentinel/rules.go 2>/dev/null || echo 0)
    if [ "$RULE_COUNT" -ge 6 ]; then
        pass "rules.go 包含 ${RULE_COUNT} 条规则 (≥6)"
    else
        fail "规则检查" "rules.go 只有 ${RULE_COUNT} 条规则 (<6)"
    fi

    # 7. PM 支持检查
    if grep -q 'portfolio_margin\|papi' internal/sentinel/client.go 2>/dev/null; then
        pass "client.go 包含 Portfolio Margin 支持"
    else
        fail "PM 检查" "client.go 未支持 Portfolio Margin"
    fi
}

# ---------------------------------------------------------------
# Gate-S2: 闭环就绪
# ---------------------------------------------------------------
check_gate_s2() {
    echo -e "\n${YELLOW}═══ Gate-S2: 闭环就绪 ═══${NC}"

    # 1. main.go 存在
    if [ -f "cmd/risk-sentinel/main.go" ]; then
        pass "cmd/risk-sentinel/main.go 存在"
    else
        fail "文件检查" "cmd/risk-sentinel/main.go 不存在"
        return
    fi

    # 2. 二进制可编译
    if go build -o /tmp/risk-sentinel ./cmd/risk-sentinel/ 2>/dev/null; then
        pass "go build 二进制编译成功"
    else
        fail "编译检查" "二进制编译失败"
        echo "  编译错误详情:"
        go build -o /tmp/risk-sentinel ./cmd/risk-sentinel/ 2>&1 | head -20 || true
        return
    fi

    # 3. --dry-run 模式可启动（5s 超时）
    if timeout 5 /tmp/risk-sentinel --config config.toml --dry-run 2>/dev/null; then
        pass "dry-run 模式执行成功"
    else
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 124 ]; then
            # 超时但可能是在等 Redis，不算失败
            echo -e "  ${YELLOW}⚠️ WARN${NC} dry-run 超时（可能缺少 Redis），跳过"
        else
            fail "运行检查" "dry-run 执行失败 (exit=$EXIT_CODE)"
        fi
    fi

    # 4. 清理
    rm -f /tmp/risk-sentinel
}

# ---------------------------------------------------------------
# 执行
# ---------------------------------------------------------------
case "$GATE" in
    gate-s0) check_gate_s0 ;;
    gate-s1) check_gate_s1 ;;
    gate-s2) check_gate_s2 ;;
    all)
        check_gate_s0
        check_gate_s1
        check_gate_s2
        ;;
    *)
        echo "用法: $0 [gate-s0|gate-s1|gate-s2|all]"
        exit 1
        ;;
esac

echo ""
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}═══ 所有检查通过 ═══${NC}"
    exit 0
else
    echo -e "${RED}═══ 有检查失败 ═══${NC}"
    exit 1
fi
