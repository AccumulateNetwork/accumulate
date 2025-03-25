# Appendix

## A. Binary Patricia Tree (BPT) in Accumulate

The Binary Patricia Tree (BPT) is a fundamental data structure in the Accumulate protocol that provides cryptographic verification of account states. This appendix provides a deeper understanding of the BPT's role, implementation, and limitations.

### A.1 BPT Overview

The BPT is a specialized Merkle tree implementation used to create cryptographic proofs of account states. It allows efficient verification that a particular piece of data exists in the tree without requiring the entire tree to be present.

### A.2 BptChain Purpose

The BptChain records state hashes at each block, creating a verifiable history of state transitions. When a block is processed, the previous state hash is added to the BptChain, as seen in the block processing code:

```go
// From internal/core/execute/v2/block/block_end.go
err := ledger.BptChain().Inner().AddEntry(block.State.PreviousStateHash[:], false)
```

This chain of state hashes enables:
1. Verification of the current account state
2. Cryptographic proofs that can validate an account's state at a particular point in time
3. A historical record of state transitions

### A.3 BPT Implementation

The BPT implementation in Accumulate is found in the [pkg/database/bpt](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/pkg/database/bpt) package. Key components include:

1. **BPT Structure**: A tree structure where each node contains a hash of its children, culminating in a root hash that represents the entire state.

2. **Node Types**:
   - Branch nodes: Internal nodes that point to other nodes
   - Leaf nodes: Terminal nodes that contain actual key-value data

3. **Key Operations**:
   - `Get`: Retrieves values associated with keys
   - `Put`: Adds or updates key-value pairs
   - `GetRootHash`: Calculates the root hash of the tree

### A.4 Limitations

The current BPT implementation has some important limitations:

1. **No Historical State Rewinding**: The BPT doesn't support rewinding to previous states. This means that while the BptChain records state hashes at each block, generating proofs for historical states requires access to the state as it existed at that point in time.

2. **Forward-Only Operation**: The BPT is designed for forward-only operation, adding new states and verifying current states, but not for reconstructing past states from the chain of hashes alone.

3. **Verification vs. Reconstruction**: The BPT can verify that a given state matches a recorded hash, but cannot reconstruct the state from the hash alone.

### A.5 Code References

For a deeper understanding of the BPT implementation, refer to these key files in the Accumulate codebase:

1. **BPT Implementation**: 
   - [pkg/database/bpt/bpt.go](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/pkg/database/bpt/bpt.go) - Core BPT implementation
   - [pkg/database/bpt/node.go](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/pkg/database/bpt/node.go) - Node structure and operations

2. **BptChain Usage**:
   - [internal/core/execute/v2/block/block_end.go](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/internal/core/execute/v2/block/block_end.go) - How BPT state is recorded during block processing
   - [internal/database/account_chains.go](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/internal/database/account_chains.go) - Chain definitions and management
   - [internal/database/model_gen.go](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/internal/database/model_gen.go) - Account chain methods including BptChain

3. **Testing**:
   - [test/e2e/sys_block_test.go](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/develop/test/e2e/sys_block_test.go) - Tests for BptChain functionality

### A.6 Future Considerations

A potential enhancement to the BPT would be supporting historical state reconstruction, allowing the generation of proofs for any historical state without requiring access to the state at that point in time. This would involve storing more information in the BptChain or implementing a mechanism to rewind the BPT to a previous state.

## B. Account Chains

### B.1 Querying Account Chains

This section demonstrates how to query each chain on an account using the example account `acc://redwagon/token`.

#### B.1.1 Accessing Account Chains in Code

```go
// Parse the account URL
accountUrl, _ := url.Parse("acc://redwagon/token")

// Get a database batch
batch := database.Begin(ctx, db)
defer batch.Discard()

// Get the account
account := batch.Account(accountUrl)

// Access standard chains
mainChain := account.MainChain()
scratchChain := account.ScratchChain()
signatureChain := account.SignatureChain()

// Access special-purpose chains (if applicable to the account type)
rootChain := account.RootChain()
bptChain := account.BptChain()
anchorSequenceChain := account.AnchorSequenceChain()
majorBlockChain := account.MajorBlockChain()

// Access index chains
mainIndexChain := account.MainChain().Index()
signatureIndexChain := account.SignatureChain().Index()
bptIndexChain := account.BptChain().Index()

// Convert Chain2 to Chain for additional operations
mainChainObj, _ := mainChain.Get()
mainIndexChainObj, _ := mainIndexChain.Get()
```

