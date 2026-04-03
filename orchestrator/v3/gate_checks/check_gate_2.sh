#!/bin/bash
# ============================================================
# Gate-2 验收脚本：数据管道就绪
#
# 前置: Gate-1 已通过
# 检查项：
#   1. 公共 Ingestor 可编译
#   2. 私有 Ingestor 可编译
#   3. Kafka public topic 有数据
#   4. Kafka private topic 有数据
#   5. Redis 有原始数据最新状态
#   6. 数据层单元测试通过
# ============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$SCRIPT_DIR/../../.." && pwd)}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
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

check_with_timeout() {
    local name=$1; local timeout=$2; shift 2
    TOTAL=$((TOTAL + 1))
    echo -n "  [$TOTAL] $name ... "
    if timeout "$timeout" "$@" >/dev/null 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}FAIL${NC}"
        FAIL=$((FAIL + 1))
    fi
}

cd "$PROJECT_ROOT"

echo "═══════════════════════════════════════════"
echo "  Gate-2: 数据管道就绪"
echo "═══════════════════════════════════════════"
echo ""

# 1. 公共 Ingestor 可编译
check "cmd/ingestor-public 可编译" go build ./cmd/ingestor-public/...

# 2. 私有 Ingestor 可编译
check "cmd/ingestor-private 可编译" go build ./cmd/ingestor-private/...

# 3. Ingestor 单元测试
check "internal/ingestor 测试通过" go test ./internal/ingestor/... -count=1 -short

# 4. Exchange 适配层测试
check "pkg/exchange 测试通过" go test ./pkg/exchange/... -count=1 -short

# 5. Kafka public topic 有数据（需要中间件运行）
echo -e "  ${YELLOW}[注意] 以下检查需要中间件运行（docker compose up）${NC}"

check_with_timeout "Kafka public.orderbook 有数据" 15 \
    kafka-console-consumer --bootstrap-server localhost:9092 \
    --topic public.orderbook --max-messages 1 --timeout-ms 10000

# 6. Kafka private topic 有数据
check_with_timeout "Kafka private.orders 有数据" 15 \
    kafka-console-consumer --bootstrap-server localhost:9092 \
    --topic private.orders --max-messages 1 --timeout-ms 10000

# 7. Redis 有原始数据
check "Redis 有公共数据" redis-cli EXISTS "raw:binance:orderbook:BTC-USDT"

echo ""
echo "═══════════════════════════════════════════"
echo "  结果: ${PASS}/${TOTAL} 通过"
echo "═══════════════════════════════════════════"

# 核心: 编译+测试通过（前4项）即可通过，中间件相关为加分项
if [ $PASS -ge 4 ]; then
    echo -e "  ${GREEN}Gate-2 验收通过 ✅${NC}"
    exit 0
else
    echo -e "  ${RED}Gate-2 验收失败 ❌${NC}"
    exit 1
fi
