# Message DB Makefile
# Docker and development commands

.PHONY: help build run test docker-build docker-run docker-stop docker-clean docker-compose-up docker-compose-down docker-compose-logs qa

# Default target
help:
	@echo "Message DB - Available Commands"
	@echo "================================"
	@echo ""
	@echo "Development:"
	@echo "  make build           - Build the Go binary"
	@echo "  make run             - Run the server locally"
	@echo "  make test            - Run all tests"
	@echo "  make qa              - Run QA checks"
	@echo ""
	@echo "Docker (single container):"
	@echo "  make docker-build    - Build Docker image"
	@echo "  make docker-run      - Run Docker container"
	@echo "  make docker-stop     - Stop Docker container"
	@echo "  make docker-clean    - Remove Docker image and container"
	@echo ""
	@echo "Docker Compose (full stack):"
	@echo "  make docker-compose-up    - Start all services"
	@echo "  make docker-compose-down  - Stop all services"
	@echo "  make docker-compose-logs  - View logs"
	@echo ""

# ============================================================================
# Development Commands
# ============================================================================

build:
	@echo "Building Message DB..."
	cd golang && go build -o messagedb ./cmd/messagedb
	@echo "✅ Build complete: golang/messagedb"

run: build
	@echo "Starting Message DB server..."
	cd golang && ./messagedb --port=8080

test:
	@echo "Running tests..."
	cd golang && go test ./... -v -timeout 30s
	@echo ""
	@echo "Running external tests..."
	cd test_external && bun test tests/

qa:
	@echo "Running QA checks..."
	./bin/qa_check.sh

# ============================================================================
# Docker Commands (single container)
# ============================================================================

DOCKER_IMAGE := messagedb
DOCKER_TAG := latest
DOCKER_CONTAINER := messagedb-server

docker-build:
	@echo "Building Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "✅ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run: docker-build
	@echo "Running Docker container: $(DOCKER_CONTAINER)"
	docker run -d \
		--name $(DOCKER_CONTAINER) \
		-p 8080:8080 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "✅ Container started: $(DOCKER_CONTAINER)"
	@echo "   Health check: http://localhost:8080/health"

docker-stop:
	@echo "Stopping Docker container: $(DOCKER_CONTAINER)"
	docker stop $(DOCKER_CONTAINER) 2>/dev/null || true
	docker rm $(DOCKER_CONTAINER) 2>/dev/null || true
	@echo "✅ Container stopped"

docker-clean: docker-stop
	@echo "Removing Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) 2>/dev/null || true
	@echo "✅ Cleanup complete"

docker-logs:
	docker logs -f $(DOCKER_CONTAINER)

docker-shell:
	docker exec -it $(DOCKER_CONTAINER) /bin/sh

# ============================================================================
# Docker Compose Commands (full stack)
# ============================================================================

docker-compose-up:
	@echo "Starting Message DB stack with Docker Compose..."
	docker-compose up -d
	@echo "✅ Stack started"
	@echo "   Message DB: http://localhost:8080"
	@echo "   Postgres:   localhost:5432"

docker-compose-down:
	@echo "Stopping Message DB stack..."
	docker-compose down
	@echo "✅ Stack stopped"

docker-compose-logs:
	docker-compose logs -f

docker-compose-restart:
	docker-compose restart

docker-compose-ps:
	docker-compose ps

docker-compose-clean:
	@echo "Removing all containers and volumes..."
	docker-compose down -v
	@echo "✅ Cleanup complete"

# ============================================================================
# Testing Docker
# ============================================================================

docker-test: docker-build
	@echo "Testing Docker container..."
	@echo ""
	@echo "1. Starting container..."
	docker run -d --name $(DOCKER_CONTAINER)-test -p 8081:8080 $(DOCKER_IMAGE):$(DOCKER_TAG)
	@sleep 3
	@echo ""
	@echo "2. Checking health endpoint..."
	curl -s http://localhost:8081/health || (docker stop $(DOCKER_CONTAINER)-test && docker rm $(DOCKER_CONTAINER)-test && exit 1)
	@echo ""
	@echo ""
	@echo "3. Checking version endpoint..."
	curl -s http://localhost:8081/version
	@echo ""
	@echo ""
	@echo "4. Stopping test container..."
	docker stop $(DOCKER_CONTAINER)-test
	docker rm $(DOCKER_CONTAINER)-test
	@echo ""
	@echo "✅ Docker tests passed!"
