// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package synthcache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func misses() uint64 { return Stats().Misses["block"] }

// A block the cache decided it does not need -- below where the restart seed
// began, or empty and trimmed past the horizon -- receipted late has nothing
// to send: the lookup finds nothing and is not a miss. A block the memory
// bound dropped held undelivered entries: the lookup stays a miss (#4467).
func TestALookupBelowWhatTheCacheAnswersForIsNotAMiss(t *testing.T) {
	t.Run("below the seed", func(t *testing.T) {
		c := New(0)
		c.Seed([]*Block{{Index: 89}})
		c.SeededFrom(89)
		before := misses()
		_, ok := c.Block(20)
		require.False(t, ok)
		require.False(t, c.Answers(20))
		require.Equal(t, before, misses(), "a block below the seed owes nothing and is not a miss")

		_, ok = c.Block(500)
		require.False(t, ok)
		require.True(t, c.Answers(500))
		require.Equal(t, before+1, misses(), "a block at or above the seed that is not held is a miss")
	})

	t.Run("empty past the horizon", func(t *testing.T) {
		c := New(10)
		for i := uint64(1); i <= 30; i++ {
			tx := c.Begin(i)
			tx.SetBlock(&Block{Index: i})
			tx.Commit()
		}
		before := misses()
		_, ok := c.Block(12)
		require.False(t, ok, "precondition: the empty block 12 is past the horizon and dropped")
		require.False(t, c.Answers(12))
		require.Equal(t, before, misses(), "an empty block trimmed past the horizon is not a miss")
	})

	t.Run("dropped by the bound", func(t *testing.T) {
		c := New(0)
		for i := uint64(1); i <= 3; i++ {
			tx := c.Begin(i)
			tx.Add(entry(i, "acc://bvn-BVN1.acme", i, int64(i)))
			tx.SetBlock(&Block{Index: i})
			tx.Commit()
		}
		c.SetBound(2, 10)
		tx := c.Begin(4)
		tx.SetBlock(&Block{Index: 4})
		tx.Commit()
		c.SeededFrom(5) // even below where the cache answers from
		before := misses()
		_, ok := c.Block(1)
		require.False(t, ok, "precondition: the bound dropped block 1 and its undelivered entry")
		require.True(t, c.Answers(1))
		require.Equal(t, before+1, misses(), "a block the bound dropped held undelivered entries: a miss")
	})
}
