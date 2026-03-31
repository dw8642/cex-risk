#!/bin/bash
# ==============================================================
# CEX 风控系统 — CI 辅助脚本
#
# 功能：代码推送、远程仓库管理、文档生成
# 用法：./ci-helper.sh <command> [options]
#
# 命令：
#   push-all        推送所有 Agent 分支 + main 到远程
#   push <agent>    推送单个 Agent 分支
#   setup-remote    配置 GitHub 远程仓库
#   gen-docs        从代码注释生成 API 文档
#   changelog <tag> 从 git log 生成 CHANGELOG
#   pre-commit      提交前检查（编译+测试+lint）
# ==============================================================

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
WORKTREE_BASE="${WORKTREE_BASE:-$HOME/Documents/Claude/projects/cexRisk}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

upper() { echo "$1" | tr '[:lower:]' '[:upper:]'; }

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}   $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERR]${NC}  $*"; }
log_step()  { echo -e "${CYAN}[STEP]${NC} $*"; }

# ---------------------------------------------------------------
# push-all: 推送所有分支到远程
# ---------------------------------------------------------------
cmd_push_all() {
    log_step "推送所有分支到远程..."

    cd "$PROJECT_ROOT"

    # 推送 main
    log_info "推送 main..."
    git push origin main 2>/dev/null && log_ok "main 推送成功" || log_error "main 推送失败"

    # 推送各 Agent 分支
    for agent in a b c d e; do
        local branch="agent-${agent}/current"
        local wt_dir="${WORKTREE_BASE}/cex-risk-agent-${agent}"

        if [ ! -d "$wt_dir" ]; then
            continue
        fi

        log_info "推送 Agent-$(upper "$agent") (${branch})..."
        cd "$wt_dir"
        git push origin "$branch" 2>/dev/null && log_ok "Agent-$(upper "$agent") 推送成功" || log_error "Agent-$(upper "$agent") 推送失败"
    done

    cd "$PROJECT_ROOT"

    # 推送 tags
    log_info "推送 tags..."
    git push --tags 2>/dev/null && log_ok "tags 推送成功" || log_error "tags 推送失败"

    echo ""
    log_ok "全部推送完成"
}

# ---------------------------------------------------------------
# push: 推送单个 Agent
# ---------------------------------------------------------------
cmd_push() {
    local agent="${1:-}"
    if [ -z "$agent" ]; then
        log_error "用法: ./ci-helper.sh push <agent>  (例: a, b, c, main)"
        exit 1
    fi

    if [ "$agent" = "main" ]; then
        cd "$PROJECT_ROOT"
        git push origin main
        log_ok "main 推送成功"
        return
    fi

    local wt_dir="${WORKTREE_BASE}/cex-risk-agent-${agent}"
    local branch="agent-${agent}/current"

    if [ ! -d "$wt_dir" ]; then
        log_error "Agent-$(upper "$agent") worktree 不存在"
        exit 1
    fi

    cd "$wt_dir"
    git push origin "$branch"
    log_ok "Agent-$(upper "$agent") 推送成功"
}

# ---------------------------------------------------------------
# setup-remote: 配置远程仓库
# ---------------------------------------------------------------
cmd_setup_remote() {
    local remote_url="${1:-}"
    if [ -z "$remote_url" ]; then
        echo ""
        echo "配置 GitHub 远程仓库"
        echo ""
        echo "用法: ./ci-helper.sh setup-remote <github-url>"
        echo ""
        echo "示例:"
        echo "  ./ci-helper.sh setup-remote git@github.com:yourname/cex-risk.git"
        echo "  ./ci-helper.sh setup-remote https://github.com/yourname/cex-risk.git"
        echo ""
        echo "准备步骤："
        echo "  1. 在 GitHub 创建新的 private repo（不要初始化 README）"
        echo "  2. 运行此命令配置 remote"
        echo "  3. 执行 ./ci-helper.sh push-all 首次推送"
        echo ""
        exit 1
    fi

    cd "$PROJECT_ROOT"

    # 检查是否已有 remote
    if git remote get-url origin 2>/dev/null; then
        log_warn "remote origin 已存在，更新为新地址..."
        git remote set-url origin "$remote_url"
    else
        git remote add origin "$remote_url"
    fi

    log_ok "远程仓库已配置: $remote_url"
    echo ""
    log_info "下一步: ./ci-helper.sh push-all  # 首次推送"
}

