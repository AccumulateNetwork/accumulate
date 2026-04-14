// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestNewShardedExecutor(t *testing.T) {
	t.Run("valid power of two", func(t *testing.T) {
		for _, n := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256} {
			se, err := NewShardedExecutor(n, nil)
			require.NoError(t, err, "shard count %d", n)
			require.Equal(t, n, se.ShardCount())
		}
	})

	t.Run("invalid not power of two", func(t *testing.T) {
		for _, n := range []int{3, 5, 6, 7, 9, 10, 100} {
			_, err := NewShardedExecutor(n, nil)
			require.Error(t, err)
		}
	})

	t.Run("invalid zero", func(t *testing.T) {
		_, err := NewShardedExecutor(0, nil)
		require.Error(t, err)
	})

	t.Run("invalid too large", func(t *testing.T) {
		_, err := NewShardedExecutor(512, nil)
		require.Error(t, err)
	})
}

func TestRouteAccount(t *testing.T) {
	t.Run("single shard routes all to zero", func(t *testing.T) {
		se, err := NewShardedExecutor(1, nil)
		require.NoError(t, err)
		for i := 0; i < 100; i++ {
			u, err := url.Parse(fmt.Sprintf("acc://account%d.acme", i))
			require.NoError(t, err)
			require.Equal(t, 0, se.RouteAccount(u))
		}
	})

	t.Run("deterministic routing", func(t *testing.T) {
		se, err := NewShardedExecutor(16, nil)
		require.NoError(t, err)

		u, err := url.Parse("acc://example.acme/tokens")
		require.NoError(t, err)

		first := se.RouteAccount(u)
		for i := 0; i < 100; i++ {
			require.Equal(t, first, se.RouteAccount(u), "routing must be deterministic")
		}
	})

	t.Run("routes within range", func(t *testing.T) {
		for _, n := range []int{2, 4, 8, 16, 64, 256} {
			se, err := NewShardedExecutor(n, nil)
			require.NoError(t, err)
			for i := 0; i < 200; i++ {
				u, err := url.Parse(fmt.Sprintf("acc://test%d.acme/tokens", i))
				require.NoError(t, err)
				id := se.RouteAccount(u)
				require.True(t, id >= 0 && id < n,
					"shard count %d: route %d out of range for account %s", n, id, u)
			}
		}
	})

	t.Run("same ADI routes to same shard", func(t *testing.T) {
		se, err := NewShardedExecutor(16, nil)
		require.NoError(t, err)

		// All accounts under the same ADI must route to the same shard.
		base, _ := url.Parse("acc://example.acme")
		tokens, _ := url.Parse("acc://example.acme/tokens")
		data, _ := url.Parse("acc://example.acme/data")
		book, _ := url.Parse("acc://example.acme/book")

		shard := se.RouteAccount(base)
		require.Equal(t, shard, se.RouteAccount(tokens))
		require.Equal(t, shard, se.RouteAccount(data))
		require.Equal(t, shard, se.RouteAccount(book))
	})

	t.Run("distribution across shards", func(t *testing.T) {
		se, err := NewShardedExecutor(4, nil)
		require.NoError(t, err)

		counts := make([]int, 4)
		for i := 0; i < 1000; i++ {
			u, err := url.Parse(fmt.Sprintf("acc://user%d.acme/tokens", i))
			require.NoError(t, err)
			counts[se.RouteAccount(u)]++
		}

		// Each shard should get some accounts (rough check: at least 10%)
		for i, c := range counts {
			require.True(t, c > 100, "shard %d only got %d/1000 accounts", i, c)
		}
	})
}

func TestAccountsPerShard(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	var accounts []*url.URL
	for i := 0; i < 20; i++ {
		u, err := url.Parse(fmt.Sprintf("acc://acct%d.acme", i))
		require.NoError(t, err)
		accounts = append(accounts, u)
	}

	result := se.AccountsPerShard(accounts)

	// Every account should appear exactly once across all shards.
	total := 0
	for _, urls := range result {
		total += len(urls)
	}
	require.Equal(t, 20, total)

	// Each account should be in the correct shard.
	for shardID, urls := range result {
		for _, u := range urls {
			require.Equal(t, shardID, se.RouteAccount(u))
		}
	}
}

func TestShardIsolation(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Two different accounts should potentially map to different shards.
	u1, _ := url.Parse("acc://alice.acme/tokens")
	u2, _ := url.Parse("acc://bob.acme/tokens")

	s1 := se.ShardForAccount(u1)
	s2 := se.ShardForAccount(u2)

	// Whether same or different shard, the shard reference should be stable.
	require.Same(t, s1, se.ShardForAccount(u1))
	require.Same(t, s2, se.ShardForAccount(u2))
}

func TestForEachShard(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	visited := make([]bool, 4)
	err = se.ForEachShard(func(shard *PerShardExecutor) error {
		visited[shard.ID] = true
		return nil
	})
	require.NoError(t, err)

	for i, v := range visited {
		require.True(t, v, "shard %d was not visited", i)
	}
}

func TestForEachShardError(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	expectedErr := fmt.Errorf("shard failure")
	err = se.ForEachShard(func(shard *PerShardExecutor) error {
		if shard.ID == 2 {
			return expectedErr
		}
		return nil
	})
	require.Error(t, err)
}
