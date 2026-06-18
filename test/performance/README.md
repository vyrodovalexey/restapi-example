# Performance Testing — restapi-example

This directory contains two complementary performance assets:

1. **Go benchmarks** — `performance_test.go` (build tag `performance`).
   In-process `httptest`/`server.New` benchmarks that require **no external
   services**. This is the primary, reproducible performance harness.
2. **Yandex Tank load config** — `load.yaml` + `ammo.txt`.
   A reproducible external HTTP load profile targeting the running server's
   REST + GraphQL endpoints with an API-key auth header.

---

## 1. Go benchmarks

Run all benchmarks (3 iterations, 5s each, with memory stats):

```bash
go test -bench=. -benchmem -benchtime=5s -count=3 -tags=performance ./test/performance/...
```

Or via Make:

```bash
make test-performance
```

Benchmarks included:

| Benchmark | Exercises |
|-----------|-----------|
| `BenchmarkHealthEndpoint`       | `GET /health` baseline (full middleware chain) |
| `BenchmarkCRUDCreate`           | `POST /api/v1/items` (instrumented store create) |
| `BenchmarkCRUDRead`             | `GET /api/v1/items/{id}` (instrumented store get) |
| `BenchmarkGraphQLQuery`         | `POST /graphql` `item(id)` query (isolated server) |
| `BenchmarkAPIKeyAuthLocal`      | API-key auth path (`auth_attempts_total`) |
| `BenchmarkMultiAuth`            | multi-auth (apikey + basic) path |
| `BenchmarkConcurrentRequests/*` | throughput at concurrency 1/5/10/25 |

All benchmarks are deterministic and self-contained: they spin up an
in-process server via `server.New` and wrap the store with
`store.NewInstrumentedStore` so the new observability code paths
(`auth_attempts_total`, `store_operations_total`,
`store_operation_duration_seconds`) are exercised. Tracing runs as the global
no-op provider (`APP_OTLP_ENDPOINT` unset), matching the default deployment.

Optional environment overrides:

- `INTEGRATION_SERVER_URL` — benchmark an external server instead of in-process.
- `INTEGRATION_API_KEY`, `INTEGRATION_BASIC_USER`, `INTEGRATION_BASIC_PASS` —
  enable `BenchmarkAPIKeyAuth` / `BenchmarkBasicAuth` against that server.

---

## 2. Yandex Tank load test

Files:

- `load.yaml` — Tank config (phantom generator, RPS schedule, autostop).
- `ammo.txt` — request-style ammo for `/health`, `/api/v1/items` (list/create)
  and `/graphql`, each carrying `X-API-Key: test-api-key-12345`.

### Prerequisites

Start the server with API-key (or multi) auth and a matching key:

```bash
APP_AUTH_MODE=apikey \
APP_API_KEYS=test-api-key-12345:loadtest \
APP_SERVER_PORT=8080 \
go run ./cmd/server
```

> Change the key in `ammo.txt` to match `APP_API_KEYS`.

### Run with Docker (no local install)

```bash
cd test/performance
docker run --rm -v "$(pwd)":/var/loadtest yandex/yandex-tank -c load.yaml
```

On macOS/Windows the ammo and config target `host.docker.internal:8080`, which
resolves to the host. On Linux, use `--net host` and set
`phantom.address: 127.0.0.1:8080`.

### Run natively

```bash
cd test/performance
yandex-tank -c load.yaml
```

### Load profile

```
line(10, 100, 60s)    # warm-up ramp 10 -> 100 RPS
const(100, 60s)       # hold 100 RPS
line(100, 500, 120s)  # ramp 100 -> 500 RPS
const(500, 120s)      # hold 500 RPS
```

### Autostop (safety)

- `http(5xx,5%,10s)` — abort on server errors.
- `http(4xx,10%,10s)` — abort on auth/config errors (e.g. wrong API key).
- `quantile(99,500,15s)` — abort if p99 > 500ms.
- `net(1xx,1%,10s)` — abort on network errors.

After a run, Tank writes `phout.txt` and a report into its per-run working
directory; summarise these into the repository's `.yandextank/` folder
(`benchmark_results.txt`, `statistics_summary.json`, `performance_report.md`,
`benchmark_data.csv`).
