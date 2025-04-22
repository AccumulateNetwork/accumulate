// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateAddressDirReport(t *testing.T) {
	// Create a test address directory with sample data
	addressDir := createTestAddressDir()

	// Generate the report to a buffer
	var buf bytes.Buffer
	GenerateAddressDirReport(addressDir, &buf)

	// Print the report for debugging
	report := buf.String()
	fmt.Println("Report Content:")
	fmt.Println(report)

	// Verify the report contains expected sections
	assert.Contains(t, report, "=== ACCUMULATE NETWORK ADDRESS DIRECTORY REPORT ===", "Missing report header")
	assert.Contains(t, report, "## Network Information", "Missing network information section")
	assert.Contains(t, report, "## Directory Network Validators", "Missing DN validators section")
	assert.Contains(t, report, "## BVN Validators", "Missing BVN validators section")
	assert.Contains(t, report, "## Network Peers", "Missing network peers section")
	assert.Contains(t, report, "## Discovery Statistics", "Missing discovery statistics section")
	assert.Contains(t, report, "=== END OF REPORT ===", "Missing report footer")

	// Verify specific content
	assert.Contains(t, report, "Apollo", "Missing Apollo partition")
	assert.Contains(t, report, "Chandrayaan", "Missing Chandrayaan partition")
	assert.Contains(t, report, "Yutu", "Missing Yutu partition")
	assert.Contains(t, report, "DN Validator 1", "Missing DN validator")
	assert.Contains(t, report, "public-api-1", "Missing network peer")
}

