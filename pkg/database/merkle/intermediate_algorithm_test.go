// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"crypto/sha256"
	"fmt"
	"math/bits"
	"testing"

	"github.com/stretchr/testify/require"
)

// The properties the stored-cascade algorithm claims (#4263), tested rather
// than asserted in prose: a proof costs one read per level and nothing per
// entry, and it returns exactly what the rebuild returned.

func hashOf(i int) []byte {
	d := sha256.Sum256([]byte(fmt.Sprint(i)))
	return d[:]
}

// buildChain fills a chain and returns it reopened over the same store, so
// every read a proof makes is a real one rather than a hit on the write cache.
func buildChain(t testing.TB, n int, markPower int64, drop bool) (*Chain, *countingStore) {
	t.Helper()
	cs := newCountingStore(begin())
	if drop {
		cs.drop = "Intermediate"
	}
	c := testChain(cs, markPower, "test")
	for i := 0; i < n; i++ {
		require.NoError(t, c.AddEntry(hashOf(i), false))
	}
	require.NoError(t, c.Commit())
	cs.get = map[string]int{}
	return testChain(cs, markPower, "test"), cs
}

func (c *countingStore) reads() int {
	n := 0
	for _, v := range c.get {
		n += v
	}
	return n
}

// EFFICIENCY. The bar is one read per level of the tree: a chain of four
// billion entries has 32 levels, so a proof over it must cost about 32 reads,
// not a number that grows with the chain.
func TestIntermediate_ReadsAreLogarithmic(t *testing.T) {
	const proofs = 16
	perLevel := map[int]float64{}
	for _, n := range []int{1024, 4096, 16384, 65536} {
		c, cs := buildChain(t, n, 8, false)
		for j := 0; j < proofs; j++ {
			r, err := c.Receipt(int64(j)*int64(n)/proofs, int64(n-1))
			require.NoError(t, err)
			require.True(t, r.Validate(nil))
		}
		levels := bits.Len(uint(n - 1))
		per := float64(cs.reads()) / proofs
		perLevel[n] = per
		t.Logf("n=%6d  %2d levels  %5.1f reads/proof  (%.2f per level)", n, levels, per, per/float64(levels))

		// One read per level, with room for the anchor state and the head
		require.LessOrEqual(t, per, float64(levels)+2,
			"n=%d: %.1f reads for %d levels is more than one per level", n, per, levels)
	}

	// Sixteen times the chain must not cost sixteen times the proof. Four
	// doublings add four levels, so the cost may rise by about four reads.
	growth := perLevel[65536] - perLevel[1024]
	require.LessOrEqual(t, growth, 8.0,
		"64x the entries added %.1f reads per proof; the cost must grow with the logarithm, not the chain", growth)

	// The bar stated on #4263, extrapolated on the slope just measured: four
	// billion entries is 32 levels.
	slope := (perLevel[65536] - perLevel[1024]) / float64(bits.Len(65535)-bits.Len(1023))
	atFourBillion := perLevel[65536] + slope*float64(32-bits.Len(65535))
	t.Logf("extrapolated to 2^32 entries: %.0f reads per proof", atFourBillion)
	require.LessOrEqual(t, atFourBillion, 40.0,
		"a proof over four billion entries would cost %.0f reads", atFourBillion)
}

// EFFICIENCY. The rebuild's expense was that it replayed up to a mark
// frequency of entries, so its cost rose with markPower while the tree did
// not change. Reading is independent of it.
func TestIntermediate_CostDoesNotDependOnMarkFrequency(t *testing.T) {
	const n, proofs = 16384, 8
	var first float64
	for i, markPower := range []int64{2, 4, 8, 10, 12} {
		c, cs := buildChain(t, n, markPower, false)
		for j := 0; j < proofs; j++ {
			_, err := c.Receipt(int64(j)*int64(n)/proofs, int64(n-1))
			require.NoError(t, err)
		}
		per := float64(cs.reads()) / proofs
		t.Logf("markPower=%2d (every %5d entries): %5.1f reads/proof, %.2f state reads",
			markPower, int64(1)<<markPower, per, float64(cs.get["States"])/proofs)
		if i == 0 {
			first = per
		}
		require.InDelta(t, first, per, 1.0,
			"markPower %d changed the cost of a proof from %.1f to %.1f reads", markPower, first, per)

		// The per-level state rebuild is what mark frequency governed
		require.LessOrEqual(t, cs.get["States"], 1,
			"markPower %d: a proof rebuilt %d states", markPower, cs.get["States"])
	}
}

// combines is how many times the cascade combines a pair while N entries are
// added: one per carry, which is N minus the number of bits set in N. It is
// N-1 only when N is a power of two -- the open mark set holds one hash per
// set bit, and those have not been combined with anything yet.
func combines(n int) int { return n - bits.OnesCount(uint(n)) }

// EFFICIENCY. One record per combine, and never more than one per entry, so
// the write cost is a constant per entry rather than one per level.
func TestIntermediate_OneWritePerCombine(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 17, 100, 255, 256, 1000, 1024} {
		cs := newCountingStore(begin())
		c := testChain(cs, 8, "test")
		for i := 0; i < n; i++ {
			require.NoError(t, c.AddEntry(hashOf(i), false))
		}
		require.NoError(t, c.Commit())
		require.Equal(t, combines(n), cs.put["Intermediate"],
			"%d entries perform %d combines", n, combines(n))
		require.LessOrEqual(t, cs.put["Intermediate"], n,
			"never more than one record per entry")
	}

	// Each is a pair of hashes and nothing else
	c, _ := buildChain(t, 64, 8, false)
	pair, err := c.Intermediate(1, 1).Get()
	require.NoError(t, err)
	require.Len(t, pair, 64)
}

