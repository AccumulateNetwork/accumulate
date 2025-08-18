// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

/*
Package client provides a high-level Go SDK for interacting with Accumulate networks.

# Overview

The client package offers a unified interface to all Accumulate API versions (V2, V3, Private, and Ethereum-compatible),
with support for multiple transport protocols and network configurations.

# Basic Usage

Create a client and query an account:

	import (
	    "context"
	    "fmt"
	    "gitlab.com/accumulatenetwork/accumulate/pkg/client"
	)

	func main() {
	    // Connect to testnet
	    c, err := client.NewTestnet()
	    if err != nil {
	        log.Fatal(err)
	    }

	    // Query an account
	    account, err := c.GetAccount(context.Background(), "acc://mytoken.acme")
	    if err != nil {
	        log.Fatal(err)
	    }

	    fmt.Printf("Account: %+v\n", account)
	}

# Network Options

The SDK supports multiple network configurations:

	// Mainnet
	client.NewMainnet()

	// Testnet (Kermit)
	client.NewTestnet()

	// Local development
	client.NewLocal("http://localhost:8080/v3")

	// Custom devnet
	client.NewDevnet("http://my-devnet:8080/v3")

	// Custom configuration
	client.New(&client.Config{
	    Endpoint: "https://custom.accumulate.io/v3",
	    Network:  client.NetworkCustom,
	    Timeout:  30 * time.Second,
	    Debug:    true,
	})

# Available Methods

Query Methods:
  - GetAccount(ctx, url) - Query account information
  - GetTransaction(ctx, txid) - Query transaction by ID
  - GetChainEntry(ctx, account, chain, index) - Query chain entry
  - GetDataEntry(ctx, account, index) - Query data entry
  - GetDirectory(ctx, account, start, count) - List directory entries
  - GetBlock(ctx, partition, number) - Query block information

Network Information:
  - GetNodeInfo(ctx) - Get node information
  - GetNetworkStatus(ctx) - Get network status
  - GetConsensusStatus(ctx) - Get consensus status
  - GetMetrics(ctx, partition, duration) - Get network metrics

Transaction Submission:
  - Submit(ctx, envelope, opts) - Submit transaction
  - Validate(ctx, envelope) - Validate transaction
  - Faucet(ctx, account) - Request testnet tokens

# Transport Protocols

The SDK automatically selects the appropriate transport:
  - JSON-RPC over HTTP (default)
  - WebSocket for event streaming
  - Binary message protocol for P2P
  - RESTful HTTP interface

# Error Handling

All methods return errors that can be inspected for specific error types:

	account, err := client.GetAccount(ctx, "acc://invalid")
	if err != nil {
	    // Handle error
	    log.Printf("Failed to get account: %v", err)
	}

# Curl Examples

Most SDK methods have equivalent curl commands for direct API access.
See individual method documentation for curl examples.
*/
package client
