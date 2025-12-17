#!/bin/bash
# Run integration tests against both SQLite and PostgreSQL backends

set -e

echo "========================================"
echo "Running Go tests with SQLite backend"
echo "========================================"
TEST_BACKEND=sqlite go test ./test_integration/... ./internal/store/integration/... -count=1 "$@"

echo ""
echo "========================================"
echo "Running Go tests with PostgreSQL backend"
echo "========================================"
echo "Using PostgreSQL connection:"
echo "  Host: ${POSTGRES_HOST:-localhost}"
echo "  Port: ${POSTGRES_PORT:-5432}"
echo "  User: ${POSTGRES_USER:-postgres}"
echo "  DB:   ${POSTGRES_DB:-postgres}"
echo ""
TEST_BACKEND=postgres go test ./test_integration/... ./internal/store/integration/... -count=1 "$@"

echo ""
echo "========================================"
echo "All tests passed for both backends!"
echo "========================================"
