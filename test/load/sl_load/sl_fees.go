//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FeeSchedule represents the fee costs for different transaction types
// All values are in internal credit units (100 = 1.00 credit)
type FeeSchedule struct {
	SendTokens         int64 // Credits per SendTokens transaction
	AddCredits         int64 // Credits per AddCredits transaction
	CreateTokenAccount int64 // Credits per CreateTokenAccount
	CreateIdentity     int64 // Credits per CreateIdentity
	CreateDataAccount  int64 // Credits per CreateDataAccount
	CreateKeyPage      int64 // Credits per CreateKeyPage
	UpdateKeyPage      int64 // Credits per UpdateKeyPage
	WriteData          int64 // Credits per byte for WriteData
	WriteDataMin       int64 // Minimum credits for WriteData
	BurnTokens         int64 // Credits per BurnTokens
	IssueTokens        int64 // Credits per IssueTokens
}

// GetDefaultFeeSchedule returns the standard fee schedule
// These are the actual values used by Accumulate networks
func GetDefaultFeeSchedule() *FeeSchedule {
	return &FeeSchedule{
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
}

// FetchFeeSchedule attempts to get fee schedule from network, falls back to defaults
func FetchFeeSchedule(client *jsonrpc.Client) (*FeeSchedule, error) {
	// Currently the API doesn't expose fee schedule directly
	// In future, this would query the network description
	// For now, return the known standard fees
	
	// Verify network is accessible
	ctx := context.Background()
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to network: %w", err)
	}
	
	// Return standard fee schedule
	return GetDefaultFeeSchedule(), nil
}

// CalculateCreditsForTransactions calculates total credits needed for a set of transactions
// Returns credits in internal units (multiply by CreditPrecision)
func CalculateCreditsForTransactions(numTxs int, fees *FeeSchedule) int64 {
	// For load tests, we primarily use SendTokens
	// Each SendTokens transaction costs 3.00 credits (300 internal units)
	return int64(numTxs) * fees.SendTokens
}

// CalculateCreditsForMix calculates credits for a mix of transaction types
// Returns credits in internal units
func CalculateCreditsForMix(sendTokens, addCredits, createAccounts int, fees *FeeSchedule) int64 {
	total := int64(0)
	total += int64(sendTokens) * fees.SendTokens
	total += int64(addCredits) * fees.AddCredits
	total += int64(createAccounts) * fees.CreateTokenAccount
	return total
}

// CreditsToACME converts credits (in external units) to ACME amount based on oracle price
// credits: Credits in external units (e.g., 600 = 600.00 credits)
// oraclePrice: Oracle price (5000 = $0.50/ACME)
// Returns: ACME amount in internal units (1e8 = 1 ACME)
func CreditsToACME(credits int64, oraclePrice uint64) int64 {
	// The inverse of ACMEToCredits
	// credits = (acme * oracle * CreditPrecision) / (AcmePrecision * AcmeOraclePrecision / 100)
	// Rearranging for acme:
	// acme = (credits * AcmePrecision * AcmeOraclePrecision / 100) / (oracle * CreditPrecision)
	
	if oraclePrice == 0 {
		// No oracle price, return minimum
		return int64(0.01 * 1e8)
	}
	
	// Convert credits to internal units
	creditsInternal := credits * protocol.CreditPrecision
	
	// Calculate ACME needed using corrected formula
	// Note: CreditsPerDollar = 100
	numerator := uint64(creditsInternal) * protocol.AcmePrecision * protocol.AcmeOraclePrecision / 100
	denominator := oraclePrice * uint64(protocol.CreditPrecision)
	
	if denominator == 0 {
		return int64(0.01 * 1e8)
	}
	
	acmeNeeded := int64(numerator / denominator)
	
	// Ensure minimum 0.01 ACME
	minACME := int64(0.01 * 1e8)
	if acmeNeeded < minACME {
		acmeNeeded = minACME
	}
	
	return acmeNeeded
}

