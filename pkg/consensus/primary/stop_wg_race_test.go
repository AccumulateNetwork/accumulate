// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestStopWhileStartLoopSpawns is the #4640/#4616 regression test. Stop waits
// on p.wg while the Start loop can still handle a header and spawn a tracked
// goroutine (handleHeader -> broadcastVoteAsync -> p.wg.Add). Adding from a
// zero counter concurrently with Wait is a sync.WaitGroup misuse that the race
// detector reports, and an Add that lands after Wait returned starts a
// goroutine that outlives Stop. The headers reach the Start loop through the
// same select the gossip channel feeds (the retest channel), so the loop keeps
// calling handleHeader while Stop runs.
func TestStopWhileStartLoopSpawns(t *testing.T) {
	const iterations = 200

	var voted int64
	for i := 0; i < iterations; i++ {
		self := newTestValidator(t)
		others := []*testValidator{newTestValidator(t), newTestValidator(t), newTestValidator(t)}
		validators := append([]*testValidator{self}, others...)
		committee := newTestCommittee(validators, 1)
		d := newTestDAG()
		createGenesisCertificates(t, validators, committee, d)

		p := New(Config{
			Partition:            "test",
			KeyPair:              self.priv,
			RoundAdvanceInterval: time.Millisecond,
		}, committee, nil, d, nil)

		// Pre-sign distinct headers (each is voted for once, so each one
		// reaches broadcastVoteAsync).
		var headers []*types.Header
		for r := 0; r < 200; r++ {
			o := others[r%len(others)]
			h := types.NewHeader(o.pub, types.Round(r/len(others)), committee.Epoch, nil, nil)
			require.NoError(t, h.Sign(o.priv))
			headers = append(headers, h)
		}

		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			close(started)
			_ = p.Start(ctx)
		}()
		stopFeed := make(chan struct{})
		go func() {
			defer wg.Done()
			for _, h := range headers {
				select {
				case p.retests <- h:
				case <-stopFeed:
					return
				}
			}
		}()

		<-started
		time.Sleep(time.Duration(i%5) * 200 * time.Microsecond)
		p.Stop()
		close(stopFeed)
		cancel()
		wg.Wait()
		voted += int64(p.votesSent.Load())

		// Stop is idempotent.
		p.Stop()
	}
	require.NotZero(t, voted, "the Start loop never voted: the test did not exercise the Add path")
}
