#!/bin/bash

# Disable CGO for consistent builds across platforms
export CGO_ENABLED=0

set -e

echo "Running EventoDB Go SDK tests..."
echo "Server: ${EVENTODB_URL:-http://localhost:8080}"
echo ""

# Run tests with verbose output and race detector
go test -v -race -timeout 60s ./...

echo ""
echo "✓ All tests passed!"
