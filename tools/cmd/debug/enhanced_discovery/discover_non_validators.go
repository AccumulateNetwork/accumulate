// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
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
	
	// Track seen Tendermint node IDs to avoid duplicates
	seenPeers := make(map[string]bool)
	
	// Track validator names for comparison
	validatorNames := make(map[string]bool)
	
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
	
	nd.Logger.Printf("Starting validator connection phase to discover initial peers")
	nd.Logger.Printf("Using longer timeouts (30 seconds) for network operations")
	
	connectedValidators := 0
	totalPeersFound := 0
	nonValidatorCount := 0
	
	// Connect to DN validators first
	for _, validator := range nd.AddressDir.DNValidators {
		if validator.IPAddress == "" {
			nd.Logger.Printf("Validator %s has no IP address, skipping", validator.Name)
			continue
		}
		
		nd.Logger.Printf("Connecting to validator %s at %s", validator.Name, validator.IPAddress)
		
		// Connect directly to port 16592 like the status_test implementation
		var c client.Client
		var err error
		var connected bool
		
		// Use port 16592 which is what status_test uses
		port := 16592
		nd.Logger.Printf("Connecting to validator %s at %s:%d", validator.Name, validator.IPAddress, port)
		base := fmt.Sprintf("http://%s:%d", validator.IPAddress, port)
		c, err = http.New(base, base+"/ws")
		if err != nil {
			nd.Logger.Printf("Error creating client for validator %s on port %d: %v", validator.Name, port, err)
			continue
		}
		
		// Create a timeout context for this validator with a longer timeout
		valCtx, valCancel := context.WithTimeout(ctx, 30*time.Second)
		
		// Try to get status to check if connection works
		nd.Logger.Printf("Attempting to get status from validator %s (timeout: 30s)", validator.Name)
		status, statusErr := c.Status(valCtx)
		
		if statusErr == nil {
			connected = true
			nd.Logger.Printf("Successfully connected to validator %s on port %d", validator.Name, port)
			nd.Logger.Printf("Validator %s info: %s, %s", validator.Name, status.NodeInfo.Moniker, status.NodeInfo.Version)
		} else {
			nd.Logger.Printf("Failed to connect to validator %s on port %d: %v", validator.Name, port, statusErr)
			valCancel()
			continue
		}
		
		if !connected {
			nd.Logger.Printf("Could not connect to validator %s at %s, skipping", validator.Name, validator.IPAddress)
			continue
		}
		
		// Create a timeout context for getting network info
		valCtx, valCancel := context.WithTimeout(ctx, 15*time.Second)
		
		// Get network info (peers)
		netInfo, err := c.NetInfo(valCtx)
		valCancel()
		if err != nil {
			nd.Logger.Printf("Error getting network info from validator %s: %v", validator.Name, err)
			continue
		}
		
		connectedValidators++
		nd.Logger.Printf("Validator %s has %d peers", validator.Name, len(netInfo.Peers))
		
		// Process peers to identify non-validators
		for _, peer := range netInfo.Peers {
			totalPeersFound++
			
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
			isValidator := false
			for _, v := range nd.AddressDir.DNValidators {
				if v.Name == peerName {
					isValidator = true
					break
				}
			}
			
			// Add to the appropriate list
			peer := &NetworkPeer{
				Name:      peerName,
				ID:        peerID.String(),
				IPAddress: peerAddr,
				LastSeen:  time.Now(),
			}
			
			if !isValidator {
				// This is a non-validator peer
				nd.Logger.Printf("Found non-validator peer: %s at %s", peerName, peerAddr)
				nd.AddressDir.AddPeer(peer)
			} else {
				nd.Logger.Printf("Peer %s is a validator, updating last seen time", peerName)
				// Update the validator's last seen time
				nd.AddressDir.AddPeer(peer)
			}
		}
	}
	
	// Process BVN validators too using the same robust approach
	for bvn, validators := range nd.AddressDir.BVNValidators {
		for _, validator := range validators {
			if validator.IPAddress == "" {
				nd.Logger.Printf("BVN validator %s in %v has no IP address, skipping", validator.Name, bvn)
				continue
			}
			
			nd.Logger.Printf("Connecting to BVN validator %s at %s", validator.Name, validator.IPAddress)
			
			// Try multiple port offsets for connecting to the validator's Tendermint RPC
			var c client.Client
			var err error
			var connected bool
			
			// Try with different port offsets and longer timeouts
			for _, port := range []int{16592, 16692} {
				base := fmt.Sprintf("http://%s:%d", validator.IPAddress, port)
				c, err = http.New(base, base+"/ws")
				if err != nil {
					nd.Logger.Printf("Error creating client for BVN validator %s on port %d: %v", validator.Name, port, err)
					continue
				}
				
				// Create a timeout context for this validator
				valCtx, valCancel := context.WithTimeout(ctx, 15*time.Second)
				
				// Try to get status to check if connection works
				_, statusErr := c.Status(valCtx)
				valCancel()
				
				if statusErr == nil {
					connected = true
					nd.Logger.Printf("Successfully connected to BVN validator %s on port %d", validator.Name, port)
					break
				}
				
				nd.Logger.Printf("Failed to connect to BVN validator %s on port %d: %v", validator.Name, port, statusErr)
			}
			
			if !connected {
				nd.Logger.Printf("Could not connect to BVN validator %s at %s, skipping", validator.Name, validator.IPAddress)
				continue
			}
			
			// Create a timeout context for getting network info
			valCtx, valCancel = context.WithTimeout(ctx, 30*time.Second)
			
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
			valCancel()
			
			if err == nil && len(netInfo2.Peers) > len(netInfo.Peers) {
				nd.Logger.Printf("Got more peers on second attempt: %d vs %d", len(netInfo2.Peers), len(netInfo.Peers))
				netInfo = netInfo2
			}
			
			connectedValidators++
			nd.Logger.Printf("BVN validator %s has %d peers", validator.Name, len(netInfo.Peers))
			
			// Process peers to identify non-validators
			for _, peer := range netInfo.Peers {
				totalPeersFound++
				
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
				isValidator := false
				for _, v := range nd.AddressDir.DNValidators {
					if v.Name == peerName {
						isValidator = true
						break
					}
				}
				
				// Add to the appropriate list
				peer := &NetworkPeer{
					Name:      peerName,
					ID:        peerID.String(),
					IPAddress: peerAddr,
					LastSeen:  time.Now(),
				}
				
				if !isValidator {
					// This is a non-validator peer
					nd.Logger.Printf("Found non-validator peer: %s at %s", peerName, peerAddr)
					nd.AddressDir.AddPeer(peer)
				} else {
					nd.Logger.Printf("Peer %s is a validator, updating last seen time", peerName)
					// Update the validator's last seen time
					nd.AddressDir.AddPeer(peer)
				}
			}
		}
	}
	
	nd.Logger.Printf("Completed validator connection phase. Connected to %d validators, found %d potential non-validator peers", 
		connectedValidators, totalPeersFound)
	
	// Process the peer queue recursively with improved handling
	nonValidatorCount := 0
	peerBatches := 0
	maxBatches := 15 // Increased batch limit to discover more peers
	
	// Use a much longer timeout for each peer connection
	peerTimeout := 30 * time.Second
	
	nd.Logger.Printf("Starting peer discovery with %d initial potential non-validator peers", len(cometPeers))
	
	// If we don't have any initial peers, return early
	if len(cometPeers) == 0 {
		nd.Logger.Printf("No potential non-validator peers found from validators, skipping peer discovery")
		return nil
	}
	
	// Process peers in batches
	for len(cometPeers) > 0 && peerBatches < maxBatches {
		peerBatches++
		nd.Logger.Printf("Processing peer batch %d with %d peers", peerBatches, len(cometPeers))
		
		// Get the next batch of peers (up to 10 at a time to avoid overwhelming)
		batchSize := 10
		if len(cometPeers) < batchSize {
			batchSize = len(cometPeers)
		}
		
		batch := cometPeers[:batchSize]
		cometPeers = cometPeers[batchSize:]
		
		// Track successful connections in this batch
		successfulConnections := 0
		nonValidatorsInBatch := 0
		newPeersDiscovered := 0
		
		for _, peer := range batch {
			peerIDStr := string(peer.NodeInfo.ID())
			nd.Logger.Printf("Checking peer: %s at %s", peerIDStr, peer.RemoteIP)
			
			// Double-check if this is a known validator (in case we missed it earlier)
			if validatorPeerIDs[peerIDStr] {
				nd.Logger.Printf("Peer %s is a known validator, skipping", peerIDStr)
				continue
			}
			
			// Double-check if this is a known validator IP
			if validatorIPs[peer.RemoteIP] {
				nd.Logger.Printf("Peer at IP %s is a known validator IP, skipping", peer.RemoteIP)
				continue
			}
			
			// Try different port offsets for connecting to the peer's RPC
			var c *http.HTTP
			var err error
			var connected bool
			var connectedPort int
			
			// Try with different port offsets and port combinations
			for _, portConfig := range []struct {
				offset config.PortOffset
				port   int
			}{
				{0, 16592},    // Default RPC port
				{100, 16692}, // BVN RPC port
				{0, 26657},   // Standard Tendermint RPC port
			} {
				
				// Try direct port connection first
				if portConfig.port > 0 {
					base := fmt.Sprintf("http://%s:%d", peer.RemoteIP, portConfig.port)
					c, err = http.New(base, base+"/ws")
					if err != nil {
						nd.Logger.Printf("Error creating direct client for peer %s on port %d: %v", 
							peerIDStr, portConfig.port, err)
						continue
					}
				} else {
					// Try with port offset calculation
					c, err = httpClientForPeer(peer, portConfig.offset)
					if err != nil {
						nd.Logger.Printf("Error creating client for peer %s with offset %d: %v", 
							peerIDStr, portConfig.offset, err)
						continue
					}
				}
				
				// Create a timeout context for this peer
				peerCtx, peerCancel := context.WithTimeout(ctx, peerTimeout)
				
				// Try to get status to check if connection works
				_, statusErr := c.Status(peerCtx)
				peerCancel()
				
				if statusErr == nil {
					connected = true
					connectedPort = portConfig.port
					nd.Logger.Printf("Successfully connected to peer %s at %s on port %d", 
						peerIDStr, peer.RemoteIP, portConfig.port)
					break
				}
				
				nd.Logger.Printf("Failed to connect to peer %s with port/offset %d: %v", 
					peerIDStr, portConfig.port, statusErr)
			}
			
			if !connected {
				nd.Logger.Printf("Could not connect to peer %s at %s after trying all ports, skipping", 
					peerIDStr, peer.RemoteIP)
				continue
			}
			
			// Create a timeout context for this peer
			peerCtx, peerCancel := context.WithTimeout(ctx, peerTimeout)
			
			// Get status to identify the peer
			status, err := c.Status(peerCtx)
			if err != nil {
				nd.Logger.Printf("Error getting status from peer %s: %v", peerIDStr, err)
				peerCancel()
				continue
			}
			
			// Double-check if this is a validator by examining the status
			// This is a more reliable way to identify validators
			pubKey := status.ValidatorInfo.PubKey.Bytes()
			if len(pubKey) > 0 {
				// Check if this public key matches any known validator
				isValidator := false
				
				// This is a more thorough check but would require additional code
				// For now, we'll rely on our peer ID and IP checks
				
				if isValidator {
					nd.Logger.Printf("Peer %s is a validator based on public key, skipping", peerIDStr)
					peerCancel()
					continue
				}
			}
			
			// Get network info to discover more peers
			netInfo, err := c.NetInfo(peerCtx)
			peerCancel()
			if err != nil {
				nd.Logger.Printf("Error getting network info from peer %s: %v", peerIDStr, err)
				continue
			}
			
			// Connection was successful
			successfulConnections++
			
			// Add new peers to the queue
			newPeersCount := 0
			for _, p := range netInfo.Peers {
				newPeerIDStr := string(p.NodeInfo.ID())
				
				// Skip if we've already seen this peer
				if seenCometIDs[newPeerIDStr] {
					continue
				}
				
				// Skip if this is a known validator
				if validatorPeerIDs[newPeerIDStr] {
					continue
				}
				
				// Skip if this is a known validator IP
				if validatorIPs[p.RemoteIP] {
					continue
				}
				
				seenCometIDs[newPeerIDStr] = true
				cometPeers = append(cometPeers, p)
				newPeersCount++
			}
			
			newPeersDiscovered += newPeersCount
			nd.Logger.Printf("Found %d new potential non-validator peers from %s", newPeersCount, peerIDStr)
			
			// This is definitely a non-validator peer since it passed all our checks
			// Check if we already have this peer in our network peers
			_, exists := nd.AddressDir.NetworkPeers[peerIDStr]
			if !exists {
				nonValidatorCount++
				nonValidatorsInBatch++
				nd.Logger.Printf("Found non-validator peer %d: %s at %s (port %d)", 
					nonValidatorCount, peerIDStr, peer.RemoteIP, connectedPort)
				
				// Create a network peer with detailed information
				networkPeer := NetworkPeer{
					ID:          peerIDStr,
					IsValidator: false,
					Addresses:   []string{fmt.Sprintf("/ip4/%s/tcp/16593", peer.RemoteIP)},
					LastSeen:    time.Now(),
					Active:      true,
				}
				
				// Add version information
				networkPeer.Version = status.NodeInfo.Version
				
				// Add to network peers
				nd.AddressDir.AddNetworkPeer(networkPeer)
				
				// Log detailed information about the non-validator
				nd.Logger.Printf("Non-validator details - Version: %s, Moniker: %s", 
					status.NodeInfo.Version, status.NodeInfo.Moniker)
			}
		}
		
		nd.Logger.Printf("Batch %d summary: %d/%d successful connections, found %d non-validators, discovered %d new peers", 
			peerBatches, successfulConnections, len(batch), nonValidatorsInBatch, newPeersDiscovered)
		
		// If we didn't find any successful connections in this batch, skip to next batch
		if successfulConnections == 0 && len(cometPeers) > 0 {
			nd.Logger.Printf("No successful connections in batch %d, skipping to next batch", peerBatches)
		}
		
		// If we found a significant number of non-validators, we can stop early
		if nonValidatorCount >= 10 {
			nd.Logger.Printf("Found %d non-validators, which is sufficient. Stopping discovery early.", nonValidatorCount)
			break
		}
	}
	
	nd.Logger.Printf("Non-validator peer discovery completed. Found %d non-validator peers out of %d total peers processed.", 
		nonValidatorCount, len(seenCometIDs))
	return nil
}

