#!/usr/bin/env bash
#
# seed-demo.sh — Populate the database with demo users and leagues for
# showing off the Leagues search page. Run it on the server AFTER deploying
# (and after the service has started at least once so the schema exists).
#
#   ./seed-demo.sh
#
# Adds 16 demo users (demo01..demo16, password "demopass1") and 23 leagues
# (public/private, full, started, concluded, upcoming) with start dates relative to today.
# Safe to re-run: existing leagues/users are matched by name and skipped.
#
# Environment overrides:
#   DB_PATH   SQLite file to seed (default: ./data/fantasylol.db next to this
#             script, or DB_PATH from ./.env if set there)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="${SCRIPT_DIR}/seed_demo_leagues.sql"

# Pick up DB_PATH from .env when not already set in the environment.
if [ -z "${DB_PATH:-}" ] && [ -f "${SCRIPT_DIR}/.env" ]; then
    DB_PATH="$(grep -E '^DB_PATH=' "${SCRIPT_DIR}/.env" | tail -n1 | cut -d= -f2- | tr -d '"'"'"'\r' || true)"
fi
DB_PATH="${DB_PATH:-${SCRIPT_DIR}/data/fantasylol.db}"

if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "ERROR: sqlite3 is not installed. Install it (e.g. sudo apt-get install -y sqlite3)." >&2
    exit 1
fi
if [ ! -f "${SQL_FILE}" ]; then
    echo "ERROR: ${SQL_FILE} not found. Re-run the deploy script to upload it." >&2
    exit 1
fi
if [ ! -f "${DB_PATH}" ]; then
    echo "ERROR: database not found at ${DB_PATH}." >&2
    echo "       Start the service once so it creates the database, or set DB_PATH." >&2
    exit 1
fi
if ! sqlite3 "${DB_PATH}" "SELECT password_hash FROM leagues LIMIT 1;" >/dev/null 2>&1; then
    echo "ERROR: ${DB_PATH} doesn't have the current leagues schema." >&2
    echo "       Deploy the latest version and restart the service first." >&2
    exit 1
fi

echo "==> Seeding demo users and leagues into ${DB_PATH}..."
sqlite3 "${DB_PATH}" < "${SQL_FILE}"

USERS=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM users WHERE username LIKE 'demo__';")
LEAGUES=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM leagues l JOIN users u ON u.user_id = l.commissioner_id WHERE u.username LIKE 'demo__';")
echo "==> Done. Demo users: ${USERS}, demo-owned leagues: ${LEAGUES}."
echo "    Demo logins: demo01..demo16 / demopass1 (also the password for private demo leagues)."