// ACMEToCredits converts ACME amount to credits based on oracle price
// acmeAmount: ACME in internal units (1e8 = 1 ACME)
// oraclePrice: Oracle price (5000 = $0.50/ACME)
// Returns: Credits in internal units (100 = 1.00 credit)
func ACMEToCredits(acmeAmount int64, oraclePrice uint64) uint64 {
	if oraclePrice == 0 {
		return 0
	}
	
	// The formula needs to account for all precisions:
	// Step 1: Convert ACME to dollars: dollars = (acme/AcmePrecision) * (oracle/OraclePrecision)
	// Step 2: Convert dollars to credits: credits = dollars * CreditsPerDollar * CreditPrecision
	// Combined: credits = (acme * oracle * CreditPrecision) / (AcmePrecision * OraclePrecision / CreditsPerDollar)
	// Where CreditsPerDollar = 100
	// Simplifying: credits = (acme * oracle * 100) / (1e8 * 10000 / 100)
	//             credits = (acme * oracle * 100) / 1e10
	
	// Note: CreditUnitsPerFiatUnit = CreditsPerDollar * CreditPrecision = 100 * 100 = 10000
	// But this doesn't account for OraclePrecision, so we can't use it directly
	
	credits := (uint64(acmeAmount) * oraclePrice * protocol.CreditPrecision) / 
		(protocol.AcmePrecision * protocol.AcmeOraclePrecision / 100)
	return credits
}

// CalculateFundingRequirements calculates total ACME and credits needed for a load test
type FundingRequirements struct {
	TotalTransactions   int
	TransactionsPerK    int
	ACMEPerTransaction  int64  // ACME cost per transaction (internal units)
	CreditsPerK         int64  // Credits needed per K account (internal units)
	ACMEPerK            int64  // ACME needed per K account for transactions (internal units)
	ACMEForCreditsPerK  int64  // ACME needed to buy credits for K account (internal units)
	TotalACMEPerK       int64  // Total ACME per K account (internal units)
	TotalACMENeeded     int64  // Total ACME for all accounts (internal units)
	FaucetCallsNeeded   int    // Number of faucet calls needed
}

// CalculateFunding calculates all funding requirements for a load test
func CalculateFunding(config LoadConfig, fees *FeeSchedule, oraclePrice uint64) *FundingRequirements {
	req := &FundingRequirements{
		TotalTransactions: config.NumTxs,
	}
	
	// Calculate transactions per K account
	req.TransactionsPerK = config.NumTxs / config.NumSenders
	if config.NumTxs % config.NumSenders != 0 {
		req.TransactionsPerK++ // Round up for remainder
	}
	
	// ACME for transactions (0.001 ACME per transaction)
	req.ACMEPerTransaction = int64(0.001 * 1e8)
	req.ACMEPerK = int64(req.TransactionsPerK) * req.ACMEPerTransaction
	
	// Credits needed per K account (6 credits per SendTokens for safety - double the actual fee)
	// Using 6 credits instead of 3 to ensure sufficient funding
	creditsNeededInternal := int64(req.TransactionsPerK) * 600 // 6 credits = 600 internal units
	creditsNeededExternal := creditsNeededInternal / protocol.CreditPrecision
	req.CreditsPerK = creditsNeededInternal
	
	// ACME to buy those credits
	req.ACMEForCreditsPerK = CreditsToACME(creditsNeededExternal, oraclePrice)
	
	// Total ACME per K account (with 20% buffer for extra safety)
	totalBeforeBuffer := req.ACMEPerK + req.ACMEForCreditsPerK
	req.TotalACMEPerK = totalBeforeBuffer + (totalBeforeBuffer / 5) // 20% buffer
	
	// Total ACME for all K accounts
	req.TotalACMENeeded = int64(config.NumSenders) * req.TotalACMEPerK
	
	// Add funding account overhead (for distributing to K accounts)
	// Estimate 2 credits per K account for distribution operations
	fundingCreditsInternal := int64(config.NumSenders * 2 * protocol.CreditPrecision)
	fundingCreditsExternal := fundingCreditsInternal / protocol.CreditPrecision
	fundingACMEForCredits := CreditsToACME(fundingCreditsExternal, oraclePrice)
	
	// Total including funding account needs
	req.TotalACMENeeded += fundingACMEForCredits
	req.TotalACMENeeded = req.TotalACMENeeded + (req.TotalACMENeeded / 5) // 20% buffer on total
	
	// Calculate faucet calls (10 ACME per call)
	faucetACME := int64(10 * 1e8)
	req.FaucetCallsNeeded = int(req.TotalACMENeeded / faucetACME)
	if req.TotalACMENeeded % faucetACME != 0 {
		req.FaucetCallsNeeded++
	}
	
	return req
}

// FormatCredits formats internal credit units for display
// Always shows with 2 decimal places
func FormatCredits(creditsInternal int64) string {
	return fmt.Sprintf("%.2f", float64(creditsInternal)/protocol.CreditPrecision)
}

// FormatACME formats internal ACME units for display
// Shows with 4 decimal places for precision
func FormatACME(acmeInternal int64) string {
	return fmt.Sprintf("%.4f", float64(acmeInternal)/1e8)
}