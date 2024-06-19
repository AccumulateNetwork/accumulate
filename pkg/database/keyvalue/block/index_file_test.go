// Copyright 2024 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func TestInsert(t *testing.T) {
	f, err := newIndexFile(filepath.Join(t.TempDir(), "index"))
	require.NoError(t, err)

	var rh common.RandHash
	const N = 2000
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

		err = f.Insert((*record.KeyHash)(&hash), &recordLocation{})
		if err != nil {
			break
		}
	}
	require.NoError(t, err)

	// const X = 64
	// count := f.count.Load()
	// end := count * indexFileEntrySize / X
	// x := (*[X]byte)(unsafe.Pointer(unsafe.SliceData(f.file.data)))
	// y := unsafe.Slice(x, len(f.file.data)/X)[:end]
	// _ = y

	for _, h := range hashes {
		_, err = f.FindBinary(record.KeyFromHash(h))
		require.NoError(t, err)
	}
}

func BenchmarkAppend(b *testing.B) {
	N := []int{7, 8, 9, 10, 11}
	for _, N := range N {
		N := 1 << N
		b.Run(fmt.Sprint(N), func(b *testing.B) {
			var rh common.RandHash
			for i := range b.N {
				f, err := newIndexFile(filepath.Join(b.TempDir(), fmt.Sprintf("index-%d", i)))
				require.NoError(b, err)

				b.SetBytes(int64(N))
				for range N {
					err = f.Append((*record.KeyHash)(rh.Next()), &recordLocation{})
					if err != nil {
						break
					}
				}
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkInsert(b *testing.B) {
	N := []int{7, 8, 9, 10, 11}
	for _, N := range N {
		N := 1 << N
		b.Run(fmt.Sprint(N), func(b *testing.B) {
			var rh common.RandHash
			for i := range b.N {
				f, err := newIndexFile(filepath.Join(b.TempDir(), fmt.Sprintf("index-%d", i)))
				require.NoError(b, err)

				b.SetBytes(int64(N))
				for range N {
					err = f.Insert((*record.KeyHash)(rh.Next()), &recordLocation{})
					if err != nil {
						break
					}
				}
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkLinear(b *testing.B) {
	N := []int{1, 10, 100, 1000, 10000}
	for _, N := range N {
		b.Run(fmt.Sprint(N), func(b *testing.B) {
			var rh common.RandHash
			hashes := make([][32]byte, N)
			for i := range hashes {
				hashes[i] = rh.NextA()
			}

			f, err := newIndexFile(filepath.Join(b.TempDir(), "index"))
			require.NoError(b, err)

			for i := range hashes {
				err = f.Append((*record.KeyHash)(&hashes[i]), &recordLocation{})
				require.NoError(b, err)
			}

			b.ResetTimer()
			for range b.N {
				h := hashes[rh.GetIntN(N)]
				_, err = f.FindLinear(record.KeyFromHash(h))
				if err != nil {
					break
				}
			}
			require.NoError(b, err)
		})
	}
}

func BenchmarkBinary(b *testing.B) {
	N := []int{1, 10, 100, 1000, 10000}
	for _, N := range N {
		b.Run(fmt.Sprint(N), func(b *testing.B) {
			var rh common.RandHash
			hashes := make([][32]byte, N)
			for i := range hashes {
				hashes[i] = rh.NextA()
			}

			f, err := newIndexFile(filepath.Join(b.TempDir(), "index"))
			require.NoError(b, err)

			slices.SortFunc(hashes, func(a, b [32]byte) int {
				return bytes.Compare(a[:], b[:])
			})

			for i := range hashes {
				err = f.Append((*record.KeyHash)(&hashes[i]), &recordLocation{})
				require.NoError(b, err)
			}

			b.ResetTimer()
			for range b.N {
				h := hashes[rh.GetIntN(N)]
				_, err = f.FindBinary(record.KeyFromHash(h))
				if err != nil {
					break
				}
			}
			require.NoError(b, err)
		})
	}
}
