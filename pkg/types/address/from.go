// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	btc "github.com/btcsuite/btcd/btcec"
	eth "github.com/ethereum/go-ethereum/crypto"
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
		skt, err := x509.ParsePKCS8PrivateKey(key)
		if err != nil {
			panic(fmt.Errorf("invalid ed25519 key length: want 32 or 64, got %d", len(key)))
		}
		var ok bool
		key, ok = skt.(ed25519.PrivateKey)
		if !ok {
			panic("invalid ed25519 private key")
		}
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

func FromEcdsaPublicKeyAsPKIX(key *ecdsa.PublicKey) *PublicKey {
	var err error
	publicKey := new(PublicKey)
	publicKey.Type = protocol.SignatureTypeEcdsaSha256
	publicKey.Key, err = x509.MarshalPKIXPublicKey(key)
	if err != nil {
		panic(err)
	}
	return publicKey
}

func FromEcdsaPrivateKey(key *ecdsa.PrivateKey) *PrivateKey {
	var err error
	priv := new(PrivateKey)
	priv.Type = protocol.SignatureTypeEcdsaSha256
	priv.Key, err = x509.MarshalECPrivateKey(key)
	if err != nil {
		panic(err)
	}
	priv.PublicKey.Type = protocol.SignatureTypeEcdsaSha256
	priv.PublicKey.Key, err = x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		panic(err)
	}
	return priv
}

func FromETHPrivateKey(key *ecdsa.PrivateKey) *PrivateKey {
	priv := new(PrivateKey)
	priv.Type = protocol.SignatureTypeEcdsaSha256
	priv.Key = eth.FromECDSA(key)
	priv.PublicKey.Type = protocol.SignatureTypeEcdsaSha256
	priv.PublicKey.Key = eth.FromECDSAPub(&key.PublicKey)
	return priv
}

func FromPrivateKeyBytes(priv []byte, typ protocol.SignatureType) (*PrivateKey, error) {
	var pub []byte
	switch typ {
	case protocol.SignatureTypeUnknown:
		return nil, fmt.Errorf("key type must be specified")
	default:
		return nil, fmt.Errorf("unknown key type %v", typ)

	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeRCD1:

		switch len(priv) {
		case ed25519.PrivateKeySize:
			// Ok
		case ed25519.SeedSize:
			priv = ed25519.NewKeyFromSeed(priv)
		default:
			skt, err := x509.ParsePKCS8PrivateKey(priv)
			if err != nil {
				return nil, fmt.Errorf("invalid ed25519 key length: want 32 or 64, got %d", len(priv))
			}
			var ok bool
			priv, ok = skt.(ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("private key is not an edcsa key")
			}
		}
		pub = priv[32:]

	case protocol.SignatureTypeETH,
		protocol.SignatureTypeBTCLegacy,
		protocol.SignatureTypeEip712TypedData:
		_, pk := btc.PrivKeyFromBytes(btc.S256(), priv)
		pub = pk.SerializeUncompressed()

	case protocol.SignatureTypeBTC:
		_, pk := btc.PrivKeyFromBytes(btc.S256(), priv)
		pub = pk.SerializeCompressed()

	case protocol.SignatureTypeRsaSha256:
		sk, err := x509.ParsePKCS1PrivateKey(priv)
		if err != nil {
			skt, err := x509.ParsePKCS8PrivateKey(priv)
			if err != nil {
				return nil, err
			}
			var ok bool
			sk, ok = skt.(*rsa.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("private key is not an rsa key")
			}
		}
		pub = x509.MarshalPKCS1PublicKey(&sk.PublicKey)

	case protocol.SignatureTypeEcdsaSha256:
		sk, err := x509.ParseECPrivateKey(priv)
		if err != nil {
			skt, err := x509.ParsePKCS8PrivateKey(priv)
			if err != nil {
				return nil, err
			}
			var ok bool
			sk, ok = skt.(*ecdsa.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("private key is not an ecdsa key")
			}
		}
		pub, err = x509.MarshalPKIXPublicKey(&sk.PublicKey)
		if err != nil {
			return nil, err
		}
	}

	return &PrivateKey{Key: priv, PublicKey: PublicKey{Type: typ, Key: pub}}, nil
}
