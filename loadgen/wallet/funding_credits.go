package wallet

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// QueryClient interface for querying accounts
type QueryClient interface {
	Query(ctx context.Context, u *url.URL, query api.Query) (api.Record, error)
}

// SubmitClient interface for submitting transactions
type SubmitClient interface {
	Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error)
}

const (
	MinimumFundingBalance = 100 * protocol.AcmePrecision // 100 ACME in lowest denomination
	MaximumCreditBalance  = 500                           // Credits before skipping top-up
	CreditsToAdd          = 1000                          // Credits to add per top-up
	OraclePrice           = 100                           // Credits per ACME
)

// CreditTarget represents an account that can receive credits
type CreditTarget interface {
	GetURL() *url.URL
	GetCreditBalance() uint64
	SetCreditBalance(uint64)
	GetType() string // "lite" or "keypage"
}

// LiteAccountTarget wraps a LiteIdentity for credit operations
type LiteAccountTarget struct {
	*LiteIdentity
}

func (l *LiteAccountTarget) GetURL() *url.URL {
	return l.URL
}

func (l *LiteAccountTarget) GetCreditBalance() uint64 {
	return l.CreditBalance
}

func (l *LiteAccountTarget) SetCreditBalance(balance uint64) {
	l.CreditBalance = balance
}

func (l *LiteAccountTarget) GetType() string {
	return "lite"
}

// KeyPageTarget wraps a KeyPage for credit operations
type KeyPageTarget struct {
	*KeyPage
}

func (k *KeyPageTarget) GetURL() *url.URL {
	return k.URL
}

func (k *KeyPageTarget) GetCreditBalance() uint64 {
	return k.CreditBalance
}

func (k *KeyPageTarget) SetCreditBalance(balance uint64) {
	k.CreditBalance = balance
}

func (k *KeyPageTarget) GetType() string {
	return "keypage"
}

// TransactionSigner handles transaction signing
type TransactionSigner interface {
	SignTransaction(txn *protocol.Transaction, signerUrl *url.URL, privateKey []byte) (*messaging.Envelope, error)
}

// CreditManager manages credit distribution
type CreditManager struct {
	client         QueryClient
	submitter      SubmitClient
	signer         TransactionSigner
	fundingAccount *LiteIdentity
}

// NewCreditManager creates a new credit manager
func NewCreditManager(client QueryClient, submitter SubmitClient, signer TransactionSigner, fundingAccount *LiteIdentity) *CreditManager {
	return &CreditManager{
		client:         client,
		submitter:      submitter,
		signer:         signer,
		fundingAccount: fundingAccount,
	}
}

// TopUpLiteAccount adds credits to a lite account if needed
func (cm *CreditManager) TopUpLiteAccount(ctx context.Context, account *LiteIdentity) error {
	target := &LiteAccountTarget{LiteIdentity: account}
	return cm.topUpAccount(ctx, target)
}

// TopUpKeyPage adds credits to a key page if needed
func (cm *CreditManager) TopUpKeyPage(ctx context.Context, keyPage *KeyPage) error {
	target := &KeyPageTarget{KeyPage: keyPage}
	return cm.topUpAccount(ctx, target)
}

// topUpAccount is the unified implementation for topping up any credit target
func (cm *CreditManager) topUpAccount(ctx context.Context, target CreditTarget) error {
	// Step 1: Check funding account balance
	fundingBalance, err := cm.checkFundingBalance(ctx)
	if err != nil {
		return fmt.Errorf("failed to check funding balance: %w", err)
	}

	if fundingBalance < MinimumFundingBalance {
		return fmt.Errorf("insufficient funding: have %d ACME, need at least %d ACME",
			int64(fundingBalance/protocol.AcmePrecision),
			int64(MinimumFundingBalance/protocol.AcmePrecision))
	}

	log.Printf("CreditManager: Funding account has %d ACME", fundingBalance/protocol.AcmePrecision)

	// Step 2: Check target account credits
	currentCredits, err := cm.checkTargetCredits(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to check target credits: %w", err)
	}

	// Update local cache
	target.SetCreditBalance(currentCredits)

	if currentCredits > MaximumCreditBalance {
		log.Printf("CreditManager: %s %s has %d credits (> %d), skipping top-up",
			target.GetType(), target.GetURL(), currentCredits, MaximumCreditBalance)
		return nil
	}

	log.Printf("CreditManager: %s %s has %d credits, adding %d credits",
		target.GetType(), target.GetURL(), currentCredits, CreditsToAdd)

	// Step 3: Add credits
	err = cm.addCredits(ctx, target, CreditsToAdd)
	if err != nil {
		return fmt.Errorf("failed to add credits: %w", err)
	}

	log.Printf("CreditManager: Successfully added %d credits to %s %s",
		CreditsToAdd, target.GetType(), target.GetURL())

	return nil
}

