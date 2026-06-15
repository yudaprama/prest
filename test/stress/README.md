# pREST Multi-Database Switching Stress Tests

Stress test scripts for the multi-database connection switching feature.

## Prerequisites

**pREST** with multi-database config (`prest.toml`):

```toml
[pg]
url = "postgresql://user:pass@host:5432/maindb?sslmode=require"
single = false

[[pg.urls]]
name = "yarsew"
url = "postgresql://user:pass@host1:5432/db1?sslmode=require"

[[pg.urls]]
name = "ogmami"
url = "postgresql://user:pass@host2:5432/db2?sslmode=require"
```

**Environment variables** (optional):

```bash
export PREST_HOST="http://localhost:3000"
export DB1="yarsew"
export DB2="ogmami"
```

## Verified URL Patterns

The stress tests use these pREST routes (validated against the router):

| URL | Purpose | Status |
|-----|---------|--------|
| `GET /{database}/{schema}` | List tables in schema | ✅ 200 |
| `GET /databases` | List databases | ✅ 200 |
| `GET /_health` | Health check | ✅ 200 |

> **Note**: `/schemas` does not support a `?dbname=` filter parameter; it uses the
> default connection. For per-database schema access, use `/{database}/{schema}`.
> The route `/{database}/{schema}/{table}` treats the last segment as a **table
> name**, not the tables listing endpoint.

## Test Scripts

### 1. Health Check (`db_switch_health_check.sh`)

Pre-flight check that both databases are reachable and the connection pool is
warm.

```bash
./test/stress/db_switch_health_check.sh
```

Validates: `/_health`, `/databases`, `/{DB1}/public`, `/{DB2}/public`.

### 2. Basic Stress Test (`db_switch_basic_test.sh`)

Sequential and alternating requests between two databases.

```bash
./test/stress/db_switch_basic_test.sh             # 100 iterations
ITERATIONS=500 ./test/stress/db_switch_basic_test.sh
```

Runs 3 sub-tests:
- Sequential DB1 → DB2 round trips
- Rapid alternation between DBs
- Repeated `/{database}/public` access on both DBs

### 3. Concurrent Stress Test (`db_switch_concurrent_test.sh`)

Parallel workers hitting different databases simultaneously to test pool
thread-safety and connection isolation.

```bash
./test/stress/db_switch_concurrent_test.sh
CONCURRENT=20 ITERATIONS=100 ./test/stress/db_switch_concurrent_test.sh
```

Runs 3 sub-tests:
- N workers → single DB
- N workers → alternating DBs
- N workers → mixed endpoints (tables/databases)

### 4. Load Test (`db_switch_load_test.sh`)

Sustained load with latency percentile measurement (P50, P95, P99).

```bash
./test/stress/db_switch_load_test.sh
DURATION=60 WORKERS=50 ./test/stress/db_switch_load_test.sh
```

### 5. Go Integration Test (`db_switch_go_test.go`)

Programmatic Go tests using `httptest`:

```bash
go test -v ./test/stress/...                         # full suite
go test -v -short ./test/stress/...                  # skip long tests
go test -v -run TestDatabaseIsolation ./test/stress/ # one test
```

Tests:
- `TestMultiDatabaseSwitching` — sequential + concurrent
- `TestDatabaseSwitchingUnderLoad` — time-bounded load
- `TestDatabaseIsolation` — cross-DB contamination check

### 6. Master Runner (`run_all_tests.sh`)

Runs every script in order with pass/fail summary. Aborts on health-check failure.

```bash
./test/stress/run_all_tests.sh
```

### 7. Make Targets (`Makefile`)

```bash
make -C test/stress stress-test-health
make -C test/stress stress-test-basic
make -C test/stress stress-test-concurrent
make -C test/stress stress-test-load
make -C test/stress stress-test-go
make -C test/stress stress-test-quick   # 5 workers, 10–20 iterations
make -C test/stress stress-test        # full suite (health + all)
```
