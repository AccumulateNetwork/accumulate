#!/bin/bash

# DevNet Load Test Script
# Simple load testing using curl to send JSON-RPC requests

DEVNET_URL="${1:-http://127.0.0.1:26660/v2}"
NUM_REQUESTS="${2:-50}"
CONCURRENT_WORKERS="${3:-5}"

echo "DevNet Load Test"
echo "================"
echo "Target: $DEVNET_URL"
echo "Requests: $NUM_REQUESTS"
echo "Workers: $CONCURRENT_WORKERS"
echo

# Test DevNet connectivity first
echo "Testing DevNet connectivity..."
response=$(curl -s -X POST "$DEVNET_URL" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}')

if echo "$response" | grep -q '"result"'; then
    echo "✓ DevNet is accessible"
else
    echo "✗ DevNet is not accessible"
    echo "Response: $response"
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

# Calculate TPS
tps=$(echo "scale=2; $total_success / $total_time" | bc -l)

echo
echo "Load Test Results"
echo "================="
echo "Total requests: $NUM_REQUESTS"
echo "Successful requests: $total_success"
echo "Failed requests: $((NUM_REQUESTS - total_success))"
echo "Success rate: $(echo "scale=2; $total_success * 100 / $NUM_REQUESTS" | bc -l)%"
echo "Total time: ${total_time}s"
echo "Requests per second: $tps"
echo

if [ $total_success -eq $NUM_REQUESTS ]; then
    echo "✓ All requests successful!"
    exit 0
else
    echo "⚠ Some requests failed"
    exit 1
fi
