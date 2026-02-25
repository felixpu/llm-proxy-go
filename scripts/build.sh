#!/bin/bash
# LLM Proxy Go - 构建发布包脚本
# 用法: ./scripts/build.sh [--version VERSION] [--os OS] [--arch ARCH] [--clean]
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 默认值
APP_NAME="llm-proxy"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"
DIST_DIR="$PROJECT_DIR/dist"

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --version) VERSION="$2"; shift 2 ;;
        --os)      TARGET_OS="$2"; shift 2 ;;
        --arch)    TARGET_ARCH="$2"; shift 2 ;;
        --clean)   rm -rf "$DIST_DIR"; echo "已清理 dist/"; exit 0 ;;
        *)         echo "未知参数: $1"; exit 1 ;;
    esac
done

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
echo "LLM Proxy Go 构建脚本"
echo "版本: $VERSION"
echo "平台: $PLATFORM_OS-$PLATFORM_ARCH ($TARGET_OS/$TARGET_ARCH)"
echo "========================================"

# [1/4] 检查构建环境
echo ""
echo "[1/4] 检查构建环境..."
if ! command -v go &>/dev/null; then
    echo "错误: 未找到 Go"; exit 1
fi
echo "  Go 版本: $(go version)"

# [2/4] 编译
echo ""
echo "[2/4] 编译可执行文件..."
MODULE="github.com/user/llm-proxy-go"
VERSION_PKG="${MODULE}/internal/version"
LDFLAGS="-s -w -X '${VERSION_PKG}.Version=${VERSION}' -X '${VERSION_PKG}.GitCommit=${GIT_COMMIT}' -X '${VERSION_PKG}.BuildTime=${BUILD_TIME}'"

BINARY_NAME="$APP_NAME"
if [ "$TARGET_OS" = "windows" ]; then
    BINARY_NAME="${APP_NAME}.exe"
fi

mkdir -p "$DIST_DIR"

CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
    go build -ldflags "$LDFLAGS" -o "$DIST_DIR/$BINARY_NAME" ./cmd/llm-proxy

BINARY_SIZE=$(du -h "$DIST_DIR/$BINARY_NAME" | cut -f1)
echo "  可执行文件: $DIST_DIR/$BINARY_NAME ($BINARY_SIZE)"

# [3/4] 创建发布包
echo ""
echo "[3/4] 创建发布包..."
rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR"

mv "$DIST_DIR/$BINARY_NAME" "$PACKAGE_DIR/"
echo "  复制: $BINARY_NAME"

if [ "$TARGET_OS" = "windows" ]; then
    cp "$PROJECT_DIR/scripts/start.bat" "$PACKAGE_DIR/start.bat"
    echo "  复制: start.bat"
else
    cp "$PROJECT_DIR/scripts/start.sh" "$PACKAGE_DIR/start.sh"
    chmod +x "$PACKAGE_DIR/start.sh"
    chmod +x "$PACKAGE_DIR/$BINARY_NAME"
    echo "  复制: start.sh"
fi

if [ -f "$PROJECT_DIR/.env.example" ]; then
    cp "$PROJECT_DIR/.env.example" "$PACKAGE_DIR/.env.example"
    echo "  复制: .env.example"
fi

mkdir -p "$PACKAGE_DIR/data" "$PACKAGE_DIR/logs"
echo "  创建: data/ logs/"

# 根据平台生成不同的 README
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

# [4/4] 创建压缩包
echo ""
echo "[4/4] 创建压缩包..."
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
echo "构建完成！"
echo "发布包: $PACKAGE_DIR"
echo "压缩包: $DIST_DIR/$ARCHIVE"
echo "========================================"
