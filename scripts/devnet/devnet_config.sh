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

# Simple environment variables for debugging (optional)
export DEVNET_DEBUG="${DEVNET_DEBUG:-false}"  # Set to "true" for debug logging

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
    
    info "🚀 Starting DevNet with automatic logging:"
    info "  - BVNs (partitions): $bvns"
    info "  - Validators per partition: $validators"
    info "  - Followers per partition: $followers"
    info "  - Debug mode: $DEVNET_DEBUG"
    
    # Calculate resource usage
    calculate_port_usage $bvns $validators $followers
    
    cd "$ROOT_DIR"
    
    # Simple command - logging is automatic!
    local cmd="go run ./cmd/accumulated run devnet"
    cmd="$cmd -w $DEVNET_DIR"
    cmd="$cmd --port $BASE_PORT"
    cmd="$cmd --bvns $bvns"
    cmd="$cmd --validators $validators"
    cmd="$cmd --followers $followers"
    cmd="$cmd --reset"  # Always reset for clean state
    
    # Add --dpm flag if DEVNET_DPM is set
    if [ ! -z "$DEVNET_DPM" ] && [ "$DEVNET_DPM" -gt 0 ]; then
        cmd="$cmd --dpm $DEVNET_DPM"
        info "  - Recovery testing: $DEVNET_DPM drops per minute"
    fi
    
    info "📝 Running command (logging auto-configured): $cmd"
    
    # Start DevNet in background with structured logging
    nohup $cmd > "$SCRIPT_DIR/devnet.log" 2>&1 &
    local devnet_pid=$!
    echo $devnet_pid > "$SCRIPT_DIR/devnet.pid"
    
    log "📝 DevNet started with PID: $devnet_pid"
    log "📄 Automatic logs: $SCRIPT_DIR/devnet.log"
    log "🔍 Debug mode: $DEVNET_DEBUG"
    
    # Wait for DevNet to be ready
    wait_for_devnet
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

# Function to analyze DevNet logs
analyze_devnet_logs() {
    local filter=${1:-""}
    
    if [ ! -f "$SCRIPT_DIR/devnet.log" ]; then
        error "❌ DevNet log file not found"
        return 1
    fi
    
    info "📊 Analyzing DevNet logs..."
    
    case "$filter" in
        "conductor"|"ccc")
            info "CrossChain Conductor Activity:"
            grep -E "devnet\.conductor|msg_processing|crosschain" "$SCRIPT_DIR/devnet.log" | tail -20 | sed 's/^/  /'
            ;;
        "partition"|"bvn")
            info "Partition Activity:"
            grep -E "devnet\.partition|partition=|BVN[0-9]+" "$SCRIPT_DIR/devnet.log" | tail -20 | sed 's/^/  /'
            ;;
        "metrics"|"performance")
            info "Performance Metrics:"
            grep -E "devnet\.metrics|throughput|latency|req/s" "$SCRIPT_DIR/devnet.log" | tail -20 | sed 's/^/  /'
            ;;
        "errors"|"failures")
            info "Errors and Failures:"
            grep -E "ERROR|WARN|failed|error" "$SCRIPT_DIR/devnet.log" | tail -20 | sed 's/^/  /'
            ;;
        "recovery"|"gaps")
            info "Gap Recovery Activity:"
            grep -E "devnet\.recovery|gap_detected|gap_recovered|gap_size=" "$SCRIPT_DIR/devnet.log" | tail -20 | sed 's/^/  /'
            ;;
        *)
            info "Recent DevNet Activity:"
            tail -20 "$SCRIPT_DIR/devnet.log" | sed 's/^/  /'
            ;;
    esac
}

