// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func TestChainIndex(t *testing.T) {
	// Cases:
	//  • N = P³-1 : 3 layers, not full
	//  • N = P³   : 3 layers, full
	//  • N = P³+1 : 4 layers, not full
	const P = 2
	for _, D := range []int{-1, 0, +1} {
		N := 1<<(P*3) + D
		t.Run(fmt.Sprint(N), func(t *testing.T) {
			store := memory.New(nil).Begin(nil, true)
			x := new(ChainIndex)
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P

			var rand lxrand.Sequence
			var v uint64 = 1
			var values []int
			for i := 0; i < N; i++ {
				require.NoError(t, x.Append(record.NewKey(v), v), "insert %d", i)
				values = append(values, int(v))
				v += uint64(rand.Byte()%16 + 1)
			}

			// Verify the sequence is correct
			require.Equal(t, values, x.all(t, x.getHead()))

			// Verify everything is persisted correctly
			require.NoError(t, x.Commit())
			x = new(ChainIndex)
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P
			require.Equal(t, values, x.all(t, x.getHead()))

			// Verify each element can be found
			for _, v := range values {
				r := x.Find(record.NewKey(uint64(v)))
				u, err := r.Exact().Index()
				require.NoError(t, err)
				require.Equal(t, v, int(u), "find %d", v)

				// Sanity check
				require.NoError(t, r.Before().Err())
				require.NoError(t, r.After().Err())
			}

			// A target before the first has Before, but no Exact or After
			r := x.Find(record.NewKey(uint64(0)))
			require.ErrorIs(t, r.Before().Err(), errors.NotFound)
			require.ErrorIs(t, r.Exact().Err(), errors.NotFound)
			require.NoError(t, r.After().Err())

			// A target after the last has After, but no Exact or Before
			r = x.Find(record.NewKey(v))
			require.NoError(t, r.Before().Err())
			require.ErrorIs(t, r.Exact().Err(), errors.NotFound)
			require.ErrorIs(t, r.After().Err(), errors.NotFound)

			// A target between two other values has Before and After, but no Exact
			for i, after := range values[1:] {
				before := values[i]
				if after == before+1 {
					continue
				}

				r := x.Find(record.NewKey(uint64((before + after) / 2)))
				require.ErrorIs(t, r.Exact().Err(), errors.NotFound)

				v, err := r.Before().Index()
				require.NoError(t, err)
				require.Equal(t, before, int(v))

				v, err = r.After().Index()
				require.NoError(t, err)
				require.Equal(t, after, int(v))
			}
		})
	}
}

func BenchmarkChainIndex_Insert(b *testing.B) {
	for _, P := range []int{8, 10, 12, 14, 16} {
		b.Run(fmt.Sprint(1<<P), func(b *testing.B) {
			store := memory.New(nil).Begin(nil, true)
			x := new(ChainIndex)
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P

			var err error
			for i := 0; i < b.N; i++ {
				v := uint64(i)
				err = x.Append(record.NewKey(v), v)
			}
			require.NoError(b, err)
		})
	}
}

func BenchmarkChainIndex_Find(b *testing.B) {
	const N = 20
	for _, P := range []int{8, 10, 12, 14, 16} {
		b.Run(fmt.Sprint(1<<P), func(b *testing.B) {
			store := memory.New(nil).Begin(nil, true)
			x := new(ChainIndex)
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P

			var err error
			for i := 0; i < 1<<N; i++ {
				v := uint64(i)
				err = x.Append(record.NewKey(v), v)
			}
			require.NoError(b, err)

			b.ResetTimer()

			v := uint64(1<<N - 1)
			var r ChainSearchResult
			for i := 0; i < b.N; i++ {
				r = x.Find(record.NewKey(v))
			}
			require.NoError(b, r.Exact().Err())
		})
	}
}

func (x *ChainIndex) all(t testing.TB, record values.Value[*chainIndexBlock]) []int {
	b, err := record.Get()
	require.NoError(t, err)

	var all []int
	if b.Level == 0 {
		for _, e := range b.Entries {
			all = append(all, int(e.Index))
		}
	} else {
		for _, e := range b.Entries {
			all = append(all, x.all(t, x.getBlock(b.Level-1, e.Index))...)
		}
	}
	return all
}
