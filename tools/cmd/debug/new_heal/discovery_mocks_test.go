// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// StatusResponse is a custom type for testing
type StatusResponse struct {
	NodeInfo struct {
		Network string
		Version string
	}
	SyncInfo struct {
		LatestBlockHeight int64
		LatestBlockTime   time.Time
		CatchingUp        bool
	}
}

// SimpleNetworkDefinition is a simplified version of protocol.NetworkDefinition
type SimpleNetworkDefinition struct {
	NetworkName string
	Validators  []SimpleValidatorInfo
}

// SimpleValidatorInfo is a simplified version of protocol.ValidatorInfo
type SimpleValidatorInfo struct {
	PublicKey []byte
	Name      string
	Partition string
}

// SimpleNetworkStatus is a simplified version of api.NetworkStatus
type SimpleNetworkStatus struct {
	Network SimpleNetworkDefinition
}

// MockNetworkService implements a simplified version of api.NetworkService for testing
type MockNetworkService struct {
	// Network status response
	NetworkStatusResponse *SimpleNetworkStatus
	NetworkStatusError    error

	// Status response
	StatusResponse *StatusResponse
	StatusError    error

	// Ping response
	PingError error

	// Delays for simulating network latency
	NetworkStatusDelay time.Duration
	StatusDelay        time.Duration
	PingDelay          time.Duration
}

// NetworkStatus implements the NetworkStatus method for testing
func (m *MockNetworkService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	if m.NetworkStatusDelay > 0 {
		time.Sleep(m.NetworkStatusDelay)
	}
	
	if m.NetworkStatusError != nil {
		return nil, m.NetworkStatusError
	}
	
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
	}, nil
}

// Status implements the Status method for testing
func (m *MockNetworkService) Status(ctx context.Context) (*StatusResponse, error) {
	if m.StatusDelay > 0 {
		time.Sleep(m.StatusDelay)
	}
	return m.StatusResponse, m.StatusError
}

// Ping implements the Ping method for testing
func (m *MockNetworkService) Ping(ctx context.Context) error {
	if m.PingDelay > 0 {
		time.Sleep(m.PingDelay)
	}
	return m.PingError
}

// CreateMockNetworkService creates a mock network service with default responses
func CreateMockNetworkService() *MockNetworkService {
	// Create a simplified mock network status response
	networkStatus := &SimpleNetworkStatus{
		Network: SimpleNetworkDefinition{
			NetworkName: "testnet",
			Validators: []SimpleValidatorInfo{
				{
					PublicKey: []byte("validator1-pubkey"),
					Name:      "validator1",
					Partition: "dn",
				},
				{
					PublicKey: []byte("validator2-pubkey"),
					Name:      "validator2",
					Partition: "Apollo",
				},
			},
		},
	}

	// Create a mock status response
	statusResponse := &StatusResponse{}
	statusResponse.NodeInfo.Network = "dn.acme"
	statusResponse.NodeInfo.Version = "v1.0.0"
	statusResponse.SyncInfo.LatestBlockHeight = 1000
	statusResponse.SyncInfo.LatestBlockTime = time.Now()
	statusResponse.SyncInfo.CatchingUp = false

	return &MockNetworkService{
		NetworkStatusResponse: networkStatus,
		StatusResponse:        statusResponse,
		NetworkStatusDelay:    10 * time.Millisecond,
		StatusDelay:           10 * time.Millisecond,
		PingDelay:             5 * time.Millisecond,
	}
}

// CreateMockHttpServer creates a mock HTTP server for testing address validation
func CreateMockHttpServer() *httptest.Server {
	handler := http.NewServeMux()
	
	// Mock RPC endpoint
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	})
	
	// Mock API endpoint
	handler.HandleFunc("/v2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	})
	
	// Mock metrics endpoint
	handler.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.\n")
	})
	
	return httptest.NewServer(handler)
}

// CreateMockValidator creates a mock validator for testing
func CreateMockValidator(name string, partitionID string) *Validator {
	partitionType := "bvn"
	if partitionID == "dn" {
		partitionType = "directory"
	}
	return &Validator{
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "active",
		LastUpdated:   time.Now(),
		Addresses:     []ValidatorAddress{},
	}
}
