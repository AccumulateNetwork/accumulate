// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func testStream(t *testing.T) StreamID {
	t.Helper()
	return StreamID{
		Ledger: url.MustParse("acc://bvn-BVN0.acme/synthetic"),
		Source: url.MustParse("acc://bvn-BVN1.acme"),
	}
}

func txid(n uint64) *url.TxID {
	return url.MustParse("acc://bvn-BVN1.acme").WithTxID([32]byte{byte(n), byte(n >> 8)})
}

func TestStaging_HoldAndRead(t *testing.T) {
	s := NewStaging()
	id := testStream(t)

	_, ok := s.IDOf(id, 5)
	assert.False(t, ok, "an empty store holds nothing")
	assert.Equal(t, uint64(0), s.Highest(id))

	s.Hold(id, 5, txid(5))
	got, ok := s.IDOf(id, 5)
	require.True(t, ok)
	assert.Equal(t, txid(5).String(), got.String())
	assert.Equal(t, uint64(5), s.Highest(id))
	assert.Equal(t, 1, s.Held(id))
}

// Streams are separate even between the same pair of partitions: anchors are
// tracked by the anchor pool and synthetics by the synthetic account. If they
// shared a store, an anchor's position would gate a synthetic's.
func TestStaging_StreamsAreSeparate(t *testing.T) {
	s := NewStaging()
	synth := testStream(t)
	anchor := StreamID{Ledger: url.MustParse("acc://bvn-BVN0.acme/anchors"), Source: synth.Source}

	s.Hold(synth, 5, txid(5))
	_, ok := s.IDOf(anchor, 5)
	assert.False(t, ok, "the anchor stream holds nothing")
	assert.Equal(t, uint64(0), s.Highest(anchor))
}

// A number can be offered twice — a block discarded and re-executed, a healed
// message racing the original — and both carry the same message, because the
// number identifies it. Keeping the first means the same block always produces
// the same staging whatever order the duplicates arrived in.
func TestStaging_FirstSightingWins(t *testing.T) {
	s := NewStaging()
	id := testStream(t)

	s.Hold(id, 5, txid(5))
	s.Hold(id, 5, txid(99))

	got, ok := s.IDOf(id, 5)
	require.True(t, ok)
	assert.Equal(t, txid(5).String(), got.String())
	assert.Equal(t, 1, s.Held(id))
}

func TestStaging_ReleaseDropsDeliveredButKeepsTheWatermark(t *testing.T) {
	s := NewStaging()
	id := testStream(t)
	for _, n := range []uint64{1, 2, 3, 7} {
		s.Hold(id, n, txid(n))
	}

	s.Release(id, 3)

	for _, n := range []uint64{1, 2, 3} {
		_, ok := s.IDOf(id, n)
		assert.Falsef(t, ok, "%d was delivered", n)
	}
	_, ok := s.IDOf(id, 7)
	assert.True(t, ok, "7 was not delivered and is still held")

	assert.Equal(t, uint64(7), s.Highest(id),
		"the high-water mark does not go backwards: the stream WAS behind")
}

// Releasing a stream that was never staged, or a number nothing reached, must
// not create anything or panic.
func TestStaging_ReleaseIsSafeOnAnUnknownStream(t *testing.T) {
	s := NewStaging()
	id := testStream(t)
	s.Release(id, 100)
	assert.Equal(t, 0, s.Held(id))
	assert.Equal(t, uint64(0), s.Highest(id))
}

func TestStaging_Missing(t *testing.T) {
	id := testStream(t)

	t.Run("nothing staged is one run", func(t *testing.T) {
		s := NewStaging()
		assert.Equal(t, [][2]uint64{{11, 20}}, s.Missing(id, 10, 20, 8))
	})

	t.Run("holes between held numbers", func(t *testing.T) {
		s := NewStaging()
		// Held: 12, 14, 15, 19. Missing: 11, 13, 16-18, 20.
		for _, n := range []uint64{12, 14, 15, 19} {
			s.Hold(id, n, txid(n))
		}
		assert.Equal(t, [][2]uint64{{11, 11}, {13, 13}, {16, 18}, {20, 20}},
			s.Missing(id, 10, 20, 8))
	})

	t.Run("everything held is no runs", func(t *testing.T) {
		s := NewStaging()
		for n := uint64(11); n <= 15; n++ {
			s.Hold(id, n, txid(n))
		}
		assert.Empty(t, s.Missing(id, 10, 15, 8))
	})

	t.Run("nothing above the watermark", func(t *testing.T) {
		s := NewStaging()
		s.Hold(id, 11, txid(11))
		assert.Empty(t, s.Missing(id, 10, 10, 8), "through is not above delivered")
		assert.Empty(t, s.Missing(id, 20, 10, 8), "through is behind delivered")
	})

	t.Run("maxRuns bounds the answer", func(t *testing.T) {
		s := NewStaging()
		// Hold every even number, so every odd one is its own run.
		for n := uint64(11); n <= 40; n++ {
			if n%2 == 0 {
				s.Hold(id, n, txid(n))
			}
		}
		runs := s.Missing(id, 10, 40, 3)
		assert.Equal(t, [][2]uint64{{11, 11}, {13, 13}, {15, 15}}, runs)
		assert.Empty(t, s.Missing(id, 10, 40, 0), "asking for none returns none")
	})
}

// The executor writes staging from the block's serial phase and healing reads
// it from its own goroutine. Run under -race.
func TestStaging_ConcurrentUseIsSafe(t *testing.T) {
	s := NewStaging()
	id := testStream(t)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for n := uint64(1); n <= 200; n++ {
				s.Hold(id, n, txid(n))
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for n := uint64(1); n <= 200; n++ {
				s.IDOf(id, n)
				s.Highest(id)
				s.Missing(id, 0, 200, 4)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 200, s.Held(id))
	assert.Equal(t, uint64(200), s.Highest(id))
}
