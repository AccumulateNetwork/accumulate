#!/bin/bash
# Bootstrap Server Deployment Script
# Run this on bootstrap.accumulate.defidevs.io

set -e

echo "=== Accumulate Bootstrap Server Update ==="
echo "This script will:"
echo "  1. Stop the current bootstrap server"
echo "  2. Backup the current binary (if exists)"
echo "  3. Deploy the new binary"
echo "  4. Start with both ports (16593, 16693, 8080)"
echo ""

# Check if running as root or with sudo
if [ "$EUID" -eq 0 ]; then
    echo "Please do not run as root. Run as the accumulate user."
    exit 1
fi

# Configuration
BINARY_PATH="/usr/local/bin/accumulated-bootstrap"
KEY_PATH="/opt/accumulate/bootstrap-key.txt"
NEW_BINARY="./accumulated-bootstrap-new"

# Check if new binary exists
if [ ! -f "$NEW_BINARY" ]; then
    echo "Error: $NEW_BINARY not found"
    echo "Please upload the binary to this directory first"
    exit 1
fi

# Backup current binary
if [ -f "$BINARY_PATH" ]; then
    BACKUP_PATH="${BINARY_PATH}.backup.$(date +%Y%m%d_%H%M%S)"
    echo "Backing up current binary to $BACKUP_PATH"
    sudo cp "$BINARY_PATH" "$BACKUP_PATH"
fi

# Stop current process (try multiple methods)
echo "Stopping current bootstrap server..."
if command -v systemctl &> /dev/null; then
    sudo systemctl stop accumulate-bootstrap 2>/dev/null || true
fi
if command -v docker &> /dev/null; then
    sudo docker stop accumulate-bootstrap 2>/dev/null || true
    sudo docker rm accumulate-bootstrap 2>/dev/null || true
fi
sudo pkill -f accumulated-bootstrap || true

# Wait for process to stop
sleep 2

# Install new binary
echo "Installing new binary..."
sudo cp "$NEW_BINARY" "$BINARY_PATH"
sudo chmod +x "$BINARY_PATH"

# Verify key exists
if [ ! -f "$KEY_PATH" ]; then
    echo "Warning: Bootstrap key not found at $KEY_PATH"
    echo "Generating a new key (NOT RECOMMENDED for production)"
    sudo mkdir -p /opt/accumulate
    accumulated key gen | sudo tee "$KEY_PATH" > /dev/null
    echo "NEW KEY GENERATED - This changes the peer ID!"
fi

# Check if running in Docker or systemd
if [ -f "/etc/systemd/system/accumulate-bootstrap.service" ]; then
    echo "Starting via systemd..."
    sudo systemctl daemon-reload
    sudo systemctl start accumulate-bootstrap
    sleep 2
    sudo systemctl status accumulate-bootstrap --no-pager
elif command -v docker &> /dev/null; then
    echo "Starting via Docker..."
    sudo docker run -d \
      --name accumulate-bootstrap \
      --restart unless-stopped \
      -p 16593:16593 \
      -p 16693:16693 \
      -p 8080:8080 \
      -v "$KEY_PATH:/key.txt:ro" \
      registry.gitlab.com/accumulatenetwork/accumulate:latest \
      accumulated-bootstrap \
      --key /key.txt \
      --listen /ip4/0.0.0.0/tcp/16593 \
      --listen /ip4/0.0.0.0/tcp/16693 \
      --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
      --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693

    sleep 3
    sudo docker logs accumulate-bootstrap
else
    echo "Starting as background process..."
    nohup "$BINARY_PATH" \
      --key "$KEY_PATH" \
      --listen /ip4/0.0.0.0/tcp/16593 \
      --listen /ip4/0.0.0.0/tcp/16693 \
      --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
      --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
      > /var/log/bootstrap.log 2>&1 &

    echo "Process started. Logs: /var/log/bootstrap.log"
fi

echo ""
echo "=== Verification ==="
sleep 3

# Check ports
echo "Checking ports..."
nc -zv localhost 16593 2>&1 | grep -q "succeeded" && echo "✓ Port 16593 (DN) is open" || echo "✗ Port 16593 (DN) failed"
nc -zv localhost 16693 2>&1 | grep -q "succeeded" && echo "✓ Port 16693 (BVN) is open" || echo "✗ Port 16693 (BVN) failed"
nc -zv localhost 8080 2>&1 | grep -q "succeeded" && echo "✓ Port 8080 (Info) is open" || echo "✗ Port 8080 (Info) failed"

echo ""
echo "Checking info endpoint..."
curl -s http://localhost:8080/info | jq -r '.peer_id' && echo "✓ Info server responding" || echo "✗ Info server not responding"

echo ""
echo "Checking health endpoint..."
curl -s http://localhost:8080/health | jq -r '.status' && echo "✓ Health check responding" || echo "✗ Health check not responding"

echo ""
echo "=== Deployment Complete ==="
echo "To monitor logs:"
if [ -f "/etc/systemd/system/accumulate-bootstrap.service" ]; then
    echo "  sudo journalctl -u accumulate-bootstrap -f"
elif command -v docker &> /dev/null; then
    echo "  sudo docker logs -f accumulate-bootstrap"
else
    echo "  tail -f /var/log/bootstrap.log"
fi

echo ""
echo "External verification:"
echo "  curl http://bootstrap.accumulate.defidevs.io:8080/info | jq"
echo "  curl http://bootstrap.accumulate.defidevs.io:8080/health | jq"
