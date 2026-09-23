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

const servingPartition = "BVN9"

func newServingSequencer(t *testing.T, machine *nodestate.Machine) *Sequencer {
	t.Helper()
	return NewSequencer(SequencerParams{
		Partition: servingPartition,
		Cache:     synthcache.New(0),
		EventBus:  events.NewBus(nil),
		Globals:   new(core.GlobalValues),
		NodeState: machine,
	})
}

// A node that is joining answers for nothing it has not executed: its producer
// cache holds what it produced before it left and nothing of the blocks it
// missed, so an answer from it is an answer from an empty cache — which
// healing reads as "the source has nothing" and which spends the requester's
// retry budget on a node that cannot help (executor spec, "Sync", step 5;
// #4287, #4295).
func TestSequencer_RefusesWhileJoining(t *testing.T) {
	machine := nodestate.New(protocol.PartitionUrl(servingPartition))
	s := newServingSequencer(t, machine)

	ctx := context.Background()
	src, dst := protocol.PartitionUrl("BVN0"), protocol.PartitionUrl(servingPartition)
	joining := func() map[string]error {
		out := map[string]error{}
		_, out["sequence"] = s.Sequence(ctx, src, dst, 1, private.SequenceOptions{})
		_, out["sequence-range"] = s.SequenceRange(ctx, src, dst, 1, 2, private.SequenceOptions{})
		_, out["major-header-range"] = s.MajorHeaderRange(ctx, dst, 1, 2, private.SequenceOptions{})
		_, out["minor-root-range"] = s.MinorRootRange(ctx, dst, 1, 2, private.SequenceOptions{})
		_, out["partition-root-range"] = s.PartitionRootRange(ctx, dst, [32]byte{1}, private.SequenceOptions{})
		_, out["snapshot-range"] = s.SnapshotRange(ctx, dst, 0, 0, private.SequenceOptions{})
		return out
	}

	for call, err := range joining() {
		require.True(t, errors.Is(err, errors.NotReady), "%s must refuse while joining: %v", call, err)
		require.Contains(t, err.Error(), "is joining", "%s must say why it refused", call)
	}

	// Once the root has matched and the node executes, it answers for itself.
	// (Some of these still refuse, for their own reasons — a node with no
	// provable state cannot pin a snapshot — but never for joining.)
	require.True(t, machine.PromoteToActive([32]byte{1}, 42))
	for call, err := range joining() {
		if err != nil {
			require.NotContains(t, err.Error(), "is joining",
				"%s must not refuse for joining once the node is active", call)
		}
	}
}

// A node that never joined has no state of its own and serves: the gate is a
// statement about a node that is catching up, not a new precondition for
// every node.
func TestSequencer_ServesWithNoStateOfItsOwn(t *testing.T) {
	s := newServingSequencer(t, nil)
	_, err := s.Sequence(context.Background(), protocol.PartitionUrl("BVN0"),
		protocol.PartitionUrl(servingPartition), 1, private.SequenceOptions{})
	require.False(t, errors.Is(err, errors.NotReady), "got %v", err)
}
