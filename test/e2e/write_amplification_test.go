// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestUserTransactionWrites counts what the source and the destination commit
// for one cross-partition transaction (#4236): the body of the synthetic
// deposit is stored once at the destination and every wrapper refers to it;
// the producer's Produced set is one record; the source keeps no status for
// the sequenced message it produced.
func TestUserTransactionWrites(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	rec := newRecordingStores()
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.Genesis(GenesisTime),
		simulator.WithDatabase(rec.open),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(10))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	sim.StepN(5)

	src, dst := rec.get("BVN0", 0), rec.get("BVN1", 0)
	src.reset()
	dst.reset()

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
	sim.StepN(10)

	// The synthetic deposit the transaction produced
	txnHash := st.TxID.Hash()
	var synth [32]byte
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		produced, err := batch.Transaction(txnHash[:]).Produced().Get()
		require.NoError(t, err)
		require.Len(t, produced, 1)
		synth = produced[0].Hash()
	})

	// The source: one Produced record, under the transaction; no status for
	// the sequenced message it produced
	var producedRecords, seqStatuses int
	var seqHashes [][32]byte
	for _, w := range src.all() {
		switch {
		case isRecord(w.key, "Transaction", txnHash, "Produced"):
			producedRecords++
		case isRecord(w.key, "Message", txnHash, "Produced"):
			t.Errorf("Produced written under the Message key as well")
		case w.key.Len() == 3 && w.key.Get(0) == "Message" && w.key.Get(2) == "Main":
			if seq, ok := decodeMessage(w.value).(*messaging.SequencedMessage); ok && seq.Message.Hash() == synth {
				seqHashes = append(seqHashes, w.key.Get(1).([32]byte))
			}
		}
	}
	for _, w := range src.all() {
		for _, h := range seqHashes {
			if isRecord(w.key, "Transaction", h, "Status") {
				seqStatuses++
			}
		}
	}
	require.Equal(t, 1, producedRecords, "the producer's Produced set is one record")
	require.NotEmpty(t, seqHashes, "the source stores the sequenced message it produced")
	require.Zero(t, seqStatuses, "the source keeps no status for a sequenced message it produced")

	// The destination: the deposit's body once, wrappers as references
	var bodies, wrappers int
	for _, w := range dst.all() {
		if w.key.Len() != 3 || w.key.Get(0) != "Message" || w.key.Get(2) != "Main" {
			continue
		}
		switch msg := decodeMessage(w.value).(type) {
		case *messaging.TransactionMessage:
			if msg.Hash() == synth {
				bodies++
			}
		case *messaging.SequencedMessage, *messaging.SyntheticMessage, *messaging.BadSyntheticMessage:
			txn, ok := messaging.UnwrapAs[*messaging.TransactionMessage](msg)
			if !ok || txn.Hash() != synth {
				continue
			}
			wrappers++
			require.Equal(t, TransactionTypeRemote, txn.Transaction.Body.Type(), "a wrapper stored after its transaction refers to it by hash")
		}
	}
	require.Equal(t, 1, bodies, "the synthetic transaction's body is stored once")
	require.GreaterOrEqual(t, wrappers, 1, "the sequenced wrapper is stored")

	// Records committed at the destination that name the deposit, by kind
	kinds := map[string]int{}
	for _, w := range dst.all() {
		for i := 0; i < w.key.Len(); i++ {
			if h, ok := w.key.Get(i).([32]byte); ok && h == synth {
				kinds[w.key.Get(0).(string)+"."+w.key.Get(w.key.Len()-1).(string)]++
				break
			}
		}
	}
	t.Logf("destination records naming the deposit: %v (total committed at the destination in the window: %d)", kinds, len(dst.all()))
	total := 0
	for _, n := range kinds {
		total += n
	}
	require.LessOrEqual(t, total, 6, "records at the destination keyed by the deposit's hash")
}

func isRecord(k *record.Key, parts ...any) bool {
	if k.Len() != len(parts) {
		return false
	}
	for i, p := range parts {
		if k.Get(i) != p {
			return false
		}
	}
	return true
}

func decodeMessage(b []byte) messaging.Message {
	msg, err := messaging.UnmarshalMessage(b)
	if err != nil {
		return nil
	}
	return msg
}
