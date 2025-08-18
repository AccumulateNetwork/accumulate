#!/bin/bash

# Flexible DevNet Configuration Manager
# Makes it easy to start DevNet with custom partition and validator configurations

set -e  # Exit on any error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEVNET_DIR="$ROOT_DIR/.devnet-test"
BASE_PORT="${BASE_PORT:-26656}"
LOG_FILE="$SCRIPT_DIR/devnet_config.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Default configuration
DEFAULT_BVNS=2
DEFAULT_VALIDATORS=3
DEFAULT_FOLLOWERS=1

# Parse command line arguments
BVNS=${1:-$DEFAULT_BVNS}
VALIDATORS=${2:-$DEFAULT_VALIDATORS}
FOLLOWERS=${3:-$DEFAULT_FOLLOWERS}

# Logging functions
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" | tee -a "$LOG_FILE"
}

success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] SUCCESS:${NC} $1" | tee -a "$LOG_FILE"
}

info() {
    echo -e "${CYAN}[$(date +'%Y-%m-%d %H:%M:%S')] INFO:${NC} $1" | tee -a "$LOG_FILE"
}

# Function to kill existing DevNet processes
kill_existing_devnet() {
    log "🔪 Killing existing DevNet processes..."
    
    # Kill by process name
    if pgrep -f "accumulated.*devnet" > /dev/null; then
        pkill -f "accumulated.*devnet" || true
        sleep 2
    fi
    
    # Kill by ports (26656-26700 range typical for DevNet)
    for port in $(seq 26656 26700); do
        local pids=$(lsof -ti:$port 2>/dev/null || true)
        if [ ! -z "$pids" ]; then
            echo "$pids" | xargs kill -9 2>/dev/null || true
        fi
    done
    
    success "✅ Existing DevNet processes killed"
}

# Function to clean DevNet data
clean_devnet_data() {
    log "🧹 Cleaning DevNet data directory..."
    if [ -d "$DEVNET_DIR" ]; then
        rm -rf "$DEVNET_DIR"
        success "✅ Cleaned $DEVNET_DIR"
    fi
}

# Function to calculate port usage
calculate_port_usage() {
    local bvns=$1
    local validators=$2
    local followers=$3
    
    # Calculate total nodes
    local dn_nodes=$((validators + followers))  # Directory Network
    local bvn_nodes=$((validators + followers))  # Per BVN
    local total_nodes=$((dn_nodes + (bvn_nodes * bvns)))
    
    # Each node uses 4 ports (RPC, P2P, Metric, API)
    local ports_needed=$((total_nodes * 4))
    
    echo "Total nodes: $total_nodes (DN: $dn_nodes, BVNs: $bvns × $bvn_nodes)"
    echo "Ports needed: $ports_needed (starting from $BASE_PORT)"
}

# Function to start DevNet with configuration
start_configured_devnet() {
    local bvns=$1
    local validators=$2
    local followers=$3
    
    info "🚀 Starting DevNet with configuration:"
    info "  - BVNs (partitions): $bvns"
    info "  - Validators per partition: $validators"
    info "  - Followers per partition: $followers"
    
    # Calculate resource usage
    calculate_port_usage $bvns $validators $followers
    
    cd "$ROOT_DIR"
    
    # Build the command
    # Check if accumulated binary exists, otherwise build it
    if [ -f "$ROOT_DIR/accumulated" ]; then
        local cmd="$ROOT_DIR/accumulated run devnet"
        info "Using existing accumulated binary"
    else
        info "Building accumulated binary..."
        (cd "$ROOT_DIR" && go build -o accumulated ./cmd/accumulated)
        local cmd="$ROOT_DIR/accumulated run devnet"
    fi
    cmd="$cmd -w $DEVNET_DIR"
    cmd="$cmd --port $BASE_PORT"
    cmd="$cmd --bvns $bvns"
    cmd="$cmd --validators $validators"
    cmd="$cmd --followers $followers"
    cmd="$cmd --reset"  # Always reset for clean state
    
    info "📝 Running command: $cmd"
    
    # Start DevNet in background
    nohup $cmd > "$SCRIPT_DIR/devnet_config.log" 2>&1 &
    local devnet_pid=$!
    echo $devnet_pid > "$SCRIPT_DIR/devnet.pid"
    
    log "📝 DevNet started with PID: $devnet_pid"
    log "📄 Logs: $SCRIPT_DIR/devnet_config.log"
    
    # Wait for DevNet to be ready
    wait_for_devnet
    
    # Export port information for test code
    export_port_info $bvns $validators $followers
}

