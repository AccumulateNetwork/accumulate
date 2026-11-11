// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// updateKeySlotCases lists, per executor version, whether UpdateKey keeps the
// entry it updates at its index on the page. A signature on a pending
// transaction is recorded against the index of its key's entry, and UpdateKey
// does not change the page's version, so the entries must keep their indices.
// Before the version below, UpdateKey moves the entry to the position its new
// key hash sorts to; the replay of history must not change.
var updateKeySlotCases = []struct {
	Version ExecutorVersion
	InPlace bool
}{
	{ExecutorVersionV2Kourou, false},
	{updateKeySlotVersion, true},
}

// updateKeySlotVersion is the first executor version at which UpdateKey keeps
// the entry in place. It must match the gate in the v2 chain executor.
const updateKeySlotVersion = ExecutorVersionV2Tanegashima

// updateKeySlotKeys returns four keys ordered by key hash, so that the first
// key sorts first on a page holding the first three and the fourth sorts after
// all of them.
func updateKeySlotKeys() []ed25519.PrivateKey {
	alice := AccountUrl("alice")
	keys := make([]ed25519.PrivateKey, 4)
	for i := range keys {
		keys[i] = acctesting.GenerateKey(alice, "slot", i)
	}
	hash := func(k ed25519.PrivateKey) []byte { h := sha256.Sum256(k[32:]); return h[:] }
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(hash(keys[i]), hash(keys[j])) < 0 })
	return keys
}

// setupUpdateKeySlot creates alice with a 2-of-3 page holding keys A, B and C,
// and a data account she controls. It initiates a write to the data account
// signed by A, then has A replace its key with A2, whose hash sorts after B
// and C. It returns the simulator, the pending write, and the keys.
func setupUpdateKeySlot(t *testing.T, version ExecutorVersion, inPlace bool) (sim *Sim, txid *url.TxID, keyA2, keyB ed25519.PrivateKey, timestamp *uint64) {
	alice := AccountUrl("alice")
	page := alice.JoinPath("book", "1")
	keys := updateKeySlotKeys()
	keyA, keyB, keyC, keyA2 := keys[0], keys[1], keys[2], keys[3]
	timestamp = new(uint64)

	sim = NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWithVersion(GenesisTime, version),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, keyA[32:], keyB[32:], keyC[32:])
	CreditCredits(t, sim.DatabaseFor(alice), page, 1e9)
	UpdateAccount(t, sim.DatabaseFor(alice), page, func(p *KeyPage) { p.AcceptThreshold = 2 })
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{
		Url:         alice.JoinPath("data"),
		AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}}},
	})

	kp := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), page)
	i, _, _ := kp.EntryByKey(keyA[32:])
	require.Equal(t, 0, i, "A must hold index 0")

	// A initiates a write; it is pending on the 2-of-3 page
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("foo")).
			SignWith(page).Version(1).Timestamp(timestamp).PrivateKey(keyA))
	sim.StepUntil(
		Txn(st.TxID).IsPending())
	id := st.TxID

	// A replaces its own key with A2
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(page).
			UpdateKey(keyA2, SignatureTypeED25519).
			SignWith(page).Version(1).Timestamp(timestamp).PrivateKey(keyA))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// UpdateKey does not change the page's version. From the gate the entry
	// keeps index 0; before it the entry moves to the end and B and C move up.
	kp = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), page)
	require.Equal(t, uint64(1), kp.Version)
	i, _, ok := kp.EntryByKey(keyA2[32:])
	require.True(t, ok, "A2 must be on the page")
	j, _, _ := kp.EntryByKey(keyB[32:])
	if inPlace {
		assert.Equal(t, 0, i, "A2 must take A's index")
		assert.Equal(t, 1, j, "B must keep its index")
	} else {
		assert.Equal(t, 2, i, "A2 sorts last")
		assert.Equal(t, 0, j, "B moves to index 0")
	}

	return sim, id, keyA2, keyB, timestamp
}

// TestUpdateKeyDoubleSign: A signs a pending 2-of-3 transaction, replaces its
// key with UpdateKey, and signs again with the new key. Before the gate the
// new key's entry sits at a different index, so the second signature fills a
// second slot and A alone executes the transaction. From the gate the second
// signature replaces A's first, the transaction stays pending, and it executes
// once B signs.
func TestUpdateKeyDoubleSign(t *testing.T) {
	page := AccountUrl("alice").JoinPath("book", "1")
	for _, c := range updateKeySlotCases {
		t.Run(c.Version.String(), func(t *testing.T) {
			sim, txid, keyA2, keyB, timestamp := setupUpdateKeySlot(t, c.Version, c.InPlace)

			// A signs again with its new key
			st := sim.BuildAndSubmitSuccessfully(
				build.SignatureForTxID(txid).Load(sim.Query()).
					Url(page).Version(1).Timestamp(timestamp).PrivateKey(keyA2))
			sim.StepUntil(
				Sig(st[1].TxID).Succeeds())

			if !c.InPlace {
				// Before the gate, A's two signatures meet the threshold
				sim.StepUntil(
					Txn(txid).Succeeds())
				return
			}

			// From the gate, A holds one slot; the write must not execute
			sim.StepN(20)
			require.Equal(t, errors.Pending, sim.QueryTransaction(txid, nil).Status, "A alone must not execute a 2-of-3 transaction")

			// B's signature completes it
			sim.BuildAndSubmitSuccessfully(
				build.SignatureForTxID(txid).Load(sim.Query()).
					Url(page).Version(1).Timestamp(timestamp).PrivateKey(keyB))
			sim.StepUntil(
				Txn(txid).Succeeds())
		})
	}
}

// TestUpdateKeySlotOverwrite: A signs a pending 2-of-3 transaction and
// replaces its key with UpdateKey; then B signs. Before the gate B's entry has
// moved to A's old index, so B's signature overwrites A's and the transaction
// stays pending with two holders having signed. From the gate B's signature
// fills its own slot and the transaction executes.
func TestUpdateKeySlotOverwrite(t *testing.T) {
	page := AccountUrl("alice").JoinPath("book", "1")
	for _, c := range updateKeySlotCases {
		t.Run(c.Version.String(), func(t *testing.T) {
			sim, txid, _, keyB, timestamp := setupUpdateKeySlot(t, c.Version, c.InPlace)

			st := sim.BuildAndSubmitSuccessfully(
				build.SignatureForTxID(txid).Load(sim.Query()).
					Url(page).Version(1).Timestamp(timestamp).PrivateKey(keyB))
			sim.StepUntil(
				Sig(st[1].TxID).Succeeds())

			if c.InPlace {
				sim.StepUntil(
					Txn(txid).Succeeds())
				return
			}

			// Before the gate, B overwrote A's slot
			sim.StepN(20)
			require.Equal(t, errors.Pending, sim.QueryTransaction(txid, nil).Status)
		})
	}
}
