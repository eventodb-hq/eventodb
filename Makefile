.PHONY: help build test test-race profile-baseline profile-compare benchmark-all clean

# Fix for Xcode 16 + Go race detector compatibility
export CGO_CFLAGS := -Wno-error=nullability-completeness -Wno-error=availability

help: ## Show this help message
	@echo "EventoDB - Development Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

VERSION := $(shell cat VERSION 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build: ## Build the server binary
	@echo "Building server..."
	@mkdir -p dist
	cd golang && CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../dist/eventodb ./cmd/eventodb

test: ## Run all tests
	@echo "Running tests..."
	cd golang && CGO_ENABLED=0 go test -v ./...

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	cd golang && go test -race -v ./...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	cd golang && CGO_ENABLED=0 go test -v -tags=integration ./...

benchmark-all: ## Run all Go benchmarks
	@echo "Running benchmarks..."
	cd golang && CGO_ENABLED=0 go test -bench=. -benchmem -benchtime=5s ./...

profile-baseline: ## Run baseline performance profiling
	@echo "Running baseline performance profile..."
	@chmod +x scripts/profile-baseline.sh
	@./scripts/profile-baseline.sh

profile-compare: ## Compare two profile runs (BASELINE=dir1 OPTIMIZED=dir2)
	@if [ -z "$(BASELINE)" ] || [ -z "$(OPTIMIZED)" ]; then \
		echo "Error: BASELINE and OPTIMIZED variables required"; \
		echo "Usage: make profile-compare BASELINE=profiles/xxx OPTIMIZED=profiles/yyy"; \
		exit 1; \
	fi
	@chmod +x scripts/compare-profiles.sh
	@./scripts/compare-profiles.sh $(BASELINE) $(OPTIMIZED)

clean: ## Clean build artifacts and profiles
	@echo "Cleaning..."
	rm -rf dist/
	rm -rf profiles/
	cd golang && go clean

run-dev: ## Run server in development mode (test mode)
	@echo "Starting server in development mode..."
	cd golang && go run ./cmd/eventodb --test-mode --log-level debug

run-prod: ## Run server in production mode (requires DB_URL)
	@if [ -z "$(DB_URL)" ]; then \
		echo "Error: DB_URL required"; \
		echo "Usage: make run-prod DB_URL=postgres://..."; \
		exit 1; \
	fi
	@echo "Starting server in production mode..."
	cd golang && go run ./cmd/eventodb --db-url $(DB_URL) --log-level info

goreleaser-snapshot: ## Build snapshot release (no publish)
	@echo "Building snapshot..."
	goreleaser release --snapshot --clean

goreleaser-release: ## Build and publish release
	@echo "Building release..."
	goreleaser release --clean

.DEFAULT_GOAL := help
