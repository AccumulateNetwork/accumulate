// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A primary restarted at a round it never advanced from in this process
// (restored by SetRound and RestoreAuthored) still recovers from a stall: it
// re-shares its certificates and asks for the ones it is missing. A whole
// restart of a committee of two that stopped with each validator holding
// only its own certificate of the round otherwise waits for good, since
// nothing is pending and no one authors (netsim two-BVN restart: the
// Directory held at round 13).
func TestARestoredRoundThatNeverAdvancesRecoversFromTheStall(t *testing.T) {
	p, _ := restoredPrimary(t, 2)
	p.SetRound(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Start(ctx) }()

	recovered := func() bool {
		p.roundSyncMu.Lock()
		defer p.roundSyncMu.Unlock()
		return !p.lastStallRecovery.IsZero()
	}
	time.Sleep(time.Second)
	require.False(t, recovered(), "a primary started a second ago was taken for stalled")
	require.Eventually(t, recovered, 10*time.Second, 50*time.Millisecond,
		"a restored round that never advanced was never recovered from the stall")

	p.roundMu.Lock()
	advanced := !p.lastRoundAdvance.IsZero()
	p.roundMu.Unlock()
	require.False(t, advanced, "the round advanced without a quorum")
}
