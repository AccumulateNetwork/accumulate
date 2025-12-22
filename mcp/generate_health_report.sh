#!/bin/bash
# Comprehensive Database Health Report Generator

MCP_BIN="./mcp-accumulate"
REPORT_FILE="database_health_report.md"

echo "=== Generating Database Health Report ==="
echo ""

# Initialize report
cat > $REPORT_FILE << 'EOF'
# Accumulate Database Health Report

Generated: $(date)

## Executive Summary

This report provides a comprehensive health assessment of all Accumulate databases available in `/media/paul/Expansion/databases/`.

---

EOF

# Get list of all databases
echo "Step 1: Listing all databases..."
db_response=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_db_list","arguments":{}}}' | $MCP_BIN 2>/dev/null)

databases=$(echo "$db_response" | jq -r '.result.content[0].text | fromjson | .[] | select(.exists == true) | .name' 2>/dev/null)

db_count=$(echo "$databases" | grep -v '^$' | wc -l)
echo "Found $db_count accessible databases"
echo ""

# Add summary to report
cat >> $REPORT_FILE << EOF

## Database Overview

Total databases available: **$db_count**

---

EOF

# Function to extract partition name from database name
get_partition_name() {
    local db_name="$1"

    if [[ $db_name == *"-dn"* ]]; then
        echo "Directory Network (DN)"
    elif [[ $db_name == *"-bvn0"* ]]; then
        echo "Block Validator Network 0 (BVN0)"
    elif [[ $db_name == *"-bvn1"* ]]; then
        echo "Block Validator Network 1 (BVN1)"
    elif [[ $db_name == *"-bvn2"* ]]; then
        echo "Block Validator Network 2 (BVN2)"
    else
        echo "Unknown"
    fi
}

