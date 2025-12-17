#!/bin/bash
#
# Start the Go server for production
# This serves static assets and proxies API requests to the backend
#

set -e

cd "$(dirname "$0")/../kids-real-ui"

# Configuration
export PORT="${PORT:-9999}"
export BACKEND_URL="${BACKEND_URL:-http://localhost:3333}"

echo "🏗️  Building frontend..."
bun run build

echo "🔨 Building Go server..."
go build -o server server.go

echo "🛑 Stopping existing server..."
pkill -f "./server" || true
sleep 1

echo "🚀 Starting Go server..."
echo "   Port: $PORT"
echo "   Backend: $BACKEND_URL"

# Run in background and save PID
nohup ./server > server.log 2>&1 &
SERVER_PID=$!
echo $SERVER_PID > server.pid

echo "✅ Server started with PID $SERVER_PID"
echo "📝 Logs: $(pwd)/server.log"
echo "🌐 URL: http://localhost:$PORT"

# Show last few log lines
sleep 1
tail -5 server.log
