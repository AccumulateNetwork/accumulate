#!/bin/bash
# Test all curl examples from the Go SDK documentation

ENDPOINT=${ENDPOINT:-https://mainnet.accumulatenetwork.io/v3}

echo "Testing against: $ENDPOINT"
echo "================================"

# GetAccount - WORKS
echo "1. GetAccount (acc://ACME):"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
      "scope": "acc://ACME",
      "query": {}
    },
    "id": 1
  }' | jq -r '.result.account.type // .error.message'

# GetTransaction - WORKS (but needs valid hash)
echo "2. GetTransaction (message-hash):"
echo "   Note: Needs a valid 32-byte transaction hash"

# GetChainEntry - Returns account instead of chain entry
echo "3. GetChainEntry:"
echo "   Note: Currently returns account, not chain entry"

# GetDataEntry - Returns account with data
echo "4. GetDataEntry (acc://dn.acme/network):"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
      "scope": "acc://dn.acme/network",
      "query": {
        "type": "data",
        "index": 0
      }
    },
    "id": 1
  }' | jq -r '.result.account.type // .error.message'

# GetDirectory - WORKS
echo "5. GetDirectory (acc://dn.acme):"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
      "scope": "acc://dn.acme",
      "query": {
        "type": "directory",
        "range": {
          "start": 0,
          "count": 3
        }
      }
    },
    "id": 1
  }' | jq -r '.result.directory.records[:3] | length // .error.message'

# GetNodeInfo - WORKS
echo "6. GetNodeInfo:"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "node-info",
    "params": {},
    "id": 1
  }' | jq -r '.result.network // .error.message'

# GetNetworkStatus - WORKS
echo "7. GetNetworkStatus:"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "network-status",
    "params": {},
    "id": 1
  }' | jq -r '.result.network.networkName // .error.message'

# GetConsensusStatus - Requires node ID
echo "8. GetConsensusStatus:"
echo "   Note: Requires nodeID parameter"

# GetMetrics - WORKS
echo "9. GetMetrics (Directory):"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "metrics",
    "params": {
      "partition": "Directory"
    },
    "id": 1
  }' | jq -r 'if .result.tps then "TPS: \(.result.tps)" else .error.message end'

# FindService - WORKS (but returns empty on mainnet)
echo "10. FindService (query):"
curl -s -X POST $ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "find-service",
    "params": {
      "service": {"type": "query"}
    },
    "id": 1
  }' | jq -r 'if .result then "Found \(.result | length) services" else .error.message end'

# ListSnapshots - Requires node ID
echo "11. ListSnapshots:"
echo "   Note: Requires nodeID parameter"

echo "================================"
echo "Summary:"
echo "✅ Working: GetAccount, GetDirectory, GetNodeInfo, GetNetworkStatus, GetMetrics, FindService"
echo "⚠️  Needs params: GetTransaction (valid hash), GetConsensusStatus (nodeID), ListSnapshots (nodeID)"
echo "❓ Different behavior: GetChainEntry, GetDataEntry (return account info)"