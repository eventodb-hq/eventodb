#!/bin/bash
set -e

# EventoDB Elixir Kit Test Runner
# Runs the EventodbKit integration tests against EventoDB server

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Test server configuration
TEST_PORT=8080
TEST_BACKEND="sqlite"
ADMIN_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print with color
print_status() {
    echo -e "${BLUE}==>${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}!${NC} $1"
}

# Cleanup on exit
cleanup() {
    if [ "${STARTED_SERVER}" = "1" ]; then
        print_status "Stopping test server..."
        "$SCRIPT_DIR/manage_test_server.sh" stop "$TEST_PORT" || true
    fi
}
trap cleanup EXIT

# Main execution
main() {
    print_status "EventodbKit Test Runner"
    echo ""
    
    # Check if server is already running
    if "$SCRIPT_DIR/manage_test_server.sh" status "$TEST_PORT" > /dev/null 2>&1; then
        print_success "EventoDB is already running on port $TEST_PORT"
        STARTED_SERVER=0
    else
        print_status "Starting EventoDB test server..."
        if EVENTODB_TEST_MODE=1 "$SCRIPT_DIR/manage_test_server.sh" start "$TEST_BACKEND" "$TEST_PORT"; then
            STARTED_SERVER=1
        else
            print_error "Failed to start test server"
            exit 1
        fi
    fi
    
    # Check PostgreSQL availability
    if psql -U postgres -h localhost -c "SELECT 1" > /dev/null 2>&1; then
        print_success "PostgreSQL is available"
    else
        print_warning "PostgreSQL not available - some tests may be skipped"
    fi
    
    # Export admin token
    export EVENTODB_ADMIN_TOKEN="$ADMIN_TOKEN"
    export EVENTODB_URL="http://localhost:$TEST_PORT"
    print_success "Admin token configured"
    
    echo ""
    print_status "Running EventodbKit tests..."
    echo ""
    
    # Run tests
    cd "$PROJECT_ROOT/clients/eventodb_kit"
    
    if mix test "$@"; then
        echo ""
        print_success "All tests passed!"
        exit 0
    else
        echo ""
        print_error "Tests failed"
        exit 1
    fi
}

main "$@"
