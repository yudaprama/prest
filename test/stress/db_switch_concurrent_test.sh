#!/bin/bash
# Concurrent stress test for multi-database connection switching
# Tests parallel requests to multiple databases

set -e

HOST="${PREST_HOST:-http://localhost:3000}"
DB1="${DB1:-yarsew}"
DB2="${DB2:-ogmami}"
CONCURRENT="${CONCURRENT:-10}"
ITERATIONS="${ITERATIONS:-50}"

echo "=== pREST Concurrent Multi-Database Stress Test ==="
echo "Host: $HOST"
echo "Database 1: $DB1"
echo "Database 2: $DB2"
echo "Concurrent workers: $CONCURRENT"
echo "Iterations per worker: $ITERATIONS"
echo ""

# Create temp directory for results
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

# Worker function
worker() {
    local worker_id=$1
    local db=$2
    local success_file="$TMPDIR/success_${worker_id}"
    local fail_file="$TMPDIR/fail_${worker_id}"
    
    echo 0 > "$success_file"
    echo 0 > "$fail_file"
    
    for i in $(seq 1 $ITERATIONS); do
        response=$(curl -s -w "\n%{http_code}" "$HOST/$db/public/tables" 2>/dev/null || echo "000")
        http_code=$(echo "$response" | tail -n1)
        
        if [ "$http_code" = "200" ]; then
            echo $(($(cat "$success_file") + 1)) > "$success_file"
        else
            echo $(($(cat "$fail_file") + 1)) > "$fail_file"
        fi
    done
}

# Test 1: Concurrent requests to same database
echo "Test 1: Concurrent requests to single database ($DB1)..."
start_time=$(date +%s.%N)

pids=""
for i in $(seq 1 $CONCURRENT); do
    worker $i "$DB1" &
    pids="$pids $!"
done

wait $pids
end_time=$(date +%s.%N)
duration=$(echo "$end_time - $start_time" | bc)

total_success=0
total_failed=0
for i in $(seq 1 $CONCURRENT); do
    s=$(cat "$TMPDIR/success_$i" 2>/dev/null || echo 0)
    f=$(cat "$TMPDIR/fail_$i" 2>/dev/null || echo 0)
    total_success=$((total_success + s))
    total_failed=$((total_failed + f))
done

echo "  Completed: $((total_success + total_failed)) requests"
echo "  Successful: $total_success"
echo "  Failed: $total_failed"
echo "  Duration: ${duration}s"
echo "  Requests/sec: $(awk "BEGIN {printf \"%.2f\", ($total_success + $total_failed) / $duration}")"
echo ""

# Test 2: Concurrent requests to different databases
echo "Test 2: Concurrent requests to different databases (alternating)..."
start_time=$(date +%s.%N)

pids=""
for i in $(seq 1 $CONCURRENT); do
    db=$([[ $((i % 2)) -eq 0 ]] && echo "$DB1" || echo "$DB2")
    worker $i "$db" &
    pids="$pids $!"
done

wait $pids
end_time=$(date +%s.%N)
duration=$(echo "$end_time - $start_time" | bc)

total_success=0
total_failed=0
for i in $(seq 1 $CONCURRENT); do
    s=$(cat "$TMPDIR/success_$i" 2>/dev/null || echo 0)
    f=$(cat "$TMPDIR/fail_$i" 2>/dev/null || echo 0)
    total_success=$((total_success + s))
    total_failed=$((total_failed + f))
done

echo "  Completed: $((total_success + total_failed)) requests"
echo "  Successful: $total_success"
echo "  Failed: $total_failed"
echo "  Duration: ${duration}s"
echo "  Requests/sec: $(awk "BEGIN {printf \"%.2f\", ($total_success + $total_failed) / $duration}")"
echo ""

# Test 3: Concurrent mixed operations
echo "Test 3: Concurrent mixed operations (tables, schemas, databases)..."
start_time=$(date +%s.%N)

for i in $(seq 1 $CONCURRENT); do
    (
        success=0
        failed=0
        for j in $(seq 1 $ITERATIONS); do
            op=$((j % 3))
            case $op in
                0) url="$HOST/$DB1/public/tables" ;;
                1) url="$HOST/schemas" ;;
                2) url="$HOST/databases" ;;
            esac
            
            http_code=$(curl -s -w "%{http_code}" -o /dev/null "$url" 2>/dev/null || echo "000")
            
            if [ "$http_code" = "200" ]; then
                ((success++))
            else
                ((failed++))
            fi
        done
        echo "$success $failed" > "$TMPDIR/mixed_$i"
    ) &
    pids="$pids $!"
done

wait $pids
end_time=$(date +%s.%N)
duration=$(echo "$end_time - $start_time" | bc)

total_success=0
total_failed=0
for i in $(seq 1 $CONCURRENT); do
    if [ -f "$TMPDIR/mixed_$i" ]; then
        read s f < "$TMPDIR/mixed_$i"
        total_success=$((total_success + s))
        total_failed=$((total_failed + f))
    fi
done

echo "  Completed: $((total_success + total_failed)) requests"
echo "  Successful: $total_success"
echo "  Failed: $total_failed"
echo "  Duration: ${duration}s"
echo "  Requests/sec: $(awk "BEGIN {printf \"%.2f\", ($total_success + $total_failed) / $duration}")"
echo ""

echo "=== Concurrent Stress Test Complete ==="
