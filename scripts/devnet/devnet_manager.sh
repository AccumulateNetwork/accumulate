#!/bin/bash

# DevNet Management Script
# Kills existing devnet, compiles new version, launches fresh devnet, and runs tests

set -e  # Exit on any error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEVNET_DIR="$ROOT_DIR/.devnet-test"
DEVNET_PORT="27004"
LOG_FILE="$SCRIPT_DIR/devnet_manager.log"
DASHBOARD_PORT="8080"
DASHBOARD_PID_FILE="$SCRIPT_DIR/dashboard.pid"

# Default configuration
DEFAULT_BVNS=4
DEFAULT_VALIDATORS=3
DEFAULT_FOLLOWERS=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" | tee -a "$LOG_FILE"
}

success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] SUCCESS:${NC} $1" | tee -a "$LOG_FILE"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1" | tee -a "$LOG_FILE"
}

# Function to kill devnet processes
kill_devnet() {
    log "🔪 Killing existing devnet processes..."
    
    # Kill dashboard if running
    if [ -f "$DASHBOARD_PID_FILE" ]; then
        local dashboard_pid=$(cat "$DASHBOARD_PID_FILE")
        if kill -0 $dashboard_pid 2>/dev/null; then
            kill $dashboard_pid 2>/dev/null || true
            log "Killed dashboard process (PID: $dashboard_pid)"
        fi
        rm -f "$DASHBOARD_PID_FILE"
    fi
    
    # Kill by process name
    if pgrep -f "accumulated.*devnet" > /dev/null; then
        pkill -f "accumulated.*devnet" || true
        sleep 2
    fi
    
    # Kill by port (more aggressive)
    local pids=$(lsof -ti:$DEVNET_PORT 2>/dev/null || true)
    if [ ! -z "$pids" ]; then
        echo "$pids" | xargs kill -9 || true
        sleep 1
    fi
    
    # Clean up any remaining go run processes
    if pgrep -f "go run.*accumulated" > /dev/null; then
        pkill -f "go run.*accumulated" || true
        sleep 2
    fi
    
    success "✅ Devnet processes killed"
}

# Function to clean devnet data
clean_devnet_data() {
    log "🧹 Cleaning devnet data directory..."
    if [ -d "$DEVNET_DIR" ]; then
        rm -rf "$DEVNET_DIR"
        success "✅ Cleaned $DEVNET_DIR"
    else
        log "ℹ️  Devnet directory doesn't exist, nothing to clean"
    fi
}

# Function to compile accumulate
compile_accumulate() {
    log "🔨 Compiling new accumulate version..."
    cd "$ROOT_DIR"
    
    # Clean any previous builds
    go clean -cache
    
    # Build with race detection for better debugging
    if go build -race -o accumulated ./cmd/accumulated; then
        success "✅ Accumulate compiled successfully"
    else
        error "❌ Compilation failed"
        exit 1
    fi
}

# Function to start dashboard
start_dashboard() {
    log "🖥️  Starting dashboard server..."
    
    # Check if dashboard is already running
    if lsof -ti:$DASHBOARD_PORT >/dev/null 2>&1; then
        log "Dashboard already running on port $DASHBOARD_PORT"
        return 0
    fi
    
    # Compile dashboard server (use the simpler dashboard.go)
    cd "$SCRIPT_DIR/dashboard"
    if go build -o dashboard dashboard.go; then
        # Start dashboard in background
        DASHBOARD_PORT=$DASHBOARD_PORT nohup ./dashboard > "$SCRIPT_DIR/dashboard.log" 2>&1 &
        local dashboard_pid=$!
        echo $dashboard_pid > "$DASHBOARD_PID_FILE"
        
        log "📊 Dashboard started with PID: $dashboard_pid"
        log "🌐 Dashboard URL: http://localhost:$DASHBOARD_PORT"
        
        # Auto-open browser after a short delay
        (sleep 2 && xdg-open "http://localhost:$DASHBOARD_PORT" 2>/dev/null || open "http://localhost:$DASHBOARD_PORT" 2>/dev/null || true) &
        
        # Wait a moment to ensure it started
        sleep 1
        if kill -0 $dashboard_pid 2>/dev/null; then
            success "✅ Dashboard is running at http://localhost:$DASHBOARD_PORT"
        else
            warn "⚠️  Dashboard failed to start - check $SCRIPT_DIR/dashboard.log"
        fi
    else
        warn "⚠️  Failed to compile dashboard server"
    fi
}

