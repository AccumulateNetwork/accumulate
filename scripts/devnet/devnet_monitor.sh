#!/bin/bash

# Real-time DevNet monitoring dashboard with enhanced logging integration
# This script provides a live view of DevNet operation status

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
BASE_PORT="${BASE_PORT:-26656}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
NC='\033[0m'

# Check if devnet is running
check_devnet_running() {
    if [ -f "$SCRIPT_DIR/devnet.pid" ]; then
        local pid=$(cat "$SCRIPT_DIR/devnet.pid")
        if kill -0 $pid 2>/dev/null; then
            return 0
        fi
    fi
    return 1
}

# Get basic devnet info from environment/logs
get_devnet_info() {
    local bvns="Unknown"
    local validators="Unknown"
    local network="Unknown"
    
    if [ -f "$SCRIPT_DIR/devnet.log" ]; then
        # Try to extract from recent logs
        bvns=$(grep -o '"bvns":[0-9]*' "$SCRIPT_DIR/devnet.log" | tail -1 | cut -d: -f2 || echo "Unknown")
        validators=$(grep -o '"validators_per_bvn":[0-9]*' "$SCRIPT_DIR/devnet.log" | tail -1 | cut -d: -f2 || echo "Unknown")
        network=$(grep -o '"network_type":"[^"]*"' "$SCRIPT_DIR/devnet.log" | tail -1 | cut -d: -f2 | tr -d '"' || echo "Unknown")
    fi
    
    echo "$bvns|$validators|$network"
}

# Check partition health
check_partition_health() {
    local bvns=$1
    local healthy=0
    local total=$((bvns + 1))  # +1 for directory
    
    # Check directory network
    local dn_port=$((BASE_PORT + 4))
    if curl -s --max-time 2 "http://127.0.0.1:$dn_port/v3" &>/dev/null; then
        healthy=$((healthy + 1))
        echo -e "  ${GREEN}✅ DN${NC} (Directory Network) - Online"
    else
        echo -e "  ${RED}❌ DN${NC} (Directory Network) - Offline"
    fi
    
    # Check BVNs
    if [ "$bvns" != "Unknown" ] && [ "$bvns" -gt 0 ]; then
        for i in $(seq 0 $((bvns-1))); do
            local bvn_port=$((BASE_PORT + 4 + ((i+1)*20)))  # Approximate port calculation
            if curl -s --max-time 2 "http://127.0.0.1:$bvn_port" &>/dev/null; then
                healthy=$((healthy + 1))
                echo -e "  ${GREEN}✅ BVN$i${NC} - Online"
            else
                echo -e "  ${RED}❌ BVN$i${NC} - Offline"  
            fi
        done
    fi
    
    return $healthy
}

# Get recent activity from logs
get_recent_activity() {
    local count=${1:-5}
    
    if [ ! -f "$SCRIPT_DIR/devnet.log" ]; then
        echo "  No logs available"
        return
    fi
    
    echo "  📊 Recent Activity:"
    tail -$count "$SCRIPT_DIR/devnet.log" | while read line; do
        # Color-code different types of events
        if echo "$line" | grep -q "conductor\|crosschain"; then
            echo -e "    ${CYAN}🔄${NC} $(echo "$line" | cut -c1-80)..."
        elif echo "$line" | grep -q "ERROR\|WARN"; then
            echo -e "    ${RED}⚠️${NC} $(echo "$line" | cut -c1-80)..."
        elif echo "$line" | grep -q "metrics\|throughput"; then
            echo -e "    ${GREEN}📈${NC} $(echo "$line" | cut -c1-80)..."
        elif echo "$line" | grep -q "partition\|BVN"; then
            echo -e "    ${BLUE}🏢${NC} $(echo "$line" | cut -c1-80)..."
        else
            echo -e "    ${NC}ℹ️${NC} $(echo "$line" | cut -c1-80)..."
        fi
    done
}

# Get performance metrics
get_performance_metrics() {
    if [ ! -f "$SCRIPT_DIR/devnet.log" ]; then
        echo "  No metrics available"
        return
    fi
    
    # Look for recent metrics in logs
    local temp_file=$(mktemp)
    tail -100 "$SCRIPT_DIR/devnet.log" | grep -E '"component":"devnet.metrics"' > "$temp_file" 2>/dev/null
    
    if [ -s "$temp_file" ]; then
        local latest=$(tail -1 "$temp_file")
        
        # Extract key metrics
        local throughput=$(echo "$latest" | grep -o '"throughput_per_second":[0-9.]*' | cut -d: -f2 | head -1)
        local success_rate=$(echo "$latest" | grep -o '"success_rate_percent":[0-9.]*' | cut -d: -f2 | head -1)
        local avg_latency=$(echo "$latest" | grep -o '"avg_latency_ms":[0-9.]*' | cut -d: -f2 | head -1)
        local gaps_detected=$(echo "$latest" | grep -o '"gaps_detected":[0-9]*' | cut -d: -f2 | head -1)
        
        if [ ! -z "$throughput" ]; then
            echo -e "  🚀 Throughput: ${GREEN}${throughput} req/s${NC}"
        fi
        if [ ! -z "$success_rate" ]; then
            echo -e "  ✅ Success Rate: ${GREEN}${success_rate}%${NC}"
        fi
        if [ ! -z "$avg_latency" ]; then
            echo -e "  ⏱️  Avg Latency: ${YELLOW}${avg_latency}ms${NC}"
        fi
        if [ ! -z "$gaps_detected" ] && [ "$gaps_detected" != "0" ]; then
            echo -e "  🔧 Gaps Detected: ${RED}${gaps_detected}${NC}"
        fi
    else
        echo "  No performance metrics available yet"
    fi
    
    rm -f "$temp_file"
}

