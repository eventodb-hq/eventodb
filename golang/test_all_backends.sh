#!/bin/bash
# Test runner for all backends
# Usage: ./test_all_backends.sh [test_pattern]

# Don't exit on error immediately - we want to test all backends
set +e

TEST_PATTERN="${1:-./test_integration/}"
TEST_FLAGS=""

# Parse test pattern to extract flags
if [[ "$TEST_PATTERN" == -* ]]; then
    TEST_FLAGS="$TEST_PATTERN"
    TEST_PATTERN="./test_integration/"
fi

echo "========================================="
echo "Running tests against all backends"
echo "Test pattern: $TEST_PATTERN"
echo "========================================="

# SQLite
echo ""
echo "📦 Testing SQLite backend..."
echo "-----------------------------------------"
CGO_ENABLED=0 TEST_BACKEND=sqlite go test -v $TEST_FLAGS $TEST_PATTERN
SQLITE_RESULT=$?

# Postgres (if available)
echo ""
echo "🐘 Testing Postgres backend..."
echo "-----------------------------------------"
if command -v pg_isready &> /dev/null && pg_isready -h localhost -p 5432 &> /dev/null; then
    CGO_ENABLED=0 TEST_BACKEND=postgres go test -v $TEST_FLAGS $TEST_PATTERN
    POSTGRES_RESULT=$?
else
    echo "⚠️  Postgres not available, skipping"
    POSTGRES_RESULT=0
fi

# Pebble
echo ""
echo "🪨 Testing Pebble backend..."
echo "-----------------------------------------"
CGO_ENABLED=0 TEST_BACKEND=pebble go test -v $TEST_FLAGS $TEST_PATTERN
PEBBLE_RESULT=$?

# Summary
echo ""
echo "========================================="
echo "Summary"
echo "========================================="
echo "SQLite:   $([ $SQLITE_RESULT -eq 0 ] && echo '✅ PASS' || echo '❌ FAIL')"
echo "Postgres: $([ $POSTGRES_RESULT -eq 0 ] && echo '✅ PASS' || echo '⚠️  SKIP')"
echo "Pebble:   $([ $PEBBLE_RESULT -eq 0 ] && echo '✅ PASS' || echo '❌ FAIL')"
echo "========================================="

# Exit with error if any backend failed
if [ $SQLITE_RESULT -ne 0 ] || [ $POSTGRES_RESULT -ne 0 ] || [ $PEBBLE_RESULT -ne 0 ]; then
    exit 1
fi

echo "✅ All backends passed!"
exit 0
