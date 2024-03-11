// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

const AcmeFaucetAmount = 10
const AcmeFaucetBalance = 200_000_000

var faucetSeed = sha256.Sum256([]byte("faucet"))
var faucetKey = ed25519.NewKeyFromSeed(faucetSeed[:])

var Faucet faucet
var FaucetUrl = LiteAuthorityForKey(Faucet.PublicKey(), SignatureTypeED25519).JoinPath("/ACME")

// TODO Set the balance to 0 and/or use a bogus URL for the faucet. Otherwise, a
// bad actor could generate the faucet private key using the same method we do,
// then sign arbitrary transactions using the faucet.

type faucet struct{}

func (faucet) Url() *url.URL {
	return FaucetUrl
}

func (faucet) PublicKey() []byte {
	return faucetKey[32:]
}

func (faucet) Signer() faucetSigner {
	return faucetSigner(time.Now().UnixNano())
}

type faucetSigner uint64

func (s faucetSigner) Timestamp() uint64 {
	return uint64(s)
}

func (s faucetSigner) Version() uint64 {
	return 1
}

func (s faucetSigner) SetPublicKey(sig Signature) error {
	switch sig := sig.(type) {
	case *LegacyED25519Signature:
		sig.PublicKey = faucetKey[32:]

	case *ED25519Signature:
		sig.PublicKey = faucetKey[32:]

	case *RCD1Signature:
		sig.PublicKey = faucetKey[32:]

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (s faucetSigner) Sign(sig Signature, sigMdHash, message []byte) error {
	switch sig := sig.(type) {
	case *LegacyED25519Signature:
		SignLegacyED25519(sig, faucetKey, sigMdHash, message)

	case *ED25519Signature:
		SignED25519(sig, faucetKey, sigMdHash, message)

	case *RCD1Signature:
		SignRCD1(sig, faucetKey, sigMdHash, message)

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
}