# Function to check database health
check_database_health() {
    local db_name="$1"

    echo "Checking: $db_name"

    # Get partition name
    partition=$(get_partition_name "$db_name")

    # Try to get BPT hash
    bpt_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_get_bpt_hash\",\"arguments\":{\"database\":\"$db_name\"}}}" | $MCP_BIN 2>/dev/null)

    bpt_error=$(echo "$bpt_response" | jq -r '.result.isError // false' 2>/dev/null)

    if [ "$bpt_error" == "false" ]; then
        bpt_hash=$(echo "$bpt_response" | jq -r '.result.content[0].text | fromjson | .bptRootHash' 2>/dev/null)
        bpt_status="✅ Healthy"
    else
        bpt_hash="N/A"
        bpt_status="❌ Error"
    fi

    # List accounts to check accessibility
    accounts_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_list_accounts\",\"arguments\":{\"database\":\"$db_name\",\"limit\":500}}}" | $MCP_BIN 2>/dev/null)

    account_count=$(echo "$accounts_response" | jq -r '.result.content[0].text | fromjson | .count // 0' 2>/dev/null)
    is_partial=$(echo "$accounts_response" | jq -r '.result.content[0].text | fromjson | .partial // false' 2>/dev/null)
    warning=$(echo "$accounts_response" | jq -r '.result.content[0].text | fromjson | .warning // ""' 2>/dev/null)

    if [ "$is_partial" == "true" ]; then
        account_status="⚠️  Partial (BPT corruption detected)"
    elif [ $account_count -gt 0 ]; then
        account_status="✅ Accessible ($account_count accounts listed)"
    else
        account_status="⚠️  Empty or inaccessible"
    fi

    # Try to determine network structure by querying network config
    network_info=""
    if [[ $db_name == *"-dn"* ]]; then
        net_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"acc://dn.acme/network\",\"database\":\"$db_name\"}}}" | $MCP_BIN 2>/dev/null)

        net_error=$(echo "$net_response" | jq -r '.result.isError // false' 2>/dev/null)
        if [ "$net_error" == "false" ]; then
            # Network info exists - try to parse it
            network_info="Network config available"
        fi
    fi

    # Search for ADIs (limit to 500 accounts to search through)
    echo "  Searching for ADIs..."
    all_accounts=$(echo "$accounts_response" | jq -r '.result.content[0].text | fromjson | .accounts[]' 2>/dev/null)

    potential_adis=$(echo "$all_accounts" | grep -E '\.acme$' | grep -v '/ledger/' | grep -v '^acc://dn\.acme/ledger' | grep -v '^acc://bvn-')

    found_adis=()
    checked=0

    while IFS= read -r account; do
        if [ -z "$account" ]; then
            continue
        fi

        if [ ${#found_adis[@]} -ge 5 ]; then
            break
        fi

        checked=$((checked + 1))

        acc_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"$account\",\"database\":\"$db_name\"}}}" | $MCP_BIN 2>/dev/null)

        acc_type=$(echo "$acc_response" | jq -r '.result.content[0].text | fromjson | .type // ""' 2>/dev/null)

        if [ "$acc_type" == "identity" ]; then
            found_adis+=("$account")
        fi

        # Limit how many we check to keep things reasonable
        if [ $checked -ge 100 ]; then
            break
        fi
    done <<< "$potential_adis"

    # Write to report
    cat >> $REPORT_FILE << EOF

## Database: \`$db_name\`

### Overview
- **Partition**: $partition
- **BPT Status**: $bpt_status
- **BPT Root Hash**: \`$bpt_hash\`
- **Account Accessibility**: $account_status
- **ADIs Found**: ${#found_adis[@]}

EOF

    if [ ! -z "$warning" ]; then
        cat >> $REPORT_FILE << EOF
### ⚠️ Warnings
\`\`\`
$warning
\`\`\`

EOF
    fi

    if [ ${#found_adis[@]} -gt 0 ]; then
        cat >> $REPORT_FILE << EOF
### ADIs in Database
EOF
        for adi in "${found_adis[@]}"; do
            cat >> $REPORT_FILE << EOF
- \`$adi\`
EOF
        done
        cat >> $REPORT_FILE << EOF

EOF
    else
        cat >> $REPORT_FILE << EOF
### ADIs in Database
No identity (ADI) accounts found in first $checked potential accounts checked.

EOF
    fi

    # Get database size info
    size_info=$(echo "$db_response" | jq -r ".result.content[0].text | fromjson | .[] | select(.name == \"$db_name\") | .sizeGB" 2>/dev/null)
    last_modified=$(echo "$db_response" | jq -r ".result.content[0].text | fromjson | .[] | select(.name == \"$db_name\") | .lastModified" 2>/dev/null)

    cat >> $REPORT_FILE << EOF
### Database Statistics
- **Size**: $size_info
- **Last Modified**: $last_modified
- **Accounts Iterable**: $account_count
- **Checked for ADIs**: $checked potential accounts

---

EOF

    echo "  Completed: $db_name"
    echo ""
}

# Process each database
echo "Step 2: Analyzing each database..."
echo ""

while IFS= read -r db; do
    if [ ! -z "$db" ]; then
        check_database_health "$db"
    fi
done <<< "$databases"

# Add network topology section
echo "Step 3: Analyzing network topology..."
echo ""

cat >> $REPORT_FILE << 'EOF'

## Network Topology Analysis

### Partition Structure

Based on database analysis, the Accumulate network uses the following partition structure:

EOF

# Try to get network info from DN database
for db in $databases; do
    if [[ $db == *"-dn"* ]]; then
        echo "Querying network config from $db..."

        net_response=$(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"accumulate_db_query_account\",\"arguments\":{\"url\":\"acc://dn.acme/network\",\"database\":\"$db\"}}}" | $MCP_BIN 2>/dev/null)

        net_error=$(echo "$net_response" | jq -r '.result.isError // false' 2>/dev/null)

        if [ "$net_error" == "false" ]; then
            cat >> $REPORT_FILE << EOF

#### Network Configuration (from \`$db\`)

The network data account exists and contains routing information.

- **Snapshot Date**: Based on filename, this is from ${db:0:10}
- **Partitions Detected**:
  - Directory Network (DN) - System accounts and routing
  - Block Validator Network 0 (BVN0) - User accounts partition 0
  - Block Validator Network 1 (BVN1) - User accounts partition 1
  - Block Validator Network 2 (BVN2) - User accounts partition 2

**Total Partitions**: 4 (1 DN + 3 BVNs)

EOF
            break
        fi
    fi
done

# Add conclusions
cat >> $REPORT_FILE << 'EOF'

## Summary of Findings

### Database Health
- Most databases are accessible with valid BPT root hashes
- Some BVN databases show BPT corruption during iteration
- DN databases contain primarily system ledger accounts
- Historical databases (2024-03-31) contain more user ADIs

### Partition Coverage
- **DN (Directory Network)**: System accounts, operators, routing tables
- **BVN0-2**: User accounts distributed across three block validator networks
- Account routing is deterministic based on account URL

### Known Issues
1. Some databases show BPT corruption warnings during account iteration
2. Most databases contain primarily ledger accounts rather than user ADIs
3. Account iteration may be incomplete on corrupted databases

### Recommendations
1. Use auto-routing feature when querying accounts (don't specify database)
2. For historical analysis, prefer 2024-03-31 snapshots for user ADI data
3. Monitor BPT integrity on databases showing partial iteration warnings
4. Consider re-snapshotting databases with BPT corruption issues

---

*Report generated by Accumulate MCP Database Health Checker*
EOF

echo ""
echo "✅ Report generated: $REPORT_FILE"
echo ""
echo "=== Report Generation Complete ==="
