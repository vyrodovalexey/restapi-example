# Makefile for restapi-example
# REST API and WebSocket server

# ==============================================================================
# Variables
# ==============================================================================

# Binary configuration
BINARY_NAME := server
BINARY_DIR := bin
BINARY_PATH := $(BINARY_DIR)/$(BINARY_NAME)

# Go configuration
GO := go
GOFLAGS := -v
CGO_ENABLED := 0

# Version information (from git)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Linker flags for version injection
LDFLAGS := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildTime=$(BUILD_TIME)"

# Docker configuration
DOCKER_REGISTRY ?= ghcr.io
DOCKER_REPO ?= user/restapi-example
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(DOCKER_REPO)
DOCKER_TAG ?= $(VERSION)

# Test configuration
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
COVERAGE_THRESHOLD := 70

# Tools
GOLANGCI_LINT := golangci-lint
GOVULNCHECK := govulncheck

# ==============================================================================
# Default target
# ==============================================================================

.DEFAULT_GOAL := help

# ==============================================================================
# Build targets
# ==============================================================================

.PHONY: all
all: lint test build ## Run lint, test, and build

.PHONY: build
build: $(BINARY_DIR) ## Build the binary to bin/server
	@echo "==> Building $(BINARY_PATH)..."
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./cmd/server

$(BINARY_DIR):
	@mkdir -p $(BINARY_DIR)

.PHONY: build-linux
build-linux: $(BINARY_DIR) ## Build for Linux (amd64)
	@echo "==> Building $(BINARY_PATH) for Linux..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./cmd/server

.PHONY: build-darwin
build-darwin: $(BINARY_DIR) ## Build for macOS (arm64)
	@echo "==> Building $(BINARY_PATH) for macOS..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./cmd/server

# ==============================================================================
# Test targets
# ==============================================================================

.PHONY: test
test: ## Run unit tests
	@echo "==> Running unit tests..."
	$(GO) test -race -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "==> Running tests with coverage..."
	$(GO) test -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@echo "==> Generating coverage report..."
	$(GO) tool cover -func=$(COVERAGE_FILE)
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "==> Coverage report generated: $(COVERAGE_HTML)"

.PHONY: test-coverage-check
test-coverage-check: test-coverage ## Run tests and check coverage threshold
	@echo "==> Checking coverage threshold ($(COVERAGE_THRESHOLD)%)..."
	@coverage=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$coverage < $(COVERAGE_THRESHOLD)" | bc -l) -eq 1 ]; then \
		echo "Coverage $$coverage% is below threshold $(COVERAGE_THRESHOLD)%"; \
		exit 1; \
	else \
		echo "Coverage $$coverage% meets threshold $(COVERAGE_THRESHOLD)%"; \
	fi

.PHONY: test-functional
test-functional: ## Run functional tests
	@echo "==> Running functional tests..."
	$(GO) test -race -v -tags=functional ./test/functional/...

.PHONY: test-functional-coverage
test-functional-coverage: ## Run functional tests with coverage
	@echo "==> Running functional tests with coverage..."
	$(GO) test -race -coverprofile=coverage-functional.out -covermode=atomic -tags=functional ./test/functional/...

.PHONY: test-integration
test-integration: ## Run integration tests (requires docker-compose)
	@echo "==> Running integration tests..."
	$(GO) test -race -v -tags=integration ./test/integration/...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (requires docker-compose)
	@echo "==> Running end-to-end tests..."
	$(GO) test -race -v -tags=e2e ./test/e2e/...

.PHONY: test-performance
test-performance: ## Run performance/benchmark tests
	@echo "==> Running performance tests..."
	$(GO) test -bench=. -benchmem -tags=performance ./test/performance/...

.PHONY: test-all-coverage
test-all-coverage: ## Run all tests with combined coverage
	@echo "==> Running all tests with coverage..."
	$(GO) test -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	$(GO) tool cover -func=$(COVERAGE_FILE)

.PHONY: test-all
test-all: test test-functional ## Run all tests (unit + functional)

# ==============================================================================
# Test environment targets (docker-compose)
# ==============================================================================

