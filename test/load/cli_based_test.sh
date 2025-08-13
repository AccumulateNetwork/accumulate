#!/bin/bash

set -e

echo "=== CLI-Based ADI and Token Test ==="

# Configuration
API_URL="http://127.0.0.1:27004/v2"
CLI="/home/paul/go/bin/accumulate"

# Test account (using the one we know has balance)
LITE_ACCOUNT="acc://17d69fe619cd40ebc7b23396fc2ef6e56e8e406abd517c93/ACME"
ADI_NAME="test-adi-cli-$(date +%s)"

echo "Using Lite Account: $LITE_ACCOUNT"
echo "Creating ADI: $ADI_NAME"

# Step 1: Check balance
echo -e "\n--- Step 1: Checking Lite Account Balance ---"
BALANCE=$($CLI -j $API_URL account get $LITE_ACCOUNT | jq -r '.data.balance // 0')
echo "Current balance: $BALANCE"

if [ "$BALANCE" -eq "0" ]; then
    echo "No balance, requesting from faucet..."
    curl -s -X POST $API_URL -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"faucet\",\"params\":{\"url\":\"$LITE_ACCOUNT\"},\"id\":1}" > /dev/null
    sleep 5
    BALANCE=$($CLI -j $API_URL account get $LITE_ACCOUNT | jq -r '.data.balance // 0')
    echo "New balance: $BALANCE"
fi

# Step 2: Create ADI
echo -e "\n--- Step 2: Creating ADI ---"
# Generate a key for the ADI
KEY_FILE="/tmp/test-key-$$.pem"
$CLI key generate $KEY_FILE

# Extract public key hash
PUBLIC_KEY=$($CLI key export $KEY_FILE | jq -r '.publicKey')

# Create ADI using CLI
echo "Creating ADI: acc://$ADI_NAME"
$CLI -j $API_URL adi create acc://$ADI_NAME $PUBLIC_KEY $LITE_ACCOUNT \
    --signing-key $KEY_FILE --wait 5s 2>/dev/null || echo "ADI creation submitted"

sleep 5

# Step 3: Check if ADI exists
echo -e "\n--- Step 3: Checking ADI Status ---"
$CLI -j $API_URL account get acc://$ADI_NAME 2>/dev/null && echo "✓ ADI created successfully" || echo "✗ ADI not found"

# Step 4: Create token account
echo -e "\n--- Step 4: Creating Token Account ---"
$CLI -j $API_URL account create token acc://$ADI_NAME/tokens acc://ACME acc://$ADI_NAME/book \
    --signing-key $KEY_FILE --wait 5s 2>/dev/null || echo "Token account creation submitted"

sleep 5

# Step 5: Check token account
echo -e "\n--- Step 5: Checking Token Account ---"
$CLI -j $API_URL account get acc://$ADI_NAME/tokens 2>/dev/null && echo "✓ Token account created" || echo "✗ Token account not found"

# Step 6: Send tokens
echo -e "\n--- Step 6: Sending Tokens ---"
$CLI -j $API_URL tx create $LITE_ACCOUNT acc://$ADI_NAME/tokens 1.0 \
    --signing-key $KEY_FILE --wait 5s 2>/dev/null || echo "Token transfer submitted"

sleep 5

# Final check
echo -e "\n--- Final Status ---"
TOKEN_BALANCE=$($CLI -j $API_URL account get acc://$ADI_NAME/tokens 2>/dev/null | jq -r '.data.balance // 0')
echo "Token account balance: $TOKEN_BALANCE"

if [ "$TOKEN_BALANCE" -gt "0" ]; then
    echo "✅ SUCCESS: Tokens moved to ADI token account!"
else
    echo "⚠️ Token balance is still 0"
fi

# Cleanup
rm -f $KEY_FILE

echo -e "\n=== Test Complete ===""