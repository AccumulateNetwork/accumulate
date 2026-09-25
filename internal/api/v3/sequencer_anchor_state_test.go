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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// validatorWithAnchors is a BVN0 validator's sequencer whose cache holds the
// anchors for blocks 5 and 9, as numbers 1 and 2, both of which it signed
// (SystemData.SignedAnchor, as the conductor records them).
func validatorWithAnchors(t *testing.T, machine *nodestate.Machine, check *execute.RootCheck) (*Sequencer, *synthcache.Cache) {
	t.Helper()
	db := database.OpenInMemory(nil)
	cache := synthcache.New(0)
	tx := cache.Begin(10)
	batch := db.Begin(true)
	for n, block := range map[uint64]uint64{1: 5, 2: 9} {
		body := &protocol.BlockValidatorAnchor{
			PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN0"), MinorBlockIndex: block},
		}
		tx.AddAnchor(n, block, &protocol.Transaction{Body: body})
		hash, err := database.SignedAnchorHash(body)
		require.NoError(t, err)
		require.NoError(t, batch.SystemData("BVN0").SignedAnchor(protocol.DnUrl(), n).Put(hash))
	}
	require.NoError(t, batch.Commit())
	tx.Commit()
	cache.Begin(10 + synthcache.InFlightBlocks).Commit() // out of the in-flight window

	_, key, _ := ed25519.GenerateKey(nil)
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	globals.Network.AddValidator(key.Public().(ed25519.PublicKey), "BVN0", true)
	return NewSequencer(SequencerParams{
		Database:     db,
		EventBus:     events.NewBus(nil),
		Globals:      globals,
		Partition:    "BVN0",
		ValidatorKey: key,
		Cache:        cache,
		NodeState:    machine,
		RootCheck:    check,
	}), cache
}

// recordSigned records body as the anchor this node signed under num for dst,
// as the conductor does before it signs (#4443).
func recordSigned(db *database.Database, part string, dst *url.URL, num uint64, body protocol.TransactionBody) {
	hash, err := database.SignedAnchorHash(body)
	if err != nil {
		panic(err)
	}
	batch := db.Begin(true)
	defer batch.Discard()
	if err := batch.SystemData(part).SignedAnchor(dst, num).Put(hash); err != nil {
		panic(err)
	}
	if err := batch.Commit(); err != nil {
		panic(err)
	}
}

var bvn0Anchors = protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

// A BOOTING node answers for the anchors it executed and signed (executor
// spec, "Sync", §6, "Two things do not wait for ACTIVE"). Before #4443 it
// refused every one with "is joining", so a partition with more than
// N - threshold validators booting at once could collect no quorum.
func TestSequencer_AnswersItsAnchorsWhileBooting(t *testing.T) {
	machine := nodestate.New(protocol.PartitionUrl("BVN0"))
	require.Equal(t, nodestate.StateBooting, machine.State())
	s, _ := validatorWithAnchors(t, machine, nil)
	ctx := context.Background()

	r, err := s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.NoError(t, err, "a BOOTING node must answer for an anchor it executed")
	require.Equal(t, uint64(1), r.Signatures.Total, "the answer carries this node's signature")

	rs, err := s.SequenceRange(ctx, bvn0Anchors, protocol.DnUrl(), 1, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, rs, 2)

	// What does wait for ACTIVE still does.
	_, err = s.Sequence(ctx, protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic), protocol.DnUrl(), 1, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
	require.Contains(t, err.Error(), "is joining")
}

// It signs only what it signed. An anchor it never signed -- the join pulled
// it, or it was produced while this node was not signing -- goes out with the
// held quorum alone, here none, so it is refused; one it signed with a
// different body under the same number is refused whatever it holds.
func TestSequencer_SignsOnlyTheAnchorsItSigned(t *testing.T) {
	s, _ := validatorWithAnchors(t, nodestate.New(protocol.PartitionUrl("BVN0")), nil)
	ctx := context.Background()

	batch := s.db.(*database.Database).Begin(true)
	require.NoError(t, batch.SystemData("BVN0").SignedAnchor(protocol.DnUrl(), 1).Put([32]byte{1}))
	require.NoError(t, batch.Commit())
	_, err := s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 1, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady, "anchor 1 was signed with another body")
	require.Contains(t, err.Error(), "signed a different anchor 1")

	r, err := s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.NoError(t, err, "anchor 2 is the body this node signed")
	require.Equal(t, uint64(1), r.Signatures.Total)

	// Nothing recorded for another destination: while BOOTING, those are the
	// anchors a join pulled, and they carry no signature of this node's.
	_, err = s.Sequence(ctx, bvn0Anchors, protocol.PartitionUrl("BVN1"), 2, private.SequenceOptions{})
	require.Error(t, err, "a BOOTING node signed an anchor it never signed")

	// An ACTIVE node signs an anchor it never signed, as before #4443 --
	// nobody else can re-sign one its conductor never sent -- and records
	// it, so the number is bound to that body from then on.
	active, _ := validatorWithAnchors(t, nil, nil)
	r, err = active.Sequence(ctx, bvn0Anchors, protocol.PartitionUrl("BVN1"), 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Equal(t, uint64(1), r.Signatures.Total)
	var recorded [32]byte
	require.NoError(t, active.db.View(func(batch *database.Batch) error {
		var err error
		recorded, err = batch.SystemData("BVN0").SignedAnchor(protocol.PartitionUrl("BVN1"), 2).Get()
		return err
	}))
	txn, _, ok := active.cache.PeekAnchor(2)
	require.True(t, ok)
	want, err := database.SignedAnchorHash(txn.Body)
	require.NoError(t, err)
	require.Equal(t, want, recorded, "the ACTIVE node did not record what it signed")
}

// A node whose newest checked root did not match signs nothing: no anchor, no
// synthetic, whatever its state (lag one). It answers again once a newer check
// matches.
func TestSequencer_SignsNothingAfterAMismatch(t *testing.T) {
	check := new(execute.RootCheck)
	s, _ := validatorWithAnchors(t, nil, check) // a node that never joined, and ACTIVE
	ctx := context.Background()

	_, err := s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.NoError(t, err, "precondition: nothing checked, so it answers")

	check.Mismatched(8)
	_, err = s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
	require.Contains(t, err.Error(), "did not match")
	_, err = s.SequenceRange(ctx, bvn0Anchors, protocol.DnUrl(), 1, 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
	_, err = s.SequenceRange(ctx, protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic), protocol.DnUrl(), 1, 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
	require.Contains(t, err.Error(), "did not match")

	check.Matched(7) // older than the mismatch: says nothing about now
	_, err = s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)

	check.Matched(9)
	_, err = s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.NoError(t, err)
}
