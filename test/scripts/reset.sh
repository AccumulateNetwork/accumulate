#!/bin/bash
# Reset Script for Load Testing Environment
#
# This script performs a complete reset of the load testing environment:
#   1. Cleanup: Remove all existing resources
#   2. Rebuild: Build fresh Docker images
#   3. Recreate: Generate new test wallet
#   4. Start: Launch the network
#
# Usage:
#   ./reset.sh [options]

set -e

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKER_DIR="$PROJECT_ROOT/test/docker"
WALLET_DIR="$HOME/.accumulate"

# Reset options
BUILD_IMAGES=true
CREATE_WALLET=true
START_NETWORK=true
NETWORK_MODE="simple"
WALLET_CONFIG="--lite 1000 --adi-token 1000 --adi-data 1000"
SKIP_CLEANUP=false
FORCE=false

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

print_usage() {
    cat <<EOF
Reset Script for Load Testing Environment

Usage: $0 [options]

Options:
    --mode <mode>           Network mode: simple|distributed (default: simple)
    --no-build              Skip Docker image rebuild
    --no-wallet             Skip wallet creation
    --no-start              Don't start network after reset
    --skip-cleanup          Skip cleanup phase
    --wallet-config <opts>  Custom wallet config (default: --lite 1000 --adi-token 1000 --adi-data 1000)
    --force                 Skip all confirmation prompts
    -h, --help              Show this help

Examples:
    $0                          # Full reset with defaults
    $0 --mode distributed       # Reset for distributed deployment
    $0 --no-start               # Prepare but don't start
    $0 --skip-cleanup --force   # Quick restart without cleanup

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode)
            NETWORK_MODE="$2"
            shift 2
            ;;
        --no-build)
            BUILD_IMAGES=false
            shift
            ;;
        --no-wallet)
            CREATE_WALLET=false
            shift
            ;;
        --no-start)
            START_NETWORK=false
            shift
            ;;
        --skip-cleanup)
            SKIP_CLEANUP=true
            shift
            ;;
        --wallet-config)
            WALLET_CONFIG="$2"
            shift 2
            ;;
        --force)
            FORCE=true
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

log_step() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo -e "${BLUE} $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""
}

# Step 1: Cleanup
step_cleanup() {
    if [ "$SKIP_CLEANUP" = true ]; then
        log_warn "Skipping cleanup phase"
        return
    fi

    log_step "Step 1: Cleanup"

    local cleanup_args="--all"
    if [ "$FORCE" = true ]; then
        cleanup_args="$cleanup_args --force"
    fi

    if [ -x "$SCRIPT_DIR/cleanup.sh" ]; then
        "$SCRIPT_DIR/cleanup.sh" $cleanup_args
    else
        log_error "Cleanup script not found or not executable"
        exit 1
    fi
}

# Step 2: Build Docker images
step_build() {
    if [ "$BUILD_IMAGES" = false ]; then
        log_warn "Skipping Docker image build"
        return
    fi

    log_step "Step 2: Build Docker Images"

    local compose_file
    if [ "$NETWORK_MODE" = "distributed" ]; then
        compose_file="$DOCKER_DIR/docker-compose.distributed.yml"
    else
        compose_file="$DOCKER_DIR/docker-compose.yml"
    fi

    if [ ! -f "$compose_file" ]; then
        log_error "Compose file not found: $compose_file"
        exit 1
    fi

    log_info "Building Docker images for $NETWORK_MODE mode..."
    docker compose -f "$compose_file" build

    log_info "Docker images built successfully"
}

# Step 3: Create test wallet
step_create_wallet() {
    if [ "$CREATE_WALLET" = false ]; then
        log_warn "Skipping wallet creation"
        return
    fi

    log_step "Step 3: Create Test Wallet"

    local wallet_path="$WALLET_DIR/test-wallet.json"

    # Check if test-wallet command exists
    if ! command -v test-wallet &> /dev/null; then
        log_error "test-wallet command not found"
        log_info "Please install it first: go install ./cmd/test-wallet"
        exit 1
    fi

    log_info "Creating test wallet with configuration: $WALLET_CONFIG"

    # Create wallet
    eval "test-wallet create -path '$wallet_path' $WALLET_CONFIG --force"

    log_info "Test wallet created: $wallet_path"

    # Show wallet info
    test-wallet info "$wallet_path"
}

# Step 4: Start network
step_start_network() {
    if [ "$START_NETWORK" = false ]; then
        log_warn "Skipping network start"
        return
    fi

    log_step "Step 4: Start Network"

    if [ -x "$DOCKER_DIR/manage.sh" ]; then
        "$DOCKER_DIR/manage.sh" start "$NETWORK_MODE"
    else
        log_error "Docker manage script not found"
        exit 1
    fi

    log_info "Network started in $NETWORK_MODE mode"
}

# Show reset summary
show_summary() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo -e "${BLUE}     Reset Configuration               ${NC}"
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""
    echo "Reset plan:"
    echo ""

    if [ "$SKIP_CLEANUP" = false ]; then
        echo -e "  ${GREEN}1.${NC} Cleanup existing resources"
    else
        echo -e "  ${YELLOW}1.${NC} Cleanup (skipped)"
    fi

    if [ "$BUILD_IMAGES" = true ]; then
        echo -e "  ${GREEN}2.${NC} Build Docker images"
    else
        echo -e "  ${YELLOW}2.${NC} Build (skipped)"
    fi

    if [ "$CREATE_WALLET" = true ]; then
        echo -e "  ${GREEN}3.${NC} Create test wallet"
        echo -e "      Config: $WALLET_CONFIG"
    else
        echo -e "  ${YELLOW}3.${NC} Wallet (skipped)"
    fi

    if [ "$START_NETWORK" = true ]; then
        echo -e "  ${GREEN}4.${NC} Start network ($NETWORK_MODE mode)"
    else
        echo -e "  ${YELLOW}4.${NC} Start (skipped)"
    fi

    echo ""
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""
}

# Main reset function
main() {
    log_info "Load Testing Environment Reset"

    # Show summary
    show_summary

    # Confirm unless forced
    if [ "$FORCE" = false ]; then
        read -p "Proceed with reset? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Reset cancelled"
            exit 0
        fi
    fi

    echo ""

    # Execute reset steps
    local start_time=$(date +%s)

    step_cleanup
    step_build
    step_create_wallet
    step_start_network

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # Success summary
    echo ""
    log_step "Reset Complete!"

    echo "Environment is ready for load testing"
    echo ""
    echo "Summary:"
    echo "  Mode: $NETWORK_MODE"
    echo "  Duration: ${duration}s"
    echo ""

    if [ "$START_NETWORK" = true ]; then
        echo "Next steps:"
        echo "  1. Check network status:"
        echo "     $DOCKER_DIR/manage.sh status $NETWORK_MODE"
        echo ""
        echo "  2. Check network health:"
        echo "     $DOCKER_DIR/manage.sh health"
        echo ""
        echo "  3. Start monitoring:"
        echo "     $PROJECT_ROOT/test/monitor/network-monitor.sh"
        echo ""
        echo "  4. Run load tests:"
        echo "     load-generator --setup --nodes=http://localhost:8080/v3"
        echo ""
    else
        echo "To start the network:"
        echo "  $DOCKER_DIR/manage.sh start $NETWORK_MODE"
        echo ""
    fi
}

# Run main
main
