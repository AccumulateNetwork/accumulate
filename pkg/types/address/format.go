// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
)

// FormatAC1 formats the hash of an Accumulate public key as an Accumulate AC1
// address. (ed25519)
func FormatAC1(hash []byte) string {
	return format2(hash, "AC1")
}

// FormatAS1 formats an Accumulate private key as an Accumulate AS1 address. (ed25519)
func FormatAS1(seed []byte) string {
	return format2(seed, "AS1")
}

// FormatAC2 formats the hash of an Accumulate public key as an Accumulate AC2 (ecdsa)
// address.
func FormatAC2(hash []byte) string {
	return format2(hash, "AC2")
}

// FormatAS2 formats an Accumulate private key as an Accumulate AS2 ecdsa address.
func FormatAS2(seed []byte) string {
	return format2(seed, "AS2")
}

// FormatAC3 formats the hash of an Accumulate public key as an Accumulate AC3 (rsa)
// address.
func FormatAC3(hash []byte) string {
	return format2(hash, "AC3")
}

// FormatAS3 formats an Accumulate private key as an Accumulate AS3 rsa address.
func FormatAS3(seed []byte) string {
	return format2(seed, "AS3")
}

// FormatFA formats the hash of a Factom public key as a Factom FA address.
func FormatFA(hash []byte) string {
	return format1(hash, 0x5f, 0xb1)
}

// FormatFs formats a Factom private key as a Factom Fs address.
func FormatFs(seed []byte) string {
	return format1(seed, 0x64, 0x78)
}

// FormatBTC formats the hash of a Bitcoin public key as a P2PKH address,
// additionally prefixed with 'BT'.
func FormatBTC(hash []byte) string {
	return "BT" + format1(hash, 0x00)
}

// FormatETH formats the hash of an Ethereum public key as an Ethereum address.
func FormatETH(hash []byte) string {
	// Take the last 20 bytes
	if len(hash) > 20 {
		hash = hash[len(hash)-20:]
	}
	return "0x" + hex.EncodeToString(hash)
}

// FormatMH formats an unknown hash as a multihash Accumulate address.
func FormatMH(hash []byte, code uint64) string {
	// Encode with multihash
	b, _ := multihash.Encode(hash, code)

	// Calculate a checksum
	c := make([]byte, len(b)+2)
	copy(c, "MH")
	copy(c[2:], b)
	checksum := sha256.Sum256(c)
	checksum = sha256.Sum256(checksum[:])

	// Encode with multibase base58
	b = append(b, checksum[:4]...)
	s, _ := multibase.Encode(multibase.Base58BTC, b)
	return "MH" + s
}

func format1(hash []byte, prefix ...byte) string {
	// Add the prefix
	b := make([]byte, len(prefix)+len(hash)+4)
	n := copy(b, prefix)
	n += copy(b[n:], hash)

	// Add the checksum
	checksum := sha256.Sum256(b[:n])
	checksum = sha256.Sum256(checksum[:])
	copy(b[n:], checksum[:4])

	// Encode
	return base58.Encode(b)
}

func format2(hash []byte, prefix string) string {
	// Add the prefix
	b := make([]byte, len(prefix)+len(hash)+4)
	n := copy(b, prefix)
	n += copy(b[n:], hash)

	// Add the checksum
	checksum := sha256.Sum256(b[:n])
	checksum = sha256.Sum256(checksum[:])
	copy(b[n:], checksum[:4])

	// Encode the hash and checksum and add the prefix
	return prefix + base58.Encode(b[len(prefix):])
}
