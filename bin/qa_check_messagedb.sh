#!/usr/bin/env bash
set -e

echo ""
echo "=== Running QA Checks for Message DB ==="
echo ""

cd golang

echo "1. Running go fmt..."
gofmt -l -w .

echo ""
echo "2. Running go vet..."
go vet ./...

echo ""
echo "3. Running all tests..."
go test ./... -v -timeout 30s

echo ""
echo "4. Checking for compilation errors..."
go build ./cmd/messagedb

echo ""
echo "✅ All QA checks passed!"
echo ""
