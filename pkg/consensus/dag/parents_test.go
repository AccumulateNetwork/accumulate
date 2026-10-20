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

// The two legitimate exceptions: a seed, where the DAG accepts certificates
// at S and S+1 without their parents (consensus.md, "Restart"), and the
// collected rounds, whose certificates cannot be checked against what a
// weak link may still name.
func TestInsert_ParentsWaivedOnlyAtTheSeedAndBelowCollection(t *testing.T) {
	t.Run("seed", func(t *testing.T) {
		d := NewDAG(2000)
		d.SetLastCommitRound(100)
		unknown := types.CertificateDigest{0xBB}
		require.NoError(t, d.Insert(strayCert(t, 100, unknown)))
		require.NoError(t, d.Insert(strayCert(t, 101, unknown)))
		require.Error(t, d.Insert(strayCert(t, 102, unknown)), "a certificate two above the seed entered without its parent")
	})

	t.Run("collected", func(t *testing.T) {
		d := NewDAG(10)
		fullRounds(t, d, 4, 30)
		d.GarbageCollect(30) // rounds below 20 are gone
		unknown := types.CertificateDigest{0xCC}
		require.NoError(t, d.Insert(strayCert(t, 20+types.WeakLinkWindow, unknown)))
		require.Error(t, d.Insert(strayCert(t, 21+types.WeakLinkWindow, unknown)))
	})
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
