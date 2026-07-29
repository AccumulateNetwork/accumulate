// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package crypto provides ARM64-compatible alternatives to btcec/v2 and ethereum/go-ethereum/crypto
// to resolve compilation issues on Android/Termux and other ARM64 platforms.
// Uses decred's pure-Go secp256k1 implementation which is ARM64 compatible.
package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/sha3"
)

// S256 returns the secp256k1 curve used by Bitcoin and Ethereum.
// Uses decred's pure-Go implementation which is ARM64 compatible.
func S256() elliptic.Curve {
	return secp256k1.S256()
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
	// staticcheck suggests crypto/ecdh, which does not support secp256k1 — there
	// is no migration for this curve.
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y) //nolint:staticcheck // SA1019: no secp256k1 in crypto/ecdh
}

// UnmarshalPubkey converts bytes to a secp256k1 public key.
// Replaces ethereum/go-ethereum/crypto.UnmarshalPubkey
func UnmarshalPubkey(pub []byte) (*ecdsa.PublicKey, error) {
	x, y := elliptic.Unmarshal(S256(), pub) //nolint:staticcheck // SA1019: no secp256k1 in crypto/ecdh
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
// Uses Keccak256 as per Ethereum specification
func PubkeyToAddress(p ecdsa.PublicKey) [20]byte {
	pubBytes := FromECDSAPub(&p)
	// Ethereum uses Keccak256 of the uncompressed public key (without 0x04 prefix)
	h := sha3.NewLegacyKeccak256()
	h.Write(pubBytes[1:]) // Skip the 0x04 prefix
	hash := h.Sum(nil)
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
// Handles WIF-decoded keys (37/38 bytes) by extracting the 32-byte key portion,
// and raw 32-byte keys directly
func BTCPrivKeyFromBytes(curve elliptic.Curve, pk []byte) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	var keyBytes []byte
	switch len(pk) {
	case 32:
		// Raw 32-byte private key
		keyBytes = pk
	case 33:
		// 32-byte key with compression flag
		keyBytes = pk[:32]
	case 37:
		// WIF uncompressed: 1 byte version + 32 bytes key + 4 bytes checksum
		keyBytes = pk[1:33]
	case 38:
		// WIF compressed: 1 byte version + 32 bytes key + 1 byte flag + 4 bytes checksum
		keyBytes = pk[1:33]
	default:
		// For other lengths, try to extract 32 bytes after version byte if long enough
		if len(pk) > 33 {
			keyBytes = pk[1:33]
		} else if len(pk) > 32 {
			keyBytes = pk[:32]
		} else {
			keyBytes = pk
		}
	}

	privKey, err := toECDSA(keyBytes, false)
	if err != nil || privKey == nil {
		return nil, nil
	}
	// Override the curve to use the provided one
	privKey.PublicKey.Curve = curve
	privKey.PublicKey.X, privKey.PublicKey.Y = curve.ScalarBaseMult(keyBytes)
	return privKey, &privKey.PublicKey
}

// ParseSignature parses a DER-encoded signature from bytes
// Replaces btcec.ParseSignature
func ParseSignature(sig []byte) (*BTCSignature, error) {
	if len(sig) < 8 {
		return nil, errors.New("signature too short")
	}

	// DER format: 0x30 [total-length] 0x02 [R-length] [R] 0x02 [S-length] [S]
	if sig[0] != 0x30 {
		return nil, errors.New("invalid DER signature: missing SEQUENCE tag")
	}

	totalLen := int(sig[1])
	if len(sig) < totalLen+2 {
		return nil, errors.New("invalid DER signature: length mismatch")
	}

	idx := 2
	if sig[idx] != 0x02 {
		return nil, errors.New("invalid DER signature: missing INTEGER tag for R")
	}
	idx++

	rLen := int(sig[idx])
	idx++
	if idx+rLen > len(sig) {
		return nil, errors.New("invalid DER signature: R length exceeds data")
	}

	rBytes := sig[idx : idx+rLen]
	// Skip leading zero if present (used for positive numbers with high bit set)
	if rBytes[0] == 0x00 && len(rBytes) > 1 {
		rBytes = rBytes[1:]
	}
	r := new(big.Int).SetBytes(rBytes)
	idx += rLen

	if idx >= len(sig) || sig[idx] != 0x02 {
		return nil, errors.New("invalid DER signature: missing INTEGER tag for S")
	}
	idx++

	sLen := int(sig[idx])
	idx++
	if idx+sLen > len(sig) {
		return nil, errors.New("invalid DER signature: S length exceeds data")
	}

	sBytes := sig[idx : idx+sLen]
	// Skip leading zero if present
	if sBytes[0] == 0x00 && len(sBytes) > 1 {
		sBytes = sBytes[1:]
	}
	s := new(big.Int).SetBytes(sBytes)

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
// DER format: 0x30 [total-length] 0x02 [R-length] [R] 0x02 [S-length] [S]
func (sig *BTCSignature) Serialize() []byte {
	rBytes := sig.R.Bytes()
	sBytes := sig.S.Bytes()

	// Add leading zero if high bit is set (to keep number positive in DER)
	if len(rBytes) > 0 && rBytes[0]&0x80 != 0 {
		rBytes = append([]byte{0x00}, rBytes...)
	}
	if len(sBytes) > 0 && sBytes[0]&0x80 != 0 {
		sBytes = append([]byte{0x00}, sBytes...)
	}

	// Calculate total length
	totalLen := 2 + len(rBytes) + 2 + len(sBytes) // 2 for each INTEGER tag+length

	result := make([]byte, 2+totalLen)
	result[0] = 0x30 // SEQUENCE tag
	result[1] = byte(totalLen)

	idx := 2
	result[idx] = 0x02 // INTEGER tag for R
	result[idx+1] = byte(len(rBytes))
	copy(result[idx+2:], rBytes)
	idx += 2 + len(rBytes)

	result[idx] = 0x02 // INTEGER tag for S
	result[idx+1] = byte(len(sBytes))
	copy(result[idx+2:], sBytes)

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