// connectToTendermint connects to a node's Tendermint RPC
func (nd *NetworkDiscoveryImpl) connectToTendermint(host string) (client.Client, error) {
	// Try both the default port and the BVN port
	for _, port := range []int{16592, 16692} {
		base := fmt.Sprintf("http://%s:%d", host, port)
		c, err := http.New(base, base+"/ws")
		if err == nil {
			return c, nil
		}
	}
	return nil, fmt.Errorf("failed to connect to Tendermint RPC")
}

// httpClientForPeer creates an HTTP client for a Tendermint peer
func httpClientForPeer(peer coretypes.Peer, offset config.PortOffset) (*http.HTTP, error) {
	// peer.node_info.listen_addr should include the Tendermint P2P port
	i := strings.IndexByte(peer.NodeInfo.ListenAddr, ':')
	if i < 0 {
		return nil, fmt.Errorf("invalid listen address")
	}

	// Convert the port number to a string
	port, err := strconv.ParseUint(peer.NodeInfo.ListenAddr[i+1:], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}

	// Calculate the RPC port from the P2P port
	port = port - uint64(config.PortOffsetTendermintP2P) + uint64(config.PortOffsetTendermintRpc) + uint64(offset)

	// Construct the address from peer.remote_ip and the calculated port
	addr := fmt.Sprintf("http://%s:%d", peer.RemoteIP, port)

	// Create a new client
	return http.New(addr, addr+"/ws")
}
