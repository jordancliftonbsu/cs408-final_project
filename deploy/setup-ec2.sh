#!/usr/bin/env bash
#
# deploy/setup-ec2.sh
# One-time setup script for a fresh EC2 instance running FantasyLoL.
#
# Usage (run once on the server as root or via sudo):
#   sudo bash deploy/setup-ec2.sh

set -euo pipefail

APP_NAME="fantasylol"
APP_DIR="/home/ubuntu/fantasylol"

# Resolve absolute path to script directory regardless of where script is invoked from
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_USER="${SUDO_USER:-ubuntu}"

say() {
    printf '\n==> %s\n' "$1"
}

die() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

# --- Checks ------------------------------------------------------------------

[ "$(id -u)" -eq 0 ] || die "Run this script with sudo: sudo bash $0"
[ -r /etc/os-release ] || die "Cannot read /etc/os-release; unsupported OS."
. /etc/os-release

case "${ID:-}" in
    ubuntu|debian)
        PKG="apt"
        export DEBIAN_FRONTEND=noninteractive
        ;;
    amzn|rhel|centos|fedora)
        PKG="dnf"
        ;;
    *)
        die "Unsupported distribution '${ID:-unknown}'"
        ;;
esac

say "Detected ${PRETTY_NAME:-$ID} (Package manager: $PKG)"

# --- Packages ----------------------------------------------------------------

say "Installing nginx, sqlite3, and rsync"

if [ "$PKG" = "apt" ]; then
    apt-get update -y
    apt-get install -y ca-certificates curl rsync nginx sqlite3
else
    dnf -y update
    dnf -y install nginx sqlite rsync
fi

# --- Directories & Permissions -----------------------------------------------

say "Creating application directory structure at $APP_DIR"

mkdir -p "$APP_DIR/public" "$APP_DIR/data"

# Ensure directory traversal for NGINX and ownership for ubuntu user
chmod 755 /home/ubuntu
chmod 755 "$APP_DIR"
chmod 755 "$APP_DIR/public"
chmod 775 "$APP_DIR/data"
chown -R "$DEPLOY_USER:$DEPLOY_USER" "$APP_DIR"

# --- Environment File -------------------------------------------------------

say "Initializing .env configuration file"

if [ ! -f "$APP_DIR/.env" ]; then
    cat <<'EOF' > "$APP_DIR/.env"
# Server Configuration
PORT=8080
DB_PATH=/home/ubuntu/fantasylol/data/fantasylol.db
INGEST_REMOTE=false

# Fandom / Leaguepedia API Bot Credentials
# REQUIRED for live ingestion (./ingest.sh). Fill these in with a Fandom
# bot account (Special:BotPasswords). Without them /admin/stats/update
# returns HTTP 500 with "FANDOM_USERNAME or FANDOM_BOT_PASS ... missing".
FANDOM_USERNAME=
FANDOM_BOT_PASS=
EOF
fi

# Always ensure correct user permissions on .env
chown "$DEPLOY_USER:$DEPLOY_USER" "$APP_DIR/.env"
chmod 640 "$APP_DIR/.env"

# --- systemd Service --------------------------------------------------------

say "Configuring systemd service"

if [ -f "$SCRIPT_DIR/fantasylol.service" ]; then
    # Overwrite target to ensure systemd picks up latest repository changes
    cp "$SCRIPT_DIR/fantasylol.service" "/etc/systemd/system/${APP_NAME}.service"
    chmod 644 "/etc/systemd/system/${APP_NAME}.service"
    systemctl daemon-reload
    systemctl enable "$APP_NAME"
else
    die "Service definition file $SCRIPT_DIR/fantasylol.service not found."
fi

# --- Nginx Configuration ----------------------------------------------------

say "Configuring Nginx reverse proxy"

if [ -f "$SCRIPT_DIR/nginx-fantasylol.conf" ]; then
    cp "$SCRIPT_DIR/nginx-fantasylol.conf" "/etc/nginx/conf.d/${APP_NAME}.conf"
    chmod 644 "/etc/nginx/conf.d/${APP_NAME}.conf"
else
    die "Nginx config file $SCRIPT_DIR/nginx-fantasylol.conf not found."
fi

# Remove Ubuntu default site to prevent port 80 collisions
if [ -L /etc/nginx/sites-enabled/default ] || [ -f /etc/nginx/sites-enabled/default ]; then
    rm -f /etc/nginx/sites-enabled/default
fi

# SELinux support
if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce)" != "Disabled" ]; then
    setsebool -P httpd_can_network_connect 1 || true
fi

nginx -t
systemctl enable nginx
systemctl restart nginx

# --- Firewall Check ---------------------------------------------------------

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
    say "Opening ports 80 and 22 in ufw"
    ufw allow 80/tcp
    ufw allow 22/tcp
fi

# --- Complete ---------------------------------------------------------------

cat <<EOF

==> EC2 Setup complete for $APP_NAME!

Installed files & structure:
  - App root:       $APP_DIR
  - Static files:   $APP_DIR/public
  - SQLite data:    $APP_DIR/data
  - Env file:       $APP_DIR/.env
  - Systemd config: /etc/systemd/system/${APP_NAME}.service
  - Nginx config:   /etc/nginx/conf.d/${APP_NAME}.conf

IMPORTANT: Live ingestion (./ingest.sh) needs Fandom bot credentials.
Edit $APP_DIR/.env and set FANDOM_USERNAME and FANDOM_BOT_PASS, then run:
    sudo systemctl restart ${APP_NAME}
Without them, /admin/stats/update returns HTTP 500.

Next step: Deploy your code using deploy/configure_app.sh from your local machine.

EOF