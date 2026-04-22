// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sdktest "gitlab.com/accumulatenetwork/accumulate/test/sdk"
)

// TestGenerateTestData validates that gen-testdata can generate valid test data files
func TestGenerateTestData(t *testing.T) {
	// Create a temporary directory for output
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "test-data.json")
	ledgerFile := filepath.Join(tmpDir, "ledger-vectors.json")

	// Create test suite
	ts := &sdktest.TestSuite{
		Transactions: transactionTests(txnTest),
		Accounts:     accountTests(txnTest),
	}

	// Store the test data
	err := ts.Store(outputFile)
	if err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatal("Output file was not created")
	}

	// Verify the file is valid JSON
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	var loaded sdktest.TestSuite
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("Output file is not valid JSON: %v", err)
	}

	// Verify we have transaction test groups
	if len(loaded.Transactions) == 0 {
		t.Error("No transaction test groups were generated")
	}

	// Verify we have account test groups
	if len(loaded.Accounts) == 0 {
		t.Error("No account test groups were generated")
	}

	// Verify transaction test groups have expected names
	expectedTxTypes := []string{
		"CreateIdentity",
		"CreateTokenAccount",
		"SendTokens",
		"CreateDataAccount",
		"WriteData",
		"AcmeFaucet",
		"CreateToken",
	}

	txTypeMap := make(map[string]bool)
	for _, group := range loaded.Transactions {
		txTypeMap[group.Name] = true
		if len(group.Cases) == 0 {
			t.Errorf("Transaction group %s has no test cases", group.Name)
		}
	}

	for _, expected := range expectedTxTypes {
		if !txTypeMap[expected] {
			t.Errorf("Expected transaction type %s not found in generated data", expected)
		}
	}

	// Verify account test groups have expected names
	expectedAcctTypes := []string{
		"Identity",
		"TokenIssuer",
		"TokenAccount",
		"LiteTokenAccount",
		"KeyPage",
		"KeyBook",
		"DataAccount",
	}

	acctTypeMap := make(map[string]bool)
	for _, group := range loaded.Accounts {
		acctTypeMap[group.Name] = true
		if len(group.Cases) == 0 {
			t.Errorf("Account group %s has no test cases", group.Name)
		}
	}

	for _, expected := range expectedAcctTypes {
		if !acctTypeMap[expected] {
			t.Errorf("Expected account type %s not found in generated data", expected)
		}
	}

	// Generate ledger test vectors
	lts := &sdktest.TestSuite{
		Transactions: transactionTests(txnUnsignedEnvelopeTestVectors),
		Accounts:     accountTests(txnUnsignedEnvelopeTestVectors),
	}

	err = lts.Store(ledgerFile)
	if err != nil {
		t.Fatalf("Failed to generate ledger test vectors: %v", err)
	}

	// Verify the ledger file exists
	if _, err := os.Stat(ledgerFile); os.IsNotExist(err) {
		t.Fatal("Ledger test vectors file was not created")
	}

	// Verify the ledger file is valid JSON
	ledgerData, err := os.ReadFile(ledgerFile)
	if err != nil {
		t.Fatalf("Failed to read ledger file: %v", err)
	}

	var loadedLedger sdktest.TestSuite
	err = json.Unmarshal(ledgerData, &loadedLedger)
	if err != nil {
		t.Fatalf("Ledger file is not valid JSON: %v", err)
	}
}

// TestTransactionTestCases verifies that all transaction test cases are properly formed
func TestTransactionTestCases(t *testing.T) {
	groups := transactionTests(txnTest)

	for _, group := range groups {
		t.Run(group.Name, func(t *testing.T) {
			if len(group.Cases) == 0 {
				t.Error("Group has no test cases")
			}

			for i, tc := range group.Cases {
				if tc == nil {
					t.Errorf("Test case %d is nil", i)
					continue
				}

				if len(tc.Binary) == 0 {
					t.Errorf("Test case %d has no binary data", i)
				}

				if len(tc.JSON) == 0 {
					t.Errorf("Test case %d has no JSON data", i)
				}
			}
		})
	}
}

// TestAccountTestCases verifies that all account test cases are properly formed
func TestAccountTestCases(t *testing.T) {
	groups := accountTests(txnTest)

	for _, group := range groups {
		t.Run(group.Name, func(t *testing.T) {
			if len(group.Cases) == 0 {
				t.Error("Group has no test cases")
			}

			for i, tc := range group.Cases {
				if tc == nil {
					t.Errorf("Test case %d is nil", i)
					continue
				}

				if len(tc.Binary) == 0 {
					t.Errorf("Test case %d has no binary data", i)
				}

				if len(tc.JSON) == 0 {
					t.Errorf("Test case %d has no JSON data", i)
				}
			}
		})
	}
}

// TestSimpleHashMode verifies that the simple hash flag works
func TestSimpleHashMode(t *testing.T) {
	// Test with simple hash enabled (default)
	*useSimpleHash = true
	groups := transactionTests(txnTest)
	if len(groups) == 0 {
		t.Fatal("No transaction groups generated with simple hash")
	}

	// Test with simple hash disabled
	*useSimpleHash = false
	groups = transactionTests(txnTest)
	if len(groups) == 0 {
		t.Fatal("No transaction groups generated without simple hash")
	}

	// Reset to default
	*useSimpleHash = true
}
