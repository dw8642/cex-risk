#!/bin/bash
# ==============================================================
# CEX 风控系统 — 多 Agent 并行开发调度器（Orchestrator）
#
# 功能：作为"大脑"统一调度多个 Claude Code Agent 的开发任务
# 用法：./orchestrator.sh <command> [options]
#
# 命令：
#   init          初始化 worktree 和 Agent 环境
#   plan <day>    显示某天的任务计划和依赖图
#   run <day>     执行某天的全部任务（按依赖顺序调度 Agent）
#   run-task <id> 执行单个任务（如 D1-A1）
#   status        显示当前所有 Agent 的工作状态
#   merge <agent> 合并某个 Agent 的分支到 main
#   sync          同步 main 到所有 Agent worktree
#   merge-all     合并所有 Agent 到 main（每日收工）
#   health        检查中间件和服务健康状态
#   report <day>  生成某天的工作报告
#   gate <day>    执行某天的验收检查
# ==============================================================

set -euo pipefail

# ---------------------------------------------------------------
# 配置区 — 根据你的实际路径修改
# ---------------------------------------------------------------
PROJECT_ROOT="${PROJECT_ROOT:-$HOME/Documents/Claude/projects/cexRisk/cex-risk}"
WORKTREE_BASE="${WORKTREE_BASE:-$HOME/Documents/Claude/projects/cexRisk}"
ORCHESTRATOR_DIR="$(cd "$(dirname "$0")" && pwd)"
TASKS_FILE="${ORCHESTRATOR_DIR}/tasks.json"
LOG_DIR="${ORCHESTRATOR_DIR}/logs"
REPORTS_DIR="${PROJECT_ROOT}/tests/reports"
STATE_FILE="${ORCHESTRATOR_DIR}/.state.json"

# Agent 列表
AGENTS=(a b c d e)
AGENT_NAMES=("Agent-A" "Agent-B" "Agent-C" "Agent-D" "Agent-E")
AGENT_ROLES=("Backend Core" "Exchange" "Risk Engine" "Frontend" "Infra/DevOps")

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
NC='\033[0m'  # 无颜色

# ---------------------------------------------------------------
# 兼容 macOS bash 3.2：大写转换函数
# ---------------------------------------------------------------
upper() { echo "$1" | tr '[:lower:]' '[:upper:]'; }

# ---------------------------------------------------------------
# 工具函数
# ---------------------------------------------------------------

# 打印带颜色的日志
log_info()  { echo -e "${BLUE}[INFO]${NC} $(date '+%H:%M:%S') $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}   $(date '+%H:%M:%S') $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $(date '+%H:%M:%S') $*"; }
log_error() { echo -e "${RED}[ERR]${NC}  $(date '+%H:%M:%S') $*"; }
log_step()  { echo -e "${PURPLE}[STEP]${NC} $(date '+%H:%M:%S') $*"; }

