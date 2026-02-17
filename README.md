# restapi-example

A production-ready REST API and WebSocket server built with Go. This project demonstrates best practices for building scalable, maintainable, and observable HTTP services with comprehensive authentication and security features.

## Features

- **RESTful API** - Full CRUD operations for item management
- **WebSocket Support** - Real-time communication with automatic random value streaming
- **Multiple Authentication Modes** - No auth, mTLS, OIDC, Basic Auth, API Key, and Multi-mode support
- **TLS/mTLS Support** - Secure communication with client certificate authentication
- **Vault Integration** - Dynamic PKI certificate management
- **Prometheus Metrics** - Built-in observability with HTTP request metrics
- **Structured Logging** - JSON-formatted logs using Zap logger
- **Graceful Shutdown** - Proper handling of shutdown signals with connection draining
- **CORS Support** - Configurable Cross-Origin Resource Sharing
- **Request Tracing** - Automatic request ID generation and propagation
- **Docker Ready** - Multi-stage Dockerfile with security best practices
- **In-Memory Storage** - Thread-safe storage implementation (easily replaceable)
- **Comprehensive Testing** - Unit, functional, integration, E2E, and performance tests

## Project Structure

```
.
├── cmd/
│   └── server/          # Application entry point
├── internal/
│   ├── auth/            # Authentication interfaces and implementations
│   ├── config/          # Configuration management
│   ├── handler/         # HTTP and WebSocket handlers
│   ├── middleware/      # HTTP middleware (auth, logging, metrics, CORS, etc.)
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

- Go 1.25.7 or later
- Docker (optional, for containerized deployment)
- Docker Compose (optional, for test environment)

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

## Authentication

The server supports multiple authentication modes that can be configured via the `APP_AUTH_MODE` environment variable:

### Authentication Modes

#### No Authentication (default)
```bash
APP_AUTH_MODE=none ./server
```
All endpoints are publicly accessible without authentication.

#### API Key Authentication
```bash
APP_AUTH_MODE=apikey APP_API_KEYS="my-secret-key:my-app,another-key:another-app" ./server
curl -H "X-API-Key: my-secret-key" http://localhost:8080/api/v1/items
```
Requires `X-API-Key` header with a valid API key. Format: `key:name,key:name,...`

#### Basic Authentication
```bash
# Generate bcrypt hash: htpasswd -nbBC 10 "" password | tr -d ':\n'
APP_AUTH_MODE=basic APP_BASIC_AUTH_USERS='admin:$2y$10$...,user:$2y$10$...' ./server
curl -u admin:password http://localhost:8080/api/v1/items
```
Requires HTTP Basic Authentication. Format: `user:bcrypt_hash,user:bcrypt_hash,...`

#### mTLS Authentication
```bash
APP_AUTH_MODE=mtls APP_TLS_ENABLED=true \
  APP_TLS_CERT_PATH=./cert.pem APP_TLS_KEY_PATH=./key.pem \
  APP_TLS_CA_PATH=./ca.pem APP_TLS_CLIENT_AUTH=require ./server
```
Requires valid client certificates signed by the configured CA.

#### OIDC Authentication
```bash
APP_AUTH_MODE=oidc \
  APP_OIDC_ISSUER_URL=http://localhost:8090/realms/restapi-test \
  APP_OIDC_CLIENT_ID=restapi-server ./server
curl -H "Authorization: Bearer <jwt_token>" http://localhost:8080/api/v1/items
```
Requires valid JWT tokens from the configured OIDC provider.

#### Multi-Mode Authentication
```bash
APP_AUTH_MODE=multi \
  APP_API_KEYS="key1:app1" \
  APP_BASIC_AUTH_USERS="admin:$2y$10$..." \
  APP_TLS_ENABLED=true ./server
