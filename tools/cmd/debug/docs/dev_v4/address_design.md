# AddressDir Design Document

> **⚠️ IMPORTANT: DO NOT DELETE THIS DEVELOPMENT PLAN ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS development plan will not be deleted.

---

## DEVELOPMENT PLAN: AddressDir Struct for Validator Multiaddresses

---

## Overview

The `AddressDir` struct is designed to collect, manage, and provide access to multiaddresses for validators in the Accumulate network. This is a critical component for the healing process, as it enables reliable communication with validators across the network.

### Important Design Goals for dev_v3

**1. NO CACHING IMPLEMENTATION**: Unlike previous versions, dev_v3 will NOT implement a caching system. The focus is on correct validator discovery and URL construction, without the added complexity of caching query results.

**2. NO TRANSACTION RETRY LOGIC**: dev_v3 will NOT include transaction retry functionality. Failed transactions will not be automatically retried, and error handling will be simplified.

These design decisions streamline the implementation and allow us to focus on the core functionality of validator discovery and management.

### URL Construction Standards

Consistent URL construction is critical for the healing process. There are two main approaches to URL construction in the codebase:

1. **Raw Partition URLs** (used in sequence.go):
   - Directory Network: `acc://dn.acme`
   - BVNs: `acc://bvn-{PartitionID}.acme` (e.g., `acc://bvn-Apollo.acme`)

2. **Anchor Pool URLs** (used in heal_anchor.go):
   - Directory Network: `acc://dn.acme/anchors`
   - BVNs with partition ID: `acc://dn.acme/anchors/{PartitionID}` (e.g., `acc://dn.acme/anchors/Apollo`)

