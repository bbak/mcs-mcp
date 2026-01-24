DIST_DIR=dist
BINARY_NAME=$(DIST_DIR)/mcs-mcp.exe
VERSION=$(shell cat VERSION)
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")$(shell if [ -n "$$(git status --porcelain)" ]; then echo "-dirty"; fi)
BUILD_DATE=$(shell date +%FT%T%z 2>/dev/null || date +%Y-%m-%dT%H:%M:%S)
PKG=mcs-mcp/cmd/mcs-mcp/commands
LDFLAGS=-ldflags "-s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildDate=$(BUILD_DATE)"

.PHONY: help build run test lint fmt verify install clean

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

## build: Build the binary
build:
	@mkdir -p $(DIST_DIR)
	@cd cmd/mcs-mcp && goversioninfo -platform-specific -ver-major $(shell echo $(VERSION) | cut -d. -f1) -ver-minor $(shell echo $(VERSION) | cut -d. -f2) -ver-patch $(shell echo $(VERSION) | cut -d. -f3) -file-version $(VERSION) -product-version $(VERSION)
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/mcs-mcp
	@cp conf/.env-example $(DIST_DIR)/.env-example

## run: Build and run the binary
run: build
	./$(BINARY_NAME)

## test: Run unit tests
test:
	go test -v ./...

## lint: Run golangci-lint
lint:
	golangci-lint run

## fmt: Run go fmt
fmt:
	go fmt ./...

## verify: Run fmt, lint, and test
verify: fmt lint test

## install: Install the binary to GOBIN
install: build
	go install $(LDFLAGS) ./cmd/mcs-mcp

## clean: Remove build artifacts
clean:
	rm -rf $(DIST_DIR)
	rm -f cmd/mcs-mcp/resource.syso
	go clean
