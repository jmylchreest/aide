# aide-web Makefile

.PHONY: all build test clean lint fmt install web web-install

BINARY=aide-web
BUILD_DIR=../bin

GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

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

LDFLAGS=-ldflags "-s -w \
  -X $(VERSION_PKG).Version=$(DEV_VERSION) \
  -X $(VERSION_PKG).Commit=$(GIT_COMMIT) \
  -X $(VERSION_PKG).Date=$(BUILD_DATE)"

all: fmt lint test web build

web-install:
	@echo "Installing web dependencies..."
	cd web && bun install

web: web-install
	@echo "Building web frontend..."
	cd web && bun run build

build: web
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/aide-web

test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

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
	@rm -f $(BUILD_DIR)/$(BINARY)
	@rm -rf web/node_modules internal/webdist/build/_astro

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

install: build
	@echo "Installing $(BINARY)..."
	@cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || \
		cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY) 2>/dev/null || \
		echo "Could not install. Binary available at $(BUILD_DIR)/$(BINARY)"

run: build
	$(BUILD_DIR)/$(BINARY) $(ARGS)

help:
	@echo "aide-web Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make web      Build frontend (Astro)"
	@echo "  make build    Build binary (includes web)"
	@echo "  make test     Run tests"
	@echo "  make lint     Run linter"
	@echo "  make fmt      Format code"
	@echo "  make clean    Remove artifacts"
	@echo "  make install  Install binary"
	@echo "  make run      Build and run"
	@echo ""
	@echo "Dev version: $(DEV_VERSION)"