For consistency, the `AddressDir` implementation uses the raw partition URL format (approach #1) as it is the original implementation and appears to be more widely used throughout the codebase. This standardization helps avoid "element does not exist" errors that can occur when using inconsistent URL formats.

### Network Topology

The Accumulate network has a specific topology that influences the design of the `AddressDir` struct:

1. **Directory Network (DN)**: All validators participate in the Directory Network.
2. **Block Validator Networks (BVNs)**: Each validator also participates in exactly one BVN.
3. **Partition Assignment**: No validator participates in multiple BVNs.

This topology simplifies how we track and query validators, as each validator is associated with exactly two partitions: the DN and exactly one BVN.

## Requirements

1. Store multiaddresses for validators, indexed by validator ID or name
2. Support filtering of valid P2P multiaddresses (must include IP, TCP, and P2P components)
3. Track problematic nodes to avoid querying them for certain types of requests
4. Provide methods to add, retrieve, and manage validator addresses
5. Support standardized URL construction for consistent addressing
6. Track partition information for each validator (DN and potentially one BVN)
7. Monitor validator status for better decision-making
8. Maintain a dynamic structure that can be updated at runtime
9. Provide methods to discover validators from the network using the NetworkStatus API
10. Ensure consistency with existing heal_v1 implementation for validator discovery and management

## Struct Definition

```go
// AddressDir manages validator multiaddresses for the healing process
type AddressDir struct {
    // Slice of validators to preserve order for reporting
    Validators []Validator
    
    // Directory Network validators (all validators participate in DN)
    DNValidators []string // List of validator IDs in the DN
    
    // BVN validators (each validator participates in exactly one BVN)
    BVNValidators map[string][]string // Map of BVN ID to list of validator IDs
    
    // Track problematic nodes to avoid querying them
    ProblemNodes []ProblemNode
    
    // URL construction helpers
    URLHelpers map[string]string
    
    // Network peers (both validators and non-validators)
    // Map of peer ID to NetworkPeer, where peer ID is derived from the validator's public key hash
    NetworkPeers map[string]NetworkPeer
    
    // Mutex for concurrent access
    mu sync.RWMutex
}

// Validator represents a validator with its multiaddresses
type Validator struct {
    // Unique identifier for the validator
    ID string
    
    // Name or description of the validator
    Name string
    
    // Partition information
    PartitionID string   // e.g., "bvn-Apollo", "dn"
    PartitionType string // "bvn" or "dn"
    
    // Status of the validator
    Status string // "active", "inactive", "unreachable", etc.
    
    // Slice of addresses for this validator
    Addresses []ValidatorAddress
    
    // URLs associated with this validator
    URLs map[string]string // Map of URL type to URL string
    
    // Last updated timestamp
    LastUpdated time.Time
}

// NetworkPeer represents any peer in the network (validator or non-validator)
type NetworkPeer struct {
    // Unique identifier for the peer (derived from the validator's public key hash)
    ID string
    
    // Whether this peer is a validator (has operator URL and at least one active partition)
    IsValidator bool
    
    // If this is a validator, the validator ID (from operator URL)
    ValidatorID string
    
    // Peer multiaddresses
    Addresses []string
    
    // Peer status
    Status string // "active", "inactive", "unreachable", etc.
    
    // Partition information (if applicable)
    PartitionID string
    
    // Partition URL in raw format (e.g., acc://bvn-Apollo.acme)
    // This follows the URL construction standard used in sequence.go
    // For Directory Network: acc://dn.acme
    // For BVNs: acc://bvn-{PartitionID}.acme
    PartitionURL string
    
    // Last seen timestamp
    LastSeen time.Time
}

// ValidatorAddress represents a validator's multiaddress with metadata
type ValidatorAddress struct {
    // The multiaddress string (e.g., "/ip4/144.76.105.23/tcp/16593/p2p/QmHash...")
    Address string
    
    // Whether this address has been validated as a proper P2P multiaddress
    Validated bool
    
    // Components of the address for easier access
    IP string
    Port string
    PeerID string
    
    // Last time this address was successfully used
    LastSuccess time.Time
    
    // Number of consecutive failures when trying to use this address
    FailureCount int
    
    // Last error encountered when using this address
    LastError string
    
    // Whether this address is preferred for certain operations
    Preferred bool
}

// ProblemNode tracks information about a problematic node
type ProblemNode struct {
    // Validator ID of the problematic node
    ValidatorID string
    
    // Partition information
    PartitionID string
    
    // Time when the node was marked as problematic
    MarkedAt time.Time
    
    // Reason for marking the node as problematic
    Reason string
    
    // Types of requests that should avoid this node
    AvoidForRequestTypes []string
    
    // Number of consecutive failures
    FailureCount int
    
    // When to retry this node (backoff strategy)
    RetryAfter time.Time
}

### Node Height Collection

A critical aspect of network monitoring is tracking the current block heights of each node across different partitions. The implementation collects and tracks the following height information:

1. **Directory Network (DN) Height**: The current block height of the node in the Directory Network partition
2. **Block Validator Network (BVN) Height**: The current block height of the node in its assigned BVN partition
3. **Zombie Status**: Whether the node is a "zombie" (not participating in consensus)

#### Height Collection Process

The process for collecting node heights follows these steps:

1. **Identify Node Endpoints**: For each discovered peer, determine its API endpoints:
   - Tendermint RPC endpoint (typically port 16592 for primary and 16692 for secondary)
   - Accumulate API endpoint (typically port 16595)

2. **Query Tendermint Status**: Use the Tendermint RPC client to query the node's status:
   ```go
   // Create a Tendermint HTTP client
   base := fmt.Sprintf("http://%s:%d", node.Host, port) // port is 16592 or 16692
   c, err := http.New(base, base+"/ws")
   if err != nil {
       // Handle error
   }
   
   // Query the node status
   status, err := c.Status(ctx)
   if err != nil {
       // Handle error
   }
   
   // Extract the height information
   height := uint64(status.SyncInfo.LatestBlockHeight)
   ```

3. **Determine Partition Type**: Based on the network information in the status response, determine which partition this height belongs to:
   ```go
   part := status.NodeInfo.Network
   if i := strings.LastIndexByte(part, '.'); i >= 0 {
       part = part[i+1:]
   }
   part = strings.ToLower(part)
   ```

4. **Update Peer Height Information**: Update the appropriate height field based on the partition type:
   ```go
   if part == strings.ToLower(protocol.Directory) {
       peer.DNHeight = height
   } else {
       peer.BVNHeight = height
   }
   ```

5. **Check Zombie Status**: Determine if the node is a zombie (not participating in consensus):
   ```go
   // Query the consensus state
   consensusState, err := c.ConsensusState(ctx)
   if err != nil {
       // Handle error
   }
   
   // A node is considered a zombie if it's not in the validator set
   // or if it hasn't voted in recent rounds
   peer.IsZombie = !isInValidatorSet(consensusState) || !hasRecentVotes(consensusState)
   ```

## Key Methods

1. **AddValidator(id string, name string, partitionID string, partitionType string) *Validator**
   - Add a new validator to the directory with partition information
   - Return a pointer to the created validator
   - If validator with ID already exists, update its information and return it

2. **AddValidatorAddress(validatorID string, address string) error**
   - Add a validator address to the specified validator
   - Validate that the address is a proper P2P multiaddress
   - Parse and store the components (IP, port, peer ID)
   - Return an error if the address is invalid or validator not found

3. **FindValidator(id string) (*Validator, int, bool)**
   - Find a validator by ID
   - Return the validator, its index in the slice, and a boolean indicating if found

4. **FindValidatorsByPartition(partitionID string) []*Validator**
   - Find all validators belonging to a specific partition
   - Return a slice of pointers to the found validators

5. **GetValidatorAddresses(id string) ([]ValidatorAddress, bool)**
   - Retrieve all addresses for a specific validator
   - Return a boolean indicating if the validator exists

6. **SetValidatorStatus(id string, status string) bool**
   - Update the status of a validator
   - Return true if the validator was found and updated

7. **AddValidatorURL(validatorID string, urlType string, url string) error**
   - Add a URL to a validator's URL map
   - Validate the URL format
   - Return an error if the validator is not found

8. **GetValidatorURL(validatorID string, urlType string) (string, bool)**
   - Get a specific URL for a validator
   - Return the URL and a boolean indicating if found

9. **MarkNodeProblematic(id string, reason string, requestTypes []string)**
   - Mark a node as problematic for specific request types
   - Record the reason, time, and partition information
   - Implement exponential backoff for retry attempts

10. **IsNodeProblematic(id string, requestType string) bool**
    - Check if a node is problematic for a specific request type
    - Consider retry time when determining if still problematic

11. **ValidateAddress(address string) (bool, map[string]string)**
    - Validate that an address is a proper P2P multiaddress
    - Must include IP, TCP, and P2P components
    - Return components as a map for easy access

12. **GetHealthyValidators(partitionID string) []Validator**
    - Return a slice of healthy validators for a specific partition
    - Filter out validators with problematic status
    - Preserves the original order for reporting

13. **GetPreferredAddresses(validatorID string) []ValidatorAddress**
    - Return preferred addresses for a validator
    - Sort by success rate and recency of successful use

14. **DiscoverValidators(ctx context.Context, client api.NetworkService) (int, error)**
    - Discover validators from the network using the NetworkStatus API
    - Process validator information and add them to the AddressDir
    - Add each validator to the DN validator list (DNValidators)
    - Add each validator to its respective BVN validator list (BVNValidators)
    - Set validator status based on whether they're active on their partitions
    - Return the number of validators discovered and any error encountered

15. **GetActiveValidators() []Validator**
    - Return all validators with "active" status
    - Used when selecting validators for operations

16. **GetDNValidators() []Validator**
    - Return all validators that participate in the Directory Network
    - Since all validators participate in the DN, this returns all validators

17. **GetBVNValidators(bvnID string) []Validator**
    - Return all validators that participate in the specified BVN
    - Each validator participates in exactly one BVN

18. **DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error)**
    - Discover all network peers (validators and non-validators) using a multi-stage discovery process
    - Follow these steps:
        1. **Initial Discovery**: Query the network status to get the list of validators and partitions
        2. **Partition-based Discovery**: For each partition, query the network status to get additional peers
        3. **Peer-to-Peer Discovery**: For known peers, attempt to discover additional peers
    - Populate the NetworkPeers map with discovered peers
    - Identify validators based on having an operator URL and at least one active partition
    - Use standardized URL format (e.g., `acc://bvn-Apollo.acme`) for partition URLs
    - Return the number of peers discovered and any error encountered

19. **RefreshNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error)**
    - Refresh the network peer information and track changes in peer status
    - Save the current state of all peers before refreshing
    - Call DiscoverNetworkPeers to get the latest network state
    - Compare the previous state with the current state to identify:
        1. New peers that were discovered
        2. Peers that were lost (present in the previous state but not in the current state)
        3. Peers whose status changed
    - Update the status of all peers, marking lost peers accordingly
    - Return detailed statistics about the refresh operation via the RefreshStats struct

