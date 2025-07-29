// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"fmt"
	"time"
)

// ADIOrchestrator manages the processing of user-provided ADIs to generate
// cryptographic proofs and retrieve verified account information.
type ADIOrchestrator struct {
	client         *LiteClient
	prover         *HealingProofGenerator
	adisOfInterest map[string]bool // Track which ADIs the user cares about
}

// NewADIOrchestrator creates a new ADI orchestrator using existing lite client infrastructure.
func NewADIOrchestrator(client *LiteClient) (*ADIOrchestrator, error) {
	// Create proof generator using existing healing logic
	prover, err := NewHealingProofGenerator(client.v2)
	if err != nil {
		return nil, fmt.Errorf("failed to create proof generator: %w", err)
	}

	return &ADIOrchestrator{
		client:         client,
		prover:         prover,
		adisOfInterest: make(map[string]bool),
	}, nil
}

// Close releases resources used by the orchestrator.
func (ao *ADIOrchestrator) Close() error {
	if ao.prover != nil {
		return ao.prover.Close()
	}
	return nil
}

// ProcessTargetADIs is the main orchestration method that processes a list of user-provided ADIs.
// It discovers accounts, generates proofs, and returns verified account information.
func (ao *ADIOrchestrator) ProcessTargetADIs(ctx context.Context, adis []string) (*ADIProcessingReport, error) {
	fmt.Printf("\n=== ADI ORCHESTRATOR: Processing %d ADIs ===\n", len(adis))

	report := &ADIProcessingReport{
		ProcessedADIs: make(map[string]*ADIResult),
		Timestamp:     time.Now(),
		Summary: &ProcessingSummary{
			TotalADIs: len(adis),
		},
	}

	for _, adi := range adis {
		fmt.Printf("\n--- Processing ADI: %s ---\n", adi)
		result, err := ao.processADI(ctx, adi)
		if err != nil {
			fmt.Printf("❌ Failed to process ADI %s: %v\n", adi, err)
			result = &ADIResult{
				ADI:      adi,
				Status:   "failed",
				Error:    err.Error(),
				Accounts: make(map[string]*VerifiedAccountInfo),
			}
		} else {
			fmt.Printf("✅ Successfully processed ADI %s with %d accounts\n", adi, len(result.Accounts))
			report.Summary.SuccessfulADIs++
		}

		report.ProcessedADIs[adi] = result
		report.Summary.TotalAccounts += len(result.Accounts)

		// Count verified accounts
		for _, account := range result.Accounts {
			if account.Verified {
				report.Summary.VerifiedAccounts++
			}
		}
	}

	fmt.Printf("\n=== ADI PROCESSING COMPLETE ===\n")
	fmt.Printf("Total ADIs: %d, Successful: %d\n", report.Summary.TotalADIs, report.Summary.SuccessfulADIs)
	fmt.Printf("Total Accounts: %d, Verified: %d\n", report.Summary.TotalAccounts, report.Summary.VerifiedAccounts)

	return report, nil
}

// processADI processes a single ADI by discovering its accounts and generating proofs.
func (ao *ADIOrchestrator) processADI(ctx context.Context, adi string) (*ADIResult, error) {
	result := &ADIResult{
		ADI:      adi,
		Accounts: make(map[string]*VerifiedAccountInfo),
	}

	// Step 1: Discover all accounts under this ADI
	accounts, err := ao.discoverADIAccounts(ctx, adi)
	if err != nil {
		return nil, fmt.Errorf("failed to discover accounts for ADI %s: %w", adi, err)
	}

	fmt.Printf("  Discovered %d accounts for ADI %s\n", len(accounts), adi)

	// Step 2: Process each discovered account
	for _, accountURL := range accounts {
		fmt.Printf("  Processing account: %s\n", accountURL)

		verifiedInfo, err := ao.processAccount(ctx, accountURL)
		if err != nil {
			fmt.Printf("    ⚠ Warning: failed to process account %s: %v\n", accountURL, err)
			// Create unverified entry for failed accounts
			verifiedInfo = &VerifiedAccountInfo{
				URL:      accountURL,
				Type:     "unknown",
				Verified: false,
				Error:    err.Error(),
			}
		}

		result.Accounts[accountURL] = verifiedInfo

		if verifiedInfo.Verified {
			fmt.Printf("    ✓ Verified: %s (%s)\n", accountURL, verifiedInfo.Type)
		} else {
			fmt.Printf("    ⚠ Unverified: %s (%s)\n", accountURL, verifiedInfo.Type)
		}
	}

	result.Status = "completed"
	return result, nil
}

