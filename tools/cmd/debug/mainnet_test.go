package debug

import (
	"context"
	"fmt"
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
	t.Logf("Network: %s", networkStatus.Network)
	t.Logf("Version: %s", networkStatus.Version)
	
	// Print partition information
	t.Logf("\nPartitions:")
	for _, partition := range networkStatus.Partitions {
		t.Logf("  %s (Type: %s)", partition.ID, partition.Type)
		t.Logf("    Validators: %d", len(partition.Validators))
		
		// Print validator information
		for _, validator := range partition.Validators {
			t.Logf("    - %s (Address: %s)", validator.ID, validator.Address)
		}
	}

	// Query node info
	nodeInfo, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		t.Fatalf("Failed to query node info: %v", err)
	}

	// Print node information
	t.Logf("\nNode Info:")
	t.Logf("  Network: %s", nodeInfo.Network)
	t.Logf("  Version: %s", nodeInfo.Version)
	t.Logf("  P2P Address: %s", nodeInfo.P2PAddress)
}
