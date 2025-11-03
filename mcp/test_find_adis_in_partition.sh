#!/bin/bash
# Test to find ADIs from a specific partition database

MCP_BIN="./mcp-accumulate"
DATABASE="2024-03-31-bvn0-historical"  # Choose a specific partition database
LIMIT=2000                   # Get more accounts to search through

echo "=== Finding ADIs in $DATABASE ==="
echo ""
echo "Listing accounts from database..."

# Get accounts from the database
response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_list_accounts\",\"arguments\":{\"database\":\"$DATABASE\",\"limit\":$LIMIT}}}" | $MCP_BIN 2>/dev/null)

# Extract all accounts
all_accounts=$(echo "$response" | jq -r '.result.content[0].text | fromjson | .accounts[]' 2>/dev/null)

total_accounts=$(echo "$all_accounts" | wc -l)
echo "Found $total_accounts accounts in database"
echo ""

# Filter for potential ADIs (ending in .acme, not ledger accounts, not system BVN accounts)
echo "Filtering for potential ADI accounts..."
potential_adis=$(echo "$all_accounts" | grep -E '\.acme$' | grep -v '/ledger/' | grep -v '^acc://dn\.acme' | grep -v '^acc://bvn-')

potential_count=$(echo "$potential_adis" | grep -v '^$' | wc -l)
echo "Found $potential_count potential ADI accounts"
echo ""

echo "Checking account types to find identity accounts..."
echo "==========================================="
echo ""

found_count=0
checked=0

while IFS= read -r account; do
    if [ -z "$account" ]; then
        continue
    fi

    if [ $found_count -ge 5 ]; then
        break
    fi

    checked=$((checked + 1))

    # Query the account to check its type
    acc_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$account\",\"database\":\"$DATABASE\"}}}" | $MCP_BIN 2>/dev/null)

    acc_type=$(echo "$acc_response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

    echo "[$checked] $account -> $acc_type"

    if [ "$acc_type" == "identity" ]; then
        found_count=$((found_count + 1))

        echo ""
        echo "    ✓ FOUND ADI #$found_count!"
        echo ""

        # Display full details
        echo "$acc_response" | jq -r '.result.content[0].text | fromjson' | jq '{
            type,
            url,
            keyBook,
            authorities: [.authorities[]? | {url: .url, disabled: (.disabled // false)}]
        }' | sed 's/^/    /'

        echo ""
        echo "---"
        echo ""
    fi
done <<< "$potential_adis"

echo ""
echo "==========================================="
echo "Search Summary:"
echo "  Database: $DATABASE"
echo "  Total accounts: $total_accounts"
echo "  Potential ADIs checked: $checked"
echo "  Identity accounts found: $found_count"
echo ""

if [ $found_count -eq 0 ]; then
    echo "No identity accounts found in potential ADIs."
    echo ""
    echo "Sample of account types found:"

    # Show types of first 10 potential ADIs
    count=0
    while IFS= read -r account; do
        if [ -z "$account" ]; then
            continue
        fi

        if [ $count -ge 10 ]; then
            break
        fi

        count=$((count + 1))

        acc_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$account\",\"database\":\"$DATABASE\"}}}" | $MCP_BIN 2>/dev/null)
        acc_type=$(echo "$acc_response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

        echo "  $account -> $acc_type"
    done <<< "$potential_adis"
fi

echo ""
echo "=== Test Complete ==="