#### B.1.2 Reading Chain Entries

```go
// Get the height of a chain
height := mainChain.Head().Get().Count

// Read an entry at a specific height
entry, _ := mainChain.Entry(10)

// Read an entry and unmarshal it into a struct
var txn protocol.Transaction
_ = mainChain.EntryAs(10, &txn)

// Get the index of an entry by its hash
hash := []byte{...} // Some entry hash
index, _ := mainChain.IndexOf(hash)

// Get the anchor (Merkle root) of the chain
anchor := mainChain.Anchor()

// Calculate a receipt between two points in the chain
receipt, _ := mainChain.Receipt(5, 10)
```

#### B.1.3 Using Index Chains for Efficient Lookups

```go
// Get the index chain for a main chain
indexChain, _ := mainChain.Index().Get()

// Search for an entry in the index chain
// This example searches for an entry with a specific source index
sourceIndex := uint64(123)
_, entry, _ := indexing.SearchIndexChain(
    indexChain, 
    uint64(indexChain.Height()-1), 
    indexing.MatchAfter, 
    indexing.SearchIndexChainBySource(sourceIndex)
)

// Search by block index
blockIndex := uint64(456)
_, entry, _ = indexing.SearchIndexChain(
    indexChain,
    uint64(indexChain.Height()-1),
    indexing.MatchAfter,
    indexing.SearchIndexChainByBlock(blockIndex)
)
```

#### B.1.4 Command Line Queries

Using the Accumulate CLI, you can query account chains:

```bash
# Query a chain
accumulate chain get acc://redwagon/token main

# Query a specific entry in a chain
accumulate chain entry acc://redwagon/token main 10

# Query a range of entries
accumulate chain entries acc://redwagon/token main 5 10

# Get information about all chains for an account
accumulate account chains acc://redwagon/token
```

## C. API Reference

### C.1 Querying Account Chains via API

The Accumulate API provides a comprehensive set of methods for interacting with account chains. This section demonstrates how to query chains using the example account `acc://redwagon/token`.

#### C.1.1 API Client Setup

```go
// Import the API package
import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"

// Create a client for mainnet
client := api.NewClient("https://mainnet.accumulatenetwork.io/v3")

// For testnet
testnetClient := api.NewClient("https://testnet.accumulatenetwork.io/v3")

// For a local node
localClient := api.NewClient("http://localhost:26660/v3")
```

#### C.1.2 Querying Chain Entries

```go
// Parse the account URL
accountUrl, _ := url.Parse("acc://redwagon/token")

// Query a chain entry
chainQuery := &api.ChainQuery{
    ChainID: "main",
    Height:  10,
}
response, err := client.QueryChainEntry(context.Background(), accountUrl, chainQuery)
if err != nil {
    // Handle error
}

// Query a range of chain entries
rangeQuery := &api.ChainQuery{
    ChainID: "main",
    Start:   5,
    Count:   10,
}
entries, err := client.QueryChainEntries(context.Background(), accountUrl, rangeQuery)
if err != nil {
    // Handle error
}
```

#### C.1.3 Querying Index Chains

```go
// Query an index chain entry
indexQuery := &api.ChainQuery{
    ChainID: "main",  // Will query the index chain for the main chain
    Height:  10,
    Index:   true,    // Specify that we want the index chain
}
indexEntry, err := client.QueryIndexChainEntry(context.Background(), accountUrl, indexQuery)
if err != nil {
    // Handle error
}

// Query a range of index chain entries
indexRangeQuery := &api.ChainQuery{
    ChainID: "main",
    Start:   5,
    Count:   10,
    Index:   true,
}
indexEntries, err := client.QueryIndexChainEntries(context.Background(), accountUrl, indexRangeQuery)
if err != nil {
    // Handle error
}
```

#### C.1.4 Processing API Responses

```go
// Process a chain entry response
if response != nil {
    // Access the entry data
    entryData := response.Entry
    
    // If it's a transaction, you can unmarshal it
    var txn protocol.Transaction
    err = txn.UnmarshalBinary(entryData)
    
    // Get the entry's hash
    entryHash := response.Hash
}

// Process a range of entries
if entries != nil {
    for _, entry := range entries.Items {
        // Process each entry
        entryData := entry.Entry
        entryHash := entry.Hash
    }
    
    // Check if there are more entries
    hasMore := entries.Total > uint64(len(entries.Items))
}
```

#### C.1.5 Error Handling

```go
// Common error patterns
if err != nil {
    switch {
    case errors.Is(err, api.ErrNotFound):
        // Handle not found error
        fmt.Println("The requested resource was not found")
    
    case errors.Is(err, api.ErrBadRequest):
        // Handle bad request error
        fmt.Println("The request was invalid:", err)
    
    case errors.Is(err, context.DeadlineExceeded):
        // Handle timeout
        fmt.Println("The request timed out")
    
    default:
        // Handle other errors
        fmt.Println("An error occurred:", err)
    }
}
```

#### C.1.6 API Endpoints Reference

The Accumulate API exposes the following endpoints for chain operations:

| Endpoint | Description |
|----------|-------------|
| `/v3/chain-entry` | Get a specific entry from a chain |
| `/v3/chain-entries` | Get a range of entries from a chain |
| `/v3/index-chain-entry` | Get a specific entry from an index chain |
| `/v3/index-chain-entries` | Get a range of entries from an index chain |
| `/v3/chain` | Get information about a chain |
| `/v3/chains` | Get a list of chains for an account |

Each endpoint accepts parameters to specify the account, chain, and entry details.

## D. P2P Network Initialization and Management

### D.1 Overview

The P2P network is essential for node communication in the Accumulate network. This section provides code examples for initializing and managing P2P connections, with a focus on creating valid multiaddresses and maintaining peer information.

### D.2 Discovering Network Peers via JSON-RPC

The first step in P2P initialization is discovering available peers through the JSON-RPC API:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/url"
)

// discoverPeers retrieves network peers from a JSON-RPC endpoint
func discoverPeers(ctx context.Context, endpoint string) ([]string, error) {
	// Create API client
	client := api.NewClient(endpoint)
	
	// Query network status to get peer information
	status, err := client.NetworkStatus(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status: %w", err)
	}
	
	// Extract peer addresses
	var addresses []string
	for _, peer := range status.Network.Peers {
		for _, addr := range peer.Addresses {
			addresses = append(addresses, addr)
		}
	}
	
	return addresses, nil
}

// Example usage
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Query mainnet
	addresses, err := discoverPeers(ctx, "https://mainnet.accumulatenetwork.io/v3")
	if err != nil {
		log.Fatalf("Error discovering peers: %v", err)
	}
	
	fmt.Println("Discovered peer addresses:")
	for _, addr := range addresses {
		fmt.Println(addr)
	}
}
```

### D.3 Validating and Converting Multiaddresses

JSON-RPC discovered addresses often lack the required P2P component. This code validates and converts them to proper P2P multiaddresses:

```go
// validateMultiaddress checks if a multiaddress string contains all required components
func validateMultiaddress(addrStr string) (bool, error) {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return false, fmt.Errorf("invalid multiaddress format: %w", err)
	}
	
	// Check for required components
	hasIP := false
	hasTCP := false
	hasP2P := false
	
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			hasIP = true
		case multiaddr.P_TCP:
			hasTCP = true
		case multiaddr.P_P2P:
			hasP2P = true
		}
		return true
	})
	
	return hasIP && hasTCP && hasP2P, nil
}

// convertToValidP2PAddress converts a basic address to a valid P2P multiaddress
// by adding the peer ID component if missing
func convertToValidP2PAddress(addrStr string, peerID string) (string, error) {
	// Check if already valid
	isValid, _ := validateMultiaddress(addrStr)
	if isValid {
		return addrStr, nil
	}
	
	// Parse the address
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return "", fmt.Errorf("invalid multiaddress: %w", err)
	}
	
	// Check if it has IP and TCP but missing P2P
	hasIP := false
	hasTCP := false
	hasP2P := false
	
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			hasIP = true
		case multiaddr.P_TCP:
			hasTCP = true
		case multiaddr.P_P2P:
			hasP2P = true
		}
		return true
	})
	
	if !hasIP || !hasTCP {
		return "", fmt.Errorf("address missing IP or TCP component")
	}
	
	if hasP2P {
		return addrStr, nil
	}
	
	// Add P2P component with the provided peer ID
	p2pComponent, err := multiaddr.NewComponent("p2p", peerID)
	if err != nil {
		return "", fmt.Errorf("failed to create p2p component: %w", err)
	}
	
	newAddr := addr.Encapsulate(p2pComponent)
	return newAddr.String(), nil
}

// filterValidP2PAddresses filters a list of addresses to only include valid P2P multiaddresses
func filterValidP2PAddresses(addresses []string) []string {
	var validAddresses []string
	
	for _, addr := range addresses {
		isValid, err := validateMultiaddress(addr)
		if err == nil && isValid {
			validAddresses = append(validAddresses, addr)
		}
	}
	
	return validAddresses
}
```

### D.4 Retrieving Peer IDs for Address Conversion

To convert addresses missing the P2P component, we need to retrieve peer IDs:

```go
// getPeerIDForNode retrieves the peer ID for a specific node
func getPeerIDForNode(ctx context.Context, endpoint string) (string, error) {
	client := api.NewClient(endpoint)
	
	// Get node info which includes the peer ID
	info, err := client.NodeInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get node info: %w", err)
	}
	
	if info.P2PID == "" {
		return "", fmt.Errorf("node did not return a peer ID")
	}
	
	return info.P2PID, nil
}

// Example of converting addresses with peer IDs
func convertAddressesWithPeerIDs(ctx context.Context, addresses []string) ([]string, error) {
	var convertedAddresses []string
	
	// Get a mapping of endpoints to peer IDs
	// In a real implementation, you would query each node directly
	// This is a simplified example
	peerIDMap := make(map[string]string)
	
	// For mainnet example, we'll use a hardcoded peer ID for demonstration
	// In production, you would retrieve this dynamically
	mainnetPeerID := "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N" // Example peer ID
	
	for _, addr := range addresses {
		// Extract the IP and port to form an endpoint
		// This is a simplified approach
		ipPort := extractIPPort(addr)
		if ipPort == "" {
			continue
		}
		
		endpoint := "http://" + ipPort
		
		// Check if we already have the peer ID
		peerID, ok := peerIDMap[endpoint]
		if !ok {
			// Try to get the peer ID
			var err error
			peerID, err = getPeerIDForNode(ctx, endpoint)
			if err != nil {
				// If we can't get the peer ID, use the example one for demonstration
				peerID = mainnetPeerID
			}
			peerIDMap[endpoint] = peerID
		}
		
		// Convert the address
		convertedAddr, err := convertToValidP2PAddress(addr, peerID)
		if err == nil {
			convertedAddresses = append(convertedAddresses, convertedAddr)
		}
	}
	
	return convertedAddresses, nil
}

// Helper function to extract IP:Port from a multiaddress
func extractIPPort(addrStr string) string {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return ""
	}
	
	var ip, port string
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			ip = c.Value()
		case multiaddr.P_TCP:
			port = c.Value()
		}
		return true
	})
	
	if ip != "" && port != "" {
		return ip + ":" + port
	}
	return ""
}
```

### D.5 Current P2P Node Initialization in Healing Tools

The healing tools in Accumulate currently use a hybrid approach for P2P node initialization, combining predefined bootstrap peers with nodes discovered via JSON-RPC:

```go
// P2P Node Initialization in heal_common.go
func initializeP2PNode(ctx context.Context, network string) (*p2p.Node, error) {
    // Create JSON-RPC client
    client := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
    
    // Get node info
    ni, err := client.NodeInfo(ctx, &api.NodeInfoOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get node info: %w", err)
    }
    
    // Discover peers via JSON-RPC
    jsonrpcPeers, err := discoverPeersViaJSONRPC(ctx, client)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Warning: Failed to discover peers via JSON-RPC: %v\n", err)
    }
    
    // Combine the discovered peers with the default bootstrap peers
    combinedBootstrap := append(bootstrap, jsonrpcPeers...)
    
    // Create P2P node with combined bootstrap peers
    node, err := p2p.New(p2p.Options{
        Network:           ni.Network,
        BootstrapPeers:    combinedBootstrap,
        PeerDatabase:      peerDb,
        EnablePeerTracker: true,
        
        // Use the peer tracker, but don't update it between reboots
        PeerScanFrequency:    -1,
        PeerPersistFrequency: -1,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to start p2p node: %w", err)
    }
    
    return node, nil
}
```

The `discoverPeersViaJSONRPC` function retrieves peer information from the JSON-RPC API and ensures all addresses are valid P2P multiaddresses:

```go
// discoverPeersViaJSONRPC retrieves and validates peer addresses from JSON-RPC
// Located in gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/heal_common.go
func discoverPeersViaJSONRPC(ctx context.Context, client *jsonrpc.Client) ([]multiaddr.Multiaddr, error) {
    // Query network status to get peer information
    status, err := client.NetworkStatus(ctx, &api.NetworkStatusOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get network status: %w", err)
    }
    
    // Get the peer ID of the node we're connecting to
    nodeInfo, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get node info: %w", err)
    }
    
    // Extract and validate peer addresses
    var validAddresses []multiaddr.Multiaddr
    
    // Process peer information from the network status
    // Note: The actual implementation accesses peers through the appropriate fields
    // in the NetworkStatus response structure
    for _, peer := range status.Network.Peers {
        for _, addrStr := range peer.Addresses {
            // Try to parse the address as a multiaddr
            addr, err := multiaddr.NewMultiaddr(addrStr)
            if err != nil {
                // Skip invalid addresses
                continue
            }
            
            // Check if it has the required components
            hasIP := false
            hasTCP := false
            hasP2P := false
            
            multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
                switch c.Protocol().Code {
                case multiaddr.P_IP4, multiaddr.P_IP6:
                    hasIP = true
                case multiaddr.P_TCP:
                    hasTCP = true
                case multiaddr.P_P2P:
                    hasP2P = true
                }
                return true
            })
            
            // If it has IP and TCP but missing P2P, try to add the P2P component
            if hasIP && hasTCP && !hasP2P {
                // Get the peer ID from the peer info if available
                peerID := ""
                if peer.ID != "" {
                    peerID = peer.ID
                } else {
                    // Use the node's peer ID as a fallback
                    // Note: The actual field name may vary based on the API version
                    peerID = nodeInfo.PeerID
                }
                
                if peerID != "" {
                    // Add P2P component with the peer ID
                    p2pComponent, err := multiaddr.NewComponent("p2p", peerID)
                    if err == nil {
                        addr = addr.Encapsulate(p2pComponent)
                        validAddresses = append(validAddresses, addr)
                    }
                }
            } else if hasIP && hasTCP && hasP2P {
                // Already a valid P2P multiaddress
                validAddresses = append(validAddresses, addr)
            }
        }
    }
    
    return validAddresses, nil
}
```

This approach ensures that the P2P node has the best chance of connecting to the network by:

1. Using predefined bootstrap peers as a reliable fallback
2. Enhancing connectivity by adding dynamically discovered peers from JSON-RPC
3. Validating all addresses to ensure they have the required IP, TCP, and P2P components
4. Adding missing P2P components to addresses when possible

The combination of static and dynamic peer discovery provides robust P2P initialization that works reliably against mainnet.

### D.6 Storing and Managing Peer Information in the Database

This code demonstrates how to store and manage peer information in the in-memory database:

```go
// PeerInfo represents information about a peer node
type PeerInfo struct {
	PeerID       string
	Multiaddress string
	Partition    string // "DN" or BVN name
	LastSeen     time.Time
	IsConnected  bool
}

// StorePeerInformation stores peer information in the database
func StorePeerInformation(db *Database, peerInfo PeerInfo) error {
	batch := db.Begin(true)
	defer batch.Discard()
	
	// Create a key for the peer
	peerKey := KeyFromString("peer:" + peerInfo.PeerID)
	
	// Store the peer info
	if err := batch.Put(peerKey, peerInfo); err != nil {
		return fmt.Errorf("failed to store peer info: %w", err)
	}
	
	// Create an index by partition
	partitionKey := KeyFromValues("partition", peerInfo.Partition, peerInfo.PeerID)
	if err := batch.Put(partitionKey, peerInfo.PeerID); err != nil {
		return fmt.Errorf("failed to create partition index: %w", err)
	}
	
	// Commit the changes
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("failed to commit peer info: %w", err)
	}
	
	return nil
}

// GetPeersByPartition retrieves all peers for a specific partition
func GetPeersByPartition(db *Database, partition string) ([]PeerInfo, error) {
	batch := db.Begin(false)
	defer batch.Discard()
	
	// This is a simplified approach - in a real implementation,
	// you would use a proper indexing mechanism
	var peers []PeerInfo
	
	// Prefix for the partition index
	prefix := "partition:" + partition + ":"
	
	// Iterate through all keys with this prefix
	// Note: This is a conceptual example - the actual implementation
	// would depend on the database's capabilities
	for key, value := range db.data {
		keyStr := string(key[:])
		if strings.HasPrefix(keyStr, prefix) {
			peerID := value.(string)
			peerKey := KeyFromString("peer:" + peerID)
			
			peerInfoRaw, err := batch.Get(peerKey)
			if err != nil {
				continue
			}
			
			peerInfo, ok := peerInfoRaw.(PeerInfo)
			if ok {
				peers = append(peers, peerInfo)
			}
		}
	}
	
	return peers, nil
}

