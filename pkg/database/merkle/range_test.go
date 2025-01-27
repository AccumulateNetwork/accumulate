// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func b2i(b []byte) int64 {
	i := int64(b[0])<<24 + int64(b[1])<<16 + int64(b[2])<<8 + int64(b[3])
	return i
}

func i2b(i int64) []byte {
	b := [32]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
	return b[:]
}

func TestConversions(t *testing.T) {
	for i := int64(0); i < 10000; i++ {
		if i != b2i(i2b(i)) {
			t.Fatalf("failed %d", i)
		}

	}
}

func TestMerkleManager_GetRange(t *testing.T) {
	for i := int64(1); i < 6; i++ {
		for NumTests := int64(50); NumTests < 64; NumTests++ {

			var rh common.RandHash
			store := begin()
			mm := testChain(store, i, "try")
			for i := int64(0); i < NumTests; i++ {
				require.NoError(t, mm.AddEntry(rh.NextList(), false))
			}
			for begin := int64(-1); begin < NumTests+1; begin++ {
				for end := begin - 1; end < NumTests+2; end++ {

					hashes, err := mm.Entries(begin, end)

					if begin < 0 || begin > end || begin >= NumTests {
						require.Errorf(t, err, "should not allow range [%d,%d] (power %d)", begin, end, i)
					} else {
						require.NoErrorf(t, err, "should have a range for [%d,%d] (power %d)", begin, end, i)
						e := end
						if e > NumTests {
							e = NumTests
						}
						require.Truef(t, len(hashes) == int(e-begin),
							"returned the wrong length for [%d,%d] %d (power %d)", begin, end, len(hashes), i)
						for k, h := range rh.List[begin:e] {
							require.Truef(t, bytes.Equal(hashes[k], h),
								"[%d,%d]returned wrong values (power %d)", begin, end, i)
						}
					}
				}
			}
		}
	}
}

// TestEntriesWithIncompleteState tests entry retrieval with incomplete state.
func TestEntriesWithIncompleteState(t *testing.T) {
	var rh common.RandHash
	store := begin()
	c := testChain(store, 2, "partial") // Create test chain with power 2

	// First mark point
	s1 := new(State)
	for (s1.Count+1)&c.markMask > 0 {
		s1.AddEntry(rh.NextList())
	}
	s1.AddEntry(rh.NextList())

	// Second mark point
	s2 := s1.Copy()
	s2.HashList = s2.HashList[:0]
	for (s2.Count+1)&c.markMask > 0 {
		s2.AddEntry(rh.NextList())
	}
	s2.AddEntry(rh.NextList())

	// Third mark point
	s3 := s2.Copy()
	s3.HashList = s3.HashList[:0]
	for (s3.Count+1)&c.markMask > 0 {
		s3.AddEntry(rh.NextList())
	}
	s3.AddEntry(rh.NextList())

	// Head
	s4 := s3.Copy()
	s4.HashList = s4.HashList[:0]
	s4.AddEntry(rh.NextList())

	// Save the head and mark points except the first
	err := c.States(uint64(s2.Count - 1)).Put(s2)
	require.NoError(t, err)
	err = c.States(uint64(s3.Count - 1)).Put(s3)
	require.NoError(t, err)
	err = c.Head().Put(s4)
	require.NoError(t, err)

	tests := []struct {
		begin int64
		end   int64
	}{
		// Within the second mark point
		{s2.Count - 1, s2.Count},
		// Across the second and third
		{s2.Count - 1, s2.Count + 1},
		// Across the third and head
		{s3.Count - 1, s3.Count + 1},

		// Across the first and second
		{s1.Count - 1, s1.Count + 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Range %d:%d", tt.begin, tt.end), func(t *testing.T) {
			entries, err := c.Entries(tt.begin, tt.end)
			require.NoError(t, err)
			require.Equal(t, rh.List[tt.begin:tt.end], entries)
		})
	}

	for _, s := range []int64{s1.Count - 2, s2.Count - 2, s3.Count - 2} {
		t.Run(fmt.Sprintf("State at %d", s), func(t *testing.T) {
			_, err := c.StateAt(s)
			require.NoError(t, err)
		})
	}
}

