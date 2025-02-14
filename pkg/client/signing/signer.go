// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package signing

import (
	"crypto/x509"
	"fmt"

	btc "github.com/btcsuite/btcd/btcec"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Signer interface {
	SetPublicKey(protocol.Signature) error
	Sign(protocol.Signature, []byte, []byte) error
}

type PrivateKey []byte

func (k PrivateKey) SetPublicKey(sig protocol.Signature) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.ED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.RCD1Signature:
		sig.PublicKey = k[32:]

	case *protocol.BTCSignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), k)
		sig.PublicKey = pubKey.SerializeCompressed()

	case *protocol.BTCLegacySignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), k)
		sig.PublicKey = pubKey.SerializeUncompressed()

	case *protocol.ETHSignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), k)
		sig.PublicKey = pubKey.SerializeUncompressed()

	case *protocol.RsaSha256Signature:
		privKey, err := x509.ParsePKCS1PrivateKey(k)
		if err != nil {
			return err
		}
		sig.PublicKey = x509.MarshalPKCS1PublicKey(&privKey.PublicKey)

	case *protocol.EcdsaSha256Signature:
		privKey, err := x509.ParseECPrivateKey(k)
		if err != nil {
			return err
		}
		sig.PublicKey, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (k PrivateKey) Sign(sig protocol.Signature, sigMdHash, message []byte) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		protocol.SignLegacyED25519(sig, k, sigMdHash, message)

	case *protocol.ED25519Signature:
		protocol.SignED25519(sig, k, sigMdHash, message)

	case *protocol.RCD1Signature:
		protocol.SignRCD1(sig, k, sigMdHash, message)

	case *protocol.BTCSignature:
		return protocol.SignBTC(sig, k, sigMdHash, message)

	case *protocol.BTCLegacySignature:
		return protocol.SignBTCLegacy(sig, k, sigMdHash, message)

	case *protocol.ETHSignature:
		return protocol.SignETH(sig, k, sigMdHash, message)

	case *protocol.RsaSha256Signature:
		return protocol.SignRsaSha256(sig, k, sigMdHash, message)

	case *protocol.EcdsaSha256Signature:
		return protocol.SignEcdsaSha256(sig, k, sigMdHash, message)

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
}
