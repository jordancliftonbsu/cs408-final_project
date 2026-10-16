#!/usr/bin/env bash
#
# start.sh - Local development start script for FantasyLoL
#

set -euo pipefail

# Always run relative to project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

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
# Step 2: Install & Sync Dependencies
# -----------------------------------------------------------------------------
say "Step 2: Syncing project dependencies..."

if [ -f "go.mod" ]; then
    go mod download
    go mod tidy
else
    die "go.mod file not found in current directory."
fi

# -----------------------------------------------------------------------------
# Step 3: Initialize Database & Data Directories
# -----------------------------------------------------------------------------
say "Step 3: Setting up database directories..."

mkdir -p "$DB_DIR"

# Optional: Apply schema.sql if present and sqlite3 binary is installed
if [ ! -f "$DB_PATH" ] && [ -f "schema.sql" ] && command -v sqlite3 >/dev/null 2>&1; then
    say "Initializing database schema from schema.sql..."
    sqlite3 "$DB_PATH" < schema.sql
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