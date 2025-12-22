#!/bin/bash
# Fast test to query known ADIs directly from the database

MCP_BIN="./mcp-accumulate"

echo "=== Querying Known ADIs ==="
echo ""

# List of known ADIs extracted from network data
known_adis=(
    "acc://kompendium.acme"
    "acc://LunaNova.acme"
    "acc://Factoshi.acme"
    "acc://TurtleBoat.acme"
    "acc://Stamp-It.acme"
    "acc://MusicCityNode.acme"
    "acc://ConsensusNetworks.acme"
    "acc://defacto.acme"
    "acc://tfa.acme"
    "acc://CodeForj.acme"
    "acc://PrestigeIT.acme"
    "acc://GOI.acme"
    "acc://defidevs.acme"
    "acc://Sphereon.acme"
    "acc://ACMEMining.acme"
    "acc://Inveniam.acme"
    "acc://HighStakes.acme"
    "acc://FederateThis.acme"
    "acc://DetroitLedgerTech.acme"
    "acc://staking.acme"
)

count=0
success=0

for adi in "${known_adis[@]}"; do
    if [ $success -ge 5 ]; then
        break
    fi

    count=$((count + 1))

    echo "[$count] Querying: $adi"

    # Query the ADI from database (auto-routing will find correct database)
    response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":$count,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$adi\"}}}" | $MCP_BIN 2>/dev/null)

    # Check if query was successful
    error=$(echo "$response" | jq -r '.result.isError // false' 2>/dev/null)

    if [ "$error" == "false" ]; then
        success=$((success + 1))

        echo ""
        echo "=== ADI #$success: $adi ==="
        echo ""

        # Extract and format the account info
        echo "$response" | jq -r '.result.content[0].text | fromjson' | jq '{
            type,
            url,
            keyBook,
            authorities: [.authorities[]? | {url: .url, disabled: (.disabled // false)}]
        }'
        echo ""
    else
        error_msg=$(echo "$response" | jq -r '.result.content[0].text // "Unknown error"' 2>/dev/null)
        echo "  Error: $error_msg"
        echo ""
    fi
done

echo ""
echo "==========="
echo "Summary:"
echo "  ADIs queried: $count"
echo "  Successfully found: $success"
echo ""
echo "=== Test Complete ==="