# Function to start devnet
start_devnet() {
    log "🚀 Starting fresh devnet..."
    cd "$ROOT_DIR"
    
    # Parse configuration options
    local bvns=$DEFAULT_BVNS
    local validators=$DEFAULT_VALIDATORS
    local followers=$DEFAULT_FOLLOWERS
    
    # Parse command line args for start command
    shift # Remove 'start' command
    while [[ $# -gt 0 ]]; do
        case $1 in
            --bvns)
                bvns="$2"
                shift 2
                ;;
            --validators)
                validators="$2"
                shift 2
                ;;
            --followers)
                followers="$2"
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done
    
    log "📝 Configuration: BVNs=$bvns, Validators=$validators, Followers=$followers"
    
    # Build the devnet command
    local devnet_cmd="go run ./cmd/accumulated run devnet -w .devnet-test --port 27000"
    devnet_cmd="$devnet_cmd --bvns $bvns --validators $validators --followers $followers"
    
    # Start devnet in background
    nohup $devnet_cmd > "$SCRIPT_DIR/devnet.log" 2>&1 &
    local devnet_pid=$!
    echo $devnet_pid > "$SCRIPT_DIR/devnet.pid"
    
    log "📝 Devnet started with PID: $devnet_pid"
    log "📄 Devnet logs: $SCRIPT_DIR/devnet.log"
    
    # Wait for devnet to start (check for API availability)
    log "⏳ Waiting for devnet to be ready..."
    local retries=0
    local max_retries=60
    
    while [ $retries -lt $max_retries ]; do
        if curl -s "http://127.0.0.1:$DEVNET_PORT/v3/describe" > /dev/null 2>&1; then
            success "✅ Devnet is ready and responding on port $DEVNET_PORT"
            
            # Start dashboard after devnet is ready
            start_dashboard
            
            return 0
        fi
        
        # Check if process is still running
        if ! kill -0 $devnet_pid 2>/dev/null; then
            error "❌ Devnet process died during startup"
            cat "$SCRIPT_DIR/devnet.log" | tail -20
            exit 1
        fi
        
        sleep 2
        retries=$((retries + 1))
        
        if [ $((retries % 10)) -eq 0 ]; then
            log "⏳ Still waiting for devnet... ($retries/$max_retries attempts)"
        fi
    done
    
    error "❌ Devnet failed to start within timeout"
    cat "$SCRIPT_DIR/devnet.log" | tail -20
    exit 1
}

# Function to run load tests
run_tests() {
    log "🧪 Running load tests..."
    cd "$SCRIPT_DIR"
    
    if go run main_branch.go; then
        success "✅ Load tests completed successfully"
    else
        error "❌ Load tests failed"
        return 1
    fi
}

# Function to show devnet status
show_status() {
    log "📊 DevNet Status:"
    
    # Check if PID file exists and process is running
    if [ -f "$SCRIPT_DIR/devnet.pid" ]; then
        local pid=$(cat "$SCRIPT_DIR/devnet.pid")
        if kill -0 $pid 2>/dev/null; then
            success "✅ DevNet is running (PID: $pid)"
        else
            warn "⚠️  PID file exists but process is not running"
        fi
    else
        warn "⚠️  No PID file found"
    fi
    
    # Check port availability
    if curl -s "http://127.0.0.1:$DEVNET_PORT/v3/describe" > /dev/null 2>&1; then
        success "✅ API responding on port $DEVNET_PORT"
    else
        error "❌ API not responding on port $DEVNET_PORT"
    fi
    
    # Show recent logs
    if [ -f "$SCRIPT_DIR/devnet.log" ]; then
        log "📄 Recent devnet logs:"
        tail -10 "$SCRIPT_DIR/devnet.log" | sed 's/^/    /'
    fi
}

# Function to restart everything
restart_all() {
    log "🔄 Full DevNet restart initiated..."
    kill_devnet
    clean_devnet_data
    compile_accumulate
    start_devnet "$@"
    success "🎉 Full restart completed successfully!"
}

# Main execution based on arguments
case "${1:-restart}" in
    "kill")
        kill_devnet
        ;;
    "clean")
        kill_devnet
        clean_devnet_data
        ;;
    "compile")
        compile_accumulate
        ;;
    "start")
        start_devnet "$@"
        ;;
    "test")
        run_tests
        ;;
    "status")
        show_status
        ;;
    "restart"|"")
        restart_all
        ;;
    *)
        echo "Usage: $0 [kill|clean|compile|start|test|status|restart]"
        echo ""
        echo "Commands:"
        echo "  kill     - Kill existing devnet processes"
        echo "  clean    - Kill processes and clean data directory"
        echo "  compile  - Compile new accumulate version"
        echo "  start    - Start fresh devnet"
        echo "  test     - Run load tests"
        echo "  status   - Show devnet status"
        echo "  restart  - Full restart (default): kill + clean + compile + start + test"
        exit 1
        ;;
esac