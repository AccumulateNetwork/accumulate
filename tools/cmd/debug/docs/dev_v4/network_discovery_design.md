# Network Discovery Design Document

## Overview

The Network Discovery module is responsible for discovering and maintaining information about validators and peers in the Accumulate Network. This document outlines the design and implementation of the network discovery functionality, focusing on how it integrates with the AddressDir structure and how it handles network state updates.

## Key Components

### NetworkDiscovery

The `NetworkDiscovery` structure is the main component responsible for discovering network peers. It works in conjunction with the `AddressDir` structure to maintain a directory of validators and peers.

```go
type NetworkDiscovery struct {
    addressDir *AddressDir
    logger     *log.Logger
    networkType string // "mainnet", "testnet", or "devnet"
}
```

### Validator

Represents a validator node in the network with its associated information:

```go
type Validator struct {
    PeerID        string
    Name          string
    PartitionID   string
    PartitionType string
    BVN           int
    Status        string
    LastUpdated   time.Time
    
    // Addresses
    P2PAddress     string
    RPCAddress     string
    APIAddress     string
    MetricsAddress string
    IPAddress      string
    
    // Additional data
    URLs          map[string]string
    Addresses     []ValidatorAddress
    AddressStatus map[string]string
}
```

### NetworkPeer

Represents a peer in the network, which may or may not be a validator:

```go
type NetworkPeer struct {
    ID                   string
    ValidatorID          string
    PartitionID          string
    IsValidator          bool
    Addresses            []NetworkAddress
    Status               string
    LastSeen             time.Time
    FirstSeen            time.Time
    PartitionURL         string
    DiscoveryMethod      string
    DiscoveredAt         time.Time
    SuccessfulConnections int
}
```

## Network Discovery Flowchart

```
┌─────────────────────┐
│ DiscoverNetworkPeers│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Initialize Statistics│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Discover Directory  │
│ Network Peers       │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Discover BVN        │
│ Validators          │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Discover Common     │
│ Non-Validators      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Update AddressDir   │
│ State               │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Calculate Statistics│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Return Statistics   │
└─────────────────────┘
```

## Discovery Process

1. **Initialize Network**: Set up the network type (mainnet, testnet, devnet) and initialize the address directory with known network information.

2. **Discover Directory Network Peers**: 
   - Query the network status for the Directory Network
   - Extract validator information from the response
   - Create validator entries for active DN validators
   - Extract multiaddresses from validator information

3. **Discover BVN Validators**:
   - For each known BVN partition (Apollo, Chandrayaan, Yutu, etc.)
   - Query the network status for that partition
   - Create validator entries for active validators
   - Extract multiaddresses from validator information

4. **Discover Common Non-Validators**:
   - Add known public API nodes
   - Add known seed nodes
   - Add bootstrap nodes

5. **Update AddressDir State**:
   - Update the DNValidators list
   - Update the BVNValidators map
   - Update the NetworkPeers map
   - Track changes (new peers, lost peers, updated peers)

## State Management

When the network discovery process runs with an existing state in the AddressDir, it performs the following operations:

1. **Preserve Existing Information**:
   - Maintain peer history (first seen, last seen)
   - Preserve connection statistics
   - Keep track of previously discovered addresses

2. **Update Existing Entries**:
   - Update validator status (active/inactive)
   - Add new discovered addresses
   - Update last seen timestamp for active peers
   - Update connection statistics

3. **Add New Entries**:
   - Add newly discovered validators
   - Add newly discovered peers
   - Set appropriate discovery method and timestamp

4. **Handle Lost Peers**:
   - Mark peers as inactive if not seen in the current discovery
   - Keep track of peers that have disappeared
   - Maintain a history of peer availability

5. **Track Changes**:
   - Record new peers discovered in this run
   - Record peers that were lost in this run
   - Record peers whose status changed in this run
   - Calculate statistics about the network state

## Discovery Statistics

The discovery process maintains statistics about the network state and changes:

```go
type RefreshStats struct {
    TotalPeers           int
    NewPeers             int
    LostPeers            int
    UpdatedPeers         int
    UnchangedPeers       int
    
    TotalValidators      int
    TotalNonValidators   int
    ActiveValidators     int
    InactiveValidators   int
    UnreachableValidators int
    ProblematicValidators int
    
    DNValidators         int
    BVNValidators        int
    
    DNLaggingNodes       int
    BVNLaggingNodes      int
    
    StatusChanged        int
    
    NewPeerIDs           []string
    LostPeerIDs          []string
    ChangedPeerIDs       []string
    
    DNLaggingNodeIDs     []string
    BVNLaggingNodeIDs    []string
}
```

## Address Extraction

The network discovery process extracts addresses from validator information using several methods:

1. **Multiaddress Parsing**: Parse multiaddresses from validator information
2. **Known Address Lookup**: Use a map of known addresses for validators
3. **URL Extraction**: Extract host information from URLs
4. **Fallback Addresses**: Use hardcoded addresses for known validators

## Integration with AddressDir

The NetworkDiscovery module integrates with the AddressDir structure to maintain a directory of validators and peers:

1. **Validator Management**:
   - Add validators to the appropriate lists (DNValidators, BVNValidators)
   - Update validator information
   - Track validator status

2. **Peer Management**:
   - Add peers to the NetworkPeers map
   - Update peer information
   - Track peer status

3. **Address Management**:
   - Add addresses to validators and peers
   - Validate multiaddresses
   - Extract host information

## Error Handling

The network discovery process includes robust error handling:

1. **Partition Errors**: Handle errors when querying specific partitions
2. **Address Parsing Errors**: Handle errors when parsing multiaddresses
3. **Network Errors**: Handle network connectivity issues
4. **API Errors**: Handle errors from the API service

## Testing

The network discovery functionality includes comprehensive testing:

1. **Unit Tests**: Test individual components and functions
2. **Integration Tests**: Test the interaction between components
3. **Mainnet Tests**: Test against the actual mainnet
4. **Mock Tests**: Test with mock network services

## Future Enhancements

1. **Periodic Refresh**: Automatically refresh the network state at regular intervals
2. **Health Monitoring**: Monitor validator health and availability
3. **P2P Integration**: Integrate with the P2P node discovery mechanism
4. **Address Validation**: Improve validation of multiaddresses
5. **Peer Ranking**: Rank peers based on performance metrics
