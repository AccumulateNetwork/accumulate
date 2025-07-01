// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
)

// ExtractReport holds statistics and data collected during snapshot extraction
type ExtractReport struct {
	// Account statistics
	TotalAccountsProcessed   int64
	AccountsWithMainChain    int64
	AccountsWithoutMainChain int64
	TotalExpectedEntries     int64
	TotalFoundEntries        int64
	TotalChainsExamined      int64
	
	// Additional statistics
	AccountCount     int64
	ChainCount       int64
	TransactionCount int64
	MessageCount     int64
}

// NewExtractReport creates a new extraction report with initialized values
func NewExtractReport() *ExtractReport {
	return &ExtractReport{}
}

// RecordAccountWithMainChain records an account with a main chain
func (r *ExtractReport) RecordAccountWithMainChain() {
	r.AccountsWithMainChain++
}

// RecordAccountWithoutMainChain records an account without a main chain
func (r *ExtractReport) RecordAccountWithoutMainChain() {
	r.AccountsWithoutMainChain++
}

// IncrementAccountCount increments the account count
func (r *ExtractReport) IncrementAccountCount() {
	r.AccountCount++
}

// IncrementChainCount increments the chain count
func (r *ExtractReport) IncrementChainCount() {
	r.ChainCount++
}

// IncrementTransactionCount increments the transaction count
func (r *ExtractReport) IncrementTransactionCount() {
	r.TransactionCount++
}

// IncrementMessageCount increments the message count
func (r *ExtractReport) IncrementMessageCount() {
	r.MessageCount++
}

// PrintReport prints the report to stdout
func (r *ExtractReport) PrintReport() {
	fmt.Println("\nSnapshot Processing Summary:")
	fmt.Printf("  Total accounts processed: %d\n", r.AccountCount)
	fmt.Printf("  Total chain sub-records found: %d\n", r.ChainCount)
	fmt.Printf("  Total transactions collected: %d\n", r.TransactionCount)
	fmt.Printf("  Total messages collected: %d\n", r.MessageCount)

	fmt.Println("\nMerkle Tree Analysis Results:")
	fmt.Printf("  Total accounts processed: %d\n", r.TotalAccountsProcessed)
	fmt.Printf("  Accounts with main chain: %d\n", r.AccountsWithMainChain)
	fmt.Printf("  Accounts without main chain: %d\n", r.AccountsWithoutMainChain)
	fmt.Printf("  Total chains examined: %d\n", r.TotalChainsExamined)
	fmt.Printf("  Total expected entries: %d\n", r.TotalExpectedEntries)
	fmt.Printf("  Total found entries: %d\n", r.TotalFoundEntries)
}
