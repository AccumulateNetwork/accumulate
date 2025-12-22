#!/bin/bash
# Extended test to find more ADIs by searching multiple databases

MCP_BIN="./mcp-accumulate"

echo "=== Finding ADIs across Multiple Databases ==="
echo ""

found_adis=()

# Function to check and add ADI
check_and_add_adi() {
    local url="$1"
    local db="$2"

    # Skip if already found
    for found in "${found_adis[@]}"; do
        if [ "$found" == "$url" ]; then
            return
        fi
    done

    if [ ${#found_adis[@]} -ge 5 ]; then
        return
    fi

    local response
    if [ -z "$db" ]; then
        response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$url\"}}}" | $MCP_BIN 2>/dev/null)
    else
        response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$url\",\"database\":\"$db\"}}}" | $MCP_BIN 2>/dev/null)
    fi

    local error=$(echo "$response" | jq -r '.result.isError // false' 2>/dev/null)

    if [ "$error" == "false" ]; then
        local acc_type=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

        if [ "$acc_type" == "identity" ]; then
            found_adis+=("$url")

            echo ""
            echo "✓ ADI #${#found_adis[@]}: $url"
            echo ""
            echo "$response" | jq -r '.result.content[0].text | fromjson' | jq '{type, url, keyBook, authorities: [.authorities[]? | {url: .url, disabled: (.disabled // false)}]}' | sed 's/^/  /'
            echo ""
            echo "---"
        fi
    fi
}

# Try multiple databases
databases=(
    "2024-03-31-bvn0-historical"
    "2025-07-13-bvn1"
    "2025-07-13-bvn2"
    "2025-10-22-bvn1"
    "2025-10-22-bvn2"
)

echo "Searching across databases..."
echo ""

for db in "${databases[@]}"; do
    if [ ${#found_adis[@]} -ge 5 ]; then
        break
    fi

    echo "Database: $db"

    response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_list_accounts\",\"arguments\":{\"database\":\"$db\",\"limit\":100}}}" | $MCP_BIN 2>/dev/null)

    accounts=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[]' 2>/dev/null | grep -E '\.acme$' | grep -v '/ledger/' | grep -v '^acc://dn\.acme' | grep -v '^acc://bvn-')

    while IFS= read -r account; do
        if [ -z "$account" ]; then
            continue
        fi

        if [ ${#found_adis[@]} -ge 5 ]; then
            break
        fi

        check_and_add_adi "$account" "$db"
    done <<< "$accounts"

    echo "  Found ${#found_adis[@]} ADIs so far"
    echo ""
done

# Also check system ADIs without database specification (auto-routing)
echo ""
echo "Checking system ADIs with auto-routing..."
echo ""

system_adis=(
    "acc://staking.acme"
    "acc://dn.acme"
)

for adi in "${system_adis[@]}"; do
    check_and_add_adi "$adi" ""
done

echo ""
echo "============================================="
echo "FINAL RESULTS"
echo "============================================="
echo ""
echo "Total ADIs found: ${#found_adis[@]}"
echo ""

if [ ${#found_adis[@]} -gt 0 ]; then
    echo "ADIs:"
    count=1
    for adi in "${found_adis[@]}"; do
        echo "  $count. $adi"
        count=$((count + 1))
    done
else
    echo "No ADIs found."
fi

echo ""
echo "=== Test Complete ==="
