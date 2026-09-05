// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// While execution lags consensus, user work is refused with a reason of its
// own and system traffic still passes (consensus spec, invariants 9 and 10).
func TestSubmit_RefusesUserWorkWhileExecutionLags(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)
	w.SetExecutionLagging(true)
	err := w.SubmitUser([]byte{1, 2, 3})
	require.ErrorIs(t, err, ErrExecutionLagging)
	require.NotErrorIs(t, err, ErrStoreFull, "a different reason, reported apart")
	require.NoError(t, w.Submit([]byte{4, 5, 6}), "system traffic drains the backlog and is never refused")
	w.SetExecutionLagging(false)
	require.NoError(t, w.SubmitUser([]byte{1, 2, 3}))
}