// TestPartialTree verifies that a Merkle tree can be correctly reconstructed from a partial state.
// Could not follow the logic of TestEntriesWithIncompleteState, so I rewrote the test.
//
// 1. Building a chain with 1000 entries (A)
// 2. Created a partial chain by starting it from a state extracted from A (B)
// 3. Demonstrated that all elements of B can match what is in A from the initial state to the end of A
// 4. Demonstrated that all elements of B match what is in A from the first mark point before the state B holds
// 5. Demonstrated that no elements from the first element of A to the mark point before the initial state are in B
func TestPartialTree(t *testing.T) {
	var rh common.RandHash
	store := begin()

	// Create merkle tree A with 1000 entries
	mmA := testChain(store, 8, "treeA") // power 2 for reasonable mark spacing
	for i := 0; i < 1000; i++ {
		require.NoError(t, mmA.AddEntry(rh.NextList(), false))
	}

	// Get the state at 700th element
	state700, err := mmA.StateAt(699) // 0-based index
	require.NoError(t, err)

	// Create truncated merkle tree B starting with state700
	mmB := testChain(store, 8, "treeB")
	err = mmB.Head().Put(state700)
	require.NoError(t, err)

	// Add the next 300 hashes from A to B
	for i := 700; i < 1000; i++ {
		require.NoError(t, mmB.AddEntry(rh.List[i], false))
	}

	// Get final states of both trees
	stateA, err := mmA.Head().Get()
	require.NoError(t, err)
	stateB, err := mmB.Head().Get()
	require.NoError(t, err)

	// Compare the final states
	require.Equal(t, stateA.Count, stateB.Count, "final counts should match")
	require.Equal(t, stateA.Anchor(), stateB.Anchor(), "final anchor hashes should match")

	// Compare range queries between trees
	entriesA, err := mmA.Entries(950, 975)
	require.NoError(t, err, "failed to get entries from tree A")
	entriesB, err := mmB.Entries(950, 975)
	require.NoError(t, err, "failed to get entries from tree B")
	require.Equal(t, entriesA, entriesB, "entries from range [950,975] should match between trees")

	// Verify all entries in tree A match the original entries
	entriesA, err = mmA.Entries(0, 1000)
	require.NoError(t, err, "failed to get all entries from tree A")
	require.Equal(t, rh.List[:1000], entriesA, "all entries in tree A should match original entries")

	// Verify tree B cannot access entries before its starting point
	_, err = mmB.Entries(0, 700)
	require.Error(t, err, "tree B should not be able to access entries before 700")

	// Verify range 700-1000 is identical in both trees and matches original entries
	entriesA, err = mmA.Entries(700, 1000)
	require.NoError(t, err, "failed to get range from tree A")
	entriesB, err = mmB.Entries(700, 1000)
	require.NoError(t, err, "failed to get range from tree B")
	require.Equal(t, rh.List[700:1000], entriesA, "entries from A should match original")
	require.Equal(t, entriesA, entriesB, "entries should match between trees")

	// With power=8, mark points are at multiples of 256
	// Tree B starts at 700, which is between mark points 512 and 768
	// B should have the complete state from mark point 512
	entriesB, err = mmB.Entries(512, 768)
	require.NoError(t, err, "tree B should be able to access entries from mark point 512")
	require.Equal(t, rh.List[512:768], entriesB, "entries from mark point 512 should match original")

	// Verify B cannot access entries before mark point 512
	_, err = mmB.Entries(256, 512)
	require.Error(t, err, "tree B should not be able to access entries before mark point 512")

	// Test precise boundary behavior at mark point 512
	_, err = mmB.Entries(511, 512)
	require.Error(t, err, "tree B should not be able to access entry 511 (before mark point)")

	entriesB, err = mmB.Entries(512, 513)
	require.NoError(t, err, "tree B should be able to access entry 512 (mark point)")
	require.Equal(t, rh.List[512:513], entriesB, "entry 512 should match original")

	entriesB, err = mmB.Entries(513, 514)
	require.NoError(t, err, "tree B should be able to access entry 513 (after mark point)")
	require.Equal(t, rh.List[513:514], entriesB, "entry 513 should match original")
}
