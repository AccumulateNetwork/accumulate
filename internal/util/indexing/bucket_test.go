// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"bytes"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
)

func TestBucket(t *testing.T) {
	dir := t.TempDir()
	k, err := OpenBucket(dir, 0, true)
	require.NoError(t, err)

	const N = 10000
	var seq lxrand.Sequence // Creates a random sequence
	var values [][32]byte
	for i := 0; i < N; i++ {
		v := seq.Hash()
		values = append(values, v)
		require.NoError(t, k.Write(v, nil))
	}
	sort.Slice(values, func(i, j int) bool {
		return bytes.Compare(values[i][:], values[j][:]) < 0
	})
	require.NoError(t, k.Close())

	k, err = OpenBucket(dir, 0, false)
	require.NoError(t, err)
	defer k.Close()
	var all [][32]byte
	for i := 0; i < BucketCount; i++ {
		e, err := k.Read(byte(i))
		require.NoError(t, err)
		sort.Slice(e, func(i, j int) bool {
			return bytes.Compare(e[i].Hash[:], e[j].Hash[:]) < 0
		})
		for _, e := range e {
			all = append(all, e.Hash)
		}
	}
	require.Equal(t, values, all)
}

func TestBucketWithValues(t *testing.T) {
	k, err := OpenBucket(t.TempDir(), 10, true)
	require.NoError(t, err)
	defer k.Close()

	const N = 10000
	var seq lxrand.Sequence // Creates a random sequence
	var values []Entry
	for i := 0; i < N; i++ {
		e := Entry{seq.Hash(), seq.Slice(10)}
		values = append(values, e)
		require.NoError(t, k.Write(e.Hash, e.Value))
	}
	sort.Slice(values, func(i, j int) bool {
		return bytes.Compare(values[i].Hash[:], values[j].Hash[:]) < 0
	})

	var all []Entry
	for i := 0; i < BucketCount; i++ {
		e, err := k.Read(byte(i))
		require.NoError(t, err)
		sort.Slice(e, func(i, j int) bool {
			return bytes.Compare(e[i].Hash[:], e[j].Hash[:]) < 0
		})
		all = append(all, e...)
	}
	require.Equal(t, values, all)
}
