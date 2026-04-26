#!/bin/bash
set -e

# Complete deployment script for Accumulate follower via torrent

MAGNET_LINK="${1}"
WORK_DIR="${2:-./follower-deployment}"

if [ -z "$MAGNET_LINK" ]; then
    echo "Usage: $0 <magnet-link> [work-dir]"
    echo ""
    echo "Example:"
    echo "  $0 'magnet:?xt=urn:btih:...' /var/lib/accumulate-follower"
    exit 1
fi

echo "=== Accumulate Follower Deployment via Torrent ==="
echo ""

# Create work directory
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

# Step 1: Build bootstrap image
echo "Step 1: Building bootstrap image..."
docker build -f ../../Dockerfile.bootstrap -t accumulate-bootstrap:latest ../..

# Step 2: Download and extract via torrent
echo "Step 2: Downloading and extracting database..."
docker run --rm \
    -v "$(pwd)/bvn-data:/output" \
    accumulate-bootstrap:latest \
    "$MAGNET_LINK" /output

# Step 3: Create config if not exists
if [ ! -f accumulate.toml ]; then
    echo "Step 3: Creating accumulate.toml..."
    cat > accumulate.toml <<'EOF'
network = "MainNet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "badger"
  enable-healing = false
  enable-snapshots = false

  dn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

  bvn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
EOF
fi

# Step 4: Launch follower
echo "Step 4: Launching follower..."
docker run -d \
    --name accumulate-follower \
    --restart unless-stopped \
    -v "$(pwd)/bvn-data:/node/bvnn" \
    -v "$(pwd)/dn-data:/node/dnn" \
    -v "$(pwd)/accumulate.toml:/node/accumulate.toml" \
    -p 16591-16593:16591-16593 \
    -p 16691-16693:16691-16693 \
    registry.gitlab.com/accumulatenetwork/accumulate:latest \
    run-dual /node/dnn /node/bvnn

echo ""
echo "=== Deployment Complete ==="
echo ""
echo "Check status:"
echo "  docker logs -f accumulate-follower"
echo ""
echo "Verify sync:"
echo "  curl -s http://localhost:16692/status | jq '.result.sync_info'"
