// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cometbft/cometbft/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// PeerStateInfo contains detailed state information for a peer
type PeerStateInfo struct {
	// Peer ID
	ID string

	// Whether this peer is a validator
	IsValidator bool

	// If this is a validator, the validator ID
	ValidatorID string

	// Partition information
	PartitionID string

	// Directory Network height
	DNHeight uint64

	// Block Validator Network height
	BVNHeight uint64

	// Whether this node is a zombie (not participating in consensus)
	IsZombie bool

	// Last time this peer was updated
	LastUpdated time.Time
}

// StateUpdateStats tracks statistics about a state update operation
type StateUpdateStats struct {
	// Total number of peers processed
	TotalPeers int

	// Number of peers with updated heights
	HeightUpdated int

	// Maximum Directory Network height observed
	DNHeightMax uint64

	// Maximum Block Validator Network height observed
	BVNHeightMax uint64

	// Number of Directory Network nodes significantly behind
	DNLaggingNodes int

	// Number of Block Validator Network nodes significantly behind
	BVNLaggingNodes int

	// IDs of peers with updated heights
	HeightUpdatedIDs []string

	// IDs of Directory Network nodes significantly behind
	DNLaggingNodeIDs []string

	// IDs of Block Validator Network nodes significantly behind
	BVNLaggingNodeIDs []string
}

// PeerState manages and tracks the state of network peers
type PeerState struct {
	// Reference to the AddressDir for peer information
	addressDir *AddressDir

	// Map of peer ID to peer state information
	peerStates map[string]PeerStateInfo

	// Mutex for concurrent access
	mu sync.RWMutex
}

// NewPeerState creates a new PeerState instance
func NewPeerState(addressDir *AddressDir) *PeerState {
	return &PeerState{
		addressDir: addressDir,
		peerStates: make(map[string]PeerStateInfo),
	}
}

// UpdatePeerStates updates the state of all peers
func (ps *PeerState) UpdatePeerStates(ctx context.Context, client api.NetworkService) (StateUpdateStats, error) {
	// Initialize statistics
	stats := StateUpdateStats{
		HeightUpdatedIDs:  make([]string, 0),
		DNLaggingNodeIDs:  make([]string, 0),
		BVNLaggingNodeIDs: make([]string, 0),
	}

	// Get all peers from AddressDir
	peers := ps.addressDir.GetNetworkPeers()
	stats.TotalPeers = len(peers)

	// Lock for writing
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Process each peer
	for _, peer := range peers {
		// Skip lost peers
		if peer.IsLost {
			continue
		}

		// Get or create peer state info
		peerState, exists := ps.peerStates[peer.ID]
		if !exists {
			peerState = PeerStateInfo{
				ID:          peer.ID,
				IsValidator: peer.IsValidator,
				ValidatorID: peer.ValidatorID,
				PartitionID: peer.PartitionID,
			}
		}

		// Update basic information
		peerState.IsValidator = peer.IsValidator
		peerState.ValidatorID = peer.ValidatorID
		peerState.PartitionID = peer.PartitionID

		// Get host information for this peer
		// This is a simplified approach - in a real implementation, you would need to
		// extract the host from the peer's addresses
		var host string
		for _, addr := range peer.Addresses {
			// Extract host from address
			// For simplicity, we'll assume the first address is usable
			parts := strings.Split(addr, "/")
			for i, part := range parts {
				if part == "ip4" && i+1 < len(parts) {
					host = parts[i+1]
					break
				}
			}
			if host != "" {
				break
			}
		}

		// Skip if no host is available
		if host == "" {
			continue
		}

		// Query heights for this peer
		prevDNHeight := peerState.DNHeight
		prevBVNHeight := peerState.BVNHeight

		err := ps.queryNodeHeights(ctx, &peerState, host)
		if err != nil {
			// Log the error but continue processing other peers
			fmt.Printf("Error querying heights for peer %s: %v\n", peer.ID, err)
			continue
		}

		// Check if heights were updated
		if peerState.DNHeight != prevDNHeight || peerState.BVNHeight != prevBVNHeight {
			stats.HeightUpdated++
			stats.HeightUpdatedIDs = append(stats.HeightUpdatedIDs, peer.ID)
		}

		// Update last updated timestamp
		peerState.LastUpdated = time.Now()

		// Update peer state in the map
		ps.peerStates[peer.ID] = peerState

		// Update maximum heights
		if peerState.DNHeight > stats.DNHeightMax {
			stats.DNHeightMax = peerState.DNHeight
		}
		if peerState.BVNHeight > stats.BVNHeightMax {
			stats.BVNHeightMax = peerState.BVNHeight
		}
	}

	// Identify lagging nodes
	// Any node more than 1 block behind is considered lagging since validators increment blocks in unison
	const significantLag = uint64(1)
	for peerID, state := range ps.peerStates {
		// Check DN height lag
		if state.DNHeight > 0 && stats.DNHeightMax > 0 {
			if stats.DNHeightMax-state.DNHeight > significantLag {
				stats.DNLaggingNodes++
				stats.DNLaggingNodeIDs = append(stats.DNLaggingNodeIDs, peerID)
			}
		}

		// Check BVN height lag
		if state.BVNHeight > 0 && stats.BVNHeightMax > 0 {
			if stats.BVNHeightMax-state.BVNHeight > significantLag {
				stats.BVNLaggingNodes++
				stats.BVNLaggingNodeIDs = append(stats.BVNLaggingNodeIDs, peerID)
			}
		}
	}

	return stats, nil
}

