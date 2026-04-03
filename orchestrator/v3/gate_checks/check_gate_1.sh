#!/bin/bash
# ============================================================
# Gate-1 验收脚本：基座就绪
#
# 检查项：
#   1. pkg/model/* 数据模型可编译
#   2. pkg/store/* 存储抽象层可编译
#   3. internal/config/* 配置框架可编译
#   4. deploy/docker-compose.yml 语法正确
#   5. 各 cmd/*/main.go 入口可编译
#   6. deploy/init_mysql.sql 存在且非空
#   7. Kafka Topic 脚本存在
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
echo "  Gate-1: 基座就绪"
echo "═══════════════════════════════════════════"
echo ""

# 1. 数据模型
check "pkg/model 可编译" go build ./pkg/model/...

# 2. 存储抽象层
check "pkg/store 可编译" go build ./pkg/store/...

# 3. 配置框架
check "internal/config 可编译" go build ./internal/config/...

# 4. Docker Compose 语法
check "docker-compose.yml 有效" docker compose -f deploy/docker-compose.yml config --quiet

# 5. 入口骨架可编译
check "cmd/* 可编译" go build ./cmd/...

# 6. MySQL DDL 存在
check "init_mysql.sql 存在" test -s deploy/init_mysql.sql

# 7. Kafka Topic 脚本
check "Kafka Topic 脚本存在" test -f deploy/create_topics.sh -o -f scripts/create_topics.sh

# 8. Redis Key 命名文档
check "Redis Key 文档存在" test -f docs/redis_key_convention.md -o -f deploy/redis_keys.md

echo ""
echo "═══════════════════════════════════════════"
echo "  结果: ${PASS}/${TOTAL} 通过"
echo "═══════════════════════════════════════════"

# Gate-1 至少需要前5项核心检查通过
if [ $PASS -ge 5 ]; then
    echo -e "  ${GREEN}Gate-1 验收通过 ✅${NC}"
    exit 0
else
    echo -e "  ${RED}Gate-1 验收失败 ❌ (需要至少 5 项通过)${NC}"
    exit 1
fi
