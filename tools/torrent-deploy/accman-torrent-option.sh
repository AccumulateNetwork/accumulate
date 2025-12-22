#!/bin/bash
# accman torrent installation option
# This script is what accman would call when user selects "Install with Torrent"

set -e

MAGNET_LINK="${MAGNET_LINK:-}"
DOWNLOAD_DIR="${DOWNLOAD_DIR:-/var/lib/accumulate/downloads}"
EXTRACT_DIR="${EXTRACT_DIR:-/var/lib/accumulate/volumes}"
TORRENT_DATA="${TORRENT_DATA:-/var/lib/accumulate/torrent-data}"
TORRENT_PORT="${TORRENT_PORT:-51413}"

echo "=== Accumulate Installation via Torrent ==="
echo ""

# Step 1: Get magnetic link (from accman config or bootstrap server)
if [ -z "$MAGNET_LINK" ]; then
    echo "Error: MAGNET_LINK not provided"
    echo "accman should provide this from:"
    echo "  - Configuration file"
    echo "  - Bootstrap server"
    echo "  - Command line argument"
    exit 1
fi

echo "Magnetic link: ${MAGNET_LINK:0:60}..."
echo "Download to: $DOWNLOAD_DIR"
echo "Extract to: $EXTRACT_DIR"
echo ""

# Step 2: Build torrent client (accman would do this once during install)
echo "Building torrent client..."
cd "$(dirname "$0")/cmd/torrent-client"
go build -o /tmp/accumulate-torrent-client .
TORRENT_CLIENT="/tmp/accumulate-torrent-client"

# Step 3: Check if file already exists
EXPECTED_FILE="$DOWNLOAD_DIR/BVN0_CLEAN_20241011.tar.gz"  # accman would know the filename
if [ -f "$EXPECTED_FILE" ]; then
    echo "✓ Volumes file already exists: $EXPECTED_FILE"
    echo "  Skipping download"
else
    # Step 4: Download via torrent
    echo "Downloading volumes via torrent..."
    echo "This may take a while depending on network and peer availability"
    echo ""

    # Run torrent client (download + seed)
    $TORRENT_CLIENT \
        -magnet "$MAGNET_LINK" \
        -download "$DOWNLOAD_DIR" \
        -data "$TORRENT_DATA" \
        -port "$TORRENT_PORT" \
        -seed-after=false  # We'll start seeding service separately

    echo ""
    echo "✓ Download complete!"
    EXPECTED_FILE=$(find "$DOWNLOAD_DIR" -name "*.tar.gz" | head -1)
fi

# Step 5: Extract volumes
echo ""
echo "Extracting volumes..."
mkdir -p "$EXTRACT_DIR"
tar -xzf "$EXPECTED_FILE" -C "$EXTRACT_DIR"
echo "✓ Volumes extracted to: $EXTRACT_DIR"

# Step 6: Install and start seeding service
echo ""
echo "Installing torrent seeding service..."
echo "This helps other nodes download faster"

cat > /tmp/torrent-seed.service <<EOF
[Unit]
Description=Accumulate Torrent Seeder
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$TORRENT_CLIENT -magnet "$MAGNET_LINK" -download "$DOWNLOAD_DIR" -data "$TORRENT_DATA" -port "$TORRENT_PORT" -seed-after=true
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
EOF

sudo mv /tmp/torrent-seed.service /etc/systemd/system/accumulate-torrent-seed.service
sudo systemctl daemon-reload
sudo systemctl enable accumulate-torrent-seed
sudo systemctl start accumulate-torrent-seed

echo "✓ Torrent seeding service started"

# Step 7: Configure firewall
if command -v ufw >/dev/null 2>&1; then
    sudo ufw allow "$TORRENT_PORT/tcp" >/dev/null 2>&1 || true
    sudo ufw allow "$TORRENT_PORT/udp" >/dev/null 2>&1 || true
    echo "✓ Firewall configured (port $TORRENT_PORT)"
fi

# Step 8: Install Accumulate
echo ""
echo "Installing Accumulate with downloaded volumes..."
# accman would continue with normal Accumulate installation here
# using volumes from $EXTRACT_DIR

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Volumes location: $EXTRACT_DIR"
echo "Torrent seeding: Active (helping other nodes)"
echo "Seeding status: sudo systemctl status accumulate-torrent-seed"
echo ""
echo "You are now helping the network by seeding the volumes file!"
