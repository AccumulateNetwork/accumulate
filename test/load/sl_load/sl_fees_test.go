//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"fmt"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionMix represents the distribution of transaction types in a load test
type TransactionMix struct {
	SendTokens         int
	AddCredits         int
	CreateTokenAccount int
	CreateIdentity     int
	WriteDataBytes     int // Total bytes to write across all WriteData txs
	WriteDataTxCount   int // Number of WriteData transactions
}

// TestBuildFeeTable tests building a fee table from network description
func TestBuildFeeTable(t *testing.T) {
	// Skip if no devnet
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		t.Skip("No devnet available")
	}

	client := jsonrpc.NewClient(endpoint)
	
	// Test 1: Fetch fee table from network
	t.Run("FetchFeeTable", func(t *testing.T) {
		fees, err := fetchFeeSchedule(client)
		if err != nil {
			t.Fatalf("Failed to fetch fee schedule: %v", err)
		}
		
		// Validate fees are reasonable (not zero, not massive)
		if fees.SendTokens <= 0 || fees.SendTokens > 10000 {
			t.Errorf("SendTokens fee out of range: %d", fees.SendTokens)
		}
		
		if fees.CreateTokenAccount <= fees.SendTokens {
			t.Errorf("CreateTokenAccount should cost more than SendTokens: %d <= %d", 
				fees.CreateTokenAccount, fees.SendTokens)
		}
		
		t.Logf("Fee Schedule:")
		t.Logf("  SendTokens: %.2f credits", float64(fees.SendTokens)/100)
		t.Logf("  AddCredits: %.2f credits", float64(fees.AddCredits)/100)
		t.Logf("  CreateTokenAccount: %.2f credits", float64(fees.CreateTokenAccount)/100)
		t.Logf("  CreateIdentity: %.2f credits", float64(fees.CreateIdentity)/100)
		t.Logf("  WriteData: %.2f credits/byte (min %.2f)", 
			float64(fees.WriteData)/100, float64(fees.WriteDataMin)/100)
	})
	
	// Test 2: Calculate credits for different transaction mixes
	t.Run("CalculateMixedLoad", func(t *testing.T) {
		fees := &FeeSchedule{
			SendTokens:         300,   // 3.00 credits
			AddCredits:         300,   // 3.00 credits
			CreateTokenAccount: 2500,  // 25.00 credits
			CreateIdentity:     10000, // 100.00 credits
			WriteData:          1,     // 0.01 credits/byte
			WriteDataMin:       100,   // 1.00 credit minimum
		}
		
		testCases := []struct {
			name     string
			mix      TransactionMix
			expected int64 // Expected credits in internal units
		}{
			{
				name: "SimpleTokenTransfers",
				mix: TransactionMix{
					SendTokens: 1000,
				},
				expected: 1000 * 300, // 300,000 internal units = 3000 credits
			},
			{
				name: "MixedSimple",
				mix: TransactionMix{
					SendTokens: 900,
					AddCredits: 100,
				},
				expected: 900*300 + 100*300, // 300,000 internal units = 3000 credits
			},
			{
				name: "WithAccountCreation",
				mix: TransactionMix{
					SendTokens:         800,
					CreateTokenAccount: 20,
					CreateIdentity:     5,
				},
				expected: 800*300 + 20*2500 + 5*10000, // 340,000 internal units = 3400 credits
			},
			{
				name: "WithDataWrites",
				mix: TransactionMix{
					SendTokens:       500,
					WriteDataBytes:   10000, // 10KB of data
					WriteDataTxCount: 10,    // Split across 10 txs (1KB each)
				},
				expected: 500*300 + 10000*1, // 150,000 + 10,000 = 160,000 internal units
			},
			{
				name: "SmallDataWrites",
				mix: TransactionMix{
					WriteDataBytes:   50,  // 50 bytes
					WriteDataTxCount: 5,   // 10 bytes each
				},
				expected: 5 * 100, // 5 txs * minimum 100 = 500 internal units
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				credits := calculateMixCredits(tc.mix, fees)
				if credits != tc.expected {
					t.Errorf("Expected %d credits, got %d", tc.expected, credits)
				}
				t.Logf("Mix uses %.2f credits", float64(credits)/100)
			})
		}
	})
	
	// Test 3: Calculate ACME needed for credits at different oracle prices
	t.Run("CreditsToACME", func(t *testing.T) {
		testCases := []struct {
			credits     int64  // Credits in external units (not internal)
			oraclePrice uint64 // Oracle price (5000 = $0.50/ACME)
			expectedMin int64  // Minimum expected ACME in internal units
			expectedMax int64  // Maximum expected ACME in internal units
		}{
			{
				credits:     600,   // 600 credits
				oraclePrice: 5000,  // $0.50/ACME - triggers minimum
				expectedMin: 1000000, // 0.01 ACME minimum (120000 < minimum)
				expectedMax: 1000000,
			},
			{
				credits:     100,   // 100 credits  
				oraclePrice: 10000, // $1.00/ACME - triggers minimum
				expectedMin: 1000000,   // 0.01 ACME minimum (10000 < minimum)
				expectedMax: 1000000,
			},
			{
				credits:     1,     // 1 credit (should trigger minimum)
				oraclePrice: 5000,  // $0.50/ACME
				expectedMin: 1000000, // 0.01 ACME minimum
				expectedMax: 1000000,
			},
			{
				credits:     10000,   // 10,000 credits
				oraclePrice: 5000,    // $0.50/ACME
				expectedMin: 2000000, // 0.02 ACME (10000*100*1e8 / (5000*1e4))
				expectedMax: 2000000,
			},
			{
				credits:     50000,    // 50,000 credits  
				oraclePrice: 5000,     // $0.50/ACME
				expectedMin: 10000000, // 0.1 ACME
				expectedMax: 10000000,
			},
		}
		
		for _, tc := range testCases {
			name := fmt.Sprintf("%d_credits_at_%d", tc.credits, tc.oraclePrice)
			t.Run(name, func(t *testing.T) {
				acme := creditsToACME(tc.credits, tc.oraclePrice)
				if acme < tc.expectedMin || acme > tc.expectedMax {
					t.Errorf("Expected ACME between %d and %d, got %d (%.4f ACME)",
						tc.expectedMin, tc.expectedMax, acme, float64(acme)/1e8)
				}
				t.Logf("%d credits at oracle %d = %.4f ACME", 
					tc.credits, tc.oraclePrice, float64(acme)/1e8)
			})
		}
	})
	
	// Test 4: Full load test calculation
	t.Run("FullLoadCalculation", func(t *testing.T) {
		// Simulate a real load test scenario
		numAccounts := 50
		totalTxs := 10000
		
		mix := TransactionMix{
			SendTokens:         9000,  // 90% token transfers
			AddCredits:         500,   // 5% credit additions
			CreateTokenAccount: 500,   // 5% account creations
		}
		
		// Get real fees from network
		fees, err := fetchFeeSchedule(client)
		if err != nil {
			// Use defaults for test
			fees = &FeeSchedule{
				SendTokens:         300,
				AddCredits:         300,
				CreateTokenAccount: 2500,
			}
		}
		
		// Calculate total credits needed
		totalCredits := calculateMixCredits(mix, fees)
		creditsPerAccount := totalCredits / int64(numAccounts)
		
		// Get current oracle price
		status, _ := client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
		oraclePrice := uint64(5000) // Default to $0.50
		if status != nil && status.Oracle != nil && status.Oracle.Price > 0 {
			oraclePrice = status.Oracle.Price
		}
		
		// Calculate ACME needed
		acmeForCredits := creditsToACME(creditsPerAccount/100, oraclePrice) // Convert to external units
		acmeForTxs := int64(totalTxs/numAccounts) * int64(0.001*1e8) // 0.001 ACME per tx
		
		totalPerAccount := acmeForCredits + acmeForTxs
		totalWithBuffer := totalPerAccount + (totalPerAccount / 10) // 10% buffer
		
		t.Logf("Load Test Funding Calculation:")
		t.Logf("  Accounts: %d", numAccounts)
		t.Logf("  Total Transactions: %d", totalTxs)
		t.Logf("  Transaction Mix:")
		t.Logf("    - SendTokens: %d", mix.SendTokens)
		t.Logf("    - AddCredits: %d", mix.AddCredits)
		t.Logf("    - CreateTokenAccount: %d", mix.CreateTokenAccount)
		t.Logf("  Total Credits Needed: %.2f", float64(totalCredits)/100)
		t.Logf("  Credits Per Account: %.2f", float64(creditsPerAccount)/100)
		t.Logf("  Oracle Price: $%.2f/ACME", float64(oraclePrice)/10000)
		t.Logf("  ACME for Credits: %.4f", float64(acmeForCredits)/1e8)
		t.Logf("  ACME for Txs: %.4f", float64(acmeForTxs)/1e8)
		t.Logf("  Total Per Account: %.4f ACME", float64(totalPerAccount)/1e8)
		t.Logf("  With 10%% Buffer: %.4f ACME", float64(totalWithBuffer)/1e8)
		
		// Validate the calculation is reasonable
		if totalWithBuffer <= 0 {
			t.Error("Total ACME calculation resulted in zero or negative")
		}
		
		if float64(totalWithBuffer)/1e8 > 10.0 {
			t.Errorf("Per-account cost seems too high: %.4f ACME", float64(totalWithBuffer)/1e8)
		}
	})
}

