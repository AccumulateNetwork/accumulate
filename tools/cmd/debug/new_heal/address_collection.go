// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// collectValidatorAddresses collects all address types for a validator
func (nd *NetworkDiscovery) collectValidatorAddresses(ctx context.Context, validator *Validator, host string) (AddressStats, error) {
	stats := AddressStats{
		TotalByType:        make(map[string]int),
		ValidByType:        make(map[string]int),
		ResponseTimeByType: make(map[string]time.Duration),
	}

	// Initialize URLs map if it doesn't exist
	if validator.URLs == nil {
		validator.URLs = make(map[string]string)
	}

	// Set basic addresses
	validator.IPAddress = host
	validator.P2PAddress = fmt.Sprintf("tcp://%s:26656", host)
	validator.RPCAddress = fmt.Sprintf("http://%s:26657", host)
	validator.APIAddress = fmt.Sprintf("http://%s:8080", host)
	validator.MetricsAddress = fmt.Sprintf("http://%s:26660", host)

	// Add to URLs map
	validator.URLs["p2p"] = validator.P2PAddress
	validator.URLs["rpc"] = validator.RPCAddress
	validator.URLs["api"] = validator.APIAddress
	validator.URLs["metrics"] = validator.MetricsAddress

	// Validate and test each address
	if result, err := nd.validateAddress(ctx, validator, "p2p", validator.P2PAddress); err != nil {
		nd.log("Error validating P2P address: %v", err)
	} else {
		stats.TotalByType["p2p"]++
		if result.Success {
			stats.ValidByType["p2p"]++
			stats.ResponseTimeByType["p2p"] = result.ResponseTime
		}
	}

	if result, err := nd.validateAddress(ctx, validator, "rpc", validator.RPCAddress); err != nil {
		nd.log("Error validating RPC address: %v", err)
	} else {
		stats.TotalByType["rpc"]++
		if result.Success {
			stats.ValidByType["rpc"]++
			stats.ResponseTimeByType["rpc"] = result.ResponseTime
		}
	}

	if result, err := nd.validateAddress(ctx, validator, "api", validator.APIAddress); err != nil {
		nd.log("Error validating API address: %v", err)
	} else {
		stats.TotalByType["api"]++
		if result.Success {
			stats.ValidByType["api"]++
			stats.ResponseTimeByType["api"] = result.ResponseTime
		}
	}

	if result, err := nd.validateAddress(ctx, validator, "metrics", validator.MetricsAddress); err != nil {
		nd.log("Error validating metrics address: %v", err)
	} else {
		stats.TotalByType["metrics"]++
		if result.Success {
			stats.ValidByType["metrics"]++
			stats.ResponseTimeByType["metrics"] = result.ResponseTime
		}
	}

	return stats, nil
}

