// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The 20260824T024122Z wedge left 63 holes scattered across ~20 separate runs
// (498-506, 514, 518, ...). One held anchor proves all of them — it postdates
// the newest hole — so a single scan must attempt every run, not one run per
// heal window (#4163).

func txid(b byte) *url.TxID { return protocol.PartitionUrl("BVN2").WithTxID([32]byte{b}) }

func TestMissingRuns_EnumeratesScatteredHolesOldestFirst(t *testing.T) {
	// Delivered=497; window: 498-506 missing, 507 known, 508 missing,
	// 509-510 known, 511-512 missing.
	pending := make([]*url.TxID, 0, 15)
	for i := 0; i < 9; i++ {
		pending = append(pending, nil) // 498-506
	}
	pending = append(pending, txid(1), nil, txid(2), txid(3), nil, nil) // 507..512

	part := &protocol.PartitionSyntheticLedger{Delivered: 497, Pending: pending}
	runs := missingRuns(part)
	assert.Equal(t, [][2]uint64{{498, 506}, {508, 508}, {511, 512}}, runs)
}

func TestMissingRuns_EmptyAndFullyKnownWindows(t *testing.T) {
	assert.Empty(t, missingRuns(&protocol.PartitionSyntheticLedger{Delivered: 5}))
	assert.Empty(t, missingRuns(&protocol.PartitionSyntheticLedger{
		Delivered: 5,
		Pending:   []*url.TxID{txid(1), txid(2)},
	}))
}

// rangeRecorder records every SequenceRange attempt and refuses to serve, so
// the scan's loop structure is observable without proof machinery.
type rangeRecorder struct {
	ranges [][2]uint64
	anchor []uint64
}

func (s *rangeRecorder) Sequence(ctx context.Context, _, _ *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound.With("per-message path not under test")
}

func (s *rangeRecorder) SequenceRange(ctx context.Context, _, _ *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	s.ranges = append(s.ranges, [2]uint64{start, end})
	s.anchor = append(s.anchor, opts.ProveAgainstAnchor)
	return nil, errors.NotReady.With("not serving; this test only records the attempts")
}

// One scan must attempt EVERY missing run, each under the SAME held anchor.
func TestRequestMissingSynthetics_PullsEveryRunUnderOneHeldAnchor(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = self.JoinPath(protocol.Synthetic)
	part := ledger.Partition(source)
	part.Delivered = 497
	part.Received = 512
	pending := make([]*url.TxID, 0, 15)
	for i := 0; i < 9; i++ {
		pending = append(pending, nil) // 498-506
	}
	pending = append(pending, txid(1), nil, txid(2), txid(3), nil, nil) // 507..512
	part.Pending = pending

	anchors := new(protocol.AnchorLedger)
	anchors.Url = self.JoinPath(protocol.AnchorPool)
	anchors.Partition(source).Delivered = 1454

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.Account(anchors.Url).Main().Put(anchors))

	seq := new(rangeRecorder)
	c := &Conductor{
		Partition:           &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		Sequencer:           seq,
		SyntheticHealWindow: time.Nanosecond, // claim fires on the second scan
	}
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Kourou})

	// First scan registers the jittered claim; second fires.
	require.NoError(t, c.requestMissingSynthetics(context.Background(), batch))
	time.Sleep(time.Millisecond)
	require.NoError(t, c.requestMissingSynthetics(context.Background(), batch))

	require.Equal(t, [][2]uint64{{498, 506}, {508, 508}, {511, 512}}, seq.ranges,
		"one scan attempts every missing run, oldest first")
	for i, a := range seq.anchor {
		assert.Equal(t, uint64(1454), a, "run %d must be proven against the same held anchor", i)
	}
}
