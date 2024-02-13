// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func FromED25519PublicKey(key []byte) *PublicKey {
	if len(key) != ed25519.PublicKeySize {
		panic("invalid ed25519 public key")
	}
	return &PublicKey{
		Type: protocol.SignatureTypeED25519,
		Key:  key,
	}
}

func FromED25519PrivateKey(key []byte) *PrivateKey {
	if len(key) != ed25519.PrivateKeySize {
		panic("invalid ed25519 private key")
	}
	return &PrivateKey{
		PublicKey: *FromED25519PublicKey(key[32:]),
		Key:       key,
	}
}
