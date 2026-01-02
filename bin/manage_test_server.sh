#!/bin/bash
#
# EventoDB Test Server Manager
# Unified script to start/stop/check test servers with consistent configuration
#
# Usage:
#   bin/manage_test_server.sh start [backend] [port]
#   bin/manage_test_server.sh stop [port]
#   bin/manage_test_server.sh restart [backend] [port]
#   bin/manage_test_server.sh status [port]
#   bin/manage_test_server.sh wait [port]
#
# Backends:
#   sqlite      - In-memory SQLite (default)
#   postgres    - PostgreSQL
#   pebble      - Pebble embedded KV store
#   timescale   - TimescaleDB
#
# Environment variables:
#   POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
#   TIMESCALE_HOST, TIMESCALE_PORT, TIMESCALE_USER, TIMESCALE_PASSWORD, TIMESCALE_DB
#   EVENTODB_TEST_MODE - If set to "1", uses --test-mode flag
#   EVENTODB_LOG_FILE - Log file path (default: /tmp/eventodb_test_<port>.log)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default configuration
DEFAULT_PORT=6789
DEFAULT_BACKEND=sqlite
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"
SERVER_BIN="$PROJECT_ROOT/dist/eventodb"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Print functions
print_error() {
    echo -e "${RED}✗${NC} $1" >&2
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_info() {
    echo -e "${BLUE}→${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}!${NC} $1"
}

# Show usage
usage() {
    echo "EventoDB Test Server Manager"
    echo ""
    echo "Usage:"
    echo "  $0 start [backend] [port]    - Start test server"
    echo "  $0 stop [port]                - Stop test server"
    echo "  $0 restart [backend] [port]   - Restart test server"
    echo "  $0 status [port]              - Check if server is running"
    echo "  $0 wait [port] [timeout]      - Wait for server to be ready"
    echo "  $0 cleanup [backend] [port]   - Cleanup test data"
    echo ""
    echo "Backends:"
    echo "  sqlite      - In-memory SQLite (default)"
    echo "  postgres    - PostgreSQL"
    echo "  pebble      - Pebble embedded KV store  "
    echo "  timescale   - TimescaleDB"
    echo ""
    echo "Environment:"
    echo "  EVENTODB_TEST_MODE=1          - Use --test-mode flag"
    echo "  EVENTODB_LOG_FILE=<path>      - Custom log file path"
    echo "  POSTGRES_HOST, POSTGRES_PORT  - PostgreSQL connection"
    echo "  POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB"
    echo ""
    echo "Examples:"
    echo "  $0 start sqlite 8080"
    echo "  $0 stop 8080"
    echo "  $0 restart postgres 6789"
    echo "  EVENTODB_TEST_MODE=1 $0 start pebble"
    exit 1
}

# Check if port is in use
is_port_in_use() {
    local port=$1
    lsof -ti:$port > /dev/null 2>&1
}

# Kill process on port
kill_port() {
    local port=$1
    if is_port_in_use "$port"; then
        print_info "Killing process on port $port..."
        kill $(lsof -ti:$port) 2>/dev/null || true
        sleep 1
        
        # Force kill if still running
        if is_port_in_use "$port"; then
            print_warning "Force killing process on port $port..."
            kill -9 $(lsof -ti:$port) 2>/dev/null || true
            sleep 1
        fi
        
        if is_port_in_use "$port"; then
            print_error "Failed to kill process on port $port"
            return 1
        fi
        print_success "Port $port is now free"
    fi
    return 0
}

# Build server if needed
build_server() {
    if [ ! -f "$SERVER_BIN" ]; then
        print_info "Building EventoDB server..."
        cd "$PROJECT_ROOT/golang"
        CGO_ENABLED=0 go build -o ../dist/eventodb ./cmd/eventodb
        cd "$PROJECT_ROOT"
        print_success "Server built"
    fi
}

# Setup PostgreSQL database
setup_postgres() {
    local host="${POSTGRES_HOST:-localhost}"
    local port="${POSTGRES_PORT:-5432}"
    local user="${POSTGRES_USER:-postgres}"
    local password="${POSTGRES_PASSWORD:-postgres}"
    local db="${POSTGRES_DB:-eventodb_store}"
    
    # Check connection
    if ! command -v psql &> /dev/null; then
        print_warning "psql not found, skipping PostgreSQL setup check"
        return 0
    fi
    
    if ! PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "$db" -c "SELECT 1" > /dev/null 2>&1; then
        print_error "Cannot connect to PostgreSQL at ${host}:${port}"
        return 1
    fi
    
    # Cleanup old test data
    print_info "Cleaning up old PostgreSQL test data..."
    PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "$db" -c "
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
    
    print_success "PostgreSQL setup complete"
}

# Setup TimescaleDB database
setup_timescale() {
    local host="${TIMESCALE_HOST:-localhost}"
    local port="${TIMESCALE_PORT:-6666}"
    local user="${TIMESCALE_USER:-postgres}"
    local password="${TIMESCALE_PASSWORD:-postgres}"
    local db="${TIMESCALE_DB:-eventodb_timescale_test}"
    
    if ! command -v psql &> /dev/null; then
        print_error "psql not found - required for TimescaleDB"
        return 1
    fi
    
    # Check connection
    if ! PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "postgres" -c "SELECT 1" > /dev/null 2>&1; then
        print_error "Cannot connect to TimescaleDB at ${host}:${port}"
        return 1
    fi
    
    # Check TimescaleDB extension
    local tsdb_version=$(PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "postgres" -t -c "SELECT default_version FROM pg_available_extensions WHERE name = 'timescaledb'" 2>/dev/null | tr -d ' ')
    if [ -z "$tsdb_version" ]; then
        print_error "TimescaleDB extension not available"
        return 1
    fi
    
    print_info "Creating TimescaleDB test database..."
    PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "postgres" -c "DROP DATABASE IF EXISTS ${db};" 2>/dev/null || true
    PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "postgres" -c "CREATE DATABASE ${db};"
    PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "$db" -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"
    
    print_success "TimescaleDB setup complete (version $tsdb_version)"
}

# Get database URL for backend
get_db_url() {
    local backend=$1
    
    case "$backend" in
        sqlite)
            echo "sqlite://:memory:"
            ;;
        pebble)
            echo "pebble://memory"
            ;;
        postgres)
            local host="${POSTGRES_HOST:-localhost}"
            local port="${POSTGRES_PORT:-5432}"
            local user="${POSTGRES_USER:-postgres}"
            local password="${POSTGRES_PASSWORD:-postgres}"
            local db="${POSTGRES_DB:-eventodb_store}"
            echo "postgres://${user}:${password}@${host}:${port}/${db}?sslmode=disable"
            ;;
        timescale)
            local host="${TIMESCALE_HOST:-localhost}"
            local port="${TIMESCALE_PORT:-6666}"
            local user="${TIMESCALE_USER:-postgres}"
            local password="${TIMESCALE_PASSWORD:-postgres}"
            local db="${TIMESCALE_DB:-eventodb_timescale_test}"
            echo "postgres://${user}:${password}@${host}:${port}/${db}?sslmode=disable"
            ;;
        *)
            print_error "Unknown backend: $backend"
            return 1
            ;;
    esac
}