# Function to monitor DevNet logs in real-time
monitor_devnet_logs() {
    local filter=${1:-""}
    
    if [ ! -f "$SCRIPT_DIR/devnet.log" ]; then
        error "❌ DevNet log file not found"
        return 1
    fi
    
    info "👁️  Monitoring DevNet logs (Ctrl+C to stop)..."
    echo ""
    
    case "$filter" in
        "conductor"|"ccc")
            tail -f "$SCRIPT_DIR/devnet.log" | grep --line-buffered -E "conductor|crosschain" | \
            while read line; do
                echo -e "${CYAN}[CCC]${NC} $line"
            done
            ;;
        "partition"|"bvn")
            tail -f "$SCRIPT_DIR/devnet.log" | grep --line-buffered -E "partition|BVN" | \
            while read line; do
                echo -e "${BLUE}[PARTITION]${NC} $line"
            done
            ;;
        "metrics"|"performance")
            tail -f "$SCRIPT_DIR/devnet.log" | grep --line-buffered -E "metrics|throughput|devnet.metrics" | \
            while read line; do
                echo -e "${GREEN}[METRICS]${NC} $line"
            done
            ;;
        "errors"|"failures")
            tail -f "$SCRIPT_DIR/devnet.log" | grep --line-buffered -E "ERROR|WARN|failed" | \
            while read line; do
                echo -e "${RED}[ERROR]${NC} $line"
            done
            ;;
        *)
            tail -f "$SCRIPT_DIR/devnet.log" | \
            while read line; do
                if echo "$line" | grep -q "ERROR\|WARN"; then
                    echo -e "${RED}$line${NC}"
                elif echo "$line" | grep -q "conductor\|crosschain"; then
                    echo -e "${CYAN}$line${NC}"
                elif echo "$line" | grep -q "metrics\|throughput"; then
                    echo -e "${GREEN}$line${NC}"
                else
                    echo "$line"
                fi
            done
            ;;
    esac
}

