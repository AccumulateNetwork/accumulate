#!/bin/bash
set -e

# Install Accumulate Torrent Server
# Can be called by accman or run standalone

INSTALL_DIR="${INSTALL_DIR:-/opt/accumulate-torrent}"
VOLUMES_FILE="${VOLUMES_FILE:-/media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz}"
TORRENT_PORT="${TORRENT_PORT:-51413}"
SYSTEMD="${SYSTEMD:-true}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== Accumulate Torrent Server Installation ===${NC}"
echo ""

# Check if running as root for system installation
if [ "$SYSTEMD" = "true" ] && [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: System installation requires root privileges${NC}"
    echo "Run with: sudo ./install-torrent-server.sh"
    echo "Or set SYSTEMD=false for user installation"
    exit 1
fi

# Create installation directory
echo -e "${YELLOW}Creating installation directory: $INSTALL_DIR${NC}"
mkdir -p "$INSTALL_DIR"/{bin,data}

# Build torrent server
echo -e "${YELLOW}Building torrent server...${NC}"
cd "$(dirname "$0")/cmd/torrent-server"
go build -o "$INSTALL_DIR/bin/torrent-server" .

echo -e "${GREEN}✓ Torrent server built${NC}"

# Create configuration file
echo -e "${YELLOW}Creating configuration...${NC}"
cat > "$INSTALL_DIR/torrent-server.conf" <<EOF
# Accumulate Torrent Server Configuration
VOLUMES_FILE=$VOLUMES_FILE
TORRENT_PORT=$TORRENT_PORT
DATA_DIR=$INSTALL_DIR/data
EOF

echo -e "${GREEN}✓ Configuration created: $INSTALL_DIR/torrent-server.conf${NC}"

# Install systemd service
if [ "$SYSTEMD" = "true" ]; then
    echo -e "${YELLOW}Installing systemd service...${NC}"

    cat > /etc/systemd/system/accumulate-torrent.service <<EOF
[Unit]
Description=Accumulate Torrent Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/torrent-server.conf
ExecStart=$INSTALL_DIR/bin/torrent-server -file \${VOLUMES_FILE} -data \${DATA_DIR} -port \${TORRENT_PORT}
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    echo -e "${GREEN}✓ Systemd service installed${NC}"
fi

# Configure firewall
if command -v ufw >/dev/null 2>&1; then
    echo -e "${YELLOW}Configuring firewall...${NC}"
    ufw allow "$TORRENT_PORT/tcp" >/dev/null 2>&1 || true
    ufw allow "$TORRENT_PORT/udp" >/dev/null 2>&1 || true
    echo -e "${GREEN}✓ Firewall configured${NC}"
fi

# Create start/stop scripts
cat > "$INSTALL_DIR/start.sh" <<'EOF'
#!/bin/bash
if systemctl is-enabled accumulate-torrent.service >/dev/null 2>&1; then
    sudo systemctl start accumulate-torrent
    echo "Torrent server started (systemd)"
    echo "View logs: sudo journalctl -u accumulate-torrent -f"
else
    source torrent-server.conf
    ./bin/torrent-server -file "$VOLUMES_FILE" -data "$DATA_DIR" -port "$TORRENT_PORT" &
    echo "Torrent server started (background)"
    echo "PID: $!"
fi
EOF

cat > "$INSTALL_DIR/stop.sh" <<'EOF'
#!/bin/bash
if systemctl is-enabled accumulate-torrent.service >/dev/null 2>&1; then
    sudo systemctl stop accumulate-torrent
    echo "Torrent server stopped (systemd)"
else
    pkill -f "torrent-server.*$VOLUMES_FILE"
    echo "Torrent server stopped"
fi
EOF

cat > "$INSTALL_DIR/status.sh" <<'EOF'
#!/bin/bash
if systemctl is-enabled accumulate-torrent.service >/dev/null 2>&1; then
    systemctl status accumulate-torrent
else
    if pgrep -f "torrent-server" >/dev/null; then
        echo "Torrent server is running"
        pgrep -af "torrent-server"
    else
        echo "Torrent server is not running"
    fi
fi
EOF

cat > "$INSTALL_DIR/magnet.sh" <<'EOF'
#!/bin/bash
source torrent-server.conf
MAGNET_FILE="$DATA_DIR/$(basename $VOLUMES_FILE).magnet"
if [ -f "$MAGNET_FILE" ]; then
    cat "$MAGNET_FILE"
else
    echo "Magnet link not yet generated. Check if server is running."
fi
EOF

chmod +x "$INSTALL_DIR"/*.sh

echo ""
echo -e "${GREEN}=== Installation Complete ===${NC}"
echo ""
echo "Installation directory: $INSTALL_DIR"
echo "Configuration: $INSTALL_DIR/torrent-server.conf"
echo ""
echo -e "${YELLOW}Next steps:${NC}"

if [ "$SYSTEMD" = "true" ]; then
    echo "  1. Edit configuration (optional):"
    echo "     sudo nano $INSTALL_DIR/torrent-server.conf"
    echo ""
    echo "  2. Enable and start service:"
    echo "     sudo systemctl enable accumulate-torrent"
    echo "     sudo systemctl start accumulate-torrent"
    echo ""
    echo "  3. Check status:"
    echo "     sudo systemctl status accumulate-torrent"
    echo "     sudo journalctl -u accumulate-torrent -f"
    echo ""
    echo "  4. Get magnetic link:"
    echo "     $INSTALL_DIR/magnet.sh"
else
    echo "  1. Edit configuration (optional):"
    echo "     nano $INSTALL_DIR/torrent-server.conf"
    echo ""
    echo "  2. Start server:"
    echo "     $INSTALL_DIR/start.sh"
    echo ""
    echo "  3. Check status:"
    echo "     $INSTALL_DIR/status.sh"
    echo ""
    echo "  4. Get magnetic link:"
    echo "     $INSTALL_DIR/magnet.sh"
    echo ""
    echo "  5. Stop server:"
    echo "     $INSTALL_DIR/stop.sh"
fi

echo ""
echo -e "${YELLOW}Firewall:${NC}"
echo "  Port $TORRENT_PORT/tcp and $TORRENT_PORT/udp should be open"
echo "  Test: nc -zv localhost $TORRENT_PORT"
echo ""
