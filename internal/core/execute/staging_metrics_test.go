// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// What a stream holds is a gauge, in entries and bytes, updated when a block
// commits; a backlog of more than several blocks' worth is reported once and
// its clearing once (#4233).
func TestStaging_HeldGauge(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	const n = heldAlarmEntries + 1
	for i := uint64(1); i <= n; i++ {
		tx.Hold(testStream, i, held(i))
	}
	tx.Commit()

	key := testStream.key()
	st := s.streams[key]
	require.Equal(t, n, st.held)
	require.Greater(t, st.bytes, 0, "held bytes are measured")
	require.True(t, st.alarmed, "more than the line is reported")
	ledger, source, _ := strings.Cut(key, "|")
	require.Equal(t, float64(n), testutil.ToFloat64(mStagingHeld.WithLabelValues(ledger, source)))
	require.Equal(t, float64(st.bytes), testutil.ToFloat64(mStagingHeldBytes.WithLabelValues(ledger, source)))

	// Draining a quarter leaves the backlog reported; draining it clears it
	tx = s.Begin()
	tx.Release(testStream, n/4)
	tx.Commit()
	require.Equal(t, n-n/4, st.held)
	require.True(t, st.alarmed, "a stream hovering at the line is not reported again and again")

	tx = s.Begin()
	tx.Release(testStream, n)
	tx.Commit()
	require.Zero(t, st.held)
	require.Zero(t, st.bytes)
	require.False(t, st.alarmed, "cleared once it drains")
	require.Zero(t, testutil.ToFloat64(mStagingHeld.WithLabelValues(ledger, source)))
	require.Zero(t, testutil.ToFloat64(mStagingHeldBytes.WithLabelValues(ledger, source)))
}
