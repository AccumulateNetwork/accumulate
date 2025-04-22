// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"fmt"
	"strings"
)

// CreateTestAddressDir creates a test address directory with sample data
func CreateTestAddressDir() *AddressDir {
	// Create an address directory
	addressDir := NewAddressDir()

	// Add some test validators
	dnValidator := CreateMockValidator("dn-validator", "dn")
	dnValidator.IPAddress = "127.0.0.1"
	dnValidator.P2PAddress = "tcp://127.0.0.1:26656"
	dnValidator.RPCAddress = "http://127.0.0.1:26657"
	dnValidator.APIAddress = "http://127.0.0.1:8080"
	dnValidator.MetricsAddress = "http://127.0.0.1:26660"
	addressDir.AddDNValidator(dnValidator)

	bvnValidator := CreateMockValidator("bvn-validator", "bvn-apollo")
	bvnValidator.IPAddress = "127.0.0.2"
	bvnValidator.P2PAddress = "tcp://127.0.0.2:26656"
	bvnValidator.RPCAddress = "http://127.0.0.2:26657"
	bvnValidator.APIAddress = "http://127.0.0.2:8080"
	bvnValidator.MetricsAddress = "http://127.0.0.2:26660"
	addressDir.AddBVNValidator(0, bvnValidator)

	return addressDir
}

// FormatValidatorInfo formats validator information for testing and debugging
func FormatValidatorInfo(validator *Validator) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Validator: %s (%s)\n", validator.Name, validator.PartitionID))
	sb.WriteString(fmt.Sprintf("  IP Address: %s\n", validator.IPAddress))
	sb.WriteString(fmt.Sprintf("  P2P Address: %s\n", validator.P2PAddress))
	sb.WriteString(fmt.Sprintf("  RPC Address: %s\n", validator.RPCAddress))
	sb.WriteString(fmt.Sprintf("  API Address: %s\n", validator.APIAddress))
	sb.WriteString(fmt.Sprintf("  Metrics Address: %s\n", validator.MetricsAddress))

	return sb.String()
}

// FormatExtendedValidatorInfo formats extended validator information for testing and debugging
func FormatExtendedValidatorInfo(validator *Validator) string {
	var sb strings.Builder

	sb.WriteString(FormatValidatorInfo(validator))
	sb.WriteString(fmt.Sprintf("  Version: %s\n", validator.Version))
	sb.WriteString(fmt.Sprintf("  API v3 Status: %v\n", validator.APIV3Status))
	sb.WriteString(fmt.Sprintf("  DN Height: %d\n", validator.DNHeight))
	sb.WriteString(fmt.Sprintf("  BVN Height: %d\n", validator.BVNHeight))
	sb.WriteString(fmt.Sprintf("  Is Zombie: %v\n", validator.IsZombie))

	return sb.String()
}



// FormatDiscoveryStats formats discovery statistics for testing and debugging
func FormatDiscoveryStats(stats DiscoveryStats) string {
	var sb strings.Builder

	sb.WriteString("Enhanced Discovery Statistics:\n")
	sb.WriteString(fmt.Sprintf("  Total Validators: %d\n", stats.TotalValidators))
	sb.WriteString(fmt.Sprintf("  DN Validators: %d\n", stats.DNValidators))
	
	sb.WriteString("  BVN Validators:\n")
	for bvn, count := range stats.BVNValidators {
		sb.WriteString(fmt.Sprintf("    %s: %d\n", bvn, count))
	}
	
	sb.WriteString("  Address Type Stats:\n")
	for addrType, typeStats := range stats.AddressTypeStats {
		sb.WriteString(fmt.Sprintf("    %s: Total=%d, Valid=%d, Invalid=%d, Success Rate=%.2f%%\n",
			addrType, typeStats.Total, typeStats.Valid, typeStats.Invalid, typeStats.SuccessRate*100))
	}
	
	sb.WriteString(fmt.Sprintf("  API v3 Available: %d\n", stats.APIV3Available))
	sb.WriteString(fmt.Sprintf("  API v3 Unavailable: %d\n", stats.APIV3Unavailable))
	sb.WriteString(fmt.Sprintf("  Zombie Nodes: %d\n", stats.ZombieNodes))
	
	sb.WriteString("  Version Distribution:\n")
	for version, count := range stats.VersionCounts {
		sb.WriteString(fmt.Sprintf("    %s: %d\n", version, count))
	}

	return sb.String()
}

// CreateTestNetworkInfo creates test network information
func CreateTestNetworkInfo(network string) NetworkInfo {
	networkInfo := NetworkInfo{
		Name:        network,
		ID:          "acme",
		APIEndpoint: ResolveWellKnownEndpoint(network, "v3"),
		PartitionMap: make(map[string]*PartitionInfo),
	}

	// Add Directory Network partition
	dnPartition := &PartitionInfo{
		ID:       "dn",
		Type:     "dn",
		URL:      "acc://dn.acme",
		Active:   true,
		BVNIndex: -1,
	}
	networkInfo.Partitions = append(networkInfo.Partitions, dnPartition)
	networkInfo.PartitionMap["dn"] = dnPartition

	// Add BVN partition
	bvnPartition := &PartitionInfo{
		ID:       "bvn-Apollo",
		Type:     "bvn",
		URL:      "acc://bvn-Apollo.acme",
		Active:   true,
		BVNIndex: 0,
	}
	networkInfo.Partitions = append(networkInfo.Partitions, bvnPartition)
	networkInfo.PartitionMap[bvnPartition.ID] = bvnPartition

	// Set mainnet flag
	if network == "mainnet" {
		networkInfo.IsMainnet = true
	}

	return networkInfo
}

// CreateTestNetworkDiscovery creates a test network discovery instance
func CreateTestNetworkDiscovery(network string) *NetworkDiscoveryImpl {
	// Create an address directory
	addressDir := CreateTestAddressDir()

	// Create a network discovery instance
	discovery := &NetworkDiscoveryImpl{
		AddressDir: addressDir,
		Network:    CreateTestNetworkInfo(network),
	}

	// Update validators with enhanced information
	for _, validator := range addressDir.GetDNValidators() {
		validator.Version = "v1.0.0-test"
		validator.DNHeight = 1000
	}

	for _, validator := range addressDir.GetAllBVNValidators() {
		validator.Version = "v1.0.0-test"
		validator.BVNHeight = 900
	}

	return discovery
}
