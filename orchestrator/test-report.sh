#!/bin/bash
# ==============================================================
# CEX 风控系统 — 测试报告生成器
#
# 功能：运行单元测试/集成测试/压力测试，生成结构化报告
# 用法：./test-report.sh <type> [options]
#
# 类型：
#   unit          运行单元测试 + 生成覆盖率报告
#   integration   运行集成测试（需中间件）
#   benchmark     运行压力测试
#   all           运行全部测试
# ==============================================================

set -euo pipefail

# 配置
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
REPORTS_DIR="${PROJECT_ROOT}/tests/reports"
TIMESTAMP=$(date '+%Y%m%d_%H%M%S')

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}   $*"; }
log_error() { echo -e "${RED}[ERR]${NC}  $*"; }
log_step()  { echo -e "${CYAN}[STEP]${NC} $*"; }

# 确保报告目录存在
mkdir -p "$REPORTS_DIR"

# ---------------------------------------------------------------
# 单元测试
# ---------------------------------------------------------------
run_unit_tests() {
    local report_file="${REPORTS_DIR}/unit_test_${TIMESTAMP}.md"
    local json_report="${REPORTS_DIR}/unit_test_${TIMESTAMP}.json"
    local coverage_file="${REPORTS_DIR}/coverage_${TIMESTAMP}.out"
    local coverage_html="${REPORTS_DIR}/coverage_${TIMESTAMP}.html"

    log_step "运行单元测试..."
    cd "$PROJECT_ROOT"

    # 运行测试并捕获 JSON 输出
    local test_output
    local exit_code=0
    test_output=$(go test ./... -v -count=1 -coverprofile="$coverage_file" -json 2>&1) || exit_code=$?

    # 解析 JSON 输出生成报告
    local total=0 passed=0 failed=0 skipped=0
    local failed_tests=""

    while IFS= read -r line; do
        local action
        action=$(echo "$line" | jq -r '.Action // empty' 2>/dev/null)
        case "$action" in
            pass)
                local test_name
                test_name=$(echo "$line" | jq -r '.Test // empty' 2>/dev/null)
                if [ -n "$test_name" ]; then
                    ((passed++)) || true
                    ((total++)) || true
                fi
                ;;
            fail)
                local test_name
                test_name=$(echo "$line" | jq -r '.Test // empty' 2>/dev/null)
                if [ -n "$test_name" ]; then
                    ((failed++)) || true
                    ((total++)) || true
                    failed_tests="${failed_tests}\n- ${test_name}"
                fi
                ;;
            skip)
                local test_name
                test_name=$(echo "$line" | jq -r '.Test // empty' 2>/dev/null)
                if [ -n "$test_name" ]; then
                    ((skipped++)) || true
                    ((total++)) || true
                fi
                ;;
        esac
    done <<< "$test_output"

    # 生成覆盖率 HTML
    if [ -f "$coverage_file" ]; then
        go tool cover -html="$coverage_file" -o "$coverage_html" 2>/dev/null || true
        local coverage_pct
        coverage_pct=$(go tool cover -func="$coverage_file" 2>/dev/null | tail -1 | awk '{print $NF}')
    fi

    # 生成 Markdown 报告
    cat > "$report_file" << REPORT
# 单元测试报告

