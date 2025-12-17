#!/bin/bash
#
# Run external tests against MessageDB server
#
# This script:
# 1. Kills any existing test server on port 6789
# 2. Starts a fresh test server with known tokens
# 3. Runs all bun.js tests
# 4. Cleans up the server
#

set -e

PORT=6789
SERVER_BIN="./golang/messagedb"
TEST_DIR="./test_external"

# Known tokens - these are deterministic and match what tests expect
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== MessageDB External Tests ===${NC}"

# Kill any existing server on the port
if lsof -ti:$PORT > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port $PORT...${NC}"
    kill $(lsof -ti:$PORT) 2>/dev/null || true
    sleep 1
fi

# Build server if needed
if [ ! -f "$SERVER_BIN" ]; then
    echo -e "${YELLOW}Building server...${NC}"
    cd golang && go build -o messagedb ./cmd/messagedb && cd ..
fi

# Start test server with known token
echo -e "${YELLOW}Starting test server on port $PORT...${NC}"
$SERVER_BIN -test-mode -port $PORT -token "$DEFAULT_TOKEN" > /dev/null 2>&1 &
SERVER_PID=$!

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    kill $SERVER_PID 2>/dev/null || true
}
trap cleanup EXIT

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server...${NC}"
for i in {1..30}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        echo -e "${GREEN}Server ready!${NC}"
        # Give the server a moment to fully initialize
        sleep 0.5
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}Server failed to start${NC}"
        exit 1
    fi
    sleep 0.1
done

# Run tests (with concurrency limited to avoid overwhelming the server)
echo -e "${YELLOW}Running tests...${NC}"
cd $TEST_DIR
MESSAGEDB_URL="http://localhost:$PORT" bun test --max-concurrency=1
TEST_EXIT_CODE=$?

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
else
    echo -e "${RED}Tests failed!${NC}"
fi

exit $TEST_EXIT_CODE
