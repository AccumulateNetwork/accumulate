// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every other test in this package runs on one goroutine, so none of them can
// find a race. This one can.
//
// positionOf memoises into b.positions, which means it WRITES shared block
// state on a cache miss. Staging is serial today, so nothing is wrong right
// now — but "correct because nothing calls it concurrently yet" is a property
// of the callers, not of the code, and #4169 step 9 routes components to
// shards. A shard that needs a stream's position would corrupt the map with
// no symptom until a block hash diverged.
//
// Run this under -race.
func TestStreamPosition_ConcurrentReadsAreSafe(t *testing.T) {
	b, s := positionBlock(t, 5, 6, 7)

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	nexts := make([]uint64, goroutines)

	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // maximise overlap on the cache miss
			p, err := b.positionOf(s)
			errs[i] = err
			if err == nil {
				nexts[i] = p.next()
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
		require.Equalf(t, uint64(6), nexts[i],
			"goroutine %d saw a different position — concurrent callers must agree", i)
	}
}