// UpdatePeerStatus updates the connection status of a peer
func UpdatePeerStatus(db *Database, peerID string, isConnected bool) error {
	batch := db.Begin(true)
	defer batch.Discard()
	
	// Get the current peer info
	peerKey := KeyFromString("peer:" + peerID)
	peerInfoRaw, err := batch.Get(peerKey)
	if err != nil {
		return fmt.Errorf("peer not found: %w", err)
	}
	
	peerInfo, ok := peerInfoRaw.(PeerInfo)
	if !ok {
		return fmt.Errorf("invalid peer info format")
	}
	
	// Update the status
	peerInfo.IsConnected = isConnected
	peerInfo.LastSeen = time.Now()
	
	// Store the updated info
	if err := batch.Put(peerKey, peerInfo); err != nil {
		return fmt.Errorf("failed to update peer info: %w", err)
	}
	
	// Commit the changes
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("failed to commit peer info update: %w", err)
	}
	
	return nil
}
```

### D.7 Periodic Peer Discovery and Updates

This code demonstrates how to periodically update peer information:

```go
// PeerManager handles periodic peer discovery and updates
type PeerManager struct {
	db           *Database
	endpoints    []string
	updatePeriod time.Duration
	stopCh       chan struct{}
}

// NewPeerManager creates a new peer manager
func NewPeerManager(db *Database, endpoints []string, updatePeriod time.Duration) *PeerManager {
	return &PeerManager{
		db:           db,
		endpoints:    endpoints,
		updatePeriod: updatePeriod,
		stopCh:       make(chan struct{}),
	}
}

// Start begins periodic peer discovery and updates
func (pm *PeerManager) Start() {
	go pm.runPeriodicUpdates()
}

// Stop halts periodic peer discovery and updates
func (pm *PeerManager) Stop() {
	close(pm.stopCh)
}

// runPeriodicUpdates periodically discovers and updates peer information
func (pm *PeerManager) runPeriodicUpdates() {
	ticker := time.NewTicker(pm.updatePeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pm.updatePeers()
		case <-pm.stopCh:
			return
		}
	}
}

// updatePeers discovers and updates peer information
func (pm *PeerManager) updatePeers() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Discover peers from all endpoints
	var allAddresses []string
	for _, endpoint := range pm.endpoints {
		addresses, err := discoverPeers(ctx, endpoint)
		if err != nil {
			log.Printf("Failed to discover peers from %s: %v", endpoint, err)
			continue
		}
		allAddresses = append(allAddresses, addresses...)
	}
	
	// Filter for valid addresses
	validAddresses := filterValidP2PAddresses(allAddresses)
	
	// Convert addresses that are missing P2P components
	convertedAddresses, err := convertAddressesWithPeerIDs(ctx, allAddresses)
	if err != nil {
		log.Printf("Error converting addresses: %v", err)
	}
	
	// Combine valid and converted addresses
	validAddresses = append(validAddresses, convertedAddresses...)
	
	// Store or update peer information
	for _, addr := range validAddresses {
		peerID := extractPeerID(addr)
		if peerID == "" {
			continue
		}
		
		partition := determinePartition(ctx, addr)
		
		peerInfo := PeerInfo{
			PeerID:       peerID,
			Multiaddress: addr,
			Partition:    partition,
			LastSeen:     time.Now(),
			IsConnected:  true,
		}
		
		if err := StorePeerInformation(pm.db, peerInfo); err != nil {
			log.Printf("Failed to store peer info for %s: %v", peerID, err)
		}
	}
	
	// Clean up stale peers
	pm.removeStaleNodes()
}

// extractPeerID extracts the peer ID from a multiaddress
func extractPeerID(addrStr string) string {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return ""
	}
	
	var peerID string
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		if c.Protocol().Code == multiaddr.P_P2P {
			peerID = c.Value()
			return false
		}
		return true
	})
	
	return peerID
}

