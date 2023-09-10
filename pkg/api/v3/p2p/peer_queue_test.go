// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPeerQueue(t *testing.T) {
	q := new(peerQueue)
	p1 := newPeer(t, 1)
	p2 := newPeer(t, 2)
	p3 := newPeer(t, 3)

	// Test while empty
	_, ok := q.Next()
	require.False(t, ok)
	require.Equal(t, 0, q.Len())
	require.Empty(t, q.All())

	// Add a peer
	q.Add(p1)
	id, ok := q.Next()
	require.True(t, ok)
	require.Equal(t, p1, id)
	require.Equal(t, 1, q.Len())
	require.Len(t, q.All(), 1)

	// Remove it and verify
	q.Remove(p1)
	_, ok = q.Next()
	require.False(t, ok)
	require.Equal(t, 0, q.Len())
	require.Empty(t, q.All())

	// Add two and verify
	q.Add(p1, p2)
	id, ok = q.Next()
	require.True(t, ok)
	require.Equal(t, p1, id)
	id, ok = q.Next()
	require.True(t, ok)
	require.Equal(t, p2, id)
	require.Equal(t, 2, q.Len())
	require.Len(t, q.All(), 2)

	// Add p3, remove p2, and verify
	q.Add(p3)
	q.Remove(p2)
	id, ok = q.Next()
	require.True(t, ok)
	require.Equal(t, p1, id)
	id, ok = q.Next()
	require.True(t, ok)
	require.Equal(t, p3, id)
	require.Equal(t, 2, q.Len())
	require.Len(t, q.All(), 2)
}

func BenchmarkPeerQueue(b *testing.B) {
	q := new(peerQueue)
	for i := 0; i < 100; i++ {
		p := newPeer(b, i)
		q.Add(p)
	}

	wg := new(sync.WaitGroup)
	wg.Add(b.N)

	b.ResetTimer()

	// Launch N calls to Next and wait for them all to complete
	for i := 0; i < b.N; i++ {
		go func() { defer wg.Done(); q.Next() }()
	}
	wg.Wait()
}
