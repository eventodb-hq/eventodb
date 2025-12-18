.PHONY: help build test profile-baseline profile-compare benchmark-all clean

help: ## Show this help message
	@echo "Message DB - Development Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the server binary
	@echo "Building server..."
	cd golang && go build -o ../bin/messagedb ./cmd/messagedb

test: ## Run all tests
	@echo "Running tests..."
	cd golang && go test -v ./...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	cd golang && go test -v -tags=integration ./...

benchmark-all: ## Run all Go benchmarks
	@echo "Running benchmarks..."
	cd golang && go test -bench=. -benchmem -benchtime=5s ./...

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
	rm -rf bin/
	rm -rf profiles/
	cd golang && go clean

run-dev: ## Run server in development mode (test mode)
	@echo "Starting server in development mode..."
	cd golang && go run ./cmd/messagedb --test-mode --log-level debug

run-prod: ## Run server in production mode (requires DB_URL)
	@if [ -z "$(DB_URL)" ]; then \
		echo "Error: DB_URL required"; \
		echo "Usage: make run-prod DB_URL=postgres://..."; \
		exit 1; \
	fi
	@echo "Starting server in production mode..."
	cd golang && go run ./cmd/messagedb --db-url $(DB_URL) --log-level info

.DEFAULT_GOAL := help
