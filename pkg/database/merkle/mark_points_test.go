// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

// The walk of a chain's records lists every mark point the chain has closed,
// including the one at Count-1 when Count is itself a mark point. That last
// state is the only record holding the entries of the set it closes, so a
// walk that leaves it out (snapshot collection) loses them.
func TestMarkPointsIncludeTheSetClosedAtTheHead(t *testing.T) {
	const markPower = 8 // 256 per mark set, as the database uses
	for _, count := range []int{255, 256, 257, 512} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			rh := new(common.RandHash)
			m := testChain(begin(), markPower, "chain")
			for i := 0; i < count; i++ {
				require.NoError(t, m.AddEntry(rh.NextList(), false))
			}

			var want []uint64
			for i := m.markFreq; i <= int64(count); i += m.markFreq {
				want = append(want, uint64(i-1))
			}

			keys, err := m.getMarkPoints()
			require.NoError(t, err)
			var got []uint64
			for _, k := range keys {
				got = append(got, k.Index)
				_, err := m.States(k.Index).Get()
				require.NoError(t, err, "mark point %d is listed but not held", k.Index)
			}
			require.Equal(t, want, got)
		})
	}
}
