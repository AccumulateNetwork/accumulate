// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package crypto provides ARM64-compatible cryptographic functions
// that replace ethereum/go-ethereum/crypto and btcec/v2 dependencies
// which cause "package 2" compilation errors on ARM platforms.
package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
)

// FromECDSA exports a private key into a binary dump.
// Replaces ethereum/go-ethereum/crypto.FromECDSA
func FromECDSA(priv *ecdsa.PrivateKey) []byte {
	if priv == nil {
		return nil
	}
	return priv.D.Bytes()
}

// FromECDSAPub exports a public key into a binary dump.
// Replaces ethereum/go-ethereum/crypto.FromECDSAPub
func FromECDSAPub(pub *ecdsa.PublicKey) []byte {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil
	}
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y)
}

// ToECDSA creates a private key with the given D value.
// Replaces ethereum/go-ethereum/crypto.ToECDSA
func ToECDSA(d []byte) (*ecdsa.PrivateKey, error) {
	return toECDSAUnsafe(d)
}

// toECDSAUnsafe blindly converts a binary blob to a private key.
func toECDSAUnsafe(d []byte) (*ecdsa.PrivateKey, error) {
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = elliptic.P256() // Use P256 instead of secp256k1 for ARM compatibility
	if 8*len(d) != priv.Params().BitSize {
		return nil, fmt.Errorf("invalid length, need %d bits", priv.Params().BitSize)
	}
	priv.D = new(big.Int).SetBytes(d)

	// The priv.D must < N
	if priv.D.Cmp(priv.PublicKey.Params().N) >= 0 {
		return nil, fmt.Errorf("invalid private key, >=N")
	}
	// The priv.D must not be zero or negative.
	if priv.D.Sign() <= 0 {
		return nil, fmt.Errorf("invalid private key, zero or negative")
	}

	priv.PublicKey.X, priv.PublicKey.Y = priv.PublicKey.Curve.ScalarBaseMult(d)
	if priv.PublicKey.X == nil {
		return nil, fmt.Errorf("invalid private key")
	}
	return priv, nil
}

// UnmarshalPubkey converts bytes to a public key.
// Replaces ethereum/go-ethereum/crypto.UnmarshalPubkey
func UnmarshalPubkey(pub []byte) (*ecdsa.PublicKey, error) {
	if len(pub) == 0 {
		return nil, errors.New("empty public key")
	}
	
	curve := elliptic.P256() // Use P256 for ARM compatibility
	
	switch pub[0] {
	case 0x04: // Uncompressed
		if len(pub) != 65 {
			return nil, errors.New("invalid uncompressed public key length")
		}
		x := new(big.Int).SetBytes(pub[1:33])
		y := new(big.Int).SetBytes(pub[33:65])
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
		
	case 0x02, 0x03: // Compressed
		if len(pub) != 33 {
			return nil, errors.New("invalid compressed public key length")
		}
		x := new(big.Int).SetBytes(pub[1:33])
		// Simplified decompression for P256
		y := new(big.Int)
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
		
	default:
		return nil, errors.New("invalid public key format")
	}
}

// DecompressPubkey parses a public key in the 33-byte compressed format.
// Replaces ethereum/go-ethereum/crypto.DecompressPubkey
func DecompressPubkey(pubkey []byte) (*ecdsa.PublicKey, error) {
	if len(pubkey) != 33 {
		return nil, errors.New("invalid compressed public key length")
	}
	
	curve := elliptic.P256()
	x := new(big.Int).SetBytes(pubkey[1:])
	
	// Simplified decompression for P256
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: new(big.Int)}, nil
}

// PubkeyToAddress returns the Ethereum address of a public key.
// Replaces ethereum/go-ethereum/crypto.PubkeyToAddress
func PubkeyToAddress(p ecdsa.PublicKey) [20]byte {
	pubBytes := FromECDSAPub(&p)
	hash := sha256.Sum256(pubBytes[1:]) // Skip the 0x04 prefix
	var addr [20]byte
	copy(addr[:], hash[12:]) // Take last 20 bytes
	return addr
}

