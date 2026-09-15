// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// A transaction appends its hash to its principal's chain once, whatever its
// type and whichever code path writes the principal (database spec,
// "Duplicates are caught at entry"). The state cache appends for every
// account it writes and the success path appends for the principal; when both
// name the same chain, the transaction's own chain-update record settles it,
// not a read of the chain. Each case: the target chain grows by exactly one
// and holds the hash exactly once.
func TestTransactionAppendsToItsPrincipalOnce(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e12).Identity().
		Tokens("tokens2").Create("ACME").Identity().
		Data("data").Create().Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().
		Page(2).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).GenerateKey(SignatureTypeED25519)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime).With(alice),
	)

	type chainRef struct {
		account *url.URL
		scratch bool
	}
	chainOf := func(batch *database.Batch, c chainRef) *database.Chain {
		acct := batch.Account(c.account)
		var ch *database.Chain2
		if c.scratch {
			ch = acct.ScratchChain()
		} else {
			ch = acct.MainChain()
		}
		got, err := ch.Get()
		require.NoError(t, err)
		return got
	}
	height := func(c chainRef) int64 {
		var h int64
		require.NoError(t, sim.DatabaseFor(c.account).View(func(batch *database.Batch) error {
			h = chainOf(batch, c).Height()
			return nil
		}))
		return h
	}
	occurrences := func(c chainRef, hash []byte) int {
		n := 0
		require.NoError(t, sim.DatabaseFor(c.account).View(func(batch *database.Batch) error {
			ch := chainOf(batch, c)
			entries, err := ch.Entries(0, ch.Height())
			require.NoError(t, err)
			for _, e := range entries {
				if bytes.Equal(e, hash) {
					n++
				}
			}
			return nil
		}))
		return n
	}

	var timestamp uint64
	type signable interface {
		SignWith(signer any, path ...string) build.SignatureBuilder
	}
	sign := func(b signable) build.SignatureBuilder {
		timestamp++
		return b.SignWith(alice, "book", "1").Version(1).Timestamp(timestamp).PrivateKey(aliceKey)
	}
	cases := []struct {
		name  string
		chain chainRef
		txn   func() build.SignatureBuilder
	}{
		{"SendTokens", chainRef{alice.Url().JoinPath("tokens"), false}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "tokens").SendTokens(1, 0).To(alice, "tokens2"))
		}},
		{"AddCredits", chainRef{alice.Url().JoinPath("tokens"), false}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "tokens").AddCredits().Spend(10).To(alice, "book", "1").WithOracle(InitialAcmeOracle))
		}},
		{"BurnTokens", chainRef{alice.Url().JoinPath("tokens"), false}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "tokens").BurnTokens(1, 0))
		}},
		{"WriteData to state", chainRef{alice.Url().JoinPath("data"), false}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "data").WriteData().DoubleHash("hello").ToState())
		}},
		{"WriteData scratch", chainRef{alice.Url().JoinPath("data"), true}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "data").WriteData().DoubleHash("scratch").Scratch())
		}},
		{"TransferCredits to another page", chainRef{alice.Url().JoinPath("book", "1"), true}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "book", "1").TransferCredits(10).To(alice, "book", "2"))
		}},
		{"TransferCredits to itself", chainRef{alice.Url().JoinPath("book", "1"), true}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice, "book", "1").TransferCredits(10).To(alice, "book", "1"))
		}},
		{"CreateTokenAccount", chainRef{alice.Url(), false}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice).CreateTokenAccount(alice, "tokens3").ForToken(AcmeUrl()))
		}},
		{"CreateDataAccount", chainRef{alice.Url(), false}, func() build.SignatureBuilder {
			return sign(build.Transaction().For(alice).CreateDataAccount(alice, "data2"))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := height(c.chain)
			st := sim.BuildAndSubmitTxnSuccessfully(c.txn())
			sim.StepUntil(Txn(st.TxID).Succeeds())
			require.Equal(t, before+1, height(c.chain), "the principal's chain grows by exactly one")
			h := st.TxID.Hash()
			require.Equal(t, 1, occurrences(c.chain, h[:]), "the hash appears exactly once")
		})
	}
}
