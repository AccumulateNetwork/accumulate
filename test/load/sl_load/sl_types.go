//go:build !testnet
// +build !testnet

package load_test

import (
	"crypto/ed25519"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var testCtx *LoadTestContext

type LoadTestContext struct {
	Client      *jsonrpc.Client
	ClientV2    *client.Client  // Add v2 client for critical ops
	Seed        [32]byte
	FundingAcct LiteAccount
	KAccounts   []LiteAccount
	AAccounts   []LiteAccount
	Oracle      uint64
	Config      LoadConfig
	AAccountsReceived map[string]int64 // Track actual sent amounts to A accounts
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
	
	// Settlement configuration
	SETTLEMENT_WAIT = 15 * time.Second  // Base settlement wait time
	FAUCET_RETRIES  = 30                // Retries for faucet verification
	
	// Debug mode values
	DEBUG_SETTLEMENT_WAIT = 10 * time.Second
	DEBUG_FAUCET_RETRIES  = 20
	DEBUG_FUNDING_ACME    = 1000 * 1e8 // 1000 ACME for debug
	
	// Production mode values
	PROD_SETTLEMENT_WAIT    = 30 * time.Second
	PROD_FAUCET_RETRIES     = 30
	PROD_FUNDING_MULTIPLIER = 1.2 // 20% buffer
	
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