20. **GetNetworkPeers() []NetworkPeer**
    - Return all network peers
    - Return an empty slice if no peers are found

21. **GetValidatorPeers() []NetworkPeer**
    - Return all peers that are validators
    - A peer is considered a validator if it has an operator URL and at least one active partition

22. **GetNonValidatorPeers() []NetworkPeer**
    - Return all peers that are not validators
    - These are peers that either have no operator URL or no active partitions

23. **GetNetworkPeerByID(peerID string) (NetworkPeer, bool)**
    - Return a specific network peer by its ID
    - Return a boolean indicating if the peer exists

24. **AddValidatorToBVN(validatorID string, bvnID string) error**
    - Add a validator to a BVN validator list
    - Each validator can only be in one BVN
    - Return an error if the validator is already in another BVN

25. **AddValidatorToDN(validatorID string) error**
    - Add a validator to the DN validator list
    - All validators must be in the DN list

## Peer Discovery Implementation

The `DiscoverNetworkPeers` method discovers all network peers (validators and non-validators) using the NetworkStatus API. It identifies validators based on having an operator URL and at least one active partition. The method returns the number of peers discovered and any error encountered.

### Node Height Collection

A critical aspect of network monitoring is tracking the current block heights of each node across different partitions. The implementation collects and tracks the following height information:

1. **Directory Network (DN) Height**: The current block height of the node in the Directory Network partition
2. **Block Validator Network (BVN) Height**: The current block height of the node in its assigned BVN partition
3. **Zombie Status**: Whether the node is a "zombie" (not participating in consensus)

#### Height Collection Process

The process for collecting node heights follows these steps:

1. **Identify Node Endpoints**: For each discovered peer, determine its API endpoints:
   - Tendermint RPC endpoint (typically port 16592 for primary and 16692 for secondary)
   - Accumulate API endpoint (typically port 16595)

2. **Query Tendermint Status**: Use the Tendermint RPC client to query the node's status:
   ```go
   // Create a Tendermint HTTP client
   base := fmt.Sprintf("http://%s:%d", node.Host, port) // port is 16592 or 16692
   c, err := http.New(base, base+"/ws")
   if err != nil {
       // Handle error
   }
   
   // Query the node status
   status, err := c.Status(ctx)
   if err != nil {
       // Handle error
   }
   
   // Extract the height information
   height := uint64(status.SyncInfo.LatestBlockHeight)
   ```

3. **Determine Partition Type**: Based on the network information in the status response, determine which partition this height belongs to:
   ```go
   part := status.NodeInfo.Network
   if i := strings.LastIndexByte(part, '.'); i >= 0 {
       part = part[i+1:]
   }
   part = strings.ToLower(part)
   ```

4. **Update Peer Height Information**: Update the appropriate height field based on the partition type:
   ```go
   if part == strings.ToLower(protocol.Directory) {
       peer.DNHeight = height
   } else {
       peer.BVNHeight = height
   }
   ```

5. **Check Zombie Status**: Determine if the node is a zombie (not participating in consensus):
   ```go
   // Query the consensus state
   consensusState, err := c.ConsensusState(ctx)
   if err != nil {
       // Handle error
   }
   
   // A node is considered a zombie if it's not in the validator set
   // or if it hasn't voted in recent rounds
   peer.IsZombie = !isInValidatorSet(consensusState) || !hasRecentVotes(consensusState)
   ```

#### Reference Implementation

The implementation is based on the existing code in `tools/cmd/debug/network.go`, which provides a comprehensive approach to querying node status and heights. Here's a detailed breakdown of the relevant components:

1. **Connection to Tendermint RPC**:
   ```go
   // From tools/cmd/debug/network.go, lines ~500-507
   base := fmt.Sprintf("http://%s:%d", node.Host, port) // port is 16592 or 16692
   c, err := http.New(base, base+"/ws")
   if err != nil {
       // Handle error and continue to the next port or node
       continue
   }
   ```

2. **Status Query with Proper Context and Error Handling**:
   ```go
   // From tools/cmd/debug/network.go, lines ~512-517
   status := promise.Call(try(func() (*coretypes.ResultStatus, error) {
       ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
       defer cancel()
       
       return c.Status(ctx)
   }))
   ```

3. **Partition Extraction and Height Assignment**:
   ```go
   // From tools/cmd/debug/network.go, lines ~519-533
   part := promise.SyncThen(mu, status, func(status *coretypes.ResultStatus) promise.Result[*nodePartStatus] {
       // Extract partition name from network name
       part := status.NodeInfo.Network
       if i := strings.LastIndexByte(part, '.'); i >= 0 {
           part = part[i+1:]
       }
       part = strings.ToLower(part)
       
       // Find or create the partition status
       st, ok := node.Parts[part]
       if !ok {
           st = new(nodePartStatus)
           node.Parts[part] = st
       }
       
       // Set the height
       st.Height = uint64(status.SyncInfo.LatestBlockHeight)
       return promise.ValueOf(st)
   })
   ```

4. **Zombie Detection Implementation**:
   ```go
   // From tools/cmd/debug/network.go, lines ~536-543 and the nodeIsZombie function
   waitFor(wg, promise.Then(part, func(part *nodePartStatus) promise.Result[any] {
       z, err := nodeIsZombie(ctx, c)
       if err != nil {
           return promise.ErrorOf[any](err)
       }
       part.Zombie = z
       return promise.ValueOf[any](nil)
   }))
   
   // The nodeIsZombie function checks if a node is not participating in consensus
   // It examines the validator set and recent votes to determine if the node is active
   ```

5. **URL Construction for API Endpoints**:
   ```go
   // Tendermint RPC endpoint for consensus queries
   tendermintURL := fmt.Sprintf("http://%s:%d", node.Host, port) // port is 16592 or 16692
   
   // Accumulate API endpoint for application-level queries
   accumulateAPIv2 := fmt.Sprintf("http://%s:16595/v2", node.Host)
   accumulateAPIv3 := fmt.Sprintf("http://%s:16595/v3", node.Host)
   ```

This reference implementation provides a solid foundation for our height collection process, ensuring consistency with the existing codebase while adding the specific functionality needed for the AddressDir struct.

#### Integration with RefreshNetworkPeers

The RefreshNetworkPeers method will be enhanced to handle height information with the following detailed approach:

1. **Save Previous Heights and Zombie Status**: Before refreshing, save the current state:
   ```go
   // Create a map to store the previous state of all peers
   previousState := make(map[string]NetworkPeer)
   for id, peer := range a.NetworkPeers {
       // Create a deep copy of each peer
       previousState[id] = peer
   }
   ```

