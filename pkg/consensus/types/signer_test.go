// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestADISigner(t *testing.T) {
	// Generate a test key
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Parse a key page URL
	keyPageURL, err := url.Parse("acc://validator-1.acme/book/1")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	// Create ADI signer
	signer, err := NewADISigner(keyPageURL, privKey)
	if err != nil {
		t.Fatalf("Failed to create ADI signer: %v", err)
	}

	// Test ValidatorID returns root identity
	validatorID := signer.ValidatorID()
	expected := "acc://validator-1.acme"
	if validatorID != expected {
		t.Errorf("ValidatorID() = %q, want %q", validatorID, expected)
	}

	// Test PublicKey
	pubKey := signer.PublicKey()
	expectedPubKey := privKey.Public().(ed25519.PublicKey)
	if string(pubKey) != string(expectedPubKey) {
		t.Error("PublicKey() does not match expected public key")
	}

	// Test Sign
	message := []byte("test message to sign")
	signature, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}

	// Verify signature
	if !ed25519.Verify(pubKey, message, signature) {
		t.Error("Signature verification failed")
	}
}

func TestADISignerValidation(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(rand.Reader)

	// Test nil URL
	_, err := NewADISigner(nil, privKey)
	if err == nil {
		t.Error("Expected error for nil URL, got nil")
	}

	// Test invalid key size
	keyPageURL, _ := url.Parse("acc://validator.acme/book/1")
	_, err = NewADISigner(keyPageURL, []byte("short"))
	if err == nil {
		t.Error("Expected error for invalid key size, got nil")
	}
}

func TestRawKeySigner(t *testing.T) {
	// Generate a test key
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create raw key signer
	signer, err := NewRawKeySigner(privKey)
	if err != nil {
		t.Fatalf("Failed to create raw key signer: %v", err)
	}

	// Test ValidatorID returns hex-encoded public key
	validatorID := signer.ValidatorID()
	expectedID := hexEncode(privKey.Public().(ed25519.PublicKey))
	if validatorID != expectedID {
		t.Errorf("ValidatorID() = %q, want %q", validatorID, expectedID)
	}

	// Test PublicKey
	pubKey := signer.PublicKey()
	expectedPubKey := privKey.Public().(ed25519.PublicKey)
	if string(pubKey) != string(expectedPubKey) {
		t.Error("PublicKey() does not match expected public key")
	}

	// Test Sign
	message := []byte("test message to sign")
	signature, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}

	// Verify signature
	if !ed25519.Verify(pubKey, message, signature) {
		t.Error("Signature verification failed")
	}

	// Test PrivateKey accessor
	retrievedKey := signer.PrivateKey()
	if string(retrievedKey) != string(privKey) {
		t.Error("PrivateKey() does not match original key")
	}
}

func TestRawKeySignerValidation(t *testing.T) {
	// Test invalid key size
	_, err := NewRawKeySigner([]byte("short"))
	if err == nil {
		t.Error("Expected error for invalid key size, got nil")
	}
}

func TestSignerInterface(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(rand.Reader)
	keyPageURL, _ := url.Parse("acc://test.acme/book/1")

	// Both signers should implement the Signer interface
	var signer Signer

	adiSigner, _ := NewADISigner(keyPageURL, privKey)
	signer = adiSigner
	_ = signer.ValidatorID()
	_ = signer.PublicKey()
	_, _ = signer.Sign([]byte("test"))

	rawSigner, _ := NewRawKeySigner(privKey)
	signer = rawSigner
	_ = signer.ValidatorID()
	_ = signer.PublicKey()
	_, _ = signer.Sign([]byte("test"))
}
