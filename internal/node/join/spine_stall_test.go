// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// poolRing is peerQuerier's rotation over anchor pools, and what peerQuerier
// says about itself (anchorsrc.Peers).
type poolRing struct {
	mu    sync.Mutex
	peers []api.Querier
	next  int
	asked []string
}

func (r *poolRing) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	start := r.next
	r.next++
	r.asked = r.asked[:0]
	var last error
	for i := range r.peers {
		n := (start + i) % len(r.peers)
		r.asked = append(r.asked, fmt.Sprintf("peer%d", n))
		rec, err := r.peers[n].Query(ctx, scope, q)
		if err == nil {
			return rec, nil
		}
		last = err
	}
	return nil, last
}

func (r *poolRing) PeerCount(context.Context) int { return len(r.peers) }
func (r *poolRing) LastAsked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.asked...)
}

type ringSources struct{ ring *poolRing }

func (ringSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return nil, nil, nil
}
func (s ringSources) Querier(*url.URL) api.Querier { return s.ring }

// (#4419c) A join whose anchor source is held at an entry no peer serves
// signed says so once a minute, naming the entry and the peers asked, and the
// entry reads on accumulate_join_spine_stalled_entry; the refusals behind it
// are said once a minute too, not once per peer per round. When a peer serves
// the entry the gauge reads -1 and the join says it moved on.
//
// Before: every 2 s round cost 64 page calls and about 64 "An anchor was
// refused" lines, Read returned nil, and nothing said the join had stopped
// (#4413 review F2).
func TestAStalledSpineIsSaidOnceAMinuteAndReadsOnTheGauge(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	values, keys := genesisValues(t, 4)
	local := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = local.Close() })
	putNetwork(t, local, here, values)

	// Anchors 0-2 signed; from 3 on, held without their signatures on every
	// peer: a rolling restart inside one window.
	var signed, bare []*api.MessageRecord[messaging.Message]
	for i := 0; i < 10; i++ {
		rec := signedAnchor(t, values, keys, uint64(100+i), [32]byte{byte(i)})
		signed = append(signed, rec)
		if i < 3 {
			bare = append(bare, rec)
			continue
		}
		stripped := *rec
		stripped.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
		bare = append(bare, &stripped)
	}
	ring := &poolRing{peers: []api.Querier{&anchorPool{entries: bare}, &anchorPool{entries: bare}}}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s, err := NewState(StateOptions{Partition: here, Database: local, Sources: ringSources{ring}, Logger: log})
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
	require.Equal(t, float64(3), testutil.ToFloat64(gauge), "the gauge names the entry the spine is held at")
	require.Equal(t, 1, lines(stalled), "one stall line in the first minute:\n%s", buf.String())
	require.LessOrEqual(t, lines(refused), 1, "the same refusal, said again inside a minute")
	require.Contains(t, buf.String(), "entry=3")
	require.Contains(t, buf.String(), "peer0")
	require.Contains(t, buf.String(), "peer1")

	// The minute is up.
	_ = s.readAnchors(ctx)
	require.Equal(t, 2, lines(stalled), "the stall is said again once a minute")

	// A peer that holds the entry signed.
	ring.mu.Lock()
	ring.peers = append(ring.peers, &anchorPool{entries: signed})
	ring.mu.Unlock()
	for round := 0; round < 3; round++ {
		_ = s.readAnchors(ctx)
	}
	require.Equal(t, float64(-1), testutil.ToFloat64(gauge), "the entry was served and the gauge still reads a stall")
	require.Contains(t, buf.String(), "The anchor source moved past the entry it was held at")
}