# Function to extract structured log metrics
extract_log_metrics() {
    local time_window=${1:-"5m"}
    
    if [ ! -f "$SCRIPT_DIR/devnet.log" ]; then
        error "❌ DevNet log file not found"
        return 1
    fi
    
    info "📈 Extracting metrics from last $time_window..."
    
    # Extract recent JSON logs and analyze
    local temp_file=$(mktemp)
    tail -1000 "$SCRIPT_DIR/devnet.log" | grep -E '^\{.*\}$' > "$temp_file"
    
    if [ ! -s "$temp_file" ]; then
        info "  No structured JSON logs found in recent entries"
        rm -f "$temp_file"
        return
    fi
    
    # Count different event types
    local conductor_events=$(grep -c '"component":"devnet.conductor"' "$temp_file" 2>/dev/null || echo 0)
    local metrics_events=$(grep -c '"component":"devnet.metrics"' "$temp_file" 2>/dev/null || echo 0)
    local partition_events=$(grep -c '"component":"devnet.partition"' "$temp_file" 2>/dev/null || echo 0)
    local recovery_events=$(grep -c '"component":"devnet.recovery"' "$temp_file" 2>/dev/null || echo 0)
    
    echo "  📊 Event Summary:"
    echo "    Conductor Events: $conductor_events"
    echo "    Partition Events: $partition_events"
    echo "    Recovery Events:  $recovery_events"
    echo "    Metrics Events:   $metrics_events"
    
    # Extract performance metrics if available
    if [ $metrics_events -gt 0 ]; then
        echo ""
        echo "  🚀 Performance Highlights:"
        grep '"component":"devnet.metrics"' "$temp_file" | tail -3 | \
        while read line; do
            # Simple extraction of key metrics
            throughput=$(echo "$line" | grep -o '"throughput_per_second":[0-9.]*' | cut -d: -f2)
            success_rate=$(echo "$line" | grep -o '"success_rate_percent":[0-9.]*' | cut -d: -f2)
            if [ ! -z "$throughput" ] && [ ! -z "$success_rate" ]; then
                echo "    Throughput: ${throughput} req/s, Success: ${success_rate}%"
            fi
        done
    fi
    
    rm -f "$temp_file"
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

# Function to check debug endpoints
check_debug_endpoints() {
    local api_port=$((BASE_PORT + 4))
    
    info "🔍 Available Debug Endpoints:"
    
    # Check basic API
    if curl -s --max-time 3 "http://127.0.0.1:$api_port/v3" &>/dev/null; then
        success "  ✅ /v3 - Basic API endpoint"
    else
        error "  ❌ /v3 - Basic API endpoint not responding"
    fi
    
    # Check CCC pause endpoints (testnet build)
    if curl -s --max-time 3 "http://127.0.0.1:$api_port/debug/ccc/status" &>/dev/null; then
        success "  ✅ /debug/ccc/status - CrossChain Conductor status"
        success "  ✅ /debug/ccc/pause - Pause CCC (testnet build)"
        success "  ✅ /debug/ccc/resume - Resume CCC (testnet build)" 
    else
        info "  ⚠️  CCC debug endpoints not available (requires testnet build)"
    fi
    
    # Check metrics endpoints
    if curl -s --max-time 3 "http://127.0.0.1:$api_port/metrics" &>/dev/null; then
        success "  ✅ /metrics - Prometheus metrics"
    else
        info "  ⚠️  Metrics endpoint not available"
    fi
    
    echo ""
    info "💡 To enable CCC pause/resume endpoints:"
    echo "  1. Build with testnet features: go build -tags testnet -o accumulated ./cmd/accumulated"
    echo "  2. Restart DevNet"
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
    
    "logs")
        # View and filter logs
        analyze_devnet_logs ${2:-""}
        ;;
    
    "monitor")
        # Real-time log monitoring
        monitor_devnet_logs ${2:-""}  
        ;;
    
    "dashboard")
        # Launch monitoring dashboard
        if [ -f "$SCRIPT_DIR/devnet_monitor.sh" ]; then
            "$SCRIPT_DIR/devnet_monitor.sh" dashboard ${2:-5}
        else
            error "❌ devnet_monitor.sh not found"
            exit 1
        fi
        ;;
    
    "metrics")
        # Extract performance metrics
        extract_log_metrics ${2:-"5m"}
        ;;
    
    "debug")
        # Debug mode with verbose logging
        DEVNET_DEBUG=true start_configured_devnet ${2:-2} ${3:-2} ${4:-1}
        ;;
    
    "endpoints")
        # Show available debug endpoints
        check_debug_endpoints
        ;;
    
    "testnet")
        # Build with testnet features for CCC pause/resume
        info "🔧 Building with testnet features..."
        cd "$ROOT_DIR"
        go build -tags testnet -o accumulated ./cmd/accumulated
        success "✅ Built with testnet features (CCC pause/resume enabled)"
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
        echo "🔧 Enhanced DevNet Configuration Manager with Logging Integration"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "🚀 Basic Commands:"
        echo "  start [bvns] [validators] [followers]  - Start DevNet with configuration"
        echo "  quick      - Start minimal DevNet (2 BVNs, 1 validator)"
        echo "  standard   - Start standard DevNet (2 BVNs, 3 validators, 1 follower)" 
        echo "  large      - Start large DevNet (3 BVNs, 3 validators, 2 followers)"
        echo "  multi      - Start multi-partition DevNet (5 BVNs, 2 validators, 1 follower)"
        echo "  debug      - Start DevNet with debug-level logging"
        echo ""
        echo "⚙️  Management Commands:"
        echo "  stop/kill  - Stop running DevNet"
        echo "  clean      - Stop DevNet and clean data"
        echo "  status     - Show DevNet status"
        echo "  configs    - Create sample configuration files"
        echo "  load <file> - Load and start from configuration file"
        echo ""
        echo "📊 Monitoring & Analysis:"
        echo "  logs [filter]     - View filtered logs (conductor|partition|metrics|errors)"
        echo "  monitor [filter]  - Real-time log monitoring with color coding"
        echo "  dashboard [interval] - Launch live monitoring dashboard"
        echo "  metrics [window]  - Extract performance metrics"
        echo ""
        echo "🔧 Debug & Testing:"
        echo "  endpoints  - Check available debug endpoints"
        echo "  testnet    - Build with testnet features (enables CCC pause/resume)"
        echo ""
        echo "📝 Examples:"
        echo "  $0 start                 # Start with automatic logging"
        echo "  $0 start 3 3 2           # 3 BVNs, 3 validators, 2 followers" 
        echo "  DEVNET_DEBUG=true $0 start # Start with debug logging"
        echo "  $0 debug                 # Shortcut for debug mode"
        echo "  $0 logs conductor        # View CrossChain Conductor logs"
        echo "  $0 monitor metrics       # Monitor performance metrics in real-time"
        echo "  $0 dashboard 10          # Live dashboard with 10s refresh"
        echo ""
        echo "🌍 Environment Variables:"
        echo "  BASE_PORT     - Starting port number (default: 26656)"
        echo "  DEVNET_DEBUG  - Enable debug logging (default: false)"
        echo ""
        echo "🔍 Manual Log Analysis (Alternative to built-in commands):"
        echo "  grep 'devnet.conductor' devnet.log                    # CrossChain Conductor activity"
        echo "  grep 'gap_size=' devnet.log                           # Gap recovery events"
        echo "  grep 'partition=BVN1' devnet.log | tail -10          # BVN1 specific activity"
        echo "  awk '/msg_processing/ {print \$1, \$2, \$5, \$6}' devnet.log  # Extract message details"
        echo "  grep 'req/s' devnet.log | tail -5                     # Recent performance metrics"
        echo ""
        ;;
    
    *)
        error "❌ Unknown command: $1"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac