#!/bin/bash

# Enhanced DevNet Load Test Script with Logging Integration
# Load testing with structured logging and performance correlation

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEVNET_URL="${1:-http://127.0.0.1:26660/v2}"
NUM_REQUESTS="${2:-50}"
CONCURRENT_WORKERS="${3:-5}"

# Test correlation ID for log analysis
TEST_ID="load_test_$(date +%s)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}DevNet Enhanced Load Test${NC}"
echo "=========================="
echo "Target: $DEVNET_URL"
echo "Requests: $NUM_REQUESTS"
echo "Workers: $CONCURRENT_WORKERS"
echo "Test ID: $TEST_ID"
echo

# Log test start marker
log_test_event() {
    local event_type=$1
    local details=$2
    local timestamp=$(date -Iseconds)
    
    # If devnet.log exists, append structured test events
    if [ -f "$SCRIPT_DIR/devnet.log" ]; then
        echo "{\"timestamp\":\"$timestamp\",\"component\":\"devnet.load_test\",\"test_id\":\"$TEST_ID\",\"event\":\"$event_type\",\"details\":\"$details\"}" >> "$SCRIPT_DIR/devnet.log"
    fi
    
    echo -e "${YELLOW}[TEST_EVENT]${NC} $event_type: $details"
}

# Test DevNet connectivity first
echo "Testing DevNet connectivity..."
log_test_event "test_start" "Starting load test with $NUM_REQUESTS requests, $CONCURRENT_WORKERS workers"

response=$(curl -s --max-time 10 -X POST "$DEVNET_URL" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}')

if echo "$response" | grep -q '"result"'; then
    echo -e "${GREEN}✓ DevNet is accessible${NC}"
    log_test_event "connectivity_check" "DevNet API responding correctly"
else
    echo -e "${RED}✗ DevNet is not accessible${NC}"
    echo "Response: $response"
    log_test_event "connectivity_error" "DevNet API not responding"
    exit 1
fi

# Function to send a single query request
send_query() {
    local worker_id=$1
    local request_id=$2
    
    # Send a query request to the DevNet
    response=$(curl -s -X POST "$DEVNET_URL" \
      -H "Content-Type: application/json" \
      -d "{\"jsonrpc\":\"2.0\",\"method\":\"query\",\"params\":{\"url\":\"acc://dn.acme\"},\"id\":$request_id}" \
      -w "%{time_total}")
    
    # Extract timing info (last line)
    time_total=$(echo "$response" | tail -n1)
    response_body=$(echo "$response" | head -n -1)
    
    if echo "$response_body" | grep -q '"result"'; then
        echo "Worker $worker_id, Request $request_id: SUCCESS (${time_total}s)"
        return 0
    else
        echo "Worker $worker_id, Request $request_id: FAILED"
        return 1
    fi
}

# Function to send describe requests (lighter load)
send_describe() {
    local worker_id=$1
    local request_id=$2
    
    response=$(curl -s -X POST "$DEVNET_URL" \
      -H "Content-Type: application/json" \
      -d "{\"jsonrpc\":\"2.0\",\"method\":\"describe\",\"params\":{},\"id\":$request_id}" \
      -w "%{time_total}")
    
    time_total=$(echo "$response" | tail -n1)
    response_body=$(echo "$response" | head -n -1)
    
    if echo "$response_body" | grep -q '"result"'; then
        echo "Worker $worker_id, Request $request_id: SUCCESS (${time_total}s)"
        return 0
    else
        echo "Worker $worker_id, Request $request_id: FAILED"
        return 1
    fi
}

# Function to run worker
run_worker() {
    local worker_id=$1
    local requests_per_worker=$2
    local success_count=0
    
    echo "Starting worker $worker_id with $requests_per_worker requests"
    
    for ((i=1; i<=requests_per_worker; i++)); do
        request_id=$((worker_id * 1000 + i))
        
        # Alternate between query and describe requests
        if ((i % 2 == 0)); then
            send_describe $worker_id $request_id && ((success_count++))
        else
            send_query $worker_id $request_id && ((success_count++))
        fi
        
        # Small delay between requests
        sleep 0.1
    done
    
    echo "Worker $worker_id completed: $success_count/$requests_per_worker successful"
    echo $success_count > "/tmp/worker_${worker_id}_results"
}

# Calculate requests per worker
requests_per_worker=$((NUM_REQUESTS / CONCURRENT_WORKERS))
remaining_requests=$((NUM_REQUESTS % CONCURRENT_WORKERS))

