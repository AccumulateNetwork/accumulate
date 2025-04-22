# Stable Node Approach for Anchor Healing

## Problem Statement

The anchor healing process has been experiencing two major issues:

1. **URL Construction Inconsistencies**: Different parts of the codebase construct URLs differently, leading to routing conflicts:
   - `sequence.go` uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
   - `heal_anchor.go` appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

2. **Backoff Mechanism Triggering**: The previous implementation attempted to submit to many different nodes when failures occurred, potentially triggering backoff mechanisms in the P2P layer:
   - The P2P layer implements an exponential backoff starting at 1 minute and doubling with each attempt
   - Hitting the same node repeatedly in a short time period causes it to ignore requests

## Solution: Stable Node Approach

We've implemented a two-part solution to address these issues:

### 1. URL Normalization

- Standardized on the URL format used in `sequence.go` (e.g., `acc://bvn-Apollo.acme`)
- Implemented URL normalization to ensure consistent format before routing
- Created a `NormalizingRouter` that normalizes URLs before routing
- Added explicit overrides for problematic URLs with a `DirectRouter`

### 2. Stable Node Submission

The new `stableNodeSubmitLoop` implementation focuses on using a single known good node per partition:

```go
// Primary node selection
primaryNodes := make(map[string]peer.ID)

// Track success/failure counts for nodes
successCounts := make(map[string]int)
failureCounts := make(map[string]int)

// Track last test time for nodes
lastTestTime := make(map[string]time.Time)
```

#### Key Features

1. **Primary Node Selection**:
   - For each partition, we maintain a "primary node" that has proven reliable
   - Submissions are first attempted through this primary node
   - This minimizes the number of different nodes we contact

2. **Limited Testing of Other Nodes**:
   - If the primary node fails or doesn't exist, we test only a small number of other nodes in the target partition
   - Nodes are tested at varying intervals based on their success/failure history:
     - Good nodes (more successes than failures): tested every hour
     - Problematic nodes (5+ failures): tested every 6 hours
     - Very problematic nodes (10+ failures): tested only once per day

3. **No Cross-Partition Fallbacks**:
   - We strictly adhere to the routing decisions and avoid triggering backoff mechanisms
   - No fallback attempts to other partitions

4. **Submission Caching**:
   - We maintain a cache of successful submissions to avoid redundant work
   - This complements our existing query caching system

## Benefits

1. **Consistent URL Handling**:
   - URLs are consistently constructed across the codebase
   - Routing conflicts are minimized or eliminated

2. **Avoiding Backoff Triggers**:
   - By focusing on a single known good node per partition, we avoid hitting the same node repeatedly
   - The limited testing of other nodes prevents the "scatter-shot" approach that might trigger backoff

3. **Improved Reliability**:
   - The system quickly identifies and sticks with reliable nodes
   - Problematic nodes are tested less frequently, reducing the chance of failures

4. **Reduced Network Load**:
   - Fewer submission attempts mean less network traffic
   - Caching prevents redundant submissions

## Implementation Details

The implementation is contained in two main files:

1. `heal_enhanced_routing.go`: Contains the URL normalization and routing enhancements
2. `heal_stable_node.go`: Contains the stable node submission loop implementation

The approach is integrated into the existing codebase through the `heal_common.go` file, which now uses the `stableNodeSubmitLoop` instead of the original `submitLoop`.

## Future Enhancements

Potential future enhancements to this approach include:

1. **Persistent Node Tracking**: Store successful nodes between runs to maintain knowledge of good nodes
2. **Adaptive Testing Intervals**: Dynamically adjust testing intervals based on network conditions
3. **Partition-Specific Strategies**: Implement custom strategies for partitions known to have specific issues (e.g., Chandrayaan)
