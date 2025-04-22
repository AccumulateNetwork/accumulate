# Network Status Design Document

## Overview

This document outlines the design for a new network status facility in the `new_heal` package. The purpose of this facility is to efficiently collect and display information about all peers in the Accumulate network, with a focus on providing data that is useful for healing operations.

## Goals

1. Collect comprehensive peer information from the network
2. Provide partition-specific views of network status
3. Identify validator nodes and their roles
4. Display multiaddress information for direct node communication
5. Show synthetic ledger sequence status for each partition pair
6. Identify potential healing opportunities based on network state

## Design Approach

### Command Structure

The network status functionality will be implemented as a subcommand of the `new-heal` command:

```
accumulate debug new-heal network-status [flags]
```

Flags will include:
- `--endpoint`: API endpoint to use for initial connection
- `--partition`: Filter results to a specific partition
- `--format`: Output format (text, json, yaml)
- `--verbose`: Show additional details

### Data Collection Process

1. **Initial Connection**:
   - Connect to the specified endpoint
   - Query basic network information

2. **Network Discovery**:
   - Identify all partitions in the network (DN and BVNs)
   - Discover all peers in each partition
   - Collect validator information

3. **Peer Information Collection**:
   - For each peer, collect:
     - Peer ID
     - Multiaddresses
     - Validator status
     - Partition membership
     - Operator information

4. **Synthetic Ledger Status**:
   - For each partition pair:
     - Query synthetic ledger sequence status
     - Identify produced/received/delivered counts
     - Detect potential healing opportunities

### Data Structure

```go
type NetworkStatus struct {
    NetworkID     string
    Partitions    map[string]*PartitionStatus
    Validators    map[string]*ValidatorInfo
    SyntheticData map[string]map[string]*SequenceStatus
}

type PartitionStatus struct {
    ID      string
    Type    string // "dn" or "bvn"
    Peers   map[string]*PeerInfo
    Version string
}

type PeerInfo struct {
    ID         string
    Addresses  []string // Multiaddresses
    Validator  bool
    KeyHash    []byte
    Operator   string
    Partition  string
    Status     string // "active", "inactive", etc.
}

type SequenceStatus struct {
    SourcePartition      string
    DestinationPartition string
    Produced             uint64
    Received             uint64
    Delivered            uint64
    PendingDelivery      uint64 // Received - Delivered
    PendingReceipt       uint64 // Produced by source - Received by destination
}
```

### API Usage

The implementation will use the following APIs:

1. **Network Discovery**:
   - `api.NetworkService.NetworkStatus()`: Get overall network status
   - `api.NodeService.NodeInfo()`: Get information about specific nodes

2. **Peer Information**:
   - `api.ConsensusService.ConsensusStatus()`: Get validator information
   - `api.NetworkService.PeerInfo()`: Get peer connection information

3. **Synthetic Ledger Status**:
   - `api.Querier.QueryUrl()`: Query synthetic ledger accounts
   - Parse sequence information from account data

### URL Construction

Following the guidelines in our URL documentation:

1. **Partition URLs**:
   ```go
   dnUrl := protocol.PartitionUrl("dn")            // acc://dn.acme
   bvnUrl := protocol.PartitionUrl("bvn-Apollo")   // acc://bvn-Apollo.acme
   ```

2. **Synthetic Ledger URLs**:
   ```go
   dnSynthUrl := dnUrl.JoinPath(protocol.Synthetic)            // acc://dn.acme/synthetic
   bvnSynthUrl := bvnUrl.JoinPath(protocol.Synthetic)          // acc://bvn-Apollo.acme/synthetic
   ```

### Multiaddress Handling

For each peer, we'll collect and format multiaddresses following the libp2p format:
```
/ip4/127.0.0.1/tcp/16593/p2p/12D3KooWL4RkUCgJiaNQG7wo8ZdkfbUr5V8W8MfTgTzgC2obqMBH
```

This will allow direct communication with specific nodes when needed for healing operations.

## Output Format

### Text Format (Default)

```
Network Status for Accumulate Mainnet
=====================================

Partitions:
  - Directory Network (DN): acc://dn.acme
    - Peers: 3 (3 validators)
  - BVN0 (Apollo): acc://bvn-Apollo.acme
    - Peers: 3 (3 validators)
  - BVN1 (Yutu): acc://bvn-Yutu.acme
    - Peers: 3 (3 validators)
  - BVN2 (Chandrayaan): acc://bvn-Chandrayaan.acme
    - Peers: 3 (3 validators)

Validators:
  - 12D3KooWL4RkUCgJiaNQG7wo8ZdkfbUr5V8W8MfTgTzgC2obqMBH
    - Partition: DN
    - Operator: acc://operator1.acme
    - Addresses: 
      - /ip4/123.456.789.012/tcp/16593/p2p/12D3KooWL4RkUCgJiaNQG7wo8ZdkfbUr5V8W8MfTgTzgC2obqMBH
  [...]

Synthetic Ledger Status:
  - DN → Apollo:
    - Produced: 9121
    - Received by destination: 9121
    - Delivered: 9121
    - Pending: 0
  [...]

Healing Opportunities:
  - None detected
```

### JSON Format

Structured JSON output for programmatic consumption, following the data structure defined above.

## Implementation Plan

1. **Phase 1**: Basic network discovery and peer information collection
   - Create command structure
   - Implement network and peer discovery
   - Display basic information

2. **Phase 2**: Synthetic ledger status collection
   - Query synthetic ledger accounts
   - Parse sequence information
   - Identify potential healing opportunities

3. **Phase 3**: Enhanced output and filtering
   - Implement different output formats
   - Add filtering capabilities
   - Provide detailed healing recommendations

## Integration with Healing Operations

The network status facility will provide the foundation for more targeted healing operations:

1. **Automatic Healing Detection**:
   - Identify missing synthetic transactions
   - Detect undelivered anchors

2. **Targeted Healing Commands**:
   - Heal specific synthetic transactions
   - Heal anchors between specific partitions

3. **Batch Healing Operations**:
   - Heal all pending transactions for a partition pair
   - Prioritize healing operations based on importance

## Conclusion

This network status facility will provide a comprehensive view of the Accumulate network, with a focus on information that is useful for healing operations. By collecting and displaying peer information, synthetic ledger status, and potential healing opportunities, it will enable more efficient and targeted healing operations.