// discoverADIAccounts discovers all accounts associated with an ADI.
// Uses existing universal account API to query and discover accounts.
func (ao *ADIOrchestrator) discoverADIAccounts(ctx context.Context, adi string) ([]string, error) {
	var accounts []string

	// Always include the ADI identity account itself
	adiURL := fmt.Sprintf("acc://%s", adi)
	accounts = append(accounts, adiURL)

	// Check if the ADI identity account exists to validate the ADI
	_, err := ao.client.getAccountData(ctx, adiURL)
	if err != nil {
		return nil, fmt.Errorf("ADI identity account not found: %w", err)
	}

	// Common ADI sub-accounts to check
	commonAccounts := []string{
		fmt.Sprintf("acc://%s/token", adi),
		fmt.Sprintf("acc://%s/staking", adi),
		fmt.Sprintf("acc://%s/book", adi),
		fmt.Sprintf("acc://%s/book/1", adi),
	}

	// Check which common accounts actually exist
	for _, account := range commonAccounts {
		if _, err := ao.client.getAccountData(ctx, account); err == nil {
			accounts = append(accounts, account)
		}
	}

	return accounts, nil
}

// processAccount processes a single account by retrieving data and generating proofs.
// Uses existing universal account API and healing proof generator.
func (ao *ADIOrchestrator) processAccount(ctx context.Context, accountURL string) (*VerifiedAccountInfo, error) {
	// Step 1: Get account data using existing universal API
	accountData, err := ao.client.getAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	// Step 2: Generate proof using existing healing proof generator
	var verified bool
	var proofMethod string

	verifiedAccount, err := ao.prover.GenerateProof(ctx, accountURL)
	if err != nil {
		fmt.Printf("      Proof generation failed: %v\n", err)
		verified = false
		proofMethod = "none"
	} else {
		// Validate the generated proof
		if ao.prover.ValidateReceipt(verifiedAccount.Receipt) {
			verified = true
			proofMethod = "healing"
		} else {
			verified = false
			proofMethod = "invalid"
		}
	}

	// Step 3: Create verified account info with type-specific data
	info := &VerifiedAccountInfo{
		URL:         accountURL,
		Type:        accountData.TypeName,
		Verified:    verified,
		ProofMethod: proofMethod,
	}

	// Add type-specific information using existing universal API methods
	if accountData.IsTokenAccount() {
		if balanceInfo, err := ao.client.getTokenBalance(ctx, accountURL); err == nil {
			info.Balance = balanceInfo.Balance
			info.TokenURL = balanceInfo.TokenURL
		}
	} else if accountData.IsIdentityAccount() {
		if identityInfo, err := ao.client.getIdentityInfo(ctx, accountURL); err == nil {
			info.KeyBook = identityInfo.KeyBook
		}
	}

	return info, nil
}

// Data structures for ADI processing results

// ADIProcessingReport contains the results of processing multiple ADIs.
type ADIProcessingReport struct {
	ProcessedADIs map[string]*ADIResult `json:"processedADIs"`
	Timestamp     time.Time             `json:"timestamp"`
	Summary       *ProcessingSummary    `json:"summary"`
}

// ADIResult contains the processing results for a single ADI.
type ADIResult struct {
	ADI      string                          `json:"adi"`
	Status   string                          `json:"status"` // "completed", "failed"
	Accounts map[string]*VerifiedAccountInfo `json:"accounts"`
	Error    string                          `json:"error,omitempty"`
}

// VerifiedAccountInfo contains verified information about a single account.
type VerifiedAccountInfo struct {
	URL      string `json:"url"`
	Type     string `json:"type"`
	Verified bool   `json:"verified"` // True if cryptographic proof succeeded

	// Token account fields
	Balance  string `json:"balance,omitempty"`
	TokenURL string `json:"tokenUrl,omitempty"`

	// Identity account fields
	KeyBook string `json:"keyBook,omitempty"`

	// Proof metadata
	ProofMethod string `json:"proofMethod,omitempty"` // "healing", "none", "invalid"
	Error       string `json:"error,omitempty"`
}

// ProcessingSummary provides aggregate statistics about ADI processing.
type ProcessingSummary struct {
	TotalADIs        int `json:"totalADIs"`
	SuccessfulADIs   int `json:"successfulADIs"`
	TotalAccounts    int `json:"totalAccounts"`
	VerifiedAccounts int `json:"verifiedAccounts"`
}

