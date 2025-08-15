#!/bin/bash

# Minimal test to get tokens moving
set -e

URL="http://127.0.0.1:27004/v2"

# Two test accounts
ACCOUNT1="acc://17d69fe619cd40ebc7b23396fc2ef6e56e8e406abd517c93/ACME"
ACCOUNT2="acc://9bbb1c97bb099e36a1fffd26fad9ccc8388160f7d94f17a7/ACME"

echo "=== Minimal Token Transfer Test ==="

# Faucet both accounts
echo "1. Getting tokens from faucet..."
curl -s -X POST $URL -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"faucet\",\"params\":{\"url\":\"$ACCOUNT1\"},\"id\":1}" > /dev/null

curl -s -X POST $URL -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"faucet\",\"params\":{\"url\":\"$ACCOUNT2\"},\"id\":2}" > /dev/null

sleep 2

# Check balances
echo "2. Checking balances..."
BAL1=$(curl -s -X POST $URL -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"query\",\"params\":{\"url\":\"$ACCOUNT1\"},\"id\":1}" | \
  jq -r '.result.data.balance // 0')

BAL2=$(curl -s -X POST $URL -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"query\",\"params\":{\"url\":\"$ACCOUNT2\"},\"id\":2}" | \
  jq -r '.result.data.balance // 0')

echo "Account 1 balance: $BAL1"
echo "Account 2 balance: $BAL2"

# Simple transfer using execute
echo "3. Attempting token transfer..."
RESULT=$(curl -s -X POST $URL -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"execute\",\"params\":{\"from\":\"$ACCOUNT1\",\"to\":\"$ACCOUNT2\",\"amount\":1000000000},\"id\":3}")

echo "Transfer result: $RESULT" | jq -r '.error.message // "Success"'

echo "=== Test Complete ===""