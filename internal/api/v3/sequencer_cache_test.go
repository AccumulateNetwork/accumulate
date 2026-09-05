// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	dbmerkle "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// With a cache, every answer the sequencer gives is built from it and nothing
// else, a miss is refused and counted, and the proofs it hands out validate
// to the anchor the block was dispatched under (healing spec, "The answer").
func TestSequencer_AnswersFromTheCache(t *testing.T) {
	// A source partition's synthetic chain with four entries, its root chain
	// anchoring it, and the Directory's chain anchoring the root: the same
	// shape dispatch proves under.
	store := memory.New(nil).Begin(nil, true)
	newChain := func(name string) *dbmerkle.Chain {
		return dbmerkle.NewChain(nil, keyvalue.RecordStore{Store: store}, record.NewKey("Chain", name), 8, dbmerkle.ChainTypeTransaction, name)
	}
	synth, root, dn := newChain("synth"), newChain("root"), newChain("dn")
	bvn1 := protocol.PartitionUrl("BVN1")
	var entries []*synthcache.Entry
	for i := uint64(1); i <= 4; i++ {
		seq := &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("alice")}, Body: &protocol.SyntheticDepositCredits{Amount: i}}},
			Source:      protocol.PartitionUrl("BVN0"),
			Destination: bvn1,
			Number:      i,
		}
		h := seq.Hash()
		require.NoError(t, synth.AddEntry(h[:], false))
		entries = append(entries, &synthcache.Entry{Stream: bvn1, Number: i, Index: int64(i - 1), Block: 7, Hash: h, Seq: seq})
	}
	var rh common.RandHash
	require.NoError(t, root.AddEntry(rh.Next(), false)) // something before the synth anchor
	synthHead, err := synth.Head().Get()
	require.NoError(t, err)
	require.NoError(t, root.AddEntry(synthHead.Anchor(), false))
	rootReceipt, err := root.Receipt(1, 1)
	require.NoError(t, err)
	rootHead, err := root.Head().Get()
	require.NoError(t, err)
	require.NoError(t, dn.AddEntry(rootHead.Anchor(), false))
	dnReceipt, err := dn.Receipt(0, 0)
	require.NoError(t, err)

	seg, err := dbmerkle.NewSegment(synth, 0)
	require.NoError(t, err)
	cache := synthcache.New(0)
	tx := cache.Begin(7)
	for _, e := range entries {
		tx.Add(e)
	}
	tx.SetBlock(&synthcache.Block{Index: 7, Segment: seg, RootReceipt: rootReceipt, Entries: entries})
	anchorTxn := &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN0"), MinorBlockIndex: 7}}}
	tx.AddAnchor(3, 7, anchorTxn)
	tx.Commit()

	_, key, _ := ed25519.GenerateKey(nil)
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	svc := NewSequencer(SequencerParams{
		Database:     database.OpenInMemory(nil),
		EventBus:     events.NewBus(nil),
		Globals:      globals,
		Partition:    "BVN0",
		ValidatorKey: key,
		Cache:        cache,
	})
	src := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)

	// Before dispatch: the entry, signed, with no proof yet
	r, err := svc.Sequence(context.Background(), src, bvn1, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Equal(t, uint64(2), r.Sequence.Number)
	require.Nil(t, r.SourceReceipt, "not dispatched yet, so no anchor to prove under")
	require.Equal(t, uint64(1), r.Signatures.Total)

	// A miss is refused and counted
	before := synthcache.Stats().Misses["entry"]
	_, err = svc.Sequence(context.Background(), src, bvn1, 9, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotFound)
	require.Equal(t, before+1, synthcache.Stats().Misses["entry"])

	// After dispatch: the proof runs through the synthetic chain, the root
	// chain and the Directory's receipt to the Directory root
	cache.MarkDispatched(7, 42, &protocol.PartitionAnchorReceipt{RootChainReceipt: dnReceipt})
	r, err = svc.Sequence(context.Background(), src, bvn1, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.NotNil(t, r.SourceReceipt)
	require.True(t, r.SourceReceipt.Validate(nil))
	require.Equal(t, entries[1].Hash[:], r.SourceReceipt.Start)
	require.Equal(t, dnReceipt.Anchor, r.SourceReceipt.Anchor)

	// A range: one list over the span, continued to the same root
	rs, err := svc.SequenceRange(context.Background(), src, bvn1, 2, 4, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, rs, 3)
	list := rs[2].SourceReceiptList
	require.NotNil(t, list)
	require.True(t, list.Validate(nil))
	require.Len(t, list.Elements, 3)
	require.Equal(t, dnReceipt.Anchor, list.ContinuedReceipt.Anchor)

	// Under a different anchor than the dispatch's, the range is not provable
	_, err = svc.SequenceRange(context.Background(), src, bvn1, 2, 4, private.SequenceOptions{ProveAgainstAnchor: 41})
	require.ErrorIs(t, err, errors.NotReady)

	// Anchors by number, shaped for the destination
	a, err := svc.Sequence(context.Background(), protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool), protocol.DnUrl(), 3, private.SequenceOptions{})
	require.NoError(t, err)
	require.True(t, protocol.DnUrl().JoinPath(protocol.AnchorPool).Equal(a.Message.(*messaging.TransactionMessage).Transaction.Header.Principal))
	_, err = svc.Sequence(context.Background(), protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool), protocol.DnUrl(), 4, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotFound)
}
