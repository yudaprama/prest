.PHONY: stress-test stress-test-basic stress-test-concurrent stress-test-load stress-test-health stress-test-go stress-test-quick

SCRIPTS := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# Multi-database switching stress tests
stress-test-health:
	@echo "Running health check..."
	@./db_switch_health_check.sh

stress-test-basic:
	@echo "Running basic stress test..."
	@./db_switch_basic_test.sh

stress-test-concurrent:
	@echo "Running concurrent stress test..."
	@./db_switch_concurrent_test.sh

stress-test-load:
	@echo "Running load test..."
	@./db_switch_load_test.sh

stress-test-go:
	@echo "Running Go integration tests..."
	@cd .. && go test -v ./test/stress/...

stress-test: stress-test-health
	@echo "Running all stress tests..."
	@./run_all_tests.sh

stress-test-quick:
	@echo "Running quick stress test (reduced iterations)..."
	@ITERATIONS=20 CONCURRENT=5 ./db_switch_basic_test.sh
	@ITERATIONS=10 CONCURRENT=5 ./db_switch_concurrent_test.sh
