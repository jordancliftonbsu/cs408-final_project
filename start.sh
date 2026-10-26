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
# Step 3: Run Verification Suite
# -----------------------------------------------------------------------------
say "Step 3: Running test suite & build check..."

if [ -f "test.sh" ]; then
    chmod +x test.sh
    ./test.sh
else
    say "Warning: test.sh not found, skipping automated tests."
fi

# -----------------------------------------------------------------------------
# Step 4: Setup Database Directory
# -----------------------------------------------------------------------------
say "Step 4: Preparing database directory..."

mkdir -p "$DB_DIR"

# -----------------------------------------------------------------------------
# Step 5: Start App
# -----------------------------------------------------------------------------
say "Step 5: Starting FantasyLoL..."

echo ""
echo "=================================================="
echo " FantasyLoL is running at: http://localhost:${PORT}"
echo " Press Ctrl+C to stop the server."
echo "=================================================="
echo ""

# Export environment variables & launch Go app
export PORT="$PORT"
export DB_PATH="$DB_PATH"

go run .