.PHONY: test-env-up
test-env-up: ## Start test environment (docker compose)
	@echo "==> Starting test environment..."
	docker compose -f test/docker-compose/docker-compose.yml --env-file test/docker-compose/.env.test up -d

.PHONY: test-env-status
test-env-status: ## Show test environment status
	@echo "==> Test environment status..."
	docker compose -f test/docker-compose/docker-compose.yml --env-file test/docker-compose/.env.test ps

.PHONY: test-env-wait
test-env-wait: ## Wait for test environment to be ready
	@echo "==> Waiting for test environment..."
	@./test/docker-compose/scripts/wait-for-services.sh || echo "Warning: wait script not found or failed"

.PHONY: test-env-down
test-env-down: ## Stop test environment
	@echo "==> Stopping test environment..."
	docker compose -f test/docker-compose/docker-compose.yml --env-file test/docker-compose/.env.test down -v

.PHONY: test-env-logs
test-env-logs: ## Show test environment logs
	docker compose -f test/docker-compose/docker-compose.yml --env-file test/docker-compose/.env.test logs -f

# ==============================================================================
# Code quality targets
# ==============================================================================

.PHONY: lint
lint: ## Run golangci-lint
	@echo "==> Running linter..."
	$(GOLANGCI_LINT) run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	@echo "==> Running linter with auto-fix..."
	$(GOLANGCI_LINT) run --fix ./...

.PHONY: vuln
vuln: ## Run govulncheck for vulnerability scanning
	@echo "==> Running vulnerability check..."
	$(GOVULNCHECK) ./...

.PHONY: fmt
fmt: ## Format Go code
	@echo "==> Formatting code..."
	$(GO) fmt ./...
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	@echo "==> Running go vet..."
	$(GO) vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	@echo "==> Tidying modules..."
	$(GO) mod tidy

.PHONY: verify
verify: ## Verify dependencies
	@echo "==> Verifying dependencies..."
	$(GO) mod verify

# ==============================================================================
# Docker targets
# ==============================================================================

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "==> Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.

.PHONY: docker-push
docker-push: ## Push Docker image to registry
	@echo "==> Pushing Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest

.PHONY: docker-run
docker-run: ## Run Docker container locally
	@echo "==> Running Docker container..."
	docker run --rm -p 8080:8080 $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-scan
docker-scan: ## Scan Docker image for vulnerabilities with Trivy
	@echo "==> Scanning Docker image with Trivy..."
	trivy image --severity HIGH,CRITICAL $(DOCKER_IMAGE):$(DOCKER_TAG)

# ==============================================================================
# Development targets
# ==============================================================================

.PHONY: run
run: build ## Build and run the server
	@echo "==> Running server..."
	./$(BINARY_PATH)

.PHONY: dev
dev: ## Run with hot reload (requires air)
	@echo "==> Running with hot reload..."
	air

.PHONY: install-tools
install-tools: ## Install development tools
	@echo "==> Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/air-verse/air@latest

# ==============================================================================
# Clean targets
# ==============================================================================

.PHONY: clean
clean: ## Clean build artifacts
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BINARY_DIR)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	rm -f coverage-functional.out coverage-integration.out
	$(GO) clean -cache -testcache

.PHONY: clean-docker
clean-docker: ## Remove Docker images
	@echo "==> Removing Docker images..."
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest 2>/dev/null || true

# ==============================================================================
# Help target
# ==============================================================================

.PHONY: help
help: ## Show available targets
	@echo "restapi-example - REST API and WebSocket Server"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Variables:"
	@echo "  VERSION        Version tag (default: git describe)"
	@echo "  DOCKER_TAG     Docker image tag (default: VERSION)"
	@echo "  DOCKER_REGISTRY Docker registry (default: ghcr.io)"
	@echo "  DOCKER_REPO    Docker repository (default: user/restapi-example)"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    # Build the binary"
	@echo "  make test                     # Run unit tests"
	@echo "  make docker-build             # Build Docker image"
	@echo "  make VERSION=v1.0.0 all       # Build with specific version"
