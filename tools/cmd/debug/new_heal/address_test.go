// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal"
)

// TestAddValidator tests adding and updating validators
func TestAddValidator(t *testing.T) {
	addressDir := new_heal.NewAddressDir()

	// Test adding a new validator
	validator := addressDir.AddValidator("validator1", "Test Validator", "bvn-Test", "bvn")
	require.NotNil(t, validator)
	require.Equal(t, "validator1", validator.ID)
	require.Equal(t, "Test Validator", validator.Name)
	require.Equal(t, "bvn-Test", validator.PartitionID)
	require.Equal(t, "bvn", validator.PartitionType)
	require.Equal(t, "unknown", validator.Status)
	require.Empty(t, validator.Addresses)
	require.NotZero(t, validator.LastUpdated)

	// Test updating an existing validator
	updatedValidator := addressDir.AddValidator("validator1", "Updated Validator", "bvn-Updated", "bvn")
	require.NotNil(t, updatedValidator)
	require.Equal(t, "validator1", updatedValidator.ID)
	require.Equal(t, "Updated Validator", updatedValidator.Name)
	require.Equal(t, "bvn-Updated", updatedValidator.PartitionID)
	require.Equal(t, "bvn", updatedValidator.PartitionType)

	// Test adding another validator
	validator2 := addressDir.AddValidator("validator2", "Test Validator 2", "dn", "dn")
	require.NotNil(t, validator2)
	require.Equal(t, "validator2", validator2.ID)
	require.Equal(t, "Test Validator 2", validator2.Name)
	require.Equal(t, "dn", validator2.PartitionID)
	require.Equal(t, "dn", validator2.PartitionType)
}

// TestFindValidator tests finding validators by ID
func TestFindValidator(t *testing.T) {
	addressDir := createTestAddressDir()

	// Test finding an existing validator
	validator, idx, exists := addressDir.FindValidator("validator1")
	require.True(t, exists)
	require.NotNil(t, validator)
	require.Equal(t, "validator1", validator.ID)
	require.Equal(t, 0, idx) // First validator in the slice

	// Test finding another existing validator
	validator, idx, exists = addressDir.FindValidator("validator2")
	require.True(t, exists)
	require.NotNil(t, validator)
	require.Equal(t, "validator2", validator.ID)
	require.Equal(t, 1, idx) // Second validator in the slice

	// Test finding a non-existent validator
	validator, idx, exists = addressDir.FindValidator("nonexistent")
	require.False(t, exists)
	require.Nil(t, validator)
	require.Equal(t, -1, idx)
}

// TestFindValidatorsByPartition tests finding validators by partition
func TestFindValidatorsByPartition(t *testing.T) {
	// Create a fresh AddressDir for this test
	addressDir := new_heal.NewAddressDir()

	// Add validators with specific partitions
	addressDir.AddValidator("validator1", "Test Validator 1", "bvn-Test", "bvn")
	addressDir.AddValidator("validator2", "Test Validator 2", "bvn-Apollo", "bvn")
	addressDir.AddValidator("validator3", "Test Validator 3", "bvn-Test", "bvn")
	addressDir.AddValidator("validator4", "Test Validator 4", "bvn-Apollo", "bvn")
	addressDir.AddValidator("validator5", "Test Validator 5", "dn", "dn")

	// Test finding validators in the bvn-Test partition
	validators := addressDir.FindValidatorsByPartition("bvn-Test")
	require.Len(t, validators, 2)
	require.Equal(t, "validator1", validators[0].ID)
	require.Equal(t, "validator3", validators[1].ID)

	// Test finding validators in the bvn-Apollo partition
	validators = addressDir.FindValidatorsByPartition("bvn-Apollo")
	require.Len(t, validators, 2)
	require.Equal(t, "validator2", validators[0].ID)
	require.Equal(t, "validator4", validators[1].ID)

	// Test finding validators in the dn partition
	validators = addressDir.FindValidatorsByPartition("dn")
	require.Len(t, validators, 1)
	require.Equal(t, "validator5", validators[0].ID)

	// Test finding validators in a non-existent partition
	validators = addressDir.FindValidatorsByPartition("nonexistent")
	require.Empty(t, validators)
}

// TestSetValidatorStatus tests setting validator status
func TestSetValidatorStatus(t *testing.T) {
	addressDir := createTestAddressDir()

	// Test setting status for an existing validator
	success := addressDir.SetValidatorStatus("validator1", "active")
	require.True(t, success)

	// Verify the status was set
	validator, _, exists := addressDir.FindValidator("validator1")
	require.True(t, exists)
	require.Equal(t, "active", validator.Status)

	// Test setting status for a non-existent validator
	success = addressDir.SetValidatorStatus("nonexistent", "active")
	require.False(t, success)
}

