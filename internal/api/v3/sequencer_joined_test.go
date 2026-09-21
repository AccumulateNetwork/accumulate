// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	dbmerkle "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A NODE THAT JOINED REFUSES PER REQUEST, NAMING THE BLOCK IT JOINED AT, AND
// NEVER SAYS NotFound (#4295, executor.md "Sync" step 6).
//
// A node that joined executed no block at or below Q, so it produced none of
// their synthetics and none of their anchors, and there is no backfill on
// this line: it never will. NotFound from it is a lie with a consequence —
// the requester counts a miss, and a run of misses strands a stream for good
// (healing spec, "Stranded streams"). It answers NotReady naming Q, which
// means "ask a node that executed them".
//
// This is per request, not per node: the same node answers for everything it
// DID produce, which after a join is most of what healing asks it for. A
// single state bit cannot express that, which is why the gate is here and not
// on nodestate (#4368 builder's statement, §3).
//
// AND IT IS BOUNDED BY WHAT THE STREAM HAD PRODUCED BY THEN. "Not from me"
// applies to the numbers that were produced before the join and to no
// others: everything above them this node produced itself, so its absence is
// a real miss and must say NotFound and be counted, or a node that has ever
// restarted — which is every node — reports no miss for the rest of its
// life, and healing.md "Stranded streams" has nothing left to show (threat
// review, finding 2).
//
// BY HAND: the cache's join mark. In production Cache.JoinedAt is set by the
// executor at the settle (collect_block.go, "Staging settled at the block the
// state is") with the counts read out of the pulled synthetic ledger and the
// anchor sequence chain; here they are passed directly, and the entries above
// and below are built the way TestSequencer_AnswersFromTheCache builds them.
// Everything else — the Sequencer, the cache, the answers — is the
// production code.
func TestSequencer_AJoinedNodeNamesItsJoinBlockAndNeverSaysNotFound(t *testing.T) {
	const joinBlock = 10
	const heldBlock = 20 // a block this node executed, after it joined

	ctx := context.Background()
	bvn1 := protocol.PartitionUrl("BVN1")
	src := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)
	anchorSrc := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	// One synthetic for BVN1, number 5, produced in a block this node
	// executed. Numbers 1..4 belong to blocks at or below Q: this node holds
	// nothing of them and never will.
	store := memory.New(nil).Begin(nil, true)
	newChain := func(name string) *dbmerkle.Chain {
		return dbmerkle.NewChain(nil, keyvalue.RecordStore{Store: store}, record.NewKey("Chain", name), 8, dbmerkle.ChainTypeTransaction, name)
	}
	synth, root, dn := newChain("synth"), newChain("root"), newChain("dn")
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("alice")}, Body: &protocol.SyntheticDepositCredits{Amount: 5}}},
		Source:      protocol.PartitionUrl("BVN0"),
		Destination: bvn1,
		Number:      5,
	}
	hash := seq.Hash()
	require.NoError(t, synth.AddEntry(hash[:], false))
	seg, err := dbmerkle.NewSegment(synth, 0)
	require.NoError(t, err)
	entry := &synthcache.Entry{Stream: bvn1, Number: 5, Index: 0, Block: heldBlock, Hash: hash, Seq: seq}

	// The proof the range answer carries: the synthetic chain anchored into
	// this partition's root chain, and that into the Directory's — the same
	// shape dispatch proves under.
	synthHead, err := synth.Head().Get()
	require.NoError(t, err)
	require.NoError(t, root.AddEntry(synthHead.Anchor(), false))
	rootReceipt, err := root.Receipt(0, 0)
	require.NoError(t, err)
	rootHead, err := root.Head().Get()
	require.NoError(t, err)
	require.NoError(t, dn.AddEntry(rootHead.Anchor(), false))
	dnReceipt, err := dn.Receipt(0, 0)
	require.NoError(t, err)

	cache := synthcache.New(0)
	tx := cache.Begin(heldBlock)
	tx.Add(entry)
	tx.SetBlock(&synthcache.Block{Index: heldBlock, Streams: map[string]*synthcache.Stream{
		strings.ToLower(bvn1.String()): {Destination: bvn1, ChainName: "synthetic(bvn1)", Segment: seg, RootReceipt: rootReceipt},
	}, Entries: []*synthcache.Entry{entry}})
	tx.AddAnchor(3, heldBlock, &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN0"), MinorBlockIndex: heldBlock}}})
	tx.Commit()
	// Dispatched, and out of the in-flight window, so what it holds is
	// servable rather than on its way.
	cache.MarkDispatched(heldBlock, 42, &protocol.PartitionAnchorReceipt{RootChainReceipt: dnReceipt})
	cache.Begin(heldBlock + synthcache.InFlightBlocks).Commit()

	// The join mark: nothing at or below block 10 was this node's to produce,
	// and by then the stream to BVN1 had produced 4 and this partition had
	// produced 2 anchors. 5 and 3 are therefore this node's own.
	cache.JoinedAt(joinBlock, map[string]uint64{strings.ToLower(bvn1.String()): 4}, 2)

	// This node's own ledger says the stream has reached six: four before the
	// join, then 5 (held) and 6 (produced by this node and lost). Without it
	// "not produced yet" would answer first and the join mark would never be
	// reached.
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = src
	ledger.Partition(bvn1).Produced = 6
	require.NoError(t, batch.Account(src).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	_, key, _ := ed25519.GenerateKey(nil)
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	svc := NewSequencer(SequencerParams{
		Database: db, EventBus: events.NewBus(nil), Globals: globals,
		Partition: "BVN0", ValidatorKey: key, Cache: cache,
	})

	// --- below the join block: NotReady, naming it, and not a miss --------
	misses := func() uint64 { return synthcache.Stats().Misses["entry"] }
	anchorMisses := func() uint64 { return synthcache.Stats().Misses["anchor"] }

	for _, c := range []struct {
		name string
		call func() error
	}{
		{"Sequence", func() error {
			_, err := svc.Sequence(ctx, src, bvn1, 1, private.SequenceOptions{})
			return err
		}},
		{"SequenceRange", func() error {
			_, err := svc.SequenceRange(ctx, src, bvn1, 1, 4, private.SequenceOptions{})
			return err
		}},
		{"Sequence(anchor)", func() error {
			_, err := svc.Sequence(ctx, anchorSrc, protocol.DnUrl(), 1, private.SequenceOptions{})
			return err
		}},
		{"SequenceRange(anchor)", func() error {
			_, err := svc.SequenceRange(ctx, anchorSrc, protocol.DnUrl(), 1, 2, private.SequenceOptions{})
			return err
		}},
	} {
		before, beforeAnchor := misses(), anchorMisses()
		err := c.call()
		require.Error(t, err, "%s answered for a block this node did not execute", c.name)
		require.False(t, errors.Is(err, errors.NotFound),
			"%s said NotFound for a block it did not execute, which a requester counts as a miss: %v", c.name, err)
		require.True(t, errors.Is(err, errors.NotReady), "%s: got %v", c.name, err)
		require.Contains(t, err.Error(), "joined at block 10",
			"%s must name the block it joined at, so the asker knows who to ask", c.name)
		require.Equal(t, before, misses(), "%s counted a miss for a block it never produced", c.name)
		require.Equal(t, beforeAnchor, anchorMisses(), "%s counted an anchor miss for a block it never produced", c.name)
	}

	// --- above the join block, and LOST: a real miss, counted -------------
	//
	// Entry 6 and anchor 4 are numbers this node produced itself, after it
	// joined. The cache does not hold them, and that is a defect of this
	// node's — the one thing a requester must be told, because a stream no
	// source can fill is a stranded stream and nothing else reports it.
	for _, c := range []struct {
		name string
		kind string
		call func() error
	}{
		{"Sequence", "entry", func() error {
			_, err := svc.Sequence(ctx, src, bvn1, 6, private.SequenceOptions{})
			return err
		}},
		{"SequenceRange", "entry", func() error {
			_, err := svc.SequenceRange(ctx, src, bvn1, 6, 6, private.SequenceOptions{})
			return err
		}},
		{"Sequence(anchor)", "anchor", func() error {
			_, err := svc.Sequence(ctx, anchorSrc, protocol.DnUrl(), 4, private.SequenceOptions{})
			return err
		}},
	} {
		before := misses()
		beforeAnchor := anchorMisses()
		err := c.call()
		require.Error(t, err, "%s answered for an entry it produced and lost", c.name)
		require.True(t, errors.Is(err, errors.NotFound),
			"%s: a number this node produced after it joined and lost is a MISS, not \"not from me\": %v", c.name, err)
		require.NotContains(t, err.Error(), "joined at block",
			"%s named its join block for a number it produced after joining", c.name)
		if c.kind == "entry" {
			require.Equal(t, before+1, misses(), "%s did not count the miss", c.name)
		} else {
			require.Equal(t, beforeAnchor+1, anchorMisses(), "%s did not count the anchor miss", c.name)
		}
	}

	// --- and a number the stream has not reached is still "not yet" --------
	before := misses()
	_, err = svc.Sequence(ctx, src, bvn1, 7, private.SequenceOptions{})
	require.True(t, errors.Is(err, errors.NotReady), "a number not produced yet: got %v", err)
	require.Equal(t, before, misses(), "asked too soon is not a miss")

	// --- above it and held: the same node answers, because it produced it --
	r, err := svc.Sequence(ctx, src, bvn1, 5, private.SequenceOptions{})
	require.NoError(t, err, "a joined node refused an entry it did produce")
	require.Equal(t, uint64(5), r.Sequence.Number)

	rs, err := svc.SequenceRange(ctx, src, bvn1, 5, 5, private.SequenceOptions{})
	require.NoError(t, err, "a joined node refused a range of entries it did produce")
	require.Len(t, rs, 1)
	require.Equal(t, uint64(5), rs[0].Sequence.Number)

	a, err := svc.Sequence(ctx, anchorSrc, protocol.DnUrl(), 3, private.SequenceOptions{})
	require.NoError(t, err, "a joined node refused an anchor it did produce")
	require.Equal(t, uint64(3), a.Sequence.Number)
}