```
Accepts any of the configured authentication methods.

## Configuration

Configuration is managed through environment variables. Environment variables take priority over default values.

### Configuration Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_SERVER_PORT` | `8080` | Server port |
| `APP_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `APP_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `APP_METRICS_ENABLED` | `true` | Enable Prometheus metrics |
| `APP_OTLP_ENDPOINT` | `` | OTLP endpoint for telemetry |
| `APP_AUTH_MODE` | `none` | Auth mode (none, mtls, oidc, basic, apikey, multi) |
| `APP_TLS_ENABLED` | `false` | Enable TLS |
| `APP_TLS_CERT_PATH` | `` | TLS certificate path |
| `APP_TLS_KEY_PATH` | `` | TLS private key path |
| `APP_TLS_CA_PATH` | `` | TLS CA certificate path |
| `APP_TLS_CLIENT_AUTH` | `none` | TLS client auth (none, request, require) |
| `APP_OIDC_ISSUER_URL` | `` | OIDC issuer URL |
| `APP_OIDC_CLIENT_ID` | `` | OIDC client ID |
| `APP_OIDC_AUDIENCE` | `` | OIDC audience |
| `APP_BASIC_AUTH_USERS` | `` | Basic auth users (user:bcrypt_hash,...) |
| `APP_API_KEYS` | `` | API keys (key:name,...) |
| `APP_VAULT_ENABLED` | `false` | Enable Vault integration |
| `APP_VAULT_ADDR` | `` | Vault address |
| `APP_VAULT_TOKEN` | `` | Vault token |
| `APP_VAULT_PKI_PATH` | `` | Vault PKI path |
| `APP_VAULT_PKI_ROLE` | `` | Vault PKI role |

### Example

```bash
export APP_SERVER_PORT=3000
export APP_LOG_LEVEL=debug
export APP_METRICS_ENABLED=true
./bin/server
```

## API Endpoints

The API provides both public and protected endpoints:

- **Public endpoints** (no authentication required): `/health`, `/ready`, `/metrics`
- **Protected endpoints** (authentication required): `/api/v1/items/*`, `/ws`

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
| `Authorization` | Bearer token for OIDC authentication |
| `X-API-Key` | API key for API key authentication |

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

## Test Environment

The project includes a comprehensive test environment using Docker Compose with Vault (PKI) and Keycloak (OIDC) for testing authentication features.

### Starting the Test Environment

```bash
# Start all services (Vault, Keycloak, PostgreSQL, REST API server)
make test-env-up

# Stop the test environment
make test-env-down

# View logs
make test-env-logs
```

The test environment provides:
- **Vault** (http://localhost:8200) - PKI certificate management
- **Keycloak** (http://localhost:8090) - OIDC identity provider
- **PostgreSQL** - Keycloak database
- **REST API Server** (https://localhost:8080) - Application under test

### Running Tests

```bash
make test              # Unit tests
make test-functional   # Functional tests
make test-integration  # Integration tests (requires docker-compose)
make test-e2e          # E2E tests (requires docker-compose)
make test-performance  # Performance benchmarks
make test-all          # All tests (unit + functional)
```

## Development

### Available Make Commands

```bash
make help              # Show all available commands
make build             # Build the binary
make run               # Build and run the server
make test              # Run unit tests
make test-coverage     # Run tests with coverage report
make test-functional   # Run functional tests
make test-integration  # Run integration tests (requires docker-compose)
make test-e2e          # Run end-to-end tests (requires docker-compose)
make test-performance  # Run performance/benchmark tests
make test-all          # Run all tests (unit + functional)
make test-env-up       # Start test environment
make test-env-down     # Stop test environment
make lint              # Run linter
make lint-fix          # Run linter with auto-fix
make fmt               # Format code
make vuln              # Run vulnerability check
make docker-build      # Build Docker image
make docker-run        # Run Docker container
make clean             # Clean build artifacts
make install-tools     # Install development tools
```

### Adding a New Authenticator

To add a new authentication method:

1. Create a new authenticator in `internal/auth/` implementing the `Authenticator` interface
2. Add the new mode to the `AuthMode` validation in `internal/config/config.go`
3. Update the auth middleware in `internal/middleware/auth.go` to handle the new mode
4. Add configuration variables and validation as needed
5. Write comprehensive tests for the new authenticator

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

### Authentication Package

The `internal/auth/` package provides a flexible authentication system:

- **`auth.go`** - Core interfaces and factory functions
- **`apikey.go`** - API key authentication
- **`basic.go`** - HTTP Basic authentication with bcrypt
- **`mtls.go`** - Mutual TLS authentication
- **`oidc.go`** - OpenID Connect JWT validation
- **`multi.go`** - Multi-mode authentication combiner

### Middleware Chain

Requests flow through the following middleware (in order):

1. **Recovery** - Catches panics and returns 500 error
2. **RequestID** - Generates/propagates request IDs
3. **Metrics** - Records Prometheus metrics (if enabled)
4. **Authentication** - Validates credentials based on auth mode
5. **Logging** - Logs request details
6. **CORS** - Handles cross-origin requests

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
