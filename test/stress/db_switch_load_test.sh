#!/bin/bash
# Load testing script for pREST multi-database switching
# Uses parallel curl requests to simulate high load

set -e

HOST="${PREST_HOST:-http://localhost:3000}"
DB1="${DB1:-yarsew}"
DB2="${DB2:-ogmami}"
DURATION="${DURATION:-30}"
RATE="${RATE:-100}"  # requests per second
WORKERS="${WORKERS:-20}"

echo "=== pREST Load Test - Multi-Database Switching ==="
echo "Host: $HOST"
echo "Duration: ${DURATION}s"
echo "Target rate: ${RATE} req/s"
echo "Workers: ${WORKERS}"
echo ""

# Create temp directory
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

# Function to perform requests for a time period
load_worker() {
    local worker_id=$1
    local duration=$2
    local success_file="$TMPDIR/load_success_${worker_id}"
    local fail_file="$TMPDIR/load_fail_${worker_id}"
    local latency_file="$TMPDIR/load_latency_${worker_id}"
    
    > "$success_file"
    > "$fail_file"
    > "$latency_file"
    
    local end_time=$(($(date +%s) + duration))
    local request_count=0
    
    while [ $(date +%s) -lt $end_time ]; do
        # Alternate between databases
        local db=$([[ $((request_count % 2)) -eq 0 ]] && echo "$DB1" || echo "$DB2")
        local op=$((request_count % 4))
        
        case $op in
            0) url="$HOST/$db/public" ;;
            1) url="$HOST/schemas" ;;
            2) url="$HOST/databases" ;;
            3) url="$HOST/$db/public?_limit=10" ;;
        esac
        
        local start=$(date +%s%N)
        local http_code=$(curl -s -w "%{http_code}" -o /dev/null "$url" 2>/dev/null || echo "000")
        local end=$(date +%s%N)
        
        local latency=$(( (end - start) / 1000000 ))  # ms
        
        if [ "$http_code" = "200" ]; then
            echo "$latency" >> "$success_file"
        else
            echo "$latency" >> "$fail_file"
        fi
        
        request_count=$((request_count + 1))
    done
}

echo "Starting load test..."
start_time=$(date +%s.%N)

# Launch workers
pids=""
for i in $(seq 1 $WORKERS); do
    load_worker $i $DURATION &
    pids="$pids $!"
done

# Show progress
for i in $(seq 1 $DURATION); do
    sleep 1
    echo -ne "  Progress: ${i}/${DURATION}s\r"
done

wait $pids
echo ""

end_time=$(date +%s.%N)
total_duration=$(echo "$end_time - $start_time" | bc)

# Aggregate results
total_success=0
total_failed=0
all_latencies=""

for i in $(seq 1 $WORKERS); do
    s=$(wc -l < "$TMPDIR/load_success_$i" 2>/dev/null || echo 0)
    f=$(wc -l < "$TMPDIR/load_fail_$i" 2>/dev/null || echo 0)
    total_success=$((total_success + s))
    total_failed=$((total_failed + f))
    
    if [ -f "$TMPDIR/load_success_$i" ]; then
        all_latencies="$all_latencies $(cat "$TMPDIR/load_success_$i")"
    fi
done

total_requests=$((total_success + total_failed))
rps=$(echo "scale=2; $total_requests / $total_duration" | bc)

# Calculate latency percentiles
if [ -n "$all_latencies" ]; then
    echo "$all_latencies" | tr ' ' '\n' | grep -v '^$' | sort -n > "$TMPDIR/all_latencies.txt"
    
    p50=$(awk 'NR==int('"$(wc -l < $TMPDIR/all_latencies.txt)"'*0.50)' "$TMPDIR/all_latencies.txt")
    p95=$(awk 'NR==int('"$(wc -l < $TMPDIR/all_latencies.txt)"'*0.95)' "$TMPDIR/all_latencies.txt")
    p99=$(awk 'NR==int('"$(wc -l < $TMPDIR/all_latencies.txt)"'*0.99)' "$TMPDIR/all_latencies.txt")
    avg=$(awk '{sum+=$1; count++} END {if(count>0) printf "%.2f", sum/count; else print "0"}' "$TMPDIR/all_latencies.txt")
    max=$(tail -1 "$TMPDIR/all_latencies.txt")
    min=$(head -1 "$TMPDIR/all_latencies.txt")
    
    echo "=== Load Test Results ==="
    echo "Duration: ${total_duration}s"
    echo "Total requests: $total_requests"
    echo "Successful: $total_success"
    echo "Failed: $total_failed"
    echo "Requests/sec: $rps"
    echo ""
    echo "Latency (ms):"
    echo "  Min: $min"
    echo "  Avg: $avg"
    echo "  P50: $p50"
    echo "  P95: $p95"
    echo "  P99: $p99"
    echo "  Max: $max"
    echo ""
    
    if [ $total_failed -eq 0 ]; then
        echo "✓ All requests succeeded!"
    else
        failure_rate=$(echo "scale=2; ($total_failed * 100) / $total_requests" | bc)
        echo "⚠ Failure rate: ${failure_rate}%"
    fi
fi