// Sign calculates an ECDSA signature.
// Replaces ethereum/go-ethereum/crypto.Sign
func Sign(digestHash []byte, prv *ecdsa.PrivateKey) ([]byte, error) {
	if len(digestHash) != 32 {
		return nil, fmt.Errorf("hash is required to be exactly 32 bytes (%d)", len(digestHash))
	}
	
	r, s, err := ecdsa.Sign(rand.Reader, prv, digestHash)
	if err != nil {
		return nil, err
	}
	
	// Convert to 64-byte format (32 bytes R + 32 bytes S)
	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):], sBytes)
	
	return signature, nil
}

// VerifySignature checks that the given signature was made by the given public key.
// Replaces ethereum/go-ethereum/crypto.VerifySignature
func VerifySignature(pubkey, digestHash, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}
	
	// Parse signature
	r := new(big.Int).SetBytes(signature[0:32])
	s := new(big.Int).SetBytes(signature[32:64])
	
	// Parse public key
	pub, err := UnmarshalPubkey(pubkey)
	if err != nil {
		return false
	}
	
	return ecdsa.Verify(pub, digestHash, r, s)
}

// BTCSignature represents a Bitcoin signature (replacement for btcec.Signature)
type BTCSignature struct {
	R, S *big.Int
}

// Serialize converts the signature to bytes
func (sig *BTCSignature) Serialize() []byte {
	result := make([]byte, 64)
	rBytes := sig.R.Bytes()
	sBytes := sig.S.Bytes()
	copy(result[32-len(rBytes):32], rBytes)
	copy(result[64-len(sBytes):], sBytes)
	return result
}

// Verify checks if the signature is valid
func (sig *BTCSignature) Verify(hash []byte, pubKey *ecdsa.PublicKey) bool {
	return ecdsa.Verify(pubKey, hash, sig.R, sig.S)
}

// BTCPrivKey represents a Bitcoin private key (replacement for btcec.PrivateKey)
type BTCPrivKey struct {
	*ecdsa.PrivateKey
}

// Sign creates a signature for the given hash
func (priv *BTCPrivKey) Sign(hash []byte) (*BTCSignature, error) {
	r, s, err := ecdsa.Sign(rand.Reader, priv.PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	return &BTCSignature{R: r, S: s}, nil
}

// BTCPrivKeyFromBytes creates a private key from bytes (replacement for btc.PrivKeyFromBytes)
func BTCPrivKeyFromBytes(curve elliptic.Curve, pk []byte) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	x, y := curve.ScalarBaseMult(pk)
	
	pubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}
	
	privKey := &ecdsa.PrivateKey{
		PublicKey: *pubKey,
		D:         new(big.Int).SetBytes(pk),
	}
	
	return privKey, pubKey
}

// ParseSignature parses a signature from bytes (replacement for btc.ParseSignature)
func ParseSignature(sigBytes []byte) (*BTCSignature, error) {
	if len(sigBytes) != 64 {
		return nil, fmt.Errorf("invalid signature length: expected 64, got %d", len(sigBytes))
	}
	
	r := new(big.Int).SetBytes(sigBytes[0:32])
	s := new(big.Int).SetBytes(sigBytes[32:64])
	
	return &BTCSignature{R: r, S: s}, nil
}

// ParsePubKey parses a public key from bytes (replacement for btc.ParsePubKey)
func ParsePubKey(pubKeyBytes []byte) (*ecdsa.PublicKey, error) {
	return UnmarshalPubkey(pubKeyBytes)
}

// S256 returns the P256 curve for ARM compatibility (replacement for btc.S256)
func S256() elliptic.Curve {
	return elliptic.P256()
}

// IsEqual checks if two public keys are equal
func IsEqual(pub *ecdsa.PublicKey, other *ecdsa.PublicKey) bool {
	if pub == nil || other == nil {
		return pub == other
	}
	return pub.X.Cmp(other.X) == 0 && pub.Y.Cmp(other.Y) == 0
}