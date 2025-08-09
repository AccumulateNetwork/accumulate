package main

import (
	"net"
	"net/http"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// OptimizeRecoveryClient optimizes a v3 client for use in recovery operations
func OptimizeRecoveryClient(client api.Querier) api.Querier {
	// Check if it's a jsonrpc.Client that we can optimize
	if jrpcClient, ok := client.(*jsonrpc.Client); ok {
		// Configure optimized transport
		transport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableKeepAlives:   false,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		}

		// Apply optimizations
		jrpcClient.Client.Transport = transport
		jrpcClient.Client.Timeout = 30 * time.Second

		return jrpcClient
	}

	// Return unchanged if not a jsonrpc.Client
	return client
}

// NewOptimizedRecoveryManager creates a recovery manager with an optimized client
func NewOptimizedRecoveryManager(conductor *crosschain.CrossChainConductor, db database.Beginner, serverURL string) *crosschain.RecoveryManager {
	// Use the pooled client
	client := GetPooledClient(serverURL)

	// Create recovery manager with optimized client
	return crosschain.NewRecoveryManager(conductor, db, client)
}

// Example usage in test files:
//
// Instead of:
//   client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
//   rm := crosschain.NewRecoveryManager(conductor, db, client)
//
// Use:
//   rm := NewOptimizedRecoveryManager(conductor, db, "http://127.0.0.1:26660/v3")
//
// Or if you already have a client:
//   client = OptimizeRecoveryClient(client)
//   rm := crosschain.NewRecoveryManager(conductor, db, client)
