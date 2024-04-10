// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	btc "github.com/btcsuite/btcd/btcec"
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
	switch len(key) {
	default:
		panic("invalid ed25519 private key")
	case ed25519.SeedSize:
		key = ed25519.NewKeyFromSeed(key)
	case ed25519.PrivateKeySize:
		// Ok
	}
	return &PrivateKey{
		PublicKey: *FromED25519PublicKey(key[32:]),
		Key:       key,
	}
}

func FromRSAPublicKey(key *rsa.PublicKey) *PublicKey {
	return &PublicKey{
		Type: protocol.SignatureTypeRsaSha256,
		Key:  x509.MarshalPKCS1PublicKey(key),
	}
}

func FromRSAPrivateKey(key *rsa.PrivateKey) *PrivateKey {
	return &PrivateKey{
		PublicKey: *FromRSAPublicKey(&key.PublicKey),
		Key:       x509.MarshalPKCS1PrivateKey(key),
	}
}

func FromPrivateKeyBytes(priv []byte, typ protocol.SignatureType) *PrivateKey {
	var pub []byte
	switch typ {
	case protocol.SignatureTypeUnknown:
		panic("key type must be specified")
	default:
		panic(fmt.Errorf("unknown key type %v", typ))

	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeRCD1:

		switch len(priv) {
		case ed25519.PrivateKeySize:
			// Ok
		case ed25519.SeedSize:
			priv = ed25519.NewKeyFromSeed(priv)
		default:
			panic(fmt.Errorf("invalid ed25519 key length: want 32 or 64, got %d", len(priv)))
		}
		pub = priv[32:]

	case protocol.SignatureTypeETH:
		_, pk := btc.PrivKeyFromBytes(btc.S256(), priv)
		pub = pk.SerializeUncompressed()

	case protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy:
		_, pk := btc.PrivKeyFromBytes(btc.S256(), priv)
		pub = pk.SerializeCompressed()

	case protocol.SignatureTypeRsaSha256:
		sk, err := x509.ParsePKCS1PrivateKey(priv)
		if err != nil {
			panic(err)
		}
		pub = x509.MarshalPKCS1PublicKey(&sk.PublicKey)
	}

	return &PrivateKey{Key: priv, PublicKey: PublicKey{Type: typ, Key: pub}}
}
