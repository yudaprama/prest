#!/bin/bash
# Master stress test runner for pREST multi-database switching
# Runs all stress tests in sequence and provides comprehensive report

set -e

HOST="${PREST_HOST:-http://localhost:3000}"
DB1="${DB1:-yarsew}"
DB2="${DB2:-ogmami}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=========================================="
echo "  pREST Multi-Database Stress Test Suite"
echo "=========================================="
echo ""
echo "Configuration:"
echo "  Host: $HOST"
echo "  Database 1: $DB1"
echo "  Database 2: $DB2"
echo ""

# Test results tracking
declare -A test_results
test_count=0
passed_count=0
failed_count=0

run_test() {
    local test_name=$1
    local test_script=$2
    
    test_count=$((test_count + 1))
    
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}Test $test_count: $test_name${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    
    if [ -f "$test_script" ] && [ -x "$test_script" ]; then
        if "$test_script"; then
            test_results[$test_name]="PASSED"
            passed_count=$((passed_count + 1))
            echo -e "${GREEN}✓ $test_name PASSED${NC}"
        else
            test_results[$test_name]="FAILED"
            failed_count=$((failed_count + 1))
            echo -e "${RED}✗ $test_name FAILED${NC}"
        fi
    else
        test_results[$test_name]="SKIPPED"
        echo -e "${YELLOW}⊘ $test_name SKIPPED (script not found or not executable)${NC}"
    fi
    
    echo ""
}

# Start timer
start_time=$(date +%s)

# Test 1: Health Check
run_test "Health Check" "$SCRIPT_DIR/db_switch_health_check.sh"

# Only continue if health check passed
if [ "${test_results["Health Check"]}" != "PASSED" ]; then
    echo -e "${RED}Health check failed. Aborting remaining tests.${NC}"
    exit 1
fi

# Test 2: Basic Stress Test
run_test "Basic Stress Test" "$SCRIPT_DIR/db_switch_basic_test.sh"

# Test 3: Concurrent Stress Test
run_test "Concurrent Stress Test" "$SCRIPT_DIR/db_switch_concurrent_test.sh"

# Test 4: Load Test
run_test "Load Test" "$SCRIPT_DIR/db_switch_load_test.sh"

# Test 5: Go Integration Test (if available)
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}Test 5: Go Integration Test${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

if command -v go &> /dev/null; then
    if go test -v -short "$SCRIPT_DIR" 2>&1; then
        test_results["Go Integration Test"]="PASSED"
        passed_count=$((passed_count + 1))
        echo -e "${GREEN}✓ Go Integration Test PASSED${NC}"
    else
        test_results["Go Integration Test"]="FAILED"
        failed_count=$((failed_count + 1))
        echo -e "${RED}✗ Go Integration Test FAILED${NC}"
    fi
else
    test_results["Go Integration Test"]="SKIPPED"
    echo -e "${YELLOW}⊘ Go Integration Test SKIPPED (Go not installed)${NC}"
fi
echo ""

# End timer
end_time=$(date +%s)
total_duration=$((end_time - start_time))

# Print summary
echo ""
echo "=========================================="
echo "           TEST SUMMARY"
echo "=========================================="
echo ""
echo "Total Duration: ${total_duration}s"
echo ""

for test_name in "${!test_results[@]}"; do
    status="${test_results[$test_name]}"
    case $status in
        PASSED)
            echo -e "  ${GREEN}✓${NC} $test_name"
            ;;
        FAILED)
            echo -e "  ${RED}✗${NC} $test_name"
            ;;
        SKIPPED)
            echo -e "  ${YELLOW}⊘${NC} $test_name"
            ;;
    esac
done

echo ""
echo "Results: $passed_count passed, $failed_count failed"
echo ""

if [ $failed_count -eq 0 ]; then
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}   ALL STRESS TESTS PASSED! 🎉${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 0
else
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}   SOME TESTS FAILED${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 1
fi
