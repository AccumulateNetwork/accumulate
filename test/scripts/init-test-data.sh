#!/bin/bash
# Test Data Initialization Script
#
# This script initializes test data on the Accumulate network using the test wallet.
# It creates lite accounts, ADI token accounts, and ADI data accounts.
#
# Usage:
#   ./init-test-data.sh [options]

set -e

# Default configuration
WALLET_PATH="$HOME/.accumulate/test-wallet.json"
NODE_URL="http://localhost:8080/v3"
BATCH_SIZE=10
CONCURRENCY=5
INITIAL_FUNDS=1000
SKIP_LITE=false
SKIP_ADI_TOKEN=false
SKIP_ADI_DATA=false
DRY_RUN=false

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_usage() {
    cat <<EOF
Test Data Initialization Script

Usage: $0 [options]

Options:
    --wallet <path>         Path to test wallet (default: ~/.accumulate/test-wallet.json)
    --node <url>            Accumulate node URL (default: http://localhost:8080/v3)
    --batch-size <n>        Accounts per batch (default: 10)
    --concurrency <n>       Concurrent workers (default: 5)
    --initial-funds <n>     Initial ACME per account (default: 1000)
    --skip-lite             Skip lite account creation
    --skip-adi-token        Skip ADI token account creation
    --skip-adi-data         Skip ADI data account creation
    --dry-run               Preview without executing
    -h, --help              Show this help

Examples:
    $0                                  # Initialize all accounts with defaults
    $0 --skip-lite                      # Initialize only ADI accounts
    $0 --batch-size 20 --concurrency 10 # Faster initialization
    $0 --dry-run                        # Preview the plan

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --wallet)
            WALLET_PATH="$2"
            shift 2
            ;;
        --node)
            NODE_URL="$2"
            shift 2
            ;;
        --batch-size)
            BATCH_SIZE="$2"
            shift 2
            ;;
        --concurrency)
            CONCURRENCY="$2"
            shift 2
            ;;
        --initial-funds)
            INITIAL_FUNDS="$2"
            shift 2
            ;;
        --skip-lite)
            SKIP_LITE=true
            shift
            ;;
        --skip-adi-token)
            SKIP_ADI_TOKEN=true
            shift
            ;;
        --skip-adi-data)
            SKIP_ADI_DATA=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            print_usage
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            print_usage
            exit 1
            ;;
    esac
done

# Logging
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check if init-test-data binary exists
    if ! command -v init-test-data &> /dev/null; then
        log_error "init-test-data command not found"
        log_info "Building init-test-data..."
        go build -o "$HOME/go/bin/init-test-data" ./cmd/init-test-data
        log_info "init-test-data built successfully"
    fi

    # Check if wallet exists
    if [ ! -f "$WALLET_PATH" ]; then
        log_error "Wallet not found: $WALLET_PATH"
        log_info "Create a wallet first:"
        log_info "  test-wallet create"
        exit 1
    fi

    # Check if node is accessible
    if ! curl -sf "$NODE_URL/describe" > /dev/null 2>&1; then
        log_warn "Node not responding: $NODE_URL"
        log_warn "Make sure the network is running:"
        log_warn "  ./test/docker/manage.sh start"
        read -p "Continue anyway? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    log_info "Prerequisites check complete"
}

# Build command arguments
build_args() {
    local args=()

    args+=("-wallet=$WALLET_PATH")
    args+=("-node=$NODE_URL")
    args+=("-batch-size=$BATCH_SIZE")
    args+=("-concurrency=$CONCURRENCY")
    args+=("-initial-funds=$INITIAL_FUNDS")

    if [ "$SKIP_LITE" = true ]; then
        args+=("-skip-lite")
    fi

    if [ "$SKIP_ADI_TOKEN" = true ]; then
        args+=("-skip-adi-token")
    fi

    if [ "$SKIP_ADI_DATA" = true ]; then
        args+=("-skip-adi-data")
    fi

    if [ "$DRY_RUN" = true ]; then
        args+=("-dry-run")
    fi

    echo "${args[@]}"
}

# Main execution
main() {
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo -e "${BLUE}     Test Data Initialization         ${NC}"
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""

    check_prerequisites

    log_info "Configuration:"
    echo "  Wallet:       $WALLET_PATH"
    echo "  Node:         $NODE_URL"
    echo "  Batch size:   $BATCH_SIZE"
    echo "  Concurrency:  $CONCURRENCY"
    echo "  Initial funds: $INITIAL_FUNDS ACME"
    echo ""

    if [ "$DRY_RUN" = false ]; then
        log_warn "This will create accounts on the network"
        read -p "Continue? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Cancelled"
            exit 0
        fi
    fi

    echo ""
    log_info "Starting initialization..."
    echo ""

    # Run init-test-data
    args=$(build_args)
    init-test-data $args

    echo ""
    log_info "Initialization complete!"

    if [ -f /tmp/init-test-data-summary.json ]; then
        echo ""
        log_info "Summary:"
        cat /tmp/init-test-data-summary.json | jq '.'
    fi
}

# Run main
main
