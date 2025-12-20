#!/bin/bash
set -e

echo "Running MessageDB Go SDK tests..."
echo "Server: ${MESSAGEDB_URL:-http://localhost:8080}"
echo ""

# Run tests with verbose output and race detector
go test -v -race -timeout 60s ./...

echo ""
echo "✓ All tests passed!"
