// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
)

func BenchmarkBucketRead(b *testing.B) {
	k, err := OpenBucket(b.TempDir(), true)
	require.NoError(b, err)

	var seq lxrand.Sequence // Creates a random sequence
	for i := 0; i < b.N; i++ {
		require.NoError(b, k.Write(seq.Hash()))
	}

	b.ResetTimer()
	it := k.Iterate(4 << 10)
	var count int
	for it.Next() {
		count += len(it.Value())
	}
	require.NoError(b, it.Close())
	require.Equal(b, b.N, count)
}
