//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"crypto/ed25519"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var testCtx *LoadTestContext

type LoadTestContext struct {
	Client      *jsonrpc.Client
	Context     context.Context
	Seed        [32]byte
	FundingAcct LiteAccount
	KAccounts   []LiteAccount
	AAccounts   []LiteAccount
	Oracle      uint64
	Config      LoadConfig
}

type LiteAccount struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	URL        *url.URL
	Balance    *big.Int
	Credits    uint64
}

type LoadConfig struct {
	NumSenders   int
	NumReceivers int
	NumTxs       int
	TxAmount     int64
	ACMEPerK     int64
	CreditsPerK  int64
}

type LoadResults struct {
	TotalSent    int
	TotalSuccess int
	TotalFailed  int
	TPS          float64
	Duration     time.Duration
}

type Summary struct {
	TotalExpectedSent     int64
	TotalActualSent       int64
	TotalExpectedReceived int64
	TotalActualReceived   int64
	SenderDiscrepancy     int64
	ReceiverDiscrepancy   int64
}

type Issue struct {
	Account     string
	Type        string
	Description string
}

type Transaction struct {
	From   LiteAccount
	To     LiteAccount
	Amount int64
	Hash   []byte
	Status string
}

const (
	// Debug mode toggle
	DEBUG_MODE = true // Set to false for production
	
	// Debug mode configuration
	DEBUG_FUNDING_ACME    = 200 * 1e8         // 200 ACME total for funding account (supports 10 senders x 5 ACME each + credits)
	DEBUG_CREDITS_AMOUNT  = 0.5 * 1e8         // 0.5 ACME worth of credits per account
	DEBUG_SETTLEMENT_WAIT = 15 * time.Second  // 15 seconds for settlement
	DEBUG_FAUCET_RETRIES  = 20                // 20 retries for faucet verification
	
	// Production mode configuration
	PROD_FUNDING_MULTIPLIER = 1.2             // 20% buffer for funding
	PROD_SETTLEMENT_WAIT    = 30 * time.Second
	PROD_FAUCET_RETRIES     = 30
	
	// Common timeouts
	DefaultTimeout = 30 * time.Second
	FaucetDelay    = 1 * time.Second
)

// GetSettlementWait returns wait time based on mode
func GetSettlementWait() time.Duration {
	if DEBUG_MODE {
		return DEBUG_SETTLEMENT_WAIT
	}
	return PROD_SETTLEMENT_WAIT
}

// GetMaxRetries returns max retries based on mode
func GetMaxRetries() int {
	if DEBUG_MODE {
		return DEBUG_FAUCET_RETRIES
	}
	return PROD_FAUCET_RETRIES
}

// GetRequiredFunding calculates required funding based on mode
func GetRequiredFunding(config LoadConfig) int64 {
	if DEBUG_MODE {
		// Calculate based on actual needs: senders * ACME + credits + buffer
		totalNeeded := int64(config.NumSenders)*config.ACMEPerK + 
		               int64(config.NumSenders)*config.CreditsPerK + 
		               10*1e8 // 10 ACME buffer for fees
		// Use the larger of calculated or configured amount
		if totalNeeded > DEBUG_FUNDING_ACME {
			return totalNeeded
		}
		return DEBUG_FUNDING_ACME
	}
	totalNeeded := int64(config.NumSenders)*config.ACMEPerK + 
	               int64(config.NumSenders)*config.CreditsPerK
	return int64(float64(totalNeeded) * PROD_FUNDING_MULTIPLIER)
}