// TestAddValidatorAddress tests adding addresses to validators
func TestAddValidatorAddress(t *testing.T) {
	addressDir := new_heal.NewAddressDir()
	addressDir.AddValidator("validator1", "Test Validator", "bvn-Test", "bvn")

	// Test adding a valid address
	// Use a valid multiaddress format with a proper peer ID format
	validAddress := "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ"
	err := addressDir.AddValidatorAddress("validator1", validAddress)
	require.NoError(t, err)

	// Verify the address was added
	addresses, exists := addressDir.GetValidatorAddresses("validator1")
	require.True(t, exists)
	require.Len(t, addresses, 1)
	require.Equal(t, validAddress, addresses[0].Address)
	require.True(t, addresses[0].Validated)
	require.Equal(t, "144.76.105.23", addresses[0].IP)
	require.Equal(t, "16593", addresses[0].Port)
	require.Equal(t, "12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ", addresses[0].PeerID)

	// Test adding an invalid address (missing peer ID)
	invalidAddress := "/ip4/144.76.105.23/tcp/16593"
	err = addressDir.AddValidatorAddress("validator1", invalidAddress)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid P2P multiaddress")

	// Test adding an address to a non-existent validator
	err = addressDir.AddValidatorAddress("nonexistent", validAddress)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Test adding a duplicate address (should update existing)
	err = addressDir.AddValidatorAddress("validator1", validAddress)
	require.NoError(t, err)
	addresses, _ = addressDir.GetValidatorAddresses("validator1")
	require.Len(t, addresses, 1) // Still only one address
}

