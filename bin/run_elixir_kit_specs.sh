#!/bin/bash
set -e

# EventoDB Elixir Kit Test Runner
# Runs the EventodbKit integration tests against EventoDB server

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

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

# Check if EventoDB is running
check_eventodb() {
    if ! curl -sf http://localhost:8080/health > /dev/null 2>&1; then
        print_error "EventoDB is not running on http://localhost:8080"
        print_status "Starting EventoDB in test mode..."
        
        # Build EventoDB if needed
        if [ ! -f "$PROJECT_ROOT/dist/eventodb" ]; then
            print_status "Building EventoDB..."
            cd "$PROJECT_ROOT"
            make build
        fi
        
        # Start EventoDB
        "$PROJECT_ROOT/dist/eventodb" --test-mode --port=8080 > /tmp/eventodb_elixir_kit_test.log 2>&1 &
        EVENTODB_PID=$!
        
        # Wait for it to be ready
        for i in {1..30}; do
            if curl -sf http://localhost:8080/health > /dev/null 2>&1; then
                print_success "EventoDB started (PID: $EVENTODB_PID)"
                break
            fi
            sleep 0.5
        done
        
        if ! curl -sf http://localhost:8080/health > /dev/null 2>&1; then
            print_error "EventoDB failed to start"
            exit 1
        fi
    else
        print_success "EventoDB is running"
    fi
}

# Check PostgreSQL
check_postgres() {
    if ! psql -U postgres -h localhost -c "SELECT 1" > /dev/null 2>&1; then
        print_warning "PostgreSQL not available - some tests may be skipped"
        return 1
    fi
    print_success "PostgreSQL is available"
    return 0
}

# Get admin token
get_admin_token() {
    # Try to get from env first
    if [ -n "$EVENTODB_ADMIN_TOKEN" ]; then
        echo "$EVENTODB_ADMIN_TOKEN"
        return
    fi
    
    # Extract from logs
    if [ -f /tmp/eventodb_elixir_kit_test.log ]; then
        grep "DEFAULT NAMESPACE TOKEN" /tmp/eventodb_elixir_kit_test.log | tail -1 | awk '{print $NF}' | sed 's/\x1b\[[0-9;]*m//g'
    fi
}

# Main execution
main() {
    print_status "EventodbKit Test Runner"
    echo ""
    
    # Check dependencies
    check_eventodb
    check_postgres
    
    # Get admin token
    ADMIN_TOKEN=$(get_admin_token)
    if [ -z "$ADMIN_TOKEN" ]; then
        print_error "Could not determine admin token"
        exit 1
    fi
    export EVENTODB_ADMIN_TOKEN="$ADMIN_TOKEN"
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