# ---------------------------------------------------------------
# gen-docs: 生成文档
# ---------------------------------------------------------------
cmd_gen_docs() {
    log_step "生成开发文档..."
    cd "$PROJECT_ROOT"
    mkdir -p docs

    # 1. 生成 Go 包文档
    log_info "生成 Go 包文档..."
    local go_doc_file="docs/go-packages.md"

    cat > "$go_doc_file" << 'HEADER'
# Go 包文档

> 自动从代码注释生成

HEADER

    # 遍历所有包
    for pkg_dir in pkg/config pkg/models pkg/store pkg/mq pkg/exchange pkg/logger; do
        if [ -d "$pkg_dir" ]; then
            local pkg_name
            pkg_name=$(basename "$pkg_dir")
            echo "## pkg/${pkg_name}" >> "$go_doc_file"
            echo "" >> "$go_doc_file"
            echo '```' >> "$go_doc_file"
            go doc "./${pkg_dir}/..." 2>/dev/null >> "$go_doc_file" || echo "(无导出符号)" >> "$go_doc_file"
            echo '```' >> "$go_doc_file"
            echo "" >> "$go_doc_file"
        fi
    done

    log_ok "Go 包文档: ${go_doc_file}"

    # 2. 生成项目结构文档
    log_info "生成项目结构文档..."
    local struct_file="docs/project-structure.md"

    cat > "$struct_file" << HEADER
# 项目结构

> 自动生成于 $(date '+%Y-%m-%d %H:%M:%S')

\`\`\`
$(find . -type f -name '*.go' | sort | sed 's|./||')
\`\`\`

## 文件统计

| 类型 | 文件数 | 代码行数 |
|------|--------|----------|
| Go 源码 | $(find . -name '*.go' ! -name '*_test.go' | wc -l | tr -d ' ') | $(find . -name '*.go' ! -name '*_test.go' -exec cat {} + 2>/dev/null | wc -l | tr -d ' ') |
| Go 测试 | $(find . -name '*_test.go' | wc -l | tr -d ' ') | $(find . -name '*_test.go' -exec cat {} + 2>/dev/null | wc -l | tr -d ' ') |
| SQL 脚本 | $(find . -name '*.sql' | wc -l | tr -d ' ') | $(find . -name '*.sql' -exec cat {} + 2>/dev/null | wc -l | tr -d ' ') |
| 配置文件 | $(find . -name '*.toml' -o -name '*.yml' -o -name '*.yaml' | wc -l | tr -d ' ') | - |
HEADER

    log_ok "项目结构: ${struct_file}"

    # 3. 生成 Agent 文件归属表
    log_info "生成 Agent 文件归属表..."
    local ownership_file="docs/file-ownership.md"

    cat > "$ownership_file" << 'HEADER'
# 文件归属表

> Agent 间文件所有权划分，避免并行开发冲突

| 路径 | 负责 Agent | 说明 |
|------|-----------|------|
| pkg/config/ | Agent-A | 配置加载 |
| pkg/models/ | Agent-A | 数据模型 |
| pkg/store/ | Agent-A | 存储层 |
| pkg/mq/ | Agent-A | Kafka 封装 |
| pkg/logger/ | Agent-A | 日志 |
| pkg/exchange/ | Agent-B | 交易所适配器 |
| services/exchange-ingestor/ | Agent-B | 数据采集 |
| services/risk-engine/ | Agent-C | 风控引擎 |
| services/api-server/ | Agent-A | API 服务 |
| services/telegram-notifier/ | Agent-A | 通知服务 |
| services/control-executor/ | Agent-A | 控制执行 |
| services/private-data-sink/ | Agent-A | 私有数据落盘 |
| services/watchdog/ | Agent-A | 看门狗 |
| frontend/ | Agent-D | 前端 |
| deploy/ | Agent-E | 部署 |
| tests/e2e/ | Agent-E | 端到端测试 |
| tests/benchmark/ | Agent-E | 压力测试 |
| scripts/ | Agent-A | 初始化脚本 |
| docs/ | 共享 | 文档（各 Agent 按职责写） |
HEADER

    log_ok "文件归属表: ${ownership_file}"
    echo ""
    log_ok "文档生成完成，共 3 个文件"
}

# ---------------------------------------------------------------
# changelog: 生成 CHANGELOG
# ---------------------------------------------------------------
cmd_changelog() {
    local tag="${1:-}"
    cd "$PROJECT_ROOT"

    local changelog_file="CHANGELOG.md"

    if [ -z "$tag" ]; then
        # 生成从最近 tag 到 HEAD 的 changelog
        local latest_tag
        latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
        if [ -z "$latest_tag" ]; then
            log_info "没有已有 tag，生成全部 commit 的 changelog"
            local range="HEAD"
        else
            log_info "从 ${latest_tag} 到 HEAD"
            local range="${latest_tag}..HEAD"
        fi
    else
        local range="$tag"
    fi

    log_step "生成 CHANGELOG..."

    cat > "$changelog_file" << HEADER
# CHANGELOG

> CEX 风控系统变更日志
> 生成时间: $(date '+%Y-%m-%d')

HEADER

    # 按 tag 分组
    git tag -l 'v*' --sort=-version:refname | while read -r vtag; do
        echo "## ${vtag}" >> "$changelog_file"
        echo "" >> "$changelog_file"

        # 获取 tag 日期
        local tag_date
        tag_date=$(git log -1 --format=%ai "$vtag" 2>/dev/null | cut -d' ' -f1)
        echo "**发布日期**: ${tag_date:-unknown}" >> "$changelog_file"
        echo "" >> "$changelog_file"

        # 获取前一个 tag
        local prev_tag
        prev_tag=$(git tag -l 'v*' --sort=-version:refname | grep -A1 "^${vtag}$" | tail -1)

        if [ "$prev_tag" = "$vtag" ]; then
            local log_range="$vtag"
        else
            local log_range="${prev_tag}..${vtag}"
        fi

        # 分类输出 commit
        local feat_commits fix_commits test_commits docs_commits other_commits
        feat_commits=$(git log --oneline "$log_range" --grep="^feat" 2>/dev/null || true)
        fix_commits=$(git log --oneline "$log_range" --grep="^fix" 2>/dev/null || true)
        test_commits=$(git log --oneline "$log_range" --grep="^test" 2>/dev/null || true)
        docs_commits=$(git log --oneline "$log_range" --grep="^docs" 2>/dev/null || true)

        if [ -n "$feat_commits" ]; then
            echo "### 新功能" >> "$changelog_file"
            echo "$feat_commits" | sed 's/^/- /' >> "$changelog_file"
            echo "" >> "$changelog_file"
        fi

        if [ -n "$fix_commits" ]; then
            echo "### 修复" >> "$changelog_file"
            echo "$fix_commits" | sed 's/^/- /' >> "$changelog_file"
            echo "" >> "$changelog_file"
        fi

        if [ -n "$test_commits" ]; then
            echo "### 测试" >> "$changelog_file"
            echo "$test_commits" | sed 's/^/- /' >> "$changelog_file"
            echo "" >> "$changelog_file"
        fi

        echo "---" >> "$changelog_file"
        echo "" >> "$changelog_file"
    done

    # 如果没有 tag，输出全部 commit
    if [ -z "$(git tag -l 'v*')" ]; then
        echo "## Unreleased" >> "$changelog_file"
        echo "" >> "$changelog_file"
        git log --oneline | sed 's/^/- /' >> "$changelog_file"
    fi

    log_ok "CHANGELOG 已生成: ${changelog_file}"
}

# ---------------------------------------------------------------
# pre-commit: 提交前检查
# ---------------------------------------------------------------
cmd_pre_commit() {
    log_step "提交前检查..."
    cd "$PROJECT_ROOT"

    local exit_code=0

    # 1. 编译检查
    echo -n "  [1/4] go build ./... "
    if go build ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
        exit_code=1
    fi

    # 2. 单元测试
    echo -n "  [2/4] go test ./...  "
    if go test ./... -count=1 -short 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
        exit_code=1
    fi

    # 3. go vet
    echo -n "  [3/4] go vet ./...   "
    if go vet ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
        exit_code=1
    fi

    # 4. gofmt 检查
    echo -n "  [4/4] gofmt check    "
    local unformatted
    unformatted=$(gofmt -l . 2>/dev/null | grep -v vendor | head -5)
    if [ -z "$unformatted" ]; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
        echo "    需要格式化的文件:"
        echo "$unformatted" | sed 's/^/      /'
        exit_code=1
    fi

    echo ""
    if [ $exit_code -eq 0 ]; then
        log_ok "所有检查通过，可以提交"
    else
        log_error "检查未通过，请修复后再提交"
    fi

    return $exit_code
}

# ---------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------
main() {
    local cmd="${1:-help}"
    shift 2>/dev/null || true

    case "$cmd" in
        push-all)     cmd_push_all ;;
        push)         cmd_push "$@" ;;
        setup-remote) cmd_setup_remote "$@" ;;
        gen-docs)     cmd_gen_docs ;;
        changelog)    cmd_changelog "$@" ;;
        pre-commit)   cmd_pre_commit ;;
        help|--help|-h)
            echo ""
            echo "CEX 风控系统 — CI 辅助脚本"
            echo ""
            echo "用法: ./ci-helper.sh <command> [options]"
            echo ""
            echo "命令:"
            echo "  push-all              推送所有分支 + tags 到远程"
            echo "  push <agent|main>     推送单个分支"
            echo "  setup-remote <url>    配置 GitHub 远程仓库"
            echo "  gen-docs              从代码生成文档"
            echo "  changelog [tag]       生成 CHANGELOG"
            echo "  pre-commit            提交前检查"
            echo ""
            ;;
        *)
            log_error "未知命令: $cmd"
            exit 1
            ;;
    esac
}

main "$@"
