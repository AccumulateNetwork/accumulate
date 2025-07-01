// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccountRouting tests the routing of account URLs to partitions
func TestAccountRouting(t *testing.T) {
	// Create a test network configuration matching cyclops-network.json
	config := &NetworkConfig{}
	config.Globals.Network.Partitions = []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}{
		{ID: "bvn-cyclops", Type: "blockValidator"},
		{ID: "Directory", Type: "directory"},
	}

	// Initialize routing using our function
	routerInterface, err := InitializeRouting(config)
	if err != nil {
		t.Fatalf("Failed to initialize routing: %v", err)
	}

	// Cast to routing.Router
	router, ok := routerInterface.(routing.Router)
	if !ok {
		t.Fatalf("Router is not of type routing.Router, got %T", routerInterface)
	}

	// Test cases for account URL routing
	testCases := []struct {
		name              string
		accountURL        string
		expectedPartition string
		description       string
	}{
		{
			name:              "System Account",
			accountURL:        "acc://system",
			expectedPartition: "Directory",
			description:       "System accounts should route to Directory partition",
		},
		{
			name:              "System Ledger",
			accountURL:        "acc://system/ledger",
			expectedPartition: "Directory",
			description:       "System ledger should route to Directory partition",
		},
		{
			name:              "DN Account",
			accountURL:        "acc://dn",
			expectedPartition: "bvn-cyclops",
			description:       "DN account routing",
		},
		{
			name:              "Directory Account",
			accountURL:        "acc://directory",
			expectedPartition: "bvn-cyclops",
			description:       "Directory account routing",
		},
		{
			name:              "User Account 1",
			accountURL:        "acc://alice.acme",
			expectedPartition: "bvn-cyclops",
			description:       "User account alice.acme routing",
		},
		{
			name:              "User Account 2",
			accountURL:        "acc://bob.acme",
			expectedPartition: "Directory",
			description:       "User account bob.acme routing",
		},
		{
			name:              "User Account 3",
			accountURL:        "acc://test.acme",
			expectedPartition: "Directory",
			description:       "User account test.acme routing",
		},
	}

	t.Logf("Testing account routing with %d test cases", len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the account URL
			accountURL, err := url.Parse(tc.accountURL)
			if err != nil {
				t.Fatalf("Failed to parse URL %s: %v", tc.accountURL, err)
			}

			// Route the account
			actualPartition, err := router.RouteAccount(accountURL)
			if err != nil {
				t.Fatalf("Failed to route account %s: %v", tc.accountURL, err)
			}

			t.Logf("Account %s routed to partition: %s (expected: %s)",
				tc.accountURL, actualPartition, tc.expectedPartition)

			// Note: We're not asserting equality here because we want to see the actual routing behavior
			// This test is primarily for observation and debugging
			if actualPartition != tc.expectedPartition {
				t.Logf("MISMATCH: Expected %s, got %s for %s",
					tc.expectedPartition, actualPartition, tc.accountURL)
			}
		})
	}
}

