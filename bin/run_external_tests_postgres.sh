#!/bin/bash
#
# Run external tests against MessageDB server with PostgreSQL backend
#
# This script:
# 1. Verifies PostgreSQL is accessible
# 2. Kills any existing test server on port 6789
# 3. Starts a fresh test server with PostgreSQL backend
# 4. Runs all bun.js tests
# 5. Cleans up the server
#
# Environment variables (with defaults):
#   POSTGRES_HOST     - PostgreSQL host (default: localhost)
#   POSTGRES_PORT     - PostgreSQL port (default: 5432)
#   POSTGRES_USER     - PostgreSQL user (default: postgres)
#   POSTGRES_PASSWORD - PostgreSQL password (default: postgres)
#   POSTGRES_DB       - PostgreSQL database (default: postgres)
#

set -e

PORT=6789
SERVER_BIN="./golang/messagedb"
TEST_DIR="./test_external"

# PostgreSQL connection settings (with defaults)
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-message_store}"

# Build the connection URL
DB_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"

# Known tokens - these are deterministic and match what tests expect
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== MessageDB External Tests (PostgreSQL) ===${NC}"
echo ""
echo -e "${CYAN}PostgreSQL Configuration:${NC}"
echo -e "  Host:     ${POSTGRES_HOST}"
echo -e "  Port:     ${POSTGRES_PORT}"
echo -e "  User:     ${POSTGRES_USER}"
echo -e "  Database: ${POSTGRES_DB}"
echo ""

# Check if PostgreSQL is accessible
echo -e "${YELLOW}Checking PostgreSQL connection...${NC}"
if command -v psql &> /dev/null; then
    if PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "SELECT 1" > /dev/null 2>&1; then
        echo -e "${GREEN}PostgreSQL connection successful!${NC}"
    else
        echo -e "${RED}Cannot connect to PostgreSQL. Please check your connection settings.${NC}"
        echo -e "${RED}You can set environment variables: POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}psql not found, skipping connection check (will fail at server start if PostgreSQL is unavailable)${NC}"
fi

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

# Clean up any leftover test namespaces from previous runs
echo -e "${YELLOW}Cleaning up old test namespaces...${NC}"
if command -v psql &> /dev/null; then
    PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "
        DO \$\$
        DECLARE
            ns RECORD;
        BEGIN
            -- Drop test schemas
            FOR ns IN SELECT schema_name FROM message_store.namespaces WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default'
            LOOP
                EXECUTE 'DROP SCHEMA IF EXISTS \"' || ns.schema_name || '\" CASCADE';
            END LOOP;
            -- Delete from registry
            DELETE FROM message_store.namespaces WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default';
        EXCEPTION
            WHEN undefined_table THEN
                -- message_store.namespaces doesn't exist yet, that's fine
                NULL;
            WHEN invalid_schema_name THEN
                -- Schema doesn't exist, that's fine
                NULL;
        END \$\$;
    " 2>/dev/null || true
fi

# Start server with PostgreSQL backend
echo -e "${YELLOW}Starting test server on port $PORT with PostgreSQL backend...${NC}"
$SERVER_BIN -port $PORT -db-url "$DB_URL" -token "$DEFAULT_TOKEN" > /tmp/messagedb_postgres.log 2>&1 &
SERVER_PID=$!

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    kill $SERVER_PID 2>/dev/null || true
    
    # Clean up test namespaces
    if command -v psql &> /dev/null; then
        echo -e "${YELLOW}Cleaning up test namespaces from PostgreSQL...${NC}"
        PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "
            DO \$\$
            DECLARE
                ns RECORD;
            BEGIN
                FOR ns IN SELECT schema_name FROM message_store.namespaces WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default'
                LOOP
                    EXECUTE 'DROP SCHEMA IF EXISTS \"' || ns.schema_name || '\" CASCADE';
                END LOOP;
                DELETE FROM message_store.namespaces WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default';
            EXCEPTION
                WHEN undefined_table THEN NULL;
                WHEN invalid_schema_name THEN NULL;
            END \$\$;
        " 2>/dev/null || true
    fi
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
        echo -e "${RED}Server failed to start. Check /tmp/messagedb_postgres.log for details.${NC}"
        cat /tmp/messagedb_postgres.log
        exit 1
    fi
    sleep 0.2
done

# Run tests (with concurrency limited to avoid overwhelming the server)
echo -e "${YELLOW}Running tests...${NC}"
cd $TEST_DIR
MESSAGEDB_URL="http://localhost:$PORT" bun test --max-concurrency=1
TEST_EXIT_CODE=$?

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}All tests passed with PostgreSQL backend!${NC}"
else
    echo -e "${RED}Tests failed!${NC}"
fi

exit $TEST_EXIT_CODE
