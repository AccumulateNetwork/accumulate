// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"fmt"
	"time"

	"github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

// DiscoverNonValidatorPeers discovers non-validator peers through Tendermint's P2P network
// This is a critical function for finding non-validator nodes in the network
func (nd *NetworkDiscoveryImpl) DiscoverNonValidatorPeers(ctx context.Context) error {
	// Skip if no validators have been discovered yet
	if len(nd.AddressDir.DNValidators) == 0 && len(nd.AddressDir.BVNValidators) == 0 {
		nd.Logger.Printf("No validators found, skipping non-validator peer discovery")
		return nil
	}

	nd.Logger.Printf("Starting network discovery to find non-validator nodes")
	
	// Track validator names for comparison
	validatorNames := make(map[string]bool)
	
	// Track seen peers to avoid duplicates
	seenPeers := make(map[string]bool)
	
	// Collect all validator names for comparison
	for _, validator := range nd.AddressDir.DNValidators {
		validatorNames[validator.Name] = true
	}
	
	// Add BVN validator names
	for _, validators := range nd.AddressDir.BVNValidators {
		for _, validator := range validators {
			validatorNames[validator.Name] = true
		}
	}
	
	nd.Logger.Printf("Starting validator connection phase to discover peers")
	nd.Logger.Printf("Using longer timeouts (30 seconds) for network operations")
	
	connectedValidators := 0
	totalPeers := 0
	
	// Connect to DN validators first
	for _, validator := range nd.AddressDir.DNValidators {
		if validator.IPAddress == "" {
			nd.Logger.Printf("Validator %s has no IP address, skipping", validator.Name)
			continue
		}
		
		nd.Logger.Printf("Connecting to validator %s at %s", validator.Name, validator.IPAddress)
		
		// Connect directly to port 16592 like the status_test implementation
		port := 16592
		base := fmt.Sprintf("http://%s:%d", validator.IPAddress, port)
		c, err := http.New(base, base+"/ws")
		if err != nil {
			nd.Logger.Printf("Error creating client for validator %s on port %d: %v", validator.Name, port, err)
			continue
		}
		
		// Create a timeout context for this validator with a longer timeout
		valCtx, valCancel := context.WithTimeout(ctx, 30*time.Second)
		
		// Try to get status to check if connection works
		nd.Logger.Printf("Attempting to get status from validator %s (timeout: 30s)", validator.Name)
		status, statusErr := c.Status(valCtx)
		
		if statusErr != nil {
			nd.Logger.Printf("Failed to connect to validator %s on port %d: %v", validator.Name, port, statusErr)
			valCancel()
			continue
		}
		
		nd.Logger.Printf("Successfully connected to validator %s on port %d", validator.Name, port)
		nd.Logger.Printf("Validator %s info: %s, %s", validator.Name, status.NodeInfo.Moniker, status.NodeInfo.Version)
		
		// Get network info (peers) with detailed logging
		nd.Logger.Printf("Getting network info from validator %s (timeout: 30s)", validator.Name)
		
		// Create a new context with a longer timeout specifically for NetInfo
		netInfoCtx, netInfoCancel := context.WithTimeout(ctx, 30*time.Second)
		netInfo, err := c.NetInfo(netInfoCtx)
		
		if err != nil {
			nd.Logger.Printf("Error getting network info from validator %s: %v", validator.Name, err)
			netInfoCancel()
			valCancel()
			continue
		}
		
		// Log the successful connection
		nd.Logger.Printf("Successfully connected to validator %s and retrieved network info with %d peers", validator.Name, len(netInfo.Peers))
		
		// Wait a bit longer to ensure we get all peers
		time.Sleep(3 * time.Second)
		
		// Try again with the same connection to get more peers
		nd.Logger.Printf("Trying again to get more peers from validator %s", validator.Name)
		netInfo2, err := c.NetInfo(netInfoCtx)
		netInfoCancel()
		
		if err == nil && len(netInfo2.Peers) > len(netInfo.Peers) {
			nd.Logger.Printf("Got more peers on second attempt: %d vs %d", len(netInfo2.Peers), len(netInfo.Peers))
			netInfo = netInfo2
		}
		
		valCancel()
		connectedValidators++
		nd.Logger.Printf("Validator %s has %d peers", validator.Name, len(netInfo.Peers))
		
		// Process peers to identify non-validators
		for _, peer := range netInfo.Peers {
			totalPeers++
			
			// Check if this peer is a validator
			peerID := peer.NodeInfo.ID()
			peerAddr := peer.RemoteIP
			peerName := peer.NodeInfo.Moniker
			
			// Skip if we've already seen this peer
			if seenPeers[peerID.String()] {
				continue
			}
			seenPeers[peerID.String()] = true
			
			nd.Logger.Printf("Found peer %s (%s) at %s", peerName, peerID, peerAddr)
			
			// Check if this peer is already a known validator
			isValidator := validatorNames[peerName]
			
			// Add to the appropriate list
			networkPeer := &NetworkPeer{
				Name:      peerName,
				ID:        peerID.String(),
				IPAddress: peerAddr,
				LastSeen:  time.Now(),
			}
			
			if !isValidator {
				// This is a non-validator peer
				nd.Logger.Printf("Found non-validator peer: %s at %s", peerName, peerAddr)
			} else {
				nd.Logger.Printf("Peer %s is a validator, updating last seen time", peerName)
			}
			
			// Add the peer to our list
			nd.AddressDir.AddPeer(networkPeer)
		}
	}
	
	// Process BVN validators too using the same approach
	for bvn, validators := range nd.AddressDir.BVNValidators {
		for _, validator := range validators {
			if validator.IPAddress == "" {
				nd.Logger.Printf("BVN validator %s in %v has no IP address, skipping", validator.Name, bvn)
				continue
			}
			
			nd.Logger.Printf("Connecting to BVN validator %s at %s", validator.Name, validator.IPAddress)
			
			// Connect directly to port 16692 for BVN validators
			port := 16692
			base := fmt.Sprintf("http://%s:%d", validator.IPAddress, port)
			c, err := http.New(base, base+"/ws")
			if err != nil {
				nd.Logger.Printf("Error creating client for BVN validator %s on port %d: %v", validator.Name, port, err)
				continue
			}
			
			// Create a timeout context for this validator with a longer timeout
			valCtx, valCancel := context.WithTimeout(ctx, 30*time.Second)
			
			// Try to get status to check if connection works
			nd.Logger.Printf("Attempting to get status from BVN validator %s (timeout: 30s)", validator.Name)
			status, statusErr := c.Status(valCtx)
			
			if statusErr != nil {
				nd.Logger.Printf("Failed to connect to BVN validator %s on port %d: %v", validator.Name, port, statusErr)
				valCancel()
				continue
			}
			
			nd.Logger.Printf("Successfully connected to BVN validator %s on port %d", validator.Name, port)
			nd.Logger.Printf("BVN validator %s info: %s, %s", validator.Name, status.NodeInfo.Moniker, status.NodeInfo.Version)
			
			// Get network info (peers) with detailed logging
			nd.Logger.Printf("Getting network info from BVN validator %s (timeout: 30s)", validator.Name)
			
			// Create a new context with a longer timeout specifically for NetInfo
			netInfoCtx, netInfoCancel := context.WithTimeout(ctx, 30*time.Second)
			netInfo, err := c.NetInfo(netInfoCtx)
			
			if err != nil {
				nd.Logger.Printf("Error getting network info from BVN validator %s: %v", validator.Name, err)
				netInfoCancel()
				valCancel()
				continue
			}
			
			// Log the successful connection
			nd.Logger.Printf("Successfully connected to BVN validator %s and retrieved network info with %d peers", validator.Name, len(netInfo.Peers))
			
			// Wait a bit longer to ensure we get all peers
			time.Sleep(3 * time.Second)
			
			// Try again with the same connection to get more peers
			nd.Logger.Printf("Trying again to get more peers from BVN validator %s", validator.Name)
			netInfo2, err := c.NetInfo(netInfoCtx)
			netInfoCancel()
			
			if err == nil && len(netInfo2.Peers) > len(netInfo.Peers) {
				nd.Logger.Printf("Got more peers on second attempt: %d vs %d", len(netInfo2.Peers), len(netInfo.Peers))
				netInfo = netInfo2
			}
			
			valCancel()
			connectedValidators++
			nd.Logger.Printf("BVN validator %s has %d peers", validator.Name, len(netInfo.Peers))
			
			// Process peers to identify non-validators
			for _, peer := range netInfo.Peers {
				totalPeers++
				
				// Check if this peer is a validator
				peerID := peer.NodeInfo.ID()
				peerAddr := peer.RemoteIP
				peerName := peer.NodeInfo.Moniker
				
				// Skip if we've already seen this peer
				if seenPeers[peerID.String()] {
					continue
				}
				seenPeers[peerID.String()] = true
				
				nd.Logger.Printf("Found peer %s (%s) at %s", peerName, peerID, peerAddr)
				
				// Check if this peer is already a known validator
				isValidator := validatorNames[peerName]
				
				// Add to the appropriate list
				networkPeer := &NetworkPeer{
					Name:      peerName,
					ID:        peerID.String(),
					IPAddress: peerAddr,
					LastSeen:  time.Now(),
				}
				
				if !isValidator {
					// This is a non-validator peer
					nd.Logger.Printf("Found non-validator peer: %s at %s", peerName, peerAddr)
				} else {
					nd.Logger.Printf("Peer %s is a validator, updating last seen time", peerName)
				}
				
				// Add the peer to our list
				nd.AddressDir.AddPeer(networkPeer)
			}
		}
	}
	
	// Count non-validator nodes
	nonValidatorCount := 0
	for _, peer := range nd.AddressDir.NetworkPeers {
		if !validatorNames[peer.Name] {
			nonValidatorCount++
		}
	}
	
	// Summarize the results
	nd.Logger.Printf("Network discovery completed. Connected to %d validators and found %d total peers", 
		connectedValidators, totalPeers)
	
	// Print statistics
	nd.Logger.Printf("Discovery Statistics:")
	nd.Logger.Printf("  Total Validators: %d", len(validatorNames))
	nd.Logger.Printf("  DN Validators: %d", len(nd.AddressDir.DNValidators))
	nd.Logger.Printf("  BVN Validators: map[%d]", len(nd.AddressDir.BVNValidators))
	nd.Logger.Printf("  Total Peers: %d", len(nd.AddressDir.NetworkPeers))
	nd.Logger.Printf("  Non-Validator Peers: %d", nonValidatorCount)
	
	// Print all network peers
	nd.Logger.Printf("\nAll Network Peers:")
	peerIndex := 1
	for _, peer := range nd.AddressDir.NetworkPeers {
		nd.Logger.Printf("  Validator Peer %d: %s", peerIndex, peer.Name)
		nd.Logger.Printf("    Last Seen: %v", peer.LastSeen)
		peerIndex++
	}
	
	// Analyze validators and non-validators
	nd.Logger.Printf("\nValidator Analysis:")
	nd.Logger.Printf("  Total Peers: %d", len(nd.AddressDir.NetworkPeers))
	
	// Count validator and non-validator peers
	validatorPeers := 0
	nonValidatorPeers := 0
	for _, peer := range nd.AddressDir.NetworkPeers {
		if validatorNames[peer.Name] {
			validatorPeers++
		} else {
			nonValidatorPeers++
		}
	}
	
	nd.Logger.Printf("  Validator Peers: %d", validatorPeers)
	nd.Logger.Printf("  Non-Validator Peers: %d", nonValidatorPeers)
	
	// Print validators not found in peer list
	nd.Logger.Printf("\nValidators not found in peer list:")
	for validatorName := range validatorNames {
		found := false
		for _, peer := range nd.AddressDir.NetworkPeers {
			if peer.Name == validatorName {
				found = true
				break
			}
		}
		if !found {
			nd.Logger.Printf("  %s", validatorName)
		}
	}
	
	// Print non-validator nodes
	nd.Logger.Printf("\nNon-Validator Nodes:")
	for _, peer := range nd.AddressDir.NetworkPeers {
		if !validatorNames[peer.Name] {
			nd.Logger.Printf("  %s at %s (Last seen: %v)", peer.Name, peer.IPAddress, peer.LastSeen)
		}
	}
	
	nd.Logger.Printf("\nFound %d non-validator nodes out of %d total peers", nonValidatorCount, len(nd.AddressDir.NetworkPeers))
	return nil
}
