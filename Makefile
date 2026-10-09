.PHONY: release build run test test-coverage vet clean fmt lint install uninstall build-mac-universal screenshots deps update-deps build-all build-linux build-windows build-darwin help

# Application name
APP_NAME := term-rest-client
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# .exe suffix on Windows
EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

# Directories
CMD_DIR := cmd/term-rest-client
BIN_DIR := bin
DIST_DIR := dist

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME)$(EXT) ./$(CMD_DIR)
	@echo "Build complete: $(BIN_DIR)/$(APP_NAME)$(EXT)"

run: build ## Build and run the application
	./$(BIN_DIR)/$(APP_NAME)$(EXT)

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) ./...

vet: ## Run go vet
	$(GOCMD) vet ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	@echo "Code formatted"

lint: ## Run linter
	@echo "Running linter..."
	@if command -v $(GOLINT) > /dev/null; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed. Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

install: ## Build and install to your PATH (macOS/Linux; INSTALL_DIR=... to override)
	@./install.sh install

uninstall: ## Remove the installed binary
	@./install.sh uninstall

build-mac-universal: ## Build one macOS binary for Intel and Apple Silicon (needs macOS lipo)
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 ./$(CMD_DIR)
	lipo -create -output $(DIST_DIR)/$(APP_NAME)-darwin-universal $(DIST_DIR)/$(APP_NAME)-darwin-amd64 $(DIST_DIR)/$(APP_NAME)-darwin-arm64
	@echo "Built $(DIST_DIR)/$(APP_NAME)-darwin-universal"

screenshots: ## Regenerate assets/screenshots/*.png (needs Node + Playwright)
	@rm -rf $(DIST_DIR)/screenshots
	SCREENSHOT_DIR=$(CURDIR)/$(DIST_DIR)/screenshots $(GOTEST) -tags screenshots -run TestScreenshots -count=1 ./internal/ui/
	NODE_PATH=$$(npm root -g) node scripts/screenshots.cjs $(DIST_DIR)/screenshots assets/screenshots

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out coverage.html
	@echo "Clean complete"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies downloaded"

update-deps: ## Update dependencies
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy
	@echo "Dependencies updated"

release: ## Build release archives for every platform into dist/ (VERSION=v1.2.3 to set the version)
	./scripts/release.sh $(VERSION)

# Cross-compilation targets
build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 ./$(CMD_DIR)
	@echo "Build complete for all platforms"

build-linux: ## Build for Linux (amd64 and arm64)
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 ./$(CMD_DIR)

build-windows: ## Build for Windows
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe ./$(CMD_DIR)

build-darwin: ## Build for macOS
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 ./$(CMD_DIR)

.DEFAULT_GOAL := help