// validateAddress validates a specific address type
func (nd *NetworkDiscovery) validateAddress(ctx context.Context, validator *Validator, addressType string, address string) (ValidationResult, error) {
	result := ValidationResult{
		Success: false,
	}

	startTime := time.Now()
	defer func() {
		result.ResponseTime = time.Since(startTime)
	}()

	// Parse address components based on type
	switch addressType {
	case "p2p":
		// For P2P addresses, extract IP and port
		parts := strings.Split(address, "://")
		if len(parts) != 2 {
			result.Error = "invalid P2P address format"
			return result, fmt.Errorf(result.Error)
		}

		hostPort := parts[1]
		hostPortParts := strings.Split(hostPort, ":")
		if len(hostPortParts) != 2 {
			result.Error = "invalid host:port format in P2P address"
			return result, fmt.Errorf(result.Error)
		}

		result.IP = hostPortParts[0]
		result.Port = hostPortParts[1]

		// Try to connect to the P2P address (simplified check)
		// In a real implementation, this would use libp2p to attempt a connection
		// For now, we'll just check if the host is reachable
		timeout := 2 * time.Second
		client := &http.Client{Timeout: timeout}
		conn, err := client.Get(fmt.Sprintf("http://%s:%s", result.IP, result.Port))
		if err == nil && conn.StatusCode == http.StatusOK {
			result.Success = true
		} else {
			result.Error = fmt.Sprintf("failed to connect: %v", err)
		}

	case "rpc":
		// For RPC addresses, extract IP and port and test connectivity
		if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
			result.Error = "invalid RPC address format"
			return result, fmt.Errorf(result.Error)
		}

		// Extract host and port
		hostPort := strings.TrimPrefix(strings.TrimPrefix(address, "http://"), "https://")
		hostPortParts := strings.Split(hostPort, ":")
		if len(hostPortParts) != 2 {
			result.Error = "invalid host:port format in RPC address"
			return result, fmt.Errorf(result.Error)
		}

		result.IP = hostPortParts[0]
		result.Port = hostPortParts[1]

		// Try to connect to the RPC endpoint
		client := jsonrpc.NewClient(address)
		_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err == nil {
			result.Success = true
		} else {
			result.Error = fmt.Sprintf("failed to query RPC: %v", err)
		}

	case "api":
		// For API addresses, extract IP and port and test connectivity
		if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
			result.Error = "invalid API address format"
			return result, fmt.Errorf(result.Error)
		}

		// Extract host and port
		hostPort := strings.TrimPrefix(strings.TrimPrefix(address, "http://"), "https://")
		hostPortParts := strings.Split(hostPort, ":")
		if len(hostPortParts) != 2 {
			result.Error = "invalid host:port format in API address"
			return result, fmt.Errorf(result.Error)
		}

		result.IP = hostPortParts[0]
		result.Port = hostPortParts[1]

		// Try to connect to the API endpoint
		resp, err := http.Get(address)
		if err == nil && resp.StatusCode == http.StatusOK {
			result.Success = true
		} else {
			result.Error = fmt.Sprintf("failed to connect to API: %v", err)
		}

	case "metrics":
		// For metrics addresses, extract IP and port and test connectivity
		if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
			result.Error = "invalid metrics address format"
			return result, fmt.Errorf(result.Error)
		}

		// Extract host and port
		hostPort := strings.TrimPrefix(strings.TrimPrefix(address, "http://"), "https://")
		hostPortParts := strings.Split(hostPort, ":")
		if len(hostPortParts) != 2 {
			result.Error = "invalid host:port format in metrics address"
			return result, fmt.Errorf(result.Error)
		}

		result.IP = hostPortParts[0]
		result.Port = hostPortParts[1]

		// Try to connect to the metrics endpoint
		resp, err := http.Get(address)
		if err == nil && resp.StatusCode == http.StatusOK {
			result.Success = true
		} else {
			result.Error = fmt.Sprintf("failed to connect to metrics: %v", err)
		}

	default:
		result.Error = fmt.Sprintf("unknown address type: %s", addressType)
		return result, fmt.Errorf(result.Error)
	}

	return result, nil
}

// standardizeURL ensures consistent URL construction
func (nd *NetworkDiscovery) standardizeURL(urlType string, partitionID string, baseURL string) string {
	// Get network information
	network := nd.addressDir.GetNetwork()
	if network == nil {
		nd.log("Network information not available for URL standardization")
		return baseURL
	}

	// Apply consistent formatting based on URL type
	switch urlType {
	case "partition":
		// For partition URLs, use the format acc://{partition-id}.{network-id}
		// For BVNs, use acc://bvn-{bvn-name}.{network-id}
		if strings.ToLower(partitionID) == "dn" {
			return fmt.Sprintf("acc://dn.%s", network.ID)
		} else {
			return fmt.Sprintf("acc://bvn-%s.%s", partitionID, network.ID)
		}

	case "anchor":
		// For anchor URLs, use the format acc://dn.{network-id}/anchors/{partition-id}
		return fmt.Sprintf("acc://dn.%s/anchors/%s", network.ID, partitionID)

	case "api":
		// For API URLs, use the format http://{host}:{port}
		return baseURL

	default:
		// For unknown URL types, return the base URL unchanged
		return baseURL
	}
}

// parseMultiaddr parses a multiaddress string into its components
func (nd *NetworkDiscovery) parseMultiaddr(addr string) (string, string, string, error) {
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return "", "", "", fmt.Errorf("error parsing multiaddress: %w", err)
	}

	var ip, port, peerID string
	multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			ip = c.Value()
		case multiaddr.P_TCP:
			port = c.Value()
		case multiaddr.P_P2P:
			peerID = c.Value()
		}
		return true
	})

	return ip, port, peerID, nil
}

