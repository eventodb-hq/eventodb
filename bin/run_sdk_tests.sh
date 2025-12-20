#!/bin/bash
#
# Run SDK tests against MessageDB server
#
# Usage: ./bin/run_sdk_tests.sh [elixir|js|node|all]
#
# This script:
# 1. Builds the server (if needed)
# 2. Starts a test server with SQLite backend
# 3. Runs SDK-specific tests
# 4. Cleans up
#

set -e

SDK="${1:-all}"
PORT=6789
SERVER_BIN="./dist/messagedb"
MESSAGEDB_URL="http://localhost:$PORT"
# Known admin token for default namespace (used for creating test namespaces)
ADMIN_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

usage() {
    echo "Usage: $0 [elixir|js|node|all]"
    echo ""
    echo "Options:"
    echo "  elixir   - Run Elixir SDK tests only"
    echo "  js       - Run Node.js SDK tests only (alias for 'node')"
    echo "  node     - Run Node.js SDK tests only"
    echo "  all      - Run all SDK tests (default)"
    exit 1
}

if [[ ! "$SDK" =~ ^(elixir|js|node|all)$ ]]; then
    echo -e "${RED}Invalid SDK: $SDK${NC}"
    usage
fi

echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   MessageDB SDK Test Runner            ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
echo ""

# Kill any existing server on the port
if lsof -ti:$PORT > /dev/null 2>&1; then
    echo -e "${YELLOW}→ Killing existing process on port $PORT...${NC}"
    kill $(lsof -ti:$PORT) 2>/dev/null || true
    sleep 1
fi

# Always build server to ensure we have the latest version
echo -e "${YELLOW}→ Building MessageDB server...${NC}"
mkdir -p dist
cd golang && CGO_ENABLED=0 go build -o ../dist/messagedb ./cmd/messagedb && cd ..
echo -e "${GREEN}✓ Server built to dist/messagedb${NC}"

# Start test server (SQLite with proper auth)
echo -e "${YELLOW}→ Starting test server on port $PORT...${NC}"
echo -e "${YELLOW}→ Admin token: $ADMIN_TOKEN${NC}"
# Use SQLite with a temp data directory for testing
TEST_DATA_DIR="/tmp/messagedb-sdk-test-$$"
mkdir -p "$TEST_DATA_DIR"
$SERVER_BIN -db-url "sqlite://:memory:" -data-dir "$TEST_DATA_DIR" -port $PORT -token "$ADMIN_TOKEN" > /tmp/messagedb-test.log 2>&1 &
SERVER_PID=$!

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}→ Cleaning up...${NC}"
    kill $SERVER_PID 2>/dev/null || true
    rm -f /tmp/messagedb-test.log
    rm -rf "$TEST_DATA_DIR" 2>/dev/null || true
}
trap cleanup EXIT

# Wait for server to be ready
echo -e "${YELLOW}→ Waiting for server to start...${NC}"
for i in {1..30}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Server ready at $MESSAGEDB_URL${NC}"
        sleep 0.5
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Server failed to start${NC}"
        echo -e "${YELLOW}Server log:${NC}"
        cat /tmp/messagedb-test.log
        exit 1
    fi
    sleep 0.1
done

echo ""

# Track test results
FAILED=0
PASSED=0

# Run Elixir tests
if [[ "$SDK" == "elixir" || "$SDK" == "all" ]]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  Elixir SDK Tests${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ -f "clients/messagedb_ex/run_tests.sh" ]; then
        cd clients/messagedb_ex
        if MESSAGEDB_URL="$MESSAGEDB_URL" MESSAGEDB_ADMIN_TOKEN="$ADMIN_TOKEN" ./run_tests.sh; then
            PASSED=$((PASSED + 1))
        else
            FAILED=$((FAILED + 1))
        fi
        cd ../..
    else
        echo -e "${RED}✗ Elixir SDK test runner not found${NC}"
        FAILED=$((FAILED + 1))
    fi
    echo ""
fi

# Run Node.js SDK tests
if [[ "$SDK" == "js" || "$SDK" == "node" || "$SDK" == "all" ]]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  Node.js SDK Tests${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ -f "clients/messagedb-node/run_tests.sh" ]; then
        cd clients/messagedb-node
        if MESSAGEDB_URL="$MESSAGEDB_URL" MESSAGEDB_ADMIN_TOKEN="$ADMIN_TOKEN" ./run_tests.sh; then
            PASSED=$((PASSED + 1))
        else
            FAILED=$((FAILED + 1))
        fi
        cd ../..
    else
        echo -e "${RED}✗ Node.js SDK test runner not found${NC}"
        FAILED=$((FAILED + 1))
    fi
    echo ""
fi

# Summary
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Test Summary${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All SDK test suites passed! ($PASSED/$PASSED)${NC}"
    exit 0
else
    echo -e "${RED}✗ Some SDK test suites failed!${NC}"
    echo -e "  Passed: ${GREEN}$PASSED${NC}"
    echo -e "  Failed: ${RED}$FAILED${NC}"
    exit 1
fi
