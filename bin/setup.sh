#!/usr/bin/env bash

# Message DB Setup Script
# Creates PostgreSQL database if not already exists


# Disable CGO for consistent builds across platforms
export CGO_ENABLED=0

set -e

# Configuration
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-eventodb_store}"
DB_USER="${DB_USER:-eventodb_store}"
DB_PASSWORD="${DB_PASSWORD:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"

echo "Message DB Setup"
echo "================"
echo "Database Host: $DB_HOST:$DB_PORT"
echo "Database Name: $DB_NAME"
echo "Database User: $DB_USER"
echo

# Check if psql is available
if ! command -v psql &> /dev/null; then
    echo "Error: psql command not found. Please install PostgreSQL client."
    exit 1
fi

# Function to execute SQL as postgres superuser
exec_sql() {
    PGPASSWORD="${PGPASSWORD:-}" psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "$1" 2>/dev/null || true
}

# Check if database exists
echo "Checking if database '$DB_NAME' exists..."
if exec_sql "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1; then
    echo "Database '$DB_NAME' already exists."
else
    echo "Creating database '$DB_NAME'..."
    exec_sql "CREATE DATABASE $DB_NAME"
    echo "Database created successfully."
fi

# Check if user exists
echo "Checking if user '$DB_USER' exists..."
if exec_sql "SELECT 1 FROM pg_user WHERE usename = '$DB_USER'" | grep -q 1; then
    echo "User '$DB_USER' already exists."
else
    echo "Creating user '$DB_USER'..."
    exec_sql "CREATE USER $DB_USER WITH PASSWORD '$DB_PASSWORD'"
    echo "User created successfully."
fi

# Grant privileges
echo "Granting privileges to user '$DB_USER' on database '$DB_NAME'..."
exec_sql "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER"

echo
echo "Database setup completed successfully!"
echo
echo "Connection string:"
echo "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME"
echo
echo "The golang server will handle schema creation and migrations."
