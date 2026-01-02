#!/bin/bash
#
# Run external (blackbox) tests against EventoDB server
#
# Usage: bin/run_blackbox.sh <backend>
#
# Backends:
#   sqlite      - In-memory SQLite (default, no setup required)
#   postgres    - PostgreSQL (requires running PostgreSQL)
#   pebble      - Pebble embedded KV store
#   timescale   - TimescaleDB (requires TimescaleDB extension)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
PORT=6789
TEST_DIR="$PROJECT_ROOT/test_external"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BLUE='\033[0;34m'
NC='\033[0m'

# Parse backend argument
BACKEND="${1:-sqlite}"

# Validate backend
case "$BACKEND" in
    sqlite|postgres|pebble|timescale)
        ;;
    *)
        echo -e "${RED}Error: Invalid backend '$BACKEND'${NC}"
        echo ""
        echo "Usage: $0 <backend>"
        echo ""
        echo "Available backends:"
        echo "  sqlite      - In-memory SQLite (default)"
        echo "  postgres    - PostgreSQL"
        echo "  pebble      - Pebble embedded KV store"
        echo "  timescale   - TimescaleDB"
        echo ""
        echo "Examples:"
        echo "  $0 sqlite"
        echo "  $0 postgres"
        echo "  $0 pebble"
        exit 1
        ;;
esac

# Backend-specific icons
case "$BACKEND" in
    sqlite) ICON="📦" ;;
    postgres) ICON="🐘" ;;
    pebble) ICON="🪨" ;;
    timescale) ICON="⏱️" ;;
esac

# Print header
BACKEND_UPPER=$(echo "$BACKEND" | tr '[:lower:]' '[:upper:]')
echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║  $ICON EventoDB Blackbox Tests - ${BACKEND_UPPER}${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Stop server
    "$SCRIPT_DIR/manage_test_server.sh" stop "$PORT" || true
    
    # Backend-specific cleanup
    "$SCRIPT_DIR/manage_test_server.sh" cleanup "$BACKEND" "$PORT" || true
}
trap cleanup EXIT

# Start test server
echo -e "${YELLOW}Starting test server...${NC}"
if EVENTODB_TEST_MODE=1 "$SCRIPT_DIR/manage_test_server.sh" start "$BACKEND" "$PORT"; then
    echo -e "${GREEN}Server ready!${NC}"
else
    echo -e "${RED}Failed to start server${NC}"
    exit 1
fi

# Run tests
echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Running tests...${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
cd "$TEST_DIR"
EVENTODB_URL="http://localhost:$PORT" bun test --max-concurrency=1
TEST_EXIT_CODE=$?

# Print results
echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║  $ICON All tests PASSED with ${BACKEND_UPPER} backend!${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
else
    echo -e "${RED}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║  $ICON Tests FAILED with ${BACKEND_UPPER} backend!${NC}"
    echo -e "${RED}╚══════════════════════════════════════════════════════════════╝${NC}"
fi

exit $TEST_EXIT_CODE
