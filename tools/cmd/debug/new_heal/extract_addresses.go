// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// extractAddressesFromValidatorPartition extracts addresses from protocol.ValidatorPartitionInfo
func (nd *NetworkDiscovery) extractAddressesFromValidatorPartition(partInfo *protocol.ValidatorPartitionInfo) []string {
	if partInfo == nil {
		return nil
	}
	
	// Check if the partition is active
	if !partInfo.Active {
		nd.log("Partition %s is not active, skipping address extraction", partInfo.ID)
		return nil
	}
	
	// Get addresses from validator repository based on partition ID
	addresses := make([]string, 0)
	partitionID := partInfo.ID
	nd.log("Extracting addresses for validator partition %s", partitionID)
	
	// Use the validator repository to get known addresses
	if nd.validatorRepo != nil {
		// Get validators for this partition
		validators := nd.validatorRepo.GetValidatorsForPartition(partitionID)
		for _, validatorID := range validators {
			// Get known address for this validator
			knownAddr := nd.validatorRepo.GetKnownAddress(validatorID)
			if knownAddr != "" {
				// Add a properly formatted multiaddress
				multiAddr := fmt.Sprintf("/ip4/%s/tcp/26656", knownAddr)
				addresses = append(addresses, multiAddr)
				nd.log("Added address %s for validator %s in partition %s", multiAddr, validatorID, partitionID)
			}
		}
	}
	
	// For mainnet, use well-known validator addresses
	if nd.networkType == "mainnet" {
		// Add some well-known mainnet validator addresses for testing
		knownAddresses := map[string][]string{
			"dn": {
				"/ip4/65.108.73.121/tcp/16593/p2p/QmPublicAPI1",
				"/ip4/144.76.105.23/tcp/16593/p2p/QmPublicAPI2",
			},
			"Apollo": {
				"/ip4/65.21.231.58/tcp/26656/p2p/12D3KooWL1NF6fdTJ7N6SnEyqTrCxqCmYKxCaKJjMVc3qRNgfhQu", // Kompendium
				"/ip4/65.109.104.118/tcp/26656/p2p/12D3KooWHFrjfPdpGC5xfBLJK9TqJHnxZ1ZExJGXLBxXE7Sbx5YE", // LunaNova
				"/ip4/135.181.114.121/tcp/26656/p2p/12D3KooWBSEYV8Hk3SrGLwRMZ3QsVQmU2eNPMcuJeHUADoiRUGQk", // TurtleBoat
			},
			"Chandrayaan": {
				"/ip4/65.108.238.102/tcp/26656/p2p/12D3KooWLANUByqHFqGwzA9SvNv7NX6JwfMeg9Yx4jxGKCXoQwKm", // PrestigeIT
				"/ip4/65.108.141.109/tcp/26656/p2p/12D3KooWCMr9mU894i8JXJFHWd4nLBJfTXCwuSeCXMV4yvQdYHaJ", // Sphereon
				"/ip4/65.108.4.175/tcp/26656/p2p/12D3KooWJwzMvhDZQ5JJF9Lc3CCKyKN8B1xXWzRuUopNkRPQCRfL", // Sphereon
			},
			"Yutu": {
				"/ip4/65.108.0.22/tcp/26656/p2p/12D3KooWNMEKxFRpAzS8G3XbMGextBrWxqVMWCEXiZMP7rkiTQJC", // MusicCityNode
				"/ip4/65.109.85.226/tcp/26656/p2p/12D3KooWJvyP3VJYymTqG7e6kJ6tvzNg8MKtUuAGXQ3Gz8b3QobU", // HighStakes
				"/ip4/65.108.201.154/tcp/26656/p2p/12D3KooWRBhwfeP2Y9CDkBVUbxdRrMg3WpZx7Dk7jKaRA7SnLrjQ", // tfa
			},
		}
		
		if knownAddrs, ok := knownAddresses[partitionID]; ok {
			nd.log("Found %d known addresses for partition %s from hardcoded list", len(knownAddrs), partitionID)
			addresses = append(addresses, knownAddrs...)
		} else {
			nd.log("No known addresses for partition %s from hardcoded list", partitionID)
		}
	}
	
	nd.log("Extracted %d addresses for partition %s", len(addresses), partitionID)
	return addresses
}

// ExtractHostFromMultiaddr extracts the host from a multiaddr string
func ExtractHostFromMultiaddr(addrStr string) (string, error) {
	// Try to parse as multiaddr
	maddr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return "", fmt.Errorf("error parsing multiaddr: %w", err)
	}

	// Extract host from multiaddr
	var host string
	multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			// IP address
			host = c.Value()
			return false
		case multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6:
			// Domain name
			host = c.Value()
			return false
		}
		return true
	})

	if host == "" {
		return "", fmt.Errorf("no host component found in multiaddr")
	}

	return host, nil
}

// ExtractHostFromURL attempts to extract a host from a URL-like string
func ExtractHostFromURL(urlStr string) (string, error) {
	// Check if it's a URL with scheme
	if strings.Contains(urlStr, "://") {
		parts := strings.Split(urlStr, "://")
		if len(parts) > 1 {
			hostPort := strings.Split(parts[1], ":")
			if len(hostPort) > 0 {
				host := hostPort[0]
				// If there's a path, remove it
				if strings.Contains(host, "/") {
					host = strings.Split(host, "/")[0]
				}
				return host, nil
			}
		}
	}

	return "", fmt.Errorf("could not extract host from URL")
}
