# Directory Network (DN) Height Research Findings

## Summary
After extensive research into the Accumulate codebase and mainnet API, I've discovered that the public mainnet API returns a cached/static value for the Directory Network anchor height.

## Key Findings

### 1. API Behavior
- The `network-status` endpoint on mainnet always returns `directoryHeight: 2460315`
- This value does not update in real-time, despite the DN being active
- The same static value is returned regardless of the partition parameter

### 2. Internal Implementation
From analyzing `internal/api/v3/network.go`:
- The DN height is retrieved from the AnchorPool account's main chain
- It searches for DirectoryAnchor transactions with MinorBlockIndex
- This implementation works correctly on internal/validator nodes

### 3. Public API Limitations
- The mainnet public API (`https://mainnet.accumulatenetwork.io/v3`) appears to cache this value
- Direct BVN endpoints (mainnet-bvn0, mainnet-bvn1, etc.) are not publicly accessible
- The query API methods for blocks don't provide real-time DN height data

### 4. Cyclops BVN Alternative
- The Cyclops BVN (accessed via `http://apollo-mainnet.accumulate.defidevs.io:16692/status`) provides real-time block height
- This endpoint updates approximately once per second
- It serves as a useful indicator of network activity, though it's BVN-specific not DN

## Implementation Decision
The network monitor has been updated to:
1. Display the DN height from the API with a warning that it's a cached value
2. Show the Cyclops BVN block height as a live indicator of network activity
3. Include proper visual indicators (⚠️) to inform users about the static nature of the DN height

## Recommendation for Production
For applications requiring real-time DN height:
1. Run your own validator or follower node to access internal APIs
2. Use alternative metrics like BVN block heights for activity monitoring
3. Contact Accumulate team about potential future public API improvements

## Test Scripts Created
- `test_dn_height.go` - Tests various DN height retrieval methods
- `test_dn_partition.go` - Tests partition-specific queries
- `test_minor_blocks.go` - Attempts to query minor blocks directly
- `test_blocks_correct.go` - Uses correct BlockQuery structure

All test scripts confirmed the static nature of the public API's DN height response.