# Function to wait for DevNet to be ready
wait_for_devnet() {
    log "⏳ Waiting for DevNet to be ready..."
    
    local retries=0
    local max_retries=60
    local api_port=$((BASE_PORT + 4))  # API port is typically base + 4
    
    while [ $retries -lt $max_retries ]; do
        # Check multiple endpoints to ensure full startup
        if curl -s "http://127.0.0.1:$api_port/v3" > /dev/null 2>&1; then
            success "✅ DevNet API is responding on port $api_port"
            
            # Additional check for all partitions
            if check_all_partitions; then
                success "✅ All partitions are ready!"
                return 0
            fi
        fi
        
        # Check if process is still running
        if [ -f "$SCRIPT_DIR/devnet.pid" ]; then
            local pid=$(cat "$SCRIPT_DIR/devnet.pid")
            if ! kill -0 $pid 2>/dev/null; then
                error "❌ DevNet process died during startup"
                tail -20 "$SCRIPT_DIR/devnet_config.log"
                exit 1
            fi
        fi
        
        sleep 2
        retries=$((retries + 1))
        
        if [ $((retries % 10)) -eq 0 ]; then
            log "⏳ Still waiting... ($retries/$max_retries attempts)"
        fi
    done
    
    error "❌ DevNet failed to start within timeout"
    tail -20 "$SCRIPT_DIR/devnet_config.log"
    exit 1
}

# Function to check all partitions are responding
check_all_partitions() {
    local api_port=$((BASE_PORT + 4))
    
    # Try to get network status which should include all partitions
    if curl -s "http://127.0.0.1:$api_port/v3/network/status" > /dev/null 2>&1; then
        return 0
    fi
    
    return 1
}