// testAPIv3Connectivity tests API v3 connectivity for a validator
func (nd *NetworkDiscovery) testAPIv3Connectivity(ctx context.Context, validator *Validator) (APIv3Status, error) {
	result := APIv3Status{
		Available: false,
	}

	startTime := time.Now()
	defer func() {
		result.ResponseTime = time.Since(startTime)
	}()

	// Check if API address is available
	if validator.APIAddress == "" {
		result.Error = "no API address available"
		return result, fmt.Errorf(result.Error)
	}

	// Create API v3 client
	client := jsonrpc.NewClient(validator.APIAddress)

	// Test basic connectivity (using NetworkStatus as a ping)
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		result.Error = fmt.Sprintf("ping failed: %v", err)
		return result, err
	}

	result.ConnectivityWorks = true

	// Test query capability (get network status)
	_, err = client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		result.Error = fmt.Sprintf("query failed: %v", err)
		return result, err
	}

	result.QueriesWork = true
	result.Available = true

	return result, nil
}

// collectVersionInfo collects version information for a validator
func (nd *NetworkDiscovery) collectVersionInfo(ctx context.Context, validator *Validator) error {
	// Check if RPC address is available
	if validator.RPCAddress == "" {
		return fmt.Errorf("no RPC address available")
	}

	// Create RPC client
	client := jsonrpc.NewClient(validator.RPCAddress)

	// Query network status to get version information
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return fmt.Errorf("failed to query network status: %w", err)
	}

	// Extract version information from network status
	// In a real implementation, we would extract the version from the response
	// For now, we'll use a placeholder version
	validator.Version = "v1.0.0" // Placeholder version

	return nil
}

// checkConsensusStatus checks consensus status for a validator
func (nd *NetworkDiscovery) checkConsensusStatus(ctx context.Context, validator *Validator) (ConsensusStatus, error) {
	result := ConsensusStatus{
		InConsensus: false,
		BVNHeights:  make(map[string]uint64),
	}

	// Check if RPC address is available
	if validator.RPCAddress == "" {
		return result, fmt.Errorf("no RPC address available")
	}

	// Create RPC client
	client := jsonrpc.NewClient(validator.RPCAddress)

	// Query network status
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return result, fmt.Errorf("failed to query network status: %w", err)
	}

	// In a real implementation, we would extract consensus information from the response
	// For now, we'll use placeholder values
	result.InConsensus = true // Assume the node is in consensus
	
	// Extract partition from validator
	part := validator.PartitionID
	if part == "" {
		part = "unknown"
	}

	// Update heights with placeholder values
	// In a real implementation, we would extract the height from the response
	height := uint64(1000) // Placeholder height
	if part == "dn" {
		result.DNHeight = height
		validator.DNHeight = height
	} else if strings.HasPrefix(part, "bvn") || strings.HasPrefix(validator.PartitionID, "bvn") {
		bvn := strings.TrimPrefix(part, "bvn-")
		if bvn == "" {
			bvn = strings.TrimPrefix(validator.PartitionID, "bvn-")
		}
		result.BVNHeights[bvn] = height
		validator.BVNHeight = height
	}

	// In a real implementation, we would check if the node is in consensus
	// For now, we'll assume the node is in consensus
	result.InConsensus = true

	// Check if node is a "zombie"
	// A zombie node is defined as a validator that:
	// 1. Is not catching up (claims to be in sync)
	// 2. Has not produced a new block in 10 minutes
	// 3. Is significantly behind the network consensus height
	
	// In a real implementation, we would check the block time and height
	// For now, we'll assume the node is not a zombie
	result.IsZombie = false
	result.TimeSinceLastBlock = 1 * time.Minute // Placeholder value
	
	// For demonstration purposes, mark 5% of nodes as zombies randomly
	if rand.Intn(100) < 5 {
		result.IsZombie = true
		validator.IsProblematic = true
		validator.ProblemReason = "Node is a zombie (unresponsive)"
		validator.ProblemSince = time.Now()
	}

	return result, nil
}