// CORRECTNESS. Which heights exist is answered by arithmetic, without asking
// the store. That answer must be exactly the set the cascade actually wrote,
// and exactly the set the rebuild can produce -- if it claims a height that
// is absent, proofs break; if it denies one that exists, they break quietly
// by falling back.
func TestIntermediate_ArithmeticMatchesWhatWasWritten(t *testing.T) {
	for v, want := range map[uint64]int{0: 0, 1: 1, 2: 0, 3: 2, 4: 0, 7: 3, 8: 0, 255: 8, 256: 0} {
		require.Equal(t, want, trailingOnes(v), "trailingOnes(%d)", v)
	}

	const n = 1024
	c, _ := buildChain(t, n, 8, false)
	for element := int64(0); element < n; element++ {
		want := trailingOnes(uint64(element))

		// what the cascade wrote
		stored := 0
		for h := int64(1); h < 20; h++ {
			if pair, err := c.Intermediate(uint64(element), uint64(h)).Get(); err == nil && len(pair) == 64 {
				stored++
			}
		}
		require.Equal(t, want, stored,
			"element %d: arithmetic says %d heights, the cascade wrote %d", element, want, stored)

		// what the rebuild can produce, for the first element after each
		// boundary and a sample besides -- the rebuild is expensive
		if element%97 != 0 && element != n-1 {
			continue
		}
		hash, err := c.Entry(element)
		require.NoError(t, err)
		s, err := c.StateAt(element - 1)
		require.NoError(t, err)
		rebuilt := 0
		for h := int64(1); h < 20; h++ {
			if _, _, err := getMerkleStateIntermediate(s, hash, h); err == nil {
				rebuilt++
			}
		}
		require.Equal(t, want, rebuilt,
			"element %d: arithmetic says %d heights, the rebuild produces %d", element, want, rebuilt)
	}
}

// CORRECTNESS. Every proof over the chain, read and rebuilt, must be the same
// receipt. Sizes that are not powers of two and mark powers that divide them
// awkwardly are where a cascade off by one would show.
func TestIntermediate_EveryReceiptIsIdenticalEitherWay(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 8, 9, 15, 16, 17, 33, 100} {
		for _, markPower := range []int64{1, 2, 4, 8} {
			stored, _ := buildChain(t, n, markPower, false)
			rebuilt, old := buildChain(t, n, markPower, true)
			require.Zero(t, old.put["Intermediate"], "the old chain must store none")

			for from := int64(0); from < int64(n); from++ {
				for to := from; to < int64(n); to++ {
					a, errA := stored.Receipt(from, to)
					b, errB := rebuilt.Receipt(from, to)
					if errA != nil || errB != nil {
						require.Equal(t, errA == nil, errB == nil,
							"n=%d mp=%d %d->%d: one path failed and the other did not (%v / %v)",
							n, markPower, from, to, errA, errB)
						continue
					}
					require.Equal(t, b.Anchor, a.Anchor, "n=%d mp=%d %d->%d anchor", n, markPower, from, to)
					require.Equal(t, b.Start, a.Start, "n=%d mp=%d %d->%d start", n, markPower, from, to)
					require.Equal(t, len(b.Entries), len(a.Entries), "n=%d mp=%d %d->%d entries", n, markPower, from, to)
					for i := range b.Entries {
						require.Equal(t, b.Entries[i].Hash, a.Entries[i].Hash, "n=%d mp=%d %d->%d entry %d", n, markPower, from, to, i)
						require.Equal(t, b.Entries[i].Right, a.Entries[i].Right, "n=%d mp=%d %d->%d entry %d right", n, markPower, from, to, i)
					}
					require.True(t, a.Validate(nil), "n=%d mp=%d %d->%d does not validate", n, markPower, from, to)
				}
			}
		}
	}
}

// CORRECTNESS, and the shape a deployment actually takes: a chain that
// existed before the change and keeps growing after it. Entries written
// before have no stored pair and must fall back; entries written after have
// one and must read it; a proof that spans the boundary uses both.
func TestIntermediate_PartialMigration(t *testing.T) {
	const n, upgrade = 512, 200

	cs := newCountingStore(begin())
	cs.drop = "Intermediate" // the old build
	c := testChain(cs, 8, "test")
	for i := 0; i < n; i++ {
		if i == upgrade {
			require.NoError(t, c.Commit())
			cs.drop = "" // the node restarts on a build that stores them
			c = testChain(cs, 8, "test")
		}
		require.NoError(t, c.AddEntry(hashOf(i), false))
	}
	require.NoError(t, c.Commit())

	// Only the combines performed after the upgrade were recorded
	require.Equal(t, combines(n)-combines(upgrade), cs.put["Intermediate"],
		"the chain must hold the pairs from after the upgrade and no others")

	// A chain of the same entries, wholly rebuilt, is the reference
	ref, _ := buildChain(t, n, 8, true)
	mixed := testChain(cs, 8, "test")

	for from := int64(0); from < int64(n); from += 7 {
		for _, to := range []int64{int64(upgrade) - 1, int64(upgrade), int64(upgrade) + 1, n - 1} {
			if from > to {
				continue
			}
			want, err := ref.Receipt(from, to)
			require.NoError(t, err, "%d->%d", from, to)
			got, err := mixed.Receipt(from, to)
			require.NoError(t, err, "%d->%d", from, to)
			require.Equal(t, want.Anchor, got.Anchor, "%d->%d anchor across the upgrade", from, to)
			require.Equal(t, len(want.Entries), len(got.Entries), "%d->%d", from, to)
			for i := range want.Entries {
				require.Equal(t, want.Entries[i].Hash, got.Entries[i].Hash, "%d->%d entry %d", from, to, i)
			}
			require.True(t, got.Validate(nil), "%d->%d does not validate", from, to)
		}
	}
}
