# Accumulate Staking Client Design

## Overview

The Accumulate Staking Client is a specialized tool for querying and monitoring staking-related accounts on the Accumulate blockchain network. It focuses specifically on staking account collection and validation, separate from the core light client functionality.

## Purpose and Scope

### Primary Goals
- Query and retrieve staking account information from the Accumulate network
- Monitor staking registry and validator accounts
- Provide staking-specific account validation and verification
- Support staking operations and account management

### Out of Scope
- Operators keybook collection (handled by light client)
- General account types (ADI, TokenAccount, DataAccount, etc.)
- Core network authority validation

## Architecture

### Core Components

The staking client is built around these key components:

1. **Staking Client** (`pkg/stakingclient`): Core package for staking account queries
2. **Staking Tool** (`tools/staking-client`): Command-line tool for staking operations
3. **Account Types**: Specialized handling for staking-related account types

### Package Structure

```
tools/staking-client/
├── main.go           # Main executable for staking client
├── design.md         # This design document
└── README.md         # Usage documentation

pkg/stakingclient/
├── client.go         # Core staking client implementation
├── accounts.go       # Staking account types and methods
├── registry.go       # Staking registry handling
├── validator.go      # Validator account handling
└── README.md         # Package documentation
```

## Staking Account Types

### Supported Account Types

The staking client focuses on these Accumulate account types:

1. **Staking Registry Accounts**: Central registry for staking information
2. **Validator Accounts**: Individual validator account states
3. **Delegation Accounts**: Accounts representing delegated stakes
4. **Reward Accounts**: Accounts tracking staking rewards

### Account Type Integration

Using existing Accumulate protocol types from `protocol/types_gen.go`:

```go
// Staking client package structures
package stakingclient

import (
    "time"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
    "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// StakingChainState represents blockchain state for staking accounts
type StakingChainState struct {
    // BlockHeight is the current block height for this account state
    BlockHeight uint64 `json:"blockHeight"`
    
    // BlockIndex is the index within the block
    BlockIndex uint64 `json:"blockIndex"`
    
    // TransactionHash is the hash of the transaction that created this state
    TransactionHash []byte `json:"transactionHash,omitempty"`
    
    // ChainHead is the current chain head hash
    ChainHead []byte `json:"chainHead,omitempty"`
    
    // Timestamp is when this state was recorded
    Timestamp time.Time `json:"timestamp"`
    
    // SequenceNumber is the sequence number for this state
    SequenceNumber uint64 `json:"sequenceNumber"`
    
    // MerkleState contains the merkle tree state for cryptographic verification
    MerkleState *merkle.State `json:"merkleState,omitempty"`
    
    // ChainMetadata provides chain information
    ChainMetadata []protocol.ChainMetadata `json:"chainMetadata,omitempty"`
}

// StakingAccountWrapper wraps staking accounts with chain state
type StakingAccountWrapper struct {
    // Account is the underlying staking account
    Account protocol.Account `json:"account"`
    
    // ChainState contains blockchain state information
    ChainState *StakingChainState `json:"chainState"`
    
    // LastUpdated is when this state was last refreshed
    LastUpdated time.Time `json:"lastUpdated"`
    
    // Source indicates where this data came from (network, cache, etc.)
    Source string `json:"source"`
}
```

## Implementation Approach

### Network-Only Architecture

Like the light client, the staking client uses a network-only approach:

- **Direct Network Queries**: All staking data is fetched directly from the Accumulate network
- **No Caching**: No persistent storage or in-memory caching
- **Fresh Data**: Every query returns the most current state from the network
- **Stateless**: The staking client maintains no persistent state between runs

### Staking Client Interface

```go
// StakingClient provides methods for querying staking accounts
type StakingClient struct {
    client *lightclient.Client // Reuse core client functionality
}

// NewStakingClient creates a new staking client
func NewStakingClient(serverURL string) *StakingClient {
    return &StakingClient{
        client: lightclient.NewClient(serverURL),
    }
}

// GetStakingRegistry retrieves the staking registry account
func (sc *StakingClient) GetStakingRegistry(registryURL string) (*StakingAccountWrapper, error) {
    // Implementation queries network and wraps staking registry
}

// GetValidator retrieves a validator account
func (sc *StakingClient) GetValidator(validatorURL string) (*StakingAccountWrapper, error) {
    // Implementation queries network and wraps validator account
}

// ListValidators retrieves all validators from the staking registry
func (sc *StakingClient) ListValidators() ([]*StakingAccountWrapper, error) {
    // Implementation queries registry and retrieves all validator accounts
}
```

### Direct Network Queries

The staking client queries the network directly without caching:

```go
// GetValidator retrieves a validator account directly from the network
func (sc *StakingClient) GetValidator(validatorURL string) (*StakingAccountWrapper, error) {
    // Query the network using existing client methods
    response, err := sc.client.Query(validatorURL)
    if err != nil {
        return nil, err
    }
    
    // Use existing protocol unmarshaling for staking account types
    var account protocol.Account // Specific staking account type
    if err := account.UnmarshalJSON(response.Data); err != nil {
        return nil, err
    }
    
    // Extract chain state and merkle information from response
    chainState := extractStakingChainState(response)
    
    return &StakingAccountWrapper{
        Account:     account,
        ChainState:  chainState,
        LastUpdated: time.Now(),
        Source:      "network",
    }, nil
}
```

## Command-Line Tool

### Staking Client Executable

The `tools/staking-client/main.go` provides a command-line interface:

```go
package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    
    "gitlab.com/accumulatenetwork/accumulate/pkg/stakingclient"
)

func main() {
    var (
        serverURL = flag.String("server", "local", "Accumulate server URL")
        command   = flag.String("cmd", "list", "Command: list, get, monitor")
        account   = flag.String("account", "", "Account URL for get command")
    )
    flag.Parse()
    
    client := stakingclient.NewStakingClient(*serverURL)
    
    switch *command {
    case "list":
        validators, err := client.ListValidators()
        if err != nil {
            log.Fatal(err)
        }
        for _, validator := range validators {
            fmt.Printf("Validator: %s\n", validator.Account.GetUrl())
        }
        
    case "get":
        if *account == "" {
            log.Fatal("Account URL required for get command")
        }
        validator, err := client.GetValidator(*account)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("Validator: %+v\n", validator)
        
    case "monitor":
        // Implementation for monitoring staking accounts
        
    default:
        fmt.Printf("Unknown command: %s\n", *command)
        os.Exit(1)
    }
}
```

## Usage Examples

### List All Validators

```bash
./staking-client -server testnet -cmd list
```

### Get Specific Validator

```bash
./staking-client -server testnet -cmd get -account acc://validator1.acme
```

### Monitor Staking Registry

```bash
./staking-client -server testnet -cmd monitor
```

## Integration with Light Client

### Separation of Concerns

- **Light Client**: Focuses on operators keybook collection and core network authorities
- **Staking Client**: Focuses on staking registry, validators, and staking-related accounts
- **Shared Infrastructure**: Both use the same underlying `pkg/lightclient` for network queries

### Code Reuse

The staking client reuses core functionality from the light client:

```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/lightclient"

type StakingClient struct {
    client *lightclient.Client // Reuse network query functionality
}
```

## Future Enhancements

### Potential Features

1. **Staking Rewards Tracking**: Monitor and calculate staking rewards
2. **Delegation Management**: Track delegation states and changes
3. **Validator Performance**: Monitor validator uptime and performance metrics
4. **Staking Analytics**: Provide analytics and reporting on staking data

### Extensibility

The staking client is designed to be extensible for future staking-related functionality while maintaining clear separation from the core light client.
