.PHONY: help build test test-race test-cover clean run dev docker-up docker-down docker-logs docker-restart lint vet fmt check install-deps

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Build targets
build: ## Build the exporter binary
	@echo "Building exporter..."
	@go build -o bin/exporter ./cmd/main.go
	@echo "Build complete: bin/exporter"

build-linux: ## Build for Linux (cross-compile from other OS)
	@echo "Building for Linux amd64..."
	@GOOS=linux GOARCH=amd64 go build -o bin/exporter-linux-amd64 ./cmd/main.go

install-deps: ## Download and install Go dependencies
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Test targets
test: ## Run all tests
	@echo "Running tests..."
	@go test ./...

test-verbose: ## Run tests with verbose output
	@go test -v ./...

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	@go test -race ./...

test-cover: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Code quality targets
vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

fmt: ## Format code with gofmt
	@echo "Formatting code..."
	@gofmt -w -s .

check: vet test ## Run vet and tests

lint: ## Run golangci-lint (if installed)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

# Run targets
run: ## Run the exporter locally (requires .env)
	@echo "Starting exporter..."
	@./bin/exporter --config config.yaml

dev: build run ## Build and run locally

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker compose build

docker-up: ## Start Docker Compose stack (Mimir + exporter)
	@echo "Starting Docker Compose stack..."
	@docker compose up -d
	@echo "Stack started. Check logs with: make docker-logs"

docker-down: ## Stop Docker Compose stack
	@echo "Stopping Docker Compose stack..."
	@docker compose down

docker-logs: ## Show Docker Compose logs (follow mode)
	@docker compose logs -f

docker-logs-exporter: ## Show exporter logs only
	@docker compose logs -f exporter

docker-restart: ## Restart Docker Compose stack
	@echo "Restarting Docker Compose stack..."
	@docker compose restart

docker-clean: ## Stop and remove all containers, volumes, and images
	@echo "Cleaning up Docker resources..."
	@docker compose down -v --remove-orphans

# Health check targets
health: ## Check /healthz endpoint
	@curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:8080/healthz

ready: ## Check /readyz endpoint
	@curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:8080/readyz

query-mimir: ## Query Mimir for meta_ads_spend metric
	@curl -s 'http://localhost:9009/prometheus/api/v1/query?query=meta_ads_spend' | jq

# Cleanup targets
clean: ## Clean build artifacts and test cache
	@echo "Cleaning..."
	@rm -f bin/exporter bin/exporter-*
	@rm -f coverage.out coverage.html
	@go clean -testcache
	@echo "Clean complete"

clean-all: clean docker-clean ## Clean everything including Docker resources
