#!/bin/bash

# Full reset script: stops server, deletes DB, starts server, creates admin user

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Get the project root (one level up from bin/)
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Full Reset Script ==="

# 1. Delete the SQL database
echo "Deleting database..."
rm -f "$PROJECT_ROOT/tmp/data/replicache.db"*

# 2. Restart server (stops existing, builds, starts new)
echo "Restarting server..."
"$SCRIPT_DIR/restart_server.sh" &

# Wait for server to be ready
echo "Waiting for server to start..."
for i in {1..30}; do
    if lsof -i :3333 | grep -q LISTEN; then
        echo "Server is running"
        break
    fi
    sleep 0.5
done

# Give server a moment to fully initialize
sleep 1

# 3. Register the first user via API
echo "Registering user 'roman'..."
RESPONSE=$(curl -s -X POST http://localhost:3333/api/auth/register \
    -H "Content-Type: application/json" \
    -H "X-Namespace-ID: default" \
    -d '{"username": "roman", "email": "roman@moo.com", "password": "password"}')

echo "Registration response: $RESPONSE"

# 4. Make this user admin via direct SQLite manipulation
echo "Making user 'roman' an admin..."
sqlite3 "$PROJECT_ROOT/tmp/data/replicache.db" "UPDATE users SET is_admin = 1 WHERE email = 'roman@moo.com';"

# Verify the user is now admin
ADMIN_STATUS=$(sqlite3 "$PROJECT_ROOT/tmp/data/replicache.db" "SELECT is_admin FROM users WHERE email = 'roman@moo.com';")
echo "Admin status for roman@moo.com: $ADMIN_STATUS"

echo "=== Full Reset Complete ==="
echo "User: roman"
echo "Email: roman@moo.com"
echo "Password: password"
echo "Admin: yes"
echo ""
echo "Server is running on http://localhost:3333"
bin/import_track.sh


## kill server
lsof -i :3333 | grep LISTEN| awk 'NR>0 {print $2}' | xargs kill -9