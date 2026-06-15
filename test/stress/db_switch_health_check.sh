#!/bin/bash
# Health check and warmup script for pREST stress testing
# Verifies both databases are accessible before running stress tests

set -e

HOST="${PREST_HOST:-http://localhost:3000}"
DB1="${DB1:-yarsew}"
DB2="${DB2:-ogmami}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== pREST Stress Test - Health Check ==="
echo ""

# Check 1: Health endpoint
echo "1. Checking health endpoint..."
health_code=$(curl -s -w "%{http_code}" -o /dev/null "$HOST/_health" 2>/dev/null || echo "000")
if [ "$health_code" = "200" ]; then
    health_body=$(curl -s "$HOST/_health" 2>/dev/null)
    echo -e "   ${GREEN}✓${NC} Health check passed ($health_code): $health_body"
else
    echo -e "   ${RED}✗${NC} Health check failed. HTTP $health_code"
    exit 1
fi
echo ""

# Check 2: Database listing (if exposed)
echo "2. Checking databases listing..."
dbs_code=$(curl -s -w "%{http_code}" -o "$TMPDIR/dbs.json" "$HOST/databases" 2>/dev/null || echo "000")
if [ "$dbs_code" = "200" ]; then
    echo -e "   ${GREEN}✓${NC} Databases accessible"
else
    echo -e "   ${YELLOW}⚠${NC} Database listing may be restricted ($dbs_code)"
fi
echo ""

# Check 3: DB1 access
echo "3. Testing database 1 ($DB1)..."
db1_code=$(curl -s -w "%{http_code}" -o /dev/null "$HOST/$DB1/public" 2>/dev/null || echo "000")
if [ "$db1_code" = "200" ]; then
    echo -e "   ${GREEN}✓${NC} $DB1/public => $db1_code OK"
else
    echo -e "   ${RED}✗${NC} $DB1/public => $db1_code FAILED"
    exit 1
fi
echo ""

# Check 4: DB2 access
echo "4. Testing database 2 ($DB2)..."
db2_code=$(curl -s -w "%{http_code}" -o /dev/null "$HOST/$DB2/public" 2>/dev/null || echo "000")
if [ "$db2_code" = "200" ]; then
    echo -e "   ${GREEN}✓${NC} $DB2/public => $db2_code OK"
else
    echo -e "   ${RED}✗${NC} $DB2/public => $db2_code FAILED"
    exit 1
fi
echo ""

# Check 5: Schema access per database (use /{database}/{schema} to verify pool resolution)
echo "5. Testing schema access per database..."
db1_schemas=$(curl -s -w "%{http_code}" -o /dev/null "$HOST/$DB1/public" 2>/dev/null || echo "000")
db2_schemas=$(curl -s -w "%{http_code}" -o /dev/null "$HOST/$DB2/public" 2>/dev/null || echo "000")
if [ "$db1_schemas" = "200" ] && [ "$db2_schemas" = "200" ]; then
    echo -e "   ${GREEN}✓${NC} Schema access OK for both databases"
else
    echo -e "   ${YELLOW}⚠${NC} Schema access: DB1=$db1_schemas DB2=$db2_schemas"
fi
echo ""

# Warmup: Send a few requests to establish connections
echo "6. Warming up connection pool..."
for i in $(seq 1 5); do
    curl -s -o /dev/null "$HOST/$DB1/public" 2>/dev/null || true
    curl -s -o /dev/null "$HOST/$DB2/public" 2>/dev/null || true
    echo -ne "   Warmup round $i/5\r"
done
echo -e "   ${GREEN}✓${NC} Connection pool warmed up"
echo ""

echo -e "${GREEN}All health checks passed. Ready for stress tests.${NC}"