2. **Query New Heights Using DiscoverNetworkPeers**: During the refresh, query the latest heights:
   ```go
   // Call DiscoverNetworkPeers to get the latest network state
   totalPeers, err := a.DiscoverNetworkPeers(ctx, client)
   if err != nil {
       return RefreshStats{}, fmt.Errorf("failed to discover network peers: %w", err)
   }
   ```

3. **Track Height Changes with Detailed Comparison**: Compare previous and current heights to identify changes:
   ```go
   // Initialize statistics
   stats := RefreshStats{
       TotalPeers: totalPeers,
   }
   
   // Compare previous and current state
   for id, currentPeer := range a.NetworkPeers {
       previousPeer, existed := previousState[id]
       
       if !existed {
           // This is a new peer
           stats.NewPeers++
           stats.NewPeerIDs = append(stats.NewPeerIDs, id)
           continue
       }
       
       // Check for status changes
       if currentPeer.Status != previousPeer.Status {
           stats.StatusChanged++
           stats.ChangedPeerIDs = append(stats.ChangedPeerIDs, id)
       }
       
       // Check for height changes
       dnHeightChanged := currentPeer.DNHeight != previousPeer.DNHeight && 
                         (currentPeer.DNHeight > 0 || previousPeer.DNHeight > 0)
       bvnHeightChanged := currentPeer.BVNHeight != previousPeer.BVNHeight && 
                          (currentPeer.BVNHeight > 0 || previousPeer.BVNHeight > 0)
       
       if dnHeightChanged || bvnHeightChanged {
           stats.HeightChanged++
           stats.HeightChangedPeerIDs = append(stats.HeightChangedPeerIDs, id)
       }
       
       // Check for zombie status changes
       if !previousPeer.IsZombie && currentPeer.IsZombie {
           // Node became a zombie
           stats.NewZombies++
           stats.NewZombiePeerIDs = append(stats.NewZombiePeerIDs, id)
       } else if previousPeer.IsZombie && !currentPeer.IsZombie {
           // Node recovered from zombie status
           stats.RecoveredZombies++
           stats.RecoveredZombiePeerIDs = append(stats.RecoveredZombiePeerIDs, id)
       }
   }
   
   // Check for lost peers
   for id, previousPeer := range previousState {
       if _, exists := a.NetworkPeers[id]; !exists {
           // This peer was lost
           stats.LostPeers++
           stats.LostPeerIDs = append(stats.LostPeerIDs, id)
           
           // Add the lost peer back to the map but mark it as lost
           previousPeer.IsLost = true
           a.NetworkPeers[id] = previousPeer
       }
   }
   ```

4. **Track Network Consensus Progress**: Identify the current blockchain state across nodes:
   ```go
   // Track the highest heights observed in the network
   stats.DNHeightMax = 0
   stats.BVNHeightMax = 0
   
   // Track nodes that are lagging behind
   stats.DNLaggingNodes = 0
   stats.BVNLaggingNodes = 0
   
   // Calculate height statistics
   for _, peer := range a.NetworkPeers {
       // Skip lost peers for height calculations
       if peer.IsLost {
           continue
       }
       
       // Update DN height statistics
       if peer.DNHeight > 0 {
           // Track the highest height seen
           if peer.DNHeight > stats.DNHeightMax {
               stats.DNHeightMax = peer.DNHeight
           }
       }
       
       // Update BVN height statistics
       if peer.BVNHeight > 0 {
           // Track the highest height seen
           if peer.BVNHeight > stats.BVNHeightMax {
               stats.BVNHeightMax = peer.BVNHeight
           }
       }
   }
   
   // Now identify nodes that are significantly behind (more than 5 blocks)
   const significantLag = uint64(5)
   for _, peer := range a.NetworkPeers {
       if peer.IsLost {
           continue
       }
       
       // Check DN height lag
       if peer.DNHeight > 0 && stats.DNHeightMax > 0 {
           if stats.DNHeightMax - peer.DNHeight > significantLag {
               stats.DNLaggingNodes++
               stats.DNLaggingNodeIDs = append(stats.DNLaggingNodeIDs, peer.ID)
           }
       }
       
       // Check BVN height lag
       if peer.BVNHeight > 0 && stats.BVNHeightMax > 0 {
           if stats.BVNHeightMax - peer.BVNHeight > significantLag {
               stats.BVNLaggingNodes++
               stats.BVNLaggingNodeIDs = append(stats.BVNLaggingNodeIDs, peer.ID)
           }
       }
   }
   ```

This detailed implementation ensures that the RefreshNetworkPeers method provides comprehensive information about the network's consensus state, including height changes, zombie status changes, and overall network health metrics.

This comprehensive approach ensures that the `AddressDir` maintains accurate and up-to-date information about the network's consensus state, which is critical for monitoring network health and identifying potential issues.

### RefreshStats Struct

The `RefreshStats` struct tracks comprehensive statistics about a network peer refresh operation, including detailed height information:

```go
// RefreshStats tracks statistics about a network peer refresh operation
type RefreshStats struct {
	// Total number of peers found
	TotalPeers int

	// Number of new peers discovered
	NewPeers int

	// Number of peers that were lost
	LostPeers int

	// Number of peers that had their status changed
	StatusChanged int

	// Number of peers that had their height changed
	HeightChanged int

	// Number of peers that became zombies
	NewZombies int

	// Number of peers that recovered from zombie status
	RecoveredZombies int

	// Directory Network height statistics
	DNHeightMax uint64 // Maximum (latest) DN height across all peers
	DNLaggingNodes int // Number of DN nodes significantly behind the max height

	// Block Validator Network height statistics
	BVNHeightMax uint64 // Maximum (latest) BVN height across all peers
	BVNLaggingNodes int // Number of BVN nodes significantly behind the max height

	// IDs of new peers discovered
	NewPeerIDs []string

	// IDs of peers that were lost
	LostPeerIDs []string

	// IDs of peers that had their status changed
	ChangedPeerIDs []string

	// IDs of peers that had their height changed
	HeightChangedPeerIDs []string

	// IDs of peers that became zombies
	NewZombiePeerIDs []string

	// IDs of peers that recovered from zombie status
	RecoveredZombiePeerIDs []string
	
	// IDs of DN nodes significantly behind the max height
	DNLaggingNodeIDs []string
	
	// IDs of BVN nodes significantly behind the max height
	BVNLaggingNodeIDs []string
}
```

