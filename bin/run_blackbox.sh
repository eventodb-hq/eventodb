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
#
# Environment variables for PostgreSQL/TimescaleDB:
#   POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
#   TIMESCALE_HOST, TIMESCALE_PORT, TIMESCALE_USER, TIMESCALE_PASSWORD, TIMESCALE_DB
#

set -e

# Configuration
PORT=6789
SERVER_BIN="./dist/eventodb"
TEST_DIR="./test_external"
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

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

# Kill any existing server on the port
if lsof -ti:$PORT > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port $PORT...${NC}"
    kill $(lsof -ti:$PORT) 2>/dev/null || true
    sleep 1
fi

# Build server if needed
if [ ! -f "$SERVER_BIN" ]; then
    echo -e "${YELLOW}Building server...${NC}"
    CGO_ENABLED=0 go build -o ../dist/eventodb ./cmd/eventodb
fi

# Backend-specific setup and server startup
case "$BACKEND" in
    sqlite)
        echo -e "${CYAN}Backend: SQLite (in-memory)${NC}"
        echo -e "${YELLOW}Starting server...${NC}"
        $SERVER_BIN -test-mode -port $PORT -token "$DEFAULT_TOKEN" > /tmp/eventodb_blackbox.log 2>&1 &
        SERVER_PID=$!
        ;;
        
    pebble)
        echo -e "${CYAN}Backend: Pebble (in-memory)${NC}"
        echo ""
        echo -e "${YELLOW}Starting server...${NC}"
        # Use test-mode flag for in-memory Pebble (no temp directory needed)
        $SERVER_BIN -port $PORT -db-url "pebble://memory" -test-mode -token "$DEFAULT_TOKEN" > /tmp/eventodb_blackbox.log 2>&1 &
        SERVER_PID=$!
        ;;
        
    postgres)
        # PostgreSQL configuration
        POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
        POSTGRES_PORT="${POSTGRES_PORT:-5432}"
        POSTGRES_USER="${POSTGRES_USER:-postgres}"
        POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
        POSTGRES_DB="${POSTGRES_DB:-eventodb_store}"
        
        DB_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"
        
        echo -e "${CYAN}Backend: PostgreSQL${NC}"
        echo -e "  Host:     $POSTGRES_HOST"
        echo -e "  Port:     $POSTGRES_PORT"
        echo -e "  User:     $POSTGRES_USER"
        echo -e "  Database: $POSTGRES_DB"
        echo ""
        
        # Check PostgreSQL connection
        echo -e "${YELLOW}Checking PostgreSQL connection...${NC}"
        if command -v psql &> /dev/null; then
            if PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT 1" > /dev/null 2>&1; then
                echo -e "${GREEN}PostgreSQL connection successful!${NC}"
            else
                echo -e "${RED}Cannot connect to PostgreSQL!${NC}"
                echo -e "${RED}Check connection settings or set environment variables.${NC}"
                exit 1
            fi
        else
            echo -e "${YELLOW}psql not found, skipping connection check${NC}"
        fi
        
        # Clean up old test data
        echo -e "${YELLOW}Cleaning up old test namespaces...${NC}"
        if command -v psql &> /dev/null; then
            PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
                DO \$\$
                DECLARE ns RECORD;
                BEGIN
                    FOR ns IN SELECT schema_name FROM eventodb_store.namespaces 
                             WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default'
                    LOOP
                        EXECUTE 'DROP SCHEMA IF EXISTS \"' || ns.schema_name || '\" CASCADE';
                    END LOOP;
                    DELETE FROM eventodb_store.namespaces 
                    WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default';
                EXCEPTION
                    WHEN undefined_table THEN NULL;
                    WHEN invalid_schema_name THEN NULL;
                END \$\$;
            " 2>/dev/null || true
        fi
        
        echo -e "${YELLOW}Starting server...${NC}"
        $SERVER_BIN -port $PORT -db-url "$DB_URL" -token "$DEFAULT_TOKEN" > /tmp/eventodb_blackbox.log 2>&1 &
        SERVER_PID=$!
        ;;
        
    timescale)
        # TimescaleDB configuration
        TIMESCALE_HOST="${TIMESCALE_HOST:-localhost}"
        TIMESCALE_PORT="${TIMESCALE_PORT:-6666}"
        TIMESCALE_USER="${TIMESCALE_USER:-postgres}"
        TIMESCALE_PASSWORD="${TIMESCALE_PASSWORD:-postgres}"
        TIMESCALE_DB="${TIMESCALE_DB:-eventodb_timescale_test}"
        
        DB_URL="postgres://${TIMESCALE_USER}:${TIMESCALE_PASSWORD}@${TIMESCALE_HOST}:${TIMESCALE_PORT}/${TIMESCALE_DB}?sslmode=disable"
        
        echo -e "${CYAN}Backend: TimescaleDB${NC}"
        echo -e "  Host:     $TIMESCALE_HOST"
        echo -e "  Port:     $TIMESCALE_PORT"
        echo -e "  User:     $TIMESCALE_USER"
        echo -e "  Database: $TIMESCALE_DB"
        echo ""
        
        # Check psql availability
        if ! command -v psql &> /dev/null; then
            echo -e "${RED}psql command not found. Please install PostgreSQL client.${NC}"
            exit 1
        fi
        
        # Check TimescaleDB connection
        echo -e "${YELLOW}Checking TimescaleDB connection...${NC}"
        if ! PGPASSWORD="$TIMESCALE_PASSWORD" psql -h "$TIMESCALE_HOST" -p "$TIMESCALE_PORT" -U "$TIMESCALE_USER" -d "postgres" -c "SELECT 1" > /dev/null 2>&1; then
            echo -e "${RED}Cannot connect to TimescaleDB at ${TIMESCALE_HOST}:${TIMESCALE_PORT}${NC}"
            exit 1
        fi
        
        # Check TimescaleDB extension availability
        echo -e "${YELLOW}Checking TimescaleDB extension...${NC}"
        TSDB_VERSION=$(PGPASSWORD="$TIMESCALE_PASSWORD" psql -h "$TIMESCALE_HOST" -p "$TIMESCALE_PORT" -U "$TIMESCALE_USER" -d "postgres" -t -c "SELECT default_version FROM pg_available_extensions WHERE name = 'timescaledb'" 2>/dev/null | tr -d ' ')
        if [ -z "$TSDB_VERSION" ]; then
            echo -e "${RED}TimescaleDB extension not available!${NC}"
            exit 1
        fi
        echo -e "${GREEN}TimescaleDB version ${TSDB_VERSION} available!${NC}"
        
        # Create and setup test database
        echo -e "${YELLOW}Creating test database...${NC}"
        PGPASSWORD="$TIMESCALE_PASSWORD" psql -h "$TIMESCALE_HOST" -p "$TIMESCALE_PORT" -U "$TIMESCALE_USER" -d "postgres" -c "DROP DATABASE IF EXISTS ${TIMESCALE_DB};" 2>/dev/null || true
        PGPASSWORD="$TIMESCALE_PASSWORD" psql -h "$TIMESCALE_HOST" -p "$TIMESCALE_PORT" -U "$TIMESCALE_USER" -d "postgres" -c "CREATE DATABASE ${TIMESCALE_DB};"
        
        echo -e "${YELLOW}Enabling TimescaleDB extension...${NC}"
        PGPASSWORD="$TIMESCALE_PASSWORD" psql -h "$TIMESCALE_HOST" -p "$TIMESCALE_PORT" -U "$TIMESCALE_USER" -d "$TIMESCALE_DB" -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"
        
        echo -e "${YELLOW}Starting server...${NC}"
        $SERVER_BIN -port $PORT -db-url "$DB_URL" -db-type timescale -token "$DEFAULT_TOKEN" > /tmp/eventodb_blackbox.log 2>&1 &
        SERVER_PID=$!
        ;;
