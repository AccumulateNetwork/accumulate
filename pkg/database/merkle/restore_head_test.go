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

// A node pulling state takes a chain's head from a peer and must be able to
// append to it — it executes the next block from the chains it pulled
// (executor.md, "Sync", step 3). RestoreHead takes the head and the open mark
// set together, and the restored chain then takes the same entry the source
// takes and lands on the same anchor.
func TestRestoreHeadIsAppendable(t *testing.T) {
	// 3: the whole set is open. 255: the next append closes the mark set,
	// which is assembled from every chunk of it. 260: past a mark point.
	for _, height := range []int{0, 3, 255, 256, 260} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			var rh common.RandHash
			src := testChain(begin(), 8, "src") // 256 hashes per mark set
			for i := 0; i < height; i++ {
				require.NoError(t, src.AddEntry(rh.NextList(), false))
			}
			head, err := src.Head().Get()
			require.NoError(t, err)
			open, err := src.OpenSet(head)
			require.NoError(t, err)

			dst := testChain(begin(), 8, "dst")
			require.NoError(t, dst.RestoreHead(head.Copy(), open))

			got, err := dst.Head().Get()
			require.NoError(t, err)
			require.Equal(t, head.Count, got.Count)
			require.Equal(t, head.Anchor(), got.Anchor())

			next := rh.NextList()
			require.NoError(t, src.AddEntry(next, false))
			require.NoError(t, dst.AddEntry(next, false), "a restored chain could not be appended to")

			sh, err := src.Head().Get()
			require.NoError(t, err)
			dh, err := dst.Head().Get()
			require.NoError(t, err)
			require.Equal(t, sh.Count, dh.Count)
			require.Equal(t, sh.Anchor(), dh.Anchor(), "the same append landed on a different anchor")
		})
	}
}

// The open set a peer serves is checked against the head it serves with it.
func TestRestoreHeadRefusesAnOpenSetThatIsNotTheHead(t *testing.T) {
	var rh common.RandHash
	src := testChain(begin(), 8, "src")
	for i := 0; i < 5; i++ {
		require.NoError(t, src.AddEntry(rh.NextList(), false))
	}
	head, err := src.Head().Get()
	require.NoError(t, err)
	open, err := src.OpenSet(head)
	require.NoError(t, err)

	// One entry short.
	dst := testChain(begin(), 8, "dst")
	require.Error(t, dst.RestoreHead(head.Copy(), open[:4]))

	// The right number of entries, one of them somebody else's.
	bent := make([][]byte, len(open))
	copy(bent, open)
	bent[2] = rh.NextList()
	require.Error(t, dst.RestoreHead(head.Copy(), bent))
}
