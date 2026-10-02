// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// relayedTotal is how many requests the nodes of a partition relayed, read
// off the registry the exporter serves.
func relayedTotal(t *testing.T, partition string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	var n float64
	for _, f := range families {
		if f.GetName() != "accumulate_sequencer_relayed_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "partition" && l.GetValue() == partition {
					n += m.GetCounter().GetValue()
				}
			}
		}
	}
	return n
}

// answerSigner is the public key of the one signature a sequencer answer
// carries.
func answerSigner(t *testing.T, r *api.MessageRecord[messaging.Message]) []byte {
	t.Helper()
	require.NotNil(t, r.Signatures)
	require.NotEmpty(t, r.Signatures.Records)
	sigs := r.Signatures.Records[0].Signatures
	require.NotEmpty(t, sigs.Records)
	msg, ok := sigs.Records[0].Message.(*messaging.SignatureMessage)
	require.True(t, ok)
	key, ok := msg.Signature.(KeySignature)
	require.True(t, ok)
	return key.GetPublicKey()
}

// TestACacheMissIsRelayedToANodeThatHoldsTheEntry — #4487 (Paul,
// 2026-10-01: "A cache miss would relay the request to another fully synced
// node"). A BVN0 node whose producer cache lost a synthetic it produced is
// asked for it by name: it relays the request one hop to another node of
// BVN0, and the requester gets that node's answer, signed by that node. When
// every node of BVN0 has lost it, the request is refused, and no node passes
// it on again.
func TestACacheMissIsRelayedToANodeThatHoldsTheEntry(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	sim.StepN(10)

	part := sim.S.Partition("BVN0")
	src := PartitionUrl("BVN0").JoinPath(Synthetic)
	dst := PartitionUrl("BVN1")
	ask := func(n int) ([]*api.MessageRecord[messaging.Message], error) {
		r, ok := part.NodePrivate(n).(private.SequenceRanger)
		require.True(t, ok)
		return r.SequenceRange(context.Background(), src, dst, 1, 1, private.SequenceOptions{})
	}

	own, err := ask(1)
	require.NoError(t, err, "precondition: node 1 does not answer the synthetic from its cache")
	require.Len(t, own, 1)
	key1 := answerSigner(t, own[0])

	// Node 1's cache loses it; the others keep it.
	part.NodeSynthCache(1).Reset()
	got, err := ask(1)
	require.NoError(t, err, "node 1's miss was refused, not relayed")
	require.Len(t, got, 1)
	require.Equal(t, own[0].ID.String(), got[0].ID.String(), "the relayed answer is another message")
	require.False(t, bytes.Equal(key1, answerSigner(t, got[0])), "the relayed answer is signed by node 1, not by the node that held it")
	require.NotNil(t, got[len(got)-1].SourceReceiptList, "the relayed answer lost its proof")

	// Every node's cache loses it: refused, by node 1, as a miss.
	for i := 0; i < part.NodeCount(); i++ {
		part.NodeSynthCache(i).Reset()
	}
	before := relayedTotal(t, "bvn0")
	start := time.Now()
	_, err = ask(1)
	require.Error(t, err, "a miss on every node was answered")
	require.ErrorIs(t, err, errors.NotFound)
	require.Less(t, time.Since(start), 5*time.Second, "a miss on every node went round before it was refused")
	require.Equal(t, before+1, relayedTotal(t, "bvn0"), "a relayed request was relayed again: only the node asked relays, once")
}
