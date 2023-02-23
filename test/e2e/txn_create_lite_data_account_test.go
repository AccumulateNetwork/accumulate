// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCreateLiteDataAccount(t *testing.T) {
	type Case struct {
		SignerUrl   *url.URL // URL of the page/lite id
		SignerKey   []byte   // Key of the page/lite id
		WriteDataTo bool     // Use WriteDataTo instead of WriteData
	}

	liteKey := acctesting.GenerateKey("Lite")
	liteSigner := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity()
	alice := AccountUrl("alice")
	alicePage := alice.JoinPath("book", "1")
	aliceKey := acctesting.GenerateKey(alice)

	fde := &FactomDataEntry{ExtIds: [][]byte{[]byte("Factom PRO"), []byte("Tutorial")}}
	fde.AccountId = *(*[32]byte)(ComputeLiteDataAccountId(fde.Wrap()))
	lda, err := LiteDataAddress(fde.AccountId[:])
	require.NoError(t, err)
	require.Equal(t, "acc://b36c1c4073305a41edc6353a094329c24ffa54c0a47fb56227a04477bcb78923", lda.String(), "Account ID is wrong")

	cases := map[string]Case{
		"WriteData With Page":   {alicePage, aliceKey, false},
		"WriteData With Lite":   {liteSigner, liteKey, false},
		"WriteDataTo With Page": {alicePage, aliceKey, true},
		"WriteDataTo With Lite": {liteSigner, liteKey, true},
	}

	Run(t, cases, func(t *testing.T, c Case) {
		var timestamp uint64

		// Initialize
		sim := NewSim(t,
			simulator.MemoryDatabase,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.Genesis(GenesisTime),
		)

		MakeAccount(t, sim.DatabaseFor(liteSigner), &LiteIdentity{Url: liteSigner, CreditBalance: 1e9})
		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		UpdateAccount(t, sim.DatabaseFor(alice), alicePage, func(p *KeyPage) { p.CreditBalance = 1e9 })

		var bld EnvelopeBuilder
		if c.WriteDataTo {
			bld = build.Transaction().For(c.SignerUrl).
				WriteData().Entry(fde.Wrap()).To(lda).
				SignWith(c.SignerUrl).Version(1).Timestamp(&timestamp).PrivateKey(c.SignerKey)
		} else {
			bld = build.Transaction().For(lda).
				WriteData().Entry(fde.Wrap()).
				SignWith(c.SignerUrl).Version(1).Timestamp(&timestamp).PrivateKey(c.SignerKey)
		}
		st := sim.BuildAndSubmitTxnSuccessfully(bld)
		sim.StepUntil(
			Txn(st.TxID).Succeeds())
	})
}
