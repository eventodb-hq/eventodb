#!/bin/bash
# Run Go SDK spec tests against one or all backends
# Usage: 
#   bin/run_golang_sdk_specs.sh [backend] [test_pattern]
#
# Examples:
#   bin/run_golang_sdk_specs.sh              # All tests, all backends
#   bin/run_golang_sdk_specs.sh sqlite       # All tests, SQLite only
#   bin/run_golang_sdk_specs.sh all TestSSE  # SSE tests, all backends
#   bin/run_golang_sdk_specs.sh pebble WRITE # Write tests, Pebble only

set -e

# Parse arguments
BACKEND="${1:-all}"
TEST_PATTERN="${2:-}"

# Available backends
BACKENDS=("sqlite" "postgres" "pebble")

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Backend icons
ICON_SQLITE="📦"
ICON_POSTGRES="🐘"
ICON_PEBBLE="🪨"

# Get icon for backend
get_icon() {
    case "$1" in
        sqlite) echo "$ICON_SQLITE" ;;
        postgres) echo "$ICON_POSTGRES" ;;
        pebble) echo "$ICON_PEBBLE" ;;
        *) echo "  " ;;
    esac
}

# Run tests for a specific backend
run_backend_tests() {
    local backend=$1
    local icon=$(get_icon "$backend")
    
    local backend_upper=$(echo "$backend" | tr '[:lower:]' '[:upper:]')
    
    echo ""
    echo -e "${BLUE}=========================================${NC}"
    echo -e "${BLUE}$icon Testing $backend_upper backend${NC}"
    echo -e "${BLUE}=========================================${NC}"
    
    # Build test command
    local test_cmd="CGO_ENABLED=0 TEST_BACKEND=$backend go test"
    
    # Add test pattern if specified
    if [ -n "$TEST_PATTERN" ]; then
        test_cmd="$test_cmd -run $TEST_PATTERN"
    fi
    
    # Add package path
    test_cmd="$test_cmd ./test_integration/"
    
    # Check if Postgres is available
    if [ "$backend" = "postgres" ]; then
        if ! command -v pg_isready &> /dev/null || ! pg_isready -h localhost -p 5432 &> /dev/null 2>&1; then
            echo -e "${YELLOW}⚠️  PostgreSQL not available, skipping${NC}"
            return 2
        fi
    fi
    
    # Run tests
    cd golang
    if eval $test_cmd; then
        echo -e "${GREEN}✅ $backend PASSED${NC}"
        cd ..
        return 0
    else
        echo -e "${RED}❌ $backend FAILED${NC}"
        cd ..
        return 1
    fi
}

# Main execution
main() {
    echo -e "${BLUE}=========================================${NC}"
    echo -e "${BLUE}Go SDK Spec Tests${NC}"
    echo -e "${BLUE}=========================================${NC}"
    echo "Backend: $BACKEND"
    if [ -n "$TEST_PATTERN" ]; then
        echo "Pattern: $TEST_PATTERN"
    else
        echo "Pattern: all tests"
    fi
    echo -e "${BLUE}=========================================${NC}"
    
    # Determine which backends to test
    local backends_to_test=()
    if [ "$BACKEND" = "all" ]; then
        backends_to_test=("${BACKENDS[@]}")
    else
        # Validate backend
        local valid=false
        for b in "${BACKENDS[@]}"; do
            if [ "$b" = "$BACKEND" ]; then
                valid=true
                break
            fi
        done
        
        if [ "$valid" = false ]; then
            echo -e "${RED}Error: Invalid backend '$BACKEND'${NC}"
            echo "Valid backends: ${BACKENDS[*]} all"
            exit 1
        fi
        
        backends_to_test=("$BACKEND")
    fi
    
    # Track results
    local results=()
    local failed=false
    
    # Run tests for each backend
    for backend in "${backends_to_test[@]}"; do
        if run_backend_tests "$backend"; then
            results+=("$backend:PASS")
        else
            local code=$?
            if [ $code -eq 2 ]; then
                results+=("$backend:SKIP")
            else
                results+=("$backend:FAIL")
                failed=true
            fi
        fi
    done
    
    # Print summary
    echo ""
    echo -e "${BLUE}=========================================${NC}"
    echo -e "${BLUE}Summary${NC}"
    echo -e "${BLUE}=========================================${NC}"
    
    for result in "${results[@]}"; do
        local backend="${result%%:*}"
        local status="${result##*:}"
        local icon=$(get_icon "$backend")
        local backend_display=$(printf "%-8s" "$backend")
        
        case "$status" in
            PASS)
                echo -e "$icon $backend_display: ${GREEN}✅ PASS${NC}"
                ;;
            FAIL)
                echo -e "$icon $backend_display: ${RED}❌ FAIL${NC}"
                ;;
            SKIP)
                echo -e "$icon $backend_display: ${YELLOW}⚠️  SKIP${NC}"
                ;;
        esac
    done
    
    echo -e "${BLUE}=========================================${NC}"
    
    if [ "$failed" = true ]; then
        echo -e "${RED}❌ Some tests failed${NC}"
        exit 1
    else
        echo -e "${GREEN}✅ All tests passed!${NC}"
        exit 0
    fi
}

# Show help
if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    echo "Usage: $0 [backend] [test_pattern]"
    echo ""
    echo "Arguments:"
    echo "  backend        Backend to test: sqlite|postgres|pebble|all (default: all)"
    echo "  test_pattern   Test pattern to run (optional, e.g., TestSSE, WRITE)"
    echo ""
    echo "Examples:"
    echo "  $0                    # All tests, all backends"
    echo "  $0 sqlite             # All tests, SQLite only"
    echo "  $0 all TestSSE        # SSE tests, all backends"
    echo "  $0 pebble WRITE       # Write tests, Pebble only"
    echo ""
    echo "Environment variables:"
    echo "  POSTGRES_HOST         PostgreSQL host (default: localhost)"
    echo "  POSTGRES_PORT         PostgreSQL port (default: 5432)"
    echo "  POSTGRES_USER         PostgreSQL user (default: postgres)"
    echo "  POSTGRES_PASSWORD     PostgreSQL password (default: postgres)"
    echo "  POSTGRES_DB           PostgreSQL database (default: postgres)"
    exit 0
fi

# Run main
main
