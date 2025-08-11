// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client

import (
	"context"
	"fmt"

	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// GetNodeInfo returns information about the network node.
//
// Example:
//
//	info, err := client.GetNodeInfo(ctx)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Node: %s, Network: %s\n", info.PeerID, info.Network)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "node-info",
//	    "params": {},
//	    "id": 1
//	  }'
func (c *Client) GetNodeInfo(ctx context.Context) (*v3.NodeInfo, error) {
	info, err := c.nodeService.NodeInfo(ctx, v3.NodeInfoOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node info: %w", err)
	}
	return info, nil
}

// GetNetworkStatus returns the status of the network.
//
// Example:
//
//	status, err := client.GetNetworkStatus(ctx)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Network: %s, Oracle: %v\n", status.Network, status.Oracle)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "network-status",
//	    "params": {},
//	    "id": 1
//	  }'
func (c *Client) GetNetworkStatus(ctx context.Context) (*v3.NetworkStatus, error) {
	status, err := c.networkService.NetworkStatus(ctx, v3.NetworkStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get network status: %w", err)
	}
	return status, nil
}

// GetConsensusStatus returns the status of the consensus node.
// This method is only available on validator nodes.
//
// Example:
//
//	status, err := client.GetConsensusStatus(ctx)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Consensus OK: %v, Validator: %v\n", status.Ok, status.ValidatorKeyHash)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "consensus-status",
//	    "params": {},
//	    "id": 1
//	  }'
func (c *Client) GetConsensusStatus(ctx context.Context) (*v3.ConsensusStatus, error) {
	// Check if we have a consensus service
	consensusService, ok := c.v3Client.(v3.ConsensusService)
	if !ok {
		return nil, fmt.Errorf("consensus service not available")
	}

	status, err := consensusService.ConsensusStatus(ctx, v3.ConsensusStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get consensus status: %w", err)
	}
	return status, nil
}

// GetMetrics returns network metrics such as transactions per second.
//
// Example:
//
//	metrics, err := client.GetMetrics(ctx, "Directory")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("TPS: %v\n", metrics.TPS)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "metrics",
//	    "params": {
//	      "partition": "Directory",
//	      "duration": "1h"
//	    },
//	    "id": 1
//	  }'
func (c *Client) GetMetrics(ctx context.Context, partition string) (*v3.Metrics, error) {
	// Check if we have a metrics service
	metricsService, ok := c.v3Client.(v3.MetricsService)
	if !ok {
		return nil, fmt.Errorf("metrics service not available")
	}

	opts := v3.MetricsOptions{
		Partition: partition,
	}

	metrics, err := metricsService.Metrics(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}
	return metrics, nil
}

// FindService searches for nodes that provide the given service.
//
// Example:
//
//	nodes, err := client.FindService(ctx, &v3.ServiceAddress{Type: v3.ServiceTypeQuery})
//	if err != nil {
//	    return err
//	}
//	for _, node := range nodes {
//	    fmt.Printf("Node: %s at %s\n", node.PeerID, node.Address)
//	}
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "find-service",
//	    "params": {
//	      "service": {"type": 1}
//	    },
//	    "id": 1
//	  }'
func (c *Client) FindService(ctx context.Context, service *v3.ServiceAddress) ([]*v3.FindServiceResult, error) {
	opts := v3.FindServiceOptions{
		Service: service,
	}

	results, err := c.nodeService.FindService(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find service: %w", err)
	}
	return results, nil
}

// ListSnapshots returns the list of available snapshots.
//
// Example:
//
//	snapshots, err := client.ListSnapshots(ctx)
//	if err != nil {
//	    return err
//	}
//	for _, snap := range snapshots {
//	    fmt.Printf("Snapshot: %s at height %d\n", snap.Name, snap.Height)
//	}
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "list-snapshots",
//	    "params": {},
//	    "id": 1
//	  }'
func (c *Client) ListSnapshots(ctx context.Context) ([]*v3.SnapshotInfo, error) {
	// Check if we have a snapshot service
	snapshotService, ok := c.v3Client.(v3.SnapshotService)
	if !ok {
		return nil, fmt.Errorf("snapshot service not available")
	}

	snapshots, err := snapshotService.ListSnapshots(ctx, v3.ListSnapshotsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}
	return snapshots, nil
}