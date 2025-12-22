// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package crypto provides ARM64-compatible alternatives to btcec/v2 and ethereum/go-ethereum/crypto
// to resolve compilation issues on Android/Termux and other ARM64 platforms.
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

// ARM64-compatible elliptic curve implementation
func S256() elliptic.Curve {
	return elliptic.P256()
}

// FromECDSA exports a private key into a binary dump.
// Replaces ethereum/go-ethereum/crypto.FromECDSA
func FromECDSA(priv *ecdsa.PrivateKey) []byte {
	if priv == nil {
		return nil
	}
	return priv.D.Bytes()
}

// ToECDSA creates a private key with the given D value.
// Replaces ethereum/go-ethereum/crypto.ToECDSA
func ToECDSA(d []byte) (*ecdsa.PrivateKey, error) {
	return toECDSA(d, true)
}

// ToECDSAUnsafe blindly converts a binary blob to a private key. It should almost
// never be used unless you are sure the input is valid and want to avoid hitting
// errors due to bad origin encoding (0 prefixes cut off).
func ToECDSAUnsafe(d []byte) *ecdsa.PrivateKey {
	priv, _ := toECDSA(d, false)
	return priv
}

// toECDSA creates a private key with the given D value. The strict parameter
// controls whether the key's length should be enforced.
func toECDSA(d []byte, strict bool) (*ecdsa.PrivateKey, error) {
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = S256()
	if strict && 8*len(d) != priv.Params().BitSize {
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
		return nil, errors.New("invalid private key")
	}
	return priv, nil
}

// FromECDSAPub exports a public key into a binary dump.
// Replaces ethereum/go-ethereum/crypto.FromECDSAPub
func FromECDSAPub(pub *ecdsa.PublicKey) []byte {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil
	}
	// Use the curve from the key, don't assume S256()
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y)
}

// UnmarshalPubkey converts bytes to a secp256k1 public key.
// Replaces ethereum/go-ethereum/crypto.UnmarshalPubkey
func UnmarshalPubkey(pub []byte) (*ecdsa.PublicKey, error) {
	x, y := elliptic.Unmarshal(S256(), pub)
	if x == nil {
		return nil, errors.New("invalid secp256k1 public key")
	}
	return &ecdsa.PublicKey{Curve: S256(), X: x, Y: y}, nil
}

// DecompressPubkey parses a public key in the 33-byte compressed format.
func DecompressPubkey(pubkey []byte) (*ecdsa.PublicKey, error) {
	if len(pubkey) != 33 {
		return nil, errors.New("invalid compressed public key length")
	}

	curve := S256()
	x := new(big.Int).SetBytes(pubkey[1:])

	// Calculate y^2 = x^3 + 7 (secp256k1 equation)
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	x3.Add(x3, big.NewInt(7))
	x3.Mod(x3, curve.Params().P)

	// Calculate y = sqrt(x^3 + 7)
	y := new(big.Int).ModSqrt(x3, curve.Params().P)
	if y == nil {
		return nil, errors.New("invalid compressed public key")
	}

	// Choose the correct y based on the compression flag
	if y.Bit(0) != uint(pubkey[0]&1) {
		y.Sub(curve.Params().P, y)
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// Sign calculates an ECDSA signature.
// Replaces ethereum/go-ethereum/crypto.Sign
func Sign(hash []byte, prv *ecdsa.PrivateKey) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("hash is required to be exactly 32 bytes (%d)", len(hash))
	}

	r, s, err := ecdsa.Sign(rand.Reader, prv, hash)
	if err != nil {
		return nil, err
	}

	// Convert to 64-byte signature (32 bytes r + 32 bytes s)
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)

	return sig, nil
}

// VerifySignature checks that the given public key created signature over hash.
// The public key should be in compressed (33 bytes) or uncompressed (65 bytes) format.
// The signature should be in [R || S] format.
// Replaces ethereum/go-ethereum/crypto.VerifySignature
func VerifySignature(pubkey, hash, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}
	if len(hash) != 32 {
		return false
	}

	pub, err := UnmarshalPubkey(pubkey)
	if err != nil {
		// Try decompressed format
		pub, err = DecompressPubkey(pubkey)
		if err != nil {
			return false
		}
	}

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	return ecdsa.Verify(pub, hash, r, s)
}