**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')
**项目**: CEX 风控系统
**Go 版本**: $(go version | awk '{print $3}')
**测试命令**: \`go test ./... -v -count=1\`

## 测试结果概览

| 指标 | 数值 |
|------|------|
| 总用例数 | ${total} |
| 通过 | ${passed} ✅ |
| 失败 | ${failed} ❌ |
| 跳过 | ${skipped} ⏭️ |
| 覆盖率 | ${coverage_pct:-N/A} |
| 退出码 | ${exit_code} |

## 测试状态: $([ $exit_code -eq 0 ] && echo "✅ 全部通过" || echo "❌ 存在失败")
REPORT

    if [ -n "$failed_tests" ]; then
        cat >> "$report_file" << REPORT

## 失败的测试

$(echo -e "$failed_tests")
REPORT
    fi

    cat >> "$report_file" << REPORT

## 覆盖率

- 总覆盖率: ${coverage_pct:-N/A}
- 详细报告: [coverage_${TIMESTAMP}.html](coverage_${TIMESTAMP}.html)

## 测试详情

\`\`\`
$(go test ./... -v -count=1 2>&1 | head -200)
\`\`\`
REPORT

    # 生成 JSON 摘要
    cat > "$json_report" << JSON
{
  "type": "unit_test",
  "timestamp": "$(date -Iseconds)",
  "project": "cex-risk",
  "go_version": "$(go version | awk '{print $3}')",
  "results": {
    "total": $total,
    "passed": $passed,
    "failed": $failed,
    "skipped": $skipped,
    "coverage": "${coverage_pct:-0%}",
    "exit_code": $exit_code
  },
  "report_file": "$report_file",
  "coverage_html": "$coverage_html"
}
JSON

    echo ""
    log_ok "单元测试完成"
    echo -e "  总计: ${total} | ${GREEN}通过: ${passed}${NC} | ${RED}失败: ${failed}${NC} | 跳过: ${skipped}"
    echo -e "  覆盖率: ${coverage_pct:-N/A}"
    echo -e "  报告: ${report_file}"
    echo ""

    return $exit_code
}

# ---------------------------------------------------------------
# 集成测试
# ---------------------------------------------------------------
run_integration_tests() {
    local report_file="${REPORTS_DIR}/integration_test_${TIMESTAMP}.md"

    log_step "运行集成测试..."
    cd "$PROJECT_ROOT"

    # 检查中间件
    log_info "检查中间件连通性..."
    local infra_ok=true
    docker exec cex-risk-mysql mysqladmin ping -uroot -p9Cn50al8W4F7dooaNIhv 2>/dev/null | grep -q alive || { log_error "MySQL 不可用"; infra_ok=false; }
    docker exec cex-risk-redis redis-cli ping 2>/dev/null | grep -q PONG || { log_error "Redis 不可用"; infra_ok=false; }

    if [ "$infra_ok" = false ]; then
        log_error "中间件未就绪，请先执行: make infra-up"
        return 1
    fi

    # 运行集成测试
    local test_output
    local exit_code=0
    test_output=$(CONFIG_PATH="${PROJECT_ROOT}/config.toml" go test ./tests/... -v -count=1 -tags=integration 2>&1) || exit_code=$?

    # 生成报告
    cat > "$report_file" << REPORT
# 集成测试报告

**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')
**测试命令**: \`go test ./tests/... -v -count=1 -tags=integration\`

## 测试状态: $([ $exit_code -eq 0 ] && echo "✅ 全部通过" || echo "❌ 存在失败")

## 中间件状态

| 服务 | 状态 |
|------|------|
| MySQL | $(docker exec cex-risk-mysql mysqladmin ping -uroot -p9Cn50al8W4F7dooaNIhv 2>/dev/null | grep -q alive && echo "✅" || echo "❌") |
| Redis | $(docker exec cex-risk-redis redis-cli ping 2>/dev/null | grep -q PONG && echo "✅" || echo "❌") |
| Kafka | $(docker exec cex-risk-kafka /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 && echo "✅" || echo "❌") |
| ClickHouse | $(docker exec cex-risk-clickhouse clickhouse-client --query "SELECT 1" >/dev/null 2>&1 && echo "✅" || echo "❌") |

## 测试输出

\`\`\`
${test_output}
\`\`\`
REPORT

    log_ok "集成测试报告: ${report_file}"
    return $exit_code
}

# ---------------------------------------------------------------
# 压力测试
# ---------------------------------------------------------------
run_benchmark_tests() {
    local report_file="${REPORTS_DIR}/benchmark_${TIMESTAMP}.md"
    local bench_output_file="${REPORTS_DIR}/benchmark_${TIMESTAMP}.txt"

    log_step "运行压力测试..."
    cd "$PROJECT_ROOT"

    local exit_code=0

    # 运行 Go benchmark
    local bench_output
    bench_output=$(go test ./... -bench=. -benchmem -benchtime=10s -run=^$ -count=3 2>&1) || exit_code=$?

    echo "$bench_output" > "$bench_output_file"

    # 生成报告
    cat > "$report_file" << REPORT
# 压力测试报告

**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')
**测试命令**: \`go test ./... -bench=. -benchmem -benchtime=10s -count=3\`

## 系统信息

| 指标 | 值 |
|------|---|
| OS | $(uname -s) $(uname -r) |
| CPU | $(sysctl -n machdep.cpu.brand_string 2>/dev/null || nproc) |
| 内存 | $(sysctl -n hw.memsize 2>/dev/null | awk '{print $1/1024/1024/1024 " GB"}' || free -h 2>/dev/null | awk '/^Mem:/{print $2}') |
| Go | $(go version | awk '{print $3}') |

## Benchmark 结果

\`\`\`
${bench_output}
\`\`\`

## 性能基准

> 以下为项目性能目标，用于与实测结果对比：

| 指标 | 目标 | 实测 |
|------|------|------|
| 单轮规则检测延迟（10 账户） | < 100ms | _待填入_ |
| Kafka 消息发布 QPS | > 10,000/s | _待填入_ |
| Redis 读写延迟 | < 1ms | _待填入_ |
| ClickHouse 批量写入 | > 5,000 rows/s | _待填入_ |

## 原始数据

详见: [benchmark_${TIMESTAMP}.txt](benchmark_${TIMESTAMP}.txt)
REPORT

    log_ok "压力测试报告: ${report_file}"
    return $exit_code
}

# ---------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------
main() {
    local type="${1:-help}"

    case "$type" in
        unit)
            run_unit_tests
            ;;
        integration)
            run_integration_tests
            ;;
        benchmark)
            run_benchmark_tests
            ;;
        all)
            log_step "运行全部测试..."
            local total_exit=0
            run_unit_tests || total_exit=1
            run_integration_tests || total_exit=1
            run_benchmark_tests || total_exit=1

            if [ $total_exit -eq 0 ]; then
                log_ok "全部测试通过！"
            else
                log_error "部分测试失败，请查看报告"
            fi
            exit $total_exit
            ;;
        help|--help|-h)
            echo ""
            echo "CEX 风控系统 — 测试报告生成器"
            echo ""
            echo "用法: ./test-report.sh <type>"
            echo ""
            echo "类型:"
            echo "  unit          单元测试 + 覆盖率"
            echo "  integration   集成测试（需中间件）"
            echo "  benchmark     压力测试"
            echo "  all           全部测试"
            echo ""
            echo "报告输出到: tests/reports/"
            echo ""
            ;;
        *)
            log_error "未知类型: $type"
            exit 1
            ;;
    esac
}

main "$@"
