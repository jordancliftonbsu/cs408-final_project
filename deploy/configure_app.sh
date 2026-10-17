#!/usr/bin/env bash
set -euo pipefail

# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------
EC2_HOST="${1:-}"
TARGET_HOST="$EC2_HOST"
APP_DIR="/home/ubuntu/fantasylol"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [ -z "$EC2_HOST" ]; then
    echo "Usage: $0 <user@ec2-ip-or-dns>"
    echo "Example: $0 ubuntu@ec2-16-147-51-129.us-west-2.compute.amazonaws.com"
    exit 1
fi

# -----------------------------------------------------------------------------
# Pre-flight Check: SSH Connection & Key Validation
# -----------------------------------------------------------------------------
echo "==> Pre-flight check: Verifying SSH connection to $EC2_HOST..."

if ! ssh -q -o BatchMode=yes -o ConnectTimeout=5 "$EC2_HOST" exit; then
    echo "ERROR: Unable to connect to $EC2_HOST."
    exit 1
fi

# -----------------------------------------------------------------------------
# Step 1: Run Local Tests
# -----------------------------------------------------------------------------
echo "==> 1. Running test suite prior to deployment..."
(cd "${PROJECT_ROOT}" && CGO_ENABLED=0 go test -v -vet=off ./...)

# -----------------------------------------------------------------------------
# Step 2: Compiling Go binary for Linux
# -----------------------------------------------------------------------------
echo "==> 2. Compiling Go binary for Linux (amd64)..."
mkdir -p "${PROJECT_ROOT}/bin"
(cd "${PROJECT_ROOT}" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/fantasylol .)

# -----------------------------------------------------------------------------
# Step 3: Remote Directory Setup & Binary Upload
# -----------------------------------------------------------------------------
echo "==> 3. Ensuring remote directory structure exists on EC2..."
ssh "${TARGET_HOST}" "mkdir -p ${APP_DIR}/public ${APP_DIR}/data"

echo "==> 4. Stopping remote service..."
ssh "$EC2_HOST" "sudo systemctl stop fantasylol || true"

echo "==> 5. Uploading compiled binary and source files to EC2..."
scp bin/fantasylol "$EC2_HOST":/home/ubuntu/fantasylol/fantasylol

if [ -f "${PROJECT_ROOT}/go.mod" ]; then
    scp "${PROJECT_ROOT}/go.mod" "$EC2_HOST":/home/ubuntu/fantasylol/
fi
if [ -f "${PROJECT_ROOT}/go.sum" ]; then
    scp "${PROJECT_ROOT}/go.sum" "$EC2_HOST":/home/ubuntu/fantasylol/
fi
scp "${PROJECT_ROOT}"/*.go "$EC2_HOST":/home/ubuntu/fantasylol/ 2>/dev/null || true

if [ -d "${PROJECT_ROOT}/scoring" ]; then
    scp -r "${PROJECT_ROOT}/scoring" "$EC2_HOST":/home/ubuntu/fantasylol/
fi

# -----------------------------------------------------------------------------
# Step 4: Resolve Remote Dependencies
# -----------------------------------------------------------------------------
echo "==> 6. Synchronizing Go modules on EC2..."
ssh "${TARGET_HOST}" "
    cd ${APP_DIR}
    if [ ! -f 'go.mod' ]; then
        go mod init fantasylol
    fi
    go mod tidy
"

# -----------------------------------------------------------------------------
# Step 5: Public Assets & Service Restart
# -----------------------------------------------------------------------------
if [ -d "${PROJECT_ROOT}/public" ]; then
    echo "==> 7. Uploading static public assets to EC2..."
    scp -r "${PROJECT_ROOT}/public/." "${TARGET_HOST}:${APP_DIR}/public/"
fi

echo "==> 8. Setting executable permissions and restarting service..."
ssh "${TARGET_HOST}" "chmod +x ${APP_DIR}/fantasylol && sudo systemctl restart fantasylol && sudo systemctl status fantasylol"

echo "==> Deployment complete!"