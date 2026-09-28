// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResumeFrom(t *testing.T) {
	cp := func(block uint64) *Checkpoint { return &Checkpoint{BlockIndex: block} }
	cases := []struct {
		name      string
		last      uint64
		cur, prev *Checkpoint
		want      uint64 // 0 = none
	}{
		{"executor at cur", 12, cp(12), cp(11), 12},
		{"crash before cur's block", 11, cp(12), cp(11), 11},
		// The Docker BVN1 of #4448: executor at 12214, checkpoints for 12217
		// and 12216; blocks 12215-12216 carried nothing.
		{"last blocks empty", 12214, cp(12217), cp(12216), 12216},
		{"checkpoints behind the executor", 20, cp(12), cp(11), 0},
		{"no prev", 5, cp(12), nil, 0},
		{"prev not the block before cur", 5, cp(12), cp(9), 0},
		{"none", 5, nil, nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResumeFrom(c.last, c.cur, c.prev)
			if c.want == 0 {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, c.want, got.BlockIndex)
		})
	}
}