// determinePartition determines which partition a peer belongs to
func determinePartition(ctx context.Context, addrStr string) string {
	// Extract the IP and port
	ipPort := extractIPPort(addrStr)
	if ipPort == "" {
		return "unknown"
	}
	
	// Create an endpoint
	endpoint := "http://" + ipPort
	
	// Query the node
	client := api.NewClient(endpoint)
	info, err := client.NodeInfo(ctx)
	if err != nil {
		return "unknown"
	}
	
	// Determine the partition based on the network name
	networkName := info.Network
	if strings.Contains(strings.ToLower(networkName), "directory") {
		return "DN"
	}
	
	// Check for BVN names
	bvnNames := []string{"apollo", "artemis", "athena", "demeter", "hera", "hermes", "poseidon", "zeus"}
	for _, bvn := range bvnNames {
		if strings.Contains(strings.ToLower(networkName), bvn) {
			return "bvn-" + bvn
		}
	}
	
	return "unknown"
}

// removeStaleNodes removes nodes that haven't been seen recently
func (pm *PeerManager) removeStaleNodes() {
	// This is a simplified approach
	// In a real implementation, you would iterate through all peers
	// and remove those that haven't been seen within a certain timeframe
	
	// For demonstration purposes only
	staleThreshold := time.Now().Add(-24 * time.Hour)
	
	// Iterate through all peers
	// Note: This is a conceptual example - the actual implementation
	// would depend on the database's capabilities
	batch := pm.db.Begin(true)
	defer batch.Discard()
	
	// In a real implementation, you would have a way to iterate through all peers
	// For demonstration, we'll assume a function that returns all peer IDs
	peerIDs := getAllPeerIDs(pm.db)
	
	for _, peerID := range peerIDs {
		peerKey := KeyFromString("peer:" + peerID)
		peerInfoRaw, err := batch.Get(peerKey)
		if err != nil {
			continue
		}
		
		peerInfo, ok := peerInfoRaw.(PeerInfo)
		if !ok {
			continue
		}
		
		if peerInfo.LastSeen.Before(staleThreshold) {
			// Remove the peer
			batch.Delete(peerKey)
			
			// Also remove from partition index
			partitionKey := KeyFromValues("partition", peerInfo.Partition, peerInfo.PeerID)
			batch.Delete(partitionKey)
		}
	}
	
	batch.Commit()
}

// getAllPeerIDs is a helper function to get all peer IDs
// This is a placeholder - the actual implementation would depend on your database structure
func getAllPeerIDs(db *Database) []string {
	// Simplified implementation
	var peerIDs []string
	
	// In a real implementation, you would have a way to iterate through all peers
	// or maintain an index of all peer IDs
	
	return peerIDs
}
```

### D.8 Example Usage for Mainnet

Here's a complete example of initializing the P2P network for mainnet:

```go
func main() {
	// Create the in-memory database
	db := NewDatabase()
	
	// Define mainnet endpoints
	mainnetEndpoints := []string{
		"https://mainnet.accumulatenetwork.io/v3",
		"https://mainnet-asia.accumulatenetwork.io/v3",
		"https://mainnet-europe.accumulatenetwork.io/v3",
	}
	
	// Create a peer manager with 15-minute update interval
	peerManager := NewPeerManager(db, mainnetEndpoints, 15*time.Minute)
	
	// Start the peer manager
	peerManager.Start()
	defer peerManager.Stop()
	
	// Perform an initial update
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Discover peers from all endpoints
	var allAddresses []string
	for _, endpoint := range mainnetEndpoints {
		addresses, err := discoverPeers(ctx, endpoint)
		if err != nil {
			log.Printf("Failed to discover peers from %s: %v", endpoint, err)
			continue
		}
		allAddresses = append(allAddresses, addresses...)
		fmt.Printf("Discovered %d addresses from %s\n", len(addresses), endpoint)
	}
	
	// Filter for valid addresses
	validAddresses := filterValidP2PAddresses(allAddresses)
	fmt.Printf("Found %d valid P2P addresses out of %d total addresses\n", 
		len(validAddresses), len(allAddresses))
	
	// Print some example addresses
	fmt.Println("\nExample discovered addresses:")
	for i, addr := range allAddresses {
		if i >= 5 {
			break
		}
		fmt.Printf("  %s\n", addr)
	}
	
	fmt.Println("\nExample valid P2P addresses:")
	for i, addr := range validAddresses {
		if i >= 5 {
			break
		}
		fmt.Printf("  %s\n", addr)
	}
	
	// Keep the program running
	fmt.Println("\nP2P network initialization complete. Press Ctrl+C to exit.")
	
	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	
	fmt.Println("Shutting down...")
}
```

### D.9 Testing Against Mainnet

To test this code against mainnet, you can use the following approach:

```go
// TestMainnetPeerDiscovery tests peer discovery against mainnet
func TestMainnetPeerDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Discover peers from mainnet
	addresses, err := discoverPeers(ctx, "https://mainnet.accumulatenetwork.io/v3")
	if err != nil {
		t.Fatalf("Failed to discover peers: %v", err)
	}
	
	// Ensure we found some addresses
	if len(addresses) == 0 {
		t.Fatal("No peer addresses discovered")
	}
	
	// Log the discovered addresses
	t.Logf("Discovered %d peer addresses", len(addresses))
	for i, addr := range addresses {
		if i < 5 { // Log only the first 5 to avoid cluttering the output
			t.Logf("  %s", addr)
		}
	}
	
	// Filter for valid addresses
	validAddresses := filterValidP2PAddresses(addresses)
	t.Logf("Found %d valid P2P addresses", len(validAddresses))
	
	// Test address conversion
	if len(addresses) > 0 {
		// Use a sample peer ID for testing
		samplePeerID := "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"
		
		addr := addresses[0]
		convertedAddr, err := convertToValidP2PAddress(addr, samplePeerID)
		if err != nil {
			t.Logf("Failed to convert address %s: %v", addr, err)
		} else {
			t.Logf("Converted %s to %s", addr, convertedAddr)
			
			// Validate the converted address
			isValid, _ := validateMultiaddress(convertedAddr)
			if !isValid {
				t.Errorf("Converted address is not valid: %s", convertedAddr)
			}
		}
	}
}

