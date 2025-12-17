#!/usr/bin/env bash
set -e

echo ""
echo "=== Running QA Checks for MDB001 ==="
echo ""

cd golang

echo "1. Running go test (all packages)..."
go test ./internal/store/... -v

echo ""
echo "2. Running go vet..."
go vet ./internal/store/...

echo ""
echo "3. Running golangci-lint (if available)..."
if command -v golangci-lint &> /dev/null; then
    golangci-lint run ./internal/store/...
else
    echo "⚠️  golangci-lint not found, skipping"
fi

echo ""
echo "4. Running benchmarks..."
go test -bench=. -benchtime=100x ./internal/store/ -run=^$

echo ""
echo "✅ All QA checks passed!"
echo ""