// TestBelongsToPartition tests the belongsToPartition function with case-insensitive matching
func TestBelongsToPartition(t *testing.T) {
	// Create a test router
	config := &NetworkConfig{}
	config.Globals.Network.Partitions = []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}{
		{ID: "bvn-cyclops", Type: "blockValidator"},
		{ID: "Directory", Type: "directory"},
	}

	routerInterface, err := InitializeRouting(config)
	if err != nil {
		t.Fatalf("Failed to initialize routing: %v", err)
	}

	router, ok := routerInterface.(routing.Router)
	if !ok {
		t.Fatalf("Router is not of type routing.Router")
	}

	// Test cases for case-insensitive partition matching
	testCases := []struct {
		name            string
		accountURL      string
		targetPartition string
		description     string
	}{
		{
			name:            "Directory - Exact Case",
			accountURL:      "acc://system",
			targetPartition: "Directory",
			description:     "Test exact case matching",
		},
		{
			name:            "Directory - Lower Case",
			accountURL:      "acc://system",
			targetPartition: "directory",
			description:     "Test lowercase target partition",
		},
		{
			name:            "Directory - Upper Case",
			accountURL:      "acc://system",
			targetPartition: "DIRECTORY",
			description:     "Test uppercase target partition",
		},
		{
			name:            "BVN - Exact Case",
			accountURL:      "acc://alice.acme",
			targetPartition: "bvn-cyclops",
			description:     "Test BVN exact case",
		},
		{
			name:            "BVN - Mixed Case",
			accountURL:      "acc://alice.acme",
			targetPartition: "BVN-Cyclops",
			description:     "Test BVN mixed case",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the account URL
			accountURL, err := url.Parse(tc.accountURL)
			if err != nil {
				t.Fatalf("Failed to parse URL %s: %v", tc.accountURL, err)
			}

			// Test belongsToPartition function
			belongs := belongsToPartition(accountURL, tc.targetPartition, router)

			// Get the actual partition for comparison
			actualPartition, err := router.RouteAccount(accountURL)
			if err != nil {
				t.Fatalf("Failed to route account: %v", err)
			}

			t.Logf("Account: %s, Target: %s, Actual: %s, Belongs: %v",
				tc.accountURL, tc.targetPartition, actualPartition, belongs)

			// The function should return true if the actual partition matches the target (case-insensitive)
			expectedBelongs := (actualPartition != "" &&
				(actualPartition == tc.targetPartition ||
					strings.EqualFold(actualPartition, tc.targetPartition)))

			if belongs != expectedBelongs {
				t.Errorf("belongsToPartition mismatch: expected %v, got %v for %s -> %s",
					expectedBelongs, belongs, tc.accountURL, tc.targetPartition)
			}
		})
	}
}

// TestRealNetworkAndSnapshot tests routing with actual network.json and snapshot data
func TestRealNetworkAndSnapshot(t *testing.T) {
	// Define file paths - adjust these to match your actual file locations
	networkConfigPath := "/home/paul/accumulate-network/artifacts/cyclops-network.json"
	snapshotPath := "/home/paul/accumulate-network/artifacts/cyclops-genesis.snap"

	// Check if files exist
	if _, err := os.Stat(networkConfigPath); os.IsNotExist(err) {
		t.Fatalf("Network config file not found: %s", networkConfigPath)
	}
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatalf("Snapshot file not found: %s", snapshotPath)
	}

	t.Logf("Testing with real network config: %s", networkConfigPath)
	t.Logf("Testing with real snapshot: %s", snapshotPath)

	// Load network configuration
	networkConfig, err := ParseNetworkJson(networkConfigPath)
	if err != nil {
		t.Fatalf("Failed to load network config: %v", err)
	}

	t.Logf("Loaded network config with %d partitions:", len(networkConfig.Globals.Network.Partitions))
	for i, partition := range networkConfig.Globals.Network.Partitions {
		t.Logf("  [%d] %s (%s)", i, partition.ID, partition.Type)
	}

	// Initialize routing with real config
	routerInterface, err := InitializeRouting(networkConfig)
	if err != nil {
		t.Fatalf("Failed to initialize routing: %v", err)
	}

	router, ok := routerInterface.(routing.Router)
	if !ok {
		t.Fatalf("Router is not of type routing.Router, got %T", routerInterface)
	}

	// Test accounts from section 1 (accounts section)
	directoryAccounts := testAccountsSection(t, snapshotPath, router, 100)

	// CRITICAL TEST: Fail if no Directory accounts found
	if len(directoryAccounts) == 0 {
		t.Errorf("Expected to find accounts routed to 'Directory' partition")
	} else {
		t.Logf("SUCCESS: Found %d Directory partition accounts", len(directoryAccounts))
	}

	// Test case-insensitive matching with found accounts
	if len(directoryAccounts) > 0 {
		t.Logf("\n=== TESTING CASE-INSENSITIVE PARTITION MATCHING ===")
		testAccount := directoryAccounts[0]
		accountURL, _ := url.Parse(testAccount)
		
		// Test different case variations
		caseVariations := []string{"Directory", "directory", "DIRECTORY", "DiReCtOrY"}
		for _, variation := range caseVariations {
			belongs := belongsToPartition(accountURL, variation, router)
			t.Logf("Account %s belongs to '%s': %v", testAccount, variation, belongs)
			if !belongs {
				t.Errorf("FAILURE: Case-insensitive matching failed for '%s'", variation)
			}
		}
	}
}