This enhanced struct provides a comprehensive view of the network's state, including:

1. **Peer Status Changes**: Tracking new, lost, and status-changed peers
2. **Blockchain Progress**: Monitoring the latest heights and identifying lagging nodes
3. **Zombie Status**: Detecting nodes that become zombies or recover

This information is crucial for network monitoring, troubleshooting, and ensuring the health of the Accumulate Network.

### RefreshNetworkPeers Method

The `RefreshNetworkPeers` method refreshes the network peer information and tracks changes in peer status:

```go
// RefreshNetworkPeers refreshes the network peer information and tracks changes in peer status
func (a *AddressDir) RefreshNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error)
```

This method performs the following operations:

1. **Save Current Peer State**: Before refreshing, it saves the current state of all peers, including their heights and zombie status
2. **Discover New Peers**: It calls `DiscoverNetworkPeers` to get the latest network state, including updated heights
3. **Compare States**: It compares the previous state with the current state to identify:
   - New peers that were discovered
   - Peers that were lost (present in the previous state but not in the current state)
   - Peers whose status changed
   - Peers whose heights changed (DN or BVN height)
   - Peers that became zombies or recovered from zombie status
4. **Update Peer Information**: It updates all peer information, including status, heights, and zombie status
5. **Return Statistics**: It returns detailed statistics about the refresh operation, including height and zombie status changes

This method is crucial for tracking changes in the network topology over time, especially for identifying nodes that have gone offline or become unreachable.

### Validator Identification

The `DiscoverNetworkPeers` method identifies validators using the following criteria:

1. **Operator URL**: A node must have an operator URL to be considered a validator candidate.
2. **Active Partitions**: A node must have at least one active partition to be considered an actual validator.

This two-part check ensures that we correctly identify the validators in the network, distinguishing them from non-validator nodes that might have operator URLs but no active partitions.

### Peer-to-Peer Discovery

The implementation uses a multi-stage approach to discover all peers in the network:

1. **Initial Discovery**: Start with validators identified from the Directory Network's NetworkStatus API.
   - Identify validators based on having an operator URL and at least one active partition
   - Add them to the NetworkPeers map with appropriate metadata

2. **Partition-based Discovery**: Query each partition's NetworkStatus API to find additional peers.
   - For each partition (Apollo, Chandrayaan, Yutu), query its network status
   - Extract peer information from each validator in the partition
   - Add newly discovered peers to the NetworkPeers map

3. **Extended Peer Discovery**: Use knowledge of common non-validator peer IDs to find additional peers.
   - Incorporate known non-validator peer IDs that might be present in the network
   - Assign these peers to appropriate partitions based on available information
   - Track all discovered peers with their metadata (ID, partition, validator status, etc.)

This comprehensive approach ensures we discover both validator and non-validator peers across all partitions, matching the behavior of the network status command which uses Tendermint's P2P network for discovery.

## Usage Example

```go
// Create a new address directory
addressDir := NewAddressDir()

// Add validators with partition information
val1 := addressDir.AddValidator("validator1", "Apollo Validator 1", "bvn-Apollo", "bvn")
val2 := addressDir.AddValidator("validator2", "Yutu Validator 1", "bvn-Yutu", "bvn")
val3 := addressDir.AddValidator("validator3", "DN Validator 1", "dn", "dn")

// Add validator addresses
addressDir.AddValidatorAddress("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/QmHash1...")
addressDir.AddValidatorAddress("validator1", "/ip4/144.76.105.24/tcp/16593/p2p/QmHash2...")
addressDir.AddValidatorAddress("validator2", "/ip4/144.76.105.25/tcp/16593/p2p/QmHash3...")

// Set validator status
addressDir.SetValidatorStatus("validator1", "active")
addressDir.SetValidatorStatus("validator2", "active")
addressDir.SetValidatorStatus("validator3", "inactive")

// Add URLs for validators
addressDir.AddValidatorURL("validator1", "partition", "acc://bvn-Apollo.acme")
addressDir.AddValidatorURL("validator1", "anchor-pool", "acc://bvn-Apollo.acme/anchors")
addressDir.AddValidatorURL("validator2", "partition", "acc://bvn-Yutu.acme")
addressDir.AddValidatorURL("validator3", "partition", "acc://dn.acme")

// Get addresses for a specific validator
addresses, exists := addressDir.GetValidatorAddresses("validator1")
if exists {
    for _, addr := range addresses {
        fmt.Printf("Address: %s (IP: %s, Port: %s, PeerID: %s)\n", 
                  addr.Address, addr.IP, addr.Port, addr.PeerID)
    }
}

// Find validators by partition
apolloBvnValidators := addressDir.FindValidatorsByPartition("bvn-Apollo")
fmt.Printf("Found %d validators for Apollo BVN\n", len(apolloBvnValidators))

// Mark a node as problematic
addressDir.MarkNodeProblematic("validator2", "Consistently failing to respond", []string{"anchor", "chain"})

// Refresh network peers and track changes
stats, err := addressDir.RefreshNetworkPeers(ctx, client)
if err != nil {
    fmt.Printf("Failed to refresh network peers: %v\n", err)
} else {
    fmt.Printf("Refresh stats: Total=%d, New=%d, Lost=%d, StatusChanged=%d\n", 
              stats.TotalPeers, stats.NewPeers, stats.LostPeers, stats.StatusChanged)
    
    if len(stats.NewPeerIDs) > 0 {
        fmt.Printf("New peers: %v\n", stats.NewPeerIDs)
    }
    
    if len(stats.LostPeerIDs) > 0 {
        fmt.Printf("Lost peers: %v\n", stats.LostPeerIDs)
    }
    
    if len(stats.ChangedPeerIDs) > 0 {
        fmt.Printf("Changed status peers: %v\n", stats.ChangedPeerIDs)
    }
}

// Check if a node is problematic for a specific request type
if !addressDir.IsNodeProblematic("validator1", "anchor") {
    // Safe to use validator1 for anchor requests
    
    // Get preferred addresses
    preferredAddresses := addressDir.GetPreferredAddresses("validator1")
    if len(preferredAddresses) > 0 {
        // Use the most reliable address
        bestAddress := preferredAddresses[0]
        fmt.Printf("Using preferred address: %s\n", bestAddress.Address)
    }
}

// Get all healthy validators for a specific partition
healthyValidators := addressDir.GetHealthyValidators("bvn-Apollo")

// Generate a report of all validators by partition
fmt.Println("Validator Report by Partition:")
for _, validator := range addressDir.Validators {
    fmt.Printf("Validator %s (%s):\n", validator.ID, validator.Name)
    fmt.Printf("  - Partition: %s (%s)\n", validator.PartitionID, validator.PartitionType)
    fmt.Printf("  - Status: %s\n", validator.Status)
    fmt.Printf("  - Last Updated: %s\n", validator.LastUpdated)
    fmt.Printf("  - URLs:\n")
    for urlType, url := range validator.URLs {
        fmt.Printf("    - %s: %s\n", urlType, url)
    }
    fmt.Printf("  - Addresses:\n")
    for _, addr := range validator.Addresses {
        fmt.Printf("    - %s (validated: %v, preferred: %v)\n", 
                  addr.Address, addr.Validated, addr.Preferred)
    }
}
```
```

## Implementation Considerations

1. **Thread Safety**: The struct uses a mutex to ensure thread-safe operations, as it may be accessed concurrently.

2. **Address Validation**: Addresses must be validated to ensure they are proper P2P multiaddresses with all required components. The validation process should extract and store the individual components (IP, port, peer ID) for easier access.

3. **Error Handling**: Methods should return appropriate errors when operations fail, with clear error messages. Error information should be stored with addresses to help diagnose connection issues.

4. **Performance**: While using slices means O(n) lookups instead of O(1) with maps, the number of validators is expected to be small, so the performance impact is negligible. The benefit of preserved order outweighs this minor performance cost.

5. **URL Standardization**: The design supports standardized URL construction through the URLHelpers map and per-validator URL storage. This ensures consistent URL usage across the codebase.

## URL Construction and Partition Discovery Issues

### URL Construction Inconsistencies

A critical issue identified in the existing codebase is the inconsistent approach to URL construction between different components:

1. **Inconsistent URL Formats**:
   - **sequence.go** uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
   - **heal_anchor.go** appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

2. **Impact of Inconsistencies**:
   - These inconsistencies can cause anchor healing to fail
   - Queries may return "element does not exist" errors when checking the wrong URL format
   - Anchor relationships might not be properly maintained between partitions
   - The caching system may not work effectively if URLs are constructed differently in different parts of the code

3. **Root Cause Analysis**:
   - The `findPendingAnchors` function in sequence.go uses `protocol.PartitionUrl(src.ID)` to construct URLs
   - The `healSingleAnchor` function in heal_anchor.go doesn't normalize URLs before using them
   - The improved version `healSingleAnchorWithNormalizedUrls` in dev_v2 includes URL normalization via the `normalizeAnchorUrl` function

4. **Standardization Approach**:
   - The AddressDir implementation should standardize on the approach used in sequence.go
   - URLs should be constructed using `protocol.PartitionUrl(partitionID)` which returns URLs in the format `acc://bvn-{partition}.acme` or `acc://dn.acme`
   - This matches the implementation in `normalizeAnchorUrl` from the dev_v2 version

## Network Peer Discovery Implementation

The `DiscoverNetworkPeers` method will collect information about all peers in the network, including both validators and non-validators. This provides a complete view of the network topology for more effective healing operations.

### Implementation Strategy

1. **API Calls**: Use the NetworkStatus API to get information about all peers in the network
2. **Peer Classification**: Identify which peers are validators and which are non-validators
3. **Address Collection**: Collect multiaddresses for each peer
4. **Status Tracking**: Monitor the status of each peer for better decision-making
5. **Partition Mapping**: Associate peers with their respective partitions when applicable

### Benefits of Complete Network Discovery

1. **Improved Healing**: With a complete view of the network, healing operations can be more effective
2. **Better Routing**: Knowledge of non-validator peers can improve message routing
3. **Network Health Monitoring**: Tracking all peers provides better insights into network health
4. **Fallback Options**: Non-validator peers can serve as fallback options when validators are unavailable

## Partition Discovery Implementation

### Current Partition Discovery Issues

1. **API Version Inconsistencies**:
   - Different parts of the codebase use different API versions (v2 vs v3)
   - The NetworkStatus API structure differs between versions
   - The v3 API should be used consistently throughout dev_v3

2. **Validator Discovery Process Inconsistencies**:
   - The ScanNetwork function in the healing package uses the NetworkStatus API to discover validators
   - It maps validators to their public key hashes and finds peers for each partition
   - The current implementation doesn't handle this consistently across different components

3. **Partition Information Handling Inconsistencies**:
   - All validators participate in the Directory Network (DN)
   - Each validator also participates in exactly one BVN
   - The current implementation doesn't consistently track this relationship

### Partition Discovery Process for dev_v3

The dev_v3 implementation will use the following process for partition discovery:

1. **Initial Query**:
   - Query the NetworkStatus API for the Directory Network (DN) partition
   - This provides the network definition with all validators and their partition assignments
   - Example API call: `client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})`

2. **Validator Extraction**:
   - Extract validators from the NetworkDefinition returned by the API
   - Each validator has a PublicKeyHash and a list of Partitions
   - Example: 
     ```go
     for _, validator := range networkStatus.Network.Validators {
         // Process each validator
         publicKeyHash := validator.PublicKeyHash
         // Extract partition information
     }
     ```

3. **Partition Assignment Processing**:
   - For each validator, process its partition assignments
   - Every validator should have the DN partition
   - Every validator should also have exactly one BVN partition
   - Add each validator to the DN list and its respective BVN list
   - Example:
     ```go
     var dnActive, bvnActive bool
     var bvnPartition string
     
     for _, partition := range validator.Partitions {
         if partition.ID == protocol.Directory {
             dnActive = partition.Active
             // Add validator to DN list
             a.DNValidators = append(a.DNValidators, validatorID)
         } else {
             // This is a BVN partition
             bvnPartition = partition.ID
             bvnActive = partition.Active
             
             // Add validator to its BVN list
             if _, exists := a.BVNValidators[bvnPartition]; !exists {
                 a.BVNValidators[bvnPartition] = make([]string, 0)
             }
             a.BVNValidators[bvnPartition] = append(a.BVNValidators[bvnPartition], validatorID)
         }
     }
     ```

4. **Validator Status Determination**:
   - A validator is considered active if it's active on either the DN or its BVN
   - This status is used when selecting validators for operations
   - Example:
     ```go
     isActive := dnActive || bvnActive
     ```

5. **URL Construction for Partitions**:
   - For each partition, construct a standardized URL using protocol.PartitionUrl
   - Store these URLs with the validator for later use
   - Example:
     ```go
     dnUrl := protocol.PartitionUrl(protocol.Directory)
     bvnUrl := protocol.PartitionUrl(bvnPartition)
     ```

6. **Multiaddress Discovery**:
   - For each validator, discover its multiaddresses
   - This may involve additional API calls or peer discovery
   - Store validated multiaddresses with the validator

7. **Periodic Updates**:
   - The discovery process should be run periodically to keep validator information up-to-date
   - This ensures that changes in the network topology are reflected in the AddressDir

### Understanding Previous Caching System (NOT Implemented in dev_v3)

While we are NOT implementing caching in dev_v3, it's important to understand how caching worked in previous versions to avoid design conflicts:

1. **Previous Query Result Caching**:
   - Previous versions stored query results indexed by URL and query type
   - Inconsistent URL construction led to cache misses and redundant network requests

2. **Previous Problem Node Tracking**:
   - Previous versions tracked problematic nodes to avoid querying them
   - This required consistent validator identification across the codebase

**Note**: These caching features are documented here for reference only and will NOT be implemented in dev_v3.

### Standardization Requirements

To address these issues, the AddressDir implementation must:

1. **Use Consistent URL Construction**:
   - Always use `protocol.PartitionUrl(partitionID)` for constructing partition URLs
   - Follow the approach used in sequence.go and normalizeAnchorUrl

2. **Normalize URLs Before Use**:
   - Implement URL normalization similar to healSingleAnchorWithNormalizedUrls
   - Ensure all URL comparisons and lookups use normalized URLs

3. **Use the Correct API Version**:
   - Consistently use the v3 API for all network interactions
   - Ensure compatibility with the NetworkStatus API structure

4. **Handle Partition Relationships Correctly**:
   - Track that all validators participate in the DN
   - Track which BVN each validator participates in (every validator has exactly one BVN)
   - Use this information for routing and status determination

5. **Avoid Caching and Retry Logic**:
   - Do NOT implement query result caching
   - Do NOT implement transaction retry mechanisms
   - Focus on correct validator discovery and URL construction

6. **Partition Awareness**: By tracking partition information for each validator (DN and exactly one BVN), the design enables partition-specific operations and reporting, which is essential for the healing process. The implementation recognizes that all validators participate in the DN and exactly one BVN.

7. **Status Tracking**: The status field in the Validator struct allows for better decision-making when selecting validators for operations. This helps avoid using validators that are known to be problematic. A validator is considered active if it's active on either its BVN or the DN.

8. **Dynamic Updates**: The design is highly dynamic, allowing for runtime updates to validator information, addresses, and status. This is crucial for adapting to changing network conditions.

9. **Order Preservation**: Using slices preserves the insertion order of validators, which is beneficial for consistent reporting and UI display.

10. **Preferred Addresses**: The design supports marking certain addresses as preferred, which can improve reliability by prioritizing addresses that have been more successful.

11. **Consistency with heal_v1**: The implementation follows the same approach as the existing heal_v1 implementation for validator discovery and management, ensuring compatibility and reliability. It uses the NetworkStatus API to discover validators and their partition information, similar to how the ScanNetwork function works in the healing package.

## Testing Design

The AddressDir implementation requires thorough testing to ensure it can reliably connect to validators using multiaddresses for both private API access and transaction submission. The following testing approach will verify the functionality against real networks like kermint or mainnet.

### Test Structure

```go
package new_heal_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/require"
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
    "gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal"
)
```

### Test Cases

#### 1. Validator Discovery Test

Test the ability to discover validators from a network and populate the AddressDir:

```go
func TestValidatorDiscovery(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Test against different networks
    networks := []string{"kermint", "mainnet"}
    
    for _, network := range networks {
        t.Run(network, func(t *testing.T) {
            // Create a new AddressDir
            addressDir := new_heal.NewAddressDir()
            
            // Configure network endpoint
            endpoint := getNetworkEndpoint(network)
            
            // Discover validators and populate the AddressDir
            err := addressDir.DiscoverValidators(context.Background(), endpoint)
            require.NoError(t, err)
            
            // Verify that validators were discovered
            require.Greater(t, len(addressDir.Validators), 0)
            
            // Verify that each validator has at least one address
            for _, validator := range addressDir.Validators {
                require.NotEmpty(t, validator.Addresses)
                
                // Verify partition information is set
                require.NotEmpty(t, validator.PartitionID)
                require.NotEmpty(t, validator.PartitionType)
                
                // Verify URLs are set
                require.NotEmpty(t, validator.URLs)
            }
        })
    }
}
```

#### 2. Private API Access Test

Test the ability to access the private API using multiaddresses without requesting signatures:

```go
func TestPrivateAPIAccess(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Test against different networks
    networks := []string{"kermint", "mainnet"}
    
    for _, network := range networks {
        t.Run(network, func(t *testing.T) {
            // Create a new AddressDir
            addressDir := new_heal.NewAddressDir()
            
            // Configure network endpoint
            endpoint := getNetworkEndpoint(network)
            
            // Discover validators and populate the AddressDir
            err := addressDir.DiscoverValidators(context.Background(), endpoint)
            require.NoError(t, err)
            
            // Test private API access for each validator
            for _, validator := range addressDir.Validators {
                // Skip if the validator has no addresses
                if len(validator.Addresses) == 0 {
                    continue
                }
                
                // Get a client for the validator's address
                client, err := addressDir.GetClientForValidator(validator.ID)
                require.NoError(t, err)
                
                // Create a context with timeout
                ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
                defer cancel()
                
                // Test private API access using a lightweight method
                // We'll use a simple ping-like request that doesn't require a signature
                err = testPrivateAPIConnection(ctx, client, validator)
                
                // If successful, mark the address as validated
                if err == nil {
                    t.Logf("Successfully connected to validator %s (%s)", validator.ID, validator.Name)
                    addressDir.SetValidatorStatus(validator.ID, "active")
                } else {
                    t.Logf("Failed to connect to validator %s: %v", validator.ID, err)
                }
            }
            
            // Verify that at least one validator is accessible
            activeValidators := addressDir.GetActiveValidators()
            require.NotEmpty(t, activeValidators)
        })
    }
}

// testPrivateAPIConnection tests the connection to a validator's private API
// without requesting a signature
func testPrivateAPIConnection(ctx context.Context, client api.Client, validator *new_heal.Validator) error {
    // Instead of requesting a signature, we'll use a context timeout approach
    // to test if we can establish a connection to the private API
    
    // Get the private API client
    privateClient := client.ForAddress(multiaddr.StringCast(validator.Addresses[0].Address)).Private()
    
    // Create dummy URLs for a lightweight request
    srcUrl, _ := url.Parse(validator.URLs["partition"])
    dstUrl, _ := url.Parse("acc://dn.acme")
    
    // Set a very short timeout to avoid actually completing the request
    // We just want to verify we can connect to the private API
    shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
    defer cancel()
    
    // Attempt to make a request to the private API
    // The request will timeout, but we can check if we got a connection error or a timeout error
    _, err := privateClient.Sequence(shortCtx, srcUrl, dstUrl, 1, private.SequenceOptions{})
    
    // If we get a context deadline exceeded error, it means we successfully connected
    // but the request timed out as expected
    if err != nil && err.Error() == "context deadline exceeded" {
        return nil
    }
    
    // If we get a different error, it might be a connection error
    return err
}
```

#### 3. Transaction Submission Test

Test the ability to submit transactions using the AddressDir:

```go
func TestTransactionSubmission(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Test against different networks
    networks := []string{"kermint"}  // Only test on testnet to avoid mainnet transactions
    
    for _, network := range networks {
        t.Run(network, func(t *testing.T) {
            // Create a new AddressDir
            addressDir := new_heal.NewAddressDir()
            
            // Configure network endpoint
            endpoint := getNetworkEndpoint(network)
            
            // Discover validators and populate the AddressDir
            err := addressDir.DiscoverValidators(context.Background(), endpoint)
            require.NoError(t, err)
            
            // Create a test transaction
            tx, err := createTestTransaction()
            require.NoError(t, err)
            
            // Submit the transaction using the AddressDir
            response, err := addressDir.SubmitTransaction(context.Background(), tx)
            require.NoError(t, err)
            require.NotNil(t, response)
            
            // Verify the transaction was accepted
            require.NotEmpty(t, response.TransactionHash)
        })
    }
}

// createTestTransaction creates a simple test transaction
func createTestTransaction() (*protocol.Transaction, error) {
    // Create a simple data account transaction that won't affect anything
    // This is just for testing connectivity
    // In a real test, you might want to use a more meaningful transaction
    
    // Implementation details would depend on the specific transaction type
    // and the test account being used
    return nil, nil
}
```

#### 4. Address Validation and Preference Test

Test the address validation and preference functionality:

```go
func TestAddressValidationAndPreference(t *testing.T) {
    // Create a new AddressDir
    addressDir := new_heal.NewAddressDir()
    
    // Add a validator with multiple addresses
    validator := addressDir.AddValidator("validator1", "Test Validator", "bvn-Test", "bvn")
    
    // Add valid and invalid addresses
    validAddress := "/ip4/144.76.105.23/tcp/16593/p2p/QmValidPeerID"
    invalidAddress1 := "/ip4/144.76.105.23/tcp/16593"  // Missing peer ID
    invalidAddress2 := "invalid-address"
    
    // Test valid address
    err := addressDir.AddValidatorAddress(validator.ID, validAddress)
    require.NoError(t, err)
    
    // Test invalid addresses
    err = addressDir.AddValidatorAddress(validator.ID, invalidAddress1)
    require.Error(t, err)
    
    err = addressDir.AddValidatorAddress(validator.ID, invalidAddress2)
    require.Error(t, err)
    
    // Test address preference
    addresses, exists := addressDir.GetValidatorAddresses(validator.ID)
    require.True(t, exists)
    require.Equal(t, 1, len(addresses))
    
    // Mark the address as preferred
    addressDir.MarkAddressPreferred(validator.ID, validAddress)
    
    // Get preferred addresses
    preferredAddresses := addressDir.GetPreferredAddresses(validator.ID)
    require.Equal(t, 1, len(preferredAddresses))
    require.Equal(t, validAddress, preferredAddresses[0].Address)
}
```

### Helper Functions

```go
// getNetworkEndpoint returns the API endpoint for a given network
func getNetworkEndpoint(network string) string {
    switch network {
    case "kermint":
        return "http://testnet.accumulatenetwork.io:26660/v2"
    case "mainnet":
        return "https://mainnet.accumulatenetwork.io/v2"
    default:
        return "http://127.0.0.1:26660/v2"  // Local
    }
}
```

### Test Configuration

To make the tests configurable and avoid hardcoding values:

```go
type TestConfig struct {
    Networks       []string          // Networks to test against
    Endpoints      map[string]string // Network endpoints
    TestAccount    string            // Account to use for test transactions
    SkipPrivateAPI bool              // Skip private API tests
    SkipTxSubmit   bool              // Skip transaction submission tests
}

func LoadTestConfig() TestConfig {
    // Load configuration from environment variables or config file
    // This allows for flexible testing without code changes
    return TestConfig{
        Networks: []string{"kermint", "mainnet"},
        Endpoints: map[string]string{
            "kermint": "http://testnet.accumulatenetwork.io:26660/v2",
            "mainnet": "https://mainnet.accumulatenetwork.io/v2",
        },
        TestAccount:    os.Getenv("TEST_ACCOUNT"),
        SkipPrivateAPI: os.Getenv("SKIP_PRIVATE_API") == "true",
        SkipTxSubmit:   os.Getenv("SKIP_TX_SUBMIT") == "true",
    }
}
```

### Integration with CI/CD

The tests can be integrated into a CI/CD pipeline with appropriate flags to control which tests run:

```yaml
# Example GitHub Actions workflow
name: Integration Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    - name: Set up Go
      uses: actions/setup-go@v2
      with:
        go-version: 1.19
    - name: Unit Tests
      run: go test -v ./tools/cmd/debug/new_heal/...
    - name: Integration Tests (Kermint)
      run: go test -v ./tools/cmd/debug/new_heal/... -tags=integration -network=kermint
      if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    - name: Integration Tests (Mainnet)
      run: go test -v ./tools/cmd/debug/new_heal/... -tags=integration -network=mainnet -skip-tx-submit=true
      if: github.event_name == 'push' && github.ref == 'refs/heads/main'
```

---

> **⚠️ IMPORTANT: DO NOT DELETE THIS DEVELOPMENT PLAN ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS development plan (above) will not be deleted.
