# Redis Timeout Tracking POC

A scalable, Redis-based timeout tracking system for the Care customer support platform that tracks customer response times and sends progressive notifications.

## Overview

This POC implements a two-phase approach:
- **Phase 1**: Single leader architecture (handles up to 10K conversations/sec)
- **Phase 2**: Multi-consumer with leader election (scales to 100K+ conversations/sec)

## Architecture

### Phase 1: Single Leader
```
┌─────────────────────────────────────────────────────────────┐
│                   Kubernetes Cluster                         │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Pod 1      │  │   Pod 2      │  │   Pod N      │     │
│  │  (Leader)    │  │  (Standby)   │  │  (Standby)   │     │
│  └──────┬───────┘  └──────────────┘  └──────────────┘     │
│         ▼                                                    │
│  ┌────────────────────────────────────────────────────┐    │
│  │                    Redis                            │    │
│  │ • waiting_conversations (Sorted Set)               │    │
│  │ • notification_states (Hash)                       │    │
│  │ • timeout:leader (String with TTL)                 │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Phase 2: Multi-Consumer
```
┌──────────────────────────────────────────────────────────────┐
│  ┌──────────────┐  Leader - Timeout Detector                │
│  │   Pod 1      │  Checks for timeouts                      │
│  │  (Leader)    │  Pushes to Redis Stream                   │
│  └──────┬───────┘                                           │
│         ▼                                                     │
│  ┌────────────────────────────────────────────────────┐     │
│  │              Redis Stream: timeout_events           │     │
│  └──────┬───────────┬───────────┬────────────────────┘     │
│         │           │           │                            │
│  ┌──────▼─────┐ ┌──▼─────┐ ┌──▼─────┐                     │
│  │   Pod 1    │ │  Pod 2  │ │ Pod N   │  All pods consume  │
│  │ (Consumer) │ │(Consumer)│ │(Consumer)│  different msgs   │
│  └────────────┘ └─────────┘ └──────────┘                   │
└──────────────────────────────────────────────────────────────┘
```

## Quick Start

### 🚀 Setup and Testing
```bash
# 1. Setup development environment
./scripts/setup-dev.sh

# 2. Test multi-instance leader election
./scripts/test-multi-leader.sh
```

### 🏃 Manual Local Development
```bash
# Build applications
make local-build

# Run Phase 1 locally
make local-run-phase1

# Run Phase 2 locally
make local-run-phase2
```

### 🐳 Docker Cluster (Production-like)
```bash
make docker-up    # Start full cluster
make docker-logs  # View logs
make docker-down  # Stop cluster
```

### Prerequisites
- Go 1.21+
- Docker and Docker Compose
- `jq` (for demo scripts): `brew install jq` (macOS) or `apt-get install jq` (Ubuntu)

### Running Phase 2
make run-phase2
```

## Configuration

### Environment Variables
- `REDIS_URL`: Redis connection string (default: redis://localhost:6379)
- `TIMEOUT_INTERVAL_MS`: Base timeout interval in milliseconds (default: 30000)
- `LEADER_ELECTION_TTL`: Leader lock TTL in seconds (default: 10)
- `CHECK_INTERVAL_MS`: How often to check for timeouts in ms (default: 1000)
- `POD_ID`: Unique identifier for this pod (default: auto-generated)
- `PORT`: HTTP server port (default: 8080)

## API Endpoints

### POST /conversations/:id/agent-message
Track when an agent sends a message to start timeout monitoring.

**Request Body:**
```json
{
  "agent_id": "agent_123",
  "message_id": "msg_456",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### POST /conversations/:id/customer-response
Clear timeout tracking when customer responds.

**Request Body:**
```json
{
  "customer_id": "customer_123",
  "message_id": "msg_789",
  "timestamp": "2024-01-01T12:05:00Z"
}
```

### GET /health
Health check endpoint.

### GET /metrics
Prometheus metrics endpoint.

## Redis Data Structures

| Key | Type | Purpose | Example |
|-----|------|---------|---------|
| `waiting_conversations` | Sorted Set | Tracks waiting conversations | Score: timestamp, Member: conv_id |
| `notification_states` | Hash | Prevents duplicate notifications | Field: conv_id, Value: level (1,2,3) |
| `timeout:leader` | String | Leader election lock | Value: pod_id, TTL: 10s |
| `metrics:timeouts` | Hash | Monitoring metrics | Fields: total, level1, level2, level3 |
| `timeout_events` | Stream | Phase 2 event queue | Messages with conversation timeouts |

## Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Load testing
make load-test
```

## Monitoring

Key metrics tracked:
- `waiting_conversations_count`: Current conversations being tracked
- `timeout_notifications_sent`: Notifications sent by level
- `timeout_leader_changes`: Number of leader changes
- `timeout_check_duration`: Performance of timeout checks

## Production Considerations

- Monitor Redis memory usage
- Set up Redis persistence (RDB + AOF)
- Configure Redis cluster for high availability
- Implement circuit breakers for external services
- Set up proper logging and alerting
- Use Redis connection pooling
- Implement graceful shutdown handling

## Development

```bash
# Format code
make fmt

# Lint code
make lint

# Build binaries
make build

# Clean up
make clean
``` 