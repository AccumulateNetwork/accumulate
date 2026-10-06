// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// group is one committed leader's certificates, in the order they were
// committed.
type seedGroups map[types.Round][]types.CertificateDigest

func collectGroups(into seedGroups, outs []ConsensusOutput) {
	for _, o := range outs {
		into[o.Leader] = append(into[o.Leader], o.Certificate.Digest())
	}
}

// weakLinkWindow is primary.weakLinkWindow: a header names every certificate
// its author holds of the round before it and of the seven rounds before
// that (header_builder.go).
const weakLinkWindow = 8

// messyDAG builds rounds 1..last on h's DAG as the primary builds headers,
// feeding every certificate to peer as it is inserted, and returns every
// certificate in insertion order. It is a network that is not tidy: a
// validator misses a round, and a certificate reaches the others one round
// late or several -- so it is named by a later round's headers, or by none --
// and a leader that way goes without support and is committed, if at all,
// through a later leader.
func messyDAG(h *testHelper, rng *rand.Rand, last types.Round, peer *Bullshark, theirs seedGroups) []*types.Certificate {
	all := h.insertGenesis()
	byRound := map[types.Round][]*types.Certificate{0: all}
	seenFrom := map[types.CertificateDigest]types.Round{} // the first round whose headers name it
	for _, c := range all {
		seenFrom[c.Digest()] = 1
	}
	n := len(h.keys)
	for r := types.Round(1); r <= last; r++ {
		var parents []*types.Certificate
		for back := types.Round(1); back <= weakLinkWindow && back <= r; back++ {
			for _, c := range byRound[r-back] {
				if seenFrom[c.Digest()] <= r {
					parents = append(parents, c)
				}
			}
		}

		// Who produces this round: everyone, or all but one. At most one of
		// four is late to the others, so the next round has its quorum.
		authors := rng.Perm(n)
		if rng.Intn(3) == 0 {
			authors = authors[:n-1]
		}
		slow := -1
		if len(authors) == n && rng.Intn(3) == 0 {
			slow = rng.Intn(n)
		}
		for k, i := range authors {
			cert := h.createCert(i, r, parents)
			require.NoError(h.t, h.dag.Insert(cert))
			collectGroups(theirs, peer.ProcessCertificate(cert))
			all = append(all, cert)
			byRound[r] = append(byRound[r], cert)
			seenFrom[cert.Digest()] = r + 1
			if k == slow {
				// Two to ten rounds late: past eight, no header names it.
				seenFrom[cert.Digest()] = r + 2 + types.Round(rng.Intn(9))
			}
		}
	}
	return all
}

// seededNode orders, on a DAG and position of its own, what a node seeded at
// s orders: the certificates above s in the order the peer saw them. Those of
// the round after the seed it takes without their parents, as the DAG does;
// a parent of a later one that it does not hold is at or below that round,
// and it asks for it and takes it the same way (primary.requestParents).
func seededNode(t *testing.T, h *testHelper, all []*types.Certificate, s types.Round) seedGroups {
	t.Helper()
	byDigest := map[types.CertificateDigest]*types.Certificate{}
	for _, c := range all {
		byDigest[c.Digest()] = c
	}
	jd := dag.NewDAG(1000)
	jd.SetLastCommitRound(s)
	joiner := New(h.committee, jd)
	joiner.SetLastCommitRound(s)
	mine := seedGroups{}
	for _, c := range all {
		if c.Round() <= s {
			continue
		}
		if c.Round() > s+1 {
			for _, d := range c.Parents() {
				if jd.Contains(d) {
					continue
				}
				parent := byDigest[d]
				require.LessOrEqual(t, parent.Round(), s+1)
				require.NoError(t, jd.Insert(parent))
				collectGroups(mine, joiner.ProcessCertificate(parent))
			}
		}
		require.NoError(t, jd.Insert(c))
		collectGroups(mine, joiner.ProcessCertificate(c))
	}
	return mine
}

