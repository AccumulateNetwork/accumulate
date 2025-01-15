// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func TestIndex(t *testing.T) {
	dir := t.TempDir()
	x, err := openIndexFileTree(&config{path: dir})
	require.NoError(t, err)

	var rh common.RandHash
	const N = 20000
	hashes := make([][32]byte, 0, N)
	for len(hashes) < N {
		// Make a new hash or pick a random old hash
		var hash [32]byte
		if len(hashes) > N/10 && rh.GetIntN(3) == 0 {
			hash = hashes[rh.GetIntN(len(hashes))]
		} else {
			hash = rh.NextA()
			hashes = append(hashes, hash)
		}

		err = x.Commit(map[[32]byte]*recordLocation{hash: {
			Block:  &blockID{},
			Offset: int64(hash[0]),
		}})
		if err != nil {
			break
		}
	}
	require.NoError(t, err)

	for _, h := range hashes {
		loc, ok := x.Get(h)
		require.Truef(t, ok, "Get %x", h)
		require.EqualValuesf(t, h[0], loc.Offset, "Check %x")
	}

	require.NoError(t, x.Close())
	x, err = openIndexFileTree(&config{path: dir})
	require.NoError(t, err)

	for _, h := range hashes {
		loc, ok := x.Get(h)
		require.True(t, ok)
		require.Truef(t, ok, "Get %x", h)
		require.EqualValuesf(t, h[0], loc.Offset, "Check %x")
	}
	require.NoError(t, x.Close())
}

func BenchmarkPut(b *testing.B) {
	N := []int{7, 8, 9, 10}
	for _, N := range N {
		N := 1 << N
		b.Run(fmt.Sprint(N), func(b *testing.B) {
			var rh common.RandHash
			for i := range b.N {
				f, err := newIndexFile(filepath.Join(b.TempDir(), fmt.Sprintf("index-%d", i)), 0)
				require.NoError(b, err)

				b.SetBytes(int64(N))
				for range N {
					_, err = f.commit([]*locationAndHash{{
						Hash:     rh.NextA(),
						Location: &recordLocation{Block: &blockID{}},
					}})
					if err != nil {
						break
					}
				}
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkGet(b *testing.B) {
	N := []int{1, 10, 100, 1000}
	for _, N := range N {
		b.Run(fmt.Sprint(N), func(b *testing.B) {
			var rh common.RandHash
			hashes := make([][32]byte, N)
			for i := range hashes {
				hashes[i] = rh.NextA()
			}

			f, err := newIndexFile(filepath.Join(b.TempDir(), "index"), 0)
			require.NoError(b, err)

			slices.SortFunc(hashes, func(a, b [32]byte) int {
				return bytes.Compare(a[:], b[:])
			})

			for _, h := range hashes {
				_, err = f.commit([]*locationAndHash{{
					Hash:     h,
					Location: &recordLocation{Block: &blockID{}},
				}})
				require.NoError(b, err)
			}

			b.ResetTimer()
			var ok bool
			for range b.N {
				h := hashes[rh.GetIntN(N)]
				_, ok = f.get(h)
				if !ok {
					break
				}
			}
			require.True(b, ok)
		})
	}
}