# Main dashboard loop
create_monitoring_dashboard() {
    local refresh_interval=${1:-5}
    
    echo -e "${PURPLE}🚀 Accumulate DevNet Live Monitor${NC}"
    echo "=================================="
    echo "Refresh every ${refresh_interval}s (Ctrl+C to exit)"
    echo ""
    
    while true; do
        # Clear screen and show header
        clear
        echo -e "${PURPLE}🚀 Accumulate DevNet Live Monitor${NC}"
        echo "=================================="
        echo "Last updated: $(date)"
        echo ""
        
        # Check if devnet is running
        if check_devnet_running; then
            echo -e "${GREEN}📊 Network Status: ONLINE${NC}"
            
            # Get network info
            local info=$(get_devnet_info)
            local bvns=$(echo "$info" | cut -d'|' -f1)
            local validators=$(echo "$info" | cut -d'|' -f2)
            local network=$(echo "$info" | cut -d'|' -f3)
            
            echo "  Network: $network"
            echo "  BVNs: $bvns | Validators: $validators"
            echo ""
            
            # Partition Health
            echo -e "${BLUE}🏢 Partition Health:${NC}"
            if [ "$bvns" != "Unknown" ]; then
                check_partition_health "$bvns"
                local healthy=$?
                local total=$((bvns + 1))
                echo "  Overall: $healthy/$total partitions online"
            else
                echo "  Unable to determine partition status"
            fi
            echo ""
            
            # Performance Metrics
            echo -e "${GREEN}📈 Performance Metrics:${NC}"
            get_performance_metrics
            echo ""
            
            # Recent Activity
            echo -e "${CYAN}📋 Recent Activity:${NC}"
            get_recent_activity 5
            
        else
            echo -e "${RED}📊 Network Status: OFFLINE${NC}"
            echo ""
            echo "DevNet is not currently running."
            echo "Use './devnet_config.sh start' to launch DevNet"
        fi
        
        echo ""
        echo "=================================="
        echo "Press Ctrl+C to exit monitor"
        
        sleep $refresh_interval
    done
}

# Command interface
case "${1:-dashboard}" in
    "dashboard"|"monitor")
        create_monitoring_dashboard ${2:-5}
        ;;
    
    "status")
        echo "🔍 DevNet Status Check:"
        if check_devnet_running; then
            echo -e "${GREEN}✅ DevNet is running${NC}"
            local info=$(get_devnet_info)
            echo "Network info: $(echo "$info" | tr '|' ' | ')"
        else
            echo -e "${RED}❌ DevNet is not running${NC}"
        fi
        ;;
    
    "health")
        echo "🏥 DevNet Health Check:"
        if check_devnet_running; then
            local info=$(get_devnet_info)
            local bvns=$(echo "$info" | cut -d'|' -f1)
            check_partition_health "$bvns"
        else
            echo -e "${RED}❌ DevNet is not running${NC}"
        fi
        ;;
    
    "metrics")
        echo "📈 DevNet Performance Metrics:"
        get_performance_metrics
        ;;
    
    "activity")
        echo "📋 Recent DevNet Activity:"
        get_recent_activity ${2:-10}
        ;;
    
    "help"|"-h"|"--help")
        echo "🔧 DevNet Monitor - Real-time DevNet monitoring"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "Commands:"
        echo "  dashboard [interval]  - Live monitoring dashboard (default: 5s refresh)"
        echo "  status               - Quick status check"
        echo "  health               - Partition health check"
        echo "  metrics              - Current performance metrics"
        echo "  activity [count]     - Recent activity (default: 10 entries)"
        echo ""
        echo "Examples:"
        echo "  $0 dashboard         # Start live monitor"
        echo "  $0 dashboard 10      # Start with 10s refresh"
        echo "  $0 health            # Check partition health"
        echo "  $0 activity 20       # Show last 20 activities"
        ;;
    
    *)
        echo "❌ Unknown command: $1"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac