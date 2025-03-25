# In-Memory Database Documentation

## 0. Overview

The following operations form the foundation of our in-memory database:

### 0.1. Basic Key-Value Operations:
- Get(key) - Retrieve a value by its 32-byte key
- Put(key, value) - Store a value with a 32-byte key
- Delete(key) - Remove a key-value pair

### 0.2. Batch Operations:
- Begin(writable) - Start a new transaction/batch
- Commit() - Commit changes to the database
- Discard() - Discard changes without committing

### 0.3. Initialization Operations:
- DetermineAnchorChainHeights() - Establish anchor chain heights across all partitions
- IdentifyMissingAnchorEntries() - Find anchor entries missing from destination chains
- MapSyntheticTransactions() - Identify synthetic transactions missing from destinations
- BuildChainIndexes() - Index all relevant chains for efficient lookups
- VerifyPartitionConnectivity() - Ensure all required partitions are accessible

### 0.4. Account Operations:
- Account(url) - Get an account by URL
- Account(url).Main() - Get the main chain of an account
- Account(url).Main().GetAs(&target) - Unmarshal account data into a struct
- Account(url).ChainByName(name) - Get a specific chain of an account

### 0.5. Chain Operations:
- Chain().EntryAs(index, &target) - Get a chain entry and unmarshal it
- Chain().Type() - Get the type of a chain
- Chain().Index() - Get the index chain of the chain

### 0.6. Indexing Operations:
- IndexAccountChains(ctx, url) - Index the chains of an account
- PullAccount(ctx, url) - Pull account data
- PullAccountWithChains(ctx, url, filterFunc) - Pull account data with chains

The in-memory database should mimic these operations while keeping all data in memory
without disk persistence. All keys should be 32-byte hashes, and values should be structs.

## 1. Basic Key-Value Operations

### 1.1. Overview
The basic key-value operations form the foundation of our in-memory database:

### 1.2. Data Structures
#### 1.2.1. Database struct
- Purpose: The main in-memory database structure
- Fields:
  * data: map[[32]byte]interface{} - The core key-value store
  * mutex: sync.RWMutex - For basic thread-safe access to the data
- Behavior:
  * Simple thread-safe access to the underlying key-value store
  * No persistence to disk
  * All keys are 32-byte hashes
  * Values are stored directly as Go objects (no serialization needed)
  * No defensive copying (assumes immutable data)

#### 1.2.2. NotFoundError struct
- Purpose: Error returned when a key is not found in the database
- Fields:
  * Key: [32]byte - The key that was not found
- Methods:
  * Error() string - Returns a formatted error message

#### 1.2.3. Interface definitions
- Database interface:
  * Get(key [32]byte) (interface{}, error)
  * Put(key [32]byte, value interface{}) error
  * Delete(key [32]byte) error
  * Begin(writable bool) Batch

### 1.3. Operations

#### 1.3.1. Get(key []byte) (interface{}, error)
- Purpose: Retrieves a value associated with a 32-byte key
- Parameters: key - 32-byte hash that uniquely identifies the value
- Returns: The stored value as a Go object and an error (nil if successful)
- Error cases: Returns an error if the key doesn't exist

#### 1.3.2. Put(key []byte, value interface{}) error
- Purpose: Stores a value with a 32-byte key
- Parameters: 
  * key - 32-byte hash that uniquely identifies the value
  * value - Go object to store
- Returns: Error if the operation fails, nil otherwise
- Behavior: Overwrites any existing value for the same key

#### 1.3.3. Delete(key []byte) error
- Purpose: Removes a key-value pair from the database
- Parameters: key - 32-byte hash of the entry to remove
- Returns: Error if the operation fails, nil otherwise
- Behavior: No error if the key doesn't exist

These operations will be implemented with thread-safety in mind to allow
concurrent access to the database.

## 2. Batch Operations

### 2.1. Overview
Batch operations allow for atomic transactions on the database:

### 2.2. Data Structures

#### 2.2.1. Batch struct
- Purpose: Represents a transaction/batch of database operations
- Fields:
  * db: *Database - Reference to the parent database
  * writable: bool - Whether the batch can modify data
  * changes: map[[32]byte]batchEntry - Pending changes in the batch
  * committed: bool - Whether the batch has been committed or discarded
  * mutex: sync.RWMutex - For basic thread-safe access to batch data
- Methods:
  * Get(key [32]byte) (interface{}, error) - Get a value, considering pending changes
  * Put(key [32]byte, value interface{}) error - Stage a value to be written
  * Delete(key [32]byte) error - Stage a key for deletion
  * Commit() error - Apply all changes to the database
  * Discard() - Discard all pending changes

#### 2.2.2. batchEntry struct
- Purpose: Represents a single change in a batch
- Fields:
  * value: interface{} - The new value (nil if this is a deletion)
  * deleted: bool - Whether this entry is marked for deletion

#### 2.2.3. Batch interface
- Methods:
  * Get(key [32]byte) (interface{}, error)
  * Put(key [32]byte, value interface{}) error
  * Delete(key [32]byte) error
  * Commit() error
  * Discard()

### 2.3. Additional Data Structures Required for Batch Operations

Batch operations require additional data structures beyond those needed for basic key-value operations:

- **Basic Key-Value Operations** use simple structures:
  * `Database` struct with a map and mutex for thread safety
  * `Key` type (32-byte array) for consistent key representation
  * Direct storage of Go objects as values (no serialization)

- **Batch Operations** require more complex structures:
  * `Batch` struct to track pending changes before they're committed
  * `batchEntry` struct to represent each individual change (new value or deletion)
  * Change tracking with a map of pending modifications
  * State management (writable/read-only, committed/active)
  * Atomic application of changes during commit

The batch system provides a way to stage multiple changes and apply them atomically, ensuring database consistency even when multiple operations are performed as a group.

### 2.4. Operations

#### 2.4.1. Begin(writable bool) Batch
- Purpose: Creates a new transaction/batch for a series of operations
- Parameters: writable - boolean indicating if the batch can modify data
- Returns: A new Batch object that can be used for database operations
- Behavior: 
  * Read-only batches (writable=false) can only perform Get operations
  * Writable batches (writable=true) can perform Get, Put, and Delete operations
  * Multiple batches can be active simultaneously

#### 2.4.2. Batch.Commit() error
- Purpose: Atomically applies all changes in the batch to the database
- Returns: Error if the commit fails, nil otherwise
- Behavior:
  * All operations in the batch are applied atomically (all or nothing)
  * After commit, the batch cannot be used for further operations
  * For read-only batches, this is a no-op (but still marks the batch as closed)

#### 2.4.3. Batch.Discard()
- Purpose: Discards all changes in the batch without applying them
- Behavior:
  * All pending operations in the batch are discarded
  * After discard, the batch cannot be used for further operations
  * This is the proper way to clean up a batch that won't be committed

The batch interface provides transaction semantics, ensuring that a series
of operations can be performed atomically, and either all succeed or all fail.

## 3. Initialization of the Database

### 3.1. Overview
The initialization process for the healing database requires gathering specific data from various partitions to identify missing entries that need to be healed. This section outlines the operations needed to properly initialize the database to support both anchor and synthetic transaction healing.

### 3.2. Required Operations

#### 3.2.1. Anchor Chain Height Determination
- Purpose: Establish the current height of anchor chains across all partitions
- Required data:
  * Directory Network (DN) anchor chain heights
  * Blockchain Validation Network (BVN) anchor chain heights
- Implementation:
  * Query each partition for its anchor chain height
  * Store these heights as reference points for healing operations
  * Use these heights to determine the range of entries to examine

#### 3.2.2. Missing Anchor Entry Identification
- Purpose: Identify anchor entries that exist in source chains but are missing from destination chains
- Directional flows:
  * Directory → BVNs (Directory anchors missing from BVNs)
  * BVNs → Directory (BVN anchors missing from Directory)
  * Note: BVNs do not exchange anchors with each other
- Implementation:
  * For each source partition, retrieve all anchor entries
  * For each destination partition, check if entries exist
  * Record missing entries for healing

#### 3.2.3. Synthetic Transaction Source/Destination Mapping
- Purpose: Identify synthetic transactions missing from destination chains
- Required data:
  * Source chains containing synthetic transactions
  * Destination chains where these transactions should be applied
- Implementation:
  * Map source chains to their corresponding destination chains
  * For each source, identify entries missing from destinations
  * Record missing synthetic transactions for healing

#### 3.2.4. Required Chains for Healing
- Purpose: Define the specific chains that must be tracked for healing on mainnet
- Directory Network (DN) Chains:
  * `acc://directory/anchor` - Directory anchor chain (source for DN→BVN anchors)
  * `acc://directory/synthetic` - Directory synthetic transaction chain

- BVN Anchor and Synthetic Chains:
  * `acc://bvn-apollo/anchor` - Apollo anchor chain (source for BVN→DN anchors)
  * `acc://bvn-apollo/synthetic` - Apollo synthetic transaction chain
  * `acc://bvn-artemis/anchor` - Artemis anchor chain
  * `acc://bvn-artemis/synthetic` - Artemis synthetic transaction chain
  * `acc://bvn-athena/anchor` - Athena anchor chain
  * `acc://bvn-athena/synthetic` - Athena synthetic transaction chain
  * `acc://bvn-demeter/anchor` - Demeter anchor chain
  * `acc://bvn-demeter/synthetic` - Demeter synthetic transaction chain
  * `acc://bvn-hera/anchor` - Hera anchor chain
  * `acc://bvn-hera/synthetic` - Hera synthetic transaction chain
  * `acc://bvn-hermes/anchor` - Hermes anchor chain
  * `acc://bvn-hermes/synthetic` - Hermes synthetic transaction chain
  * `acc://bvn-poseidon/anchor` - Poseidon anchor chain
  * `acc://bvn-poseidon/synthetic` - Poseidon synthetic transaction chain
  * `acc://bvn-zeus/anchor` - Zeus anchor chain
  * `acc://bvn-zeus/synthetic` - Zeus synthetic transaction chain

- Healing Flow Directions:
  * Directory → BVNs: Directory anchor entries to all BVNs
  * BVNs → Directory: Each BVN's anchor entries to Directory
  * Note: BVNs do not exchange anchors with each other

#### 3.2.5. P2P Network and Multiaddress Management
- Purpose: Maintain up-to-date peer node information and valid multiaddresses
- Required Data:
  * Peer node identifiers
  * Valid P2P multiaddresses (including IP, TCP, and P2P components)
  * Network partition assignments (DN or specific BVN)
  * Node status and connectivity information
- Implementation:
  * Discover peer nodes via JSON-RPC
  * Validate multiaddresses to ensure they contain required components:
    * IP component (e.g., `/ip4/144.76.105.23`)
    * TCP component (e.g., `/tcp/16593`)
    * P2P component with peer ID (e.g., `/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N`)
  * Filter out invalid addresses missing any required components
  * Store valid peer information in the database
  * Periodically update peer information to account for network changes
  * Track connection status and availability of each peer
- Dynamic Updates:
  * Schedule regular peer discovery and validation
  * Update peer information when network topology changes
  * Remove stale or unreachable peers
  * Add newly discovered peers
  * Maintain separate peer lists for each network partition

#### 3.2.6. Chain Indexing
- Purpose: Build indexes of all relevant chains to enable efficient lookups
- Required data:
  * Account chains across all partitions
  * Index chains for efficient entry verification
- Implementation:
  * Index all account chains relevant to healing
  * Build lookup structures for quick verification of entry existence
  * Optimize for the specific healing patterns required

#### 3.2.7. Partition Connectivity Verification
- Purpose: Ensure all required partitions are accessible for healing
- Implementation:
  * Verify connectivity to all Directory and BVN partitions
  * Record connection status for each partition
  * Provide fallback mechanisms for temporarily unavailable partitions

### 3.3. Database Initialization Workflow

The initialization process follows a specific sequence to ensure all necessary data is gathered before healing begins:

1. **Partition Discovery**: Identify and connect to all relevant Directory and BVN partitions
2. **Height Determination**: Query and record anchor chain heights for all partitions
3. **Entry Collection**: Gather all entries from source anchor chains
4. **Missing Entry Identification**: Compare source and destination chains to identify missing entries
5. **Synthetic Transaction Mapping**: Identify synthetic transactions and their required destinations
6. **Index Building**: Construct efficient indexes for all relevant chains
7. **Initialization Verification**: Confirm all required data has been collected and indexed

This initialization process creates the foundation for efficient healing operations by ensuring all necessary data is available and properly indexed in the in-memory database.

### 3.4. P2P Network Database Initialization

The following section outlines a simplified approach to initializing the in-memory database for P2P network operations. This approach is designed to be more straightforward than the current healing implementation while ensuring all necessary peer information is properly stored and accessible.

#### 3.4.1. Core Data Structures

```go
// PeerInfo stores essential information about a peer node
type PeerInfo struct {
    ID         string   // Peer ID (e.g., "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
    Addresses  []string // Valid multiaddresses (e.g., "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
    Partition  string   // Network partition (e.g., "directory", "bvn-apollo")
    LastSeen   int64    // Unix timestamp of last successful connection
    Status     string   // Connection status (e.g., "active", "unreachable")
}

// Simple key construction for peer storage
func PeerKey(peerID string) Key {
    return KeyFromString("peer:" + peerID)
}

// Simple key construction for partition-based peer lists
func PartitionPeersKey(partition string) Key {
    return KeyFromString("partition:" + partition)
}
```

#### 3.4.2. Basic Initialization Process

The initialization process follows these simple steps:

1. **Discover Peers**: Query a known endpoint to discover initial peers
2. **Validate Addresses**: Ensure all addresses are valid P2P multiaddresses
3. **Store Peer Information**: Save validated peer data to the database
4. **Create Partition Indexes**: Build simple indexes for quick partition-based lookups

```go
// Initialize P2P network database
func InitP2PDatabase(db *MemoryDB, endpoint string) error {
    // Step 1: Discover peers from a known endpoint
    peers, err := DiscoverPeersFromEndpoint(endpoint)
    if err != nil {
        return fmt.Errorf("peer discovery failed: %w", err)
    }
    
    // Step 2 & 3: Validate and store peer information
    batch := db.Begin(true)
    defer batch.Discard()
    
    partitionPeers := make(map[string][]string)
    
    for _, peer := range peers {
        // Validate and fix addresses if needed
        validAddresses := ValidateAddresses(peer.Addresses)
        if len(validAddresses) == 0 {
            continue // Skip peers with no valid addresses
        }
        
        // Store the peer information
        peerInfo := PeerInfo{
            ID:        peer.ID,
            Addresses: validAddresses,
            Partition: peer.Partition,
            LastSeen:  time.Now().Unix(),
            Status:    "active",
        }
        
        err = batch.Put(PeerKey(peer.ID), peerInfo)
        if err != nil {
            return fmt.Errorf("failed to store peer %s: %w", peer.ID, err)
        }
        
        // Track peers by partition for indexing
        partitionPeers[peer.Partition] = append(partitionPeers[peer.Partition], peer.ID)
    }
    
    // Step 4: Create partition indexes
    for partition, peerIDs := range partitionPeers {
        err = batch.Put(PartitionPeersKey(partition), peerIDs)
        if err != nil {
            return fmt.Errorf("failed to create index for partition %s: %w", partition, err)
        }
    }
    
    return batch.Commit()
}
```

#### 3.4.3. Address Validation and Conversion

A critical part of P2P initialization is ensuring all multiaddresses are valid. This simplified approach handles validation and conversion in a straightforward manner:

