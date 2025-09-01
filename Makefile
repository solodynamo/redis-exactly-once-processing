.PHONY: build test clean docker-up docker-down run-phase1 run-phase2 load-test

# Build targets
build: build-phase1 build-phase2

build-phase1:
	go build -o bin/phase1 cmd/phase1/main.go

build-phase2:
	go build -o bin/phase2 cmd/phase2/main.go

# Test targets
test:
	go test ./... -v

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Development targets
run-phase1:
	go run cmd/phase1/main.go

run-phase2:
	go run cmd/phase2/main.go

# Docker targets
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-build:
	docker-compose build

# Load testing
load-test:
	go run tests/load-test/main.go

# Cleanup
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Development setup
dev-setup: deps
	@echo "Setting up development environment..."
	@echo "1. Install dependencies: make deps"
	@echo "2. Start Redis: make docker-up"
	@echo "3. Run Phase 1: make run-phase1"
	@echo "4. Run Phase 2: make run-phase2"
	@echo "5. Run tests: make test"

# Local development
local-build:
	mkdir -p bin
	go build -o bin/phase1 cmd/phase1/main.go
	go build -o bin/phase2 cmd/phase2/main.go

setup:
	./scripts/setup-dev.sh

test-multi-leader:
	./scripts/test-multi-leader.sh

local-run-phase1: local-build
	@echo "Starting Phase 1 locally..."
	@echo "Make sure Redis is running: make redis-start or make native-redis-start"
	@if [ -f .env.local ]; then source .env.local; fi && ./bin/phase1

local-run-phase2: local-build
	@echo "Starting Phase 2 locally..."
	@echo "Make sure Redis is running: make redis-start or make native-redis-start"
	@if [ -f .env.local ]; then source .env.local; fi && ./bin/phase2

# Container-based Redis (Docker/Podman)
redis-start:
	@if command -v podman-compose >/dev/null 2>&1; then \
		podman-compose up -d redis; \
	elif command -v podman >/dev/null 2>&1; then \
		podman run -d --name redis-timeout-poc -p 6379:6379 redis:7-alpine; \
	else \
		docker-compose up -d redis; \
	fi
	@echo "Waiting for Redis to be ready..."
	@sleep 3

redis-stop:
	@if command -v podman-compose >/dev/null 2>&1; then \
		podman-compose down redis; \
	elif command -v podman >/dev/null 2>&1; then \
		podman stop redis-timeout-poc && podman rm redis-timeout-poc; \
	else \
		docker-compose down redis; \
	fi

# Native Redis (installed locally)
native-redis-start:
	@if redis-cli ping >/dev/null 2>&1; then \
		echo "✅ Redis is already running"; \
	else \
		echo "🚀 Starting native Redis..."; \
		mkdir -p redis-data; \
		redis-server --daemonize yes --port 6379 --maxmemory 256mb --maxmemory-policy allkeys-lru --appendonly yes --dir ./redis-data --logfile ./redis-data/redis.log; \
		sleep 2; \
		echo "✅ Redis started"; \
	fi

native-redis-stop:
	@redis-cli shutdown || echo "Redis was not running"

# Redis utilities (works with both container and native)
redis-cli:
	@if docker-compose ps redis >/dev/null 2>&1 && docker-compose exec redis redis-cli ping >/dev/null 2>&1; then \
		docker-compose exec redis redis-cli; \
	elif command -v podman >/dev/null 2>&1 && podman exec redis-timeout-poc redis-cli ping >/dev/null 2>&1; then \
		podman exec -it redis-timeout-poc redis-cli; \
	else \
		redis-cli; \
	fi

redis-monitor:
	@if docker-compose ps redis >/dev/null 2>&1 && docker-compose exec redis redis-cli ping >/dev/null 2>&1; then \
		docker-compose exec redis redis-cli monitor; \
	elif command -v podman >/dev/null 2>&1 && podman exec redis-timeout-poc redis-cli ping >/dev/null 2>&1; then \
		podman exec -it redis-timeout-poc redis-cli monitor; \
	else \
		redis-cli monitor; \
	fi

redis-info:
	@if docker-compose ps redis >/dev/null 2>&1 && docker-compose exec redis redis-cli ping >/dev/null 2>&1; then \
		docker-compose exec redis redis-cli info; \
	elif command -v podman >/dev/null 2>&1 && podman exec redis-timeout-poc redis-cli ping >/dev/null 2>&1; then \
		podman exec -it redis-timeout-poc redis-cli info; \
	else \
		redis-cli info; \
	fi

# Monitoring
metrics-phase1:
	curl http://localhost:8080/metrics

metrics-phase2:
	curl http://localhost:8081/metrics

health-phase1:
	curl http://localhost:8080/health

health-phase2:
	curl http://localhost:8081/health 