echo "Starting load test..."
echo "Requests per worker: $requests_per_worker"
if [ $remaining_requests -gt 0 ]; then
    echo "Extra requests for first worker: $remaining_requests"
fi
echo

# Record start time
start_time=$(date +%s.%N)

# Start workers in background
pids=()
for ((worker=1; worker<=CONCURRENT_WORKERS; worker++)); do
    worker_requests=$requests_per_worker
    if [ $worker -eq 1 ]; then
        worker_requests=$((requests_per_worker + remaining_requests))
    fi
    
    run_worker $worker $worker_requests &
    pids+=($!)
done

# Wait for all workers to complete
echo "Waiting for workers to complete..."
for pid in "${pids[@]}"; do
    wait $pid
done

# Record end time
end_time=$(date +%s.%N)

# Calculate results
total_time=$(echo "$end_time - $start_time" | bc -l)
total_success=0

for ((worker=1; worker<=CONCURRENT_WORKERS; worker++)); do
    if [ -f "/tmp/worker_${worker}_results" ]; then
        worker_success=$(cat "/tmp/worker_${worker}_results")
        total_success=$((total_success + worker_success))
        rm "/tmp/worker_${worker}_results"
    fi
done

# Calculate metrics
tps=$(echo "scale=2; $total_success / $total_time" | bc -l)
success_rate=$(echo "scale=2; $total_success * 100 / $NUM_REQUESTS" | bc -l)
failure_rate=$((NUM_REQUESTS - total_success))

echo
echo -e "${BLUE}Load Test Results${NC}"
echo "================="
echo "Total requests: $NUM_REQUESTS"
echo -e "Successful requests: ${GREEN}$total_success${NC}"
echo -e "Failed requests: ${RED}$failure_rate${NC}"
echo -e "Success rate: ${GREEN}${success_rate}%${NC}"
echo "Total time: ${total_time}s"
echo -e "Requests per second: ${YELLOW}$tps${NC}"

# Log test completion
log_test_event "test_complete" "Completed $NUM_REQUESTS requests: $total_success successful ($success_rate%), ${tps} req/s"

# Analyze DevNet logs during test period if available
echo
echo -e "${BLUE}DevNet Performance During Test:${NC}"
if [ -f "$SCRIPT_DIR/devnet.log" ]; then
    echo "📊 Analyzing DevNet activity during load test..."
    
    # Look for conductor activity during test period
    conductor_activity=$(grep -c "$TEST_ID\|conductor\|crosschain" "$SCRIPT_DIR/devnet.log" 2>/dev/null || echo 0)
    if [ $conductor_activity -gt 0 ]; then
        echo -e "  ${CYAN}CrossChain Conductor activity: $conductor_activity events${NC}"
    fi
    
    # Look for errors during test
    error_count=$(tail -100 "$SCRIPT_DIR/devnet.log" | grep -c "ERROR\|WARN\|failed" 2>/dev/null || echo 0)
    if [ $error_count -gt 0 ]; then
        echo -e "  ${RED}Errors/warnings during test: $error_count${NC}"
        echo "  Recent errors:"
        tail -50 "$SCRIPT_DIR/devnet.log" | grep "ERROR\|WARN" | tail -3 | sed 's/^/    /'
    else
        echo -e "  ${GREEN}No errors detected during test${NC}"
    fi
    
    # Look for recent metrics
    metrics_found=$(tail -100 "$SCRIPT_DIR/devnet.log" | grep -c "devnet.metrics" 2>/dev/null || echo 0)
    if [ $metrics_found -gt 0 ]; then
        echo "  📈 Recent performance metrics available in logs"
        echo "     Use './devnet_config.sh metrics' to view detailed metrics"
    fi
else
    echo "  ⚠️  No DevNet logs available for analysis"
fi

echo
if [ $total_success -eq $NUM_REQUESTS ]; then
    echo -e "${GREEN}✅ All requests successful!${NC}"
    log_test_event "test_result" "SUCCESS - All requests completed successfully"
    exit 0
else
    echo -e "${RED}⚠️  Some requests failed${NC}"
    log_test_event "test_result" "PARTIAL_SUCCESS - $failure_rate requests failed"
    
    echo
    echo "Troubleshooting tips:"
    echo "1. Check DevNet status: ./devnet_config.sh status"
    echo "2. View recent logs: ./devnet_config.sh logs errors"
    echo "3. Monitor in real-time: ./devnet_config.sh monitor"
    exit 1
fi
