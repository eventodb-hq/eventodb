.PHONY: help build test profile-baseline profile-compare benchmark-all clean

help: ## Show this help message
	@echo "EventoDB - Development Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the server binary
	@echo "Building server..."
	@mkdir -p dist
	cd golang && CGO_ENABLED=0 go build -o ../dist/eventodb ./cmd/eventodb

test: ## Run all tests
	@echo "Running tests..."
	cd golang && CGO_ENABLED=0 go test -v ./...

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

.DEFAULT_GOAL := help
