package client

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// NodeInfo gets information about the node
func (c *Client) NodeInfo(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	opts := api.NodeInfoOptions{}

	// Note: PeerID would need to be parsed from string to p2p.PeerID type
	// For now, we omit it and let the API use defaults

	info, err := c.client.NodeInfo(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info: %w", err)
	}

	return info, nil
}

// NetworkStatus gets the network status including globals and routing
func (c *Client) NetworkStatus(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	opts := api.NetworkStatusOptions{}

	// Set partition if provided
	if partition, ok := params["partition"].(string); ok {
		opts.Partition = partition
	}

	status, err := c.client.NetworkStatus(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status: %w", err)
	}

	return status, nil
}

// ConsensusStatus gets the consensus status of the node
func (c *Client) ConsensusStatus(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	opts := api.ConsensusStatusOptions{}

	// Set partition if provided
	if partition, ok := params["partition"].(string); ok {
		opts.Partition = partition
	}

	// Set node ID if provided
	if nodeID, ok := params["nodeID"].(string); ok {
		opts.NodeID = nodeID
	}

	status, err := c.client.ConsensusStatus(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get consensus status: %w", err)
	}

	return status, nil
}

// Metrics gets network metrics including TPS
func (c *Client) Metrics(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	opts := api.MetricsOptions{}

	// Set partition if provided
	if partition, ok := params["partition"].(string); ok {
		opts.Partition = partition
	}

	// Set span if provided (window width in blocks)
	if span, ok := params["span"].(uint64); ok {
		opts.Span = span
	} else if spanFloat, ok := params["span"].(float64); ok {
		opts.Span = uint64(spanFloat)
	}

	metrics, err := c.client.Metrics(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	return metrics, nil
}

// Faucet requests testnet tokens from the faucet
func (c *Client) Faucet(ctx context.Context, accountURL string, params map[string]interface{}) (interface{}, error) {
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	opts := api.FaucetOptions{}

	// Set token if provided (defaults to ACME)
	if token, ok := params["token"].(string); ok {
		tokenUrl, err := url.Parse(token)
		if err != nil {
			return nil, fmt.Errorf("invalid token URL: %w", err)
		}
		opts.Token = tokenUrl
	}

	submission, err := c.client.Faucet(ctx, accountUrl, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to request faucet: %w", err)
	}

	return submission, nil
}
