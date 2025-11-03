#!/bin/bash
# Final test to find 5 ADIs from databases

MCP_BIN="./mcp-accumulate"

echo "=== Finding 5 ADIs from Accumulate Databases ==="
echo ""

# List of known system ADIs and databases to try
known_adis=(
    "acc://staking.acme"
    "acc://marketplace.acme"
)

found_adis=()

# First, try known system ADIs
echo "Step 1: Checking known system ADIs..."
echo "========================================"
echo ""

for adi in "${known_adis[@]}"; do
    if [ ${#found_adis[@]} -ge 5 ]; then
        break
    fi

    echo "Querying: $adi"

    response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$adi\"}}}" | $MCP_BIN 2>/dev/null)

    error=$(echo "$response" | jq -r '.result.isError // false' 2>/dev/null)

    if [ "$error" == "false" ]; then
        acc_type=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

        if [ "$acc_type" == "identity" ] || [ "$acc_type" == "keyBook" ] || [ ! -z "$acc_type" ]; then
            found_adis+=("$adi")

            echo ""
            echo "✓ FOUND #${#found_adis[@]}: $adi"
            echo ""
            echo "$response" | jq -r '.result.content[0].text | fromjson' | jq '{type, url, keyBook, authorities}' | sed 's/^/  /'
            echo ""
            echo "---"
            echo ""
        fi
    else
        echo "  Not found in database"
        echo ""
    fi
done

# Step 2: Search historical database for user ADIs
echo ""
echo "Step 2: Searching 2024-03-31-bvn0-historical for user ADIs..."
echo "=============================================================="
echo ""

if [ ${#found_adis[@]} -lt 5 ]; then
    response=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_db_list_accounts","arguments":{"database":"2024-03-31-bvn0-historical","limit":200}}}' | $MCP_BIN 2>/dev/null)

    accounts=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[]' 2>/dev/null | grep -E '\.acme$' | grep -v '/ledger/' | grep -v '^acc://dn\.acme' | grep -v '^acc://bvn-')

    while IFS= read -r account; do
        if [ -z "$account" ]; then
            continue
        fi

        if [ ${#found_adis[@]} -ge 5 ]; then
            break
        fi

        # Skip if already found
        skip=false
        for found in "${found_adis[@]}"; do
            if [ "$found" == "$account" ]; then
                skip=true
                break
            fi
        done

        if [ "$skip" == "true" ]; then
            continue
        fi

        acc_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$account\",\"database\":\"2024-03-31-bvn0-historical\"}}}" | $MCP_BIN 2>/dev/null)

        acc_type=$(echo "$acc_response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

        echo "Checking: $account -> $acc_type"

        if [ "$acc_type" == "identity" ]; then
            found_adis+=("$account")

            echo ""
            echo "✓ FOUND #${#found_adis[@]}: $account"
            echo ""
            echo "$acc_response" | jq -r '.result.content[0].text | fromjson' | jq '{type, url, keyBook, authorities}' | sed 's/^/  /'
            echo ""
            echo "---"
            echo ""
        fi
    done <<< "$accounts"
fi

# Step 3: Try checking sub-accounts of marketplace.acme if we need more
if [ ${#found_adis[@]} -lt 5 ]; then
    echo ""
    echo "Step 3: Checking for additional accounts..."
    echo "==========================================="
    echo ""

    # Try a few possible ADI variations
    possible_adis=(
        "acc://acme.acme"
        "acc://accumulate.acme"
        "acc://factom.acme"
    )

    for adi in "${possible_adis[@]}"; do
        if [ ${#found_adis[@]} -ge 5 ]; then
            break
        fi

        # Skip if already found
        skip=false
        for found in "${found_adis[@]}"; do
            if [ "$found" == "$adi" ]; then
                skip=true
                break
            fi
        done

        if [ "$skip" == "true" ]; then
            continue
        fi

        echo "Trying: $adi"

        response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$adi\"}}}" | $MCP_BIN 2>/dev/null)

        error=$(echo "$response" | jq -r '.result.isError // false' 2>/dev/null)

        if [ "$error" == "false" ]; then
            acc_type=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

            if [ "$acc_type" == "identity" ]; then
                found_adis+=("$adi")

                echo ""
                echo "✓ FOUND #${#found_adis[@]}: $adi"
                echo ""
                echo "$response" | jq -r '.result.content[0].text | fromjson' | jq '{type, url, keyBook, authorities}' | sed 's/^/  /'
                echo ""
                echo "---"
                echo ""
            fi
        fi
    done
fi

echo ""
echo "============================================="
echo "FINAL SUMMARY"
echo "============================================="
echo ""
echo "Total ADIs found: ${#found_adis[@]}"
echo ""

if [ ${#found_adis[@]} -gt 0 ]; then
    echo "Found ADIs:"
    count=1
    for adi in "${found_adis[@]}"; do
        echo "  $count. $adi"
        count=$((count + 1))
    done
fi

echo ""
echo "=== Test Complete ==="