// A node seeded at a round below a state's round orders, above that state,
// the groups its peers ordered, whatever kind of round the seed is: a
// committed leader's, a skipped leader's, an odd round. It holds only what a
// seeded node holds -- every certificate above the seed, with its parents,
// and of the rounds at or below the one after the seed only what something
// above names -- and it has no committed-digest set.
//
// What it needs is that the state's round is more than the rescue window
// above the seed: the first leader after the state walks the rescue window
// below the state, and there the node holds every certificate and has
// committed, in its own groups since the seed, exactly what its peers had.
// That is why a joining node is seeded the rescue window and one leader below
// the first state it proves, and not at that state's round, from which its
// first handoff is always refused (#4530). The seed need not be a committed
// leader's round: the leader chain above it is fixed by the DAG above it.
// (In this DAG every margin from two rounds up gave the peers' groups; the
// window is the bound the commit walk guarantees, not the least that works.)
func TestSeededBelowAStateOrdersThePeersGroupsAboveIt(t *testing.T) {
	const last = types.Round(160)
	for seed := int64(1); seed <= 6; seed++ {
		rng := rand.New(rand.NewSource(seed))
		h := newTestHelper(t, 4)
		h.dag = dag.NewDAG(1000)
		peer := New(h.committee, h.dag)
		theirs := seedGroups{}
		all := messyDAG(h, rng, last, peer, theirs)
		byDigest := map[types.CertificateDigest]*types.Certificate{}
		for _, c := range all {
			byDigest[c.Digest()] = c
		}

		// The precondition the test is worth nothing without: leaders were
		// skipped, and stragglers were committed by a later leader.
		var skipped, rescued int
		for l := types.Round(2); l <= last-2; l += 2 {
			if _, ok := theirs[l]; !ok {
				skipped++
			}
		}
		for l, ds := range theirs {
			for _, d := range ds {
				if byDigest[d].Round()+2 < l {
					rescued++
				}
			}
		}
		require.NotZero(t, skipped, "seed %d: no leader was skipped", seed)
		require.NotZero(t, rescued, "seed %d: no straggler was committed late", seed)

		// Every state a node could prove: each committed leader's round P.
		// And every way of seeding below it: the rescue window and one
		// leader below, the one round fewer that is still enough, and
		// further.
		states := 0
		for p := types.Round(60); p <= last-20; p += 2 {
			if _, ok := theirs[p]; !ok {
				continue
			}
			states++
			for _, below := range []types.Round{RescueWindow + 1, RescueWindow + 2, RescueWindow + 7, RescueWindow + 20} {
				s := p - below
				name := fmt.Sprintf("seed %d: state at round %d, seeded %d below at %d", seed, p, below, s)

				mine := seededNode(t, h, all, s)

				// It committed the state's leader, which is how the handoff
				// knows where the state ends, and every group above it is
				// the peers' group.
				require.Contains(t, mine, p, name+": the node committed no leader at the state's round")
				for l := p + 2; l <= last; l += 2 {
					require.Equal(t, theirs[l], mine[l], "%s: the group committed at leader %d", name, l)
				}
			}
		}
		require.NotZero(t, states)
	}
}

// A margin is needed: seeded at the state's own round, as a joining node was
// before #4530, a node orders groups above the state that are not its peers'
// -- it has neither the state's leader nor what that leader committed. That
// is the refusal the handoff makes (dagbft.Service.checkRound). The margin
// the handoff asks for is the whole rescue window, the furthest a commit
// walks; this DAG, linked as the primary links, never needed that much.
func TestSeededAtAStatesOwnRoundOrdersAnotherGroup(t *testing.T) {
	const last = types.Round(160)
	differ := 0
	for seed := int64(1); seed <= 6 && differ == 0; seed++ {
		rng := rand.New(rand.NewSource(seed))
		h := newTestHelper(t, 4)
		h.dag = dag.NewDAG(1000)
		peer := New(h.committee, h.dag)
		theirs := seedGroups{}
		all := messyDAG(h, rng, last, peer, theirs)
		byDigest := map[types.CertificateDigest]*types.Certificate{}
		for _, c := range all {
			byDigest[c.Digest()] = c
		}
		for p := types.Round(60); p <= last-20; p += 2 {
			if _, ok := theirs[p]; !ok {
				continue
			}
			// Seeded at the state's own round, as a node was before #4530.
			mine := seededNode(t, h, all, p)
			for l := p + 2; l <= last; l += 2 {
				if fmt.Sprint(theirs[l]) != fmt.Sprint(mine[l]) {
					differ++
				}
			}
		}
	}
	require.NotZero(t, differ, "a node seeded at a state's own round ordered its peers' groups above it every time: no margin is shown to be needed")
}
