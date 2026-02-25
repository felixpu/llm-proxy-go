# ============ 变量定义 ============
APP_NAME     := llm-proxy
MODULE       := github.com/user/llm-proxy-go
VERSION      := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME   := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
VERSION_PKG  := $(MODULE)/internal/version

# LDFLAGS 唯一定义处
LDFLAGS := -s -w \
    -X '$(VERSION_PKG).Version=$(VERSION)' \
    -X '$(VERSION_PKG).GitCommit=$(GIT_COMMIT)' \
    -X '$(VERSION_PKG).BuildTime=$(BUILD_TIME)'

CGO_ENABLED  := 0
DIST_DIR     := dist
BINARY       := $(APP_NAME)

GOOS     := $(shell go env GOOS)
GOARCH   := $(shell go env GOARCH)

# 跨平台构建目标列表
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

# ============ 跨平台构建模板 ============

# binary_name(os) — windows 加 .exe 后缀
binary_name = $(if $(filter windows,$(1)),$(DIST_DIR)/$(APP_NAME)-$(1)-$(2).exe,$(DIST_DIR)/$(APP_NAME)-$(1)-$(2))

# 生成 build-<os>-<arch> target
define BUILD_TEMPLATE
build-$(1)-$(2):
	@echo "Building for $(1)/$(2)..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=$(1) GOARCH=$(2) \
	    go build -ldflags "$$(LDFLAGS)" -o $$(call binary_name,$(1),$(2)) ./cmd/llm-proxy
endef

# 生成 release-<os>-<arch> target
define RELEASE_TEMPLATE
release-$(1)-$(2): build-$(1)-$(2)
	@./scripts/build.sh --binary $$(call binary_name,$(1),$(2)) --os $(1) --arch $(2) --version $$(VERSION)
endef

# 展开所有平台的 build 和 release target
$(foreach p,$(PLATFORMS),$(eval $(call BUILD_TEMPLATE,$(word 1,$(subst -, ,$(p))),$(word 2,$(subst -, ,$(p))))))
$(foreach p,$(PLATFORMS),$(eval $(call RELEASE_TEMPLATE,$(word 1,$(subst -, ,$(p))),$(word 2,$(subst -, ,$(p))))))

# ============ .PHONY 声明 ============

BUILD_TARGETS  := $(foreach p,$(PLATFORMS),build-$(p))
RELEASE_TARGETS := $(foreach p,$(PLATFORMS),release-$(p))

.PHONY: build build-all $(BUILD_TARGETS) \
        release release-all $(RELEASE_TARGETS) \
        clean docker docker-compose version \
        test test-unit test-integration test-e2e test-all test-coverage help

# ============ 构建目标 ============

build:
	@echo "Building $(APP_NAME) $(VERSION) for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/llm-proxy
	@echo "Done: ./$(BINARY)"

build-all: $(BUILD_TARGETS)

# ============ 发布目标 ============

release: build
	@./scripts/build.sh --binary ./$(BINARY) --os $(GOOS) --arch $(GOARCH) --version $(VERSION)

release-all: $(RELEASE_TARGETS)

# ============ Docker 目标 ============

docker:
	docker build -t $(APP_NAME):$(VERSION) -t $(APP_NAME):latest \
	    --build-arg VERSION=$(VERSION) \
	    --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	    --build-arg BUILD_TIME=$(BUILD_TIME) \
	    --build-arg MODULE=$(MODULE) .

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
	@echo "  make build                - Build for current platform"
	@echo "  make build-<os>-<arch>    - Cross-compile (e.g. build-linux-amd64)"
	@echo "  make build-all            - Cross-compile all platforms"
	@echo "  make release              - Build + create release archive (current platform)"
	@echo "  make release-<os>-<arch>  - Build + release for specific platform"
	@echo "  make release-all          - Build + release all platforms"
	@echo "  make docker               - Build Docker image"
	@echo "  make docker-compose       - Start with docker-compose"
	@echo "  make clean                - Remove build artifacts"
	@echo "  make version              - Show current version"
	@echo ""
	@echo "Test targets:"
	@echo "  make test                 - Run unit tests (default)"
	@echo "  make test-unit            - Run unit tests only"
	@echo "  make test-integration     - Run integration tests"
	@echo "  make test-e2e             - Run E2E tests"
	@echo "  make test-all             - Run all tests"
	@echo "  make test-coverage        - Run tests with coverage report"
	@echo ""
	@echo "Platforms: $(PLATFORMS)"