// createTestAddressDir creates a test address directory with sample data
func createTestAddressDir() *AddressDir {
	addressDir := NewTestAddressDir()

	// Set up network info
	addressDir.NetworkInfo.Name = "mainnet"
	addressDir.NetworkInfo.IsMainnet = true

	// Add partitions
	partitions := []*PartitionInfo{
		{ID: "dn", Type: "dn", URL: "acc://dn.acme", Active: true},
		{ID: "Apollo", Type: "bvn", URL: "acc://bvn-Apollo.acme", Active: true, BVNIndex: 0},
		{ID: "Chandrayaan", Type: "bvn", URL: "acc://bvn-Chandrayaan.acme", Active: true, BVNIndex: 1},
		{ID: "Yutu", Type: "bvn", URL: "acc://bvn-Yutu.acme", Active: true, BVNIndex: 2},
	}

	for _, p := range partitions {
		addressDir.NetworkInfo.Partitions = append(addressDir.NetworkInfo.Partitions, p)
		addressDir.NetworkInfo.PartitionMap[p.ID] = p
	}

	// Initialize BVNValidators array
	addressDir.BVNValidators = make([][]Validator, 3)
	for i := 0; i < 3; i++ {
		addressDir.BVNValidators[i] = make([]Validator, 0)
	}

	// Add DN validators
	for i := 1; i <= 3; i++ {
		v := TestValidator(
			"validator-dn-"+fmt.Sprintf("%d", i),
			"DN Validator "+fmt.Sprintf("%d", i),
			"dn",
			"dn",
		)
		v.P2PAddress = "/ip4/10.0.0." + fmt.Sprintf("%d", i) + "/tcp/26656/p2p/12D3KooWDN" + fmt.Sprintf("%d", i)
		v.RPCAddress = "http://10.0.0." + fmt.Sprintf("%d", i) + ":26657"
		v.APIAddress = "http://10.0.0." + fmt.Sprintf("%d", i) + ":8080"
		
		// Add addresses
		v.Addresses = append(v.Addresses, ValidatorAddress{
			Address:   v.P2PAddress,
			Validated: true,
			IP:        "10.0.0." + fmt.Sprintf("%d", i),
			Port:      "26656",
			PeerID:    "12D3KooWDN" + fmt.Sprintf("%d", i),
			LastSuccess: time.Now().Add(-time.Hour),
		})
		
		addressDir.DNValidators = append(addressDir.DNValidators, v)
	}

	// Add BVN validators
	bvnNames := []string{"Apollo", "Chandrayaan", "Yutu"}
	for bvnIdx, bvnName := range bvnNames {
		for i := 1; i <= 2; i++ {
			v := TestValidator(
				"validator-"+bvnName+"-"+fmt.Sprintf("%d", i),
				bvnName+" Validator "+fmt.Sprintf("%d", i),
				bvnName,
				"bvn",
			)
			v.P2PAddress = "/ip4/20." + fmt.Sprintf("%d", bvnIdx) + ".0." + fmt.Sprintf("%d", i) + "/tcp/26656/p2p/12D3KooWBVN" + fmt.Sprintf("%d", bvnIdx) + fmt.Sprintf("%d", i)
			v.RPCAddress = "http://20." + fmt.Sprintf("%d", bvnIdx) + ".0." + fmt.Sprintf("%d", i) + ":26657"
			v.APIAddress = "http://20." + fmt.Sprintf("%d", bvnIdx) + ".0." + fmt.Sprintf("%d", i) + ":8080"
			
			// Add addresses
			v.Addresses = append(v.Addresses, ValidatorAddress{
				Address:   v.P2PAddress,
				Validated: true,
				IP:        "20." + fmt.Sprintf("%d", bvnIdx) + ".0." + fmt.Sprintf("%d", i),
				Port:      "26656",
				PeerID:    "12D3KooWBVN" + fmt.Sprintf("%d", bvnIdx) + fmt.Sprintf("%d", i),
				LastSuccess: time.Now().Add(-time.Hour),
			})
			
			addressDir.BVNValidators[bvnIdx] = append(addressDir.BVNValidators[bvnIdx], v)
		}
	}

	// Add network peers
	peerIDs := []string{"public-api-1", "public-api-2"}
	for i, peerID := range peerIDs {
		peer := TestNetworkPeer(peerID, false, "", "dn")
		peer.Addresses[0].Address = "/ip4/65.108.73." + fmt.Sprintf("%d", 1+i) + "/tcp/16593/p2p/" + peerID
		peer.Addresses[0].IP = "65.108.73." + fmt.Sprintf("%d", 1+i)
		addressDir.NetworkPeers[peerID] = peer
	}

	// Add discovery stats
	stats := addressDir.DiscoveryStats
	stats.LastDiscovery = time.Now().Add(-time.Hour)
	stats.DiscoveryAttempts = 10
	stats.SuccessfulDiscoveries = 9
	stats.FailedDiscoveries = 1
	stats.TotalDiscoveryTime = 15 * time.Second
	stats.ValidatorsDiscovered = 9
	stats.PeersDiscovered = 2
	stats.MultiaddrSuccess = 15
	stats.URLSuccess = 5
	stats.ValidatorMapSuccess = 3
	stats.Failures = 2
	
	// Add method stats
	stats.MethodStats["directory"] = 3
	stats.MethodStats["partition"] = 7
	stats.ByMethod["directory"] = 3
	stats.ByMethod["partition"] = 7
	stats.ByPartition["dn"] = 3
	stats.ByPartition["Apollo"] = 2
	stats.ByPartition["Chandrayaan"] = 2
	stats.ByPartition["Yutu"] = 2

	return addressDir
}

// TestGenerateAddressDirReportEmpty tests the report generation with an empty address directory
func TestGenerateAddressDirReportEmpty(t *testing.T) {
	// Create an empty address directory
	addressDir := NewAddressDir()

	// Generate the report to a buffer
	var buf bytes.Buffer
	GenerateAddressDirReport(addressDir, &buf)

	// Verify the report contains expected sections
	report := buf.String()
	assert.Contains(t, report, "=== ACCUMULATE NETWORK ADDRESS DIRECTORY REPORT ===")
	assert.Contains(t, report, "## Network Information")
	assert.Contains(t, report, "## Directory Network Validators")
	assert.Contains(t, report, "No Directory Network validators found.")
	assert.Contains(t, report, "## BVN Validators")
	assert.Contains(t, report, "No BVN validators found.")
	assert.Contains(t, report, "## Network Peers")
	assert.Contains(t, report, "No network peers found.")
	assert.Contains(t, report, "## Discovery Statistics")
	assert.Contains(t, report, "=== END OF REPORT ===")
}

// TestGenerateAddressDirReportNilWriter tests the report generation with a nil writer
func TestGenerateAddressDirReportNilWriter(t *testing.T) {
	// This test just verifies that the function doesn't panic with a nil writer
	addressDir := NewAddressDir()
	assert.NotPanics(t, func() {
		GenerateAddressDirReport(addressDir, nil)
	})
}
