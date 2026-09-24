// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// sequencerOf is one validator's sequencer answering a produced anchor by
// number out of a list: anchor n is entries[n-1].
type sequencerOf struct {
	mu      sync.Mutex
	entries []*api.MessageRecord[messaging.Message]
}

func (v *sequencerOf) Sequence(_ context.Context, _, _ *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if num == 0 || num > uint64(len(v.entries)) {
		return nil, errors.NotFound.WithFormat("anchor %d is not in the cache", num)
	}
	rec := *v.entries[num-1]
	if rec.Signatures != nil {
		sigs := *rec.Signatures
		sigs.Records = append([]*api.SignatureSetRecord(nil), sigs.Records...)
		rec.Signatures = &sigs
	}
	return &rec, nil
}

// validatorRing is a partition whose validators the test names, and whose
// anchor ledger says the last anchor produced is number 1.
type validatorRing struct {
	mu   sync.Mutex
	vals []anchorsrc.Validator
}

func (r *validatorRing) ValidatorsOf(context.Context, *url.URL) ([]anchorsrc.Validator, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]anchorsrc.Validator(nil), r.vals...), nil
}

func (*validatorRing) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return nil, nil, nil
}

func (*validatorRing) Querier(*url.URL) api.Querier { return anchorLedgerAt(1) }

// anchorLedgerAt answers a partition's anchor ledger, saying the last anchor
// it produced is number n, and nothing else.
type anchorLedgerAt uint64

func (n anchorLedgerAt) Query(_ context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	if _, ok := q.(*api.DefaultQuery); ok && scope.PathEqual(protocol.AnchorPool) {
		return &api.AccountRecord{Account: &protocol.AnchorLedger{Url: scope, MinorBlockSequenceNumber: uint64(n)}}, nil
	}
	return nil, errors.NotFound.WithFormat("%v: not held", scope)
}

// (#4419c) A join whose anchor source is held at an anchor no quorum of the
// partition's validators signed says so once a minute, naming the anchor and
// the validators asked, and the anchor reads on
// accumulate_join_spine_stalled_entry; the refusals behind it are said once a
// minute too, not once per validator per round. When a validator signs it
// the gauge reads -1 and the join says it moved on.
//
// The anchors are the partition's own, collected from its validators
// (executor spec, "Sync", "The algorithm", step 3; #4438). Before #4419 every
// 2 s round cost 64 page calls and about 64 "An anchor was refused" lines,
// the read returned nil, and nothing said the join had stopped (#4413 review
// F2).
func TestAStalledSpineIsSaidOnceAMinuteAndReadsOnTheGauge(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	values, keys := genesisValues(t, 4)
	local := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = local.Close() })
	putNetwork(t, local, here, values)

	// Anchors 1-3 signed; from 4 on, answered without their signatures by
	// every validator asked: a rolling restart inside one window.
	var signed, bare []*api.MessageRecord[messaging.Message]
	for i := 1; i <= 10; i++ {
		rec := signedAnchor(t, values, keys, uint64(i), [32]byte{byte(i)})
		signed = append(signed, rec)
		if i <= 3 {
			bare = append(bare, rec)
			continue
		}
		stripped := *rec
		stripped.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
		bare = append(bare, &stripped)
	}
	ring := &validatorRing{vals: []anchorsrc.Validator{
		{Name: "peer0", Sequencer: &sequencerOf{entries: bare}},
		{Name: "peer1", Sequencer: &sequencerOf{entries: bare}},
	}}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s, err := NewState(StateOptions{Partition: here, Database: local, Sources: ring, Logger: log})
	require.NoError(t, err)
	clock := time.Unix(1_000_000, 0)
	s.now = func() time.Time { return clock }

	gauge := mSpineStalled.WithLabelValues("BVN0")
	require.Equal(t, float64(-1), testutil.ToFloat64(gauge), "a join that has read nothing is not stalled")

	lines := func(msg string) int { return strings.Count(buf.String(), msg) }
	const stalled = "This partition's spine is stalled"
	const refused = "An anchor was refused"

	// Thirty rounds, 2 s apart: one minute less one round.
	for round := 0; round < 30; round++ {
		_ = s.readAnchors(ctx)
		clock = clock.Add(2 * time.Second)
	}
	t.Logf("30 rounds: %d stall lines, %d refusal lines, gauge %v", lines(stalled), lines(refused), testutil.ToFloat64(gauge))
	require.Equal(t, float64(4), testutil.ToFloat64(gauge), "the gauge names the anchor the spine is held at")
	require.Equal(t, 1, lines(stalled), "one stall line in the first minute:\n%s", buf.String())
	require.LessOrEqual(t, lines(refused), 1, "the same refusal, said again inside a minute")
	require.Contains(t, buf.String(), "anchor=4")
	require.Contains(t, buf.String(), "peer0")
	require.Contains(t, buf.String(), "peer1")

	// The minute is up.
	_ = s.readAnchors(ctx)
	require.Equal(t, 2, lines(stalled), "the stall is said again once a minute")

	// A validator that signs it.
	ring.mu.Lock()
	ring.vals = append(ring.vals, anchorsrc.Validator{Name: "peer2", Sequencer: &sequencerOf{entries: signed}})
	ring.mu.Unlock()
	for round := 0; round < 3; round++ {
		_ = s.readAnchors(ctx)
	}
	require.Equal(t, float64(-1), testutil.ToFloat64(gauge), "the anchor was signed and the gauge still reads a stall")
	require.Contains(t, buf.String(), "The anchor source moved past the anchor it was held at")
}
