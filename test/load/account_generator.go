// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package load_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccount represents a test account with its keys and URLs
type TestAccount struct {
	Key          ed25519.PrivateKey
	LiteURL      *url.URL // The token account URL (with /ACME)
	LiteIdentity *url.URL // The lite identity URL (without /ACME)
	Balance      *big.Int
}

// GenerateTestAccounts generates a set of test accounts with deterministic keys
func GenerateTestAccounts(prefix string, count int) []TestAccount {
	accounts := make([]TestAccount, count)
	
	for i := range accounts {
		// Generate deterministic key from seed
		seed := fmt.Sprintf("%s%d test seed", prefix, i+1)
		hash := sha256.Sum256([]byte(seed))
		accounts[i].Key = ed25519.NewKeyFromSeed(hash[:])
		
		// Generate lite URLs
		pubKeyHash := sha256.Sum256(accounts[i].Key.Public().(ed25519.PublicKey))
		accounts[i].LiteIdentity = protocol.LiteAuthorityForKey(pubKeyHash[:20], protocol.SignatureTypeED25519)
		accounts[i].LiteURL = accounts[i].LiteIdentity.JoinPath("ACME")
		accounts[i].Balance = big.NewInt(0)
	}
	
	return accounts
}

// GenerateKAccounts generates 10 'k' accounts for sending
func GenerateKAccounts() []TestAccount {
	return GenerateTestAccounts("k", 10)
}

// GenerateAAccounts generates 10 'a' accounts for receiving
func GenerateAAccounts() []TestAccount {
	return GenerateTestAccounts("a", 10)
}

// GetAccountInfo returns formatted account information
func GetAccountInfo(accounts []TestAccount, prefix string) []string {
	info := make([]string, len(accounts))
	for i, acc := range accounts {
		info[i] = fmt.Sprintf("%s%d: %s", prefix, i+1, acc.LiteURL)
	}
	return info
}