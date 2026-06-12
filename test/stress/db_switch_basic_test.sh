#!/bin/bash
# Basic stress test for multi-database connection switching
# Tests rapid switching between configured databases

set -e

HOST="${PREST_HOST:-http://localhost:3000}"
DB1="${DB1:-yarsew}"
DB2="${DB2:-ogmami}"
ITERATIONS="${ITERATIONS:-100}"

echo "=== pREST Multi-Database Switching Stress Test ==="
echo "Host: $HOST"
echo "Database 1: $DB1"
echo "Database 2: $DB2"
echo "Iterations: $ITERATIONS"
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

success=0
failed=0
start_time=$(date +%s)

# Test 1: Sequential switching
echo "Test 1: Sequential database switching..."
for i in $(seq 1 $ITERATIONS); do
    # Request to DB1
    response1=$(curl -s -w "\n%{http_code}" "$HOST/$DB1/public/tables" 2>/dev/null || echo "000")
    http_code1=$(echo "$response1" | tail -n1)
    
    # Request to DB2
    response2=$(curl -s -w "\n%{http_code}" "$HOST/$DB2/public/tables" 2>/dev/null || echo "000")
    http_code2=$(echo "$response2" | tail -n1)
    
    if [ "$http_code1" = "200" ] && [ "$http_code2" = "200" ]; then
        ((success+=2))
        echo -ne "${GREEN}.${NC}"
    else
        ((failed+=2))
        echo -ne "${RED}F${NC}"
        echo ""
        echo "Failed at iteration $i: DB1=$http_code1, DB2=$http_code2"
    fi
    
    if [ $((i % 50)) -eq 0 ]; then
        echo " [$i/$ITERATIONS]"
    fi
done

echo ""
echo ""

# Test 2: Rapid alternating requests
echo "Test 2: Rapid alternating requests..."
for i in $(seq 1 $ITERATIONS); do
    db=$([[ $((i % 2)) -eq 0 ]] && echo "$DB1" || echo "$DB2")
    response=$(curl -s -w "\n%{http_code}" "$HOST/$db/public/tables" 2>/dev/null || echo "000")
    http_code=$(echo "$response" | tail -n1)
    
    if [ "$http_code" = "200" ]; then
        ((success++))
        echo -ne "${GREEN}.${NC}"
    else
        ((failed++))
        echo -ne "${RED}F${NC}"
        echo ""
        echo "Failed at iteration $i on $db: $http_code"
    fi
    
    if [ $((i % 50)) -eq 0 ]; then
        echo " [$i/$ITERATIONS]"
    fi
done

echo ""
echo ""

# Test 3: Schema listing on both databases
echo "Test 3: Schema operations on different databases..."
for i in $(seq 1 50); do
    response1=$(curl -s -w "\n%{http_code}" "$HOST/schemas?dbname=$DB1" 2>/dev/null || echo "000")
    http_code1=$(echo "$response1" | tail -n1)
    
    response2=$(curl -s -w "\n%{http_code}" "$HOST/schemas?dbname=$DB2" 2>/dev/null || echo "000")
    http_code2=$(echo "$response2" | tail -n1)
    
    if [ "$http_code1" = "200" ] && [ "$http_code2" = "200" ]; then
        ((success+=2))
        echo -ne "${GREEN}.${NC}"
    else
        ((failed+=2))
        echo -ne "${RED}F${NC}"
    fi
done

echo ""
echo ""

end_time=$(date +%s)
duration=$((end_time - start_time))

# Results
echo "=== Test Results ==="
echo "Total requests: $((success + failed))"
echo -e "Successful: ${GREEN}$success${NC}"
echo -e "Failed: ${RED}$failed${NC}"
echo "Duration: ${duration}s"
echo "Requests/sec: $(awk "BEGIN {printf \"%.2f\", ($success + $failed) / $duration}")"

if [ $failed -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed!${NC}"
    exit 1
fi
