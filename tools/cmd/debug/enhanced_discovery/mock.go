// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MockNetworkService implements a mock of api.NetworkService for testing
type MockNetworkService struct {
	// Network status response
	NetworkStatusResponse *api.NetworkStatus
	NetworkError          error

	// Response delay
	ResponseDelay time.Duration
	
	// Special fields for testing
	ZombieValidators map[string]bool
	
	// Peers for network peer discovery testing
	Peers map[string]NetworkPeer
}

// NetworkStatus implements the NetworkStatus method for testing
func (m *MockNetworkService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	if m.ResponseDelay > 0 {
		time.Sleep(m.ResponseDelay)
	}

	if m.NetworkError != nil {
		return nil, m.NetworkError
	}

	if m.NetworkStatusResponse != nil {
		return m.NetworkStatusResponse, nil
	}

	// Return a default network status if none is provided
	return createDefaultNetworkStatus(), nil
}

// createDefaultNetworkStatus creates a default network status for testing
func createDefaultNetworkStatus() *api.NetworkStatus {
	// Create a network definition with validators
	network := &protocol.NetworkDefinition{
		NetworkName: "testnet",
		Partitions: []*protocol.PartitionInfo{
			{
				ID:   "dn",
				Type: protocol.PartitionTypeDirectory,
			},
			{
				ID:   "bvn-apollo",
				Type: protocol.PartitionTypeBlockValidator,
			},
		},
		Validators: []*protocol.ValidatorInfo{
			{
				// Validator 1 - active in DN
				Operator: protocol.AccountUrl("operator1.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
				},
			},
			{
				// Validator 2 - active in BVN
				Operator: protocol.AccountUrl("operator2.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-apollo",
						Active: true,
					},
				},
			},
			{
				// Validator 3 - active in both
				Operator: protocol.AccountUrl("operator3.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
					{
						ID:     "bvn-apollo",
						Active: true,
					},
				},
			},
		},
	}

	return &api.NetworkStatus{
		Network: network,
	}
}

// NewMockNetworkService creates a new mock network service
func NewMockNetworkService() *MockNetworkService {
	return &MockNetworkService{
		ResponseDelay: 10 * time.Millisecond,
		Peers: make(map[string]NetworkPeer),
	}
}

// NetworkPeers implements the NetworkPeers method for testing
func (m *MockNetworkService) NetworkPeers(ctx context.Context, opts interface{}) (interface{}, error) {
	if m.ResponseDelay > 0 {
		time.Sleep(m.ResponseDelay)
	}

	if m.NetworkError != nil {
		return nil, m.NetworkError
	}

	// Create a simple response with our mock peers
	type PeerInfo struct {
		ID        string   `json:"id"`
		Addresses []string `json:"addresses"`
	}
	
	type PeersResponse struct {
		Peers []PeerInfo `json:"peers"`
	}
	
	response := &PeersResponse{
		Peers: make([]PeerInfo, 0),
	}
	
	// Add each peer to the response
	for _, peer := range m.Peers {
		response.Peers = append(response.Peers, PeerInfo{
			ID:        peer.ID,
			Addresses: peer.Addresses,
		})
	}
	
	return response, nil
}

// CreateMockValidator creates a mock validator for testing
func CreateMockValidator(name, partitionID string) *Validator {
	partitionType := "bvn"
	if partitionID == "dn" {
		partitionType = "directory"
	}

	return &Validator{
		Name:          name,
		PeerID:        name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "active",
		LastUpdated:   time.Now(),
		URLs:          make(map[string]string),
		BVNHeights:    make(map[string]uint64),
	}
}

// CreateTestValidator creates a test validator with all fields populated
func CreateTestValidator(name, partitionID string) *Validator {
	validator := CreateMockValidator(name, partitionID)
	
	// Set up enhanced fields
	validator.Version = "v1.0.0-test"
	validator.DNHeight = 1000
	validator.BVNHeight = 900
	
	// Set up addresses
	validator.IPAddress = "127.0.0.1"
	validator.P2PAddress = "tcp://127.0.0.1:26656"
	validator.RPCAddress = "http://127.0.0.1:26657"
	validator.APIAddress = "http://127.0.0.1:8080"
	validator.MetricsAddress = "http://127.0.0.1:26660"
	
	// Set up URLs
	validator.URLs["p2p"] = validator.P2PAddress
	validator.URLs["rpc"] = validator.RPCAddress
	validator.URLs["api"] = validator.APIAddress
	validator.URLs["metrics"] = validator.MetricsAddress
	
	return validator
}
