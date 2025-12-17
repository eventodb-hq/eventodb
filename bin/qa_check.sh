#!/usr/bin/env bash
set -e

echo ""
echo "=== Running QA Checks ==="
echo ""

echo "1. Running go test..."
(cd golang && go test ./... -v)


echo "RUNNING GOLANG WITH POSTGRES..."
(cd golang && DB_DRIVER=postgres go test ./... -v)

echo ""
echo "2. Running bun tests..."
(cd kids-real-ui && bun test)

echo ""
echo "3. Typescript check..."
(cd kids-real-ui && bun run tsc --noEmit)

echo ""
echo "✅ All QA checks passed!"
echo ""
