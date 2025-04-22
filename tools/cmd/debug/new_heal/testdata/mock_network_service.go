// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// NetworkData represents the collected network information
type NetworkData struct {
	NetworkStatus *api.NetworkStatus            `json:"networkStatus"`
	Partitions    map[string]*api.NetworkStatus `json:"partitions"`
	Timestamp     string                        `json:"timestamp"`
}

// MockNetworkService is a mock implementation of the NetworkService interface
// that uses pre-recorded network data
type MockNetworkService struct {
	networkData *NetworkData
}

// NewMockNetworkService creates a new MockNetworkService from a JSON file
func NewMockNetworkService(filePath string) (*MockNetworkService, error) {
	// Read the JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read network data file: %v", err)
	}

	// Parse the JSON data
	var networkData NetworkData
	err = json.Unmarshal(data, &networkData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse network data: %v", err)
	}

	return &MockNetworkService{
		networkData: &networkData,
	}, nil
}

// NetworkStatus implements the NetworkService interface
func (m *MockNetworkService) NetworkStatus(ctx context.Context, options api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	// If a partition is specified, return the network status for that partition
	if options.Partition != "" {
		// Convert to lowercase for case-insensitive comparison
		partitionID := strings.ToLower(options.Partition)
		
		// Check each partition
		for id, status := range m.networkData.Partitions {
			if strings.ToLower(id) == partitionID {
				return status, nil
			}
		}
		
		return nil, fmt.Errorf("partition not found: %s", options.Partition)
	}
	
	// Otherwise, return the main network status
	return m.networkData.NetworkStatus, nil
}

// DefaultMockNetworkService returns a MockNetworkService using the default network data file
func DefaultMockNetworkService() (*MockNetworkService, error) {
	// Get the directory of this file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("failed to get current file path")
	}
	
	// Construct the path to the network data file
	filePath := filepath.Join(filepath.Dir(filename), "network_data.json")
	
	return NewMockNetworkService(filePath)
}
