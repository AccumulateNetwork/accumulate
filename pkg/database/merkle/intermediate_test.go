// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// #4263: the cascade computes every intermediate a proof needs, so the proof
// must read them rather than rebuild the Merkle state that held them.
//
// The stored pair must be exactly what the rebuild would have produced —
// otherwise a proof taken from storage would differ from one taken by
// computation, which is a consensus fault, not a performance question. This
// asserts equality across a chain, and that the receipts agree.
func TestIntermediate_StoredPairMatchesTheRebuild(t *testing.T) {
	const n = 1024
	c := testChain(begin(), 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}

	var checked, stored int
	for element := int64(1); element < n; element++ {
		for height := int64(1); height < 16; height++ {
			// what the rebuild produces
			hash, err := c.Entry(element)
			if err != nil {
				continue
			}
			s, err := c.StateAt(element - 1)
			if err != nil {
				continue
			}
			wantL, wantR, wantErr := getMerkleStateIntermediate(s, hash, height)

			// what is stored
			pair, err := c.Intermediate(uint64(element), uint64(height)).Get()
			if err != nil || len(pair) != 64 {
				// the cascade stopped before this height; the rebuild must
				// agree that there is nothing here
				require.Error(t, wantErr, "element %d height %d: nothing stored but the rebuild found a pair", element, height)
				continue
			}
			stored++
			require.NoError(t, wantErr, "element %d height %d: stored a pair the rebuild does not have", element, height)
			require.Equal(t, wantL, pair[:32], "element %d height %d: left", element, height)
			require.Equal(t, wantR, pair[32:], "element %d height %d: right", element, height)
			checked++
		}
	}
	require.NotZero(t, stored, "no intermediates were stored")
	t.Logf("%d stored intermediates over %d elements, every one equal to the rebuild", checked, n)
}

// And the receipt itself must be identical whether it was read or rebuilt.
func TestIntermediate_ReceiptIsTheSameEitherWay(t *testing.T) {
	const n = 512
	c := testChain(begin(), 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}

	for _, start := range []int64{0, 1, 7, 100, 255, 300, n - 1} {
		fromStore, err := c.Receipt(start, n-1)
		require.NoError(t, err, "start %d", start)

		// force the rebuild by going through getMerkleStateIntermediate
		rebuilt := new(Receipt)
		rebuilt.StartIndex, rebuilt.EndIndex = start, n-1
		rebuilt.Start, err = c.Entry(start)
		require.NoError(t, err)
		rebuilt.End, err = c.Entry(n - 1)
		require.NoError(t, err)
		anchorState, err := c.StateAt(n - 1)
		require.NoError(t, err)
		anchorState.trim()
		err = rebuilt.build(func(element, height int64) ([]byte, []byte, error) {
			hash, err := c.Entry(element)
			if err != nil {
				return nil, nil, err
			}
			s, err := c.StateAt(element - 1)
			if err != nil {
				return nil, nil, err
			}
			return getMerkleStateIntermediate(s, hash, height)
		}, anchorState)
		require.NoError(t, err, "start %d", start)

		require.Equal(t, rebuilt.Anchor, fromStore.Anchor, "start %d: anchor", start)
		require.Equal(t, len(rebuilt.Entries), len(fromStore.Entries), "start %d: length", start)
		for i := range rebuilt.Entries {
			require.Equal(t, rebuilt.Entries[i].Hash, fromStore.Entries[i].Hash, "start %d entry %d", start, i)
			require.Equal(t, rebuilt.Entries[i].Right, fromStore.Entries[i].Right, "start %d entry %d side", start, i)
		}
		require.True(t, fromStore.Validate(nil), "start %d: the receipt must verify", start)
	}
}

// countingStore records which kinds of record a chain reads and writes, so a
// test can assert what a proof costs rather than only what it returns.
type countingStore struct {
	inner database.Store
	get   map[string]int
	put   map[string]int
	drop  string // a record kind to discard writes for, to model an old chain
}

func newCountingStore(inner database.Store) *countingStore {
	return &countingStore{inner: inner, get: map[string]int{}, put: map[string]int{}}
}

func kindOf(k *record.Key) string {
	for i := k.Len() - 1; i >= 0; i-- {
		if s, ok := k.Get(i).(string); ok {
			return s
		}
	}
	return "?"
}

func (c *countingStore) GetValue(k *record.Key, v database.Value) error {
	c.get[kindOf(k)]++
	return c.inner.GetValue(k, v)
}

func (c *countingStore) PutValue(k *record.Key, v database.Value) error {
	if kind := kindOf(k); kind == c.drop {
		return nil
	} else {
		c.put[kind]++
	}
	return c.inner.PutValue(k, v)
}

