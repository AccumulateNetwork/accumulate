#!/bin/bash
# Run a local consensus testnet with 3 nodes

set -e

# Generate seeds for 3 validators
SEED1="0000000000000000000000000000000000000000000000000000000000000001"
SEED2="0000000000000000000000000000000000000000000000000000000000000002"
SEED3="0000000000000000000000000000000000000000000000000000000000000003"

# Derive public keys from seeds (using Go to compute them)
# For ed25519, pubkey = seed -> private key -> public key
# These are precomputed for the above seeds:
PUBKEY1="4cb5abf6ad79fbf5abbccafcc269d85cd2651ed4b885b5869f241aedf0a5ba29"
PUBKEY2="3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c"
PUBKEY3="fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025"

VALIDATORS="${PUBKEY1},${PUBKEY2},${PUBKEY3}"

echo "Starting consensus testnet with 3 nodes..."
echo "Validators: ${VALIDATORS}"

# Clean up on exit
cleanup() {
    echo "Stopping nodes..."
    kill $PID1 $PID2 $PID3 2>/dev/null || true
}
trap cleanup EXIT

# Build the binary
echo "Building consensus-testnet..."
go build -o /tmp/consensus-testnet ./cmd/consensus-testnet

# Start node 1
echo "Starting node 1..."
/tmp/consensus-testnet \
    --seed "$SEED1" \
    --listen "/ip4/127.0.0.1/tcp/9001" \
    --validators "$VALIDATORS" \
    --block-interval "3s" \
    --tx-rate 10 \
    --log-level info \
    > /tmp/node1.log 2>&1 &
PID1=$!

sleep 1

# Start node 2 (connect to node 1)
echo "Starting node 2..."
/tmp/consensus-testnet \
    --seed "$SEED2" \
    --listen "/ip4/127.0.0.1/tcp/9002" \
    --peers "/ip4/127.0.0.1/tcp/9001/p2p/12D3KooWDpJ7As7P1QUp44PfPNNpZsGM7MJkXKQSKfbJZxm3xP8K" \
    --validators "$VALIDATORS" \
    --block-interval "3s" \
    --tx-rate 10 \
    --log-level info \
    > /tmp/node2.log 2>&1 &
PID2=$!

sleep 1

# Start node 3 (connect to both)
echo "Starting node 3..."
/tmp/consensus-testnet \
    --seed "$SEED3" \
    --listen "/ip4/127.0.0.1/tcp/9003" \
    --peers "/ip4/127.0.0.1/tcp/9001/p2p/12D3KooWDpJ7As7P1QUp44PfPNNpZsGM7MJkXKQSKfbJZxm3xP8K,/ip4/127.0.0.1/tcp/9002/p2p/12D3KooWCVmV8qC8yXLxpQQvmPTSJSmpE3ShvZBqVqM1qszBLuPJ" \
    --validators "$VALIDATORS" \
    --block-interval "3s" \
    --tx-rate 10 \
    --log-level info \
    > /tmp/node3.log 2>&1 &
PID3=$!

echo ""
echo "Nodes started! Logs:"
echo "  Node 1: /tmp/node1.log"
echo "  Node 2: /tmp/node2.log"
echo "  Node 3: /tmp/node3.log"
echo ""
echo "Press Ctrl+C to stop..."

# Follow logs
tail -f /tmp/node1.log /tmp/node2.log /tmp/node3.log
