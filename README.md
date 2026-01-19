# restapi-example

A production-ready REST API and WebSocket server built with Go. This project demonstrates best practices for building scalable, maintainable, and observable HTTP services.

## Features

- **RESTful API** - Full CRUD operations for item management
- **WebSocket Support** - Real-time communication with automatic random value streaming
- **Prometheus Metrics** - Built-in observability with HTTP request metrics
- **Structured Logging** - JSON-formatted logs using Zap logger
- **Graceful Shutdown** - Proper handling of shutdown signals with connection draining
- **CORS Support** - Configurable Cross-Origin Resource Sharing
- **Request Tracing** - Automatic request ID generation and propagation
- **Docker Ready** - Multi-stage Dockerfile with security best practices
- **In-Memory Storage** - Thread-safe storage implementation (easily replaceable)

## Project Structure

```
.
├── cmd/
│   └── server/          # Application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── handler/         # HTTP and WebSocket handlers
│   ├── middleware/      # HTTP middleware (logging, metrics, CORS, etc.)
│   ├── model/           # Data models and validation
│   ├── server/          # HTTP server setup
│   └── store/           # Data storage interface and implementations
├── test/
│   └── functional/      # Functional/integration tests
├── Dockerfile           # Multi-stage Docker build
├── Makefile             # Build automation
└── README.md
```

## Quick Start

### Prerequisites

- Go 1.25.5 or later
- Docker (optional, for containerized deployment)

### Running Locally

```bash
# Build and run
make run

# Or run directly with Go
go run ./cmd/server
```

The server will start on `http://localhost:8080` by default.

### Running with Docker

```bash
# Build Docker image
make docker-build

# Run container
make docker-run
```

## Configuration

Configuration is managed through environment variables. Environment variables take priority over default values.

| Variable | Description | Default |
|----------|-------------|---------|
| `APP_SERVER_PORT` | HTTP server port | `8080` |
| `APP_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `APP_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout | `30s` |
| `APP_METRICS_ENABLED` | Enable Prometheus metrics | `true` |
| `APP_OTLP_ENDPOINT` | OpenTelemetry collector endpoint | `` |

### Example

```bash
export APP_SERVER_PORT=3000
export APP_LOG_LEVEL=debug
export APP_METRICS_ENABLED=true
./bin/server
```

## API Endpoints

### Health Check

Check the health status of the service.

```
GET /health
```

**Response:**
```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "version": "1.0.0"
  }
}
```

---

### Items API

#### List All Items

Retrieve all items from the store.

```
GET /api/v1/items
```

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Example Item",
      "description": "An example item description",
      "price": 29.99,
      "created_at": "2026-01-19T10:00:00Z",
      "updated_at": "2026-01-19T10:00:00Z"
    }
  ]
}
```

---

#### Get Item by ID

Retrieve a specific item by its ID.

```
GET /api/v1/items/{id}
```

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Item UUID |

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Example Item",
    "description": "An example item description",
    "price": 29.99,
    "created_at": "2026-01-19T10:00:00Z",
    "updated_at": "2026-01-19T10:00:00Z"
  }
}
```

**Error Response (404):**
```json
{
  "code": 404,
  "message": "item not found"
}
```

---

#### Create Item

Create a new item.

```
POST /api/v1/items
```

**Request Body:**
```json
{
  "name": "New Item",
  "description": "Item description (optional)",
  "price": 19.99
}
```

**Validation Rules:**
| Field | Rule |
|-------|------|
| `name` | Required, max 255 characters |
| `description` | Optional, max 1000 characters |
| `price` | Required, must be non-negative |

