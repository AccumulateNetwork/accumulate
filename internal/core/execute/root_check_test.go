// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Lag one: a node signs unless the newest root it checked did not match.
// Nothing checked signs, or a partition whose validators are all booting
// could never anchor; an older report says nothing about the node now.
func TestRootCheckSignsUnlessTheNewestCheckMismatched(t *testing.T) {
	var none *RootCheck
	require.True(t, none.MaySign(), "a node with no check wired signs")
	none.Mismatched(3) // and a report to nothing is not a panic

	r := new(RootCheck)
	require.True(t, r.MaySign(), "nothing checked yet")

	r.Matched(5)
	require.True(t, r.MaySign())

	r.Mismatched(7)
	require.False(t, r.MaySign())
	block, matched, checked := r.Last()
	require.Equal(t, uint64(7), block)
	require.False(t, matched)
	require.True(t, checked)

	r.Matched(6)
	require.False(t, r.MaySign(), "a match older than the mismatch does not reopen the gate")

	r.Matched(7)
	require.True(t, r.MaySign(), "the same block checked again after a repair")

	r.Mismatched(9)
	r.Matched(10)
	require.True(t, r.MaySign())
}
