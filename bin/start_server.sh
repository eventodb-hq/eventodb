#!/bin/bash
set -e

# Start EventoDB server with SQLite backend (persistent mode)
# Port: 8080
# Data directory: ./data
# Database file: ./data/eventodb.db

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# Stop any server running on port 8080
echo "Checking for processes on port 8080..."
PID=$(lsof -ti:8080 || true)
if [ -n "$PID" ]; then
    echo "Stopping process $PID on port 8080..."
    kill -9 $PID
    sleep 1
fi

# Ensure data directory exists
mkdir -p ./data

# Build if binary doesn't exist
if [ ! -f "./dist/eventodb" ]; then
    echo "Building EventoDB..."
    make build
fi

echo "Starting EventoDB server..."
echo "  Backend: SQLite (persistent)"
echo "  Database: ./data/eventodb.db"
echo "  Port: 8080"
echo "  Data dir: ./data"
echo ""

# Use a fixed token for stable namespace across restarts
FIXED_TOKEN="ns_ZGVmYXVsdA_4fbfef0ff8b8f25e4e26c294a3c03500c954adba32b0bfefc43853679dd7a700"

./dist/eventodb \
    --db-url="sqlite://eventodb.db" \
    --data-dir=./data \
    --port=8080 \
    --token="$FIXED_TOKEN"
