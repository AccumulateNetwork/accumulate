// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A header this primary authors is durable before anyone can see it
// (#4448, #4592): persisted and synced before it is broadcast, and before
// the rebroadcast path can send it. The sync is two device flushes, and the
// primary holds no lock while it runs — votes, certificates and the round
// go on meanwhile — so the round is claimed before the persist, and a
// second attempt at the round while the persist runs authors nothing.
func TestAuthoredHeaderIsDurableBeforeBroadcast(t *testing.T) {
	// Four validators: a self-vote alone certifies nothing, so the header
	// stays in vote collection, where the rebroadcast path would find it.
	var validators []*testValidator
	for i := 0; i < 4; i++ {
		validators = append(validators, newTestValidator(t))
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	createGenesisCertificates(t, validators, committee, d)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	store, err := persist.OpenDAGStore(t.TempDir())
	require.NoError(t, err)
	persisted := make(chan struct{})
	var broadcasts atomic.Int32

	p.SetOnAuthored(func(h *types.Header) error {
		// While the header is being made durable: the primary answers, the
		// header is not yet in vote collection, a rebroadcast sends nothing,
		// and the round cannot be authored again.
		probes := make(chan struct{})
		go func() {
			defer close(probes)
			if p.RoundStatus().OwnHeader {
				t.Error("the header is in vote collection before it is durable")
			}
			p.rebroadcastPendingHeaders()
			p.tryCreateAndBroadcastHeader()
		}()
		select {
		case <-probes:
		case <-time.After(5 * time.Second):
			t.Error("the primary is locked while the authored header is persisted")
		}
		if n := broadcasts.Load(); n != 0 {
			t.Errorf("the header was broadcast %d times before it was durable", n)
		}
		err := store.PutAuthored(h)
		close(persisted)
		return err
	})

	var once sync.Once
	sent := make(chan struct{})
	p.broadcastHeader = func(_ context.Context, h *types.Header) error {
		broadcasts.Add(1)
		select {
		case <-persisted:
		default:
			t.Error("the header was broadcast before it was persisted")
		}
		if got, err := store.Authored(); err != nil || got == nil || got.Digest() != h.Digest() {
			t.Errorf("the authored file does not hold the broadcast header: %v", err)
		}
		once.Do(func() { close(sent) })
		return nil
	}

	// Round 1, with the genesis certificates as parents.
	p.AdvanceRound()
	select {
	case <-sent:
	case <-time.After(10 * time.Second):
		t.Fatal("the header was never broadcast")
	}
	p.wg.Wait()
	require.Equal(t, uint64(1), p.headersCreated.Load(), "the round was authored again while its header was persisted")
	require.Equal(t, int32(1), broadcasts.Load())
	require.True(t, p.RoundStatus().OwnHeader, "the durable header is not in vote collection")
	last, has := p.LastAuthoredRound()
	require.True(t, has)
	require.Equal(t, types.Round(1), last)
}
