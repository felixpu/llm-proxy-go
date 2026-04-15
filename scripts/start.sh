#!/bin/bash

# LLM Proxy Go 启动脚本
# 用法: ./start.sh [command] [options]
#   command: start(默认), stop, restart, status, build
#   options: -f 前台运行, -d 后台运行(默认), --build 强制重新编译, --run-mode 指定运行模式

set -e

# 配置
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$SCRIPT_DIR"

# 从 .env 读取端口，如果没有则使用默认值
if [ -f .env ]; then
    source .env 2>/dev/null || true
fi
PORT="${LLM_PROXY_PORT:-8000}"

PID_FILE="/tmp/llm-proxy-go.pid"
# 后台模式下的 stdout/stderr 重定向文件（避免与应用 JSON 文件日志混写）
CONSOLE_LOG_FILE="logs/llm-proxy-console.log"
BINARY_NAME="llm-proxy"
RUN_MODE_OVERRIDE=""

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 确保必要的目录存在
mkdir -p data logs

# 检查 Go 环境
check_go_env() {
    if ! command -v go &> /dev/null; then
        echo -e "${RED}错误: 未找到 Go${NC}"
        echo "请安装 Go 1.24 或更高版本"
        return 1
    fi

    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    local required_version="1.24"

    # 简单版本比较（假设格式为 major.minor.patch）
    if [ "$(printf '%s\n' "$required_version" "$go_version" | sort -V | head -n1)" != "$required_version" ]; then
        echo -e "${RED}错误: Go 版本不兼容${NC}"
        echo "需要 Go 1.24 或更高版本，当前版本: $go_version"
        return 2
    fi

    return 0
}

# 检测运行模式
# 返回: binary, source, none
detect_mode_auto() {
    if [ -f "$SCRIPT_DIR/$BINARY_NAME" ]; then
        echo "binary"
    elif [ -f "$SCRIPT_DIR/cmd/llm-proxy/main.go" ]; then
        echo "source"
    else
        echo "none"
    fi
}

# 检测运行模式（支持 --run-mode 覆盖）
# 返回: binary, source, none
detect_mode() {
    local auto_mode
    auto_mode=$(detect_mode_auto)

    if [ -z "$RUN_MODE_OVERRIDE" ]; then
        echo "$auto_mode"
        return
    fi

    case "$RUN_MODE_OVERRIDE" in
        source)
            if [ -f "$SCRIPT_DIR/cmd/llm-proxy/main.go" ]; then
                echo "source"
            else
                echo "none"
            fi
            ;;
        binary)
            if [ -f "$SCRIPT_DIR/$BINARY_NAME" ] || [ -f "$SCRIPT_DIR/cmd/llm-proxy/main.go" ]; then
                echo "binary"
            else
                echo "none"
            fi
            ;;
        *)
            echo "$auto_mode"
            ;;
    esac
}

# 编译项目
build() {
    echo -e "${GREEN}正在编译 LLM Proxy...${NC}"
    check_go_env || exit 1

    if [ -f "$SCRIPT_DIR/Makefile" ]; then
        # 开发环境：通过 make 编译，确保 LDFLAGS 一致
        make -C "$SCRIPT_DIR" build
    else
        # 发布包环境：直接编译（无版本信息注入）
        echo -e "${YELLOW}提示: 未找到 Makefile，版本信息将为默认值${NC}"
        go build -o "$BINARY_NAME" ./cmd/llm-proxy
    fi

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ 编译成功${NC}"
    else
        echo -e "${RED}✗ 编译失败${NC}"
        exit 1
    fi
}

