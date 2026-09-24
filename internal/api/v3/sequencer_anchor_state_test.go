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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// validatorWithAnchors is a BVN0 validator's sequencer whose cache holds the
// anchors for blocks 5 and 9, as numbers 1 and 2.
func validatorWithAnchors(t *testing.T, machine *nodestate.Machine, check *execute.RootCheck) (*Sequencer, *synthcache.Cache) {
	t.Helper()
	cache := synthcache.New(0)
	tx := cache.Begin(10)
	for n, block := range map[uint64]uint64{1: 5, 2: 9} {
		tx.AddAnchor(n, block, &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{
			PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN0"), MinorBlockIndex: block},
		}})
	}
	tx.Commit()
	cache.Begin(10 + synthcache.InFlightBlocks).Commit() // out of the in-flight window

	_, key, _ := ed25519.GenerateKey(nil)
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	globals.Network.AddValidator(key.Public().(ed25519.PublicKey), "BVN0", true)
	return NewSequencer(SequencerParams{
		Database:     database.OpenInMemory(nil),
		EventBus:     events.NewBus(nil),
		Globals:      globals,
		Partition:    "BVN0",
		ValidatorKey: key,
		Cache:        cache,
		NodeState:    machine,
		RootCheck:    check,
	}), cache
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

// It refuses only the blocks it did not execute: at or below the block it
// joined at, the cache holds anchors the join pulled, and this node's
// signature on one would be on a root it never computed.
func TestSequencer_RefusesAnchorsForBlocksItDidNotExecute(t *testing.T) {
	s, cache := validatorWithAnchors(t, nodestate.New(protocol.PartitionUrl("BVN0")), nil)
	cache.JoinedAt(7)
	ctx := context.Background()

	_, err := s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 1, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady, "anchor 1 is for block 5, which this node joined past")
	require.Contains(t, err.Error(), "did not execute block 5")

	_, err = s.Sequence(ctx, bvn0Anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.NoError(t, err, "anchor 2 is for block 9, which this node executed")

	// A range answers the part it executed; one that starts below it is refused.
	_, err = s.SequenceRange(ctx, bvn0Anchors, protocol.DnUrl(), 1, 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
	rs, err := s.SequenceRange(ctx, bvn0Anchors, protocol.DnUrl(), 2, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, rs, 1)
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
