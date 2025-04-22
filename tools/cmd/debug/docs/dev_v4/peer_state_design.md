# PeerState Design Document

## Overview

The `PeerState` struct is designed to track and monitor the state of network peers in the Accumulate network, with a particular focus on blockchain heights and consensus participation. It works alongside the existing `AddressDir` struct, which manages the discovery and basic tracking of network peers.

## Design Goals

1. **Separation of Concerns**: Keep the `AddressDir` focused on peer discovery and basic tracking, while `PeerState` handles detailed state monitoring
2. **Non-Invasive Integration**: Work with the existing `AddressDir` implementation without requiring modifications to it
3. **Comprehensive Monitoring**: Track blockchain heights, consensus participation, and identify lagging nodes
4. **Efficient State Updates**: Provide mechanisms to efficiently update and query the state of peers

## Struct Definition

```go
// PeerState manages and tracks the state of network peers
type PeerState struct {
    // Reference to the AddressDir for peer information
    addressDir *AddressDir
    
    // Map of peer ID to peer state information
    peerStates map[string]PeerStateInfo
    
    // Mutex for concurrent access
    mu sync.RWMutex
}

// PeerStateInfo contains detailed state information for a peer
type PeerStateInfo struct {
    // Peer ID
    ID string
    
    // Whether this peer is a validator
    IsValidator bool
    
    // If this is a validator, the validator ID
    ValidatorID string
    
    // Partition information
    PartitionID string
    
    // Directory Network height
    DNHeight uint64
    
    // Block Validator Network height
    BVNHeight uint64
    
    // Whether this node is a zombie (not participating in consensus)
    IsZombie bool
    
    // Last time this peer was updated
    LastUpdated time.Time
}

// StateUpdateStats tracks statistics about a state update operation
type StateUpdateStats struct {
    // Total number of peers processed
    TotalPeers int
    
    // Number of peers with updated heights
    HeightUpdated int
    
    // Number of peers that became zombies
    NewZombies int
    
    // Number of peers that recovered from zombie status
    RecoveredZombies int
    
    // Maximum Directory Network height observed
    DNHeightMax uint64
    
    // Maximum Block Validator Network height observed
    BVNHeightMax uint64
    
    // Number of Directory Network nodes significantly behind
    DNLaggingNodes int
    
    // Number of Block Validator Network nodes significantly behind
    BVNLaggingNodes int
    
    // IDs of peers with updated heights
    HeightUpdatedIDs []string
    
    // IDs of peers that became zombies
    NewZombieIDs []string
    
    // IDs of peers that recovered from zombie status
    RecoveredZombieIDs []string
    
    // IDs of Directory Network nodes significantly behind
    DNLaggingNodeIDs []string
    
    // IDs of Block Validator Network nodes significantly behind
    BVNLaggingNodeIDs []string
}
```

## Key Methods

1. **NewPeerState(addressDir *AddressDir) *PeerState**
   - Create a new PeerState instance with a reference to an AddressDir
   - Initialize the peerStates map

2. **UpdatePeerStates(ctx context.Context, client api.NetworkService) (StateUpdateStats, error)**
   - Query the heights and zombie status of all peers
   - Update the peerStates map with the latest information
   - Return statistics about the update operation

3. **QueryNodeHeights(ctx context.Context, peerID string, host string) error**
   - Query the heights of a specific node for both DN and BVN partitions
   - Update the corresponding PeerStateInfo

4. **GetPeerState(peerID string) (PeerStateInfo, bool)**
   - Get the state information for a specific peer
   - Return a boolean indicating if the peer exists

5. **GetLaggingPeers(significantLag uint64) []PeerStateInfo**
   - Get a list of peers that are significantly behind the highest observed heights
   - The significantLag parameter defines how many blocks behind is considered "lagging"

6. **GetZombiePeers() []PeerStateInfo**
   - Get a list of peers that are currently marked as zombies

7. **GetPeersByPartition(partitionID string) []PeerStateInfo**
   - Get a list of peers belonging to a specific partition

## Implementation Details

### Height Collection Process

The process for collecting node heights follows these steps:

1. **Identify Node Endpoints**: For each peer in the AddressDir, determine its API endpoints:
   - Tendermint RPC endpoint (typically port 16592 for primary and 16692 for secondary)
   - Accumulate API endpoint (typically port 16595)

2. **Query Tendermint Status**: Use the Tendermint RPC client to query the node's status:
   ```go
   // Create a Tendermint HTTP client
   base := fmt.Sprintf("http://%s:%d", host, port) // port is 16592 or 16692
   c, err := http.New(base, base+"/ws")
   
   // Query the node status
   status, err := c.Status(ctx)
   
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
       peerState.DNHeight = height
   } else {
       peerState.BVNHeight = height
   }
   ```

5. **Check Zombie Status**: Determine if the node is a zombie (not participating in consensus):
   ```go
   // Query the consensus state
   consensusState, err := c.ConsensusState(ctx)
   
   // Check if the node is in the validator set and has recent votes
   // A node is considered a zombie if it's not participating in consensus
   ```

### Lagging Node Detection

The PeerState struct identifies nodes that are significantly behind the blockchain tip:

1. **Find Maximum Heights**: Determine the highest observed heights for both DN and BVN:
   ```go
   // Track the highest heights observed in the network
   var dnHeightMax, bvnHeightMax uint64
   
   // Find the maximum heights
   for _, state := range ps.peerStates {
       if state.DNHeight > dnHeightMax {
           dnHeightMax = state.DNHeight
       }
       if state.BVNHeight > bvnHeightMax {
           bvnHeightMax = state.BVNHeight
       }
   }
   ```

2. **Identify Lagging Nodes**: Find nodes that are significantly behind:
   ```go
   // Identify nodes that are significantly behind (more than 5 blocks)
   const significantLag = uint64(5)
   
   for peerID, state := range ps.peerStates {
       // Check DN height lag
       if state.DNHeight > 0 && dnHeightMax > 0 {
           if dnHeightMax - state.DNHeight > significantLag {
               stats.DNLaggingNodes++
               stats.DNLaggingNodeIDs = append(stats.DNLaggingNodeIDs, peerID)
           }
       }
       
       // Check BVN height lag
       if state.BVNHeight > 0 && bvnHeightMax > 0 {
           if bvnHeightMax - state.BVNHeight > significantLag {
               stats.BVNLaggingNodes++
               stats.BVNLaggingNodeIDs = append(stats.BVNLaggingNodeIDs, peerID)
           }
       }
   }
   ```

## Usage Example

```go
// Create a new AddressDir
addressDir := new_heal.NewAddressDir()

// Discover network peers
ctx := context.Background()
totalPeers, err := addressDir.DiscoverNetworkPeers(ctx, client)
if err != nil {
    log.Fatalf("Failed to discover network peers: %v", err)
}

// Create a new PeerState
peerState := new_heal.NewPeerState(addressDir)

// Update peer states
stats, err := peerState.UpdatePeerStates(ctx, client)
if err != nil {
    log.Fatalf("Failed to update peer states: %v", err)
}

// Print statistics
fmt.Printf("Total peers: %d\n", stats.TotalPeers)
fmt.Printf("Peers with updated heights: %d\n", stats.HeightUpdated)
fmt.Printf("Maximum DN height: %d\n", stats.DNHeightMax)
fmt.Printf("Maximum BVN height: %d\n", stats.BVNHeightMax)
fmt.Printf("DN lagging nodes: %d\n", stats.DNLaggingNodes)
fmt.Printf("BVN lagging nodes: %d\n", stats.BVNLaggingNodes)

// Get lagging peers
laggingPeers := peerState.GetLaggingPeers(5)
for _, peer := range laggingPeers {
    fmt.Printf("Lagging peer: ID=%s, DNHeight=%d, BVNHeight=%d\n", 
               peer.ID, peer.DNHeight, peer.BVNHeight)
}

// Get zombie peers
zombiePeers := peerState.GetZombiePeers()
for _, peer := range zombiePeers {
    fmt.Printf("Zombie peer: ID=%s, ValidatorID=%s\n", 
               peer.ID, peer.ValidatorID)
}
```

## Integration with AddressDir

The PeerState struct works alongside the AddressDir struct without modifying it:

1. **Initialization**: PeerState is initialized with a reference to an existing AddressDir
2. **Peer Discovery**: AddressDir handles peer discovery and basic tracking
3. **State Monitoring**: PeerState handles detailed state monitoring and tracking
4. **Refresh Operations**: AddressDir's RefreshNetworkPeers method continues to work as before, while PeerState's UpdatePeerStates method provides additional state tracking

This separation of concerns allows each component to focus on its specific responsibilities while working together to provide a comprehensive view of the network's state.
