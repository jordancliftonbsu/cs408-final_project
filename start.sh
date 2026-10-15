#!/usr/bin/env bash
#
# start.sh - Local development start script for FantasyLoL
#

set -euo pipefail

# Configuration
PORT="${PORT:-8080}"
DB_DIR="./data"
DB_PATH="${DB_DIR}/fantasylol.db"

say() {
    printf '\n==> %s\n' "$1"
}

die() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

# -----------------------------------------------------------------------------
# Step 1: Check Prerequisites
# -----------------------------------------------------------------------------
say "Step 1: Checking required tools..."

if ! command -v go >/dev/null 2>&1; then
    die "Go (Golang) is not installed. Please install Go (v1.20+) before running this script."
fi

say "Go compiler detected: $(go version)"

# -----------------------------------------------------------------------------
# Step 2: Install Dependencies
# -----------------------------------------------------------------------------
say "Step 2: Installing project dependencies..."

if [ -f "go.mod" ]; then
    go mod download
else
    die "go.mod file not found in current directory."
fi

# -----------------------------------------------------------------------------
# Step 3: Initialize Database & Data Directories
# -----------------------------------------------------------------------------
say "Step 3: Setting up database and directories..."

mkdir -p "$DB_DIR"

if [ ! -f "$DB_PATH" ]; then
    say "Creating SQLite database at $DB_PATH..."
    touch "$DB_PATH"
    
    # If you have a schema file (e.g., schema.sql), you can seed it here:
    if [ -f "schema.sql" ] && command -v sqlite3 >/dev/null 2>&1; then
        sqlite3 "$DB_PATH" < schema.sql
        echo "Database schema initialized from schema.sql."
    fi
fi

# -----------------------------------------------------------------------------
# Step 4: Start App
# -----------------------------------------------------------------------------
say "Step 4: Starting FantasyLoL..."

echo ""
echo "=================================================="
echo " FantasyLoL is running at: http://localhost:${PORT}"
echo " Press Ctrl+C to stop the server."
echo "=================================================="
echo ""

# Run the Go server
export PORT="$PORT"
export DB_PATH="$DB_PATH"

go run main.go