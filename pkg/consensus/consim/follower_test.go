// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"context"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestFollowerJoinCatchesUp is #4469: a follower -- in no committee -- joins
// a running network, is seeded by Rejoin a few dozen rounds below the
// frontier, as the join seeds it from the leader round its pulled state
// records, and must reach the frontier by certificate catch-up. In Docker run
// 20260929T152146Z the follower's BVN1 (4 validators) did so in seconds while
// its Directory (12 validators) sat at its seed round for four minutes.
func TestFollowerJoinCatchesUp(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the full 24-node consensus stack")
	}

	sim, err := New(Config{
		BVNs:             3,
		ValidatorsPerBVN: 4,
		TPS:              20,
		// Slow enough that a caught-up follower can outpace the frontier:
		// catch-up is paced at one round request a second, each answered with
		// at most 100 certificates (about 8 Directory rounds), and the soak's
		// frontier moves about 2 rounds a second.
		MinRoundInterval: 250 * time.Millisecond,
		Duration:         3 * time.Minute,
		StallAfter:       30 * time.Second,
		Out:              os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := sim.Run(ctx); done <- err }()

	// Let the network run, as the soak's did for minutes before the follower
	// was added.
	select {
	case err := <-done:
		t.Fatalf("network stopped before the follower joined: %v", err)
	case <-time.After(40 * time.Second):
	}

	frontier := func(part string) (r types.Round) {
		for _, n := range sim.Validators(part) {
			if x := n.CurrentRound(); x > r {
				r = x
			}
		}
		return r
	}
	// The join seeds consensus at the leader round its pulled state records:
	// a committed leader some way below the frontier (3574 against a handoff
	// at 3606 in the Docker run).
	const behind = 32
	// The join pulls and proves a state before it seeds consensus; until
	// then the node's DAG floor is zero and every live certificate it
	// receives waits for parents (the Docker follower's Directory started
	// its primary at round 0 and rejoined seconds later).
	const followerPull = 10 * time.Second
	seeds := map[string]types.Round{}
	seed := func(part string) types.Round {
		// A network that has not committed past the seed distance cannot
		// stage this join. Unchecked, the seed wrapped below zero to a
		// round near 2^64: the follower then counted as at the frontier,
		// or its DAG's collection walked 2^64 rounds holding its lock and
		// the test hung (#4610).
		last := sim.Validators(part)[0].LastCommitRound()
		if last <= behind {
			t.Fatalf("%s had committed only to round %d when the follower joined; the join is seeded %d rounds below the last commit", part, last, behind)
		}
		seeds[part] = last - behind
		return seeds[part]
	}
	parts := []string{"Directory", "BVN1"}
	followers, err := sim.JoinFollower(ctx, seed, followerPull, parts...)
	if err != nil {
		t.Fatalf("join follower: %v", err)
	}

	caughtUp := map[string]time.Duration{}
	start := time.Now()
	for len(caughtUp) < len(parts) && time.Since(start) < 60*time.Second {
		time.Sleep(500 * time.Millisecond)
		for i, part := range parts {
			if _, ok := caughtUp[part]; ok {
				continue
			}
			fr, r := frontier(part), followers[i].CurrentRound()
			if r+4 >= fr {
				caughtUp[part] = time.Since(start)
				t.Logf("%s follower reached the frontier (round %d, frontier %d) in %s", part, r, fr, caughtUp[part].Truncate(time.Millisecond))
			}
		}
	}
	for i, part := range parts {
		if _, ok := caughtUp[part]; ok {
			continue
		}
		// Say what the follower is waiting for: the certificates of the
		// round after its DAG's latest, and which of their parents it lacks,
		// by round.
		fd, vd := followers[i].DAG(), sim.Validators(part)[0].DAG()
		next := fd.LatestRound() + 1
		missing := map[types.Round]int{}
		for _, c := range vd.GetRound(next) {
			for _, pd := range c.Parents() {
				if !fd.Contains(pd) {
					if pc := vd.GetByDigest(pd); pc != nil {
						missing[pc.Round()]++
					}
				}
			}
		}
		t.Errorf("%s follower did not reach the frontier in 60s: seeded at %d, at round %d (DAG latest %d, %d certificates held pending), frontier %d; round %d's certificates lack parents by round %v",
			part, seeds[part], followers[i].CurrentRound(), fd.LatestRound(), followers[i].Primary().PendingCertsCount(), frontier(part), next, missing)
	}
}
