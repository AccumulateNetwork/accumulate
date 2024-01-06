// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func b2i(b []byte) int64 {
	i := int64(b[0])<<24 + int64(b[1])<<16 + int64(b[2])<<8 + int64(b[3])
	return i
}

func i2b(i int64) []byte {
	b := [32]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
	return b[:]
}

func TestConversions(t *testing.T) {
	for i := int64(0); i < 10000; i++ {
		if i != b2i(i2b(i)) {
			t.Fatalf("failed %d", i)
		}

	}
}

func TestMerkleManager_GetRange(t *testing.T) {
	for i := int64(1); i < 6; i++ {
		for NumTests := int64(50); NumTests < 64; NumTests++ {

			var rh common.RandHash
			store := begin()
			mm := testChain(store, i, "try")
			for i := int64(0); i < NumTests; i++ {
				require.NoError(t, mm.AddEntry(rh.NextList(), false))
			}
			for begin := int64(-1); begin < NumTests+1; begin++ {
				for end := begin - 1; end < NumTests+2; end++ {

					hashes, err := mm.Entries(begin, end)

					if begin < 0 || begin > end || begin >= NumTests {
						require.Errorf(t, err, "should not allow range [%d,%d] (power %d)", begin, end, i)
					} else {
						require.NoErrorf(t, err, "should have a range for [%d,%d] (power %d)", begin, end, i)
						e := end
						if e > NumTests {
							e = NumTests
						}
						require.Truef(t, len(hashes) == int(e-begin),
							"returned the wrong length for [%d,%d] %d (power %d)", begin, end, len(hashes), i)
						for k, h := range rh.List[begin:e] {
							require.Truef(t, bytes.Equal(hashes[k], h),
								"[%d,%d]returned wrong values (power %d)", begin, end, i)
						}
					}
				}
			}
		}
	}
}
