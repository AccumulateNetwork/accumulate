// Copyright 2025 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func TestLog(t *testing.T) {
	// Cases:
	//  • N = P³-1 : 3 layers, not full
	//  • N = P³   : 3 layers, full
	//  • N = P³+1 : 4 layers, not full
	const P = 2
	for _, D := range []int{-1, 0, +1} {
		N := 1<<(P*3) + D
		t.Run(fmt.Sprint(N), func(t *testing.T) {
			store := memory.New(nil).Begin(nil, true)
			x := new(Log[string])
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P

			var rand lxrand.Sequence
			var v uint64 = 1
			var values []int
			var strs []string
			for i := 0; i < N; i++ {
				require.NoError(t, x.Append(record.NewKey(v), fmt.Sprint(v)), "insert %d", i)
				values = append(values, int(v))
				strs = append(strs, fmt.Sprint(v))
				v += uint64(rand.Byte()%16 + 1)
			}

			// Verify the sequence is correct
			require.Equal(t, strs, getAll(t, x))

			// Verify everything is persisted correctly
			require.NoError(t, x.Commit())
			x = new(Log[string])
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P
			require.Equal(t, strs, getAll(t, x))

			// Verify each element can be found
			for _, v := range values {
				r := x.Find(record.NewKey(uint64(v)))
				_, u, err := r.Exact().Get()
				require.NoError(t, err)
				require.Equal(t, fmt.Sprint(v), u, "find %d", v)

				// Sanity check
				require.NoError(t, getErr(r.Before()))
				require.NoError(t, getErr(r.After()))
			}

			// A target before the first has Before, but no Exact or After
			r := x.Find(record.NewKey(uint64(0)))
			require.ErrorIs(t, getErr(r.Before()), errors.NotFound)
			require.ErrorIs(t, getErr(r.Exact()), errors.NotFound)
			require.NoError(t, getErr(r.After()))

			// A target after the last has After, but no Exact or Before
			r = x.Find(record.NewKey(v))
			require.NoError(t, getErr(r.Before()))
			require.ErrorIs(t, getErr(r.Exact()), errors.NotFound)
			require.ErrorIs(t, getErr(r.After()), errors.NotFound)

			// A target between two other values has Before and After, but no Exact
			for i, after := range values[1:] {
				before := values[i]
				if after == before+1 {
					continue
				}

				r := x.Find(record.NewKey(uint64((before + after) / 2)))
				require.ErrorIs(t, getErr(r.Exact()), errors.NotFound)

				_, v, err := r.Before().Get()
				require.NoError(t, err)
				require.Equal(t, fmt.Sprint(before), v)

				_, v, err = r.After().Get()
				require.NoError(t, err)
				require.Equal(t, fmt.Sprint(after), v)
			}
		})
	}
}

func BenchmarkLog_Append(b *testing.B) {
	for _, P := range []int{8, 10, 12, 14, 16} {
		b.Run(fmt.Sprint(1<<P), func(b *testing.B) {
			store := memory.New(nil).Begin(nil, true)
			x := new(Log[uint64])
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

func BenchmarkLog_Find(b *testing.B) {
	const N = 20
	for _, P := range []int{8, 10, 12, 14, 16} {
		b.Run(fmt.Sprint(1<<P), func(b *testing.B) {
			store := memory.New(nil).Begin(nil, true)
			x := new(Log[string])
			x.store = keyvalue.RecordStore{Store: store}
			x.blockSize = 1 << P

			var err error
			for i := 0; i < 1<<N; i++ {
				v := fmt.Sprint(i)
				err = x.Append(record.NewKey(uint64(i)), v)
			}
			require.NoError(b, err)

			b.ResetTimer()

			v := uint64(1<<N - 1)
			var r Query[string]
			for i := 0; i < b.N; i++ {
				r = x.Find(record.NewKey(v))
			}
			require.NoError(b, getErr(r.Exact()))
		})
	}
}
func getAll[V any](t testing.TB, x *Log[V]) []V {
	var all []V
	x.All(func(r QueryResult[V]) bool {
		_, v, err := r.Get()
		require.NoError(t, err)
		all = append(all, v)
		return true
	})
	return all
}

func getErr[V any](q QueryResult[V]) error {
	_, _, err := q.Get()
	return err
}