**Response (201 Created):**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "New Item",
    "description": "Item description",
    "price": 19.99,
    "created_at": "2026-01-19T10:00:00Z",
    "updated_at": "2026-01-19T10:00:00Z"
  }
}
```

**Error Response (400):**
```json
{
  "code": 400,
  "message": "name cannot be empty"
}
```

---

#### Update Item

Update an existing item.

```
PUT /api/v1/items/{id}
```

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Item UUID |

**Request Body:**
```json
{
  "name": "Updated Item",
  "description": "Updated description",
  "price": 24.99
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Updated Item",
    "description": "Updated description",
    "price": 24.99,
    "created_at": "2026-01-19T10:00:00Z",
    "updated_at": "2026-01-19T11:00:00Z"
  }
}
```

---

#### Delete Item

Delete an item by its ID.

```
DELETE /api/v1/items/{id}
```

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Item UUID |

**Response:** `204 No Content`

**Error Response (404):**
```json
{
  "code": 404,
  "message": "item not found"
}
```

---

### WebSocket Endpoint

Connect to receive real-time random value updates.

```
GET /ws
```

**Connection:** Upgrade to WebSocket protocol

**Message Format (Server -> Client):**
```json
{
  "type": "random_value",
  "value": 1234567890,
  "timestamp": "2026-01-19T10:00:00Z"
}
```

**Features:**
- Sends random values every 1 second
- Automatic ping/pong for connection health
- Graceful close on server shutdown

**Example (JavaScript):**
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data.type, data.value);
};

ws.onclose = () => {
  console.log('Connection closed');
};
```

---

### Metrics Endpoint

Prometheus metrics endpoint (when enabled).

```
GET /metrics
```

**Available Metrics:**
| Metric | Type | Description |
|--------|------|-------------|
| `http_requests_total` | Counter | Total HTTP requests by method, path, status |
| `http_request_duration_seconds` | Histogram | Request duration distribution |
| `http_requests_in_flight` | Gauge | Current number of requests being processed |

---

## HTTP Headers

### Request Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | Should be `application/json` for POST/PUT requests |
| `X-Request-ID` | Optional request ID for tracing (auto-generated if not provided) |

### Response Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` |
| `X-Request-ID` | Request ID for tracing |
| `Access-Control-Allow-Origin` | CORS origin header |

---

## Error Responses

All error responses follow a consistent format:

```json
{
  "code": 400,
  "message": "error description"
}
```

### HTTP Status Codes

| Code | Description |
|------|-------------|
| `200` | Success |
| `201` | Created |
| `204` | No Content (successful deletion) |
| `400` | Bad Request (validation error) |
| `404` | Not Found |
| `409` | Conflict (resource already exists) |
| `500` | Internal Server Error |

---

## Development

### Available Make Commands

```bash
make help              # Show all available commands
make build             # Build the binary
make run               # Build and run the server
make test              # Run unit tests
make test-coverage     # Run tests with coverage report
make test-functional   # Run functional tests
make test-all          # Run all tests
make lint              # Run linter
make lint-fix          # Run linter with auto-fix
make fmt               # Format code
make docker-build      # Build Docker image
make docker-run        # Run Docker container
make clean             # Clean build artifacts
make install-tools     # Install development tools
```

### Running Tests

```bash
# Unit tests
make test

# With coverage
make test-coverage

# Functional tests
make test-functional

# All tests
make test-all
```

### Code Quality

```bash
# Install tools
make install-tools

# Run linter
make lint

# Format code
make fmt

# Vulnerability check
make vuln
```

---

## Architecture

### Middleware Chain

Requests flow through the following middleware (in order):

1. **Recovery** - Catches panics and returns 500 error
2. **RequestID** - Generates/propagates request IDs
3. **Metrics** - Records Prometheus metrics (if enabled)
4. **Logging** - Logs request details
5. **CORS** - Handles cross-origin requests

### Storage Interface

The `Store` interface allows easy swapping of storage backends:

```go
type Store interface {
    List(ctx context.Context) ([]model.Item, error)
    Get(ctx context.Context, id string) (*model.Item, error)
    Create(ctx context.Context, item *model.Item) (*model.Item, error)
    Update(ctx context.Context, id string, item *model.Item) (*model.Item, error)
    Delete(ctx context.Context, id string) error
}
```

Currently implemented:
- `MemoryStore` - Thread-safe in-memory storage

---

## Docker

### Build Image

```bash
docker build -t restapi-example:latest .
```

### Run Container

```bash
docker run -p 8080:8080 \
  -e APP_LOG_LEVEL=info \
  -e APP_METRICS_ENABLED=true \
  restapi-example:latest
```

### Health Check

The Docker image includes a built-in health check that queries `/health` every 30 seconds.

---

## License

MIT License - see LICENSE file for details.
