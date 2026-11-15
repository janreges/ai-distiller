.PHONY: all build test bench lint fmt fmt-check clean install aid

# Variables
BINARY_NAME = aid
BUILD_DIR = build
INSTALL_DIR = /usr/local/bin

# Version comes from the latest git tag (single source of truth)
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
ifeq ($(VERSION),)
VERSION := dev
endif
# Get git metadata
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Define ldflags
LDFLAGS_VARS = -X 'github.com/janreges/ai-distiller/internal/version.Version=$(VERSION)' \
               -X 'github.com/janreges/ai-distiller/internal/version.Commit=$(GIT_COMMIT)' \
               -X 'github.com/janreges/ai-distiller/internal/version.Date=$(BUILD_DATE)'
LDFLAGS = -ldflags "$(LDFLAGS_VARS)"

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod
GOFMT = gofmt
GOLINT = golangci-lint
# Go files that are ours: not test input under testdata, not vendored grammar code
GO_FILES = $(shell git ls-files 'cmd/*.go' 'internal/*.go' | grep -vE '/testdata/|^internal/parser/grammars/')

all: build

# Build for current platform (with CGO - full language support and embedded templates)
build: clean
	@echo "==> Building $(BINARY_NAME) $(VERSION) with full language support and embedded templates"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 CGO_CFLAGS="-O2 -w" $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/aid
	@echo "✅ Successfully built $(BINARY_NAME) at: $(BUILD_DIR)/$(BINARY_NAME) ($$(du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1))"

# Build with verbose output (shows all warnings)
build-verbose: clean
	@echo "==> Building $(BINARY_NAME) $(VERSION) with verbose output (shows warnings)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/aid 2>&1
	@echo ""
	@echo "✅ Successfully built $(BINARY_NAME) at: $(BUILD_DIR)/$(BINARY_NAME) ($$(du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1))"

# Run tests with enhanced output
test:
	@echo "==> Running tests"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname --junitfile test-results.xml -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/' | grep -v 'cmd/aid-validate'); \
	else \
		echo "📝 For prettier test output, install gotestsum: go install gotest.tools/gotestsum@latest"; \
		$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/' | grep -v 'cmd/aid-validate'); \
	fi

# Run tests with different output formats
test-pretty:
	@echo "==> Running tests with pretty output"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format pkgname-and-test-fails --junitfile test-results.xml -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --format pkgname-and-test-fails --junitfile test-results.xml -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Run tests with dots progress
test-dots:
	@echo "==> Running tests with dots progress"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format dots-v2 -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --format dots-v2 -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Run tests with short format (just pass/fail)
test-short:
	@echo "==> Running tests with short output"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format short-verbose -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --format short-verbose -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Run tests with standard format and progress
test-standard:
	@echo "==> Running tests with standard format"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format standard-verbose -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --format standard-verbose -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Run tests with GitHub Actions format  
test-github:
	@echo "==> Running tests with GitHub Actions format"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format github-actions -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --format github-actions -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Run tests in watch mode (rerun on file changes)
test-watch:
	@echo "==> Running tests in watch mode"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --watch --format testname -- -race $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --watch --format testname -- -race $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Run tests with BDD-style output (testdox)
test-bdd:
	@echo "==> Running tests with BDD-style output"
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	export CGO_CFLAGS="-O2 -w"; \
	if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testdox -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	else \
		echo "❌ gotestsum not found. Installing..."; \
		go install gotest.tools/gotestsum@latest; \
		gotestsum --format testdox -- -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/'); \
	fi

# Original test command for compatibility
test-basic:
	@echo "==> Running tests (basic output)"
	@export CGO_CFLAGS="-O2 -w"; \
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic $$(go list ./... | grep -v '/tools/' | grep -v '/test_project/')

# Run integration tests with new test runner
test-integration:
	@echo "==> Running integration tests"
	$(GOTEST) -v -count=1 ./internal/testrunner

# Update expected test files
test-update:
	@echo "==> Updating expected test files"
	UPDATE_EXPECTED=true $(GOTEST) -v -count=1 ./internal/testrunner

# Audit test structure for consistency issues
test-audit:
	@echo "==> Auditing test structure"
	@go run ./cmd/test-audit

