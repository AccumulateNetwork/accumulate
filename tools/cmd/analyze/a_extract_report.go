// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package main implements snapshot extraction tools.
// IMPORTANT: This implementation uses a streaming architecture to process snapshots
// efficiently without loading the entire database into memory. The goal is to process
// a 2GB snapshot using less than 5GB of memory (compared to previous implementations
// that required 40GB for a 2GB snapshot).
package main

import (
	"fmt"
)

// ExtractReport holds statistics and data collected during snapshot extraction
type ExtractReport struct {
	// Account statistics
	TotalAccounts        int64
	AccountWithMainChain int64
	AccountNoMainChain   int64
	TotalEntries         int64
	TotalEntriesMissing  int64
	TotalFoundEntries    int64

	// Collection counts
	AccountCount     int64
	ChainCount       int64
	TransactionCount int64
	MessageCount     int64

	// Partition statistics
	PartitionCounts map[string]int64

	// Merkle tree analysis
	TotalChainEntries    int64
	TotalSnapshotEntries int64
	ChainsWithMerkleData int64
	ChainsWithoutMerkleData int64

	// Chain type statistics
	MainChainCount   int64
	AnchorChainCount int64
	OtherChainCount  int64

	// DN partition statistics
	DNStats *DNAccountStats
}

// NewExtractReport creates a new extraction report with initialized values
func NewExtractReport() *ExtractReport {
	return &ExtractReport{
		PartitionCounts: make(map[string]int64),
	}
}

// PrintReport prints the report to stdout
func (r *ExtractReport) PrintReport() {
	fmt.Println("\nSnapshot Processing Summary:")
	fmt.Printf("  Total accounts processed: %d\n", r.AccountCount)
	fmt.Printf("  Total chain sub-records found: %d\n", r.ChainCount)
	fmt.Printf("  Total transactions collected: %d\n", r.TransactionCount)
	fmt.Printf("  Total messages collected: %d\n", r.MessageCount)

	fmt.Println("\nChain Type Statistics:")
	fmt.Printf("  Main chains: %d\n", r.MainChainCount)
	fmt.Printf("  Anchor chains: %d\n", r.AnchorChainCount)
	fmt.Printf("  Other chains: %d\n", r.OtherChainCount)

	fmt.Println("\nMerkle Tree Analysis Results:")
	fmt.Printf("  Total accounts processed: %d\n", r.TotalAccounts)
	fmt.Printf("  Accounts with main chain: %d\n", r.AccountWithMainChain)
	fmt.Printf("  Accounts without main chain: %d\n", r.AccountNoMainChain)
	fmt.Printf("  Chains with Merkle data: %d\n", r.ChainsWithMerkleData)
	fmt.Printf("  Chains without Merkle data: %d\n", r.ChainsWithoutMerkleData)
	fmt.Printf("  Total chain entries: %d\n", r.TotalChainEntries)
	fmt.Printf("  Total snapshot entries: %d\n", r.TotalSnapshotEntries)
	fmt.Printf("  Total expected entries: %d\n", r.TotalEntries)
	fmt.Printf("  Total found entries: %d\n", r.TotalFoundEntries)

	// Print partition statistics if available
	if len(r.PartitionCounts) > 0 {
		fmt.Println("\nPartition Statistics:")
		for partition, count := range r.PartitionCounts {
			fmt.Printf("  %s: %d accounts\n", partition, count)
		}
	}

	// Print DN partition statistics if available
	if r.DNStats != nil {
		r.DNStats.PrintDNStats()
	}
}
