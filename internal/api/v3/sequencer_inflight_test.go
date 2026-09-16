// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// anchorSequencer is a node that produced anchor 3 in block 7 and has since
// executed up to newest, with its executor lag blocks behind consensus.
func anchorSequencer(lag int, newest uint64) *Sequencer {
	cache := synthcache.New(0)
	cache.SetExecutionLagSource(func() int { return lag })
	tx := cache.Begin(7)
	tx.SetBlock(&synthcache.Block{Index: 7})
	tx.AddAnchor(3, 7, &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{
		PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN0"), MinorBlockIndex: 7},
	}})
	tx.Commit()
	if newest > 7 {
		cache.Begin(newest).Commit()
	}

	_, key, _ := ed25519.GenerateKey(nil)
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	return NewSequencer(SequencerParams{
		Database:     database.OpenInMemory(nil),
		EventBus:     events.NewBus(nil),
		Globals:      globals,
		Partition:    "BVN0",
		ValidatorKey: key,
		Cache:        cache,
	})
}

// The anchor path measures the same window as the entry path, and for the
// same reason: the node that answers is not the node that sent. A node whose
// executor is behind consensus has to assume the leader's send is behind by
// about as much, or it answers for anchors that are still to leave and the
// destination heals every one of them (#4248).
func TestSequencer_AnchorInFlightWindowFollowsTheSender(t *testing.T) {
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	dn := protocol.DnUrl()

	// Caught up, and the window has passed: the anchor is served.
	svc := anchorSequencer(0, 7+synthcache.InFlightBlocks)
	rs, err := svc.getAnchorRangeFromCache(globals, dn, 3, 3)
	require.NoError(t, err)
	require.Len(t, rs, 1)

	// Inside the window: on its way, not missing.
	svc = anchorSequencer(0, 7+synthcache.InFlightBlocks-1)
	_, err = svc.getAnchorRangeFromCache(globals, dn, 3, 3)
	require.ErrorIs(t, err, errors.NotReady)

	// Lagging by 12, and the unwidened window has passed. Before the fix
	// this served; the leader has not sent the anchor yet.
	svc = anchorSequencer(12, 7+synthcache.InFlightBlocks)
	_, err = svc.getAnchorRangeFromCache(globals, dn, 3, 3)
	require.ErrorIs(t, err, errors.NotReady)

	// Far enough past the mark that even a leader 12 blocks behind has sent.
	svc = anchorSequencer(12, 7+synthcache.InFlightBlocks+12)
	rs, err = svc.getAnchorRangeFromCache(globals, dn, 3, 3)
	require.NoError(t, err)
	require.Len(t, rs, 1)
}