// #4263: a proof must read the stored intermediates and rebuild nothing. The
// rebuild reads States, so a proof that reads no States record did not rebuild.
// This is the assertion the manual panic check was standing in for.
func TestIntermediate_ProofReadsNoState(t *testing.T) {
	const n = 4096
	cs := newCountingStore(begin())
	c := testChain(cs, 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}

	require.NoError(t, c.Commit())

	// One record per combine; see TestIntermediate_OneWritePerCombine for the
	// count in general, which is N minus the bits set in N
	require.Equal(t, combines(n), cs.put["Intermediate"], "one intermediate per combine")

	// A fresh chain over the same store, so every read is a real one
	cs.get = map[string]int{}
	c = testChain(cs, 8, "test")
	for _, start := range []int64{0, 1, 7, 100, 300, 1000, 2047, n - 1} {
		r, err := c.Receipt(start, n-1)
		require.NoError(t, err, "start %d", start)
		require.True(t, r.Validate(nil), "start %d", start)
	}
	// A proof legitimately reads the state at its anchor -- those are the
	// peaks it folds into -- and that is memoised, so eight proofs against
	// the same anchor read it once. What must not appear is a state read per
	// level, which is the rebuild this change removes.
	require.NotZero(t, cs.get["Intermediate"], "a proof must read the stored intermediates")
	require.LessOrEqual(t, cs.get["States"], 1,
		"a proof reads the anchor state and no more; it read %d States records", cs.get["States"])
}

// A chain written before the record existed has no intermediates. It must
// still produce correct receipts, by falling back to the rebuild.
func TestIntermediate_ChainWithoutThemStillProves(t *testing.T) {
	const n = 512
	cs := newCountingStore(begin())
	c := testChain(cs, 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}
	require.NoError(t, c.Commit())
	want, err := c.Receipt(7, n-1)
	require.NoError(t, err)

	// A second chain, with every intermediate write discarded — an old chain
	old := newCountingStore(begin())
	old.drop = "Intermediate"
	c2 := testChain(old, 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c2.AddEntry(d[:], false))
	}
	require.NoError(t, c2.Commit())
	require.Zero(t, old.put["Intermediate"], "the old chain must store none")

	old.get = map[string]int{}
	c2 = testChain(old, 8, "test")
	got, err := c2.Receipt(7, n-1)
	require.NoError(t, err)
	require.Greater(t, old.get["States"], 1,
		"without the intermediates the proof rebuilds state per level; it read %d", old.get["States"])
	require.Equal(t, want.Anchor, got.Anchor, "the receipt must be the same either way")
	require.Equal(t, len(want.Entries), len(got.Entries))
	for i := range want.Entries {
		require.Equal(t, want.Entries[i].Hash, got.Entries[i].Hash, "entry %d", i)
		require.Equal(t, want.Entries[i].Right, got.Entries[i].Right, "entry %d", i)
	}
	require.True(t, got.Validate(nil))
}

// What a proof costs, measured the same way in both arms: one chain size, one
// set of proofs, run twice over identical chains -- one that stored the
// cascade pairs and one that discarded them. Anything else is comparing two
// different experiments.
func TestIntermediate_WhatAProofCosts(t *testing.T) {
	const n = 4096
	starts := []int64{0, 1, 7, 100, 300, 1000, 2047, n - 1}

	build := func(drop bool) *countingStore {
		cs := newCountingStore(begin())
		if drop {
			cs.drop = "Intermediate"
		}
		c := testChain(cs, 8, "test")
		for i := 0; i < n; i++ {
			d := sha256.Sum256([]byte(fmt.Sprint(i)))
			require.NoError(t, c.AddEntry(d[:], false))
		}
		require.NoError(t, c.Commit())

		// A fresh chain over the same store, so every read is a real one
		cs.get = map[string]int{}
		c = testChain(cs, 8, "test")
		for _, s := range starts {
			r, err := c.Receipt(s, n-1)
			require.NoError(t, err, "start %d", s)
			require.True(t, r.Validate(nil), "start %d", s)
		}
		return cs
	}

	with, without := build(false), build(true)
	p := len(starts)
	t.Logf("%d proofs over %d elements, per proof:", p, n)
	t.Logf("  with the record:    %5.1f intermediate, %5.1f state, %5.1f element reads",
		float64(with.get["Intermediate"])/float64(p), float64(with.get["States"])/float64(p), float64(with.get["Element"])/float64(p))
	t.Logf("  without it:         %5.1f intermediate, %5.1f state, %5.1f element reads",
		float64(without.get["Intermediate"])/float64(p), float64(without.get["States"])/float64(p), float64(without.get["Element"])/float64(p))

	// The point of the change: the per-level state rebuild is gone.
	require.Greater(t, without.get["States"], with.get["States"],
		"the rebuild must read more state than the read does")
	require.LessOrEqual(t, with.get["States"], 1,
		"with the record a proof reads the anchor state and no more; it read %d", with.get["States"])
}

func benchChain(b *testing.B, n int, drop bool) *Chain {
	cs := newCountingStore(begin())
	if drop {
		cs.drop = "Intermediate"
	}
	c := testChain(cs, 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		if err := c.AddEntry(d[:], false); err != nil {
			b.Fatal(err)
		}
	}
	if err := c.Commit(); err != nil {
		b.Fatal(err)
	}
	return testChain(cs, 8, "test")
}

// The cost a proof actually pays. The read count understates the difference:
// the rebuild's expense is StateAt replaying up to a mark frequency of entries
// in CPU, not the reads it makes.
func BenchmarkReceipt(b *testing.B) {
	for _, n := range []int{4096, 65536} {
		for _, arm := range []struct {
			name string
			drop bool
		}{{"stored", false}, {"rebuilt", true}} {
			b.Run(fmt.Sprintf("%d/%s", n, arm.name), func(b *testing.B) {
				c := benchChain(b, n, arm.drop)
				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := c.Receipt(int64(i%(n-1)), int64(n-1)); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
