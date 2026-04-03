#!/bin/bash
# ==============================================================
# CEX 做市风控系统 — V3.0 多 Agent 并行开发调度器（Orchestrator）
#
# 基于 multi_agent_development_plan.md 设计：
#   7 Agent（F/D/I/R/S/W/Q）+ 6 Gate 质量关卡
#   事件驱动 + Gate 检查的混合调度模型
#
# 用法：./orchestrator.sh <command> [options]
#
# 命令：
#   init                初始化 worktree 和 Agent 环境
#   plan [sprint]       显示 Sprint 任务计划 + 依赖 DAG
#   dag                 渲染完整任务依赖图（ASCII）
#   run <task-id>       执行单个任务（如 F-01）
#   run-sprint <n>      执行某个 Sprint 的全部就绪任务
#   run-ready           自动执行所有可运行的任务（核心调度循环）
#   status              显示当前所有 Agent/Task/Gate 状态
#   gate <gate-id>      执行 Gate 验收检查（如 gate-1）
#   gate-all            检查所有 Gate 状态
#   merge <agent>       合并 Agent 分支到 main
#   merge-all           合并所有 Agent（Sprint 结束时）
#   sync                同步 main 到所有 Agent worktree
#   report              生成进度报告
#   reset               重置状态文件（危险操作）
# ==============================================================

set -euo pipefail

# ---------------------------------------------------------------
# 配置区
# ---------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
WORKTREE_BASE="${WORKTREE_BASE:-$(dirname "$PROJECT_ROOT")}"
TASKS_FILE="${SCRIPT_DIR}/tasks_v3.json"
STATE_FILE="${SCRIPT_DIR}/.state_v3.json"
LOG_DIR="${SCRIPT_DIR}/logs"
GATE_DIR="${SCRIPT_DIR}/gate_checks"
PROMPT_DIR="${SCRIPT_DIR}/agent_prompts"
CONTRACT_DIR="${SCRIPT_DIR}/contracts"

# Agent 列表（新七角色）
ALL_AGENTS=(F D I R S W Q)

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; PURPLE='\033[0;35m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

# Gate 状态符号
GATE_OPEN="✅"; GATE_CLOSED="❌"; GATE_RUNNING="⏳"

# ---------------------------------------------------------------
# 工具函数
# ---------------------------------------------------------------
upper() { echo "$1" | tr '[:lower:]' '[:upper:]'; }
lower() { echo "$1" | tr '[:upper:]' '[:lower:]'; }

log_info()  { echo -e "${BLUE}[INFO]${NC}  $(date '+%H:%M:%S') $*"; }
log_ok()    { echo -e "${GREEN}[ OK ]${NC}  $(date '+%H:%M:%S') $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $(date '+%H:%M:%S') $*"; }
log_error() { echo -e "${RED}[ ERR]${NC}  $(date '+%H:%M:%S') $*"; }
log_gate()  { echo -e "${PURPLE}[GATE]${NC}  $(date '+%H:%M:%S') $*"; }
log_agent() { echo -e "${CYAN}[AGENT]${NC} $(date '+%H:%M:%S') $*"; }

