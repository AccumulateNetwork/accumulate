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

// While checkpoints are refused (consensus.CheckpointAt, #4625) the two on
// disk fall behind the executor: neither matches its last block and they are
// not both above it, so nothing is resumed and the node starts at round zero
// (DIFFERENCES C29, E11), rather than from a position that would hand the
// executor groups it has produced.
func TestResumeFrom_RefusedCheckpointsResumeNothing(t *testing.T) {
	cur, prev := &Checkpoint{BlockIndex: 100}, &Checkpoint{BlockIndex: 99}
	require.Same(t, cur, ResumeFrom(100, cur, prev))
	require.Nil(t, ResumeFrom(101, cur, prev))
	require.Nil(t, ResumeFrom(500, cur, prev))
}
