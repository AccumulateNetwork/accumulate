#!/bin/bash
# Test script to find and display ADIs from the database

MCP_BIN="./mcp-accumulate"

echo "=== Finding ADIs in Database ==="
echo ""

# Query more accounts to find ADIs (increase limit to 500)
echo "Searching for ADIs in 2025-07-13-dn database..."
echo ""

# Get accounts and filter for ADIs (accounts ending in .acme)
response=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_db_list_accounts","arguments":{"database":"2025-07-13-dn","limit":500}}}' | $MCP_BIN 2>/dev/null)

# Extract accounts array and filter for ADIs
adis=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[] | select(endswith(".acme") and (. | test("^acc://[a-zA-Z0-9-]+\\.acme$")))' 2>/dev/null | head -5)

if [ -z "$adis" ]; then
    echo "No ADIs found in first 500 accounts. Trying BVN0..."
    echo ""

    # Try BVN0 database
    response=$(echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"accumulate_db_list_accounts","arguments":{"database":"2025-07-13-bvn0","limit":500}}}' | $MCP_BIN 2>/dev/null)

    adis=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[] | select(endswith(".acme") and (. | test("^acc://[a-zA-Z0-9-]+\\.acme$")))' 2>/dev/null | head -5)
fi

echo "Found ADIs:"
echo "==========="
echo ""

count=0
while IFS= read -r adi; do
    if [ ! -z "$adi" ]; then
        count=$((count + 1))
        echo "$count. $adi"
        echo ""

        # Query each ADI to get details
        echo "   Details:"
        details=$(echo "{\"jsonrpc\":\"2.0\",\"id\":$count,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$adi\"}}}" | $MCP_BIN 2>/dev/null | jq -r '.result.content[0].text | fromjson')

        # Extract and display key information
        type=$(echo "$details" | jq -r '.type // "unknown"')
        authorities=$(echo "$details" | jq -r '.authorities[]?.url // empty' | head -1)

        echo "   Type: $type"
        if [ ! -z "$authorities" ]; then
            echo "   Authority: $authorities"
        fi

        # For identity accounts, show additional info
        if [ "$type" == "identity" ]; then
            keybook=$(echo "$details" | jq -r '.keyBook // empty')
            if [ ! -z "$keybook" ] && [ "$keybook" != "null" ]; then
                echo "   KeyBook: $keybook"
            fi
        fi

        echo ""
    fi
done <<< "$adis"

if [ $count -eq 0 ]; then
    echo "No ADIs found. The database may contain mostly lite accounts or system accounts."
    echo ""
    echo "Showing some example accounts instead:"
    echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[0:5][]'
else
    echo "==========="
    echo "Total ADIs found: $count"
fi

echo ""
echo "=== Test Complete ==="
