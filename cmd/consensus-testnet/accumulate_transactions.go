// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"math/big"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AccumulateTransactionGenerator generates valid Accumulate protocol transactions.
type AccumulateTransactionGenerator struct {
	// Key pair for signing
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey

	// Test account URLs
	liteTokenAccount *url.URL // Lite token account for this key

	// Nonce tracking
	timestamp atomic.Uint64

	// Token precision (ACME uses 8 decimal places)
	precision uint64
}

// NewAccumulateTransactionGenerator creates a new generator for Accumulate transactions.
func NewAccumulateTransactionGenerator(privateKey ed25519.PrivateKey) (*AccumulateTransactionGenerator, error) {
	pubKey := privateKey.Public().(ed25519.PublicKey)

	// Create lite token account URL from public key
	liteTokenAccount, err := protocol.LiteTokenAddress(pubKey, "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	gen := &AccumulateTransactionGenerator{
		privateKey:       privateKey,
		publicKey:        pubKey,
		liteTokenAccount: liteTokenAccount,
		precision:        protocol.AcmePrecisionPower,
	}

	// Initialize timestamp
	gen.timestamp.Store(uint64(time.Now().UTC().UnixMilli()))

	return gen, nil
}

// GenerateSendTokens creates a valid SendTokens transaction.
// This is the most common transaction type for testing throughput.
func (g *AccumulateTransactionGenerator) GenerateSendTokens(amount uint64, recipient *url.URL) (*messaging.Envelope, error) {
	ts := g.timestamp.Add(1)

	env, err := build.Transaction().For(g.liteTokenAccount).
		SendTokens(amount, g.precision).To(recipient).
		SignWith(g.liteTokenAccount).Version(1).Timestamp(&ts).PrivateKey(g.privateKey).
		Done()
	if err != nil {
		return nil, err
	}

	return env, nil
}

// GenerateSelfSendTokens creates a SendTokens transaction to self (for testing).
func (g *AccumulateTransactionGenerator) GenerateSelfSendTokens(amount uint64) (*messaging.Envelope, error) {
	return g.GenerateSendTokens(amount, g.liteTokenAccount)
}

// GenerateWriteData creates a WriteData transaction with arbitrary data.
func (g *AccumulateTransactionGenerator) GenerateWriteData(data []byte) (*messaging.Envelope, error) {
	ts := g.timestamp.Add(1)

	// Create a lite data account URL from the public key hash
	// LiteDataAddress expects a 32-byte chain ID
	liteDataAccount, err := protocol.LiteDataAddress(g.publicKey)
	if err != nil {
		return nil, err
	}

	env, err := build.Transaction().For(liteDataAccount).
		WriteData(data).
		SignWith(g.liteTokenAccount).Version(1).Timestamp(&ts).PrivateKey(g.privateKey).
		Done()
	if err != nil {
		return nil, err
	}

	return env, nil
}

// GenerateBurnTokens creates a BurnTokens transaction.
func (g *AccumulateTransactionGenerator) GenerateBurnTokens(amount uint64) (*messaging.Envelope, error) {
	ts := g.timestamp.Add(1)

	env, err := build.Transaction().For(g.liteTokenAccount).
		BurnTokens(amount, g.precision).
		SignWith(g.liteTokenAccount).Version(1).Timestamp(&ts).PrivateKey(g.privateKey).
		Done()
	if err != nil {
		return nil, err
	}

	return env, nil
}

// GenerateTransaction creates a transaction based on the specified type.
// This provides a unified interface for transaction generation.
func (g *AccumulateTransactionGenerator) GenerateTransaction(txType AccumulateTxType, data []byte) (*messaging.Envelope, error) {
	switch txType {
	case AccumulateTxTypeSendTokens:
		return g.GenerateSelfSendTokens(1)
	case AccumulateTxTypeWriteData:
		return g.GenerateWriteData(data)
	case AccumulateTxTypeBurnTokens:
		return g.GenerateBurnTokens(1)
	default:
		return g.GenerateSelfSendTokens(1)
	}
}

// AccumulateTxType identifies the type of Accumulate transaction to generate.
type AccumulateTxType uint8

const (
	AccumulateTxTypeSendTokens AccumulateTxType = iota
	AccumulateTxTypeWriteData
	AccumulateTxTypeBurnTokens
)

// GetLiteTokenAccount returns the lite token account URL for this generator.
func (g *AccumulateTransactionGenerator) GetLiteTokenAccount() *url.URL {
	return g.liteTokenAccount
}

// GetPublicKey returns the public key used by this generator.
func (g *AccumulateTransactionGenerator) GetPublicKey() ed25519.PublicKey {
	return g.publicKey
}

// ValidateEnvelope performs basic validation on an Accumulate envelope.
// Returns true if the envelope appears to be valid.
func ValidateEnvelope(env *messaging.Envelope) bool {
	if env == nil {
		return false
	}

	// Must have at least one transaction
	if len(env.Transaction) == 0 {
		return false
	}

	// Must have at least one signature
	if len(env.Signatures) == 0 {
		return false
	}

	// Verify each transaction has a body
	for _, txn := range env.Transaction {
		if txn == nil || txn.Body == nil {
			return false
		}
	}

	// Verify each signature
	for _, sig := range env.Signatures {
		if sig == nil {
			return false
		}

		// For ED25519 signatures, verify the signature
		if ed25519Sig, ok := sig.(*protocol.ED25519Signature); ok {
			if len(ed25519Sig.Signature) != ed25519.SignatureSize {
				return false
			}
			if len(ed25519Sig.PublicKey) != ed25519.PublicKeySize {
				return false
			}
		}
	}

	return true
}

// ExtractTransactionHash extracts the transaction hash from an envelope.
func ExtractTransactionHash(env *messaging.Envelope) [32]byte {
	if len(env.Transaction) > 0 && env.Transaction[0] != nil {
		return env.Transaction[0].ID().Hash()
	}
	return [32]byte{}
}

// CreateGenesisLiteAccount creates genesis state for a lite token account.
// In a real implementation, this would initialize the account in the state database.
type GenesisAccount struct {
	URL       *url.URL
	Balance   big.Int
	PublicKey ed25519.PublicKey
}

// CreateGenesisAccounts creates test accounts for the consensus testnet.
func CreateGenesisAccounts(keys []ed25519.PrivateKey, initialBalance uint64) ([]GenesisAccount, error) {
	accounts := make([]GenesisAccount, 0, len(keys))

	for _, key := range keys {
		pubKey := key.Public().(ed25519.PublicKey)
		liteURL, err := protocol.LiteTokenAddress(pubKey, "ACME", protocol.SignatureTypeED25519)
		if err != nil {
			return nil, err
		}

		balance := new(big.Int).SetUint64(initialBalance)
		// Scale by precision
		balance.Mul(balance, big.NewInt(int64(protocol.AcmePrecision)))

		accounts = append(accounts, GenesisAccount{
			URL:       liteURL,
			Balance:   *balance,
			PublicKey: pubKey,
		})
	}

	return accounts, nil
}