// Helper method to get a summary string for the report
func (r *ADIProcessingReport) GetSummaryString() string {
	return fmt.Sprintf("Processed %d ADIs (%d successful), %d accounts (%d verified)",
		r.Summary.TotalADIs, r.Summary.SuccessfulADIs,
		r.Summary.TotalAccounts, r.Summary.VerifiedAccounts)
}

// Helper method to check if an ADI was successfully processed
func (r *ADIProcessingReport) IsADISuccessful(adi string) bool {
	if result, exists := r.ProcessedADIs[adi]; exists {
		return result.Status == "completed"
	}
	return false
}

// Helper method to get all verified accounts across all ADIs
func (r *ADIProcessingReport) GetAllVerifiedAccounts() map[string]*VerifiedAccountInfo {
	verified := make(map[string]*VerifiedAccountInfo)

	for _, result := range r.ProcessedADIs {
		for url, account := range result.Accounts {
			if account.Verified {
				verified[url] = account
			}
		}
	}

	return verified
}

// GetADIData retrieves complete ADI information in simplified format
// This method handles the conversion from internal processing report to public API format
func (ao *ADIOrchestrator) GetADIData(ctx context.Context, adiURL string) (*ADIData, error) {
	// Process the ADI using existing orchestration logic
	report, err := ao.ProcessTargetADIs(ctx, []string{adiURL})
	if err != nil {
		return nil, fmt.Errorf("failed to process ADI %s: %w", adiURL, err)
	}

	// Convert the processing report to simplified ADI data format
	return ao.convertReportToADIData(adiURL, report)
}

// AddADIOfInterest adds an ADI to the list of ADIs this client cares about
func (ao *ADIOrchestrator) AddADIOfInterest(adiURL string) error {
	ao.adisOfInterest[adiURL] = true
	return nil
}

// RemoveADIOfInterest removes an ADI from the list and prunes its cache data
func (ao *ADIOrchestrator) RemoveADIOfInterest(adiURL string) error {
	delete(ao.adisOfInterest, adiURL)

	// Prune all cached data for this ADI
	accounts := ao.client.unifiedCache.GetADIAccounts(adiURL)
	for _, account := range accounts {
		ao.client.unifiedCache.InvalidateAccount(account.URL)
	}

	return nil
}

// GetADIsOfInterest returns the list of ADIs this client is currently tracking
func (ao *ADIOrchestrator) GetADIsOfInterest() []string {
	adis := make([]string, 0, len(ao.adisOfInterest))
	for adi := range ao.adisOfInterest {
		adis = append(adis, adi)
	}
	return adis
}

// convertReportToADIData converts an ADI processing report to simplified ADI data format
// This handles the architectural separation between internal processing and public API
func (ao *ADIOrchestrator) convertReportToADIData(adiURL string, report *ADIProcessingReport) (*ADIData, error) {
	if report == nil {
		return nil, fmt.Errorf("processing report is nil")
	}

	// Find the specific ADI in the report
	adiResult, exists := report.ProcessedADIs[adiURL]
	if !exists {
		return nil, fmt.Errorf("ADI %s not found in processing report", adiURL)
	}

	if adiResult.Status != "completed" {
		return nil, fmt.Errorf("ADI %s processing failed: %s", adiURL, adiResult.Error)
	}

	// Convert verified accounts to simplified format
	var simplifiedAccounts []*SimpleAccountData
	for _, verifiedAccount := range adiResult.Accounts {
		if verifiedAccount == nil {
			continue
		}

		// Get transactions for this account from cache
		txs, _ := ao.client.unifiedCache.GetTransactions(verifiedAccount.URL)
		simplifiedTxs := make([]*SimpleTransaction, len(txs))
		for i, tx := range txs {
			simplifiedTxs[i] = &SimpleTransaction{
				TxID:      tx.TxID,
				Type:      tx.Type,
				Status:    tx.Status,
				Timestamp: tx.Timestamp,
				Amount:    tx.Amount,
				From:      tx.From,
				To:        tx.To,
			}
		}

		simplifiedAccounts = append(simplifiedAccounts, &SimpleAccountData{
			URL:          verifiedAccount.URL,
			Type:         verifiedAccount.Type,
			Balance:      verifiedAccount.Balance,
			Transactions: simplifiedTxs,
		})
	}

	return &ADIData{
		URL:         adiURL,
		Accounts:    simplifiedAccounts,
		LastUpdated: report.Timestamp,
		FromCache:   false, // Fresh data from processing
	}, nil
}
