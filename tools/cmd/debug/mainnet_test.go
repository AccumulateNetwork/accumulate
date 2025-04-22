package main

import (
	"context"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// TestMainnetPeerState tests connecting to mainnet and retrieving peer state information
func TestMainnetPeerState(t *testing.T) {
	// Create a client to connect to the mainnet
	endpoint := "https://mainnet.accumulatenetwork.io/v3"
	client := jsonrpc.NewClient(endpoint)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query network status
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		t.Fatalf("Failed to query network status: %v", err)
	}

	// Print network information
	t.Logf("Network: %+v", networkStatus.Network)
	// t.Logf("Version: %s", networkStatus.Version) // commented out: field does not exist

	// Query node info
	nodeInfo, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		t.Fatalf("Failed to query node info: %v", err)
	}

	// Print node information
	t.Logf("\nNode Info:")
	t.Logf("  Network: %s", nodeInfo.Network)
	// t.Logf("  Version: %s", nodeInfo.Version) // commented out: field does not exist
	// t.Logf("  P2P Address: %s", nodeInfo.P2PAddress) // commented out: field does not exist
}