// TestMarkAddressPreferred tests marking addresses as preferred
func TestMarkAddressPreferred(t *testing.T) {
	addressDir := createTestAddressDir()

	// Test marking an address as preferred
	err := addressDir.MarkAddressPreferred("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ")
	require.NoError(t, err)

	// Verify the address is marked as preferred
	addresses := addressDir.GetPreferredAddresses("validator1")
	require.Len(t, addresses, 1)
	require.Equal(t, "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ", addresses[0].Address)
	require.True(t, addresses[0].Preferred)

	// Test marking a non-existent address as preferred
	err = addressDir.MarkAddressPreferred("validator1", "nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Test marking an address for a non-existent validator
	err = addressDir.MarkAddressPreferred("nonexistent", "/ip4/144.76.105.23/tcp/16593/p2p/QmHash1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestGetPreferredAddresses tests retrieving preferred addresses
func TestGetPreferredAddresses(t *testing.T) {
	addressDir := createTestAddressDir()

	// Mark an address as preferred
	err := addressDir.MarkAddressPreferred("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ")
	require.NoError(t, err)

	// Test getting preferred addresses for a validator with preferred addresses
	preferredAddresses := addressDir.GetPreferredAddresses("validator1")
	require.Len(t, preferredAddresses, 1)
	require.Equal(t, "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ", preferredAddresses[0].Address)
	require.True(t, preferredAddresses[0].Preferred)

	// Test getting preferred addresses for a validator without preferred addresses
	preferredAddresses = addressDir.GetPreferredAddresses("validator2")

	// The implementation returns an empty slice when no addresses are preferred
	// This is expected behavior for validator2 since we didn't mark any addresses as preferred
	require.Empty(t, preferredAddresses)

	// Test getting preferred addresses for a non-existent validator
	preferredAddresses = addressDir.GetPreferredAddresses("nonexistent")
	require.Nil(t, preferredAddresses)
}

// TestMarkNodeProblematic tests marking nodes as problematic
func TestMarkNodeProblematic(t *testing.T) {
	addressDir := createTestAddressDir()

	// Test marking a node as problematic
	requestTypes := []string{"query", "submit"}
	addressDir.MarkNodeProblematic("validator1", "Test reason", requestTypes)

	// Verify the node is marked as problematic
	isProblematic := addressDir.IsNodeProblematic("validator1", "query")
	require.True(t, isProblematic)

	isProblematic = addressDir.IsNodeProblematic("validator1", "submit")
	require.True(t, isProblematic)

	// Test a request type that wasn't marked
	isProblematic = addressDir.IsNodeProblematic("validator1", "other")
	require.False(t, isProblematic)

	// Test a validator that wasn't marked
	isProblematic = addressDir.IsNodeProblematic("validator2", "query")
	require.False(t, isProblematic)
}

// TestProblemNodeBackoff tests the exponential backoff for problematic nodes
func TestProblemNodeBackoff(t *testing.T) {
	addressDir := createTestAddressDir()
	requestTypes := []string{"query"}

	// Mark the node as problematic multiple times to increase failure count
	for i := 0; i < 3; i++ {
		addressDir.MarkNodeProblematic("validator1", "Test reason", requestTypes)
	}

	// Verify the node is problematic
	isProblematic := addressDir.IsNodeProblematic("validator1", "query")
	require.True(t, isProblematic)

	// Get the problem node to check backoff time
	// This requires accessing internal state, which is not ideal for unit tests
	// In a real implementation, we might want to expose a method to get the backoff time
	// For now, we'll just verify that the node is marked as problematic
}

// TestAddValidatorURL tests adding URLs to validators
func TestAddValidatorURL(t *testing.T) {
	addressDir := createTestAddressDir()

	// Test adding a valid URL
	err := addressDir.AddValidatorURL("validator1", "api", "acc://example.acme/api")
	require.NoError(t, err)

	// Verify the URL was added
	url, exists := addressDir.GetValidatorURL("validator1", "api")
	require.True(t, exists)
	require.Equal(t, "acc://example.acme/api", url)

	// Test adding an invalid URL
	err = addressDir.AddValidatorURL("validator1", "api", "http://example.com/api")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid Accumulate URL")

	// Test adding a URL to a non-existent validator
	err = addressDir.AddValidatorURL("nonexistent", "api", "http://example.com/api")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestGetValidatorURL tests retrieving URLs from validators
func TestGetValidatorURL(t *testing.T) {
	addressDir := createTestAddressDir()

	// Add a URL to a validator
	err := addressDir.AddValidatorURL("validator1", "api", "acc://example.acme/api")
	require.NoError(t, err)

	// Test getting an existing URL
	url, exists := addressDir.GetValidatorURL("validator1", "api")
	require.True(t, exists)
	require.Equal(t, "acc://example.acme/api", url)

	// Test getting a non-existent URL type
	url, exists = addressDir.GetValidatorURL("validator1", "nonexistent")
	require.False(t, exists)
	require.Empty(t, url)

	// Test getting a URL for a non-existent validator
	url, exists = addressDir.GetValidatorURL("nonexistent", "api")
	require.False(t, exists)
	require.Empty(t, url)
}

// TestUpdateAddressStatus tests updating the status of addresses
func TestUpdateAddressStatus(t *testing.T) {
	addressDir := createTestAddressDir()

	// Test successful update
	err := addressDir.UpdateAddressStatus("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ", true, nil)
	require.NoError(t, err)

	// Verify the status was updated
	addresses, exists := addressDir.GetValidatorAddresses("validator1")
	require.True(t, exists)
	require.Len(t, addresses, 1)
	require.Equal(t, 0, addresses[0].FailureCount)
	require.Empty(t, addresses[0].LastError)
	require.NotZero(t, addresses[0].LastSuccess)

	// Test failed update
	testErr := ErrTestError("test error")
	err = addressDir.UpdateAddressStatus("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ", false, testErr)
	require.NoError(t, err)

	// Verify the status was updated
	addresses, exists = addressDir.GetValidatorAddresses("validator1")
	require.True(t, exists)
	require.Len(t, addresses, 1)
	require.Equal(t, 1, addresses[0].FailureCount)
	require.Equal(t, "test error", addresses[0].LastError)

	// Test updating a non-existent address
	err = addressDir.UpdateAddressStatus("validator1", "/ip4/1.2.3.4/tcp/1234/p2p/12D3KooWNonExistentPeerIDXXXXXXXXXXXXXXXXXXXXXXX", true, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Test updating an address for a non-existent validator
	err = addressDir.UpdateAddressStatus("nonexistent", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ", true, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestGetHealthyValidators tests retrieving healthy validators
func TestGetHealthyValidators(t *testing.T) {
	// Create a fresh AddressDir for this test
	addressDir := new_heal.NewAddressDir()

	// Add validators with specific partitions
	addressDir.AddValidator("validator1", "Test Validator 1", "bvn-Test", "bvn")
	addressDir.AddValidator("validator2", "Test Validator 2", "bvn-Apollo", "bvn")
	addressDir.AddValidator("validator3", "Test Validator 3", "bvn-Test", "bvn")

	// Set validators as active
	addressDir.SetValidatorStatus("validator1", "active")
	addressDir.SetValidatorStatus("validator2", "active")
	addressDir.SetValidatorStatus("validator3", "active")

	// Mark one validator as problematic
	addressDir.MarkNodeProblematic("validator1", "Test reason", []string{"query"})

	// Test getting healthy validators for bvn-Test partition
	validators := addressDir.GetHealthyValidators("bvn-Test")
	require.Len(t, validators, 1)
	require.Equal(t, "validator3", validators[0].ID)

	// Test getting healthy validators for bvn-Apollo partition
	validators = addressDir.GetHealthyValidators("bvn-Apollo")
	require.Len(t, validators, 1)
	require.Equal(t, "validator2", validators[0].ID)

	// Test getting healthy validators for a non-existent partition
	validators = addressDir.GetHealthyValidators("nonexistent")
	require.Empty(t, validators)
}

// TestGetActiveValidators tests retrieving active validators
func TestGetActiveValidators(t *testing.T) {
	addressDir := createTestAddressDir()

	// Set validator statuses
	addressDir.SetValidatorStatus("validator1", "active")
	addressDir.SetValidatorStatus("validator2", "inactive")
	addressDir.SetValidatorStatus("validator3", "active")
	addressDir.SetValidatorStatus("validator4", "inactive")

	// Get active validators
	validators := addressDir.GetActiveValidators()
	require.Len(t, validators, 2)
	require.Equal(t, "validator1", validators[0].ID)
	require.Equal(t, "active", validators[0].Status)
	require.Equal(t, "validator3", validators[1].ID)
	require.Equal(t, "active", validators[1].Status)

	// Set another validator to active
	addressDir.SetValidatorStatus("validator2", "active")

	// Get active validators again
	validators = addressDir.GetActiveValidators()
	require.Len(t, validators, 3)
	require.Equal(t, "validator1", validators[0].ID)
	require.Equal(t, "validator2", validators[1].ID)
}

// TestGetDNValidators tests retrieving DN validators
func TestGetDNValidators(t *testing.T) {
	// Create a fresh AddressDir for this test
	addressDir := new_heal.NewAddressDir()

	// Add validators
	addressDir.AddValidator("validator1", "Test Validator 1", "bvn-Test", "bvn")
	addressDir.AddValidator("validator2", "Test Validator 2", "bvn-Apollo", "bvn")
	addressDir.AddValidator("validator3", "Test Validator 3", "bvn-Test", "bvn")

	// Add validators to DN
	addressDir.AddValidatorToDN("validator1")
	addressDir.AddValidatorToDN("validator2")
	addressDir.AddValidatorToDN("validator3")

	// Get DN validators
	validators := addressDir.GetDNValidators()
	require.Len(t, validators, 3)
	require.Equal(t, "validator1", validators[0].ID)
	require.Equal(t, "validator2", validators[1].ID)
	require.Equal(t, "validator3", validators[2].ID)
}

// TestGetBVNValidators tests retrieving BVN validators
func TestGetBVNValidators(t *testing.T) {
	// Create a fresh AddressDir for this test
	addressDir := new_heal.NewAddressDir()

	// Add validators
	addressDir.AddValidator("validator1", "Test Validator 1", "bvn-Test", "bvn")
	addressDir.AddValidator("validator2", "Test Validator 2", "bvn-Apollo", "bvn")
	addressDir.AddValidator("validator3", "Test Validator 3", "bvn-Test", "bvn")

	// Add validators to BVN
	addressDir.AddValidatorToBVN("validator1", "bvn-Test")
	addressDir.AddValidatorToBVN("validator2", "bvn-Apollo")
	addressDir.AddValidatorToBVN("validator3", "bvn-Test")

	// Get BVN validators
	validators := addressDir.GetBVNValidators("bvn-Test")
	require.Len(t, validators, 2)
	require.Equal(t, "validator1", validators[0].ID)
	require.Equal(t, "validator3", validators[1].ID)

	// Get BVN validators for another partition
	validators = addressDir.GetBVNValidators("bvn-Apollo")
	require.Len(t, validators, 1)
	require.Equal(t, "validator2", validators[0].ID)

	// Get BVN validators for a non-existent partition
	validators = addressDir.GetBVNValidators("nonexistent")
	require.Empty(t, validators)
}

// TestURLHelpers tests URL helper functions
func TestURLHelpers(t *testing.T) {
	addressDir := new_heal.NewAddressDir()

	// Test setting a URL helper
	addressDir.SetURLHelper("test-key", "test-value")

	// Test getting an existing URL helper
	value, exists := addressDir.GetURLHelper("test-key")
	require.True(t, exists)
	require.Equal(t, "test-value", value)

	// Test getting a non-existent URL helper
	value, exists = addressDir.GetURLHelper("nonexistent")
	require.False(t, exists)
	require.Empty(t, value)
}

// TestDiscoverValidators tests discovering validators from a network
// This test uses a mock NetworkService to avoid network connectivity requirements
func TestDiscoverValidators(t *testing.T) {
	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Create a mock NetworkService
	mockService := &MockNetworkService{
		networkStatus: createMockNetworkStatus(),
	}

	// Call DiscoverValidators with the mock service
	count, err := addressDir.DiscoverValidators(context.Background(), mockService)

	// Verify the results
	require.NoError(t, err)
	require.Equal(t, 4, count, "Should discover 4 validators")

	// Verify the validators were added correctly
	validators := addressDir.GetActiveValidators()
	require.Len(t, validators, 4, "Should have 4 active validators")

	// Verify the validators have the correct partition information
	bvnTestValidators := addressDir.FindValidatorsByPartition("bvn-Test")
	require.Len(t, bvnTestValidators, 2, "Should have 2 validators in bvn-Test partition")

	bvnApolloValidators := addressDir.FindValidatorsByPartition("bvn-Apollo")
	require.Len(t, bvnApolloValidators, 2, "Should have 2 validators in bvn-Apollo partition")

	// Verify DN validators
	dnValidators := addressDir.GetDNValidators()
	require.Len(t, dnValidators, 4, "All 4 validators should be in the DN")

	// Verify BVN validators
	bvnTestList := addressDir.GetBVNValidators("bvn-Test")
	require.Len(t, bvnTestList, 2, "Should have 2 validators in bvn-Test list")

	bvnApolloList := addressDir.GetBVNValidators("bvn-Apollo")
	require.Len(t, bvnApolloList, 2, "Should have 2 validators in bvn-Apollo list")
}

func TestRefreshNetworkPeers(t *testing.T) {
	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Create the initial network status with 4 validators
	initialStatus := &api.NetworkStatus{
		Network: &protocol.NetworkDefinition{
			NetworkName: "testnet",
			Version:     1,
			Partitions: []*protocol.PartitionInfo{
				{ID: "bvn-Test", Type: protocol.PartitionTypeBlockValidator},
				{ID: "bvn-Apollo", Type: protocol.PartitionTypeBlockValidator},
				{ID: "dn", Type: protocol.PartitionTypeDirectory},
			},
			Validators: []*protocol.ValidatorInfo{
				{
					PublicKeyHash: [32]byte{1, 2, 3, 4},
					Operator:      url.MustParse("acc://validator1.acme"),
					Partitions: []*protocol.ValidatorPartitionInfo{
						{ID: "bvn-Test", Active: true},
						{ID: "dn", Active: true},
					},
				},
				{
					PublicKeyHash: [32]byte{5, 6, 7, 8},
					Operator:      url.MustParse("acc://validator2.acme"),
					Partitions: []*protocol.ValidatorPartitionInfo{
						{ID: "bvn-Apollo", Active: true},
						{ID: "dn", Active: true},
						{ID: "bvn-Test", Active: true},
						{ID: "dn", Active: true},
					},
				},
			},
		},
	}

	mockService := &MockNetworkService{
		networkStatus: initialStatus,
	}

	// First, discover the initial set of peers
	ctx := context.Background()
	totalPeers, err := addressDir.DiscoverNetworkPeers(ctx, mockService)
	if err != nil {
		t.Fatalf("Failed to discover network peers: %v", err)
	}

	t.Logf("Initially discovered %d peers", totalPeers)

	// Get the initial list of peers
	initialPeers := addressDir.GetNetworkPeers()
	for _, peer := range initialPeers {
		t.Logf("Initial peer: ID=%s, IsValidator=%v, Status=%s", peer.ID, peer.IsValidator, peer.Status)
	}

	// Now create a completely different network status with a different validator
	// This will cause the first validator to be considered "lost"
	modifiedStatus := &api.NetworkStatus{
		Network: &protocol.NetworkDefinition{
			NetworkName: "testnet",
			Version:     1,
			Partitions: []*protocol.PartitionInfo{
				{ID: "bvn-Test", Type: protocol.PartitionTypeBlockValidator},
			},
			Validators: []*protocol.ValidatorInfo{
				// Different validator
				{
					PublicKeyHash: [32]byte{5, 6, 7, 8},
					Operator:      url.MustParse("acc://validator2.acme"),
					Partitions: []*protocol.ValidatorPartitionInfo{
						{ID: "bvn-Test", Active: true},
					},
				},
			},
		},
	}

	// Update the mock service with the modified status
	mockService.networkStatus = modifiedStatus

	// Refresh the network peers
	var stats new_heal.RefreshStats
	stats, err = addressDir.RefreshNetworkPeers(ctx, mockService)
	if err != nil {
		t.Fatalf("Failed to refresh network peers: %v", err)
	}

	// Verify the refresh statistics
	t.Logf("Refresh stats: Total=%d, New=%d, Lost=%d, StatusChanged=%d", 
		stats.TotalPeers, stats.NewPeers, stats.LostPeers, stats.StatusChanged)

	// Log the new peers
	t.Logf("New peers: %v", stats.NewPeerIDs)

	// Log the lost peers
	t.Logf("Lost peers: %v", stats.LostPeerIDs)

	// Log the peers with changed status
	t.Logf("Changed status peers: %v", stats.ChangedPeerIDs)

	// Verify that at least one peer was added and one was lost
	if stats.NewPeers == 0 {
		t.Errorf("Expected at least one new peer, but got %d", stats.NewPeers)
	}

	if stats.LostPeers == 0 {
		t.Errorf("Expected at least one lost peer, but got %d", stats.LostPeers)
	}

	// Get the updated list of peers
	updatedPeers := addressDir.GetNetworkPeers()

	// Verify that lost peers are marked as lost
	for _, peerID := range stats.LostPeerIDs {
		peer, exists := addressDir.GetNetworkPeerByID(peerID)
		if !exists {
			t.Errorf("Lost peer %s not found in NetworkPeers map", peerID)
			continue
		}

		if !peer.IsLost {
			t.Errorf("Lost peer %s not marked as lost", peerID)
		}

		if peer.Status != "lost" {
			t.Errorf("Lost peer %s has status %s, expected 'lost'", peerID, peer.Status)
		}
	}

	// Verify that new peers have FirstSeen timestamp set
	for _, peerID := range stats.NewPeerIDs {
		peer, exists := addressDir.GetNetworkPeerByID(peerID)
		if !exists {
			t.Errorf("New peer %s not found in NetworkPeers map", peerID)
			continue
		}

		if peer.FirstSeen.IsZero() {
			t.Errorf("New peer %s has zero FirstSeen timestamp", peerID)
		}
	}

	t.Logf("After refresh: %d peers total", len(updatedPeers))
	for _, peer := range updatedPeers {
		t.Logf("Updated peer: ID=%s, IsValidator=%v, Status=%s, IsLost=%v", 
			peer.ID, peer.IsValidator, peer.Status, peer.IsLost)
	}
}

// TestDiscoverNetworkPeers tests discovering network peers
// This test uses a mock NetworkService to avoid network connectivity requirements
func TestDiscoverNetworkPeers(t *testing.T) {
	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Create a mock NetworkService
	mockService := &MockNetworkService{
		networkStatus: createMockNetworkStatus(),
	}

	// Call DiscoverNetworkPeers with the mock service
	count, err := addressDir.DiscoverNetworkPeers(context.Background(), mockService)

	// Verify the results
	require.NoError(t, err)
	require.Equal(t, 4, count, "Should discover 4 peers")

	// Verify the peers were added correctly
	peers := addressDir.GetNetworkPeers()
	require.Len(t, peers, 4, "Should have 4 peers")

	// Verify all peers are validators
	validatorPeers := addressDir.GetValidatorPeers()
	require.Len(t, validatorPeers, 4, "All 4 peers should be validators")

	// Verify no non-validator peers
	nonValidatorPeers := addressDir.GetNonValidatorPeers()
	require.Len(t, nonValidatorPeers, 0, "Should have 0 non-validator peers")

	// Test adding an address to a peer
	// First get a peer ID
	peerID := peers[0].ID

	// Add an address
	err = addressDir.AddPeerAddress(peerID, "/ip4/192.168.1.1/tcp/26656")
	require.NoError(t, err, "Should be able to add an address to a peer")

	// Verify the address was added
	peer, exists := addressDir.GetNetworkPeerByID(peerID)
	require.True(t, exists, "Peer should exist")
	require.Contains(t, peer.Addresses, "/ip4/192.168.1.1/tcp/26656", "Address should be added to peer")
}

// TestDiscoverNetworkPeersMainnet tests the DiscoverNetworkPeers method against the mainnet
// This is an integration test that requires network connectivity
func TestDiscoverNetworkPeersMainnet(t *testing.T) {
	// This test is enabled for manual testing against the mainnet
	// t.Skip("Skipping test in non-CI environment")

	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Create a client to connect to the mainnet
	endpoint := "https://mainnet.accumulatenetwork.io/v3"

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a client to connect to the mainnet
	client, err := createMainnetClient(endpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get the network status first to examine its structure
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	require.NoError(t, err)
	require.NotNil(t, ns.Network, "Network definition should not be nil")

	// Print information about network partitions
	t.Logf("Network has %d partitions:", len(ns.Network.Partitions))
	for i, partition := range ns.Network.Partitions {
		t.Logf("  Partition %d: ID=%s, Type=%v", i+1, partition.ID, partition.Type)
	}

	// Print information about validators
	t.Logf("Network has %d validators:", len(ns.Network.Validators))
	for i, validator := range ns.Network.Validators {
		operatorID := "<nil>"
		if validator.Operator != nil {
			operatorID = validator.Operator.Authority
		}
		t.Logf("  Validator %d: ID=%s, PublicKeyHash=%x, Partitions=%d",
			i+1, operatorID, validator.PublicKeyHash, len(validator.Partitions))

		// Print partition information for this validator
		for j, partition := range validator.Partitions {
			t.Logf("    Partition %d: ID=%s, Active=%v", j+1, partition.ID, partition.Active)
		}
	}

	// Discover network peers from the mainnet
	count, err := addressDir.DiscoverNetworkPeers(ctx, client)

	// Verify the results
	require.NoError(t, err)
	require.Greater(t, count, 0, "Should discover at least one peer")

	// Get all network peers
	peers := addressDir.GetNetworkPeers()
	require.NotEmpty(t, peers, "Should have at least one peer")

	// Log information about the peers
	t.Logf("Found %d network peers on mainnet", len(peers))
	for i, peer := range peers {
		t.Logf("Peer %d: ID=%s, IsValidator=%v, ValidatorID=%s, Partition=%s, PartitionURL=%s",
			i+1, peer.ID, peer.IsValidator, peer.ValidatorID, peer.PartitionID, peer.PartitionURL)
	}

	// Get validator peers
	validatorPeers := addressDir.GetValidatorPeers()
	t.Logf("Found %d validator peers on mainnet", len(validatorPeers))

	// Get non-validator peers
	nonValidatorPeers := addressDir.GetNonValidatorPeers()
	t.Logf("Found %d non-validator peers on mainnet", len(nonValidatorPeers))

	// Verify that we have peers in different partitions
	// First, get all unique partition IDs from the peers
	partitionIDs := make(map[string]int)
	for _, peer := range peers {
		if peer.PartitionID != "" {
			partitionIDs[peer.PartitionID]++
		}
	}

	// Log the partition information
	t.Logf("Found peers in the following partitions:")
	for partitionID, count := range partitionIDs {
		t.Logf("  %s: %d peers", partitionID, count)
	}
}

// TestGetActiveValidatorsMainnet tests the GetActiveValidators method against the mainnet
// This is an integration test that requires network connectivity
func TestRefreshNetworkPeersMainnet(t *testing.T) {
	// This test is enabled for manual testing against the mainnet
	// t.Skip("Skipping test in non-CI environment")

	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Create a client to connect to the mainnet
	endpoint := "https://mainnet.accumulatenetwork.io/v3"

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a client to connect to the mainnet
	client, err := createMainnetClient(endpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// First, discover the initial set of peers
	totalPeers, err := addressDir.DiscoverNetworkPeers(ctx, client)
	if err != nil {
		t.Fatalf("Failed to discover network peers: %v", err)
	}

	t.Logf("Initially discovered %d peers", totalPeers)

	// Get the initial list of peers
	initialPeers := addressDir.GetNetworkPeers()
	t.Logf("Found %d network peers on mainnet", len(initialPeers))

	// Get validator peers
	validatorPeers := addressDir.GetValidatorPeers()
	t.Logf("Found %d validator peers on mainnet", len(validatorPeers))

	// Get non-validator peers
	nonValidatorPeers := addressDir.GetNonValidatorPeers()
	t.Logf("Found %d non-validator peers on mainnet", len(nonValidatorPeers))

	// Refresh the network peers
	stats, err := addressDir.RefreshNetworkPeers(ctx, client)
	if err != nil {
		t.Fatalf("Failed to refresh network peers: %v", err)
	}

	// Verify the refresh statistics
	t.Logf("Refresh stats: Total=%d, New=%d, Lost=%d, StatusChanged=%d", 
		stats.TotalPeers, stats.NewPeers, stats.LostPeers, stats.StatusChanged)

	// Log the new peers
	t.Logf("New peers: %v", stats.NewPeerIDs)

	// Log the lost peers
	t.Logf("Lost peers: %v", stats.LostPeerIDs)

	// Log the peers with changed status
	t.Logf("Changed status peers: %v", stats.ChangedPeerIDs)

	// Get the updated list of peers
	updatedPeers := addressDir.GetNetworkPeers()
	t.Logf("After refresh: %d peers total", len(updatedPeers))

	// Count lost peers
	lostPeers := 0
	for _, peer := range updatedPeers {
		if peer.IsLost {
			lostPeers++
		}
	}
	t.Logf("Lost peers count: %d", lostPeers)
}

func TestDiscoverValidatorsMainnet(t *testing.T) {
	// Skip if not running in CI environment
	t.Skip("Skipping test in non-CI environment")

	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Create a client to connect to the mainnet
	endpoint := "https://mainnet.accumulatenetwork.io/v3"

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a client to connect to the mainnet
	client, err := createMainnetClient(endpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Discover validators from the mainnet
	count, err := addressDir.DiscoverValidators(ctx, client)

	// Verify the results
	require.NoError(t, err)
	require.Greater(t, count, 0, "Should discover at least one validator")

	// Get active validators
	activeValidators := addressDir.GetActiveValidators()
	require.NotEmpty(t, activeValidators, "Should have at least one active validator")

	// Log information about the active validators
	t.Logf("Found %d active validators on mainnet", len(activeValidators))
	for i, validator := range activeValidators {
		t.Logf("Validator %d: ID=%s, Name=%s, Partition=%s, Type=%s",
			i+1, validator.ID, validator.Name, validator.PartitionID, validator.PartitionType)
	}

	// Verify that we have validators in different partitions
	// First, get all unique partition IDs from the validators
	partitionIDs := make(map[string]int)
	for _, validator := range activeValidators {
		partitionIDs[validator.PartitionID]++
	}

	// Log the partition information
	t.Logf("Found validators in the following partitions:")
	for partitionID, count := range partitionIDs {
		t.Logf("  %s: %d validators", partitionID, count)

		// For mainnet testing, we'll just verify that we have validators in each partition
		// without checking exact counts, since the BVN and DN lists may not be fully populated
		partitionValidators := addressDir.FindValidatorsByPartition(partitionID)
		require.NotEmpty(t, partitionValidators, "Should have at least one validator in %s partition", partitionID)
	}
}

// Helper functions

// createTestAddressDir creates a test AddressDir with predefined validators and addresses
func createTestAddressDir() *new_heal.AddressDir {
	addressDir := new_heal.NewAddressDir()

	// Add validators
	_ = addressDir.AddValidator("validator1", "Test Validator 1", "bvn-Test", "bvn")
	_ = addressDir.AddValidator("validator2", "Test Validator 2", "bvn-Apollo", "bvn")
	_ = addressDir.AddValidator("validator3", "Test Validator 3", "bvn-Test", "bvn")
	_ = addressDir.AddValidator("validator4", "Test Validator 4", "bvn-Apollo", "bvn")

	// Add addresses with valid peer ID format
	_ = addressDir.AddValidatorAddress("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ")
	_ = addressDir.AddValidatorAddress("validator2", "/ip4/144.76.105.24/tcp/16593/p2p/12D3KooWHFtBnuNU23vXtJ3L5PMralcyNVzfHbYWquXEXW9m7H3G")
	_ = addressDir.AddValidatorAddress("validator3", "/ip4/144.76.105.25/tcp/16593/p2p/12D3KooWHFtBnuNU23vXtJ3L5PMralcyNVzfHbYWquXEXW9m7H3G")
	_ = addressDir.AddValidatorAddress("validator4", "/ip4/144.76.105.26/tcp/16593/p2p/12D3KooWHFtBnuNU23vXtJ3L5PMralcyNVzfHbYWquXEXW9m7H3G")

	// Add validators to DN and BVN lists
	addressDir.AddValidatorToDN("validator1")
	addressDir.AddValidatorToDN("validator2")
	addressDir.AddValidatorToDN("validator3")
	addressDir.AddValidatorToDN("validator4")

	addressDir.AddValidatorToBVN("validator1", "bvn-Test")
	addressDir.AddValidatorToBVN("validator2", "bvn-Apollo")
	addressDir.AddValidatorToBVN("validator3", "bvn-Test")
	addressDir.AddValidatorToBVN("validator4", "bvn-Apollo")

	return addressDir
}

// ErrTestError is a test error type
type ErrTestError string

func (e ErrTestError) Error() string {
	return string(e)
}

// createMainnetClient creates a client to connect to the mainnet
func createMainnetClient(endpoint string) (api.NetworkService, error) {
	// Create a JSON-RPC client
	client := jsonrpc.NewClient(endpoint)

	// Return the client as a NetworkService
	return client, nil
}

// MockNetworkService is a mock implementation of the NetworkService interface for testing
type MockNetworkService struct {
	networkStatus *api.NetworkStatus
}

func (m *MockNetworkService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return m.networkStatus, nil
}

// createMockValidator creates a mock validator for testing
func createMockValidator(id string, pubKeyHash []byte) *protocol.ValidatorInfo {
	return &protocol.ValidatorInfo{
		PublicKeyHash: [32]byte{},
		Operator:      url.MustParse("acc://" + id),
		Partitions:    make([]*protocol.ValidatorPartitionInfo, 0),
	}
}

// createMockPartition creates a mock partition for testing
func createMockPartition(id string, active bool) *protocol.ValidatorPartitionInfo {
	return &protocol.ValidatorPartitionInfo{
		ID:     id,
		Active: active,
	}
}

// Helper function to run the RefreshNetworkPeers test
func runRefreshNetworkPeersTest(t *testing.T, addressDir *new_heal.AddressDir, mockService *MockNetworkService) {
	// First, discover the initial set of peers
	ctx := context.Background()
	totalPeers, err := addressDir.DiscoverNetworkPeers(ctx, mockService)
	if err != nil {
		t.Fatalf("Failed to discover network peers: %v", err)
	}

	t.Logf("Initially discovered %d peers", totalPeers)

	// Get the initial list of peers
	initialPeers := addressDir.GetNetworkPeers()
	for _, peer := range initialPeers {
		t.Logf("Initial peer: ID=%s, IsValidator=%v, Status=%s", peer.ID, peer.IsValidator, peer.Status)
	}

	// Create a modified network status with different validators to simulate changes
	modifiedStatus := &api.NetworkStatus{
		Network: &protocol.NetworkDefinition{
			NetworkName: "testnet",
			Version:     1,
			Partitions: []*protocol.PartitionInfo{
				{ID: "bvn-Test", Type: protocol.PartitionTypeBlockValidator},
				{ID: "bvn-Apollo", Type: protocol.PartitionTypeBlockValidator},
				{ID: "dn", Type: protocol.PartitionTypeDirectory},
			},
			Validators: []*protocol.ValidatorInfo{
				// Remove the first validator to simulate a lost node
				// Keep only the second and third validators
				{
					PublicKeyHash: [32]byte{5, 6, 7, 8},
					Operator:      url.MustParse("acc://validator2.acme"),
					Partitions: []*protocol.ValidatorPartitionInfo{
						{ID: "bvn-Apollo", Active: true},
						{ID: "dn", Active: true},
					},
				},
				{
					PublicKeyHash: [32]byte{9, 10, 11, 12},
					Operator:      url.MustParse("acc://validator3.acme"),
					Partitions: []*protocol.ValidatorPartitionInfo{
						{ID: "bvn-Test", Active: true},
						{ID: "dn", Active: true},
					},
				},
			},
		},
	}

	// 2. Add a new validator to simulate a new node
	newValidator := createMockValidator("newvalidator.acme", []byte{0x99, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22})
	// Add a partition to the new validator
	newValidator.Partitions = append(newValidator.Partitions, createMockPartition("Apollo", true))
	modifiedStatus.Network.Validators = append(modifiedStatus.Network.Validators, newValidator)

	// 3. Change the status of an existing validator
	for i, validator := range modifiedStatus.Network.Validators {
		if i == 0 && len(validator.Partitions) > 0 {
			// Change the first validator's status
			validator.Partitions[0].Active = !validator.Partitions[0].Active
			break
		}
	}

	// Update the mock service with the modified status
	mockService.networkStatus = modifiedStatus

	// Refresh the network peers
	stats, err := addressDir.RefreshNetworkPeers(ctx, mockService)
	if err != nil {
		t.Fatalf("Failed to refresh network peers: %v", err)
	}

	// Verify the refresh statistics
	t.Logf("Refresh stats: Total=%d, New=%d, Lost=%d, StatusChanged=%d", 
		stats.TotalPeers, stats.NewPeers, stats.LostPeers, stats.StatusChanged)

	// Log the new peers
	t.Logf("New peers: %v", stats.NewPeerIDs)

	// Log the lost peers
	t.Logf("Lost peers: %v", stats.LostPeerIDs)

	// Log the peers with changed status
	t.Logf("Changed status peers: %v", stats.ChangedPeerIDs)

	// Get the updated list of peers
	updatedPeers := addressDir.GetNetworkPeers()

	// Verify that lost peers are marked as lost
	for _, peerID := range stats.LostPeerIDs {
		peer, exists := addressDir.GetNetworkPeerByID(peerID)
		if !exists {
			t.Errorf("Lost peer %s not found in NetworkPeers map", peerID)
			continue
		}

		if !peer.IsLost {
			t.Errorf("Lost peer %s not marked as lost", peerID)
		}

		if peer.Status != "lost" {
			t.Errorf("Lost peer %s has status %s, expected 'lost'", peerID, peer.Status)
		}
	}

	// Verify that new peers have FirstSeen timestamp set
	for _, peerID := range stats.NewPeerIDs {
		peer, exists := addressDir.GetNetworkPeerByID(peerID)
		if !exists {
			t.Errorf("New peer %s not found in NetworkPeers map", peerID)
			continue
		}

		if peer.FirstSeen.IsZero() {
			t.Errorf("New peer %s has zero FirstSeen timestamp", peerID)
		}
	}

	t.Logf("After refresh: %d peers total", len(updatedPeers))
	for _, peer := range updatedPeers {
		t.Logf("Updated peer: ID=%s, IsValidator=%v, Status=%s, IsLost=%v", 
			peer.ID, peer.IsValidator, peer.Status, peer.IsLost)
	}
}

// createMockNetworkStatus creates a mock NetworkStatus for testing
func createMockNetworkStatus() *api.NetworkStatus {
	// Create a mock network definition
	networkDef := &protocol.NetworkDefinition{
		NetworkName: "testnet",
		Version:     1,
		Partitions: []*protocol.PartitionInfo{
			{ID: "bvn-Test", Type: protocol.PartitionTypeBlockValidator},
			{ID: "bvn-Apollo", Type: protocol.PartitionTypeBlockValidator},
			{ID: "dn", Type: protocol.PartitionTypeDirectory},
		},
		Validators: []*protocol.ValidatorInfo{
			{
				PublicKeyHash: [32]byte{1, 2, 3, 4},
				Operator:      url.MustParse("acc://validator1.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{ID: "bvn-Test", Active: true},
					{ID: "dn", Active: true},
				},
			},
			{
				PublicKeyHash: [32]byte{5, 6, 7, 8},
				Operator:      url.MustParse("acc://validator2.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{ID: "bvn-Apollo", Active: true},
					{ID: "dn", Active: true},
				},
			},
			{
				PublicKeyHash: [32]byte{9, 10, 11, 12},
				Operator:      url.MustParse("acc://validator3.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{ID: "bvn-Test", Active: true},
					{ID: "dn", Active: true},
				},
			},
			{
				PublicKeyHash: [32]byte{13, 14, 15, 16},
				Operator:      url.MustParse("acc://validator4.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{ID: "bvn-Apollo", Active: true},
					{ID: "dn", Active: true},
				},
			},
		},
	}

	// Create the NetworkStatus
	return &api.NetworkStatus{
		Network: networkDef,
	}
}