# Start test server
start_server() {
    local backend=${1:-$DEFAULT_BACKEND}
    local port=${2:-$DEFAULT_PORT}
    local log_file="${EVENTODB_LOG_FILE:-/tmp/eventodb_test_${port}.log}"
    
    print_info "Starting EventoDB test server..."
    print_info "  Backend: $backend"
    print_info "  Port: $port"
    print_info "  Log: $log_file"
    
    # Validate backend
    case "$backend" in
        sqlite|pebble|postgres|timescale) ;;
        *)
            print_error "Invalid backend: $backend"
            print_error "Valid backends: sqlite, postgres, pebble, timescale"
            return 1
            ;;
    esac
    
    # Kill existing process on port
    kill_port "$port" || return 1
    
    # Build server
    build_server
    
    # Setup backend if needed
    case "$backend" in
        postgres)
            setup_postgres || return 1
            ;;
        timescale)
            setup_timescale || return 1
            ;;
    esac
    
    # Build command
    local cmd="$SERVER_BIN -port $port -token $DEFAULT_TOKEN"
    
    # Add database URL
    local db_url=$(get_db_url "$backend")
    cmd="$cmd -db-url \"$db_url\""
    
    # Add backend type for timescale
    if [ "$backend" = "timescale" ]; then
        cmd="$cmd -db-type timescale"
    fi
    
    # Add data dir for SQLite (always needed)
    if [ "$backend" = "sqlite" ]; then
        local data_dir="/tmp/eventodb-test-${port}-$$"
        mkdir -p "$data_dir"
        cmd="$cmd -data-dir \"$data_dir\""
        echo "$data_dir" > "/tmp/eventodb_datadir_${port}.txt"
    fi
    
    # Add test mode flag if requested
    if [ "${EVENTODB_TEST_MODE}" = "1" ]; then
        cmd="$cmd -test-mode"
    fi
    
    # Start server in background
    print_info "Command: $cmd"
    eval "$cmd" > "$log_file" 2>&1 &
    local pid=$!
    echo "$pid" > "/tmp/eventodb_pid_${port}.txt"
    
    print_success "Server started (PID: $pid)"
    
    # Wait for server to be ready
    wait_for_server "$port" 30
    
    return $?
}