// PubkeyToAddress returns the Ethereum address of a public key
// Replaces ethereum/go-ethereum/crypto.PubkeyToAddress
func PubkeyToAddress(p ecdsa.PublicKey) [20]byte {
	pubBytes := FromECDSAPub(&p)
	hash := sha256.Sum256(pubBytes[1:]) // Skip the 0x04 prefix
	var addr [20]byte
	copy(addr[:], hash[12:]) // Take last 20 bytes
	return addr
}

// BTCPrivKey represents a Bitcoin private key with ECDSA operations
type BTCPrivKey struct {
	*ecdsa.PrivateKey
}

// BTCPrivKeyFromBytes creates a private key from bytes
// Replaces btcec.PrivKeyFromBytes
func BTCPrivKeyFromBytes(curve elliptic.Curve, pk []byte) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	privKey, err := toECDSA(pk, false)
	if err != nil || privKey == nil {
		return nil, nil
	}
	return privKey, &privKey.PublicKey
}

// ParseSignature parses a signature from bytes
// Replaces btcec.ParseSignature
func ParseSignature(sig []byte) (*BTCSignature, error) {
	if len(sig) < 64 {
		return nil, errors.New("signature too short")
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	return &BTCSignature{R: r, S: s}, nil
}

// ParsePubKey parses a public key from bytes
// Replaces btcec.ParsePubKey
func ParsePubKey(pubkey []byte) (*ecdsa.PublicKey, error) {
	if len(pubkey) == 33 {
		return DecompressPubkey(pubkey)
	}
	return UnmarshalPubkey(pubkey)
}

// BTCSignature represents a Bitcoin-style signature
type BTCSignature struct {
	R, S *big.Int
}

// Serialize returns the signature in DER format
func (sig *BTCSignature) Serialize() []byte {
	result := make([]byte, 64)
	rBytes := sig.R.Bytes()
	sBytes := sig.S.Bytes()

	copy(result[32-len(rBytes):32], rBytes)
	copy(result[64-len(sBytes):64], sBytes)

	return result
}

// Verify verifies the signature against a hash and public key
func (sig *BTCSignature) Verify(hash []byte, pubKey *ecdsa.PublicKey) bool {
	return ecdsa.Verify(pubKey, hash, sig.R, sig.S)
}

// Sign creates a signature for the given hash
func (privKey *BTCPrivKey) Sign(hash []byte) (*BTCSignature, error) {
	r, s, err := ecdsa.Sign(rand.Reader, privKey.PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	return &BTCSignature{R: r, S: s}, nil
}

// IsEqual checks if two public keys are equal
// Replaces pubkey comparison functions
func IsEqual(a, b *ecdsa.PublicKey) bool {
	return a.X.Cmp(b.X) == 0 && a.Y.Cmp(b.Y) == 0
}

// SerializeCompressed returns the 33-byte compressed format of the public key
func SerializeCompressed(pubKey *ecdsa.PublicKey) []byte {
	if pubKey == nil || pubKey.X == nil || pubKey.Y == nil {
		return nil
	}

	compressed := make([]byte, 33)
	xBytes := pubKey.X.Bytes()
	copy(compressed[33-len(xBytes):33], xBytes)

	// Set compression flag based on y coordinate parity
	if pubKey.Y.Bit(0) == 1 {
		compressed[0] = 0x03
	} else {
		compressed[0] = 0x02
	}

	return compressed
}

// SerializeUncompressed returns the uncompressed format of the public key
func SerializeUncompressed(pubKey *ecdsa.PublicKey) []byte {
	return FromECDSAPub(pubKey)
}

// Serialize returns the private key as bytes
func SerializePrivateKey(privKey *ecdsa.PrivateKey) []byte {
	return FromECDSA(privKey)
}
