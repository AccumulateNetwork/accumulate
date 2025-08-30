// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package client provides a high-level, unified Go SDK for interacting with
// Accumulate networks. It wraps the underlying API implementations and provides
// a simple, idiomatic interface for all Accumulate operations.
package client

import (
	"context"
	"fmt"
	"time"

	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Client is the main Accumulate client that provides access to all API operations.
type Client struct {
	// config holds the client configuration
	config *Config

	// v3Client is the underlying V3 API client
	v3Client v3.Querier

	// Additional service interfaces
	nodeService     v3.NodeService
	networkService  v3.NetworkService
	submitter       v3.Submitter
	validator       v3.Validator
	faucet          v3.Faucet
	eventService    v3.EventService
}

// Config holds the configuration for the client.
type Config struct {
	// Endpoint is the network endpoint to connect to
	Endpoint string

	// Network identifies the network type
	Network NetworkType

	// Timeout for requests (default: 15s)
	Timeout time.Duration

	// Debug enables debug logging
	Debug bool
}

// NetworkType identifies the type of network.
type NetworkType string

const (
	// NetworkMainnet is the Accumulate mainnet
	NetworkMainnet NetworkType = "mainnet"

	// NetworkTestnet is the Accumulate testnet (Kermit)
	NetworkTestnet NetworkType = "testnet"

	// NetworkLocal is a local development network
	NetworkLocal NetworkType = "local"

	// NetworkDevnet is a development network
	NetworkDevnet NetworkType = "devnet"

	// NetworkCustom is a custom network
	NetworkCustom NetworkType = "custom"
)

// New creates a new Accumulate client with the given configuration.
func New(config *Config) (*Client, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	if config.Timeout == 0 {
		config.Timeout = 15 * time.Second
	}

	// Create the underlying JSON-RPC client
	jrpcClient := jsonrpc.NewClient(config.Endpoint)
	jrpcClient.Client.Timeout = config.Timeout
	jrpcClient.Debug = config.Debug

	client := &Client{
		config:         config,
		v3Client:       jrpcClient,
		nodeService:    jrpcClient,
		networkService: jrpcClient,
		submitter:      jrpcClient,
		validator:      jrpcClient,
		faucet:         jrpcClient,
		// Note: eventService would require WebSocket client
	}

	return client, nil
}

// NewMainnet creates a client connected to the Accumulate mainnet.
func NewMainnet() (*Client, error) {
	return New(&Config{
		Endpoint: "https://mainnet.accumulatenetwork.io/v3",
		Network:  NetworkMainnet,
	})
}

// NewTestnet creates a client connected to the Accumulate testnet (Kermit).
func NewTestnet() (*Client, error) {
	return New(&Config{
		Endpoint: "https://kermit.accumulatenetwork.io/v3",
		Network:  NetworkTestnet,
	})
}

// NewLocal creates a client connected to a local network.
func NewLocal(endpoint string) (*Client, error) {
	if endpoint == "" {
		endpoint = "http://localhost:8080/v3"
	}
	return New(&Config{
		Endpoint: endpoint,
		Network:  NetworkLocal,
	})
}

// NewDevnet creates a client connected to a devnet.
func NewDevnet(endpoint string) (*Client, error) {
	return New(&Config{
		Endpoint: endpoint,
		Network:  NetworkDevnet,
	})
}

// GetAccount queries an account by its URL.
//
// Example:
//
//	account, err := client.GetAccount(ctx, "acc://mytoken.acme")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Account: %+v\n", account)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "query",
//	    "params": {
//	      "scope": "acc://mytoken.acme",
//	      "query": {}
//	    },
//	    "id": 1
//	  }'
func (c *Client) GetAccount(ctx context.Context, accountURL string) (*v3.AccountRecord, error) {
	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use DefaultQuery to get the account
	query := &v3.DefaultQuery{}
	
	record, err := c.v3Client.Query(ctx, u, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	// Type assert to AccountRecord
	accountRecord, ok := record.(*v3.AccountRecord)
	if !ok {
		return nil, fmt.Errorf("unexpected record type: %T", record)
	}

	return accountRecord, nil
}