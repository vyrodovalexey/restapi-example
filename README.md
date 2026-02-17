# restapi-example

A production-ready REST API and WebSocket server built with Go. This project demonstrates best practices for building scalable, maintainable, and observable HTTP services with comprehensive authentication, security features, and enterprise-grade deployment capabilities.

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
- **Kubernetes Ready** - Comprehensive Helm chart with production features
- **In-Memory Storage** - Thread-safe storage implementation (easily replaceable)
- **Comprehensive Testing** - Unit, functional, integration, E2E, and performance tests
- **CI/CD Pipeline** - GitHub Actions with security scanning and automated releases
- **Dedicated Probe Port** - Separate HTTP server for health checks, readiness, and metrics

## Table of Contents

- [Project Structure](#project-structure)
- [Quick Start](#quick-start)
- [Authentication](#authentication)
- [Configuration](#configuration)
- [API Endpoints](#api-endpoints)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Testing](#testing)
- [Performance](#performance)
- [CI/CD Pipeline](#cicd-pipeline)
- [Development](#development)
- [Architecture](#architecture)
- [Docker](#docker)

## Project Structure

```
.
├── .github/
│   └── workflows/
│       └── ci.yml           # GitHub Actions CI/CD pipeline
├── cmd/
│   └── server/              # Application entry point
├── helm/
│   └── restapi-example/     # Kubernetes Helm chart
│       ├── templates/       # K8s resource templates
│       ├── values.yaml      # Default configuration values
│       └── README.md        # Helm chart documentation
├── internal/
│   ├── auth/                # Authentication interfaces and implementations
│   ├── config/              # Configuration management
│   ├── handler/             # HTTP and WebSocket handlers
│   ├── middleware/          # HTTP middleware (auth, logging, metrics, CORS, etc.)
│   ├── model/               # Data models and validation
│   ├── server/              # HTTP server setup
│   └── store/               # Data storage interface and implementations
├── test/
│   ├── cases/               # Test case definitions (JSON)
│   ├── docker-compose/      # Docker Compose test environment
│   ├── e2e/                 # End-to-end tests
│   ├── functional/          # Functional tests (REST API + WebSocket + Auth)
│   ├── integration/         # Integration tests (requires external services)
│   └── performance/         # Performance benchmarks
├── Dockerfile               # Multi-stage Docker build
├── Makefile                 # Build automation
└── README.md
```

## Quick Start

### Prerequisites

- Go 1.25.7 or later
- Docker (optional, for containerized deployment)
- Docker Compose (optional, for test environment)
- Kubernetes + Helm (optional, for K8s deployment)

### Running Locally

```bash
# Build and run
make run

# Or run directly with Go
go run ./cmd/server
```

The server will start on `http://localhost:8080` by default. A dedicated probe server will also start on port `9090` for health checks, readiness probes, and metrics.

### Running with Docker

```bash
# Build Docker image
make docker-build

# Run container
make docker-run
```

### Running on Kubernetes

```bash
# Install with Helm
helm install my-api ./helm/restapi-example

# Or with custom values
helm install my-api ./helm/restapi-example -f my-values.yaml
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
Requires valid JWT tokens from the configured OIDC provider. Features:
- JWT token verification using OIDC discovery
- Supports RS256/RS384/RS512 signing algorithms
- JWKS caching with automatic refresh
- No external dependencies (stdlib only)

#### Multi-Mode Authentication
```bash
APP_AUTH_MODE=multi \
  APP_API_KEYS="key1:app1" \
  APP_BASIC_AUTH_USERS="admin:$2y$10$..." \
  APP_OIDC_ISSUER_URL=http://localhost:8090/realms/restapi-test \
  APP_OIDC_CLIENT_ID=restapi-server \
  APP_TLS_ENABLED=true ./server
```
Accepts any of the configured authentication methods (mTLS, Basic Auth, API Key, OIDC).

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
| `APP_PROBE_PORT` | `9090` | Dedicated probe server port (0 = disabled) |

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

**Note:** Health, readiness, and metrics endpoints are also available on the dedicated probe port (9090 by default) without authentication or TLS, making them ideal for Docker health checks and Kubernetes probes.

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

## Kubernetes Deployment

The project includes a comprehensive Helm chart for Kubernetes deployment with production-ready features.

### Helm Chart Features

- **Deployment** - Configurable replicas with rolling updates
- **Service** - ClusterIP/NodePort/LoadBalancer support
- **Ingress** - HTTP/HTTPS routing with TLS termination
- **ConfigMap** - Application configuration management
- **Secret** - Secure credential storage
- **HPA** - Horizontal Pod Autoscaler for automatic scaling
- **PDB** - Pod Disruption Budget for high availability
- **ServiceMonitor** - Prometheus metrics scraping
- **All Authentication Modes** - Support for none, mTLS, OIDC, Basic, API Key, Multi
- **TLS/mTLS Configuration** - Secure communication setup
- **Vault Integration** - Dynamic certificate management
- **Probe Port Configuration** - Dedicated port for health checks and metrics

### Quick Deployment

```bash
# Basic deployment
helm install my-api ./helm/restapi-example

# Production deployment with scaling
helm install my-api ./helm/restapi-example \
  --set replicaCount=3 \
  --set autoscaling.enabled=true \
  --set autoscaling.maxReplicas=10 \
  --set podDisruptionBudget.enabled=true
```

### Configuration Examples

#### API Key Authentication
```yaml
# values.yaml
config:
  auth:
    mode: apikey
  apiKey:
    keys: "prod-key-001:frontend,prod-key-002:mobile"
```

#### TLS with mTLS Authentication
```yaml
# values.yaml
config:
  tls:
    enabled: true
    existingSecret: my-tls-secret
    clientAuth: require
  auth:
    mode: mtls
```

#### OIDC with Keycloak
```yaml
# values.yaml
config:
  auth:
    mode: oidc
  oidc:
    issuerURL: "https://keycloak.example.com/realms/production"
    clientID: "restapi-server"
    audience: "restapi-audience"
```

#### Production Setup with Monitoring
```yaml
# values.yaml
replicaCount: 3

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

podDisruptionBudget:
  enabled: true
  minAvailable: 2

serviceMonitor:
  enabled: true
  interval: 30s

resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 256Mi

config:
  probePort: 9090  # Dedicated probe server port

service:
  probePort: 9090  # Expose probe port for health checks
```

For detailed Helm chart documentation, see [helm/restapi-example/README.md](helm/restapi-example/README.md).

---

## Testing

The project includes comprehensive testing at multiple levels with dedicated test environments.

### Test Structure

- **`test/functional/`** - Functional tests (REST API + WebSocket + Auth) - 13 test cases including multi-auth and OIDC
- **`test/integration/`** - Integration tests (requires external services) - includes OIDC password grant and multi-auth methods
- **`test/e2e/`** - End-to-end tests - includes mTLS and OIDC workflows with Keycloak
- **`test/performance/`** - Performance benchmarks - includes multi-auth and API key auth benchmarks
- **`test/cases/`** - Test case definitions (JSON)
- **`test/docker-compose/`** - Docker Compose test environment

### Running Tests

```bash
# Unit tests
make test

# Functional tests (API + WebSocket + Auth)
make test-functional

# Integration tests (requires external services)
make test-integration

# End-to-end tests
make test-e2e

# Performance benchmarks
make test-performance

# All tests with coverage
make test-all-coverage
```

### Test Environment

The project includes a comprehensive test environment using Docker Compose with Vault (PKI) and Keycloak (OIDC) for testing authentication features.

#### Starting the Test Environment

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
- **Keycloak** (http://localhost:8090) - OIDC identity provider with realm `restapi-test`, client `restapi-server`, and test users (`test-user`, `admin-user`)
- **PostgreSQL** - Keycloak database
- **REST API Server** (https://localhost:8080) - Application under test

#### Test Coverage

```bash
# Run tests with coverage report
make test-coverage

# Check coverage threshold (70%)
make test-coverage-check
```

---

## Performance

### Benchmark Results

The project includes comprehensive performance benchmarks. Here are sample results from the latest run:

| Benchmark | Operations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkHealthEndpoint-10 | 62,427 | 20,964 | 9,709 | 118 |
| BenchmarkCRUDCreate-10 | 66,046 | 17,970 | 13,390 | 160 |
| BenchmarkCRUDRead-10 | 62,896 | 16,053 | 10,768 | 126 |
| BenchmarkConcurrentRequests/concurrency_1-10 | 77,272 | 16,675 | 10,164 | 122 |
| BenchmarkConcurrentRequests/concurrency_5-10 | 117,372 | 11,293 | 10,172 | 122 |
| BenchmarkConcurrentRequests/concurrency_10-10 | 119,530 | 10,878 | 10,197 | 122 |
| BenchmarkConcurrentRequests/concurrency_25-10 | 106,312 | 11,254 | 10,155 | 121 |

### Running Performance Tests

```bash
# Run all benchmarks
make test-performance

# Run specific benchmarks with memory profiling
go test -bench=BenchmarkHealthEndpoint -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./test/performance/

# Analyze profiles
go tool pprof cpu.prof
go tool pprof mem.prof
```

### Performance Characteristics

- **Health endpoint**: ~62K ops/sec with minimal memory allocation
- **CRUD operations**: ~60-66K ops/sec with reasonable memory usage
- **Concurrent performance**: Scales well up to 10 concurrent connections
- **Memory efficiency**: Low allocation rates across all operations

---

## CI/CD Pipeline

The project uses GitHub Actions for comprehensive CI/CD with security scanning and automated releases.

### Pipeline Stages

#### Parallel Stage 1: Code Quality & Testing
- **Lint** - golangci-lint with comprehensive rules
- **Vulnerability Check** - govulncheck for security scanning
- **Unit Tests** - Fast unit tests with coverage
- **Functional Tests** - API and WebSocket functionality tests

#### Parallel Stage 2: Integration & E2E
- **Integration Tests** - Tests with Vault + Keycloak + PostgreSQL services, including OIDC integration
- **E2E Tests** - End-to-end testing with real server instance

#### Sequential Stage 3: Analysis & Build
- **SonarCloud Analysis** - Code quality and security analysis
- **Build** - Binary compilation and artifact upload
- **Docker Build** - Multi-platform container images (PR validation)

#### Release Stage (Tags only)
- **Build & Release** - Multi-platform binaries with GitHub releases
- **Docker Build & Push** - Multi-platform images to GitHub Container Registry
- **SBOM Generation** - Software Bill of Materials for security
- **Trivy Security Scan** - Container vulnerability scanning

### Pipeline Features

- **Parallel Execution** - Optimized for speed with parallel jobs
- **Comprehensive Coverage** - Unit, functional, integration, and E2E tests
- **Security First** - Vulnerability scanning, SBOM generation, container scanning
- **Supply Chain Security** - All GitHub Actions pinned to SHA hashes
- **Multi-platform** - Builds for linux/amd64, linux/arm64, darwin/arm64
- **Artifact Management** - Binaries, coverage reports, security scans
- **Release Automation** - Automated GitHub releases with checksums
- **Helm Validation** - Uses Kind (Kubernetes-in-Docker) cluster for real install testing

### Triggering the Pipeline

```bash
# Trigger on pull request (runs all tests + build validation)
git push origin feature-branch

# Trigger release (runs full pipeline + docker push + security scan)
git tag v1.0.0
git push origin v1.0.0
```

### Pipeline Configuration

The pipeline is configured in [`.github/workflows/ci.yml`](.github/workflows/ci.yml) with:

- **Go Version**: 1.25.7
- **golangci-lint**: v2.8.0
- **Coverage Upload**: Codecov integration
- **Container Registry**: GitHub Container Registry (ghcr.io)
- **Security Scanning**: Trivy, govulncheck, SonarCloud

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
make test-integration  # Run integration tests (requires docker compose)
make test-e2e          # Run end-to-end tests (requires docker compose)
make test-performance  # Run performance/benchmark tests
make test-all          # Run all tests (unit + functional)
make test-env-up       # Start test environment
make test-env-down     # Stop test environment
make test-env-logs     # View test environment logs
make test-env-status   # Check test environment status
make test-env-wait     # Wait for test environment to be ready
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
- **`oidc.go`** - OpenID Connect JWT validation (stub interface)
- **`oidc_verifier.go`** - Full OIDC JWT token verification implementation
- **`multi.go`** - Multi-mode authentication combiner

### Server Architecture

The application runs two HTTP servers:

1. **Main Server** (port 8080) - Handles API requests with full middleware chain and authentication
2. **Probe Server** (port 9090) - Dedicated server for health checks, readiness probes, and metrics without authentication or TLS

### Middleware Chain

Main server requests flow through the following middleware (in order):

1. **Recovery** - Catches panics and returns 500 error
2. **RequestID** - Generates/propagates request IDs
3. **Metrics** - Records Prometheus metrics (if enabled)
4. **Authentication** - Validates credentials based on auth mode
5. **Logging** - Logs request details
6. **CORS** - Handles cross-origin requests

The probe server serves endpoints directly without middleware for optimal performance and reliability.

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
docker run -p 8080:8080 -p 9090:9090 \
  -e APP_LOG_LEVEL=info \
  -e APP_METRICS_ENABLED=true \
  restapi-example:latest
```

### Health Check

The Docker image includes a built-in health check that queries the dedicated probe port (`/health` on port 9090) every 30 seconds. The probe port always uses plain HTTP without authentication, making health checks reliable regardless of the main server's TLS or authentication configuration.

### Multi-platform Support

The CI/CD pipeline builds multi-platform images:
- `linux/amd64` - Standard x86_64 architecture
- `linux/arm64` - ARM64 architecture (Apple Silicon, AWS Graviton)

---

## License

MIT License - see LICENSE file for details.