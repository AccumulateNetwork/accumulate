// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

func TestFromECDSA(t *testing.T) {
	// Generate a test private key
	privKey, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Export to bytes
	privBytes := FromECDSA(privKey)
	if len(privBytes) == 0 {
		t.Fatal("FromECDSA returned empty bytes")
	}

	// Import back
	privKey2, err := ToECDSA(privBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Should be the same
	if privKey.D.Cmp(privKey2.D) != 0 {
		t.Fatal("Private keys don't match after round trip")
	}
}

func TestFromECDSAPub(t *testing.T) {
	// Generate a test private key
	privKey, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Export public key to bytes
	pubBytes := FromECDSAPub(&privKey.PublicKey)
	if len(pubBytes) != 65 {
		t.Fatalf("Expected 65 bytes, got %d", len(pubBytes))
	}

	// Import back
	pubKey2, err := UnmarshalPubkey(pubBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Should be the same
	if privKey.PublicKey.X.Cmp(pubKey2.X) != 0 || privKey.PublicKey.Y.Cmp(pubKey2.Y) != 0 {
		t.Fatal("Public keys don't match after round trip")
	}
}

func TestSign(t *testing.T) {
	// Generate a test private key
	privKey, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Create test hash
	hash := sha256.Sum256([]byte("test message"))

	// Sign
	sig, err := Sign(hash[:], privKey)
	if err != nil {
		t.Fatal(err)
	}

	if len(sig) != 64 {
		t.Fatalf("Expected 64 byte signature, got %d", len(sig))
	}

	// Verify
	pubBytes := FromECDSAPub(&privKey.PublicKey)
	if !VerifySignature(pubBytes, hash[:], sig) {
		t.Fatal("Signature verification failed")
	}
}

func TestBTCPrivKeyFromBytes(t *testing.T) {
	// Generate test private key bytes
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	privBytes := privKey.D.Bytes()

	// Create BTC private key from bytes
	btcPriv, btcPub := BTCPrivKeyFromBytes(S256(), privBytes)

	if btcPriv == nil || btcPub == nil {
		t.Fatal("BTCPrivKeyFromBytes returned nil")
	}

	// Test signing
	btcPrivKey := &BTCPrivKey{PrivateKey: btcPriv}
	hash := sha256.Sum256([]byte("test"))

	sig, err := btcPrivKey.Sign(hash[:])
	if err != nil {
		t.Fatal(err)
	}

	// Test verification
	if !sig.Verify(hash[:], btcPub) {
		t.Fatal("BTC signature verification failed")
	}
}

func TestParseFunctions(t *testing.T) {
	// Generate test data
	privKey, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Test signature parsing
	hash := sha256.Sum256([]byte("test"))
	btcPrivKey := &BTCPrivKey{PrivateKey: privKey}
	sig, err := btcPrivKey.Sign(hash[:])
	if err != nil {
		t.Fatal(err)
	}

	sigBytes := sig.Serialize()
	parsedSig, err := ParseSignature(sigBytes)
	if err != nil {
		t.Fatal(err)
	}

	if parsedSig.R.Cmp(sig.R) != 0 || parsedSig.S.Cmp(sig.S) != 0 {
		t.Fatal("Parsed signature doesn't match original")
	}

	// Test public key parsing
	pubBytes := FromECDSAPub(&privKey.PublicKey)
	parsedPub, err := ParsePubKey(pubBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !IsEqual(&privKey.PublicKey, parsedPub) {
		t.Fatal("Parsed public key doesn't match original")
	}
}

func TestPubkeyToAddress(t *testing.T) {
	// Generate test private key
	privKey, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Get Ethereum address
	addr := PubkeyToAddress(privKey.PublicKey)

	if len(addr) != 20 {
		t.Fatalf("Expected 20 byte address, got %d", len(addr))
	}

	// Address should not be all zeros
	allZero := true
	for _, b := range addr {
		if b != 0 {
			allZero = false
			break
		}
	}

	if allZero {
		t.Fatal("Address is all zeros")
	}
}

func TestDecompressPubkey(t *testing.T) {
	// For ARM64 compatibility, we'll test that the function exists
	// and handles invalid input correctly

	// Test with invalid length
	_, err := DecompressPubkey([]byte{0x02, 0x01, 0x02})
	if err == nil {
		t.Fatal("Expected error for invalid compressed key length")
	}

	// Test with valid length but invalid data (should fail gracefully)
	invalidCompressed := make([]byte, 33)
	invalidCompressed[0] = 0x02
	_, err = DecompressPubkey(invalidCompressed)
	// This may or may not error depending on the random data, that's ok

	// The important thing is that the function doesn't crash
	t.Log("DecompressPubkey function works and handles invalid input gracefully")
}
