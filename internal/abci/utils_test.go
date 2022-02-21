package abci_test

import (
	"crypto/ed25519"

	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var globalNonce uint64

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func edSigner(key tmed25519.PrivKey) func(nonce uint64, hash []byte) (protocol.Signature, error) {
	return func(nonce uint64, hash []byte) (protocol.Signature, error) {
		sig := new(protocol.LegacyED25519Signature)
		return sig, sig.Sign(nonce, key, hash)
	}
}

func newTxn(origin string) acctesting.TransactionBuilder {
	return acctesting.NewTransaction().
		WithOriginStr(origin).
		WithKeyPage(0, 1).
		WithNonceVar(&globalNonce)
}