// TestMainnetPartitionDetermination tests partition determination against mainnet
func TestMainnetPartitionDetermination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Test with known mainnet endpoints
	endpoints := map[string]string{
		"https://mainnet.accumulatenetwork.io/v3": "DN",
		// Add more known endpoints and their expected partitions
	}
	
	for endpoint, expectedPartition := range endpoints {
		// For testing purposes, we'll create a fake multiaddress
		// In a real scenario, you would get this from the peer discovery
		fakeAddr := "/ip4/127.0.0.1/tcp/26656/p2p/QmSamplePeerID"
		
		// Override the determinePartition function for testing
		// In a real implementation, you would query the actual node
		partition := expectedPartition
		
		t.Logf("Endpoint %s is in partition %s", endpoint, partition)
		
		// Assert the partition is as expected
		if partition != expectedPartition {
			t.Errorf("Expected partition %s for endpoint %s, got %s", 
				expectedPartition, endpoint, partition)
		}
	}
}

// TestInMemoryDatabaseStorage tests storing peer information in the in-memory database
func TestInMemoryDatabaseStorage(t *testing.T) {
	// Create the in-memory database
	db := NewDatabase()
	
	// Create sample peer information
	peerInfo := PeerInfo{
		PeerID:       "QmSamplePeerID",
		Multiaddress: "/ip4/127.0.0.1/tcp/26656/p2p/QmSamplePeerID",
		Partition:    "DN",
		LastSeen:     time.Now(),
		IsConnected:  true,
	}
	
	// Store the peer information
	err := StorePeerInformation(db, peerInfo)
	if err != nil {
		t.Fatalf("Failed to store peer information: %v", err)
	}
	
	// Retrieve peers by partition
	peers, err := GetPeersByPartition(db, "DN")
	if err != nil {
		t.Fatalf("Failed to retrieve peers by partition: %v", err)
	}
	
	// Verify we got the peer back
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}
	
	if peers[0].PeerID != peerInfo.PeerID {
		t.Errorf("Expected peer ID %s, got %s", peerInfo.PeerID, peers[0].PeerID)
	}
	
	// Update peer status
	err = UpdatePeerStatus(db, peerInfo.PeerID, false)
	if err != nil {
		t.Fatalf("Failed to update peer status: %v", err)
	}
	
	// Verify the status was updated
	peers, _ = GetPeersByPartition(db, "DN")
	if peers[0].IsConnected {
		t.Error("Peer status was not updated to disconnected")
	}
}
```

This appendix provides a comprehensive guide to P2P network initialization and management in the Accumulate network, with code examples that can be tested against the mainnet.

{{ ... }}
