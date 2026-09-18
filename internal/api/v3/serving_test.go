// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A node that is joining answers for nothing it has not executed: its
// producer cache holds what it produced before it left and nothing of the
// blocks it missed, so an answer from it is an answer from an empty cache —
// which healing reads as "the source has nothing" and which spends the
// requester's retry budget on a node that cannot help (executor spec, "Sync",
// step 5; #4287, #4295).
func TestSequencer_RefusesWhileJoining(t *testing.T) {
	const partition = "BVN9"
	s := NewSequencer(SequencerParams{
		Partition: partition,
		Cache:     synthcache.New(0),
		EventBus:  events.NewBus(nil),
		Globals:   new(core.GlobalValues),
	})

	machine := nodestate.New(protocol.PartitionUrl(partition))
	nodestate.Register(partition, machine)
	t.Cleanup(func() { nodestate.Register(partition, nil) })

	ctx := context.Background()
	src, dst := protocol.PartitionUrl("BVN0"), protocol.PartitionUrl(partition)

	_, err := s.Sequence(ctx, src, dst, 1, private.SequenceOptions{})
	require.True(t, errors.Is(err, errors.NotReady), "a joining node refuses: %v", err)

	_, err = s.SequenceRange(ctx, src, dst, 1, 2, private.SequenceOptions{})
	require.True(t, errors.Is(err, errors.NotReady), "and refuses a range: %v", err)

	_, err = s.StagingSnapshot(ctx, &private.StagingSnapshotRequest{Partition: partition})
	require.True(t, errors.Is(err, errors.NotReady), "and refuses its stage: %v", err)

	// Once the root has matched and the node executes, it answers for itself.
	require.True(t, machine.PromoteToActive([32]byte{1}, 42))
	_, err = s.Sequence(ctx, src, dst, 1, private.SequenceOptions{})
	require.False(t, errors.Is(err, errors.NotReady), "an active node does not refuse for joining: %v", err)
}

// A node that never joined has no machine and serves: the gate is a statement
// about a node that is catching up, not a new precondition for every node.
func TestSequencer_ServesWhenNothingIsRegistered(t *testing.T) {
	require.True(t, nodestate.Serving("BVN-never-registered"))
}