esac

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Kill server
    if [ ! -z "$SERVER_PID" ]; then
        kill $SERVER_PID 2>/dev/null || true
    fi
    
    # Backend-specific cleanup
    case "$BACKEND" in
        pebble)
            # In-memory mode - no cleanup needed
            ;;
            
        postgres)
            if command -v psql &> /dev/null; then
                echo -e "${YELLOW}Cleaning up test namespaces...${NC}"
                PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
                    DO \$\$
                    DECLARE ns RECORD;
                    BEGIN
                        FOR ns IN SELECT schema_name FROM eventodb_store.namespaces 
                                 WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default'
                        LOOP
                            EXECUTE 'DROP SCHEMA IF EXISTS \"' || ns.schema_name || '\" CASCADE';
                        END LOOP;
                        DELETE FROM eventodb_store.namespaces 
                        WHERE id LIKE 'test_%' OR id LIKE 'bench_%' OR id = 'default';
                    EXCEPTION
                        WHEN undefined_table THEN NULL;
                        WHEN invalid_schema_name THEN NULL;
                    END \$\$;
                " 2>/dev/null || true
            fi
            ;;
            
        timescale)
            if [ "${KEEP_TEST_DB}" != "1" ]; then
                echo -e "${YELLOW}Dropping test database...${NC}"
                PGPASSWORD="$TIMESCALE_PASSWORD" psql -h "$TIMESCALE_HOST" -p "$TIMESCALE_PORT" -U "$TIMESCALE_USER" -d "postgres" -c "DROP DATABASE IF EXISTS ${TIMESCALE_DB};" 2>/dev/null || true
            fi
            ;;
    esac
}
trap cleanup EXIT

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server to be ready...${NC}"
for i in {1..50}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        echo -e "${GREEN}Server ready!${NC}"
        # Give extra time for Pebble/Postgres to fully initialize
        sleep 1
        break
    fi
    if [ $i -eq 50 ]; then
        echo -e "${RED}Server failed to start!${NC}"
        echo -e "${YELLOW}Server logs:${NC}"
        cat /tmp/eventodb_blackbox.log
        exit 1
    fi
    sleep 0.2
done

# Run tests
echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Running tests...${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
cd $TEST_DIR
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
    echo ""
    echo -e "${YELLOW}Server logs (last 30 lines):${NC}"
    tail -30 /tmp/eventodb_blackbox.log
fi

exit $TEST_EXIT_CODE