// checkFundingBalance queries the funding account's ACME balance
func (cm *CreditManager) checkFundingBalance(ctx context.Context) (uint64, error) {
	// Build token URL for ACME balance
	tokenUrl := cm.fundingAccount.URL.WithPath("/ACME")

	// Query token account
	query := &api.DefaultQuery{}
	resp, err := cm.client.Query(ctx, tokenUrl, query)
	if err != nil {
		return 0, fmt.Errorf("failed to query funding account: %w", err)
	}

	// Extract balance - handle both TokenAccount and LiteTokenAccount
	if accRecord, ok := resp.(*api.AccountRecord); ok && accRecord.Account != nil {
		switch acc := accRecord.Account.(type) {
		case *protocol.TokenAccount:
			return acc.Balance.Uint64(), nil
		case *protocol.LiteTokenAccount:
			return acc.Balance.Uint64(), nil
		default:
			return 0, fmt.Errorf("funding account is wrong type: %T", accRecord.Account)
		}
	}

	return 0, fmt.Errorf("funding account not found")
}

// checkTargetCredits queries the target account's credit balance
func (cm *CreditManager) checkTargetCredits(ctx context.Context, target CreditTarget) (uint64, error) {
	query := &api.DefaultQuery{}
	resp, err := cm.client.Query(ctx, target.GetURL(), query)
	if err != nil {
		return 0, fmt.Errorf("failed to query target account: %w", err)
	}

	// Extract credit balance based on account type
	if accRecord, ok := resp.(*api.AccountRecord); ok && accRecord.Account != nil {
		switch acc := accRecord.Account.(type) {
		case *protocol.LiteIdentity:
			return acc.CreditBalance, nil
		case *protocol.KeyPage:
			return acc.CreditBalance, nil
		default:
			return 0, fmt.Errorf("account is not a credit-bearing type")
		}
	}

	return 0, fmt.Errorf("target account not found")
}

// addCredits creates and submits an AddCredits transaction
func (cm *CreditManager) addCredits(ctx context.Context, target CreditTarget, credits uint64) error {
	// Calculate ACME amount needed
	// credits * (ACME precision / credit precision)
	acmeAmount := new(big.Int).SetUint64(credits)
	acmeAmount.Mul(acmeAmount, big.NewInt(protocol.AcmePrecision))
	acmeAmount.Div(acmeAmount, big.NewInt(protocol.CreditPrecision))

	// Create AddCredits transaction
	txn := &protocol.AddCredits{
		Recipient: target.GetURL(),
		Amount:    *acmeAmount,
		Oracle:    OraclePrice,
	}

	// Create transaction wrapper
	transaction := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: cm.fundingAccount.URL,
		},
		Body: txn,
	}

	// Sign transaction
	var privateKey []byte
	if cm.fundingAccount.Key != nil {
		privateKey = cm.fundingAccount.Key.PrivateKey
	}
	envelope, err := cm.signer.SignTransaction(transaction, cm.fundingAccount.URL, privateKey)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit transaction
	submissions, err := cm.submitter.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		return fmt.Errorf("failed to submit transaction: %w", err)
	}

	if len(submissions) == 0 {
		return fmt.Errorf("no submissions returned")
	}

	// Check submission status
	for _, sub := range submissions {
		if sub.Status != nil && sub.Status.Failed() {
			return fmt.Errorf("transaction failed: %v", sub.Status)
		}
	}

	return nil
}

// DefaultTransactionSigner provides a basic transaction signer implementation
type DefaultTransactionSigner struct{}

func (s *DefaultTransactionSigner) SignTransaction(txn *protocol.Transaction, signerUrl *url.URL, privateKey []byte) (*messaging.Envelope, error) {
	// This is a simplified implementation
	// In production, you would:
	// 1. Create proper transaction hash
	// 2. Sign with private key
	// 3. Build complete envelope with signatures
	
	// For now, return a basic envelope structure
	// The actual implementation would depend on the specific Accumulate SDK version
	env := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.TransactionMessage{
				Transaction: txn,
			},
		},
	}

	// TODO: Implement actual signing logic
	// This would involve:
	// - Creating signature from private key
	// - Adding signature to envelope
	// - Setting proper routing

	return env, nil
}