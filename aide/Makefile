# aide Makefile

.PHONY: all build test clean lint fmt install install-plugin

# Binary name
BINARY=aide

# Build directory
BUILD_DIR=../bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Version injection for local dev builds.
# Computes next-dev version from the latest git tag:
#   v0.0.4 + 21 commits → 0.0.5-dev.21+abc1234
VERSION_PKG=github.com/jmylchreest/aide/aide/internal/version
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
GIT_SHORT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LATEST_TAG := $(shell git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null)
ifdef LATEST_TAG
  _V_MAJOR   := $(shell echo $(LATEST_TAG) | sed 's/^v//' | cut -d. -f1)
  _V_MINOR   := $(shell echo $(LATEST_TAG) | sed 's/^v//' | cut -d. -f2)
  _V_PATCH   := $(shell echo $(LATEST_TAG) | sed 's/^v//' | cut -d. -f3)
  _NEXT_PATCH := $(shell echo $$(($(_V_PATCH) + 1)))
  _COMMITS   := $(shell git rev-list $(LATEST_TAG)..HEAD --count 2>/dev/null || echo 0)
  DEV_VERSION := $(_V_MAJOR).$(_V_MINOR).$(_NEXT_PATCH)-dev.$(_COMMITS)+$(GIT_SHORT)
else
  DEV_VERSION := 0.0.1-dev.0+$(GIT_SHORT)
endif

# Build flags — inject version, commit, date
LDFLAGS=-ldflags "-s -w \
  -X $(VERSION_PKG).Version=$(DEV_VERSION) \
  -X $(VERSION_PKG).Commit=$(GIT_COMMIT) \
  -X $(VERSION_PKG).Date=$(BUILD_DATE)"

all: fmt lint test build

build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/aide

build-pprof:
	@echo "Building $(BINARY) with pprof support..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -tags pprof $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/aide

build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/aide
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/aide

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/aide
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/aide

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/aide

test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	@echo "Running linter..."
	@if command -v $(GOLINT) > /dev/null; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

clean:
	@echo "Cleaning..."
	@rm -f $(BUILD_DIR)/$(BINARY)*
	@rm -f coverage.out coverage.html

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

install: build
	@echo "Installing $(BINARY)..."
	@cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || \
		cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY) 2>/dev/null || \
		echo "Could not install to GOPATH or /usr/local/bin. Binary available at $(BUILD_DIR)/$(BINARY)"

install-plugin: build
	@echo "Built $(BINARY) $(DEV_VERSION) → $(BUILD_DIR)/$(BINARY)"

# Development helpers
dev-deps:
	@echo "Installing development dependencies..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

run: build
	$(BUILD_DIR)/$(BINARY) $(ARGS)

# Docker build (optional)
docker-build:
	docker build -t aide:latest .

help:
	@echo "aide Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build        Build binary for current platform"
	@echo "  make build-pprof  Build with pprof profiling support"
	@echo "  make build-all    Build for all platforms"
	@echo "  make test         Run tests"
	@echo "  make test-coverage Run tests with coverage report"
	@echo "  make lint         Run linter"
	@echo "  make fmt          Format code"
	@echo "  make clean        Remove build artifacts"
	@echo "  make deps         Download dependencies"
	@echo "  make install        Install binary to GOPATH or /usr/local/bin"
	@echo "  make install-plugin Install dev binary to .claude-plugin/bin/"
	@echo "  make help           Show this help"
	@echo ""
	@echo "Dev version: $(DEV_VERSION)"
