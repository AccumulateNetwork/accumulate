// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// fullRounds inserts rounds 0..last of n authors, each certificate naming
// every certificate of the round before, and returns them by round.
func fullRounds(t *testing.T, d *DAG, n int, last types.Round) [][]*types.Certificate {
	t.Helper()
	var pubs []ed25519.PublicKey
	for i := 0; i < n; i++ {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		pubs = append(pubs, pub)
	}
	rounds := make([][]*types.Certificate, last+1)
	for r := types.Round(0); r <= last; r++ {
		var parents []types.CertificateDigest
		if r > 0 {
			for _, p := range rounds[r-1] {
				parents = append(parents, p.Digest())
			}
		}
		for _, pub := range pubs {
			c := types.NewCertificate(types.NewHeader(pub, r, 0, nil, parents), nil, nil)
			if r == 0 {
				require.NoError(t, d.InsertGenesis(c))
			} else {
				require.NoError(t, d.Insert(c))
			}
			rounds[r] = append(rounds[r], c)
		}
	}
	return rounds
}

func strayCert(t *testing.T, r types.Round, parents ...types.CertificateDigest) *types.Certificate {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return types.NewCertificate(types.NewHeader(pub, r, 0, nil, parents), nil, nil)
}

// A certificate enters the DAG only with its parents present (Narwhal §4),
// however the commit floor stands (#4595): ordering flattens a leader's
// ancestors as found here and skips a parent it does not hold, which is
// deterministic only if every certificate's history is here. The floor
// used to waive the check for everything at or below the last commit plus
// one, which in normal running is every late certificate.
func TestInsert_RequiresParentsWhateverTheCommitFloor(t *testing.T) {
	d := NewDAG(10)
	rounds := fullRounds(t, d, 4, 20)
	d.GarbageCollect(18)

	unknown := types.CertificateDigest{0xAA}
	late := strayCert(t, 19, rounds[18][0].Digest(), unknown)
	require.Error(t, d.Insert(late), "a round-19 certificate entered the DAG without its parent")
	require.Nil(t, d.GetByDigest(late.Digest()))
}

// The legitimate exceptions, where the parents cannot be fetched
// (consensus.md, "DAG retention"): a seed into an empty DAG, which accepts
// certificates up to the round after it without their parents; and the
// weak-link window above a collection floor, whose certificates may name a
// collected parent. A restored tail sets the collection floor and ends the
// seed's waiver: a late certificate anywhere an ordering walk reaches is
// refused without its parents (#4595).
func TestInsert_ParentsWaivedOnlyWhereTheyCannotBeFetched(t *testing.T) {
	unknown := types.CertificateDigest{0xBB}

	t.Run("seed", func(t *testing.T) {
		d := NewDAG(2000)
		d.SetLastCommitRound(100)
		require.NoError(t, d.Insert(strayCert(t, 90, unknown)))
		require.NoError(t, d.Insert(strayCert(t, 100, unknown)))
		require.NoError(t, d.Insert(strayCert(t, 101, unknown)))
		require.Error(t, d.Insert(strayCert(t, 102, unknown)), "a certificate two above the seed entered without its parent")
	})

	t.Run("restored", func(t *testing.T) {
		const L, floor = 100, 100 - types.RescueWindow - types.WeakLinkWindow
		rounds := fullRounds(t, NewDAG(2000), 4, 105)
		d := NewDAG(2000)
		d.SetLastCommitRound(L)
		var tail []*types.Certificate
		for _, rr := range rounds[floor-5:] {
			tail = append(tail, rr...)
		}
		orphans, err := d.Restore(floor, tail)
		require.NoError(t, err)
		require.Empty(t, orphans)
		require.Equal(t, types.Round(floor), d.CollectedBelow())
		require.Nil(t, d.GetByDigest(rounds[floor-1][0].Digest()), "a round below the tail's floor was restored")

		require.NoError(t, d.Insert(strayCert(t, floor+types.WeakLinkWindow-1, unknown)))
		for _, r := range []types.Round{floor + types.WeakLinkWindow, L - types.RescueWindow, L - 1, L, L + 1} {
			require.Error(t, d.Insert(strayCert(t, r, unknown)), "a round-%d certificate entered the restored DAG without its parent", r)
		}
	})

	t.Run("collected", func(t *testing.T) {
		d := NewDAG(10)
		fullRounds(t, d, 4, 30)
		d.GarbageCollect(30) // rounds below 20 are gone
		require.NoError(t, d.Insert(strayCert(t, 20+types.WeakLinkWindow-1, unknown)))
		require.Error(t, d.Insert(strayCert(t, 20+types.WeakLinkWindow, unknown)))
	})
}

// A restored tail that is not closed under parents — a write lost to a
// power loss, or two inserters reaching the store out of order — leaves its
// orphans out, and their descendants with them, rather than insert a child
// whose parent its peers order with it (#4595). Restore returns them for
// the node to take in again. Within the weak-link window above the floor a
// parent may be below it, so there an absent parent is not asked for.
func TestRestore_DropsWhatLacksAParent(t *testing.T) {
	const floor = 60
	rounds := fullRounds(t, NewDAG(2000), 4, 90)
	lost := rounds[80][2]
	bandLost := rounds[floor+1][0]
	var tail []*types.Certificate
	for _, rr := range rounds[floor:] {
		for _, c := range rr {
			if c != lost && c != bandLost {
				tail = append(tail, c)
			}
		}
	}

	d := NewDAG(2000)
	d.SetLastCommitRound(70)
	orphans, err := d.Restore(floor, tail)
	require.NoError(t, err)
	require.Len(t, orphans, 4*10, "rounds 81 to 90 all descend from the lost certificate")
	for i, c := range orphans {
		require.Greater(t, c.Round(), types.Round(80))
		require.Nil(t, d.GetByDigest(c.Digest()))
		if i > 0 {
			require.GreaterOrEqual(t, c.Round(), orphans[i-1].Round())
		}
	}
	require.NotNil(t, d.GetByDigest(rounds[80][0].Digest()))
	require.NotNil(t, d.GetByDigest(rounds[floor+2][0].Digest()), "a child in the weak-link window was dropped")
}

// A certificate inserted is reported to the store (#4595 path 2): the
// persisted tail is then closed under parents whatever a weak link names.
func TestInsert_ReportsEachInsertOnce(t *testing.T) {
	d := NewDAG(10)
	var seen []types.CertificateDigest
	d.SetOnInsert(func(c *types.Certificate) { seen = append(seen, c.Digest()) })
	rounds := fullRounds(t, d, 2, 2)
	require.Len(t, seen, 6)
	require.NoError(t, d.Insert(rounds[2][0])) // idempotent re-insert
	require.Len(t, seen, 6, "a re-insert was reported again")
}