# Stop test server
stop_server() {
    local port=${1:-$DEFAULT_PORT}
    
    print_info "Stopping EventoDB test server on port $port..."
    
    # Kill process
    kill_port "$port"
    
    # Cleanup PID file
    rm -f "/tmp/eventodb_pid_${port}.txt"
    
    # Cleanup data dir
    if [ -f "/tmp/eventodb_datadir_${port}.txt" ]; then
        local data_dir=$(cat "/tmp/eventodb_datadir_${port}.txt")
        if [ -d "$data_dir" ]; then
            print_info "Cleaning up data directory: $data_dir"
            rm -rf "$data_dir"
        fi
        rm -f "/tmp/eventodb_datadir_${port}.txt"
    fi
    
    print_success "Server stopped"
}

# Restart server
restart_server() {
    local backend=${1:-$DEFAULT_BACKEND}
    local port=${2:-$DEFAULT_PORT}
    
    print_info "Restarting EventoDB test server..."
    stop_server "$port" || true
    sleep 1
    start_server "$backend" "$port"
}

# Check server status
check_status() {
    local port=${1:-$DEFAULT_PORT}
    
    if curl -sf http://localhost:$port/health > /dev/null 2>&1; then
        print_success "EventoDB is running on port $port"
        
        # Show PID if available
        if [ -f "/tmp/eventodb_pid_${port}.txt" ]; then
            local pid=$(cat "/tmp/eventodb_pid_${port}.txt")
            echo "  PID: $pid"
        fi
        
        return 0
    else
        print_error "EventoDB is not running on port $port"
        return 1
    fi
}

# Wait for server to be ready
wait_for_server() {
    local port=${1:-$DEFAULT_PORT}
    local timeout=${2:-30}
    
    print_info "Waiting for server to be ready (timeout: ${timeout}s)..."
    
    for i in $(seq 1 $timeout); do
        if curl -sf http://localhost:$port/health > /dev/null 2>&1; then
            print_success "Server is ready!"
            # Give extra time for full initialization
            sleep 1
            return 0
        fi
        sleep 1
    done
    
    print_error "Server failed to become ready within ${timeout}s"
    
    # Show logs on failure
    local log_file="${EVENTODB_LOG_FILE:-/tmp/eventodb_test_${port}.log}"
    if [ -f "$log_file" ]; then
        print_error "Server logs:"
        tail -20 "$log_file"
    fi
    
    return 1
}

# Cleanup backend data
cleanup_backend() {
    local backend=${1:-$DEFAULT_BACKEND}
    local port=${2:-$DEFAULT_PORT}
    
    print_info "Cleaning up $backend test data..."
    
    case "$backend" in
        postgres)
            setup_postgres
            ;;
        timescale)
            local host="${TIMESCALE_HOST:-localhost}"
            local port="${TIMESCALE_PORT:-6666}"
            local user="${TIMESCALE_USER:-postgres}"
            local password="${TIMESCALE_PASSWORD:-postgres}"
            local db="${TIMESCALE_DB:-eventodb_timescale_test}"
            
            if command -v psql &> /dev/null; then
                print_info "Dropping TimescaleDB test database..."
                PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "postgres" -c "DROP DATABASE IF EXISTS ${db};" 2>/dev/null || true
            fi
            ;;
        sqlite|pebble)
            # In-memory - nothing to cleanup
            print_info "In-memory backend - nothing to cleanup"
            ;;
    esac
    
    print_success "Cleanup complete"
}

# Main command dispatcher
case "${1:-}" in
    start)
        start_server "$2" "$3"
        ;;
    stop)
        stop_server "$2"
        ;;
    restart)
        restart_server "$2" "$3"
        ;;
    status)
        check_status "$2"
        ;;
    wait)
        wait_for_server "$2" "$3"
        ;;
    cleanup)
        cleanup_backend "$2" "$3"
        ;;
    -h|--help|help|"")
        usage
        ;;
    *)
        print_error "Unknown command: $1"
        echo ""
        usage
        ;;
esac