check_deps() {
    local missing=()
    for cmd in git jq claude; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [ ${#missing[@]} -gt 0 ]; then
        log_error "缺少依赖: ${missing[*]}"
        echo "  brew install ${missing[*]}"
        exit 1
    fi
}

ensure_dirs() {
    mkdir -p "$LOG_DIR" "$GATE_DIR" "$PROMPT_DIR" "$CONTRACT_DIR"
}

# ---------------------------------------------------------------
# 状态管理（.state_v3.json）
# ---------------------------------------------------------------
init_state() {
    cat > "$STATE_FILE" << EOF
{
  "started_at": "$(date -Iseconds)",
  "current_sprint": 0,
  "tasks": {},
  "gates": {
    "gate-1": "closed",
    "gate-2": "closed",
    "gate-3": "closed",
    "gate-4": "closed",
    "gate-5": "closed",
    "gate-6": "closed"
  }
}
EOF
}

ensure_state() {
    [ -f "$STATE_FILE" ] || init_state
}

# 读取任务状态
task_status() {
    local tid=$1
    ensure_state
    jq -r ".tasks.\"${tid}\".status // \"pending\"" "$STATE_FILE"
}

# 更新任务状态
set_task_status() {
    local tid=$1 status=$2
    ensure_state
    local ts; ts=$(date '+%Y-%m-%d %H:%M:%S')
    local tmp; tmp=$(mktemp)
    jq ".tasks.\"${tid}\" = {\"status\": \"${status}\", \"updated\": \"${ts}\"}" \
        "$STATE_FILE" > "$tmp" && mv "$tmp" "$STATE_FILE"
}

# 读取 Gate 状态
gate_status() {
    local gid=$1
    ensure_state
    jq -r ".gates.\"${gid}\" // \"closed\"" "$STATE_FILE"
}

# 更新 Gate 状态
set_gate_status() {
    local gid=$1 status=$2
    ensure_state
    local tmp; tmp=$(mktemp)
    jq ".gates.\"${gid}\" = \"${status}\"" "$STATE_FILE" > "$tmp" && mv "$tmp" "$STATE_FILE"
}

# 设置当前 Sprint
set_current_sprint() {
    local sprint=$1
    local tmp; tmp=$(mktemp)
    jq ".current_sprint = ${sprint}" "$STATE_FILE" > "$tmp" && mv "$tmp" "$STATE_FILE"
}

# ---------------------------------------------------------------
# 任务 DAG 查询
# ---------------------------------------------------------------

# 获取任务的所有依赖
task_depends() {
    local tid=$1
    jq -r ".tasks.\"${tid}\".depends[]? // empty" "$TASKS_FILE"
}

# 获取任务所属 Agent
task_agent() {
    local tid=$1
    jq -r ".tasks.\"${tid}\".agent" "$TASKS_FILE"
}

# 获取任务标题
task_title() {
    local tid=$1
    jq -r ".tasks.\"${tid}\".title" "$TASKS_FILE"
}

# 获取任务解锁的 Gate
task_gate_unlock() {
    local tid=$1
    jq -r ".tasks.\"${tid}\".gate_unlock // empty" "$TASKS_FILE"
}

# 获取某 Sprint 的所有任务
sprint_tasks() {
    local sprint=$1
    jq -r ".tasks | to_entries[] | select(.value.sprint == ${sprint}) | .key" "$TASKS_FILE"
}

# 获取某 Agent 的所有任务
agent_tasks() {
    local agent=$1
    jq -r ".tasks | to_entries[] | select(.value.agent == \"${agent}\") | .key" "$TASKS_FILE"
}

# 检查一个任务是否就绪（所有依赖满足）
is_task_ready() {
    local tid=$1
    local deps
    deps=$(task_depends "$tid")

    if [ -z "$deps" ]; then
        echo "true"
        return
    fi

    while IFS= read -r dep; do
        [ -z "$dep" ] && continue

        # 如果依赖是 gate-X 形式
        if [[ "$dep" == gate-* ]]; then
            local gs; gs=$(gate_status "$dep")
            if [ "$gs" != "open" ]; then
                echo "false"
                return
            fi
        else
            # 普通任务依赖
            local ts; ts=$(task_status "$dep")
            if [ "$ts" != "done" ]; then
                echo "false"
                return
            fi
        fi
    done <<< "$deps"

    echo "true"
}

# 获取所有可运行任务（pending + 依赖满足）
ready_tasks() {
    local all_tasks
    all_tasks=$(jq -r '.tasks | keys[]' "$TASKS_FILE")

    while IFS= read -r tid; do
        [ -z "$tid" ] && continue
        local ts; ts=$(task_status "$tid")
        if [ "$ts" = "pending" ]; then
            local ready; ready=$(is_task_ready "$tid")
            if [ "$ready" = "true" ]; then
                echo "$tid"
            fi
        fi
    done <<< "$all_tasks"
}

# ---------------------------------------------------------------
# Agent worktree 路径
# ---------------------------------------------------------------
agent_dir() {
    local agent=$1
    echo "${WORKTREE_BASE}/cex-risk-agent-$(lower "$agent")"
}

# ---------------------------------------------------------------
# 命令: init — 初始化七个 Agent worktree
# ---------------------------------------------------------------
cmd_init() {
    log_info "初始化 V3.0 七 Agent 开发环境..."
    check_deps
    ensure_dirs

    cd "$PROJECT_ROOT"

    # 确保 main 分支干净
    if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
        log_warn "main 有未提交改动，自动提交..."
        git add -A
        git commit -m "chore: orchestrator v3 init 前自动提交 $(date '+%Y%m%d_%H%M%S')" || true
    fi

    local agents_to_init=("${@}")
    [ ${#agents_to_init[@]} -eq 0 ] && agents_to_init=("${ALL_AGENTS[@]}")

    for agent in "${agents_to_init[@]}"; do
        local wt_dir; wt_dir=$(agent_dir "$agent")
        local branch="agent-$(lower "$agent")/current"

        if [ -d "$wt_dir" ]; then
            log_info "Agent-${agent} worktree 已存在: $wt_dir"
        else
            log_agent "创建 Agent-${agent} worktree..."
            git worktree add "$wt_dir" -b "$branch" 2>/dev/null || \
                git worktree add "$wt_dir" "$branch" 2>/dev/null || {
                    log_error "创建 Agent-${agent} worktree 失败"
                    continue
                }
            log_ok "Agent-${agent} worktree: $wt_dir"
        fi

        # 复制 Agent System Prompt 到 worktree CLAUDE.md
        local prompt_file="${PROMPT_DIR}/agent_$(lower "$agent").md"
        if [ -f "$prompt_file" ]; then
            cp "$prompt_file" "${wt_dir}/CLAUDE.md"
            log_ok "Agent-${agent} CLAUDE.md 已写入"
        fi
    done

    # 复制接口契约到共享位置
    if [ -d "$CONTRACT_DIR" ] && [ "$(ls -A "$CONTRACT_DIR" 2>/dev/null)" ]; then
        for agent in "${agents_to_init[@]}"; do
            local wt_dir; wt_dir=$(agent_dir "$agent")
            if [ -d "$wt_dir" ]; then
                mkdir -p "${wt_dir}/.contracts"
                cp "$CONTRACT_DIR"/* "${wt_dir}/.contracts/" 2>/dev/null || true
            fi
        done
        log_ok "接口契约已分发到所有 Agent worktree"
    fi

    # 初始化状态
    init_state

    echo ""
    log_ok "V3.0 初始化完成！"
    echo ""
    git worktree list 2>/dev/null
    echo ""
    echo -e "${BOLD}下一步：${NC}"
    echo "  ./orchestrator.sh plan 0       # 查看 Sprint 0 计划"
    echo "  ./orchestrator.sh run-ready    # 自动执行所有就绪任务"
    echo "  ./orchestrator.sh dag          # 查看完整依赖图"
}

# ---------------------------------------------------------------
# 命令: plan — 显示 Sprint 计划
# ---------------------------------------------------------------
cmd_plan() {
    local sprint=${1:-}
    check_deps
    ensure_state

    if [ -z "$sprint" ]; then
        # 显示所有 Sprint 概览
        echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
        echo -e "${BOLD}  CEX 风控系统 V3.0 — Sprint 概览${NC}"
        echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
        echo ""

        for s in 0 1 2 3 4 5; do
            local name; name=$(jq -r ".sprints.\"${s}\".name" "$TASKS_FILE")
            local days; days=$(jq -r ".sprints.\"${s}\".days" "$TASKS_FILE")
            local gate; gate=$(jq -r ".sprints.\"${s}\".target_gate" "$TASKS_FILE")
            local agents; agents=$(jq -r ".sprints.\"${s}\".active_agents | join(\", \")" "$TASKS_FILE")

            # 计算该 Sprint 的任务进度
            local total=0 done=0
            while IFS= read -r tid; do
                [ -z "$tid" ] && continue
                total=$((total + 1))
                [ "$(task_status "$tid")" = "done" ] && done=$((done + 1))
            done <<< "$(sprint_tasks "$s")"

            local pct=0
            [ $total -gt 0 ] && pct=$((done * 100 / total))

            # 进度条
            local bar_len=20
            local filled=$((pct * bar_len / 100))
            local empty=$((bar_len - filled))
            local bar=""
            for ((i=0; i<filled; i++)); do bar+="█"; done
            for ((i=0; i<empty; i++)); do bar+="░"; done

            local color=$NC
            [ $pct -eq 100 ] && color=$GREEN
            [ $pct -gt 0 ] && [ $pct -lt 100 ] && color=$YELLOW

            echo -e "  ${BOLD}Sprint ${s}${NC} │ ${name}"
            echo -e "          │ ${DIM}${days}  |  Gate: ${gate}${NC}"
            echo -e "          │ Agent: ${CYAN}${agents}${NC}"
            echo -e "          │ ${color}${bar} ${pct}% (${done}/${total})${NC}"
            echo ""
        done

        # Gate 状态面板
        echo -e "${BOLD}── Gate 状态 ──────────────────────────────────────────${NC}"
        for g in 1 2 3 4 5 6; do
            local gid="gate-${g}"
            local gs; gs=$(gate_status "$gid")
            local gname; gname=$(jq -r ".gates.\"${gid}\".name" "$TASKS_FILE")
            local icon=$GATE_CLOSED
            [ "$gs" = "open" ] && icon=$GATE_OPEN

            echo -e "  ${icon} Gate-${g}: ${gname} [${gs}]"
        done
        echo ""
        return
    fi

    # 显示某个 Sprint 的详细任务
    local name; name=$(jq -r ".sprints.\"${sprint}\".name" "$TASKS_FILE")
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  ${name}${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo ""

    # 按 Agent 分组显示任务
    for agent in "${ALL_AGENTS[@]}"; do
        local tasks_for_agent=""
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            local ta; ta=$(task_agent "$tid")
            [ "$ta" = "$agent" ] && tasks_for_agent+="${tid}\n"
        done <<< "$(sprint_tasks "$sprint")"

        [ -z "$tasks_for_agent" ] && continue

        echo -e "  ${CYAN}${BOLD}Agent-${agent}${NC}"

        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            local title; title=$(task_title "$tid")
            local hours; hours=$(jq -r ".tasks.\"${tid}\".hours" "$TASKS_FILE")
            local deps; deps=$(jq -r ".tasks.\"${tid}\".depends | join(\", \")" "$TASKS_FILE")
            local ts; ts=$(task_status "$tid")
            local gu; gu=$(task_gate_unlock "$tid")
            local ready; ready=$(is_task_ready "$tid")

            # 状态图标
            local icon="○"
            case "$ts" in
                done)    icon="${GREEN}●${NC}" ;;
                running) icon="${YELLOW}◉${NC}" ;;
                failed)  icon="${RED}✗${NC}" ;;
                *)
                    if [ "$ready" = "true" ]; then
                        icon="${BLUE}◎${NC}"  # 就绪
                    fi
                    ;;
            esac

            local gate_tag=""
            [ -n "$gu" ] && gate_tag=" ${PURPLE}→ ${gu}${NC}"

            echo -e "    ${icon} ${BOLD}${tid}${NC} ${title}  ${DIM}(${hours}h | deps: ${deps:-无})${NC}${gate_tag}"
        done <<< "$(echo -e "$tasks_for_agent")"
        echo ""
    done
}

# ---------------------------------------------------------------
# 命令: dag — 渲染完整依赖 DAG
# ---------------------------------------------------------------
cmd_dag() {
    ensure_state
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  任务依赖 DAG（有向无环图）${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo ""

    cat << 'DAG'
  ┌──────────────────────────────────────────────────────────────────┐
  │                        START                                     │
  │                          │                                       │
  │           ┌──────────────┼──────────────┐                       │
  │           ▼              ▼              ▼                        │
  │       ┌───────┐    ┌───────┐     ┌───────┐                     │
  │       │ F-01  │    │ F-02  │     │ W-01  │  ← Day 1 并行      │
  │       └──┬────┘    └──┬────┘     └──┬────┘                     │
  │    ┌─────┼─────┐      │         ┌───┼────┐                     │
  │    ▼     ▼     ▼      ▼         ▼   ▼    ▼                     │
  │  F-03  F-04  F-05   F-07      W-02 W-03 W-04...W-10           │
  │  F-06  F-08  F-09                                               │
  │    │     │     │                                                 │
  │    ▼     ▼     ▼                                                 │
  │  F-10  F-11                                                      │
  │           │                                                      │
  │           ▼                                                      │
  │    ╔═══════════╗                                                │
  │    ║  GATE-1   ║  基座就绪                                      │
  │    ╚═════╤═════╝                                                │
  │    ┌─────┼──────┬──────────┬──────────┐                        │
  │    ▼     ▼      ▼          ▼          ▼                         │
  │  D-01  I-01   R-01       S-01       W-11                       │
  │  D-05  I-02   R-02       S-03       Q-01                       │
  │    │   I-03   R-03         │                                    │
  │    ▼     │      │          ▼                                    │
  │  D-02    ▼      ▼        S-02                                   │
  │  D-03  I-04   R-04       S-04                                   │
  │  D-04  I-05   R-05         │                                    │
  │    │   I-06   R-09        S-05                                  │
  │    ▼   I-07   R-10                                              │
  │  D-06    │      │                                               │
  │  D-07    │      ▼                                               │
  │    │     │    R-06                                              │
  │    ▼     │      │                                               │
  │  D-08    │    R-07                                              │
  │    │     │      │                                               │
  │    ▼     │    R-08                                              │
  │ ╔════════╧══╗                                                   │
  │ ║  GATE-2   ║  数据管道就绪                                     │
  │ ╚═════╤═════╝                                                   │
  │       ▼                                                         │
  │     I-08  Q-02                                                  │
  │       │                                                         │
  │ ╔═════╧═════╗                                                   │
  │ ║  GATE-3   ║  指标层就绪                                       │
  │ ╚═════╤═════╝                                                   │
  │       ▼                                                         │
  │     R-11  Q-03                                                  │
  │       │                                                         │
  │ ╔═════╧═════╗                                                   │
  │ ║  GATE-4   ║  规则层就绪                                       │
  │ ╚═════╤═════╝                                                   │
  │       ▼                                                         │
  │     W-12  Q-04  Q-05  Q-06                                     │
  │       │                                                         │
  │ ╔═════╧═════╗                                                   │
  │ ║  GATE-5   ║  API 就绪                                        │
  │ ╚═════╤═════╝                                                   │
  │    ┌──┴──┐                                                      │
  │    ▼     ▼                                                      │
  │  W-13  W-14  Q-07                                               │
  │    │     │                                                      │
  │    ▼     ▼                                                      │
  │   W-15                                                          │
  │    │                                                            │
  │ ╔══╧════════╗                                                   │
  │ ║  GATE-6   ║  配置 API 就绪                                   │
  │ ╚═════╤═════╝                                                   │
  │       ▼                                                         │
  │  Q-08  Q-09                                                     │
  │          │                                                      │
  │        Q-10                                                     │
  │          │                                                      │
  │    ╔═════╧═════╗                                                │
  │    ║ MVP DONE  ║                                                │
  │    ╚═══════════╝                                                │
  └──────────────────────────────────────────────────────────────────┘
DAG

    echo ""
    echo -e "${DIM}图例: ╔══╗ = Gate 检查点, ──▶ = 依赖方向${NC}"
    echo ""

    # 当前就绪任务
    echo -e "${BOLD}── 当前可执行任务 ──${NC}"
    local ready
    ready=$(ready_tasks)
    if [ -z "$ready" ]; then
        echo -e "  ${DIM}（无就绪任务 — 请检查 Gate 或完成阻塞任务）${NC}"
    else
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            local agent; agent=$(task_agent "$tid")
            local title; title=$(task_title "$tid")
            echo -e "  ${BLUE}◎${NC} ${BOLD}${tid}${NC} [Agent-${agent}] ${title}"
        done <<< "$ready"
    fi
    echo ""
}

# ---------------------------------------------------------------
# 命令: run <task-id> — 执行单个任务
# ---------------------------------------------------------------
cmd_run() {
    local tid=$1
    check_deps
    ensure_state
    ensure_dirs

    # 验证任务存在
    local exists; exists=$(jq -r ".tasks | has(\"${tid}\")" "$TASKS_FILE")
    if [ "$exists" != "true" ]; then
        log_error "任务 ${tid} 不存在"
        exit 1
    fi

    # 检查是否就绪
    local ready; ready=$(is_task_ready "$tid")
    if [ "$ready" != "true" ]; then
        log_warn "任务 ${tid} 依赖未满足:"
        for dep in $(task_depends "$tid"); do
            if [[ "$dep" == gate-* ]]; then
                echo -e "  ${GATE_CLOSED} ${dep} = $(gate_status "$dep")"
            else
                echo -e "  $(task_status "$dep" | sed 's/done/✅/;s/pending/❌/;s/running/⏳/') ${dep}"
            fi
        done
        echo ""
        read -p "是否强制执行？(y/N) " -n 1 -r
        echo
        [[ ! $REPLY =~ ^[Yy]$ ]] && exit 0
    fi

    local agent; agent=$(task_agent "$tid")
    local title; title=$(task_title "$tid")
    local wt_dir; wt_dir=$(agent_dir "$agent")
    local prompt_file="${PROMPT_DIR}/agent_$(lower "$agent").md"
    local log_file="${LOG_DIR}/${tid}_$(date '+%Y%m%d_%H%M%S').log"

    log_agent "分发任务 ${tid} → Agent-${agent}: ${title}"

    # 标记为运行中
    set_task_status "$tid" "running"

    # 构建任务 Prompt
    local task_prompt
    task_prompt=$(_build_task_prompt "$tid")

    # 检查 worktree 是否存在
    if [ ! -d "$wt_dir" ]; then
        log_warn "Agent-${agent} worktree 不存在，使用项目根目录"
        wt_dir="$PROJECT_ROOT"
    fi

    echo -e "${DIM}  工作目录: ${wt_dir}${NC}"
    echo -e "${DIM}  日志文件: ${log_file}${NC}"
    echo ""

    # 执行 Claude Agent
    if claude -p "$task_prompt" \
        --output-format text \
        -d "$wt_dir" \
        2>&1 | tee "$log_file"; then

        set_task_status "$tid" "done"
        log_ok "任务 ${tid} 完成"

        # 检查是否解锁 Gate
        local gu; gu=$(task_gate_unlock "$tid")
        if [ -n "$gu" ]; then
            log_gate "任务 ${tid} 是 ${gu} 的最后一个解锁条件，触发 Gate 检查..."
            cmd_gate "$gu"
        fi
    else
        set_task_status "$tid" "failed"
        log_error "任务 ${tid} 执行失败，查看日志: ${log_file}"
    fi
}

# 构建任务 Prompt（集成上下文）
_build_task_prompt() {
    local tid=$1
    local agent; agent=$(task_agent "$tid")
    local title; title=$(task_title "$tid")
    local hours; hours=$(jq -r ".tasks.\"${tid}\".hours" "$TASKS_FILE")

    # 读取 Agent System Prompt
    local sys_prompt=""
    local prompt_file="${PROMPT_DIR}/agent_$(lower "$agent").md"
    if [ -f "$prompt_file" ]; then
        sys_prompt=$(cat "$prompt_file")
    fi

    # 读取接口契约
    local contracts=""
    if [ -d "$CONTRACT_DIR" ]; then
        for cf in "$CONTRACT_DIR"/*.go; do
            [ -f "$cf" ] || continue
            contracts+="
--- $(basename "$cf") ---
$(cat "$cf")
"
        done
    fi

    cat << PROMPT
${sys_prompt}

## 当前任务

**任务 ID**: ${tid}
**标题**: ${title}
**预计工时**: ${hours}h
**Agent**: Agent-${agent}

### 验收标准
- 代码可编译: go build ./...
- 单元测试通过: go test ./... -count=1
- 符合代码规范（中文注释、显式错误处理、参数校验）

### 接口契约（必须严格遵守）
${contracts}

### 参考文档
- docs/risk_control_plan_v3.0.md
- docs/multi_agent_development_plan.md
- docs/indicator_library_v2.0.md

### 约束
- 只修改你负责的目录
- 所有对外接口严格遵守契约定义
- 完成后运行 go build ./... 和 go test ./... 确认通过
- 遇到需要修改共享代码的情况，在代码中加 TODO 注释标记，不要自行修改
- 每完成一个功能点立即 commit
PROMPT
}

# ---------------------------------------------------------------
# 命令: run-ready — 核心调度循环
# ---------------------------------------------------------------
cmd_run_ready() {
    check_deps
    ensure_state
    ensure_dirs

    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  Orchestrator 调度循环${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo ""

    local ready
    ready=$(ready_tasks)

    if [ -z "$ready" ]; then
        log_info "当前无可执行任务"
        echo ""
        echo -e "${DIM}可能的原因：${NC}"
        echo "  1. 所有任务已完成"
        echo "  2. 被 Gate 阻塞 — 运行 ./orchestrator.sh gate-all 检查"
        echo "  3. 前置任务未完成 — 运行 ./orchestrator.sh status 查看"
        return
    fi

    # 按 Agent 分组显示就绪任务
    echo -e "${BOLD}就绪任务（可并行执行）：${NC}"
    echo ""

    local agent_tasks_map=""
    while IFS= read -r tid; do
        [ -z "$tid" ] && continue
        local agent; agent=$(task_agent "$tid")
        local title; title=$(task_title "$tid")
        echo -e "  ${BLUE}◎${NC} ${BOLD}${tid}${NC} [Agent-${agent}] ${title}"
        agent_tasks_map+="${agent}:${tid}\n"
    done <<< "$ready"

    echo ""
    echo -e "${BOLD}选择执行模式：${NC}"
    echo "  1) 逐个执行（依次执行所有就绪任务）"
    echo "  2) 选择执行（选择特定任务执行）"
    echo "  3) 并行执行（为每个 Agent 开一个终端窗口）"
    echo "  4) 仅生成 Prompt（不执行，输出到 logs/）"
    echo ""
    read -p "选择 [1-4]: " -n 1 -r choice
    echo ""

    case "$choice" in
        1)
            while IFS= read -r tid; do
                [ -z "$tid" ] && continue
                cmd_run "$tid"
            done <<< "$ready"
            ;;
        2)
            read -p "输入任务 ID（多个用空格分隔）: " -r task_ids
            for tid in $task_ids; do
                cmd_run "$tid"
            done
            ;;
        3)
            _run_parallel "$ready"
            ;;
        4)
            _generate_prompts "$ready"
            ;;
    esac
}

# 并行执行：为每个 Agent 打开终端窗口
_run_parallel() {
    local tasks=$1
    local seen_agents=""

    while IFS= read -r tid; do
        [ -z "$tid" ] && continue
        local agent; agent=$(task_agent "$tid")

        # 每个 Agent 只开一个窗口
        if echo "$seen_agents" | grep -q "$agent"; then
            continue
        fi
        seen_agents+=" $agent"

        local wt_dir; wt_dir=$(agent_dir "$agent")
        local prompt; prompt=$(_build_task_prompt "$tid")
        local prompt_file="${LOG_DIR}/.prompt_${tid}.tmp"
        echo "$prompt" > "$prompt_file"

        set_task_status "$tid" "running"

        log_agent "启动 Agent-${agent} 处理 ${tid}..."

        # macOS: 用 osascript 打开新终端窗口
        if command -v osascript &>/dev/null; then
            osascript -e "
                tell application \"Terminal\"
                    do script \"cd '${wt_dir}' && claude -p '$(cat "$prompt_file" | sed "s/'/'\\''/g")' 2>&1 | tee '${LOG_DIR}/${tid}_$(date '+%Y%m%d_%H%M%S').log'\"
                    activate
                end tell
            " 2>/dev/null &
        else
            # Linux: 后台执行
            (
                cd "$wt_dir"
                claude -p "$prompt" \
                    --output-format text \
                    2>&1 | tee "${LOG_DIR}/${tid}_$(date '+%Y%m%d_%H%M%S').log"
            ) &
            log_info "Agent-${agent} PID: $!"
        fi
    done <<< "$tasks"

    log_info "所有 Agent 已启动，使用 ./orchestrator.sh status 查看进度"
}

# 仅生成 Prompt 文件
_generate_prompts() {
    local tasks=$1
    while IFS= read -r tid; do
        [ -z "$tid" ] && continue
        local prompt; prompt=$(_build_task_prompt "$tid")
        local out="${LOG_DIR}/prompt_${tid}.md"
        echo "$prompt" > "$out"
        log_ok "Prompt 已生成: ${out}"
    done <<< "$tasks"
}

# ---------------------------------------------------------------
# 命令: run-sprint <n>
# ---------------------------------------------------------------
cmd_run_sprint() {
    local sprint=$1
    local name; name=$(jq -r ".sprints.\"${sprint}\".name" "$TASKS_FILE")
    log_info "开始执行 ${name}..."

    set_current_sprint "$sprint"

    local tasks
    tasks=$(sprint_tasks "$sprint")

    # 过滤出就绪的任务
    local ready_in_sprint=""
    while IFS= read -r tid; do
        [ -z "$tid" ] && continue
        local ts; ts=$(task_status "$tid")
        if [ "$ts" = "pending" ]; then
            local r; r=$(is_task_ready "$tid")
            [ "$r" = "true" ] && ready_in_sprint+="${tid}\n"
        fi
    done <<< "$tasks"

    if [ -z "$ready_in_sprint" ]; then
        log_warn "Sprint ${sprint} 无就绪任务（可能被 Gate 阻塞）"
        return
    fi

    echo -e "\n${BOLD}Sprint ${sprint} 就绪任务：${NC}"
    echo -e "$ready_in_sprint" | while read -r tid; do
        [ -z "$tid" ] && continue
        echo -e "  ◎ ${tid}: $(task_title "$tid")"
    done
    echo ""

    read -p "开始执行？(Y/n) " -n 1 -r
    echo
    [[ $REPLY =~ ^[Nn]$ ]] && return

    echo -e "$ready_in_sprint" | while read -r tid; do
        [ -z "$tid" ] && continue
        cmd_run "$tid"
    done
}

# ---------------------------------------------------------------
# 命令: gate <gate-id> — 执行 Gate 验收
# ---------------------------------------------------------------
cmd_gate() {
    local gid=$1
    ensure_state

    local gname; gname=$(jq -r ".gates.\"${gid}\".name" "$TASKS_FILE")
    local gdesc; gdesc=$(jq -r ".gates.\"${gid}\".description" "$TASKS_FILE")

    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  Gate 验收: ${gid} — ${gname}${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "  ${DIM}${gdesc}${NC}"
    echo ""

    # 检查前置 Gate
    local dep_gate; dep_gate=$(jq -r ".gates.\"${gid}\".depends_on_gate // empty" "$TASKS_FILE")
    if [ -n "$dep_gate" ]; then
        local dep_gs; dep_gs=$(gate_status "$dep_gate")
        if [ "$dep_gs" != "open" ]; then
            log_error "前置 ${dep_gate} 未通过，无法检查 ${gid}"
            return 1
        fi
    fi

    # 检查是否有自定义 Gate 脚本
    local gate_script="${GATE_DIR}/check_$(echo "$gid" | tr '-' '_').sh"
    if [ -f "$gate_script" ] && [ -x "$gate_script" ]; then
        log_gate "执行自定义 Gate 脚本: $gate_script"
        if bash "$gate_script"; then
            set_gate_status "$gid" "open"
            log_ok "${gid} 验收通过 ${GATE_OPEN}"
            _on_gate_open "$gid"
            return 0
        else
            set_gate_status "$gid" "closed"
            log_error "${gid} 验收失败 ${GATE_CLOSED}"
            return 1
        fi
    fi

    # 使用 tasks_v3.json 中定义的检查
    local checks; checks=$(jq -c ".gates.\"${gid}\".checks[]" "$TASKS_FILE" 2>/dev/null)
    local total=0 passed=0

    while IFS= read -r check; do
        [ -z "$check" ] && continue
        total=$((total + 1))

        local cid; cid=$(echo "$check" | jq -r '.id')
        local cmd; cmd=$(echo "$check" | jq -r '.cmd')
        local expect; expect=$(echo "$check" | jq -r '.expect')

        echo -ne "  检查 ${cid}: ${cmd:0:60}... "

        # 执行检查
        cd "$PROJECT_ROOT"
        if eval "$cmd" >/dev/null 2>&1; then
            passed=$((passed + 1))
            echo -e "${GREEN}PASS${NC}"
        else
            echo -e "${RED}FAIL${NC} (期望: ${expect})"
        fi
    done <<< "$checks"

    echo ""
    echo -e "  结果: ${passed}/${total} 通过"

    if [ $passed -eq $total ] && [ $total -gt 0 ]; then
        set_gate_status "$gid" "open"
        log_ok "${gid} ${gname} 验收通过 ${GATE_OPEN}"
        _on_gate_open "$gid"
        return 0
    else
        set_gate_status "$gid" "closed"
        log_error "${gid} ${gname} 验收失败 ${GATE_CLOSED} (${passed}/${total})"
        return 1
    fi
}

# Gate 通过后的回调
_on_gate_open() {
    local gid=$1
    local unlocks; unlocks=$(jq -r ".gates.\"${gid}\".unlocks[]?" "$TASKS_FILE" 2>/dev/null)

    if [ -n "$unlocks" ]; then
        echo ""
        log_gate "${gid} 已解锁以下任务："
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            echo -e "  ${BLUE}◎${NC} ${tid}: $(task_title "$tid")"
        done <<< "$unlocks"
    fi
}

# ---------------------------------------------------------------
# 命令: gate-all — 检查所有 Gate
# ---------------------------------------------------------------
cmd_gate_all() {
    ensure_state
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  全 Gate 状态检查${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo ""

    for g in 1 2 3 4 5 6; do
        local gid="gate-${g}"
        local gs; gs=$(gate_status "$gid")
        local gname; gname=$(jq -r ".gates.\"${gid}\".name" "$TASKS_FILE")
        local icon=$GATE_CLOSED
        local color=$RED
        [ "$gs" = "open" ] && { icon=$GATE_OPEN; color=$GREEN; }

        echo -e "  ${icon} ${BOLD}${gid}${NC}: ${gname}  ${color}[${gs}]${NC}"

        # 如果 closed，显示缺失条件
        if [ "$gs" != "open" ]; then
            local checks; checks=$(jq -c ".gates.\"${gid}\".checks[]" "$TASKS_FILE" 2>/dev/null)
            while IFS= read -r check; do
                [ -z "$check" ] && continue
                local cid; cid=$(echo "$check" | jq -r '.id')
                local cmd; cmd=$(echo "$check" | jq -r '.cmd')

                cd "$PROJECT_ROOT" 2>/dev/null
                if eval "$cmd" >/dev/null 2>&1; then
                    echo -e "    ${GREEN}✓${NC} ${cid}"
                else
                    echo -e "    ${RED}✗${NC} ${cid}"
                fi
            done <<< "$checks"
        fi
    done
    echo ""
}

# ---------------------------------------------------------------
# 命令: status — 全局状态面板
# ---------------------------------------------------------------
cmd_status() {
    ensure_state
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  CEX 风控系统 V3.0 — 开发状态面板${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo ""

    # 总览统计
    local total=0 done_count=0 running_count=0 failed_count=0
    while IFS= read -r tid; do
        [ -z "$tid" ] && continue
        total=$((total + 1))
        local ts; ts=$(task_status "$tid")
        case "$ts" in
            done)    done_count=$((done_count + 1)) ;;
            running) running_count=$((running_count + 1)) ;;
            failed)  failed_count=$((failed_count + 1)) ;;
        esac
    done <<< "$(jq -r '.tasks | keys[]' "$TASKS_FILE")"

    local pending_count=$((total - done_count - running_count - failed_count))
    local pct=0
    [ $total -gt 0 ] && pct=$((done_count * 100 / total))

    echo -e "  ${BOLD}总进度:${NC} ${done_count}/${total} (${pct}%)"
    echo -e "  ${GREEN}完成: ${done_count}${NC}  ${YELLOW}进行中: ${running_count}${NC}  ${RED}失败: ${failed_count}${NC}  ${DIM}待执行: ${pending_count}${NC}"
    echo ""

    # 每个 Agent 的进度
    echo -e "${BOLD}── Agent 进度 ──${NC}"
    for agent in "${ALL_AGENTS[@]}"; do
        local a_total=0 a_done=0
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            a_total=$((a_total + 1))
            [ "$(task_status "$tid")" = "done" ] && a_done=$((a_done + 1))
        done <<< "$(agent_tasks "$agent")"

        local a_pct=0
        [ $a_total -gt 0 ] && a_pct=$((a_done * 100 / a_total))

        local role; role=$(jq -r ".meta.agents.\"${agent}\".role" "$TASKS_FILE")
        local bar_len=15
        local filled=$((a_pct * bar_len / 100))
        local empty=$((bar_len - filled))
        local bar=""
        for ((i=0; i<filled; i++)); do bar+="█"; done
        for ((i=0; i<empty; i++)); do bar+="░"; done

        echo -e "  ${CYAN}Agent-${agent}${NC} (${role})  ${bar} ${a_pct}% (${a_done}/${a_total})"
    done
    echo ""

    # Gate 状态
    echo -e "${BOLD}── Gate 状态 ──${NC}"
    for g in 1 2 3 4 5 6; do
        local gid="gate-${g}"
        local gs; gs=$(gate_status "$gid")
        local gname; gname=$(jq -r ".gates.\"${gid}\".name" "$TASKS_FILE")
        local icon=$GATE_CLOSED
        [ "$gs" = "open" ] && icon=$GATE_OPEN
        echo -e "  ${icon} Gate-${g}: ${gname}"
    done
    echo ""

    # 就绪任务
    local ready; ready=$(ready_tasks)
    if [ -n "$ready" ]; then
        echo -e "${BOLD}── 就绪任务 ──${NC}"
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            echo -e "  ${BLUE}◎${NC} ${tid} [Agent-$(task_agent "$tid")] $(task_title "$tid")"
        done <<< "$ready"
        echo ""
    fi

    # 失败任务
    if [ $failed_count -gt 0 ]; then
        echo -e "${BOLD}── ${RED}失败任务${NC} ──"
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            [ "$(task_status "$tid")" = "failed" ] && \
                echo -e "  ${RED}✗${NC} ${tid}: $(task_title "$tid")"
        done <<< "$(jq -r '.tasks | keys[]' "$TASKS_FILE")"
        echo ""
    fi
}

# ---------------------------------------------------------------
# 命令: merge <agent> — 合并 Agent 分支到 main
# ---------------------------------------------------------------
cmd_merge() {
    local agent=$1
    local branch="agent-$(lower "$agent")/current"

    cd "$PROJECT_ROOT"

    # 确保 main 干净
    if [ -n "$(git status --porcelain)" ]; then
        log_warn "main 有未提交改动，自动提交..."
        git add -A
        git commit -m "chore: merge 前自动提交" || true
    fi

    log_info "合并 Agent-${agent} (${branch}) → main..."

    if git merge "$branch" --no-edit -m "merge(agent-${agent}): 合并 Agent-${agent} 工作 $(date '+%Y%m%d')"; then
        log_ok "Agent-${agent} 合并成功"
    else
        log_error "合并冲突！请手动解决："
        echo "  cd $PROJECT_ROOT"
        echo "  git status  # 查看冲突文件"
        echo "  # 解决冲突后: git add . && git commit"
        return 1
    fi
}

# ---------------------------------------------------------------
# 命令: merge-all — 合并所有 Agent
# ---------------------------------------------------------------
cmd_merge_all() {
    log_info "合并所有 Agent 到 main..."

    # 按优先级排序: F → D → I → R → S → W → Q
    for agent in F D I R S W Q; do
        local branch="agent-$(lower "$agent")/current"
        # 检查分支是否有新提交
        cd "$PROJECT_ROOT"
        local ahead; ahead=$(git rev-list --count main.."$branch" 2>/dev/null || echo "0")
        if [ "$ahead" -gt 0 ]; then
            cmd_merge "$agent"
        else
            log_info "Agent-${agent} 无新提交，跳过"
        fi
    done
}

# ---------------------------------------------------------------
# 命令: sync — 同步 main 到所有 worktree
# ---------------------------------------------------------------
cmd_sync() {
    log_info "同步 main 到所有 Agent worktree..."

    for agent in "${ALL_AGENTS[@]}"; do
        local wt_dir; wt_dir=$(agent_dir "$agent")
        [ ! -d "$wt_dir" ] && continue

        log_info "同步 Agent-${agent}..."
        cd "$wt_dir"

        # 先提交本地改动
        if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
            git add -A
            git commit -m "chore: sync 前自动提交 Agent-${agent}" 2>/dev/null || true
        fi

        # rebase onto main
        if git rebase main 2>/dev/null; then
            log_ok "Agent-${agent} 已同步"
        else
            log_warn "Agent-${agent} rebase 有冲突，跳过（需手动处理）"
            git rebase --abort 2>/dev/null || true
        fi
    done

    cd "$PROJECT_ROOT"
}

# ---------------------------------------------------------------
# 命令: report — 生成进度报告
# ---------------------------------------------------------------
cmd_report() {
    ensure_state

    local report_file="${LOG_DIR}/report_$(date '+%Y%m%d_%H%M%S').md"

    {
        echo "# CEX 风控系统 V3.0 — 进度报告"
        echo ""
        echo "> 生成时间: $(date '+%Y-%m-%d %H:%M:%S')"
        echo ""

        # 总览
        local total=0 done_count=0 running_count=0 failed_count=0
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            total=$((total + 1))
            case "$(task_status "$tid")" in
                done)    done_count=$((done_count + 1)) ;;
                running) running_count=$((running_count + 1)) ;;
                failed)  failed_count=$((failed_count + 1)) ;;
            esac
        done <<< "$(jq -r '.tasks | keys[]' "$TASKS_FILE")"

        echo "## 总览"
        echo ""
        echo "| 指标 | 值 |"
        echo "|------|-----|"
        echo "| 总任务 | ${total} |"
        echo "| 已完成 | ${done_count} |"
        echo "| 进行中 | ${running_count} |"
        echo "| 失败 | ${failed_count} |"
        echo "| 待执行 | $((total - done_count - running_count - failed_count)) |"
        echo ""

        # Gate 状态
        echo "## Gate 状态"
        echo ""
        for g in 1 2 3 4 5 6; do
            local gid="gate-${g}"
            local gs; gs=$(gate_status "$gid")
            local gname; gname=$(jq -r ".gates.\"${gid}\".name" "$TASKS_FILE")
            local icon="❌"; [ "$gs" = "open" ] && icon="✅"
            echo "- ${icon} Gate-${g}: ${gname} [${gs}]"
        done
        echo ""

        # Agent 进度
        echo "## Agent 进度"
        echo ""
        echo "| Agent | 角色 | 进度 |"
        echo "|-------|------|------|"
        for agent in "${ALL_AGENTS[@]}"; do
            local a_total=0 a_done=0
            while IFS= read -r tid; do
                [ -z "$tid" ] && continue
                a_total=$((a_total + 1))
                [ "$(task_status "$tid")" = "done" ] && a_done=$((a_done + 1))
            done <<< "$(agent_tasks "$agent")"
            local role; role=$(jq -r ".meta.agents.\"${agent}\".role" "$TASKS_FILE")
            echo "| Agent-${agent} | ${role} | ${a_done}/${a_total} |"
        done
        echo ""

        # 详细任务状态
        echo "## 任务详情"
        echo ""
        echo "| ID | Agent | 标题 | 状态 |"
        echo "|----|-------|------|------|"
        while IFS= read -r tid; do
            [ -z "$tid" ] && continue
            local agent; agent=$(task_agent "$tid")
            local title; title=$(task_title "$tid")
            local ts; ts=$(task_status "$tid")
            echo "| ${tid} | ${agent} | ${title} | ${ts} |"
        done <<< "$(jq -r '.tasks | keys[]' "$TASKS_FILE")"

    } > "$report_file"

    log_ok "报告已生成: ${report_file}"
}

# ---------------------------------------------------------------
# 命令: reset — 重置状态
# ---------------------------------------------------------------
cmd_reset() {
    echo -e "${RED}${BOLD}警告：将重置所有任务和 Gate 状态！${NC}"
    read -p "确认重置？(y/N) " -n 1 -r
    echo
    [[ ! $REPLY =~ ^[Yy]$ ]] && return
    init_state
    log_ok "状态已重置"
}

# ---------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------
main() {
    local cmd=${1:-help}
    shift 2>/dev/null || true

    ensure_dirs

    case "$cmd" in
        init)        cmd_init "$@" ;;
        plan)        cmd_plan "$@" ;;
        dag)         cmd_dag ;;
        run)         cmd_run "$@" ;;
        run-sprint)  cmd_run_sprint "$@" ;;
        run-ready)   cmd_run_ready ;;
        status)      cmd_status ;;
        gate)        cmd_gate "$@" ;;
        gate-all)    cmd_gate_all ;;
        merge)       cmd_merge "$@" ;;
        merge-all)   cmd_merge_all ;;
        sync)        cmd_sync ;;
        report)      cmd_report ;;
        reset)       cmd_reset ;;
        help|--help|-h)
            echo "用法: ./orchestrator.sh <命令> [参数]"
            echo ""
            echo "命令:"
            echo "  init                初始化七 Agent worktree"
            echo "  plan [sprint]       显示 Sprint 计划（不传参显示全览）"
            echo "  dag                 渲染任务依赖 DAG"
            echo "  run <task-id>       执行单个任务"
            echo "  run-sprint <n>      执行某 Sprint 就绪任务"
            echo "  run-ready           自动执行所有就绪任务"
            echo "  status              全局状态面板"
            echo "  gate <gate-id>      Gate 验收（如 gate-1）"
            echo "  gate-all            检查所有 Gate"
            echo "  merge <agent>       合并 Agent 到 main"
            echo "  merge-all           合并所有 Agent"
            echo "  sync                同步 main 到所有 worktree"
            echo "  report              生成进度报告"
            echo "  reset               重置状态"
            ;;
        *)
            log_error "未知命令: $cmd"
            echo "运行 ./orchestrator.sh help 查看帮助"
            exit 1
            ;;
    esac
}

main "$@"
