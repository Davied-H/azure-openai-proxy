#!/bin/bash

# Azure OpenAI Proxy 构建脚本
# 支持 Linux、macOS、Windows 跨平台编译

set -e

APP_NAME="azure-openai-proxy"
VERSION=${VERSION:-"1.0.0"}
BUILD_DIR="build"
CGO_ENABLED=0

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 清理构建目录
clean() {
    log_info "清理构建目录..."
    rm -rf ${BUILD_DIR}
    log_info "清理完成"
}

# 构建单个平台
build_platform() {
    local os=$1
    local arch=$2
    local output_name="${APP_NAME}"

    if [ "$os" = "windows" ]; then
        output_name="${APP_NAME}.exe"
    fi

    local output_path="${BUILD_DIR}/${os}-${arch}/${output_name}"

    log_info "构建 ${os}/${arch}..."

    mkdir -p "${BUILD_DIR}/${os}-${arch}"

    CGO_ENABLED=${CGO_ENABLED} GOOS=${os} GOARCH=${arch} go build \
        -ldflags="-s -w -X main.Version=${VERSION}" \
        -o "${output_path}" \
        .

    log_info "构建完成: ${output_path}"
}

# 构建 Linux 版本
build_linux() {
    log_info "构建 Linux 版本..."
    build_platform "linux" "amd64"
    build_platform "linux" "arm64"
}

# 构建 macOS 版本
build_darwin() {
    log_info "构建 macOS 版本..."
    build_platform "darwin" "amd64"
    build_platform "darwin" "arm64"
}

# 构建 Windows 版本
build_windows() {
    log_info "构建 Windows 版本..."
    build_platform "windows" "amd64"
}

# 构建当前平台
build_current() {
    local os=$(go env GOOS)
    local arch=$(go env GOARCH)
    log_info "构建当前平台 (${os}/${arch})..."
    build_platform "$os" "$arch"
}

# 构建所有平台
build_all() {
    log_info "构建所有平台..."
    build_linux
    build_darwin
    build_windows
    log_info "所有平台构建完成"
}

# 显示帮助
show_help() {
    echo "用法: $0 [命令]"
    echo ""
    echo "命令:"
    echo "  linux      构建 Linux 版本 (amd64, arm64)"
    echo "  darwin     构建 macOS 版本 (amd64, arm64)"
    echo "  windows    构建 Windows 版本 (amd64)"
    echo "  current    构建当前平台版本"
    echo "  all        构建所有平台版本"
    echo "  clean      清理构建目录"
    echo "  help       显示帮助信息"
    echo ""
    echo "环境变量:"
    echo "  VERSION    设置版本号 (默认: 1.0.0)"
    echo ""
    echo "示例:"
    echo "  $0 linux              # 构建 Linux 版本"
    echo "  $0 all                # 构建所有平台"
    echo "  VERSION=2.0.0 $0 all  # 使用指定版本构建所有平台"
}

# 主函数
main() {
    case "${1:-current}" in
        linux)
            build_linux
            ;;
        darwin)
            build_darwin
            ;;
        windows)
            build_windows
            ;;
        current)
            build_current
            ;;
        all)
            build_all
            ;;
        clean)
            clean
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
