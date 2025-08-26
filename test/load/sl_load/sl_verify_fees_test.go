//go:build !testnet
// +build !testnet

package load_test

import (
	"testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestVerifyFeeCalculations verifies that our fee calculations are correct
func TestVerifyFeeCalculations(t *testing.T) {
	t.Run("BasicCalculations", func(t *testing.T) {
		fees := GetDefaultFeeSchedule()
		
		// Test 1: 100 transactions should need 300 credits (3 per tx)
		numTxs := 100
		creditsNeeded := CalculateCreditsForTransactions(numTxs, fees)
		expectedCredits := int64(100 * 300) // 30000 internal units = 300 credits
		
		if creditsNeeded != expectedCredits {
			t.Errorf("Expected %d internal units (%.2f credits), got %d (%.2f credits)",
				expectedCredits, float64(expectedCredits)/protocol.CreditPrecision,
				creditsNeeded, float64(creditsNeeded)/protocol.CreditPrecision)
		}
		
		t.Logf("✓ %d transactions need %s credits", numTxs, FormatCredits(creditsNeeded))
	})
	
	t.Run("ACMEToCreditsConversion", func(t *testing.T) {
		testCases := []struct {
			acme        int64  // ACME in internal units
			oraclePrice uint64 // Oracle price
			expected    uint64 // Expected credits in internal units
			description string
		}{
			{
				acme:        int64(0.01 * 1e8), // 0.01 ACME
				oraclePrice: 5000,               // $0.50/ACME
				expected:    500000,             // 5000.00 credits (formula output)
				description: "0.01 ACME at $0.50/ACME = 5000 credits",
			},
			{
				acme:        int64(0.01 * 1e8), // 0.01 ACME
				oraclePrice: 10000,              // $1.00/ACME
				expected:    1000000,            // 10000.00 credits (formula output)
				description: "0.01 ACME at $1.00/ACME = 10000 credits",
			},
			{
				acme:        int64(0.01 * 1e8),  // 0.01 ACME
				oraclePrice: 10000000,           // $1000.00/ACME
				expected:    1000000000,         // 10000000.00 credits (formula output)
				description: "0.01 ACME at $1000/ACME = 10,000,000 credits",
			},
		}
		
		for _, tc := range testCases {
			credits := ACMEToCredits(tc.acme, tc.oraclePrice)
			if credits != tc.expected {
				t.Errorf("%s: Expected %d (%.2f credits), got %d (%.2f credits)",
					tc.description,
					tc.expected, float64(tc.expected)/protocol.CreditPrecision,
					credits, float64(credits)/protocol.CreditPrecision)
			}
			t.Logf("✓ %s", tc.description)
		}
	})
	
	t.Run("LoadTestFundingCalculation", func(t *testing.T) {
		config := LoadConfig{
			NumSenders:   5,
			NumReceivers: 5,
			NumTxs:       100,
			TxAmount:     int64(0.001 * 1e8),
		}
		
		fees := GetDefaultFeeSchedule()
		oraclePrice := uint64(5000) // $0.50/ACME
		
		funding := CalculateFunding(config, fees, oraclePrice)
		
		t.Logf("Funding calculation for %d txs with %d senders:", config.NumTxs, config.NumSenders)
		t.Logf("  Transactions per sender: %d", funding.TransactionsPerK)
		t.Logf("  Credits per sender: %s", FormatCredits(funding.CreditsPerK))
		t.Logf("  ACME for transactions: %s", FormatACME(funding.ACMEPerK))
		t.Logf("  ACME for credits: %s", FormatACME(funding.ACMEForCreditsPerK))
		t.Logf("  Total ACME per sender: %s", FormatACME(funding.TotalACMEPerK))
		t.Logf("  Total ACME needed: %s", FormatACME(funding.TotalACMENeeded))
		t.Logf("  Faucet calls: %d", funding.FaucetCallsNeeded)
		
		// Verify calculations
		// 20 txs per sender * 3 credits = 60 credits
		expectedCreditsPerSender := int64(20 * 300) // 6000 internal units
		if funding.CreditsPerK != expectedCreditsPerSender {
			t.Errorf("Credits per sender: expected %s, got %s",
				FormatCredits(expectedCreditsPerSender),
				FormatCredits(funding.CreditsPerK))
		}
		
		// 20 txs * 0.001 ACME = 0.02 ACME
		expectedACMEPerK := int64(20 * 0.001 * 1e8)
		if funding.ACMEPerK != expectedACMEPerK {
			t.Errorf("ACME per sender: expected %s, got %s",
				FormatACME(expectedACMEPerK),
				FormatACME(funding.ACMEPerK))
		}
	})
}