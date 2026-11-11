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

// legacyHead writes the records of a chain of count entries the way the code
// before the Tail records wrote them: the head carries the hashes since the
// last mark point and, at a mark point, still the whole set that just closed
// -- that code cleared the head's hash list on the next append, not when the
// set closed. grown is a store that appended every entry itself: every mark
// point and every element. Otherwise the store is shaped as a restore of a
// snapshot taken by that code: no elements, and no mark point at Count-1 when
// Count is a mark point, because the walk that collected it did not list one.
func legacyHead(t *testing.T, m *Chain, hashes [][]byte, grown bool) {
	t.Helper()
	st := new(State)
	for _, h := range hashes {
		switch (st.Count + 1) & m.markMask {
		case 0:
			st.AddEntry(h)
			if grown || st.Count < int64(len(hashes)) {
				require.NoError(t, m.States(uint64(st.Count-1)).Put(st.Copy()))
			}
		case 1:
			st.HashList = st.HashList[:0]
			fallthrough
		default:
			st.AddEntry(h)
		}
	}
	require.NoError(t, m.Head().Put(st.Copy()))
	if grown {
		for i, h := range hashes {
			require.NoError(t, m.Element(uint64(i)).Put(h))
			require.NoError(t, m.ElementIndex(h).Put(uint64(i)))
		}
	}
}

// A chain written before the Tail records takes its next appends, at a mark
// point and around one, and ends up the chain the current code builds from the
// same entries: the same head, every entry readable, the same next mark point.
func TestALegacyHeadTakesItsNextAppends(t *testing.T) {
	const markPower = 8 // 256 per mark set, as the database uses
	for _, grown := range []bool{true, false} {
		for _, count := range []int{255, 256, 257, 512} {
			name := fmt.Sprintf("count=%d/restored", count)
			if grown {
				name = fmt.Sprintf("count=%d/grown", count)
			}
			t.Run(name, func(t *testing.T) {
				rh := new(common.RandHash)
				legacy := testChain(begin(), markPower, "legacy")
				native := testChain(begin(), markPower, "native")
				total := count + int(legacy.markFreq) + 1 // past the next mark point
				for i := 0; i < total; i++ {
					rh.NextList()
				}
				legacyHead(t, legacy, rh.List[:count], grown)
				for _, h := range rh.List[:count] {
					require.NoError(t, native.AddEntry(h, false))
				}

				for _, h := range rh.List[count:] {
					require.NoError(t, legacy.AddEntry(h, false))
					require.NoError(t, native.AddEntry(h, false))
				}

				head, err := legacy.Head().Get()
				require.NoError(t, err)
				want, err := native.Head().Get()
				require.NoError(t, err)
				require.Equal(t, want.Count, head.Count)
				require.Equal(t, want.Anchor(), head.Anchor())

				for i := 0; i < total; i++ {
					h, err := legacy.Entry(int64(i))
					require.NoError(t, err, "entry %d", i)
					require.Equal(t, rh.List[i], h, "entry %d", i)
				}
				for i := legacy.markFreq; i <= int64(total); i += legacy.markFreq {
					got, err := legacy.States(uint64(i - 1)).Get()
					require.NoError(t, err, "mark point %d", i-1)
					exp, err := native.States(uint64(i - 1)).Get()
					require.NoError(t, err, "mark point %d", i-1)
					require.Equal(t, exp.HashList, got.HashList, "mark point %d", i-1)
					require.Equal(t, exp.Anchor(), got.Anchor(), "mark point %d", i-1)
				}
			})
		}
	}
}