# Function to export port information for test code
export_port_info() {
    local bvns=$1
    local validators=$2
    local followers=$3
    
    local discovery_file="$SCRIPT_DIR/devnet_ports.json"
    local env_file="$SCRIPT_DIR/devnet.env"
    
    info "📝 Exporting port information for test code..."
    
    # Calculate port assignments
    local api_port=$((BASE_PORT + 4))
    local metrics_port=$((BASE_PORT + 60))
    
    # Create JSON discovery file with port mappings
    cat > "$discovery_file" <<EOF
{
  "created_at": "$(date -Iseconds)",
  "base_port": $BASE_PORT,
  "api": {
    "primary": "http://127.0.0.1:$api_port/v3",
    "port": $api_port
  },
  "metrics": {
    "endpoint": "http://127.0.0.1:$metrics_port/metrics",
    "port": $metrics_port
  },
  "network": {
    "bvns": $bvns,
    "validators_per_partition": $validators,
    "followers_per_partition": $followers,
    "total_nodes": $((4 + (bvns * 4)))
  },
  "partitions": {
    "directory": {
      "api": "http://127.0.0.1:$api_port/v3",
      "rpc": "http://127.0.0.1:$((BASE_PORT + 1))",
      "p2p": $BASE_PORT,
      "nodes": $((validators + followers))
    },
EOF
    
    # Add BVN partition information
    for ((i=0; i<bvns; i++)); do
        local bvn_base=$((BASE_PORT + 100 * (i + 1)))
        if [ $i -gt 0 ]; then
            echo "," >> "$discovery_file"
        fi
        cat >> "$discovery_file" <<EOF
    "bvn$i": {
      "api": "http://127.0.0.1:$((bvn_base + 4))/v3",
      "rpc": "http://127.0.0.1:$((bvn_base + 1))",
      "p2p": $bvn_base,
      "nodes": $((validators + followers))
    }
EOF
    done
    
    # Close JSON
    cat >> "$discovery_file" <<EOF
  }
}
EOF
    
    # Create environment file for shell scripts
    cat > "$env_file" <<EOF
# DevNet Environment Configuration
# Generated: $(date)
export DEVNET_BASE_PORT=$BASE_PORT
export DEVNET_API_PORT=$api_port
export DEVNET_API_ENDPOINT="http://127.0.0.1:$api_port/v3"
export DEVNET_METRICS_PORT=$metrics_port
export DEVNET_METRICS_ENDPOINT="http://127.0.0.1:$metrics_port/metrics"
export DEVNET_BVNS=$bvns
export DEVNET_VALIDATORS=$validators
export DEVNET_FOLLOWERS=$followers
export DEVNET_WORK_DIR="$DEVNET_DIR"
export DEVNET_DISCOVERY_FILE="$discovery_file"
EOF
    
    # Also export to current shell
    export DEVNET_BASE_PORT=$BASE_PORT
    export DEVNET_API_PORT=$api_port
    export DEVNET_API_ENDPOINT="http://127.0.0.1:$api_port/v3"
    export DEVNET_DISCOVERY_FILE="$discovery_file"
    
    success "✅ Port information exported to:"
    info "  JSON: $discovery_file"
    info "  ENV:  $env_file"
    info "  Primary API: http://127.0.0.1:$api_port/v3"
}

# Function to show DevNet status
show_devnet_status() {
    info "📊 DevNet Status:"
    
    # Check PID
    if [ -f "$SCRIPT_DIR/devnet.pid" ]; then
        local pid=$(cat "$SCRIPT_DIR/devnet.pid")
        if kill -0 $pid 2>/dev/null; then
            success "✅ DevNet is running (PID: $pid)"
        else
            error "❌ PID file exists but process is not running"
        fi
    else
        error "❌ No PID file found"
    fi
    
    # Check API endpoints
    local api_port=$((BASE_PORT + 4))
    if curl -s "http://127.0.0.1:$api_port/v3" > /dev/null 2>&1; then
        success "✅ API responding on port $api_port"
        
        # Get network status
        local status=$(curl -s "http://127.0.0.1:$api_port/v3/network/status" 2>/dev/null)
        if [ ! -z "$status" ]; then
            info "Network configuration detected"
        fi
    else
        error "❌ API not responding"
    fi
    
    # Show recent logs
    if [ -f "$SCRIPT_DIR/devnet_config.log" ]; then
        info "📄 Recent logs:"
        tail -10 "$SCRIPT_DIR/devnet_config.log" | sed 's/^/    /'
    fi
}

# Function to create test configuration files
create_test_configs() {
    local config_dir="$SCRIPT_DIR/devnet_configs"
    mkdir -p "$config_dir"
    
    # Create different test configurations
    cat > "$config_dir/minimal.conf" <<EOF
# Minimal DevNet configuration (for quick tests)
BVNS=2
VALIDATORS=1
FOLLOWERS=0
EOF

    cat > "$config_dir/standard.conf" <<EOF
# Standard DevNet configuration (balanced)
BVNS=2
VALIDATORS=3
FOLLOWERS=1
EOF

    cat > "$config_dir/large.conf" <<EOF
# Large DevNet configuration (stress testing)
BVNS=3
VALIDATORS=3
FOLLOWERS=2
EOF

    cat > "$config_dir/multi_partition.conf" <<EOF
# Multi-partition configuration (cross-chain testing)
BVNS=5
VALIDATORS=2
FOLLOWERS=1
EOF

    success "✅ Created test configuration files in $config_dir"
}

