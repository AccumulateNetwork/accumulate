#!/bin/bash
# Test complete ADI creation workflow

set -e

echo "=== Testing ADI Creation Workflow ==="
echo

# Step 1: Generate key for funding account
echo "Step 1: Generating key for funding account..."
KEYGEN_OUTPUT=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_generate_key"}}' | ./mcp-accumulate 2>&1)
echo "$KEYGEN_OUTPUT" | grep "liteAccountURL"

LITE_ACCOUNT=$(echo "$KEYGEN_OUTPUT" | jq -r '.result.content[0].text' | jq -r '.liteAccountURL')
PRIVATE_KEY=$(echo "$KEYGEN_OUTPUT" | jq -r '.result.content[0].text' | jq -r '.privateKey')
PUBLIC_KEY=$(echo "$KEYGEN_OUTPUT" | jq -r '.result.content[0].text' | jq -r '.publicKey')

echo "  Lite Account: $LITE_ACCOUNT"
echo "  Public Key: $PUBLIC_KEY"
echo

# Step 2: Fund the lite account using faucet
echo "Step 2: Funding lite account from faucet..."
for i in {1..10}; do
    echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_faucet","arguments":{"url":"'"$LITE_ACCOUNT"'","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate 2>&1 > /dev/null
    echo "  Faucet call $i/10"
    sleep 1
done
echo "  Waiting for balance to settle..."
sleep 5
echo

# Step 3: Check balance
echo "Step 3: Checking balance..."
BALANCE_OUTPUT=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_query_account","arguments":{"url":"'"$LITE_ACCOUNT"'","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate 2>&1)
echo "$BALANCE_OUTPUT" | grep -i "balance" | head -3
echo

# Step 4: Add credits to the lite account
echo "Step 4: Adding credits to lite account..."
CREDITS_OUTPUT=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_add_credits","arguments":{"recipient":"'"$LITE_ACCOUNT"'","payer":"'"$LITE_ACCOUNT"'","amount":"100000000","private_key":"'"$PRIVATE_KEY"'","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate 2>&1)
echo "$CREDITS_OUTPUT" | grep "txHash"
echo "  Waiting for credits transaction..."
sleep 5
echo

# Step 5: Generate key for ADI
echo "Step 5: Generating key for ADI..."
ADI_KEYGEN=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_generate_key"}}' | ./mcp-accumulate 2>&1)
ADI_PUBLIC_KEY=$(echo "$ADI_KEYGEN" | jq -r '.result.content[0].text' | jq -r '.publicKey')
echo "  ADI Public Key: $ADI_PUBLIC_KEY"
echo

# Step 6: Create ADI
ADI_URL="acc://test-adi-$(date +%s).acme"
echo "Step 6: Creating ADI: $ADI_URL"
CREATE_ADI_OUTPUT=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_create_adi","arguments":{"url":"'"$ADI_URL"'","public_key":"'"$ADI_PUBLIC_KEY"'","sponsor":"'"$LITE_ACCOUNT"'","sponsor_private_key":"'"$PRIVATE_KEY"'","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate 2>&1)
echo "$CREATE_ADI_OUTPUT" | grep -E "(txHash|adiURL|keyBookURL)"
echo "  Waiting for ADI creation..."
sleep 5
echo

# Step 7: Query the ADI's KeyBook
KEYBOOK_URL="$ADI_URL/book"
echo "Step 7: Querying ADI KeyBook: $KEYBOOK_URL"
KEYBOOK_OUTPUT=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_query_keybook","arguments":{"url":"'"$KEYBOOK_URL"'","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate 2>&1)
echo "$KEYBOOK_OUTPUT" | grep -E "(pageCount|type)" | head -5

echo
echo "=== Workflow Complete ==="
echo "ADI URL: $ADI_URL"
echo "KeyBook URL: $KEYBOOK_URL"
echo "KeyPage URL: $ADI_URL/book/1"
