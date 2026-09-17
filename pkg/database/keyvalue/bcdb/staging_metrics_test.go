// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// D5's instrument: the number of commits held in memory behind an open
// reader, and how old that reader is, as metrics rather than a stats file
// rewritten every 50 commits.
func TestStagingGauges(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bvnn", "data", "accumulate.db")
	db, err := Open(dir)
	require.NoError(t, err)
	defer db.Close()
	require.Equal(t, "bvnn", db.metricLabel)

	commit := func(k string) {
		b := db.Begin(nil, true)
		require.NoError(t, b.Put(record.NewKey("Account", k, "Main"), []byte(k)))
		require.NoError(t, b.Commit())
	}

	// A reader opened now pins version 0; everything committed after it
	// stays staged.
	reader := db.Begin(nil, false)
	commit("a")
	commit("b")
	require.Equal(t, 2.0, testutil.ToFloat64(stagedCommitsGauge.WithLabelValues("bvnn")),
		"two commits behind an open reader are two staged commits")
	require.Greater(t, testutil.ToFloat64(oldestViewAgeGauge.WithLabelValues("bvnn")), 0.0,
		"the open reader has an age")

	// Release the reader; the next commit drains everything.
	reader.Discard()
	commit("c")
	require.Equal(t, 0.0, testutil.ToFloat64(stagedCommitsGauge.WithLabelValues("bvnn")),
		"with no reader open, a commit reaches the store and nothing is staged")
	require.Equal(t, 0.0, testutil.ToFloat64(oldestViewAgeGauge.WithLabelValues("bvnn")))
}

func TestMetricLabelFor(t *testing.T) {
	require.Equal(t, "dnn", metricLabelFor("/root/.accumulate/bvn1-val1/dnn/data/accumulate.db"))
	require.Equal(t, "bvnn", metricLabelFor("x/bvnn/data/accumulate.db"))
	require.Equal(t, "accumulate.db", metricLabelFor("accumulate.db"))
}