# 确保依赖工具存在
check_deps() {
    local missing=()
    for cmd in git jq claude; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [ ${#missing[@]} -gt 0 ]; then
        log_error "缺少依赖工具: ${missing[*]}"
        log_info "请安装: brew install ${missing[*]}"
        exit 1
    fi
}

# 确保目录存在
ensure_dirs() {
    mkdir -p "$LOG_DIR" "$REPORTS_DIR"
}

# 自动提交 main 分支上的脏文件（避免合并冲突）
auto_commit_main() {
    cd "$PROJECT_ROOT"
    if [ -n "$(git status --porcelain)" ]; then
        log_info "main 分支有未提交的改动，自动提交..."
        git add -A
        git commit -m "chore: orchestrator 自动提交 main 脏文件 $(date '+%Y%m%d_%H%M%S')" 2>/dev/null || true
        log_ok "main 脏文件已提交"
    fi
}

# 获取 Agent 的 worktree 路径
agent_dir() {
    local agent=$1
    echo "${WORKTREE_BASE}/cex-risk-agent-${agent}"
}

# 读取任务定义（需要 jq）
read_task() {
    local task_id=$1
    local field=$2
    jq -r ".days[].tasks[] | select(.id == \"${task_id}\") | .${field}" "$TASKS_FILE"
}

# 读取当天任务列表
read_day_tasks() {
    local day=$1
    jq -r ".days.${day}.tasks[].id" "$TASKS_FILE" 2>/dev/null
}

# 读取当天信息
read_day_info() {
    local day=$1
    local field=$2
    jq -r ".days.${day}.${field}" "$TASKS_FILE" 2>/dev/null
}

# 更新状态文件
update_state() {
    local task_id=$1
    local status=$2  # pending / running / done / failed
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    # 初始化状态文件
    if [ ! -f "$STATE_FILE" ]; then
        echo '{"tasks":{}}' > "$STATE_FILE"
    fi

    # 更新任务状态
    local tmp
    tmp=$(mktemp)
    jq ".tasks.\"${task_id}\" = {\"status\": \"${status}\", \"updated\": \"${timestamp}\"}" \
        "$STATE_FILE" > "$tmp" && mv "$tmp" "$STATE_FILE"
}

# 读取任务状态
get_state() {
    local task_id=$1
    if [ -f "$STATE_FILE" ]; then
        jq -r ".tasks.\"${task_id}\".status // \"pending\"" "$STATE_FILE"
    else
        echo "pending"
    fi
}

# ---------------------------------------------------------------
# 命令: init — 初始化 worktree 和 Agent 环境
# ---------------------------------------------------------------
cmd_init() {
    log_step "初始化多 Agent 开发环境..."

    # 检查项目根目录
    if [ ! -d "$PROJECT_ROOT/.git" ]; then
        log_error "项目根目录 $PROJECT_ROOT 不是 git 仓库"
        log_info "请先执行: cd $PROJECT_ROOT && git init && git add . && git commit -m 'initial'"
        exit 1
    fi

    cd "$PROJECT_ROOT"

    # 先自动提交脏文件
    auto_commit_main

    # 确保在 main 分支
    local current_branch
    current_branch=$(git branch --show-current)
    if [ "$current_branch" != "main" ]; then
        log_warn "当前分支是 $current_branch，切换到 main..."
        git checkout main 2>/dev/null || git checkout -b main
    fi

    # 为需要的 Agent 创建 worktree
    local agents_to_create=("${@:-a b}")  # 默认只创建 A 和 B（D1 够用）

    for agent in "${agents_to_create[@]}"; do
        local wt_dir
        wt_dir=$(agent_dir "$agent")
        local branch="agent-${agent}/current"

        if [ -d "$wt_dir" ]; then
            log_info "Agent-$(upper "$agent") worktree 已存在: $wt_dir"
        else
            log_step "创建 Agent-$(upper "$agent") worktree..."
            git worktree add "$wt_dir" -b "$branch" 2>/dev/null || {
                # 分支已存在，直接创建 worktree
                git worktree add "$wt_dir" "$branch" 2>/dev/null || {
                    log_error "创建 worktree 失败: $wt_dir"
                    continue
                }
            }
            log_ok "Agent-$(upper "$agent") worktree 创建成功: $wt_dir"
        fi

        # 创建 Agent 专属 CLAUDE.md 补充指令
        _create_agent_claude_md "$agent" "$wt_dir"
    done

    # 初始化状态文件
    echo '{"tasks":{}, "current_day": "D1", "started_at": "'$(date -Iseconds)'"}' > "$STATE_FILE"

    echo ""
    log_ok "初始化完成！当前 worktree 状态："
    git worktree list
    echo ""
    log_info "下一步：./orchestrator.sh plan D1  # 查看 D1 任务计划"
    log_info "       ./orchestrator.sh run D1   # 启动 D1 任务"
}

# 创建 Agent 专属 CLAUDE.md
_create_agent_claude_md() {
    local agent=$1
    local wt_dir=$2
    local claude_md="${wt_dir}/CLAUDE.md"

    # 如果已有 CLAUDE.md 且包含 Agent 指令，跳过
    if [ -f "$claude_md" ] && grep -q "Agent-$(upper "$agent") 专属指令" "$claude_md" 2>/dev/null; then
        return
    fi

    case "$agent" in
        a)
            cat >> "$claude_md" << 'AGENT_EOF'

## Agent-A 专属指令
你是 Agent-A（Backend Core），负责：
- pkg/ 下所有公共包（config, models, store, mq, logger）
- cmd/ 下的服务入口
- services/api-server/、services/telegram-notifier/
- services/control-executor/、services/private-data-sink/、services/watchdog/
- scripts/ 下的 SQL 初始化脚本

### 你的文件归属（只能修改这些）
- pkg/**、cmd/**
- services/api-server/**、services/telegram-notifier/**
- services/control-executor/**、services/private-data-sink/**、services/watchdog/**
- scripts/**、config.toml

### 禁止修改
- services/exchange-ingestor/**（Agent-B）
- services/risk-engine/**（Agent-C）
- frontend/**（Agent-D）
- deploy/**（Agent-E）
AGENT_EOF
            ;;
        b)
            cat >> "$claude_md" << 'AGENT_EOF'

## Agent-B 专属指令
你是 Agent-B（Exchange），负责：
- pkg/exchange/ — ExchangeAdapter 接口及所有交易所实现
- services/exchange-ingestor/ — 数据采集服务全部逻辑

### 你的文件归属
- pkg/exchange/**、services/exchange-ingestor/**

### 你可以读但不能改的
- pkg/config/、pkg/models/、pkg/store/、pkg/mq/

### 禁止修改
- services/risk-engine/**、frontend/**
AGENT_EOF
            ;;
        c)
            cat >> "$claude_md" << 'AGENT_EOF'

## Agent-C 专属指令
你是 Agent-C（Risk Engine），负责：
- services/risk-engine/ 全部 — metrics、alert、rules

### 你的文件归属
- services/risk-engine/**

### 你可以读但不能改的
- pkg/**（需要扩展时告诉 Agent-A）
- services/exchange-ingestor/ 的接口定义

### 关键接口
- Rule interface 定义在 services/risk-engine/alert/rules/rule.go
- Metrics Engine 消费 Kafka exchange.* topics
- Alert Engine 产出 risk.events topic
AGENT_EOF
            ;;
        d)
            cat >> "$claude_md" << 'AGENT_EOF'

## Agent-D 专属指令
你是 Agent-D（Frontend），负责：
- frontend/ 全部 — Next.js 应用

### 你的文件归属
- frontend/**

### API 契约
- 后端 API: http://localhost:8080/api
- WebSocket: ws://localhost:8080/ws
- API Schema: docs/api-schema.md（Agent-A 维护）

### 技术选型
- Next.js 14 (App Router)、TypeScript strict、Tailwind CSS、SWR、recharts
AGENT_EOF
            ;;
        e)
            cat >> "$claude_md" << 'AGENT_EOF'

## Agent-E 专属指令
你是 Agent-E（Infra/DevOps），负责：
- deploy/ — Docker Compose、Dockerfile、Grafana、Prometheus
- 端到端测试、压测、安全审计
- 文档（docs/ 下的运维文档）

### 你的文件归属
- deploy/**
- docs/deploy.md、docs/ops-runbook.md、docs/testing-report.md
- tests/e2e/**、tests/benchmark/**

### 你可以读所有文件，但只能改上述目录
AGENT_EOF
            ;;
    esac
}

# ---------------------------------------------------------------
# 命令: plan — 显示某天的任务计划
# ---------------------------------------------------------------
cmd_plan() {
    local day="${1:-}"
    if [ -z "$day" ]; then
        log_error "用法: ./orchestrator.sh plan <day>  (例: D1, D2, D3)"
        exit 1
    fi

    local title goal gate
    title=$(read_day_info "$day" "title")
    goal=$(read_day_info "$day" "goal")
    gate=$(read_day_info "$day" "acceptance_gate")

    if [ "$title" = "null" ] || [ -z "$title" ]; then
        log_error "找不到 ${day} 的任务定义"
        exit 1
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  ${day} — ${title}${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "  ${YELLOW}目标${NC}: $goal"
    echo -e "  ${YELLOW}验收${NC}: $gate"
    echo ""

    # 列出任务
    echo -e "${PURPLE}  任务列表：${NC}"
    echo "  ─────────────────────────────────────────────────"

    local tasks
    tasks=$(jq -r ".days.${day}.tasks[] | [.id, .agent, .type, .phase, .title] | @tsv" "$TASKS_FILE" 2>/dev/null)

    while IFS=$'\t' read -r id agent type phase task_title; do
        local status
        status=$(get_state "$id")
        local status_icon
        case "$status" in
            done)    status_icon="${GREEN}✓${NC}" ;;
            running) status_icon="${YELLOW}⚙${NC}" ;;
            failed)  status_icon="${RED}✗${NC}" ;;
            *)       status_icon="○" ;;
        esac

        local type_icon
        case "$type" in
            blocker)      type_icon="🔒" ;;
            parallel)     type_icon="⚡" ;;
            half_day_dep) type_icon="⏳" ;;
            *)            type_icon="  " ;;
        esac

        local deps
        deps=$(jq -r ".days.${day}.tasks[] | select(.id == \"${id}\") | .depends_on | join(\", \")" "$TASKS_FILE" 2>/dev/null)

        printf "  %b %-8s ${type_icon} Agent-%-2s %-10s %s" "$status_icon" "$id" "$(upper "$agent")" "[$phase]" "$task_title"
        if [ -n "$deps" ] && [ "$deps" != "" ]; then
            echo -e "  ${RED}← 依赖: $deps${NC}"
        else
            echo ""
        fi
    done <<< "$tasks"

    echo ""

    # 显示合并顺序
    local merge_seq
    merge_seq=$(read_day_info "$day" "merge_sequence | join(\" → \")")
    if [ "$merge_seq" != "null" ] && [ -n "$merge_seq" ]; then
        echo -e "  ${YELLOW}合并顺序${NC}: $merge_seq"
    fi

    # 检查是否有 release
    local release_tag
    release_tag=$(read_day_info "$day" "release.tag")
    if [ "$release_tag" != "null" ] && [ -n "$release_tag" ]; then
        echo ""
        echo -e "  ${GREEN}🏷️  Release: $release_tag${NC}"
        echo -e "  ${YELLOW}验收清单：${NC}"
        jq -r ".days.${day}.release.checklist[]" "$TASKS_FILE" 2>/dev/null | while read -r item; do
            echo "    □ $item"
        done
    fi

    echo ""
}

# ---------------------------------------------------------------
# 命令: run — 执行某天的全部任务
# ---------------------------------------------------------------
cmd_run() {
    local day="${1:-}"
    if [ -z "$day" ]; then
        log_error "用法: ./orchestrator.sh run <day>"
        exit 1
    fi

    check_deps
    ensure_dirs

    # 先自动提交 main 上的脏文件，避免后续合并冲突
    auto_commit_main

    log_step "开始执行 ${day} 任务..."
    echo ""

    # 显示计划
    cmd_plan "$day"

    # 获取合并顺序（即执行顺序）
    local merge_seq
    merge_seq=$(jq -r ".days.${day}.merge_sequence[]" "$TASKS_FILE" 2>/dev/null)

    if [ -z "$merge_seq" ]; then
        log_error "${day} 没有定义任务"
        exit 1
    fi

    # 按合并顺序执行
    for task_id in $merge_seq; do
        local status
        status=$(get_state "$task_id")

        if [ "$status" = "done" ]; then
            log_ok "${task_id} 已完成，跳过"
            continue
        fi

        # 检查依赖
        local deps
        deps=$(jq -r ".days[].tasks[] | select(.id == \"${task_id}\") | .depends_on[]" "$TASKS_FILE" 2>/dev/null)

        local deps_met=true
        for dep in $deps; do
            local dep_status
            dep_status=$(get_state "$dep")
            if [ "$dep_status" != "done" ]; then
                log_warn "${task_id} 依赖 ${dep}（状态: ${dep_status}），等待中..."
                deps_met=false
                break
            fi
        done

        if [ "$deps_met" = false ]; then
            log_error "${task_id} 的依赖未满足，请先完成依赖任务"
            log_info "可以手动执行: ./orchestrator.sh run-task ${task_id}"
            continue
        fi

        # 执行任务
        if ! _execute_task "$task_id"; then
            local task_status
            task_status=$(get_state "$task_id")
            if [ "$task_status" = "rate_limited" ]; then
                log_error "检测到限流，暂停 ${day} 后续任务。限额重置后重新执行: ./orchestrator.sh run ${day}"
                return 1
            fi
            log_warn "${task_id} 执行失败，继续下一个任务..."
            continue
        fi

        # 任务完成后合并到 main
        local agent
        agent=$(read_task "$task_id" "agent")
        _merge_agent "$agent" "${task_id} 完成"

        echo ""
    done

    # 执行验收
    echo ""
    log_step "${day} 所有任务执行完毕，开始验收..."
    cmd_gate "$day"
}

# ---------------------------------------------------------------
# 命令: run-task — 执行单个任务
# ---------------------------------------------------------------
cmd_run_task() {
    local task_id="${1:-}"
    if [ -z "$task_id" ]; then
        log_error "用法: ./orchestrator.sh run-task <task_id>  (例: D1-A1)"
        exit 1
    fi

    check_deps
    ensure_dirs

    _execute_task "$task_id"
}

# 执行单个任务的内部函数
_execute_task() {
    local task_id=$1

    local agent title prompt acceptance
    agent=$(read_task "$task_id" "agent")
    title=$(read_task "$task_id" "title")
    prompt=$(read_task "$task_id" "prompt")
    acceptance=$(read_task "$task_id" "acceptance")

    if [ -z "$agent" ] || [ "$agent" = "null" ]; then
        log_error "找不到任务: ${task_id}"
        return 1
    fi

    local wt_dir
    wt_dir=$(agent_dir "$agent")

    # 确认 worktree 存在
    if [ ! -d "$wt_dir" ]; then
        log_error "Agent-$(upper "$agent") 的 worktree 不存在: $wt_dir"
        log_info "请先执行: ./orchestrator.sh init ${agent}"
        return 1
    fi

    # 同步 main 到 worktree
    log_info "同步 main 到 Agent-$(upper "$agent") worktree..."
    cd "$wt_dir"

    # 先提交 worktree 里的脏文件
    if [ -n "$(git status --porcelain)" ]; then
        git add -A
        git commit -m "chore: Agent-$(upper "$agent") 自动提交未保存改动" 2>/dev/null || true
    fi

    git fetch origin 2>/dev/null || true
    if ! git rebase main 2>/dev/null; then
        git rebase --abort 2>/dev/null
        log_warn "rebase 有冲突，尝试 merge..."
        if ! git merge main --no-edit 2>/dev/null; then
            # 自动解决冲突：优先保留 agent 自己的版本
            local conflict_files
            conflict_files=$(git diff --name-only --diff-filter=U 2>/dev/null || true)
            if [ -n "$conflict_files" ]; then
                while IFS= read -r f; do
                    [ -n "$f" ] && git checkout --ours "$f" 2>/dev/null && git add "$f" 2>/dev/null
                done <<< "$conflict_files"
                git commit --no-edit 2>/dev/null || true
                log_ok "同步冲突已自动解决（保留 Agent 本地版本）"
            fi
        fi
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  执行任务: ${task_id} — ${title}${NC}"
    echo -e "${CYAN}  Agent: Agent-$(upper "$agent") | 目录: ${wt_dir}${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo ""

    update_state "$task_id" "running"

    # 构造完整的 prompt（附加开发规范提醒）
    local full_prompt
    full_prompt="${prompt}

---
完成后请执行以下检查：
1. go build ./... 确认编译通过
2. go test ./... -v 确认测试通过
3. 用 git add 和 git commit 提交你的改动，commit message 格式: feat|fix|test|docs(模块): 中文描述
4. 如果有新模块，确认已写对应的 _test.go 单元测试

验收标准: ${acceptance}"

    # 记录日志
    local log_file="${LOG_DIR}/${task_id}_$(date '+%Y%m%d_%H%M%S').log"

    # 启动 Claude Code Agent
    log_step "启动 Agent-$(upper "$agent")..."
    log_info "日志: ${log_file}"
    echo ""

    cd "$wt_dir"

    # 使用 claude -p 非交互模式执行任务
    # --dangerously-skip-permissions: Agent 自动化开发需要跳过逐次权限确认
    local prompt_file="${LOG_DIR}/.prompt_${task_id}.txt"
    printf '%s\n' "$full_prompt" > "$prompt_file"

    log_info "Prompt 已写入: ${prompt_file}"
    log_info "开始执行 claude CLI..."

    # 先输出到日志文件，再打印到终端（避免 tee 管道吞掉输出）
    local exit_code=0
    claude --dangerously-skip-permissions -p "$(cat "$prompt_file")" > "$log_file" 2>&1 || exit_code=$?

    # 打印日志到终端
    if [ -s "$log_file" ]; then
        cat "$log_file"
    else
        log_warn "日志为空，尝试 stdin 管道方式..."
        claude --dangerously-skip-permissions -p < "$prompt_file" > "$log_file" 2>&1 || exit_code=$?
        [ -s "$log_file" ] && cat "$log_file"
    fi

    rm -f "$prompt_file"

    # 检测限流：日志为空或包含限流关键词
    if [ -s "$log_file" ] && grep -qi "hit your limit\|rate.limit\|resets.*pm\|resets.*am\|quota.*exceeded" "$log_file" 2>/dev/null; then
        log_error "检测到 Claude API 限流！任务 ${task_id} 标记为 rate_limited"
        log_info "请等待限额重置后重新执行: ./orchestrator.sh run-task ${task_id}"
        update_state "$task_id" "rate_limited"
        return 1
    fi

    if [ "$exit_code" -eq 0 ] && [ -s "$log_file" ]; then
        log_ok "任务 ${task_id} 执行完成"
        update_state "$task_id" "done"

        # 自动提交（如果 Agent 没有提交的话）
        if [ -n "$(git status --porcelain)" ]; then
            log_info "Agent 有未提交的改动，自动提交..."
            git add -A
            git commit -m "feat: ${task_id} ${title}" 2>/dev/null || true
        fi

        # 推送到远程
        git push origin "agent-${agent}/current" 2>/dev/null || {
            log_warn "推送到远程失败（可能没有配置 remote），本地 commit 已保存"
        }
    elif [ ! -s "$log_file" ]; then
        log_error "任务 ${task_id} 日志为空（可能限流或 CLI 异常）"
        log_info "标记为 rate_limited，请等待后重试: ./orchestrator.sh run-task ${task_id}"
        update_state "$task_id" "rate_limited"
        return 1
    else
        log_error "任务 ${task_id} 执行失败（exit_code=${exit_code}）"
        update_state "$task_id" "failed"
        return 1
    fi
}

# ---------------------------------------------------------------
# 命令: merge — 合并 Agent 到 main
# ---------------------------------------------------------------
cmd_merge() {
    local agent="${1:-}"
    if [ -z "$agent" ]; then
        log_error "用法: ./orchestrator.sh merge <agent>  (例: a, b, c)"
        exit 1
    fi
    _merge_agent "$agent" "手动合并"
}

_merge_agent() {
    local agent=$1
    local msg="${2:-merge}"

    cd "$PROJECT_ROOT"
    local branch="agent-${agent}/current"

    log_step "合并 Agent-$(upper "$agent") (${branch}) 到 main..."

    # 检查分支是否有新 commit
    local diff_count
    diff_count=$(git rev-list --count main.."$branch" 2>/dev/null || echo "0")

    if [ "$diff_count" = "0" ]; then
        log_info "Agent-$(upper "$agent") 没有新的 commit，跳过合并"
        return 0
    fi

    # 先确保 main 干净
    auto_commit_main

    # 合并
    if git merge "$branch" --no-ff -m "merge: Agent-$(upper "$agent") — ${msg}" 2>/dev/null; then
        log_ok "Agent-$(upper "$agent") 合并成功 (${diff_count} commits)"
    else
        log_warn "合并冲突，自动解决中（优先采用 Agent 的改动）..."

        # 获取冲突文件列表
        local conflict_files
        conflict_files=$(git diff --name-only --diff-filter=U 2>/dev/null || true)

        if [ -n "$conflict_files" ]; then
            # 对每个冲突文件，优先采用 agent 分支的版本
            while IFS= read -r f; do
                if [ -n "$f" ]; then
                    git checkout --theirs "$f" 2>/dev/null && git add "$f" 2>/dev/null
                    log_info "  冲突自动解决: $f （采用 Agent-$(upper "$agent") 版本）"
                fi
            done <<< "$conflict_files"

            git commit --no-edit 2>/dev/null || {
                # 如果还有问题，abort 并报错
                git merge --abort 2>/dev/null
                log_error "自动解决失败，请手动处理："
                log_info "  cd $PROJECT_ROOT && git merge $branch"
                return 1
            }
            log_ok "Agent-$(upper "$agent") 合并成功（自动解决了冲突）"
        else
            # 没有冲突文件但 merge 报错，可能是其他问题
            git merge --abort 2>/dev/null
            log_error "合并失败（非冲突原因），请手动检查"
            return 1
        fi
    fi
}

# ---------------------------------------------------------------
# 命令: sync — 同步 main 到所有 worktree
# ---------------------------------------------------------------
cmd_sync() {
    log_step "同步 main 到所有 Agent worktree..."

    # 先确保 main 干净
    auto_commit_main

    for agent in "${AGENTS[@]}"; do
        local wt_dir
        wt_dir=$(agent_dir "$agent")

        if [ ! -d "$wt_dir" ]; then
            continue
        fi

        cd "$wt_dir"

        # 先提交 worktree 的脏文件
        if [ -n "$(git status --porcelain)" ]; then
            git add -A
            git commit -m "chore: Agent-$(upper "$agent") sync 前自动提交" 2>/dev/null || true
        fi

        if git rebase main 2>/dev/null; then
            log_ok "Agent-$(upper "$agent") 同步成功"
        else
            git rebase --abort 2>/dev/null
            log_warn "Agent-$(upper "$agent") rebase 冲突，尝试 merge..."
            if ! git merge main --no-edit 2>/dev/null; then
                # 自动解决：保留 agent 本地版本
                local conflict_files
                conflict_files=$(git diff --name-only --diff-filter=U 2>/dev/null || true)
                if [ -n "$conflict_files" ]; then
                    while IFS= read -r f; do
                        [ -n "$f" ] && git checkout --ours "$f" 2>/dev/null && git add "$f" 2>/dev/null
                    done <<< "$conflict_files"
                    git commit --no-edit 2>/dev/null || true
                    log_ok "Agent-$(upper "$agent") 冲突已自动解决"
                else
                    log_error "Agent-$(upper "$agent") 合并失败，需手动处理"
                fi
            else
                log_ok "Agent-$(upper "$agent") 同步成功"
            fi
        fi
    done
}

# ---------------------------------------------------------------
# 命令: merge-all — 每日收工：合并所有 Agent
# ---------------------------------------------------------------
cmd_merge_all() {
    local day="${1:-}"
    log_step "每日收工合并 — ${day:-unknown day}..."

    # 先确保 main 干净
    auto_commit_main

    # 先让每个 Agent 提交
    for agent in "${AGENTS[@]}"; do
        local wt_dir
        wt_dir=$(agent_dir "$agent")

        if [ ! -d "$wt_dir" ]; then
            continue
        fi

        cd "$wt_dir"
        if [ -n "$(git status --porcelain)" ]; then
            git add -A
            git commit -m "daily: Agent-$(upper "$agent") ${day} progress" 2>/dev/null || true
        fi
    done

    # 按顺序合并到 main
    cd "$PROJECT_ROOT"
    for agent in "${AGENTS[@]}"; do
        local wt_dir
        wt_dir=$(agent_dir "$agent")
        [ ! -d "$wt_dir" ] && continue

        _merge_agent "$agent" "${day} daily merge" || true
    done

    # 推送 main 到远程
    log_step "推送 main 到远程..."
    git push origin main 2>/dev/null || {
        log_warn "推送失败（检查 remote 配置）"
    }

    # 同步回所有 worktree
    cmd_sync

    log_ok "每日合并完成"
}

# ---------------------------------------------------------------
# 命令: status — 显示所有 Agent 状态
# ---------------------------------------------------------------
cmd_status() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  Agent 工作状态${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo ""

    cd "$PROJECT_ROOT"

    printf "  %-12s %-18s %-10s %-8s %s\n" "Agent" "Role" "Branch" "Commits" "Status"
    echo "  ──────────────────────────────────────────────────────────────"

    for i in "${!AGENTS[@]}"; do
        local agent="${AGENTS[$i]}"
        local name="${AGENT_NAMES[$i]}"
        local role="${AGENT_ROLES[$i]}"
        local wt_dir
        wt_dir=$(agent_dir "$agent")
        local branch="agent-${agent}/current"

        if [ ! -d "$wt_dir" ]; then
            printf "  %-12s %-18s %-10s %-8s %s\n" "$name" "$role" "-" "-" "未创建"
            continue
        fi

        local diff_count
        diff_count=$(git rev-list --count main.."$branch" 2>/dev/null || echo "?")

        local dirty=""
        cd "$wt_dir"
        if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
            dirty=" (有未提交改动)"
        fi
        cd "$PROJECT_ROOT"

        printf "  %-12s %-18s %-10s %-8s %s%s\n" "$name" "$role" "$branch" "+$diff_count" "活跃" "$dirty"
    done

    echo ""

    # 显示任务完成状态
    if [ -f "$STATE_FILE" ]; then
        echo -e "  ${YELLOW}任务进度：${NC}"
        local total done_count running failed
        total=$(jq '.tasks | length' "$STATE_FILE")
        done_count=$(jq '[.tasks[] | select(.status == "done")] | length' "$STATE_FILE")
        running=$(jq '[.tasks[] | select(.status == "running")] | length' "$STATE_FILE")
        failed=$(jq '[.tasks[] | select(.status == "failed")] | length' "$STATE_FILE")

        echo -e "  总计: $total | ${GREEN}完成: $done_count${NC} | ${YELLOW}进行中: $running${NC} | ${RED}失败: $failed${NC}"
    fi

    echo ""
}

# ---------------------------------------------------------------
# 命令: health — 检查中间件健康
# ---------------------------------------------------------------
cmd_health() {
    log_step "检查中间件健康状态..."

    cd "$PROJECT_ROOT"
    if [ -f "Makefile" ]; then
        make health
    else
        # 手动检查
        echo -n "MySQL:      " && docker exec cex-risk-mysql mysqladmin ping -uroot -p9Cn50al8W4F7dooaNIhv 2>/dev/null | grep -q alive && echo "✓ OK" || echo "✗ FAIL"
        echo -n "Redis:      " && docker exec cex-risk-redis redis-cli ping 2>/dev/null | grep -q PONG && echo "✓ OK" || echo "✗ FAIL"
        echo -n "Kafka:      " && docker exec cex-risk-kafka /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 && echo "✓ OK" || echo "✗ FAIL"
        echo -n "ClickHouse: " && docker exec cex-risk-clickhouse clickhouse-client --query "SELECT 1" >/dev/null 2>&1 && echo "✓ OK" || echo "✗ FAIL"
    fi
}

# ---------------------------------------------------------------
# 命令: gate — 验收检查
# ---------------------------------------------------------------
cmd_gate() {
    local day="${1:-}"
    if [ -z "$day" ]; then
        log_error "用法: ./orchestrator.sh gate <day>"
        exit 1
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  ${day} 验收检查${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo ""

    local gate
    gate=$(read_day_info "$day" "acceptance_gate")
    echo -e "  ${YELLOW}验收标准${NC}: $gate"
    echo ""

    cd "$PROJECT_ROOT"

    # 基础检查
    echo -e "  ${PURPLE}基础检查：${NC}"
    echo -n "  [1] go build ./... "
    if go build ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
    fi

    echo -n "  [2] go test ./...  "
    if go test ./... -count=1 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL (部分测试失败)${NC}"
    fi

    echo -n "  [3] go vet ./...   "
    if go vet ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
    fi

    # 中间件检查
    echo ""
    echo -e "  ${PURPLE}中间件：${NC}"
    cmd_health 2>/dev/null

    # Release 检查
    local release_tag
    release_tag=$(read_day_info "$day" "release.tag")
    if [ "$release_tag" != "null" ] && [ -n "$release_tag" ]; then
        echo ""
        echo -e "  ${GREEN}🏷️  Release 验收 — ${release_tag}${NC}"
        jq -r ".days.${day}.release.checklist[]" "$TASKS_FILE" 2>/dev/null | while read -r item; do
            echo "    □ $item"
        done
        echo ""
        echo -e "  ${YELLOW}请逐项手动确认后执行: git tag ${release_tag}${NC}"
    fi

    # 生成验收报告
    _generate_report "$day"
}

# ---------------------------------------------------------------
# 命令: report — 生成工作报告
# ---------------------------------------------------------------
cmd_report() {
    local day="${1:-}"
    if [ -z "$day" ]; then
        log_error "用法: ./orchestrator.sh report <day>"
        exit 1
    fi
    _generate_report "$day"
}

_generate_report() {
    local day=$1
    local report_file="${REPORTS_DIR}/${day}_report_$(date '+%Y%m%d').md"
    local title
    title=$(read_day_info "$day" "title")

    cat > "$report_file" << REPORT_EOF
# ${day} 工作报告 — ${title}

**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')
**项目**: CEX 风控系统

## 任务完成情况

| 任务 ID | Agent | 标题 | 状态 |
|---------|-------|------|------|
REPORT_EOF

    # 填入任务状态
    local tasks
    tasks=$(jq -r ".days.${day}.tasks[] | [.id, .agent, .title] | @tsv" "$TASKS_FILE" 2>/dev/null)

    while IFS=$'\t' read -r id agent task_title; do
        local status
        status=$(get_state "$id")
        local status_emoji
        case "$status" in
            done)    status_emoji="✅ 完成" ;;
            running) status_emoji="⚙️ 进行中" ;;
            failed)  status_emoji="❌ 失败" ;;
            *)       status_emoji="⬜ 未开始" ;;
        esac
        echo "| ${id} | Agent-$(upper "$agent") | ${task_title} | ${status_emoji} |" >> "$report_file"
    done <<< "$tasks"

    # 编译和测试结果
    cd "$PROJECT_ROOT"
    cat >> "$report_file" << 'REPORT_EOF'

## 编译 & 测试

```
REPORT_EOF

    echo "$ go build ./..." >> "$report_file"
    go build ./... >> "$report_file" 2>&1 || true
    echo "" >> "$report_file"
    echo "$ go test ./... -count=1" >> "$report_file"
    go test ./... -count=1 >> "$report_file" 2>&1 || true
    echo '```' >> "$report_file"

    # Git 统计
    cat >> "$report_file" << REPORT_EOF

## Git 提交统计

\`\`\`
$(git log --oneline -20)
\`\`\`

## 验收标准
$(read_day_info "$day" "acceptance_gate")
REPORT_EOF

    log_ok "报告已生成: ${report_file}"
}

# ---------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------
main() {
    local cmd="${1:-help}"
    shift 2>/dev/null || true

    case "$cmd" in
        init)      cmd_init "$@" ;;
        plan)      cmd_plan "$@" ;;
        run)       cmd_run "$@" ;;
        run-task)  cmd_run_task "$@" ;;
        status)    cmd_status ;;
        merge)     cmd_merge "$@" ;;
        sync)      cmd_sync ;;
        merge-all) cmd_merge_all "$@" ;;
        health)    cmd_health ;;
        gate)      cmd_gate "$@" ;;
        report)    cmd_report "$@" ;;
        help|--help|-h)
            echo ""
            echo "CEX 风控系统 — 多 Agent 并行开发调度器"
            echo ""
            echo "用法: ./orchestrator.sh <command> [options]"
            echo ""
            echo "命令:"
            echo "  init [agents...]   初始化 worktree (默认: a b)"
            echo "  plan <day>         显示某天的任务计划"
            echo "  run <day>          执行某天全部任务（按依赖顺序）"
            echo "  run-task <id>      执行单个任务（如 D1-A1）"
            echo "  status             显示所有 Agent 状态"
            echo "  merge <agent>      合并某个 Agent 到 main"
            echo "  sync               同步 main 到所有 worktree"
            echo "  merge-all [day]    每日收工：合并所有 Agent"
            echo "  health             检查中间件健康"
            echo "  gate <day>         执行验收检查"
            echo "  report <day>       生成工作报告"
            echo ""
            echo "示例:"
            echo "  ./orchestrator.sh init a b           # 初始化 Agent-A 和 Agent-B"
            echo "  ./orchestrator.sh plan D1            # 查看 D1 计划"
            echo "  ./orchestrator.sh run D1             # 执行 D1 全部任务"
            echo "  ./orchestrator.sh run-task D1-A1     # 只执行 D1 的 A1 任务"
            echo "  ./orchestrator.sh merge-all D1       # D1 结束合并"
            echo "  ./orchestrator.sh gate D3            # D3 验收（最小闭环）"
            echo ""
            ;;
        *)
            log_error "未知命令: $cmd"
            echo "运行 ./orchestrator.sh help 查看帮助"
            exit 1
            ;;
    esac
}

main "$@"
