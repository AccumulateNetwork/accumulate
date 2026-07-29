#!/bin/bash
# Advanced test script to find and display ADIs from the database

MCP_BIN="./mcp-accumulate"

echo "=== Advanced ADI Search ==="
echo ""

# Function to query and check if account is an ADI (identity account)
check_if_adi() {
    local url="$1"
    local response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$url\"}}}" | $MCP_BIN 2>/dev/null)

    # Check if account type is identity
    local type=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

    if [ "$type" == "identity" ]; then
        echo "$url|$response"
        return 0
    fi
    return 1
}

# Strategy: Search across multiple databases with larger limits
databases=("2025-07-13-bvn0" "2025-07-13-bvn1" "2025-07-13-bvn2" "2025-07-13-dn")

echo "Searching for ADI (identity) accounts across databases..."
echo ""

found_count=0
checked_count=0

for db in "${databases[@]}"; do
    if [ $found_count -ge 5 ]; then
        break
    fi

    echo "Checking database: $db"

    # Get accounts from database
    response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_list_accounts\",\"arguments\":{\"database\":\"$db\",\"limit\":100}}}" | $MCP_BIN 2>/dev/null)

    # Get all accounts ending with .acme that aren't system accounts
    accounts=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[] | select(endswith(".acme") and (contains("/ledger/") | not) and (contains("dn.acme") | not) and (startswith("acc://bvn-") | not))' 2>/dev/null)

    # Check each account to see if it's an identity
    while IFS= read -r account; do
        if [ $found_count -ge 5 ]; then
            break 2
        fi

        if [ ! -z "$account" ]; then
            checked_count=$((checked_count + 1))

            # Query the account to check its type
            acc_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$account\"}}}" | $MCP_BIN 2>/dev/null)

            acc_type=$(echo "$acc_response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

            if [ "$acc_type" == "identity" ]; then
                found_count=$((found_count + 1))

                echo ""
                echo "=== ADI #$found_count: $account ==="
                echo ""
                echo "$acc_response" | jq -r '.result.content[0].text | fromjson' | jq '{
                    type,
                    url,
                    keyBook,
                    authorities: .authorities[]?.url
                }'
                echo ""
            fi
        fi
    done <<< "$accounts"
done

echo ""
echo "==========="
echo "Search Summary:"
echo "  Databases checked: ${#databases[@]}"
echo "  Accounts checked: $checked_count"
echo "  ADIs found: $found_count"
echo ""

if [ $found_count -eq 0 ]; then
    echo "No identity accounts found. Let's check what account types exist..."
    echo ""

    # Sample some accounts and show their types
    echo "Sample account types from 2025-07-13-dn:"
    response=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_db_list_accounts","arguments":{"database":"2025-07-13-dn","limit":10}}}' | $MCP_BIN 2>/dev/null)

    accounts=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[]' 2>/dev/null)

    while IFS= read -r account; do
        if [ ! -z "$account" ]; then
            acc_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$account\"}}}" | $MCP_BIN 2>/dev/null)
            acc_type=$(echo "$acc_response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)
            echo "  $account -> $acc_type"
        fi
    done <<< "$accounts"
fi

echo ""
echo "=== Test Complete ==="
