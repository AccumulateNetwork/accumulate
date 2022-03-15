package abci_test

import (
	"crypto/ed25519"

	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
)

var globalNonce uint64

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func newTxn(origin string) acctesting.TransactionBuilder {
	return acctesting.NewTransaction().
		WithOriginStr(origin).
		WithKeyPage(0, 1).
		WithNonceVar(&globalNonce)
}
