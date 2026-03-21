#!/bin/bash
# DAG-BFT Stress Test Runner
# Starts 7-validator network and runs load generator

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
NODES_DIR="${ROOT_DIR}/.nodes"
LOG_DIR="/tmp/dagbft-stress"

# Faucet account derived from seed "load test faucet"
FAUCET_URL="acc://7cf23160d39d51ac2b59f36d936862c77902d27a6c452d7d/ACME"
FAUCET_KEY="924a0963318e868947d9858bc94d8499b9bb8cab68581a368dbd1995221a946e36b64dcdb69a88bc6b1a7e00f85f3ca2327c9b22b377480d52f3185d08523370"

# Node endpoints
NODES="http://127.0.1.1:26660/v3,http://127.0.1.2:26660/v3,http://127.0.1.3:26660/v3,http://127.0.2.1:26660/v3,http://127.0.2.2:26660/v3,http://127.0.2.3:26660/v3,http://127.0.2.4:26660/v3"

usage() {
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  init       Initialize network (creates .nodes directory)"
    echo "  start      Start all 7 validators"
    echo "  stop       Stop all validators"
    echo "  setup      Create test accounts (run once after start)"
    echo "  load       Run load test at specified TPS"
    echo "  status     Check validator status"
    echo "  clean      Remove all state and logs"
    echo ""
    echo "Examples:"
    echo "  $0 init                    # Initialize network"
    echo "  $0 start                   # Start validators"
    echo "  $0 setup                   # Create 10k accounts"
    echo "  $0 load --tps=1000         # Run at 1000 TPS"
    exit 1
}

cmd_init() {
    echo "Initializing 7-validator network..."
    mkdir -p "$LOG_DIR"

    # Clean previous state
    rm -rf "$NODES_DIR"

    # Initialize with faucet
    cd "$ROOT_DIR"
    ./accumulated init network config/dagbft-testnet.yml -w .nodes --faucet-seed "load test faucet" > "$LOG_DIR/init.log" 2>&1

    echo "Network initialized. Faucet: $FAUCET_URL (200M ACME)"
    echo "Node configs in: $NODES_DIR"
}

cmd_start() {
    echo "Starting 7 validators..."
    mkdir -p "$LOG_DIR"

    cd "$ROOT_DIR"

    for node in bvn1-{1,2,3} bvn2-{1,2,3,4}; do
        echo "  Starting $node..."
        ./accumulated run -w ".nodes/$node" > "$LOG_DIR/$node.log" 2>&1 &
        echo $! > "$LOG_DIR/$node.pid"
    done

    echo "All validators starting. Logs in: $LOG_DIR"
    echo "Wait 30 seconds for network to stabilize..."
}

cmd_stop() {
    echo "Stopping validators..."

    for pidfile in "$LOG_DIR"/*.pid; do
        if [ -f "$pidfile" ]; then
            pid=$(cat "$pidfile")
            node=$(basename "$pidfile" .pid)
            if kill -0 "$pid" 2>/dev/null; then
                echo "  Stopping $node (PID $pid)..."
                kill "$pid"
            fi
            rm "$pidfile"
        fi
    done

    echo "All validators stopped."
}

cmd_setup() {
    echo "Setting up test accounts..."
    cd "$ROOT_DIR"

    ./load-generator \
        --nodes="$NODES" \
        --funder="$FAUCET_URL" \
        --funder-key="$FAUCET_KEY" \
        --accounts=10000 \
        --setup
}

cmd_load() {
    echo "Running load test..."
    cd "$ROOT_DIR"

    ./load-generator \
        --nodes="$NODES" \
        --funder="$FAUCET_URL" \
        --funder-key="$FAUCET_KEY" \
        "$@"
}

cmd_status() {
    echo "Checking validator status..."

    for node in 127.0.1.{1,2,3} 127.0.2.{1,2,3,4}; do
        status=$(curl -s "http://$node:26660/v3" -X POST -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","id":1,"method":"consensus-status","params":{}}' 2>/dev/null | head -c 200)
        if [ -n "$status" ]; then
            echo "  $node: OK"
        else
            echo "  $node: NOT RESPONDING"
        fi
    done
}

cmd_clean() {
    echo "Cleaning up..."
    cmd_stop 2>/dev/null || true
    rm -rf "$NODES_DIR" "$LOG_DIR"
    echo "Cleaned."
}

# Main
case "${1:-}" in
    init)   cmd_init ;;
    start)  cmd_start ;;
    stop)   cmd_stop ;;
    setup)  cmd_setup ;;
    load)   shift; cmd_load "$@" ;;
    status) cmd_status ;;
    clean)  cmd_clean ;;
    *)      usage ;;
esac
