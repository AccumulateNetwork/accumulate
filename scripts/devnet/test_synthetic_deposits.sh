#!/bin/bash

echo "🚀 Testing Synthetic Transaction Deposits"
echo "========================================="

# Base URL for devnet
BASE_URL="http://localhost:27004"

# Known test account
TEST_ACCOUNT="acc://8e9490b0be1f5ba3d08edd6a64d8a61a59ef736b5af55cf9/ACME"

echo ""
echo "1️⃣ Checking initial balance of test account..."
curl -s -X POST $BASE_URL/v3 \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"query\",\"params\":{\"url\":\"$TEST_ACCOUNT\"}}" | \
  jq -r '.result.account.balance // "Error"' | \
  xargs -I {} echo "   Initial balance: {} credits"

echo ""
echo "2️⃣ Running balance checker in background..."
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate2/accumulate/test/load
go run check_balances.go -interval 1s &
CHECKER_PID=$!

echo ""
echo "3️⃣ Generating transactions that should create synthetic deposits..."
echo "   (Transactions between different partitions create synthetics)"

# Give the balance checker time to start
sleep 2

# Generate some test transactions using the load test endpoint
echo ""
echo "4️⃣ Triggering test transactions via load test API..."
curl -s -X POST http://localhost:8086/api/test \
  -H "Content-Type: application/json" \
  -d '{}' | jq -r '.message // "No response"'

echo ""
echo "5️⃣ Waiting for synthetic transactions to be processed..."
sleep 10

echo ""
echo "6️⃣ Checking final balance..."
curl -s -X POST $BASE_URL/v3 \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"query\",\"params\":{\"url\":\"$TEST_ACCOUNT\"}}" | \
  jq -r '.result.account.balance // "Error"' | \
  xargs -I {} echo "   Final balance: {} credits"

echo ""
echo "7️⃣ Stopping balance checker..."
kill $CHECKER_PID 2>/dev/null

echo ""
echo "✅ Test complete!"
echo ""
echo "📊 Summary:"
echo "   - If balance changed, synthetic deposits were detected"
echo "   - Check the load test dashboard at http://localhost:8086"
echo "   - Monitor gap recovery when pausing/resuming partitions"