```go
// ValidateAddresses ensures all addresses are valid P2P multiaddresses
func ValidateAddresses(addresses []string) []string {
    var validAddresses []string
    
    for _, addrStr := range addresses {
        // Parse the multiaddress
        addr, err := multiaddr.NewMultiaddr(addrStr)
        if err != nil {
            continue // Skip invalid addresses
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
        
        // If address has all required components, add it to valid addresses
        if hasIP && hasTCP && hasP2P {
            validAddresses = append(validAddresses, addrStr)
            continue
        }
        
        // If address is missing P2P component but has IP and TCP, try to fix it
        if hasIP && hasTCP && !hasP2P {
            // This would be implemented by looking up the peer ID
            // and adding it to the address
            fixedAddr := TryFixAddress(addrStr)
            if fixedAddr != "" {
                validAddresses = append(validAddresses, fixedAddr)
            }
        }
    }
    
    return validAddresses
}

// TryFixAddress attempts to fix an address missing the P2P component
func TryFixAddress(addrStr string) string {
    // Implementation would look up the peer ID for this address
    // and append it to create a valid P2P multiaddress
    // This is a simplified placeholder
    return ""
}
```

#### 3.4.4. Simple Database Access Patterns

Once the database is initialized, accessing peer information follows these simple patterns:

```go
// Get information about a specific peer
func GetPeer(db *MemoryDB, peerID string) (*PeerInfo, error) {
    batch := db.Begin(false)
    defer batch.Discard()
    
    value, err := batch.Get(PeerKey(peerID))
    if err != nil {
        return nil, err
    }
    
    peer, ok := value.(PeerInfo)
    if !ok {
        return nil, fmt.Errorf("invalid peer data format")
    }
    
    return &peer, nil
}

// Get all peers for a specific partition
func GetPeersByPartition(db *MemoryDB, partition string) ([]PeerInfo, error) {
    batch := db.Begin(false)
    defer batch.Discard()
    
    value, err := batch.Get(PartitionPeersKey(partition))
    if err != nil {
        return nil, err
    }
    
    peerIDs, ok := value.([]string)
    if !ok {
        return nil, fmt.Errorf("invalid partition index format")
    }
    
    var peers []PeerInfo
    for _, peerID := range peerIDs {
        value, err := batch.Get(PeerKey(peerID))
        if err != nil {
            continue // Skip peers that can't be retrieved
        }
        
        peer, ok := value.(PeerInfo)
        if !ok {
            continue // Skip peers with invalid format
        }
        
        peers = append(peers, peer)
    }
    
    return peers, nil
}
```

#### 3.4.5. Periodic Updates

To keep the peer database current, implement a simple update mechanism:

```go
// Update peer information periodically
func UpdatePeerDatabase(db *MemoryDB, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    for range ticker.C {
        // Get current peers from the database
        currentPeers, err := GetAllPeers(db)
        if err != nil {
            continue
        }
        
        // For each peer, check if it's still reachable
        for _, peer := range currentPeers {
            // Try to connect to verify the peer is still active
            isActive := CheckPeerConnectivity(peer)
            
            // Update the peer's status
            UpdatePeerStatus(db, peer.ID, isActive)
        }
        
        // Discover new peers and add them to the database
        DiscoverAndAddNewPeers(db)
    }
}
```

#### 3.4.6. Complete Example

