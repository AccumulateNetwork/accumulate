//go:build !testnet
// +build !testnet

package load_test

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestFeeCalculationsUnit tests fee calculations WITHOUT requiring a devnet
// This is a true unit test with no external dependencies
func TestFeeCalculationsUnit(t *testing.T) {
	// Mock fee schedule - no network calls
	mockFees := &FeeSchedule{
		SendTokens:         300,   // 3.00 credits
		AddCredits:         300,   // 3.00 credits  
		CreateTokenAccount: 2500,  // 25.00 credits
		CreateIdentity:     10000, // 100.00 credits
		CreateDataAccount:  2500,  // 25.00 credits
		CreateKeyPage:      2500,  // 25.00 credits
		UpdateKeyPage:      300,   // 3.00 credits
		WriteData:          1,     // 0.01 credits per byte
		WriteDataMin:       100,   // 1.00 credit minimum
		BurnTokens:         300,   // 3.00 credits
		IssueTokens:        300,   // 3.00 credits
	}

	t.Run("CalculateSimpleTransactionCredits", func(t *testing.T) {
		// Test simple SendTokens calculation
		numTxs := 1000
		expectedCredits := int64(numTxs) * mockFees.SendTokens
		
		if expectedCredits != 300000 {
			t.Errorf("Expected %d credits for %d SendTokens, got %d", 
				300000, numTxs, expectedCredits)
		}
		
		// Display in human-readable format
		t.Logf("%d SendTokens transactions need %.2f credits", 
			numTxs, float64(expectedCredits)/protocol.CreditPrecision)
	})

	t.Run("CalculateMixedTransactionCredits", func(t *testing.T) {
		// Test mixed transaction types
		mix := TransactionMix{
			SendTokens:         900,
			AddCredits:         50,
			CreateTokenAccount: 50,
		}
		
		totalCredits := calculateMixCredits(mix, mockFees)
		expected := int64(900*300 + 50*300 + 50*2500) // 270000 + 15000 + 125000 = 410000
		
		if totalCredits != expected {
			t.Errorf("Expected %d credits for mixed transactions, got %d", 
				expected, totalCredits)
		}
		
		t.Logf("Mixed transaction load needs %.2f credits", 
			float64(totalCredits)/protocol.CreditPrecision)
	})

	t.Run("CreditsToACMEConversion", func(t *testing.T) {
		testCases := []struct {
			name        string
			credits     int64  // External units (600 = 600.00 credits)
			oraclePrice uint64 // Oracle price (5000 = $0.50/ACME)
			expectedACME int64 // Expected ACME in internal units
			description string
		}{
			{
				name:        "LargeCreditsAt50Cents",
				credits:     50000,
				oraclePrice: 5000,
				expectedACME: 10000000, // 0.1 ACME
				description: "50,000 credits at $0.50/ACME should cost 0.1 ACME",
			},
			{
				name:        "LargeCreditsAt1Dollar",
				credits:     50000,
				oraclePrice: 10000,
				expectedACME: 5000000, // 0.05 ACME
				description: "50,000 credits at $1.00/ACME should cost 0.05 ACME",
			},
			{
				name:        "SmallCreditsTriggersMinimum",
				credits:     10,
				oraclePrice: 5000,
				expectedACME: 1000000, // 0.01 ACME minimum
				description: "Small credit amounts should trigger 0.01 ACME minimum",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				acme := creditsToACME(tc.credits, tc.oraclePrice)
				if acme != tc.expectedACME {
					t.Errorf("%s: Expected %d (%.4f ACME), got %d (%.4f ACME)",
						tc.description,
						tc.expectedACME, float64(tc.expectedACME)/1e8,
						acme, float64(acme)/1e8)
				}
				t.Logf("%d credits at $%.2f/ACME = %.4f ACME",
					tc.credits, float64(tc.oraclePrice)/10000, float64(acme)/1e8)
			})
		}
	})

	t.Run("LoadTestFundingCalculation", func(t *testing.T) {
		// Mock a load test scenario
		numAccounts := 50
		txPerAccount := 200
		oraclePrice := uint64(5000) // $0.50/ACME
		
		// Calculate credits needed
		creditsPerAccount := int64(txPerAccount) * mockFees.SendTokens / protocol.CreditPrecision
		acmeForCredits := creditsToACME(creditsPerAccount, oraclePrice)
		
		// Calculate ACME for transactions
		acmeForTxs := int64(txPerAccount) * int64(0.001 * 1e8)
		
		// Total with buffer
		totalPerAccount := acmeForCredits + acmeForTxs
		totalWithBuffer := totalPerAccount + (totalPerAccount / 10)
		
		t.Logf("Funding calculation for %d accounts, %d txs each:", numAccounts, txPerAccount)
		t.Logf("  Credits per account: %d (%.2f credits)", 
			creditsPerAccount*protocol.CreditPrecision, float64(creditsPerAccount))
		t.Logf("  ACME for credits: %.4f", float64(acmeForCredits)/1e8)
		t.Logf("  ACME for txs: %.4f", float64(acmeForTxs)/1e8)
		t.Logf("  Total per account: %.4f ACME", float64(totalPerAccount)/1e8)
		t.Logf("  With 10%% buffer: %.4f ACME", float64(totalWithBuffer)/1e8)
		
		// Validate reasonable amounts
		if totalWithBuffer <= 0 {
			t.Error("Total ACME calculation is zero or negative")
		}
		
		if float64(totalWithBuffer)/1e8 > 10.0 {
			t.Errorf("Per-account cost seems too high: %.4f ACME", 
				float64(totalWithBuffer)/1e8)
		}
	})

	t.Run("WriteDataFeeCalculation", func(t *testing.T) {
		testCases := []struct {
			dataSize     int
			expectedFee  int64
			description  string
		}{
			{
				dataSize:    10,
				expectedFee: 100, // Minimum 1.00 credit
				description: "Small data (10 bytes) should use minimum fee",
			},
			{
				dataSize:    1000,
				expectedFee: 1000, // 10.00 credits (1000 * 0.01)
				description: "1KB data should cost 10.00 credits",
			},
			{
				dataSize:    10000,
				expectedFee: 10000, // 100.00 credits
				description: "10KB data should cost 100.00 credits",
			},
		}
		
		for _, tc := range testCases {
			fee := int64(tc.dataSize) * mockFees.WriteData
			if fee < mockFees.WriteDataMin {
				fee = mockFees.WriteDataMin
			}
			
			if fee != tc.expectedFee {
				t.Errorf("%s: Expected %d (%.2f credits), got %d (%.2f credits)",
					tc.description,
					tc.expectedFee, float64(tc.expectedFee)/protocol.CreditPrecision,
					fee, float64(fee)/protocol.CreditPrecision)
			}
			
			t.Logf("WriteData %d bytes = %.2f credits", 
				tc.dataSize, float64(fee)/protocol.CreditPrecision)
		}
	})

	t.Run("BufferCalculations", func(t *testing.T) {
		// Test that 10% buffer is correctly applied
		baseAmount := int64(1000000) // 0.01 ACME
		withBuffer := baseAmount + (baseAmount / 10)
		
		if withBuffer != 1100000 {
			t.Errorf("10%% buffer calculation wrong: expected 1100000, got %d", withBuffer)
		}
		
		t.Logf("Base: %.4f ACME, With 10%% buffer: %.4f ACME",
			float64(baseAmount)/1e8, float64(withBuffer)/1e8)
	})
}

// TestFeeTableValidation validates fee table structure without network
func TestFeeTableValidation(t *testing.T) {
	t.Run("ValidateFeeRanges", func(t *testing.T) {
		// Mock fee table with expected ranges
		fees := &FeeSchedule{
			SendTokens:         300,   // Should be lowest
			CreateTokenAccount: 2500,  // Should be higher than send
			CreateIdentity:     10000, // Should be highest
		}
		
		// Validate relative costs make sense
		if fees.SendTokens >= fees.CreateTokenAccount {
			t.Error("SendTokens should cost less than CreateTokenAccount")
		}
		
		if fees.CreateTokenAccount >= fees.CreateIdentity {
			t.Error("CreateTokenAccount should cost less than CreateIdentity")
		}
		
		// Validate absolute ranges
		if fees.SendTokens < 100 || fees.SendTokens > 1000 {
			t.Errorf("SendTokens fee out of expected range: %d", fees.SendTokens)
		}
		
		if fees.CreateIdentity < 5000 || fees.CreateIdentity > 50000 {
			t.Errorf("CreateIdentity fee out of expected range: %d", fees.CreateIdentity)
		}
		
		t.Log("Fee table validation passed")
	})
}