# Go parameters
BINARY_NAME=mcp-obsidian
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_CLEAN=$(GO_CMD) clean
GO_TEST=$(GO_CMD) test
GO_GET=$(GO_CMD) get
GO_MOD=$(GO_CMD) mod
GO_VET=$(GO_CMD) vet
GO_FMT=$(GO_CMD) fmt

# Linter versions. This is the only place these numbers live (no leading "v"):
# `make lint` and `make staticcheck` install exactly these versions into bin/ and run
# them from there, never whatever is on PATH, and CI runs the same targets.
#
# Why the pins move with the go directive (canonical explanation; the workflow and
# README point here):
#  - golangci-lint refuses a module whose language version (major.minor) is newer than
#    the Go it was built with: "can't load config: the Go language version (goX) used
#    to build golangci-lint is lower than the targeted Go version (Y)".
#  - golangci-lint and staticcheck embed golang.org/x/tools' export-data reader, so a Go
#    release that changes the export data format needs tools built against a newer
#    x/tools, even across patch releases. go1.27.2 moved to export data version 5:
#    staticcheck fails with "export data version 5 is greater than maximum supported
#    version 4", golangci-lint with spurious typecheck errors. golangci-lint 2.13.x
#    fails on it (2.14.0 happened to work when tested). The pins track the current
#    releases.
GOLANGCI_LINT_VERSION=2.15.0
STATICCHECK_VERSION=0.8.2

# Pinned linters are built from source with the current Go toolchain, so the install
# path carries both the tool version and the Go version: bumping either pin, or the
# toolchain, selects a new path and the next run reinstalls. Remove bin/ to reclaim
# space from old builds.
TOOLS_GO_VERSION:=$(shell $(GO_CMD) env GOVERSION 2>/dev/null)
TOOLS_DIR=bin
GOLANGCI_LINT=$(TOOLS_DIR)/golangci-lint-v$(GOLANGCI_LINT_VERSION)-$(TOOLS_GO_VERSION)/golangci-lint
STATICCHECK=$(TOOLS_DIR)/staticcheck-v$(STATICCHECK_VERSION)-$(TOOLS_GO_VERSION)/staticcheck

# Build parameters
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Directories
BUILD_DIR=build
COVERAGE_DIR=coverage

.PHONY: all build clean test lint staticcheck fmt vet deps help install run check coverage tidy verify

# Default target
all: check build

## help: Display this help message
help:
	@echo "Available targets:"
	@echo "  make build        - Build the binary"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make coverage     - Run tests with coverage report"
	@echo "  make lint         - Run the pinned golangci-lint"
	@echo "  make staticcheck  - Run the pinned staticcheck"
	@echo "  make fmt          - Format code"
	@echo "  make vet          - Run go vet"
	@echo "  make check        - Run all checks (fmt, vet, lint, staticcheck, test)"
	@echo "  make deps         - Download dependencies"
	@echo "  make tidy         - Tidy and verify dependencies"
	@echo "  make verify       - Verify dependencies"
	@echo "  make install      - Install the binary"
	@echo "  make run          - Run the application (requires VAULT=/path/to/vault)"
	@echo "  make all          - Run checks and build"

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Built: $(BUILD_DIR)/$(BINARY_NAME)"

## clean: Remove build artifacts and test cache
clean:
	@echo "Cleaning..."
	@$(GO_CLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

## test: Run tests
test:
	@echo "Running tests (short; make test-full runs the large-frame test too)..."
	@$(GO_TEST) -v -race -short -timeout 60s ./...

## test-full: Run all tests including the large-frame transport test (CI runs the full suite too)
test-full:
	@echo "Running all tests..."
	@$(GO_TEST) -v -race -timeout 120s ./...

## coverage: Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	@$(GO_TEST) -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	@$(GO_CMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"
	@$(GO_CMD) tool cover -func=$(COVERAGE_DIR)/coverage.out

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@$(GO_FMT) ./...
	@echo "Code formatted"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@$(GO_VET) ./...
	@echo "go vet passed"

## lint: Run the pinned golangci-lint (installed into bin/ on first use)
lint: $(GOLANGCI_LINT)
	@echo "Running golangci-lint v$(GOLANGCI_LINT_VERSION)..."
	@$(GOLANGCI_LINT) run ./...

## staticcheck: Run the pinned staticcheck (installed into bin/ on first use)
staticcheck: $(STATICCHECK)
	@echo "Running staticcheck v$(STATICCHECK_VERSION)..."
	@$(STATICCHECK) ./...

# A failed install leaves no file at the target path, so the recipe exits non-zero and
# the next run retries; a missing linter can never let `make check` pass.
$(GOLANGCI_LINT):
	@echo "Installing golangci-lint v$(GOLANGCI_LINT_VERSION) into $(@D)..."
	@GOBIN="$(CURDIR)/$(@D)" $(GO_CMD) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION) || { \
		echo "Error: could not install golangci-lint v$(GOLANGCI_LINT_VERSION) with $(GO_CMD) install; make lint needs it (check network access to the Go module proxy)."; \
		exit 1; \
	}

$(STATICCHECK):
	@echo "Installing staticcheck v$(STATICCHECK_VERSION) into $(@D)..."
	@GOBIN="$(CURDIR)/$(@D)" $(GO_CMD) install honnef.co/go/tools/cmd/staticcheck@v$(STATICCHECK_VERSION) || { \
		echo "Error: could not install staticcheck v$(STATICCHECK_VERSION) with $(GO_CMD) install; make staticcheck needs it (check network access to the Go module proxy)."; \
		exit 1; \
	}

## check: Run all checks (fmt, vet, lint, staticcheck, test)
check: fmt vet lint staticcheck test
	@echo "All checks passed!"

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@$(GO_GET) -v -t -d ./...
	@echo "Dependencies downloaded"

## tidy: Tidy and verify dependencies
tidy:
	@echo "Tidying dependencies..."
	@$(GO_MOD) tidy
	@$(GO_MOD) verify
	@echo "Dependencies tidied and verified"

## verify: Verify dependencies
verify:
	@echo "Verifying dependencies..."
	@$(GO_MOD) verify
	@echo "Dependencies verified"

## install: Install the binary to $GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	@$(GO_CMD) install $(LDFLAGS)
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

## run: Run the application (requires VAULT parameter)
run: build
	@if [ -z "$(VAULT)" ]; then \
		echo "Error: VAULT parameter is required"; \
		echo "Usage: make run VAULT=/path/to/vault"; \
		exit 1; \
	fi
	@echo "Running $(BINARY_NAME) with vault: $(VAULT)"
	@$(BUILD_DIR)/$(BINARY_NAME) $(VAULT)

# Advanced targets

## build-all: Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Multi-platform builds complete"

## release: Build optimized release binaries
release:
	@echo "Building release binaries..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO_BUILD) $(LDFLAGS) -a -installsuffix cgo -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Release build complete: $(BUILD_DIR)/$(BINARY_NAME)"
