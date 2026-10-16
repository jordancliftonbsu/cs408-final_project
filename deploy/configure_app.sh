#!/usr/bin/env bash
set -euo pipefail

# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------
EC2_HOST="${1:-}"
TARGET_HOST="$EC2_HOST"
APP_DIR="/home/ubuntu/fantasylol"

# Script and Project Root relative paths
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
    echo ""
    echo "ERROR: Unable to connect to $EC2_HOST."
    echo "---------------------------------------------------------------------"
    echo "Tip: Ensure your AWS .pem key is placed in your local ~/.ssh/ directory:"
    echo "  1. Move key:     mv ~/Downloads/your-key.pem ~/.ssh/"
    echo "  2. Fix permissions: chmod 400 ~/.ssh/your-key.pem"
    echo "---------------------------------------------------------------------"
    exit 1
fi

# -----------------------------------------------------------------------------
# Step 1: Compiling Go binary for Linux
# -----------------------------------------------------------------------------
echo "==> 1. Compiling Go binary for Linux (amd64)..."
mkdir -p "${PROJECT_ROOT}/bin"
(cd "${PROJECT_ROOT}" && GOOS=linux GOARCH=amd64 go build -o bin/fantasylol .)

echo "==> 2. Ensuring remote directory structure exists on EC2..."
ssh "${TARGET_HOST}" "mkdir -p ${APP_DIR}/public ${APP_DIR}/data"

# Inside configure_app.sh:
echo "==> 3. Stopping remote service before upload..."
ssh "$EC2_HOST" "sudo systemctl stop fantasylol || true"

echo "==> 4. Uploading binary to EC2..."
scp bin/fantasylol "$EC2_HOST":/home/ubuntu/fantasylol/fantasylol

echo "==> 5. Restarting remote service..."
ssh "$EC2_HOST" "sudo systemctl restart fantasylol"

if [ -d "${PROJECT_ROOT}/public" ]; then
    echo "==> 6. Uploading static public assets to EC2..."
    scp -r "${PROJECT_ROOT}/public/." "${TARGET_HOST}:${APP_DIR}/public/"
fi

echo "==> 7. Setting executable permissions and restarting service..."
ssh "${TARGET_HOST}" "chmod +x ${APP_DIR}/fantasylol && sudo systemctl restart fantasylol && sudo systemctl status fantasylol"

echo "==> Deployment complete!"