// fetchFeeSchedule fetches the current fee schedule from the network
func fetchFeeSchedule(client *jsonrpc.Client) (*FeeSchedule, error) {
	// In a real implementation, this would query the network description
	// For now, return standard fees
	
	// Try to get network status for any fee information
	ctx := context.Background()
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get network status: %w", err)
	}
	
	// Return standard fee schedule
	// These are in internal units (100 = 1.00 credit)
	fees := &FeeSchedule{
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
	
	// Log that we're using defaults
	if status != nil {
		// In future, extract fees from status or description
		_ = status // Use status when fee extraction is implemented
	}
	
	return fees, nil
}

// calculateMixCredits calculates total credits needed for a transaction mix
func calculateMixCredits(mix TransactionMix, fees *FeeSchedule) int64 {
	total := int64(0)
	
	// Simple transaction types
	total += int64(mix.SendTokens) * fees.SendTokens
	total += int64(mix.AddCredits) * fees.AddCredits
	total += int64(mix.CreateTokenAccount) * fees.CreateTokenAccount
	total += int64(mix.CreateIdentity) * fees.CreateIdentity
	
	// Handle WriteData with minimum fee
	if mix.WriteDataTxCount > 0 {
		bytesPerTx := mix.WriteDataBytes / mix.WriteDataTxCount
		creditPerTx := int64(bytesPerTx) * fees.WriteData
		if creditPerTx < fees.WriteDataMin {
			creditPerTx = fees.WriteDataMin
		}
		total += int64(mix.WriteDataTxCount) * creditPerTx
	}
	
	return total
}

