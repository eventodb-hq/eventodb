#!/usr/bin/env bash
set -e

echo ""
echo "=== Running QA Checks for Message DB Go ==="
echo ""

echo "1. Running go vet..."
go vet ./...

echo ""
echo "2. Running go test..."
go test ./... -v

echo ""
echo "3. Running go test with race detector..."
go test ./... -race

echo ""
echo "4. Checking go mod tidy..."
go mod tidy
git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum needs tidying" && exit 1)

echo ""
echo "✅ All QA checks passed!"
echo ""
