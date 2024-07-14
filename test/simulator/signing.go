// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Simulator) SignWithNode(partition string, i int) nodeSigner {
	return nodeSigner(s.partitions[partition].nodes[i].network.PrivValKey)
}

type nodeSigner []byte

var _ signing.Signer = nodeSigner{}

func (n nodeSigner) Key() []byte { return n }

func (n nodeSigner) SetPublicKey(sig protocol.Signature) error {
	k := n
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.ED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.RCD1Signature:
		sig.PublicKey = k[32:]

	case *protocol.BTCSignature:
		_, pubKey := btcec.PrivKeyFromBytes(btcec.S256(), k)
		sig.PublicKey = pubKey.SerializeCompressed()

	case *protocol.BTCLegacySignature:
		_, pubKey := btcec.PrivKeyFromBytes(btcec.S256(), k)
		sig.PublicKey = pubKey.SerializeUncompressed()

	case *protocol.ETHSignature:
		_, pubKey := btcec.PrivKeyFromBytes(btcec.S256(), k)
		sig.PublicKey = pubKey.SerializeUncompressed()

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (n nodeSigner) Sign(sig protocol.Signature, sigMdHash, message []byte) error {
	k := n
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

	// case *protocol.Eip712TypedDataSignature:
	// 	return protocol.SignEip712TypedData(sig, k, sigMdHash, message)

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
}
