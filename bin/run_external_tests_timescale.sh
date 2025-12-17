#!/bin/bash
#
# Run external tests against MessageDB server with TimescaleDB backend
#
# This script:
# 1. Creates a dedicated test database (messagedb_timescale_test)
# 2. Enables TimescaleDB extension
# 3. Verifies TimescaleDB is accessible
# 4. Kills any existing test server on port 6789
# 5. Starts a fresh test server with TimescaleDB backend
# 6. Runs all bun.js tests
# 7. Cleans up the server and test database
#
# Environment variables (with defaults):
#   TIMESCALE_HOST     - TimescaleDB host (default: localhost)
#   TIMESCALE_PORT     - TimescaleDB port (default: 6666)
#   TIMESCALE_USER     - TimescaleDB user (default: postgres)
#   TIMESCALE_PASSWORD - TimescaleDB password (default: postgres)
#   TIMESCALE_DB       - TimescaleDB database for tests (default: messagedb_timescale_test)
#   KEEP_TEST_DB       - If set to "1", don't drop test database after tests
#

set -e

PORT=6789
SERVER_BIN="./golang/messagedb"
TEST_DIR="./test_external"

# TimescaleDB connection settings (with defaults)
TIMESCALE_HOST="${TIMESCALE_HOST:-localhost}"
TIMESCALE_PORT="${TIMESCALE_PORT:-6666}"
TIMESCALE_USER="${TIMESCALE_USER:-postgres}"
TIMESCALE_PASSWORD="${TIMESCALE_PASSWORD:-postgres}"
TIMESCALE_DB="${TIMESCALE_DB:-messagedb_timescale_test}"

# Admin connection (to postgres database for creating test database)
ADMIN_DB_URL="postgres://${TIMESCALE_USER}:${TIMESCALE_PASSWORD}@${TIMESCALE_HOST}:${TIMESCALE_PORT}/postgres?sslmode=disable"

# Test database connection
DB_URL="postgres://${TIMESCALE_USER}:${TIMESCALE_PASSWORD}@${TIMESCALE_HOST}:${TIMESCALE_PORT}/${TIMESCALE_DB}?sslmode=disable"

# Known tokens - these are deterministic and match what tests expect
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     MessageDB External Tests (TimescaleDB Backend)           ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${CYAN}TimescaleDB Configuration:${NC}"
echo -e "  Host:          ${TIMESCALE_HOST}"
echo -e "  Port:          ${TIMESCALE_PORT}"
echo -e "  User:          ${TIMESCALE_USER}"
echo -e "  Test Database: ${TIMESCALE_DB}"
echo ""

# Check if psql is available
if ! command -v psql &> /dev/null; then
    echo -e "${RED}psql command not found. Please install PostgreSQL client.${NC}"
    exit 1
fi

# Check if TimescaleDB is accessible
echo -e "${YELLOW}Checking TimescaleDB connection...${NC}"
if ! PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "postgres" -c "SELECT 1" > /dev/null 2>&1; then
    echo -e "${RED}Cannot connect to TimescaleDB at ${TIMESCALE_HOST}:${TIMESCALE_PORT}${NC}"
    echo -e "${RED}Please ensure TimescaleDB is running.${NC}"
    echo -e "${YELLOW}Hint: Check docker-compose.yml or start TimescaleDB manually.${NC}"
    exit 1
fi

# Check if TimescaleDB extension is available
echo -e "${YELLOW}Checking TimescaleDB extension availability...${NC}"
TSDB_VERSION=$(PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "postgres" -t -c "SELECT default_version FROM pg_available_extensions WHERE name = 'timescaledb'" 2>/dev/null | tr -d ' ')
if [ -z "$TSDB_VERSION" ]; then
    echo -e "${RED}TimescaleDB extension is not available on this PostgreSQL server.${NC}"
    echo -e "${RED}Please use a TimescaleDB-enabled PostgreSQL instance.${NC}"
    exit 1
fi
echo -e "${GREEN}TimescaleDB version ${TSDB_VERSION} available!${NC}"

