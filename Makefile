# ============ 变量定义 ============
APP_NAME     := llm-proxy
MODULE       := github.com/user/llm-proxy-go
VERSION      := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME   := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
VERSION_PKG  := $(MODULE)/internal/version

LDFLAGS := -s -w \
    -X '$(VERSION_PKG).Version=$(VERSION)' \
    -X '$(VERSION_PKG).GitCommit=$(GIT_COMMIT)' \
    -X '$(VERSION_PKG).BuildTime=$(BUILD_TIME)'

CGO_ENABLED  := 0
DIST_DIR     := dist
BINARY       := $(APP_NAME)

GOOS     := $(shell go env GOOS)
GOARCH   := $(shell go env GOARCH)

.PHONY: build build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 \
        build-windows-amd64 release clean docker docker-compose version \
        test test-unit test-integration test-e2e test-all test-coverage help

# ============ 构建目标 ============

build:
	@echo "Building $(APP_NAME) $(VERSION) for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/llm-proxy
	@echo "Done: ./$(BINARY)"

build-linux-amd64:
	@echo "Building for linux/amd64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	    go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 ./cmd/llm-proxy

build-linux-arm64:
	@echo "Building for linux/arm64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
	    go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 ./cmd/llm-proxy

build-darwin-amd64:
	@echo "Building for darwin/amd64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
	    go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/llm-proxy

build-darwin-arm64:
	@echo "Building for darwin/arm64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
	    go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/llm-proxy

build-windows-amd64:
	@echo "Building for windows/amd64..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
	    go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe ./cmd/llm-proxy

# ============ 发布目标 ============

release: build
	@./scripts/build.sh --version $(VERSION)

# ============ Docker 目标 ============

docker:
	docker build -t $(APP_NAME):$(VERSION) -t $(APP_NAME):latest \
	    --build-arg VERSION=$(VERSION) \
	    --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	    --build-arg BUILD_TIME=$(BUILD_TIME) .

docker-compose:
	docker-compose up -d --build

# ============ 清理 ============

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
	rm -f coverage.out coverage.html

# ============ 版本信息 ============

version:
	@echo "$(VERSION)"

# ============ 测试目标 ============

test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test ./internal/... -v -count=1

test-integration:
	@echo "Running integration tests..."
	go test ./internal/... -tags=integration -v -count=1

test-e2e:
	@echo "Running E2E tests..."
	go test ./tests/e2e/... -tags=e2e -v -count=1 -timeout=120s

test-all:
	@echo "Running all tests..."
	go test ./... -tags="integration e2e" -v -count=1 -timeout=120s

test-coverage:
	@echo "Running tests with coverage..."
	go test ./internal/... -v -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# ============ 帮助 ============

help:
	@echo "Build targets:"
	@echo "  make build              - Build for current platform"
	@echo "  make build-linux-amd64  - Cross-compile for Linux x64"
	@echo "  make build-linux-arm64  - Cross-compile for Linux arm64"
	@echo "  make build-darwin-amd64 - Cross-compile for macOS x64"
	@echo "  make build-darwin-arm64 - Cross-compile for macOS arm64"
	@echo "  make build-windows-amd64 - Cross-compile for Windows x64"
	@echo "  make release            - Build + create release archive"
	@echo "  make docker             - Build Docker image"
	@echo "  make docker-compose     - Start with docker-compose"
	@echo "  make clean              - Remove build artifacts"
	@echo "  make version            - Show current version"
	@echo ""
	@echo "Test targets:"
	@echo "  make test               - Run unit tests (default)"
	@echo "  make test-unit          - Run unit tests only"
	@echo "  make test-integration   - Run integration tests"
	@echo "  make test-e2e           - Run E2E tests"
	@echo "  make test-all           - Run all tests"
	@echo "  make test-coverage      - Run tests with coverage report"