// queryNodeHeights queries the heights of a node for both DN and BVN partitions
// This implementation is consistent with the existing network query code in the codebase
func (ps *PeerState) queryNodeHeights(ctx context.Context, peerState *PeerStateInfo, host string) error {
	// Skip if no host is provided
	if host == "" {
		return fmt.Errorf("no host provided")
	}

	// Try both primary and secondary Tendermint RPC ports
	// Primary port is 16592, secondary is 16692 (as documented in address_design.md)
	for _, port := range []int{16592, 16692} {
		// Create the Tendermint RPC client
		base := fmt.Sprintf("http://%s:%d", host, port)
		c, err := http.New(base, base+"/ws")
		if err != nil {
			// Log the error but continue to the next port
			fmt.Printf("Error creating Tendermint client for %s:%d: %v\n", host, port, err)
			continue
		}

		// Set a timeout for the query
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Query the node status
		status, err := c.Status(queryCtx)
		if err != nil {
			fmt.Printf("Error querying status from %s:%d: %v\n", host, port, err)
			continue // Try the next port
		}

		// Extract the partition name from the network name
		// The network name format is typically: "accumulate-{network}.{partition}"
		// For example: "accumulate-mainnet.Directory" or "accumulate-mainnet.Apollo"
		part := status.NodeInfo.Network
		if i := strings.LastIndexByte(part, '.'); i >= 0 {
			part = part[i+1:]
		}
		part = strings.ToLower(part)

		// Extract the height information
		height := uint64(status.SyncInfo.LatestBlockHeight)

		// Update the appropriate height field based on the partition type
		if part == strings.ToLower(protocol.Directory) {
			// This is a Directory Network node
			peerState.DNHeight = height
			fmt.Printf("Updated DN height for peer %s: %d\n", peerState.ID, height)
		} else {
			// This is a BVN node
			peerState.BVNHeight = height
			fmt.Printf("Updated BVN height for peer %s (partition %s): %d\n", 
				peerState.ID, part, height)
		}

		// Check if the node is a zombie by examining consensus participation
		// A node is considered a zombie if it's not participating in consensus
		// For now, we'll use a simplified approach and check if the node has recent votes
		consensusState, err := c.ConsensusState(queryCtx)
		if err == nil && consensusState != nil {
			// If we can get a consensus state, the node is likely not a zombie
			peerState.IsZombie = false
		} else {
			// If we can't get a consensus state, the node might be a zombie
			// But we need more evidence, so we'll check if it's syncing
			peerState.IsZombie = !status.SyncInfo.CatchingUp
		}

		// Successfully queried this port, no need to try the next one
		return nil
	}

	return fmt.Errorf("failed to query node status on any port")
}

// GetPeerState returns the state information for a specific peer
func (ps *PeerState) GetPeerState(peerID string) (PeerStateInfo, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	state, exists := ps.peerStates[peerID]
	return state, exists
}

// GetAllPeerStates returns all peer states
func (ps *PeerState) GetAllPeerStates() []PeerStateInfo {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	states := make([]PeerStateInfo, 0, len(ps.peerStates))
	for _, state := range ps.peerStates {
		states = append(states, state)
	}
	return states
}

// GetLaggingPeers returns peers that are significantly behind
// By default, any node more than 1 block behind is considered lagging since validators increment blocks in unison
func (ps *PeerState) GetLaggingPeers(significantLag uint64) []PeerStateInfo {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// Find maximum heights
	var dnHeightMax, bvnHeightMax uint64
	for _, state := range ps.peerStates {
		if state.DNHeight > dnHeightMax {
			dnHeightMax = state.DNHeight
		}
		if state.BVNHeight > bvnHeightMax {
			bvnHeightMax = state.BVNHeight
		}
	}

	// Find lagging peers
	laggingPeers := make([]PeerStateInfo, 0)
	for _, state := range ps.peerStates {
		isDNLagging := state.DNHeight > 0 && dnHeightMax > 0 && dnHeightMax-state.DNHeight > significantLag
		isBVNLagging := state.BVNHeight > 0 && bvnHeightMax > 0 && bvnHeightMax-state.BVNHeight > significantLag

		if isDNLagging || isBVNLagging {
			laggingPeers = append(laggingPeers, state)
		}
	}

	return laggingPeers
}

// GetPeersByPartition returns peers belonging to a specific partition
func (ps *PeerState) GetPeersByPartition(partitionID string) []PeerStateInfo {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	peers := make([]PeerStateInfo, 0)
	for _, state := range ps.peerStates {
		if state.PartitionID == partitionID {
			peers = append(peers, state)
		}
	}

	return peers
}