# Create test database
echo -e "${YELLOW}Creating test database '${TIMESCALE_DB}'...${NC}"
PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "postgres" -c "DROP DATABASE IF EXISTS ${TIMESCALE_DB};" 2>/dev/null || true
PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "postgres" -c "CREATE DATABASE ${TIMESCALE_DB};"

# Enable TimescaleDB extension in test database
echo -e "${YELLOW}Enabling TimescaleDB extension...${NC}"
PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "${TIMESCALE_DB}" -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"

# Verify TimescaleDB is enabled
TSDB_INSTALLED=$(PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "${TIMESCALE_DB}" -t -c "SELECT extversion FROM pg_extension WHERE extname = 'timescaledb'" | tr -d ' ')
if [ -z "$TSDB_INSTALLED" ]; then
    echo -e "${RED}Failed to enable TimescaleDB extension!${NC}"
    exit 1
fi
echo -e "${GREEN}TimescaleDB ${TSDB_INSTALLED} enabled in test database!${NC}"

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

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Kill server
    if [ ! -z "$SERVER_PID" ]; then
        kill $SERVER_PID 2>/dev/null || true
    fi
    
    # Drop test database (unless KEEP_TEST_DB is set)
    if [ "${KEEP_TEST_DB}" != "1" ]; then
        echo -e "${YELLOW}Dropping test database '${TIMESCALE_DB}'...${NC}"
        PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "postgres" -c "DROP DATABASE IF EXISTS ${TIMESCALE_DB};" 2>/dev/null || true
        echo -e "${GREEN}Test database dropped.${NC}"
    else
        echo -e "${CYAN}Keeping test database '${TIMESCALE_DB}' (KEEP_TEST_DB=1)${NC}"
    fi
}
trap cleanup EXIT

# Start server with TimescaleDB backend
echo -e "${YELLOW}Starting test server on port $PORT with TimescaleDB backend...${NC}"
$SERVER_BIN -port $PORT -db-url "$DB_URL" -db-type timescale -token "$DEFAULT_TOKEN" > /tmp/messagedb_timescale.log 2>&1 &
SERVER_PID=$!

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server...${NC}"
for i in {1..30}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        echo -e "${GREEN}Server ready!${NC}"
        sleep 0.5
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}Server failed to start. Check /tmp/messagedb_timescale.log for details:${NC}"
        cat /tmp/messagedb_timescale.log
        exit 1
    fi
    sleep 0.2
done

# Show server info
echo ""
echo -e "${CYAN}Server Info:${NC}"
curl -s http://localhost:$PORT/health | jq . 2>/dev/null || curl -s http://localhost:$PORT/health
echo ""

# Run tests
echo -e "${YELLOW}Running tests...${NC}"
cd $TEST_DIR
MESSAGEDB_URL="http://localhost:$PORT" bun test --max-concurrency=1
TEST_EXIT_CODE=$?

# Show TimescaleDB-specific info after tests
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo ""
    echo -e "${CYAN}TimescaleDB Chunk Information:${NC}"
    PGPASSWORD="${TIMESCALE_PASSWORD}" psql -h "${TIMESCALE_HOST}" -p "${TIMESCALE_PORT}" -U "${TIMESCALE_USER}" -d "${TIMESCALE_DB}" -c "
        SELECT 
            hypertable_schema,
            hypertable_name,
            chunk_name,
            range_start,
            range_end,
            is_compressed,
            pg_size_pretty(pg_total_relation_size(format('%I.%I', chunk_schema, chunk_name)::regclass)) as size
        FROM timescaledb_information.chunks
        ORDER BY hypertable_schema, range_start
        LIMIT 10;
    " 2>/dev/null || true
    
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║     All tests passed with TimescaleDB backend!               ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
else
    echo ""
    echo -e "${RED}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║     Tests failed!                                            ║${NC}"
    echo -e "${RED}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${YELLOW}Server logs:${NC}"
    tail -50 /tmp/messagedb_timescale.log
fi

exit $TEST_EXIT_CODE