# Function to load configuration from file
load_config() {
    local config_file=$1
    if [ -f "$config_file" ]; then
        source "$config_file"
        info "📄 Loaded configuration from $config_file"
        info "  BVNS=$BVNS, VALIDATORS=$VALIDATORS, FOLLOWERS=$FOLLOWERS"
    else
        error "❌ Configuration file not found: $config_file"
        exit 1
    fi
}

# Main execution
case "${1:-help}" in
    "start")
        # Start with specified or default configuration
        kill_existing_devnet
        clean_devnet_data
        start_configured_devnet ${2:-$DEFAULT_BVNS} ${3:-$DEFAULT_VALIDATORS} ${4:-$DEFAULT_FOLLOWERS}
        ;;
    
    "stop"|"kill")
        kill_existing_devnet
        ;;
    
    "clean")
        kill_existing_devnet
        clean_devnet_data
        ;;
    
    "status")
        show_devnet_status
        ;;
    
    "configs")
        create_test_configs
        ;;
    
    "load")
        # Load and start from config file
        if [ -z "$2" ]; then
            error "❌ Please specify a configuration file"
            echo "Example: $0 load devnet_configs/large.conf"
            exit 1
        fi
        load_config "$2"
        kill_existing_devnet
        clean_devnet_data
        start_configured_devnet $BVNS $VALIDATORS $FOLLOWERS
        ;;
    
    "quick")
        # Quick minimal setup for testing
        info "🚀 Starting minimal DevNet (2 BVNs, 1 validator each)"
        kill_existing_devnet
        clean_devnet_data
        start_configured_devnet 2 1 0
        ;;
    
    "standard")
        # Standard setup
        info "🚀 Starting standard DevNet (2 BVNs, 3 validators, 1 follower)"
        kill_existing_devnet
        clean_devnet_data
        start_configured_devnet 2 3 1
        ;;
    
    "large")
        # Large setup for stress testing
        info "🚀 Starting large DevNet (3 BVNs, 3 validators, 2 followers)"
        kill_existing_devnet
        clean_devnet_data
        start_configured_devnet 3 3 2
        ;;
    
    "multi")
        # Multi-partition setup for cross-chain testing
        info "🚀 Starting multi-partition DevNet (5 BVNs, 2 validators, 1 follower)"
        kill_existing_devnet
        clean_devnet_data
        start_configured_devnet 5 2 1
        ;;
    
    "help"|"-h"|"--help")
        echo "🔧 Flexible DevNet Configuration Manager"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "Commands:"
        echo "  start [bvns] [validators] [followers]"
        echo "      Start DevNet with specified configuration"
        echo "      Default: 2 BVNs, 3 validators, 1 follower"
        echo ""
        echo "  quick    - Start minimal DevNet (2 BVNs, 1 validator)"
        echo "  standard - Start standard DevNet (2 BVNs, 3 validators, 1 follower)"
        echo "  large    - Start large DevNet (3 BVNs, 3 validators, 2 followers)"
        echo "  multi    - Start multi-partition DevNet (5 BVNs, 2 validators, 1 follower)"
        echo ""
        echo "  stop/kill - Stop running DevNet"
        echo "  clean     - Stop DevNet and clean data"
        echo "  status    - Show DevNet status"
        echo "  configs   - Create sample configuration files"
        echo "  load <file> - Load and start from configuration file"
        echo ""
        echo "Examples:"
        echo "  $0 start               # Start with defaults"
        echo "  $0 start 3 3 2         # 3 BVNs, 3 validators, 2 followers"
        echo "  $0 quick               # Quick minimal setup"
        echo "  $0 load my.conf        # Load from config file"
        echo ""
        echo "Environment Variables:"
        echo "  BASE_PORT - Starting port number (default: 26656)"
        echo ""
        ;;
    
    *)
        error "❌ Unknown command: $1"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac