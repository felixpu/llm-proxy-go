#!/bin/bash
# LLM Proxy Go - 发布包打包脚本（纯打包，不编译）
# 注意: 编译由 Makefile 负责，本脚本仅负责打包
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 默认值
APP_NAME="llm-proxy"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
DIST_DIR="$PROJECT_DIR/dist"
BINARY_PATH=""
TARGET_OS=""
TARGET_ARCH=""

usage() {
    cat <<EOF
用法: $0 --binary <path> --os <os> --arch <arch> [选项]

将已编译的二进制文件打包为发布压缩包（tar.gz / zip）。
本脚本仅负责打包，编译请使用 make 命令。

必需参数:
  --binary <path>       已编译的二进制文件路径
  --os <os>             目标操作系统 (linux, darwin, windows)
  --arch <arch>         目标架构 (amd64, arm64)

可选参数:
  --version <version>   版本号（默认: git describe 自动生成）
  --clean               清理 dist/ 目录并退出
  -h, --help            显示此帮助信息

推荐用法（通过 Makefile 调用）:
  make release                  当前平台编译+打包
  make release-linux-amd64      指定平台编译+打包
  make release-all              所有平台编译+打包

直接调用示例:
  make build-linux-amd64
  $0 --binary dist/llm-proxy-linux-amd64 --os linux --arch amd64

打包产物:
  dist/<app>-<version>-<os>-<arch>/       发布目录
  dist/<app>-<version>-<os>-<arch>.tar.gz 压缩包 (Linux/macOS)
  dist/<app>-<version>-<os>-<arch>.zip    压缩包 (Windows)
EOF
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --binary)  BINARY_PATH="$2"; shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        --os)      TARGET_OS="$2"; shift 2 ;;
        --arch)    TARGET_ARCH="$2"; shift 2 ;;
        --clean)   rm -rf "$DIST_DIR"; echo "已清理 dist/"; exit 0 ;;
        -h|--help) usage; exit 0 ;;
        *)         echo "错误: 未知参数: $1"; echo ""; usage; exit 1 ;;
    esac
done

# 校验必需参数
if [ -z "$BINARY_PATH" ]; then
    echo "错误: 缺少 --binary 参数"
    echo ""
    usage
    exit 1
fi

if [ ! -f "$BINARY_PATH" ]; then
    echo "错误: 二进制文件不存在: $BINARY_PATH"
    exit 1
fi

# 从二进制路径推断 OS/ARCH（如果未指定）
if [ -z "$TARGET_OS" ] || [ -z "$TARGET_ARCH" ]; then
    # 尝试从文件名推断: llm-proxy-<os>-<arch>[.exe]
    BASENAME=$(basename "$BINARY_PATH")
    if [[ "$BASENAME" =~ ${APP_NAME}-([a-z]+)-([a-z0-9]+) ]]; then
        TARGET_OS="${TARGET_OS:-${BASH_REMATCH[1]}}"
        TARGET_ARCH="${TARGET_ARCH:-${BASH_REMATCH[2]}}"
    else
        # 当前平台 fallback
        TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
        TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
    fi
fi

# 标准化平台名称（与 Python 版一致）
normalize_os() {
    case $1 in
        darwin) echo "macos" ;;
        *)      echo "$1" ;;
    esac
}

normalize_arch() {
    case $1 in
        amd64) echo "x64" ;;
        arm64) echo "arm64" ;;
        386)   echo "x86" ;;
        *)     echo "$1" ;;
    esac
}

PLATFORM_OS=$(normalize_os "$TARGET_OS")
PLATFORM_ARCH=$(normalize_arch "$TARGET_ARCH")
PACKAGE_NAME="${APP_NAME}-${VERSION}-${PLATFORM_OS}-${PLATFORM_ARCH}"
PACKAGE_DIR="$DIST_DIR/$PACKAGE_NAME"

echo "========================================"
echo "LLM Proxy Go 打包脚本"
echo "版本: $VERSION"
echo "平台: $PLATFORM_OS-$PLATFORM_ARCH ($TARGET_OS/$TARGET_ARCH)"
echo "二进制: $BINARY_PATH"
echo "========================================"

# [1/2] 创建发布包
echo ""
echo "[1/2] 创建发布包..."
rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR"

# 复制二进制文件
BINARY_NAME="$APP_NAME"
if [ "$TARGET_OS" = "windows" ]; then
    BINARY_NAME="${APP_NAME}.exe"
fi
cp "$BINARY_PATH" "$PACKAGE_DIR/$BINARY_NAME"
echo "  复制: $BINARY_NAME"

# 复制平台对应的启动脚本
if [ "$TARGET_OS" = "windows" ]; then
    cp "$PROJECT_DIR/scripts/start.bat" "$PACKAGE_DIR/start.bat"
    echo "  复制: start.bat"
else
    cp "$PROJECT_DIR/scripts/start.sh" "$PACKAGE_DIR/start.sh"
    chmod +x "$PACKAGE_DIR/start.sh"
    chmod +x "$PACKAGE_DIR/$BINARY_NAME"
    echo "  复制: start.sh"
fi

# 复制 .env.example
if [ -f "$PROJECT_DIR/.env.example" ]; then
    cp "$PROJECT_DIR/.env.example" "$PACKAGE_DIR/.env.example"
    echo "  复制: .env.example"
fi

# 创建数据和日志目录
mkdir -p "$PACKAGE_DIR/data" "$PACKAGE_DIR/logs"
echo "  创建: data/ logs/"

# 生成 README.txt
if [ "$TARGET_OS" = "windows" ]; then
    START_CMD="start.bat"
    COPY_CMD="copy .env.example .env"
else
    START_CMD="./start.sh"
    COPY_CMD="cp .env.example .env"
fi

cat > "$PACKAGE_DIR/README.txt" << READMEEOF
LLM Proxy v${VERSION} (Go)
==================

智能 AI 模型代理服务，支持多端点负载均衡和基于内容的智能路由。

快速开始
--------
1. 复制环境变量模板：${COPY_CMD}
2. 编辑 .env 文件，设置安全密钥和管理员密码
3. 启动服务：${START_CMD} (或直接运行 ${BINARY_NAME})
4. 访问管理界面：http://localhost:8000
5. 默认管理员：admin / admin123

环境变量
--------
- LLM_PROXY_HOST: 监听地址（默认 0.0.0.0）
- LLM_PROXY_PORT: 监听端口（默认 8000）
- LLM_PROXY_SECRET_KEY: 会话加密密钥
- LLM_PROXY_DB: 数据库文件路径
- LOG_LEVEL: 日志级别（DEBUG/INFO/WARN/ERROR）
READMEEOF
echo "  创建: README.txt"

# [2/2] 创建压缩包
echo ""
echo "[2/2] 创建压缩包..."
cd "$DIST_DIR"
if [ "$TARGET_OS" = "windows" ]; then
    zip -r "${PACKAGE_NAME}.zip" "$PACKAGE_NAME"
    ARCHIVE="${PACKAGE_NAME}.zip"
else
    tar -czf "${PACKAGE_NAME}.tar.gz" "$PACKAGE_NAME"
    ARCHIVE="${PACKAGE_NAME}.tar.gz"
fi

ARCHIVE_SIZE=$(du -h "$DIST_DIR/$ARCHIVE" | cut -f1)
echo "  压缩包: $DIST_DIR/$ARCHIVE ($ARCHIVE_SIZE)"

echo ""
echo "========================================"
echo "打包完成！"
echo "发布包: $PACKAGE_DIR"
echo "压缩包: $DIST_DIR/$ARCHIVE"
echo "========================================"
