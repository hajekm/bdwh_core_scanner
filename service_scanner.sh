#!/usr/bin/env bash
set -e

REPO_URL="https://github.com/hajekm/bdwh_core_scanner.git"
INSTALL_DIR="/opt/bdwh_core_scanner"
BIN_PATH="/usr/local/bin/bdwh_core_scanner"
SERVICE_NAME="bdwh_core_scanner.service"
USER="mhajek"

echo "=== Cloning repository ==="
sudo rm -rf "$INSTALL_DIR"
sudo git clone "$REPO_URL" "$INSTALL_DIR"

echo "=== Building Go project ==="
cd "$INSTALL_DIR"
go mod tidy
CGO_ENABLED=1 GOARCH=arm64 go build -o "$BIN_PATH" .

echo "=== Setting executable permissions ==="
sudo chmod +x "$BIN_PATH"

echo "=== Creating systemd service ==="
SERVICE_FILE="/etc/systemd/system/$SERVICE_NAME"

sudo bash -c "cat > $SERVICE_FILE" <<EOF
[Unit]
Description=BDWH Core Scanner Service
After=network.target

[Service]
Type=simple
User=$USER
Group=input
ExecStart=$BIN_PATH
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=10s

# Give access to /dev/hidraw devices
AmbientCapabilities=CAP_SYS_RAWIO

[Install]
WantedBy=multi-user.target
EOF

echo "=== Reloading systemd and enabling service ==="
sudo systemctl daemon-reexec
sudo systemctl daemon-reload
sudo systemctl enable "$SERVICE_NAME"
sudo systemctl restart "$SERVICE_NAME"

echo "=== Service status ==="
sudo systemctl --no-pager --full status "$SERVICE_NAME"