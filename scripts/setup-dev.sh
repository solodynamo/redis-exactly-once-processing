#!/bin/bash

# Development setup script for Redis Timeout Tracking POC
set -e

echo "🛠️  Setting up Redis Timeout Tracking POC Development Environment"
echo "=================================================================="

# Check prerequisites
echo "📋 Checking prerequisites..."

# Check Go version
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
if [[ $(echo "$GO_VERSION < 1.21" | bc -l) -eq 1 ]]; then
    echo "❌ Go version $GO_VERSION is too old. Please install Go 1.21 or later."
    exit 1
fi
echo "✅ Go version $GO_VERSION detected"

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker."
    exit 1
fi
echo "✅ Docker detected"

# Check Docker Compose
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose is not installed. Please install Docker Compose."
    exit 1
fi
echo "✅ Docker Compose detected"

# Check if jq is available (for demo scripts)
if ! command -v jq &> /dev/null; then
    echo "⚠️  jq is not installed. Demo scripts may not work properly."
    echo "   Install jq for better demo experience: brew install jq (macOS) or apt-get install jq (Ubuntu)"
else
    echo "✅ jq detected"
fi

echo ""
echo "🔧 Setting up project..."

# Install Go dependencies
echo "📦 Installing Go dependencies..."
go mod download
go mod tidy
echo "✅ Go dependencies installed"

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file..."
    cp .env.example .env
    echo "✅ .env file created (you can customize it if needed)"
else
    echo "✅ .env file already exists"
fi

# Start Redis with Docker
echo "🚀 Starting Redis with Docker..."
docker-compose up -d redis

# Wait for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
timeout=30
counter=0
while ! docker-compose exec -T redis redis-cli ping > /dev/null 2>&1; do
    if [ $counter -ge $timeout ]; then
        echo "❌ Redis failed to start within $timeout seconds"
        exit 1
    fi
    sleep 1
    counter=$((counter + 1))
done
echo "✅ Redis is ready"

# Build the applications
echo "🔨 Building applications..."
make build
echo "✅ Applications built successfully"

# Run tests to verify everything works
echo "🧪 Running tests..."
if go test ./... -v; then
    echo "✅ All tests passed"
else
    echo "⚠️  Some tests failed. This might be expected if Redis is not running locally."
fi

echo ""
echo "🎉 Development environment setup complete!"
echo ""
echo "🚀 Quick Start Commands:"
echo "  1. Run Phase 1: make run-phase1"
echo "  2. Run Phase 2: make docker-up (starts all services)"
echo "  3. Run demo: ./scripts/demo.sh"
echo "  4. Run Phase 2 demo: ./scripts/demo-phase2.sh"
echo "  5. Run load test: make load-test"
echo ""
echo "🔍 Useful Commands:"
echo "  - Check Redis: make redis-cli"
echo "  - Monitor Redis: make redis-monitor"
echo "  - View logs: make docker-logs"
echo "  - Stop all: make docker-down"
echo ""
echo "📚 Documentation:"
echo "  - README.md: General overview and usage"
echo "  - docs/ARCHITECTURE.md: Detailed architecture"
echo "  - examples/redis-commands.md: Redis debugging commands"
echo "  - examples/api-examples.http: API testing examples"
echo ""
echo "🏥 Health Checks:"
echo "  - Phase 1: http://localhost:8080/health"
echo "  - Phase 2 Leader: http://localhost:8081/health"
echo "  - Phase 2 Consumer 1: http://localhost:8082/health"
echo "  - Phase 2 Consumer 2: http://localhost:8083/health"
echo ""
echo "📊 Metrics:"
echo "  - Phase 1: http://localhost:8080/metrics"
echo "  - Phase 2 Leader: http://localhost:8081/metrics"
echo ""
echo "Happy coding! 🎯" 