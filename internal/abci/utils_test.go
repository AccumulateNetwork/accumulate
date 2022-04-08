package abci_test

import (
	"crypto/ed25519"

	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var globalNonce uint64

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func newTxn(origin string) acctesting.TransactionBuilder {
	u := url.MustParse(origin)
	tb := acctesting.NewTransaction().
		WithPrincipal(u).
		WithTimestampVar(&globalNonce)

	if key, _, _ := protocol.ParseLiteTokenAddress(u); key != nil {
		tb = tb.WithSigner(u, 1)
	}

	return tb
}