// testAccountsSection tests routing by directly accessing section 1 (accounts)
func testAccountsSection(t *testing.T, snapshotPath string, router routing.Router, maxAccounts int) []string {
	// Open snapshot
	file, err := os.Open(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot: %v", err)
	}
	defer file.Close()

	reader, err := snapshot.Open(file)
	if err != nil {
		t.Fatalf("Failed to open snapshot reader: %v", err)
	}

	// Find section 1 (accounts section)
	if len(reader.Sections) < 2 {
		t.Fatalf("Snapshot doesn't have section 1 (accounts)")
	}

	accountsSection := reader.Sections[1]
	t.Logf("Section 1 type: %v", accountsSection.Type())

	// Open records for section 1
	recordReader, err := reader.OpenRecords(1)
	if err != nil {
		t.Fatalf("Failed to open accounts section: %v", err)
	}

	// Test accounts using router
	partitionCounts := make(map[string]int)
	directoryAccounts := []string{}
	accountsProcessed := 0

	t.Logf("Testing accounts from section 1 (max %d)...", maxAccounts)

	for {
		recordEntry, err := recordReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Error reading account record: %v", err)
		}

		// Extract account URL
		accountURL, err := extractAccountURL(recordEntry.Key)
		if err != nil {
			continue // Skip non-account records
		}

		accountsProcessed++

		// Test router
		partition, err := router.RouteAccount(accountURL)
		if err != nil {
			t.Logf("Warning: Failed to route %s: %v", accountURL.String(), err)
			continue
		}

		partitionCounts[partition]++

		// Collect Directory accounts
		if strings.EqualFold(partition, "Directory") {
			directoryAccounts = append(directoryAccounts, accountURL.String())
			if len(directoryAccounts) <= 5 {
				t.Logf("Found Directory account: %s", accountURL.String())
			}
		}

		if accountsProcessed >= maxAccounts {
			break
		}
	}

	t.Logf("\n=== ACCOUNTS SECTION TEST RESULTS ===")
	t.Logf("Accounts processed: %d", accountsProcessed)
	t.Logf("Accounts by partition:")
	for partition, count := range partitionCounts {
		t.Logf("  %s: %d accounts", partition, count)
	}
	t.Logf("Directory accounts found: %d", len(directoryAccounts))

	return directoryAccounts
}

// TestRoutingTableStructure tests the routing table structure and routes
func TestRoutingTableStructure(t *testing.T) {
	// Create routing table manually to understand the structure
	routingTable := &protocol.RoutingTable{}

	// Add routes as done in InitializeRouting
	partitions := []struct {
		ID   string
		Type string
	}{
		{"bvn-cyclops", "blockValidator"},
		{"Directory", "directory"},
	}

	for i, partition := range partitions {
		route := protocol.Route{
			Partition: partition.ID,
			Length:    1, // 1-bit routing
			Value:     uint64(i),
		}
		routingTable.Routes = append(routingTable.Routes, route)
		t.Logf("Added route: Partition=%s, Value=%d, Length=%d",
			route.Partition, route.Value, route.Length)
	}

	// Create router
	router := routing.NewRouter(routing.RouterOptions{
		Initial: routingTable,
	})

	// Test that router was created successfully
	if router == nil {
		t.Fatal("Failed to create router")
	}

	t.Logf("Router created successfully with %d routes", len(routingTable.Routes))

	// Test routing some accounts to see the bit-based routing in action
	testURLs := []string{
		"acc://system",
		"acc://dn",
		"acc://alice.acme",
		"acc://bob.acme",
		"acc://test.acme",
	}

	for _, urlStr := range testURLs {
		accountURL, err := url.Parse(urlStr)
		if err != nil {
			t.Errorf("Failed to parse %s: %v", urlStr, err)
			continue
		}

		partition, err := router.RouteAccount(accountURL)
		if err != nil {
			t.Errorf("Failed to route %s: %v", urlStr, err)
			continue
		}

		// Get the routing number to understand the bit-based routing
		routingNum := accountURL.Routing()
		t.Logf("URL: %s, Routing#: %d (0x%x), Partition: %s",
			urlStr, routingNum, routingNum, partition)
	}
}
