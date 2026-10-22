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

# Tool versions. This is the single source of truth for the golangci-lint pin:
# CI reads it via `make print-golangci-lint-version`. No leading "v" - it is compared
# with sort -V against `golangci-lint version --short`, which prints no prefix.
#
# Why the pin moves with the go directive (canonical explanation; the workflow and
# README point here):
#  - golangci-lint refuses a module whose language version (major.minor) is newer than
#    the Go it was built with: "can't load config: the Go language version (goX) used
#    to build golangci-lint is lower than the targeted Go version (Y)".
#  - golangci-lint and staticcheck embed golang.org/x/tools' export-data reader, so a Go
#    release that changes the export data format needs tools built against a newer
#    x/tools, even across patch releases. go1.27.2 moved to export data version 5:
#    staticcheck fails with "export data version 5 is greater than maximum supported
#    version 4", golangci-lint with spurious typecheck errors. golangci-lint 2.13.x
#    fails on it (2.14.0 happened to work when tested). The pin tracks the current
#    release; make lint requires at least the pin, not the oldest release that
#    works, so local and CI stay on the same line (CI runs exactly the pin; a newer
#    local release may lint differently).
GOLANGCI_LINT_VERSION=2.15.0

# Build parameters
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Directories
BUILD_DIR=build
COVERAGE_DIR=coverage

.PHONY: all build clean test lint fmt vet deps help install run check coverage tidy verify print-golangci-lint-version

# Default target
all: check build

## help: Display this help message
help:
	@echo "Available targets:"
	@echo "  make build        - Build the binary"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make coverage     - Run tests with coverage report"
	@echo "  make lint         - Run linters (golangci-lint)"
	@echo "  make fmt          - Format code"
	@echo "  make vet          - Run go vet"
	@echo "  make check        - Run all checks (fmt, vet, lint, test)"
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

## test-full: Run all tests including the large-frame transport test (what CI runs)
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

## lint: Run golangci-lint
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		installed=$$(golangci-lint version --short 2>/dev/null); \
		case "$$installed" in \
			''|*[!0-9.]*) \
				echo "Could not read a version number from 'golangci-lint version --short' (got: '$$installed')."; \
				echo "This module needs golangci-lint $(GOLANGCI_LINT_VERSION) or newer, the version CI uses (the comment above GOLANGCI_LINT_VERSION in the Makefile explains why)."; \
				echo "Check your install: https://golangci-lint.run/docs/welcome/install/"; \
				exit 1; \
				;; \
		esac; \
		oldest=$$(printf '%s\n%s\n' "$(GOLANGCI_LINT_VERSION)" "$$installed" | sort -V | head -1); \
		if [ "$$oldest" != "$(GOLANGCI_LINT_VERSION)" ]; then \
			echo "golangci-lint $$installed is older than the pinned $(GOLANGCI_LINT_VERSION), which CI runs; install at least that version."; \
			echo "Releases older than the pin may also fail to read the export data your toolchain ($$($(GO_CMD) env GOVERSION)) writes,"; \
			echo "or may fail golangci-lint's own language-version check; see the comment above GOLANGCI_LINT_VERSION in the Makefile."; \
			echo "Upgrade to $(GOLANGCI_LINT_VERSION) or newer: https://golangci-lint.run/docs/welcome/install/"; \
			exit 1; \
		fi; \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install $(GOLANGCI_LINT_VERSION) or newer from https://golangci-lint.run/docs/welcome/install/"; \
		echo "Running basic checks instead..."; \
		$(MAKE) fmt vet; \
	fi

## print-golangci-lint-version: Print the pinned golangci-lint version (read by CI)
print-golangci-lint-version:
	@echo $(GOLANGCI_LINT_VERSION)

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
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
