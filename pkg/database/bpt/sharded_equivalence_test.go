// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// These tests do not prove that a Merkle tree can be split and recombined.
// That is irrefutable and needs no test: the BPT routes on bits of the key
// hash, so its shape is a function of the key set alone, and splitting at a
// bit boundary is the same hash computation reassociated —
// H(H(a,b), H(c,d)) whichever goroutine computed which half. Were that not so,
// a root would not identify the contents and Merkle trees would be useless
// cryptographically.
//
// What these tests check is that OUR IMPLEMENTATION follows the math. The risk
// is not arithmetic, it is convention: sharded.go warns that BPT routing
// inverts bits, so combining adjacent shards must be hash(i+1, i) and not
// hash(i, i+1), and "any change to this logic will break correctness". A
// mutation test confirms it — swapping that pairing fails every depth. Empty
// shards, delete-collapse and iteration order are the same kind of hazard.
//
// So: the math is assumed, the code is tested. TestRootHashEquivalence covers
// depths 2, 4 and 5 with sequential inserts; these cover what it does not —
// the default depth, both ends of the valid range, mutation, and orderings.

// bothPaths builds a sharded and an unsharded BPT over independent stores.
func bothPaths(t *testing.T, depth int) (*ShardedBPT, *BPT) {
	t.Helper()
	kvs1 := memory.New(nil).Begin(nil, true)
	t.Cleanup(func() { kvs1.Discard() })
	kvs2 := memory.New(nil).Begin(nil, true)
	t.Cleanup(func() { kvs2.Discard() })

	key := record.NewKey("test")
	sharded, err := NewShardedBPT(keyvalue.RecordStore{Store: kvs1}, key, depth)
	require.NoError(t, err)
	return sharded, New(nil, nil, keyvalue.RecordStore{Store: kvs2}, key)
}