# 自动初始化
auto_init() {
    echo -e "${GREEN}🚀 开始自动初始化...${NC}"

    # 1. 创建 .env 文件（如果不存在）
    if [ ! -f .env ]; then
        if [ -f ../.env.example ]; then
            echo -e "${YELLOW}⚙️ 创建环境配置文件...${NC}"
            cp ../.env.example .env
            echo -e "${GREEN}✓ 已创建 .env 文件${NC}"
        else
            echo -e "${YELLOW}⚙️ 创建默认环境配置文件...${NC}"
            cat > .env << EOF
# LLM Proxy 配置
LLM_PROXY_HOST=0.0.0.0
LLM_PROXY_PORT=8000
LLM_PROXY_LOG_LEVEL=info
LLM_PROXY_DB=data/llm-proxy.db
LLM_PROXY_SECRET_KEY=$(openssl rand -hex 32 2>/dev/null || echo "your-secret-key-change-this")
EOF
            echo -e "${GREEN}✓ 已创建默认 .env 文件${NC}"
        fi
    fi

    # 2. 确保必要目录存在
    mkdir -p data logs

    # 3. 如果需要编译则编译
    local mode=$(detect_mode)
    if [ "$mode" = "source" ]; then
        build
    fi

    echo -e "${GREEN}🎉 初始化完成！${NC}"
    echo -e "  配置文件: .env"
    echo -e "  数据目录: data/"
    echo -e "  日志目录: logs/"
}

# 获取启动命令
get_start_command() {
    if [ -f "$SCRIPT_DIR/$BINARY_NAME" ]; then
        echo "$SCRIPT_DIR/$BINARY_NAME"
    else
        echo ""
    fi
}

# 获取运行中的 PID
get_pid() {
    # 优先从 PID 文件读取
    if [ -f "$PID_FILE" ]; then
        local pid=$(cat "$PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            # 验证进程名
            local proc_name=$(ps -p "$pid" -o comm= 2>/dev/null)
            if echo "$proc_name" | grep -q "llm-proxy"; then
                echo "$pid"
                return
            fi
        fi
    fi
    # 否则通过端口查找，仅匹配监听（LISTEN）状态的 llm-proxy 进程
    lsof -ti ":$PORT" -sTCP:LISTEN -c llm-proxy 2>/dev/null | head -1
}

# 检查服务状态
status() {
    local pid=$(get_pid)
    local mode=$(detect_mode)
    if [ -n "$pid" ]; then
        echo -e "${GREEN}● LLM Proxy 正在运行${NC} (PID: $pid, 端口: $PORT, 模式: $mode)"
        return 0
    else
        echo -e "${YELLOW}○ LLM Proxy 未运行${NC} (模式: $mode)"
        return 1
    fi
}

# 停止服务
stop() {
    local pid=$(get_pid)
    if [ -z "$pid" ]; then
        echo -e "${YELLOW}服务未运行${NC}"
        return 0
    fi

    # 验证进程名
    local proc_name=$(ps -p "$pid" -o comm= 2>/dev/null)
    if [ -z "$proc_name" ]; then
        echo -e "${YELLOW}进程 $pid 不存在${NC}"
        rm -f "$PID_FILE"
        return 0
    fi

    if ! echo "$proc_name" | grep -q "llm-proxy"; then
        echo -e "${RED}错误: PID $pid 不是 llm-proxy 进程 (实际: $proc_name)${NC}"
        echo -e "${YELLOW}清理 PID 文件...${NC}"
        rm -f "$PID_FILE"
        return 1
    fi

    echo -e "${YELLOW}正在停止 LLM Proxy (PID: $pid)...${NC}"
    kill "$pid" 2>/dev/null || true
    sleep 1
    # 如果还在运行，强制终止
    if ps -p "$pid" > /dev/null 2>&1; then
        kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
    echo -e "${GREEN}已停止${NC}"
}

# 启动服务（后台）
start_daemon() {
    local pid=$(get_pid)
    if [ -n "$pid" ]; then
        echo -e "${YELLOW}服务已在运行 (PID: $pid)，先停止...${NC}"
        stop
    fi

    # 检查端口是否被其他进程占用（仅检查监听状态）
    local port_pids=$(lsof -ti ":$PORT" -sTCP:LISTEN 2>/dev/null)
    if [ -n "$port_pids" ]; then
        echo -e "${YELLOW}警告: 端口 $PORT 被以下进程占用:${NC}"
        echo "$port_pids" | while read p; do
            ps -p "$p" -o pid=,comm=,args= 2>/dev/null
        done
        echo -e "${RED}请先停止这些进程或更改 LLM_PROXY_PORT${NC}"
        return 1
    fi

    local cmd=$(get_start_command)
    local mode=$(detect_mode)

    if [ -z "$cmd" ]; then
        echo -e "${RED}错误: 找不到 LLM Proxy${NC}"
        echo "请确保在项目目录中运行"
        exit 1
    fi

    echo -e "${GREEN}正在启动 LLM Proxy (后台模式, $mode)...${NC}"

    # 执行命令（过滤错误消息）
    if ! $cmd > "$CONSOLE_LOG_FILE" 2>&1 & then
        echo -e "${RED}启动失败，请查看日志: $CONSOLE_LOG_FILE${NC}"
        exit 1
    fi

    local new_pid=$!
    echo "$new_pid" > "$PID_FILE"

    sleep 2
    if ps -p "$new_pid" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 启动成功${NC} (PID: $new_pid, 端口: $PORT)"
        echo -e "  结构化日志: logs/llm-proxy.log"
        echo -e "  控制台重定向日志: $CONSOLE_LOG_FILE"
        echo -e "  访问地址: http://localhost:$PORT"
    else
        echo -e "${RED}✗ 启动失败，请查看日志: $CONSOLE_LOG_FILE${NC}"
        exit 1
    fi
}

# 启动服务（前台）
start_foreground() {
    local pid=$(get_pid)
    if [ -n "$pid" ]; then
        echo -e "${YELLOW}服务已在运行 (PID: $pid)，先停止...${NC}"
        stop
    fi

    local cmd=$(get_start_command)
    local mode=$(detect_mode)

    if [ -z "$cmd" ]; then
        echo -e "${RED}错误: 找不到 LLM Proxy${NC}"
        echo "请确保在项目目录中运行"
        exit 1
    fi

    local mode=$(detect_mode)
    echo -e "${GREEN}正在启动 LLM Proxy (前台模式, $mode)...${NC}"
    echo -e "  端口: $PORT"
    echo -e "  按 Ctrl+C 停止服务"
    echo ""
    $cmd
}

# 重启服务
restart() {
    stop
    sleep 1
    start_daemon
}

# 显示帮助
show_help() {
    echo "LLM Proxy Go 启动脚本"
    echo ""
    echo "用法: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  start     启动服务（默认后台运行）"
    echo "  stop      停止服务"
    echo "  restart   重启服务"
    echo "  status    查看服务状态"
    echo "  build     编译项目"
    echo ""
    echo "Options:"
    echo "  -f, --foreground    前台运行（可查看实时日志）"
    echo "  -d, --daemon        后台运行（默认）"
    echo "  --build             启动前强制重新编译"
    echo "  --run-mode MODE     指定运行模式: binary 或 source（默认自动检测）"
    echo "  --init-only         仅初始化环境，不启动服务"
    echo "  -h, --help         显示帮助"
    echo ""
    echo "示例:"
    echo "  $0                  # 后台启动（自动初始化）"
    echo "  $0 -f               # 前台启动"
    echo "  $0 --build          # 重新编译并启动"
    echo "  $0 --run-mode source -f   # 强制源码模式启动（会自动编译）"
    echo "  $0 --run-mode binary      # 强制二进制模式启动"
    echo "  $0 build            # 仅编译"
    echo "  $0 stop             # 停止服务"
    echo "  $0 restart          # 重启服务"
    echo "  $0 status           # 查看状态"
    echo ""
    echo "运行模式:"
    echo "  binary    - 使用编译后的二进制文件"
    echo "  source    - 使用源码（需要先编译）"
}

# 主逻辑
main() {
    local command="start"
    local mode="daemon"
    local force_build=false
    local init_only=false

    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            start)
                command="start"
                shift
                ;;
            stop)
                command="stop"
                shift
                ;;
            restart)
                command="restart"
                shift
                ;;
            status)
                command="status"
                shift
                ;;
            build)
                command="build"
                shift
                ;;
            -f|--foreground)
                mode="foreground"
                shift
                ;;
            -d|--daemon)
                mode="daemon"
                shift
                ;;
            --build)
                force_build=true
                shift
                ;;
            --run-mode)
                if [ -z "${2:-}" ]; then
                    echo -e "${RED}错误: --run-mode 需要参数 (binary|source)${NC}"
                    exit 1
                fi
                RUN_MODE_OVERRIDE="$2"
                shift 2
                ;;
            --run-mode=*)
                RUN_MODE_OVERRIDE="${1#*=}"
                shift
                ;;
            --init-only)
                init_only=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                echo -e "${RED}未知参数: $1${NC}"
                show_help
                exit 1
                ;;
        esac
    done

    # 验证运行模式参数
    if [ -n "$RUN_MODE_OVERRIDE" ]; then
        case "$RUN_MODE_OVERRIDE" in
            binary|source)
                ;;
            *)
                echo -e "${RED}错误: 无效的运行模式 '$RUN_MODE_OVERRIDE'，仅支持 binary|source${NC}"
                exit 1
                ;;
        esac
    fi

    # 检查是否需要初始化
    local current_mode=$(detect_mode)
    if [ "$current_mode" = "none" ]; then
        if [ "$RUN_MODE_OVERRIDE" = "source" ]; then
            echo -e "${RED}错误: 指定为 source 模式，但未找到源码入口 cmd/llm-proxy/main.go${NC}"
        elif [ "$RUN_MODE_OVERRIDE" = "binary" ]; then
            echo -e "${RED}错误: 指定为 binary 模式，但未找到可用二进制或源码${NC}"
            echo "可先执行: $0 build"
        else
            echo -e "${RED}错误: 找不到项目文件${NC}"
            echo "请确保在 Go 项目目录中运行"
        fi
        exit 1
    fi

    # 自动初始化（如果需要）
    if [ ! -f .env ] || [ ! -d data ] || [ ! -d logs ]; then
        auto_init
    fi

    # 如果只是初始化，不启动服务
    if [ "$init_only" = true ]; then
        echo -e "${GREEN}初始化完成，使用 ./start.sh 启动服务${NC}"
        exit 0
    fi

    # 启动前编译策略（仅在 start/restart 前执行）
    if [ "$command" = "start" ] || [ "$command" = "restart" ]; then
        if [ "$force_build" = true ]; then
            build
        elif [ "$current_mode" = "source" ]; then
            # 源码模式下始终自动编译（确保代码改动生效）
            echo -e "${YELLOW}检测到源码模式，自动编译项目...${NC}"
            build
        elif [ "$current_mode" = "binary" ]; then
            # binary 模式但二进制不存在时，若有源码则自动编译
            if [ ! -f "$BINARY_NAME" ] && [ -f "cmd/llm-proxy/main.go" ]; then
                echo -e "${YELLOW}未找到二进制，自动编译项目...${NC}"
                build
            fi

            # 二进制模式下检查是否需要重新编译
            if [ -f "cmd/llm-proxy/main.go" ] && [ -f "$BINARY_NAME" ]; then
                # 检查源码是否比二进制新
                if [ "cmd/llm-proxy/main.go" -nt "$BINARY_NAME" ]; then
                    echo -e "${YELLOW}检测到源码更新，自动重新编译...${NC}"
                    build
                fi
            fi
        fi
    fi

    # 执行命令
    case $command in
        start)
            if [ "$mode" = "foreground" ]; then
                start_foreground
            else
                start_daemon
            fi
            ;;
        stop)
            stop
            ;;
        restart)
            restart
            ;;
        status)
            status
            ;;
        build)
            build
            ;;
    esac
}

main "$@"