# Run benchmarks
bench:
	@echo "==> Running benchmarks"
	$(GOTEST) -run '^$$' -bench=. -benchmem ./internal/...

# Run linter
lint:
	@echo "==> Running linter"
	CGO_ENABLED=1 CGO_CFLAGS="-O2 -w" $(GOLINT) run ./cmd/... ./internal/...

# Format code
fmt:
	@echo "==> Formatting code"
	$(GOFMT) -s -w $(GO_FILES)

# Fail when a Go file is not gofmt -s clean (same check as CI)
fmt-check:
	@out=$$($(GOFMT) -s -l $(GO_FILES)); test -z "$$out" || { echo "$$out"; exit 1; }

# Install binary
install: build
	@echo "==> Installing $(BINARY_NAME) to $(INSTALL_DIR)"
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)

# Clean build artifacts
clean:
	@echo "==> Cleaning build artifacts"
	@rm -rf $(BUILD_DIR) coverage.txt

# Download dependencies
deps:
	@echo "==> Downloading dependencies"
	$(GOMOD) download
	$(GOMOD) tidy

# Development setup
setup:
	@echo "==> Setting up development environment"
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
	@$(GOMOD) download

# Initialize development environment with all dependencies
dev-init:
	@echo "==> Initializing development environment..."
	@echo "  - Downloading Go module dependencies"
	@$(GOMOD) download
	@echo "  - Installing testing dependencies"
	@go get -t ./...
	@echo "  - Installing development tools"
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/segmentio/golines@latest
	@go install github.com/air-verse/air@latest
	@go install gotest.tools/gotestsum@latest
	@echo "  - Running go mod tidy"
	@$(GOMOD) tidy
	@echo "==> Development environment initialized successfully!"
	@echo "==> Run 'make test' to verify everything is working"

# Run the application
run: build
	@echo "==> Running $(BINARY_NAME)"
	@$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Quick build and run for development
# Usage: make aid ARGS="<args>"
aid:
	@if $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/aid 2>&1 | grep -E "(error|cannot|undefined)" >&2; then \
		exit 1; \
	fi
	@if [ -n "$(ARGS)" ]; then \
		$(BUILD_DIR)/$(BINARY_NAME) $(ARGS); \
	else \
		echo "Usage: make aid ARGS=\"<arguments>\""; \
		echo "Example: make aid ARGS=\"testdata/java/02_simple/source.java --implementation=1 --stdout\""; \
	fi

# Catch-all target to allow passing arguments to aid
%:
	@:

# Show help
help:
	@echo "Available targets:"
	@echo "  build           - Build the application (CGO enabled, full language support)"
	@echo "  build-verbose   - Build with verbose output (shows CGO warnings)"
	@echo ""
	@echo "✨ Test commands (with gotestsum - pretty output):"
	@echo "  test            - Run tests with enhanced output (default)"
	@echo "  test-pretty     - Run tests with pretty package output ✓✖"
	@echo "  test-dots       - Run tests with dots progress indicator ···"
	@echo "  test-short      - Run tests with short verbose output"
	@echo "  test-bdd        - Run tests with BDD-style output (testdox)"
	@echo "  test-standard   - Run tests with standard format"
	@echo "  test-github     - Run tests with GitHub Actions format"
	@echo "  test-watch      - Run tests in watch mode (rerun on changes) 👀"
	@echo "  test-basic      - Run tests with basic Go output (no gotestsum)"
	@echo ""
	@echo "  bench           - Run benchmarks"
	@echo "  lint            - Run linter"
	@echo "  fmt             - Format code"
	@echo "  fmt-check       - Fail if code is not gofmt -s clean"
	@echo "  install         - Install binary to system"
	@echo "  clean           - Remove build artifacts"
	@echo "  deps            - Download dependencies"
	@echo "  setup           - Set up development environment"
	@echo "  dev-init        - Initialize dev environment with all dependencies"
	@echo "  run ARGS=...    - Run the application with arguments"

# Version management
version:
	@echo "AI Distiller version: $(VERSION)"
	@echo "Git commit: $(GIT_COMMIT)"
	@echo "Build date: $(BUILD_DATE)"

# Default target
.DEFAULT_GOAL := build