func val(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func roots(t *testing.T, s *ShardedBPT, p *BPT) ([32]byte, [32]byte) {
	t.Helper()
	a, err := s.GetRootHash()
	require.NoError(t, err)
	b, err := p.GetRootHash()
	require.NoError(t, err)
	return a, b
}

// Every valid depth must agree, including 6 — the default — and both ends of
// the 1..8 range that NewShardedBPT accepts.
func TestEquivalence_EveryValidDepth(t *testing.T) {
	for depth := 1; depth <= 8; depth++ {
		t.Run(fmt.Sprintf("depth-%d/%d-shards", depth, 1<<depth), func(t *testing.T) {
			sharded, plain := bothPaths(t, depth)
			for i := 0; i < 500; i++ {
				k := record.NewKey(fmt.Sprintf("key-%d", i))
				v := val(fmt.Sprintf("value-%d", i))
				require.NoError(t, sharded.Insert(k, v))
				require.NoError(t, plain.Insert(k, v))
			}
			a, b := roots(t, sharded, plain)
			require.Equal(t, b, a, "depth %d (%d shards) must match the unsharded root", depth, 1<<depth)
		})
	}
}

// An empty tree has a root too, and the two paths must agree on it — otherwise
// a fresh node disagrees with a fresh node before it has done anything.
func TestEquivalence_EmptyTree(t *testing.T) {
	for _, depth := range []int{1, 6, 8} {
		sharded, plain := bothPaths(t, depth)
		a, b := roots(t, sharded, plain)
		require.Equal(t, b, a, "an empty tree must agree at depth %d", depth)
	}
}

// A single entry, which lands in exactly one shard and leaves the rest empty.
func TestEquivalence_SingleEntry(t *testing.T) {
	sharded, plain := bothPaths(t, 6)
	k, v := record.NewKey("only"), val("only")
	require.NoError(t, sharded.Insert(k, v))
	require.NoError(t, plain.Insert(k, v))
	a, b := roots(t, sharded, plain)
	require.Equal(t, b, a, "one entry across 64 shards must still agree")
}

// Overwriting a key must converge, not diverge.
func TestEquivalence_Updates(t *testing.T) {
	sharded, plain := bothPaths(t, 6)
	for i := 0; i < 200; i++ {
		k := record.NewKey(fmt.Sprintf("key-%d", i%50)) // deliberate collisions
		v := val(fmt.Sprintf("value-%d", i))
		require.NoError(t, sharded.Insert(k, v))
		require.NoError(t, plain.Insert(k, v))
	}
	a, b := roots(t, sharded, plain)
	require.Equal(t, b, a, "repeated overwrites must agree")
}

// Deletes must agree, including deleting everything back to empty.
func TestEquivalence_Deletes(t *testing.T) {
	sharded, plain := bothPaths(t, 6)
	var keys []*record.Key
	for i := 0; i < 300; i++ {
		k := record.NewKey(fmt.Sprintf("key-%d", i))
		keys = append(keys, k)
		v := val(fmt.Sprintf("value-%d", i))
		require.NoError(t, sharded.Insert(k, v))
		require.NoError(t, plain.Insert(k, v))
	}

	// Delete half, interleaved.
	for i := 0; i < len(keys); i += 2 {
		require.NoError(t, sharded.Delete(keys[i]))
		require.NoError(t, plain.Delete(keys[i]))
	}
	a, b := roots(t, sharded, plain)
	require.Equal(t, b, a, "after deleting half, the roots must agree")

	// Delete the rest.
	for i := 1; i < len(keys); i += 2 {
		require.NoError(t, sharded.Delete(keys[i]))
		require.NoError(t, plain.Delete(keys[i]))
	}
	a, b = roots(t, sharded, plain)
	require.Equal(t, b, a, "after deleting everything, the roots must agree")
}

// The root must depend on the SET, not on the order it was built in. Sharded
// inserts land in different shards and are combined afterwards, so an
// order-dependent result here would mean two validators applying the same
// block in different orders diverge.
func TestEquivalence_InsertionOrderDoesNotMatter(t *testing.T) {
	const n = 400
	build := func(order []int) [32]byte {
		sharded, _ := bothPaths(t, 6)
		for _, i := range order {
			require.NoError(t, sharded.Insert(
				record.NewKey(fmt.Sprintf("key-%d", i)), val(fmt.Sprintf("value-%d", i))))
		}
		h, err := sharded.GetRootHash()
		require.NoError(t, err)
		return h
	}

	forward := make([]int, n)
	for i := range forward {
		forward[i] = i
	}
	reverse := make([]int, n)
	for i := range reverse {
		reverse[i] = n - 1 - i
	}
	shuffled := make([]int, n)
	copy(shuffled, forward)
	rand.New(rand.NewSource(42)).Shuffle(n, func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	a, b, c := build(forward), build(reverse), build(shuffled)
	require.Equal(t, a, b, "reverse insertion must produce the same root")
	require.Equal(t, a, c, "shuffled insertion must produce the same root")
}

// Keys in real use are hashes of account URLs, not a tidy sequence. Clustered
// keys concentrate into fewer shards and are the case most likely to expose a
// combining bug.
func TestEquivalence_ClusteredKeys(t *testing.T) {
	sharded, plain := bothPaths(t, 6)
	for i := 0; i < 300; i++ {
		// A shared prefix, so the keys crowd together.
		k := record.NewKey(fmt.Sprintf("acc://alice.acme/tokens/%04d", i))
		v := val(fmt.Sprintf("value-%d", i))
		require.NoError(t, sharded.Insert(k, v))
		require.NoError(t, plain.Insert(k, v))
	}
	a, b := roots(t, sharded, plain)
	require.Equal(t, b, a, "clustered keys must agree")
}

// Reading back must agree too: a matching root over differing contents would
// be worse than a mismatch, because nothing would report it.
func TestEquivalence_ValuesReadBackIdentically(t *testing.T) {
	sharded, plain := bothPaths(t, 6)
	var keys []*record.Key
	for i := 0; i < 200; i++ {
		k := record.NewKey(fmt.Sprintf("key-%d", i))
		keys = append(keys, k)
		v := val(fmt.Sprintf("value-%d", i))
		require.NoError(t, sharded.Insert(k, v))
		require.NoError(t, plain.Insert(k, v))
	}
	for _, k := range keys {
		sv, err := sharded.Get(k)
		require.NoError(t, err)
		pv, err := plain.Get(k)
		require.NoError(t, err)
		require.Equal(t, pv, sv, "value for %v must match", k)
	}
}
