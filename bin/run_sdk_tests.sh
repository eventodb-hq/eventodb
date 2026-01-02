#!/bin/bash
#
# Run SDK tests against EventoDB server
#
# Usage: ./bin/run_sdk_tests.sh [elixir|js|node|golang|go|all]
#
# This script:
# 1. Builds the server (if needed)
# 2. Starts a test server with SQLite backend
# 3. Runs SDK-specific tests
# 4. Cleans up
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SDK="${1:-all}"
PORT=6789
BACKEND="sqlite"
EVENTODB_URL="http://localhost:$PORT"
# Known admin token for default namespace (used for creating test namespaces)
ADMIN_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

usage() {
    echo "Usage: $0 [elixir|js|node|golang|go|all]"
    echo ""
    echo "Options:"
    echo "  elixir   - Run Elixir SDK tests only"
    echo "  js       - Run Node.js SDK tests only (alias for 'node')"
    echo "  node     - Run Node.js SDK tests only"
    echo "  golang   - Run Golang SDK tests only"
    echo "  go       - Run Golang SDK tests only (alias for 'golang')"
    echo "  all      - Run all SDK tests (default)"
    exit 1
}

if [[ ! "$SDK" =~ ^(elixir|js|node|golang|go|all)$ ]]; then
    echo -e "${RED}Invalid SDK: $SDK${NC}"
    usage
fi

echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   EventoDB SDK Test Runner            ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}→ Cleaning up...${NC}"
    "$SCRIPT_DIR/manage_test_server.sh" stop "$PORT" || true
}
trap cleanup EXIT

# Start test server
echo -e "${YELLOW}→ Starting test server on port $PORT...${NC}"
if "$SCRIPT_DIR/manage_test_server.sh" start "$BACKEND" "$PORT"; then
    echo -e "${GREEN}✓ Server ready at $EVENTODB_URL${NC}"
else
    echo -e "${RED}✗ Server failed to start${NC}"
    exit 1
fi

echo ""

# Track test results
FAILED=0
PASSED=0

# Run Elixir tests
if [[ "$SDK" == "elixir" || "$SDK" == "all" ]]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  Elixir SDK Tests${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ -f "clients/eventodb_ex/run_tests.sh" ]; then
        cd clients/eventodb_ex
        if EVENTODB_URL="$EVENTODB_URL" EVENTODB_ADMIN_TOKEN="$ADMIN_TOKEN" ./run_tests.sh; then
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
    
    if [ -f "clients/eventodb-node/run_tests.sh" ]; then
        cd clients/eventodb-node
        if EVENTODB_URL="$EVENTODB_URL" EVENTODB_ADMIN_TOKEN="$ADMIN_TOKEN" ./run_tests.sh; then
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

# Run Golang SDK tests
if [[ "$SDK" == "golang" || "$SDK" == "go" || "$SDK" == "all" ]]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  Golang SDK Tests${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ -f "clients/eventodb-go/run_tests.sh" ]; then
        cd clients/eventodb-go
        if EVENTODB_URL="$EVENTODB_URL" EVENTODB_ADMIN_TOKEN="$ADMIN_TOKEN" ./run_tests.sh; then
            PASSED=$((PASSED + 1))
        else
            FAILED=$((FAILED + 1))
        fi
        cd ../..
    else
        echo -e "${RED}✗ Golang SDK test runner not found${NC}"
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
