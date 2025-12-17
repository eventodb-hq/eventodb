#!/bin/bash
#
# Run external tests against MessageDB server with both SQLite and PostgreSQL backends
#
# This script runs the full external test suite against both backends to ensure
# behavioral parity between SQLite and PostgreSQL.
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║     MessageDB External Tests - All Backends                ║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Run SQLite tests
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}  Running tests with SQLite backend${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
"$SCRIPT_DIR/run_external_tests.sh"
SQLITE_EXIT=$?

echo ""
echo ""

# Run PostgreSQL tests
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}  Running tests with PostgreSQL backend${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
"$SCRIPT_DIR/run_external_tests_postgres.sh"
POSTGRES_EXIT=$?

echo ""
echo ""

# Summary
echo -e "${CYAN}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║                      Summary                               ║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════════════════════╝${NC}"

if [ $SQLITE_EXIT -eq 0 ]; then
    echo -e "  SQLite:     ${GREEN}✓ PASSED${NC}"
else
    echo -e "  SQLite:     ${RED}✗ FAILED${NC}"
fi

if [ $POSTGRES_EXIT -eq 0 ]; then
    echo -e "  PostgreSQL: ${GREEN}✓ PASSED${NC}"
else
    echo -e "  PostgreSQL: ${RED}✗ FAILED${NC}"
fi

echo ""

if [ $SQLITE_EXIT -eq 0 ] && [ $POSTGRES_EXIT -eq 0 ]; then
    echo -e "${GREEN}All tests passed on both backends!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
