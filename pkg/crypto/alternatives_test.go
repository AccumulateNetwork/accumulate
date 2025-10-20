package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

func TestFromECDSA(t *testing.T) {
	// Generate a test private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Test FromECDSA
	bytes := FromECDSA(priv)
	if len(bytes) == 0 {
		t.Error("FromECDSA returned empty bytes")
	}

	// Test ToECDSA roundtrip
	recovered, err := ToECDSA(bytes)
	if err != nil {
		t.Error("ToECDSA failed:", err)
	}

	if priv.D.Cmp(recovered.D) != 0 {
		t.Error("ToECDSA did not recover the original key")
	}
}

func TestFromECDSAPub(t *testing.T) {
	// Generate a test private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Test FromECDSAPub
	pubBytes := FromECDSAPub(&priv.PublicKey)
	if len(pubBytes) == 0 {
		t.Error("FromECDSAPub returned empty bytes")
	}

	// Test UnmarshalPubkey roundtrip
	recovered, err := UnmarshalPubkey(pubBytes)
	if err != nil {
		t.Error("UnmarshalPubkey failed:", err)
	}

	if priv.PublicKey.X.Cmp(recovered.X) != 0 || priv.PublicKey.Y.Cmp(recovered.Y) != 0 {
		t.Error("UnmarshalPubkey did not recover the original public key")
	}
}

func TestSign(t *testing.T) {
	// Generate a test private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Test message
	message := []byte("test message for signing")
	hash := make([]byte, 32)
	copy(hash, message[:min(len(message), 32)])

	// Sign the hash
	signature, err := Sign(hash, priv)
	if err != nil {
		t.Error("Sign failed:", err)
	}

	if len(signature) != 64 {
		t.Errorf("Expected signature length 64, got %d", len(signature))
	}

	// Test verification
	pubBytes := FromECDSAPub(&priv.PublicKey)
	if !VerifySignature(pubBytes, hash, signature) {
		t.Error("VerifySignature failed to verify valid signature")
	}

	// Test with invalid signature
	invalidSig := make([]byte, 64)
	if VerifySignature(pubBytes, hash, invalidSig) {
		t.Error("VerifySignature verified invalid signature")
	}
}

func TestBTCSignature(t *testing.T) {
	// Generate test key
	privateKey := make([]byte, 32)
	for i := range privateKey {
		privateKey[i] = byte(i + 1)
	}

	// Create BTC private key
	ecdsaPriv, ecdsaPub := BTCPrivKeyFromBytes(S256(), privateKey)
	btcPriv := &BTCPrivKey{PrivateKey: ecdsaPriv}

	// Test signing
	message := []byte("test message for BTC signing")
	hash := make([]byte, 32)
	copy(hash, message[:min(len(message), 32)])

	signature, err := btcPriv.Sign(hash)
	if err != nil {
		t.Error("BTCPrivKey.Sign failed:", err)
	}

	// Test verification
	if !signature.Verify(hash, ecdsaPub) {
		t.Error("BTCSignature.Verify failed")
	}

	// Test serialization
	serialized := signature.Serialize()
	if len(serialized) != 64 {
		t.Errorf("Expected serialized signature length 64, got %d", len(serialized))
	}

	// Test parsing
	parsed, err := ParseSignature(serialized)
	if err != nil {
		t.Error("ParseSignature failed:", err)
	}

	if signature.R.Cmp(parsed.R) != 0 || signature.S.Cmp(parsed.S) != 0 {
		t.Error("ParseSignature did not recover original signature")
	}
}

func TestPubkeyToAddress(t *testing.T) {
	// Generate a test private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Test PubkeyToAddress
	addr := PubkeyToAddress(priv.PublicKey)
	if len(addr) != 20 {
		t.Errorf("Expected address length 20, got %d", len(addr))
	}

	// Test that same key produces same address
	addr2 := PubkeyToAddress(priv.PublicKey)
	if addr != addr2 {
		t.Error("Same key produced different addresses")
	}
}

func TestCurveCompatibility(t *testing.T) {
	// Test that S256 returns P256 for ARM compatibility
	curve := S256()
	if curve != elliptic.P256() {
		t.Error("S256() should return P256 for ARM compatibility")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}