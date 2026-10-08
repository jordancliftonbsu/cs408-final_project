#!/usr/bin/env bash
#
# deploy/deploy.sh
# Deploys FantasyLoL from a local laptop to the EC2 instance.
#
# Usage:
#   ./deploy/deploy.sh -h <PUBLIC-IP> -i ~/keys/mykey.pem

set -euo pipefail

HOST=""
KEY=""
USER="ubuntu"
APP_NAME="fantasylol"
APP_DIR="/opt/fantasylol"

usage() {
    echo "Usage: $0 -h <PUBLIC-IP-OR-DNS> -i <PATH-TO-KEY-PEM> [-u <SSH-USER>]"
    exit 1
}

while getopts "h:i:u:" opt; do
    case "$opt" in
        h) HOST="$OPTARG" ;;
        i) KEY="$OPTARG" ;;
        u) USER="$OPTARG" ;;
        *) usage ;;
    esac
done

if [ -z "$HOST" ] || [ -z "$KEY" ]; then
    usage
fi

say() {
    printf '\n==> %s\n' "$1"
}

die() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

SSH_CMD="ssh -i $KEY -o StrictHostKeyChecking=accept-new $USER@$HOST"

# 1. Run Tests Locally
say "Running local tests..."
go test ./...

# 2. Cross-Compile Go Binary for Linux AMD64
say "Compiling Go binary for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -o bin/fantasylol main.go

# 3. Stop running service & deploy files via scp
say "Stopping remote service for binary update..."
$SSH_CMD "sudo systemctl stop $APP_NAME" || true

say "Deploying files to server ($HOST)..."
$SSH_CMD "mkdir -p $APP_DIR/public"

# Copy binary
scp -i "$KEY" bin/fantasylol "$USER@$HOST:$APP_DIR/fantasylol"
$SSH_CMD "chmod +x $APP_DIR/fantasylol"

# Copy static assets
if [ -d "public" ]; then
    scp -i "$KEY" -r public/* "$USER@$HOST:$APP_DIR/public/"
fi

# 4. Start Service on EC2
say "Starting systemd service on server..."
$SSH_CMD "sudo systemctl start $APP_NAME"

# 5. Health Check Verification
say "Verifying deployment health..."
sleep 2

HEALTH_URL="http://$HOST/api/health"
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_URL" || true)

if [ "$HTTP_STATUS" -eq 200 ]; then
    say "Deployment successful! App is live at http://$HOST"
else
    say "WARNING: Health check to $HEALTH_URL returned HTTP status: $HTTP_STATUS"
    say "Checking service status on remote machine:"
    $SSH_CMD "sudo systemctl status $APP_NAME --no-pager" || true
    die "Deployment health check failed."
fi