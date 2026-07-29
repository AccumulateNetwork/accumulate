#!/bin/bash
# Example accman integration for torrent server deployment
# This script shows how accman would call the torrent server installer

set -e

# accman would set these based on node configuration
ACCMAN_NODE_NAME="${ACCMAN_NODE_NAME:-node1}"
ACCMAN_VOLUMES_PATH="${ACCMAN_VOLUMES_PATH:-/var/lib/accumulate/volumes}"
ACCMAN_INSTALL_DIR="${ACCMAN_INSTALL_DIR:-/opt/accumulate-torrent}"

echo "=== accman Torrent Server Deployment ==="
echo "Node: $ACCMAN_NODE_NAME"
echo "Volumes path: $ACCMAN_VOLUMES_PATH"
echo ""

# Function: Install torrent server
install_torrent_server() {
    local volumes_file="$1"
    local install_dir="$2"
    local port="${3:-51413}"

    echo "Installing torrent server..."

    VOLUMES_FILE="$volumes_file" \
    INSTALL_DIR="$install_dir" \
    TORRENT_PORT="$port" \
    SYSTEMD=true \
    "$(dirname "$0")/install-torrent-server.sh"
}

# Function: Start torrent server
start_torrent_server() {
    local install_dir="$1"

    echo "Starting torrent server..."
    systemctl start accumulate-torrent

    # Wait for magnetic link generation
    echo "Waiting for magnetic link..."
    local magnet_file="$install_dir/data/*.magnet"
    for i in {1..30}; do
        if ls $magnet_file 2>/dev/null | grep -q .; then
            echo "Magnetic link generated!"
            return 0
        fi
        sleep 1
    done

    echo "Warning: Magnetic link not generated yet"
    return 1
}

# Function: Get magnetic link
get_magnet_link() {
    local install_dir="$1"
    "$install_dir/magnet.sh"
}

# Function: Check if volumes archive exists
check_volumes() {
    local volumes_path="$1"

    # Look for .tar.gz files in volumes directory
    local volumes_file
    volumes_file=$(find "$volumes_path" -name "*.tar.gz" -type f | head -1)

    if [ -z "$volumes_file" ]; then
        echo "Error: No volumes .tar.gz found in $volumes_path"
        return 1
    fi

    echo "$volumes_file"
}

# Function: Full deployment
deploy() {
    echo "Checking for volumes archive..."
    local volumes_file
    volumes_file=$(check_volumes "$ACCMAN_VOLUMES_PATH")

    if [ $? -ne 0 ]; then
        echo "Error: Cannot deploy without volumes archive"
        exit 1
    fi

    echo "Found volumes: $volumes_file"
    echo ""

    # Install
    install_torrent_server "$volumes_file" "$ACCMAN_INSTALL_DIR"

    # Start
    start_torrent_server "$ACCMAN_INSTALL_DIR"

    # Get and display magnetic link
    echo ""
    echo "=== MAGNETIC LINK ==="
    get_magnet_link "$ACCMAN_INSTALL_DIR"
    echo "===================="
    echo ""

    echo "Torrent server deployed successfully!"
    echo ""
    echo "Distribute this magnetic link to other nodes to join the swarm."
}

# Main
case "${1:-deploy}" in
    deploy)
        deploy
        ;;
    start)
        start_torrent_server "$ACCMAN_INSTALL_DIR"
        ;;
    stop)
        systemctl stop accumulate-torrent
        echo "Torrent server stopped"
        ;;
    status)
        systemctl status accumulate-torrent
        ;;
    magnet)
        get_magnet_link "$ACCMAN_INSTALL_DIR"
        ;;
    *)
        echo "Usage: $0 {deploy|start|stop|status|magnet}"
        exit 1
        ;;
esac
