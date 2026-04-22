// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package wallet

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestWalletCreateAndLoad(t *testing.T) {
	// Create temp wallet file
	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	// Create wallet
	w := New(walletPath)

	// Generate funder
	err := w.GenerateFunder("acc://test-funder.acme/tokens")
	if err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}

	// Generate accounts
	err = w.GenerateAccounts(10, 10, 10)
	if err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}

	// Verify counts
	if w.Count() != 30 {
		t.Errorf("Expected 30 accounts, got %d", w.Count())
	}
	if w.CountByType("lite") != 10 {
		t.Errorf("Expected 10 lite accounts, got %d", w.CountByType("lite"))
	}
	if w.CountByType("adi-token") != 10 {
		t.Errorf("Expected 10 ADI token accounts, got %d", w.CountByType("adi-token"))
	}
	if w.CountByType("adi-data") != 10 {
		t.Errorf("Expected 10 ADI data accounts, got %d", w.CountByType("adi-data"))
	}

	// Save wallet
	err = w.Save()
	if err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(walletPath); os.IsNotExist(err) {
		t.Errorf("Wallet file was not created")
	}

	// Load wallet
	w2, err := Load(walletPath)
	if err != nil {
		t.Fatalf("Failed to load wallet: %v", err)
	}

	// Verify loaded wallet
	if w2.Count() != 30 {
		t.Errorf("Loaded wallet has %d accounts, expected 30", w2.Count())
	}
	if w2.Funder.URL != "acc://test-funder.acme/tokens" {
		t.Errorf("Funder URL mismatch: got %s", w2.Funder.URL)
	}
}

func TestGetAccount(t *testing.T) {
	w := New("")
	_ = w.GenerateFunder("acc://test-funder.acme/tokens")
	_ = w.GenerateAccounts(5, 5, 5)

	// Test valid index
	acct, err := w.GetAccount(0)
	if err != nil {
		t.Fatalf("Failed to get account 0: %v", err)
	}
	if acct.Index != 0 {
		t.Errorf("Expected account index 0, got %d", acct.Index)
	}

	// Test invalid index
	_, err = w.GetAccount(999)
	if err == nil {
		t.Errorf("Expected error for invalid index, got nil")
	}
}

func TestGetAccountByURL(t *testing.T) {
	w := New("")
	_ = w.GenerateFunder("acc://test-funder.acme/tokens")
	_ = w.GenerateAccounts(5, 5, 5)

	// Get first ADI token account
	adiAccounts := w.GetAccountsByType("adi-token")
	if len(adiAccounts) != 5 {
		t.Fatalf("Expected 5 ADI token accounts, got %d", len(adiAccounts))
	}

	// Test finding by URL
	acct, err := w.GetAccountByURL(adiAccounts[0].URL)
	if err != nil {
		t.Fatalf("Failed to get account by URL: %v", err)
	}
	if acct.URL != adiAccounts[0].URL {
		t.Errorf("URL mismatch: got %s, expected %s", acct.URL, adiAccounts[0].URL)
	}

	// Test non-existent URL
	_, err = w.GetAccountByURL("acc://nonexistent.acme/tokens")
	if err == nil {
		t.Errorf("Expected error for non-existent URL, got nil")
	}
}

func TestAccountKeys(t *testing.T) {
	w := New("")
	_ = w.GenerateFunder("acc://test-funder.acme/tokens")
	_ = w.GenerateAccounts(1, 0, 0)

	acct := w.Accounts[0]

	// Test GetPrivateKey
	privKey, err := acct.GetPrivateKey()
	if err != nil {
		t.Fatalf("Failed to get private key: %v", err)
	}
	if len(privKey) != 64 {
		t.Errorf("Expected 64-byte private key, got %d bytes", len(privKey))
	}

	// Test GetPublicKey
	pubKey, err := acct.GetPublicKey()
	if err != nil {
		t.Fatalf("Failed to get public key: %v", err)
	}
	if len(pubKey) != 32 {
		t.Errorf("Expected 32-byte public key, got %d bytes", len(pubKey))
	}

	// Verify keys match
	derivedPubKey := privKey.Public().(ed25519.PublicKey)
	if string(derivedPubKey) != string(pubKey) {
		t.Errorf("Public key derived from private key doesn't match stored public key")
	}
}

func TestGetAccountsByType(t *testing.T) {
	w := New("")
	_ = w.GenerateFunder("acc://test-funder.acme/tokens")
	_ = w.GenerateAccounts(3, 4, 5)

	liteAccounts := w.GetAccountsByType("lite")
	if len(liteAccounts) != 3 {
		t.Errorf("Expected 3 lite accounts, got %d", len(liteAccounts))
	}

	adiTokenAccounts := w.GetAccountsByType("adi-token")
	if len(adiTokenAccounts) != 4 {
		t.Errorf("Expected 4 ADI token accounts, got %d", len(adiTokenAccounts))
	}

	adiDataAccounts := w.GetAccountsByType("adi-data")
	if len(adiDataAccounts) != 5 {
		t.Errorf("Expected 5 ADI data accounts, got %d", len(adiDataAccounts))
	}

	// Verify all accounts have correct type
	for _, acct := range liteAccounts {
		if acct.Type != "lite" {
			t.Errorf("Account type mismatch: expected 'lite', got '%s'", acct.Type)
		}
	}
}

func TestAccountURLFormat(t *testing.T) {
	w := New("")
	_ = w.GenerateFunder("acc://test-funder.acme/tokens")
	_ = w.GenerateAccounts(1, 1, 1)

	// Check lite account URL format
	liteAccounts := w.GetAccountsByType("lite")
	if len(liteAccounts) != 1 {
		t.Fatalf("Expected 1 lite account")
	}
	// Lite URLs should start with acc://
	if liteAccounts[0].URL[:6] != "acc://" {
		t.Errorf("Lite account URL should start with 'acc://', got: %s", liteAccounts[0].URL)
	}

	// Check ADI token account URL format
	adiTokenAccounts := w.GetAccountsByType("adi-token")
	if len(adiTokenAccounts) != 1 {
		t.Fatalf("Expected 1 ADI token account")
	}
	// Should be acc://test-a00000001.acme/tokens
	if adiTokenAccounts[0].URL != "acc://test-a00000001.acme/tokens" {
		t.Errorf("ADI token account URL mismatch, got: %s", adiTokenAccounts[0].URL)
	}

	// Check ADI data account URL format
	adiDataAccounts := w.GetAccountsByType("adi-data")
	if len(adiDataAccounts) != 1 {
		t.Fatalf("Expected 1 ADI data account")
	}
	// Should be acc://test-d00000001.acme/data
	if adiDataAccounts[0].URL != "acc://test-d00000001.acme/data" {
		t.Errorf("ADI data account URL mismatch, got: %s", adiDataAccounts[0].URL)
	}
}