For a complete, working example of P2P network initialization that can be tested against the mainnet, see [Appendix D: P2P Network Initialization and Management](appendix.md#d-p2p-network-initialization-and-management).

The approach outlined here provides a simpler and more straightforward method for initializing and maintaining P2P network information in the in-memory database compared to the current healing implementation. It focuses on:

1. **Simplicity**: Clear, easy-to-understand data structures and functions
2. **Reliability**: Proper validation and handling of multiaddresses
3. **Efficiency**: Simple indexing for quick access by partition
4. **Maintainability**: Straightforward update mechanisms

This simplified approach makes it easier to understand and implement P2P network initialization while ensuring all necessary peer information is properly stored and accessible.

## 4. Account Operations

### 4.1. Overview
These operations provide account-level access to the database:

### 4.2. Data Structures
#### 4.2.1. AccountDB struct
- Purpose: Provides access to account data
- Fields:
  * db: Database - Reference to the parent database
  * key: [32]byte - The key for the account
- Methods:
  * Main() ChainDB - Gets the main chain of the account
  * ChainByName(name string) (ChainDB, error) - Gets a specific chain of the account

### 4.3. Standard Account Chains

Accumulate accounts have several chains associated with them. The standard chains that are common to most account types include:

1. **MainChain** - The primary chain that stores the main transaction history for an account. Contains the hash of each transaction that affects the account's state.

2. **ScratchChain** - Used for temporary storage during transaction processing. Holds intermediate data that may be needed during complex operations.

3. **SignatureChain** - Stores signatures and authorization information for the account. Contains a record of all signatures that have authorized transactions for this account.

#### Special-Purpose Chains

In addition to the standard chains, some account types have special-purpose chains:

- **RootChain** - Maintains the root state of certain accounts. Used for state validation and as a reference point for other chains.

- **BptChain** - Binary Patricia Tree chain that records state hashes at each block. It stores the previous state hash in each block, creating a verifiable history of state transitions. This chain is used for cryptographic verification of account states, but historical state validation requires the state at that point in time, as the BPT implementation doesn't support rewinding to previous states.

- **AnchorSequenceChain** - Records the sequence of anchors that reference an account. Used primarily by system accounts for cross-chain validation.

- **MajorBlockChain** - Tracks major block events related to an account. Used for synchronization and consensus by system accounts.

- **SyntheticSequenceChain** - Used by the protocol for synthetic transaction management.

#### Index Chains

Each chain in Accumulate has an associated index chain that facilitates efficient lookups and queries. For more detailed information about index chains, see [Appendix B: Account Chains](appendix.md#appendix-b-account-chains).

##### Referencing Account Chains and Index Chains

To access an account's chains and their associated index chains:

```go
// Get an account
account := batch.Account(accountUrl)

// Access a standard chain
mainChain := account.MainChain()

// Access the index chain for that chain
mainIndexChain := mainChain.Index()

// Convert to Chain objects for operations
mainChainObj, _ := mainChain.Get()
mainIndexChainObj, _ := mainIndexChain.Get()
```

The index chain relationship is always one-to-one: each chain has exactly one index chain, and each index chain is associated with exactly one parent chain. This design enables efficient querying and lookup operations without requiring full chain scans.

### 4.4. Operations

#### 4.4.1. Account(url *url.URL) AccountDB
- Purpose: Creates an AccountDB object for the specified account URL
- Parameters: url - The URL of the account
- Returns: An AccountDB object that provides access to the account's data
- Implementation: Uses KeyFromURL(url) to generate the account key

#### 4.4.2. AccountDB.Main() ChainDB
- Purpose: Gets the main chain of an account
- Returns: A ChainDB object for the account's main chain
- Implementation: Creates a composite key from the account key and "main"

#### 4.4.3. AccountDB.Main().GetAs(&target) - Unmarshal account data into a struct
- Purpose: Retrieves the account's main data
- Returns: The stored Go object and an error if retrieval fails
- Implementation: Gets the value directly from the database

#### 4.4.4. AccountDB.ChainByName(name string) (ChainDB, error)
- Purpose: Gets a specific chain of an account by name
- Parameters: name - The name of the chain
- Returns: A ChainDB object for the specified chain and an error if it fails
- Implementation: Creates a composite key from the account key and chain name

These account operations build on the key-value operations and provide a higher-level
interface for working with account data in the database.

## 5. Chain Operations

### 5.1. Overview
Chain operations provide access to chain data in the database:

### 5.2. Data Structures
#### 5.2.1. ChainDB struct
- Purpose: Provides access to chain data
- Fields:
  * db: Database - Reference to the parent database
  * key: [32]byte - The key for the chain
- Methods:
  * EntryAs(index uint64, target interface{}) error - Gets a chain entry
  * Type() (string, error) - Gets the type of the chain
  * Index() ChainDB - Gets the index chain of the chain

### 5.3. Operations

#### 5.3.1. Chain().EntryAs(index, &target)
- Purpose: Gets a chain entry and unmarshals it into a target struct
- Parameters: 
  * index - The index of the entry in the chain
  * target - A pointer to a struct to unmarshal the entry into
- Returns: Error if retrieval or unmarshaling fails
- Implementation: Uses a composite key of chain ID and index

#### 5.3.2. Chain().Type()
- Purpose: Gets the type of a chain
- Returns: The chain type as a string
- Implementation: Retrieves a special metadata entry for the chain

#### 5.3.3. Chain().Index()
- Purpose: Gets the index chain of a chain
- Returns: A ChainDB object for the index chain
- Implementation: Creates a composite key from the chain key and "index"

These chain operations provide a higher-level interface for working with chain data,
building on the key-value operations of the database.

## 6. Indexing Operations

### 6.1. Overview
Indexing operations help manage and access data relationships:

### 6.2. Data Structures
#### 6.2.1. IndexEntry struct
- Purpose: Represents an entry in an index
- Fields:
  * Key: [32]byte - The key of the indexed item
  * Value: interface{} - The value of the indexed item

### 6.3. Operations

#### 6.3.1. IndexAccountChains(ctx, url)
- Purpose: Indexes the chains of an account
- Parameters:
  * ctx - Context for the operation
  * url - The URL of the account to index
- Returns: Error if indexing fails
- Implementation: Creates index entries for each chain in the account

#### 6.3.2. PullAccount(ctx, url)
- Purpose: Pulls account data into the database
- Parameters:
  * ctx - Context for the operation
  * url - The URL of the account to pull
- Returns: Error if pulling fails
- Implementation: Retrieves account data and stores it in the database

#### 6.3.3. PullAccountWithChains(ctx, url, filterFunc)
- Purpose: Pulls account data with chains into the database
- Parameters:
  * ctx - Context for the operation
  * url - The URL of the account to pull
  * filterFunc - Function to filter which chains to pull
- Returns: Error if pulling fails
- Implementation: Retrieves account and chain data and stores it in the database

These indexing operations help manage the relationships between different data entities
in the database, making it easier to navigate and query the data.

## 7. Key Functions

### 7.0. Overview
For our in-memory database, we'll leverage Accumulate's existing key construction functions:

### 7.1. Operations

#### 7.1.1. KeyFromURL(url *url.URL) [32]byte
- Purpose: Converts a URL to a 32-byte key hash
- Implementation: Uses url.Hash32() which returns the hash of the URL as a [32]byte
- Usage: Primary way to convert account URLs to database keys

#### 7.1.2. KeyFromString(s string) [32]byte
- Purpose: Converts a string to a 32-byte key hash
- Implementation: Uses record.NewKey(s).Hash() to create a key from a string
- Usage: For simple string-based keys (like chain names)

#### 7.1.3. KeyFromValues(values ...any) [32]byte
- Purpose: Creates a composite key from multiple values
- Implementation: Uses record.NewKey(values...).Hash() to create a key from multiple values
- Usage: For complex keys that combine multiple values (e.g., account + chain + index)

#### 7.1.4. KeyFromBytes(b []byte) [32]byte
- Purpose: Creates a key directly from a byte slice
- Implementation: If len(b) == 32, copies directly; otherwise hashes the bytes
- Usage: For interoperability with existing byte-based keys

These key functions will be used throughout the database implementation to ensure
consistent key generation and lookup.

## 8. Initialization and Configuration

### 8.0. Overview
The in-memory database can be initialized and configured as follows:

### 8.1. Operations

#### 8.1.1. NewDatabase() *Database
- Purpose: Creates a new empty in-memory database
- Returns: A pointer to a new Database instance
- Implementation: Initializes the data map and mutex

#### 8.1.2. Database.WithMaxBatchSize(size int) *Database
- Purpose: Sets the maximum number of operations in a batch
- Parameters: size - The maximum number of operations
- Returns: The database instance for method chaining
- Implementation: Sets a configuration field in the database

#### 8.1.3. Database.WithTimeout(timeout time.Duration) *Database
- Purpose: Sets the timeout for database operations
- Parameters: timeout - The maximum duration for an operation
- Returns: The database instance for method chaining
- Implementation: Sets a configuration field in the database

#### 8.1.4. Database.WithLogger(logger Logger) *Database
- Purpose: Sets a logger for database operations
- Parameters: logger - The logger to use
- Returns: The database instance for method chaining
- Implementation: Sets a logger field in the database

These initialization and configuration options allow for flexible setup
of the in-memory database to suit different use cases.

## 9. Concurrency Considerations

### 9.0. Overview
The in-memory database is designed with simplified thread safety:

### 9.1. Database-level concurrency
- The main Database struct uses a sync.RWMutex to protect access to the data map
- Read operations (Get) acquire a read lock
- Write operations (Put, Delete) acquire a write lock
- Multiple concurrent reads are allowed
- Writes block all other operations

### 9.2. Batch-level concurrency
- Each Batch has its own mutex to protect its internal state
- Multiple batches can be active simultaneously
- Batch operations are isolated from each other until commit
- Commit operation acquires the database write lock to apply changes atomically

### 9.3. Simplified isolation guarantees
- Read-your-writes: A batch will see its own uncommitted changes
- No dirty reads: A batch will not see uncommitted changes from other batches
- No complex isolation levels or MVCC needed for this specific use case

### 9.4. No defensive copying
- Since we assume data won't be modified after insertion or retrieval
- We can store and return direct references to objects
- This improves performance but requires careful usage

These simplified concurrency considerations provide adequate thread safety for
the specific needs of heal_synth.go without the overhead of a more complex
concurrency control mechanism.

## 10. Error Handling

### 10.0. Overview
The following errors will be defined for the in-memory database:

### 10.1. ErrKeyNotFound
- Purpose: Returned when a key is not found in the database
- Type: Custom error type with the key that was not found
- Usage: Returned by Get when the key doesn't exist

### 10.2. ErrBatchClosed
- Purpose: Returned when an operation is attempted on a committed or discarded batch
- Type: Sentinel error value
- Usage: Returned by any batch operation after Commit or Discard

### 10.3. ErrReadOnlyBatch
- Purpose: Returned when a write operation is attempted on a read-only batch
- Type: Sentinel error value
- Usage: Returned by Put or Delete on a read-only batch

### 10.4. ErrInvalidKeyType
- Purpose: Returned when a key is not a 32-byte array
- Type: Sentinel error value
- Usage: Returned by any operation with an invalid key

### 10.5. ErrNilValue
- Purpose: Returned when nil is provided as a value to Put
- Type: Sentinel error value
- Usage: Returned by Put when the value is nil (use Delete instead)

These errors provide clear and specific information about what went wrong
during database operations, making debugging easier.

## 11. Implementation Simplifications

### 11.0. Overview
Since this in-memory database is specifically designed to support the analysis
and healing of Accumulate, we can make the following simplifying assumptions:

### 11.1. Immutable Data
- Data placed into the database won't be modified after insertion
- Data retrieved from the database won't be modified by the caller
- This allows us to store direct references without defensive copying

### 11.2. Single-Purpose Usage
- The database is tailored for heal_synth.go's specific needs
- No need for complex query capabilities or indexing
- Optimized for the specific access patterns of the healing process

### 11.3. Minimal Error Handling
- Focus on errors that are likely to occur in this specific use case
- Less emphasis on edge cases that won't arise in this controlled environment

### 11.4. Simplified Concurrency
- Basic thread safety is sufficient for the expected usage patterns
- No need for complex isolation levels or conflict resolution

These simplifications allow us to create a more streamlined implementation
that is perfectly suited for its intended purpose while avoiding unnecessary
complexity that would be required in a general-purpose database.

## Appendix

The appendix sections have been moved to a separate file for easier review and maintenance.
Please see [appendix.md](appendix.md) for the following sections:

- **Appendix A: Binary Patricia Tree (BPT) in Accumulate** - Details on the BPT implementation, its role, and limitations
- **Appendix B: Account Chains** - Examples of querying and working with account chains
- **Appendix C: API Reference** - Guide to using the Accumulate API for chain operations
- **Appendix D: Collecting Data from Mainnet for Testing** - Methods for gathering real-world data for testing