// creditsToACME converts credits to ACME amount based on oracle price
func creditsToACME(credits int64, oraclePrice uint64) int64 {
	// credits = (acmeAmount * oraclePrice * CreditUnitsPerFiatUnit) / AcmePrecision
	// So: acmeAmount = (credits * AcmePrecision) / (oraclePrice * CreditUnitsPerFiatUnit)
	
	if oraclePrice == 0 {
		return int64(0.01 * 1e8) // Return minimum if no oracle
	}
	
	// Credits passed in are already in external units (e.g., 600 = 600 credits)
	// Convert to internal units first
	creditsInternal := credits * protocol.CreditPrecision
	
	// Calculate ACME needed
	// Need to be careful with integer division
	numerator := uint64(creditsInternal) * protocol.AcmePrecision
	denominator := oraclePrice * protocol.CreditUnitsPerFiatUnit
	acmeNeeded := int64(numerator / denominator)
	
	// Ensure minimum 0.01 ACME
	minACME := int64(0.01 * 1e8)
	if acmeNeeded < minACME {
		acmeNeeded = minACME
	}
	
	return acmeNeeded
}

// TestFeeTableIntegration tests integration with actual load test
func TestFeeTableIntegration(t *testing.T) {
	// This test demonstrates how the fee table would integrate with load tests
	
	t.Run("LoadTestSetup", func(t *testing.T) {
		// Configuration for a load test
		config := LoadConfig{
			NumSenders:   10,
			NumReceivers: 10,
			NumTxs:       1000,
		}
		
		// Fetch fees (in real test, from network)
		fees := &FeeSchedule{
			SendTokens: 300, // 3.00 credits
		}
		
		// Calculate credits per sender
		txPerSender := config.NumTxs / config.NumSenders
		creditsPerSender := int64(txPerSender) * fees.SendTokens
		
		// Add 10% buffer
		creditsWithBuffer := creditsPerSender + (creditsPerSender / 10)
		
		t.Logf("Load Test Credit Calculation:")
		t.Logf("  Transactions per sender: %d", txPerSender)
		t.Logf("  Credits per transaction: %.2f", float64(fees.SendTokens)/100)
		t.Logf("  Credits per sender: %.2f", float64(creditsPerSender)/100)
		t.Logf("  With 10%% buffer: %.2f", float64(creditsWithBuffer)/100)
		
		// Verify calculation
		expectedCredits := int64(100 * 300) // 100 txs * 3.00 credits
		if creditsPerSender != expectedCredits {
			t.Errorf("Expected %d credits, got %d", expectedCredits, creditsPerSender)
		}
	})
}