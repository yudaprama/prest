# pREST Multi-Database Switching Stress Tests

This directory contains stress test scripts for testing the multi-database connection switching feature in pREST.

## Prerequisites

1. **pREST server running** with multi-database configuration:
   ```toml
   [pg]
   url = "postgresql://..."
   single = false
   
   [[pg.urls]]
   name = "yarsew"
   url = "postgresql://..."
   
   [[pg.urls]]
   name = "ogmami"
   url = "postgresql://..."
   ```

2. **Environment variables** (optional):
   ```bash
   export PREST_HOST="http://localhost:3000"
   export DB1="yarsew"
   export DB2="ogmami"
   ```

## Test Scripts

### 1. Health Check (`db_switch_health_check.sh`)
Verifies both databases are accessible before running stress tests.

```bash
./test/stress/db_switch_health_check.sh
```

### 2. Basic Stress Test (`db_switch_basic_test.sh`)
Sequential and alternating requests between databases.

```bash
# Default: 100 iterations
./test/stress/db_switch_basic_test.sh

# Custom iterations
ITERATIONS=500 ./test/stress/db_switch_basic_test.sh
```

### 3. Concurrent Stress Test (`db_switch_concurrent_test.sh`)
Parallel requests from multiple workers to test connection pool behavior.

```bash
# Default: 10 workers, 50 iterations each
./test/stress/db_switch_concurrent_test.sh

# Custom configuration
CONCURRENT=20 ITERATIONS=100 ./test/stress/db_switch_concurrent_test.sh
```

### 4. Load Test (`db_switch_load_test.sh`)
Sustained load testing with latency measurements.

```bash
# Default: 30s duration, 100 req/s target, 20 workers
./test/stress/db_switch_load_test.sh

# Custom configuration
DURATION=60 RATE=200 WORKERS=50 ./test/stress/db_switch_load_test.sh
```

### 5. Go Integration Test (`db_switch_go_test.go`)
Programmatic stress test using Go's testing framework.

```bash
# Run all stress tests
go test -v ./test/stress/...

# Run specific test
go test -v -run TestMultiDatabaseSwitching ./test/stress/

# Skip stress tests
go test -v -short ./test/stress/...
```

## Test Scenarios

### Sequential Switching
- Alternates between two databases in sequence
- Tests basic connection switching without concurrency
- Validates each database responds correctly

### Concurrent Switching
- Multiple workers hitting different databases simultaneously
- Tests connection pool thread-safety
- Validates no connection leaks or race conditions

### Load Testing
- Sustained high request rate
- Measures latency percentiles (P50, P95, P99)
- Identifies performance bottlenecks

### Database Isolation
- Verifies queries to one database don't affect another
- Tests connection pool isolation
- Validates no cross-database contamination

## Metrics Reported

- **Total requests**: Sum of all HTTP requests made
- **Success rate**: Percentage of 200 OK responses
- **Requests/sec**: Throughput measurement
- **Latency (ms)**: Min, Avg, P50, P95, P99, Max
- **Failure rate**: Percentage of failed requests

## Expected Results

With a properly configured multi-database setup:

- **Success rate**: Should be 100% (all 200 OK)
- **Latency P95**: Typically < 100ms for local databases
- **Throughput**: Varies by hardware, 100-1000+ req/sec

## Troubleshooting

### Connection Pool Exhaustion
If you see connection errors:
```bash
# Check pool settings in config
[pg]
max_idle_conn = 10
max_open_conn = 100
```

### High Latency
- Check network latency to databases
- Verify database server load
- Review connection pool sizing

### Authentication Errors
- Verify connection strings are correct
- Check database credentials
- Ensure SSL mode is appropriate

## Integration with CI/CD

```bash
# Example: Run health check before tests
./test/stress/db_switch_health_check.sh && \
./test/stress/db_switch_basic_test.sh && \
go test -v ./test/stress/...
```

## Performance Tuning

For better stress test performance:

1. **Increase worker count**: `CONCURRENT=50`
2. **Extend duration**: `DURATION=120`
3. **Monitor pREST metrics**: Check server logs for errors
4. **Database tuning**: Optimize PostgreSQL settings

## Architecture Notes

The multi-database feature uses:
- **Connection pool**: `map[string]*sqlx.DB` with mutex protection
- **Name resolution**: Logical name → actual database name mapping
- **Context propagation**: Database name passed via context
- **Thread-safe access**: Concurrent request handling

See `adapters/postgres/internal/connection/conn.go` for implementation details.
