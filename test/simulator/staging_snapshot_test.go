// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// stagingShape is what a stage says about itself, in a form two nodes can be
// compared by without comparing pointers.
type stagingShape struct {
	Stream    string
	Delivered uint64
	Sighted   uint64
	Reach     uint64
	Held      int
	Entries   map[uint64]string
	Validated map[uint64][32]byte
}

func shapeOf(t testing.TB, s *execute.Staging) []stagingShape {
	t.Helper()
	tx := s.Begin()
	defer tx.Discard()
	var out []stagingShape
	for _, st := range tx.Streams() {
		shape := stagingShape{
			Stream:    st.ID.Ledger.String() + "|" + st.ID.Source.String(),
			Delivered: st.Delivered,
			Sighted:   st.Sighted,
			Reach:     st.Reach,
			Held:      st.Held,
			Entries:   map[uint64]string{},
			Validated: map[uint64][32]byte{},
		}
		for n := st.Delivered + 1; n <= st.Delivered+1024; n++ {
			if h, ok := tx.IDOf(st.ID, n); ok {
				shape.Entries[n] = fmt.Sprintf("%v/%v/%x", h.ID, h.Collected, h.Hash[:4])
			}
			if v, ok := tx.Validated(st.ID, n); ok {
				shape.Validated[n] = v
			}
		}
		out = append(out, shape)
	}
	return out
}

// A running validator serves its staging as of its last committed block, and
// what it serves is what it holds: a node that loads the snapshot stands
// where the validator stands (executor spec, "Sync" step 2).
func TestStagingSnapshotIsWhatTheNodeHolds(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	// Directory anchors are held back from BVN1, so what it receives from
	// BVN0 is collected and held unproven on every one of its nodes: a stage
	// with something in it is the only thing worth snapshotting.
	var hookMu sync.Mutex
	var hold atomic.Bool
	delayed := map[int][]*messaging.Envelope{}
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		hookMu.Lock()
		defer hookMu.Unlock()
		if !hold.Load() {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			isAnchor := false
			for _, m := range env.Messages {
				blk, ok := m.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if seq, ok := blk.Anchor.(*messaging.SequencedMessage); ok {
					if txn, ok := seq.Message.(*messaging.TransactionMessage); ok && txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
						isAnchor = true
					}
				}
			}
			if isAnchor {
				delayed[node] = append(delayed[node], env)
				continue
			}
			kept = append(kept, env)
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}

	hold.Store(true)
	for i := uint64(1); i <= 10; i++ {
		send(i)
		sim.Step()
	}
	sim.StepN(30)

	p := sim.S.Partition("BVN1")
	held := 0
	{
		tx := p.NodeStaging(0).Begin()
		for _, st := range tx.Streams() {
			held += st.Held
		}
		tx.Discard()
	}
	require.Greater(t, held, 0, "precondition: BVN1's nodes hold unproven entries")

	// What node 0 serves through the private API is what node 0 holds.
	client, ok := p.NodePrivate(0).(private.StagingSnapshotter)
	require.True(t, ok, "the node's private API serves staging")
	snap, err := private.FetchStagingSnapshot(context.Background(), client, "BVN1")
	require.NoError(t, err)
	require.Equal(t, sim.S.BlockIndex("BVN1"), snap.Block, "the snapshot is as of the last committed block")

	loaded := execute.NewStaging()
	require.NoError(t, loaded.Load(snap))
	require.Equal(t, shapeOf(t, p.NodeStaging(0)), shapeOf(t, loaded))

	// And a partition this node does not serve is refused.
	_, err = client.StagingSnapshot(context.Background(), &private.StagingSnapshotRequest{Partition: "BVN0"})
	require.Error(t, err)
}
