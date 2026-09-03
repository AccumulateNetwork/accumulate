// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/stretchr/testify/require"
)

// The steady-state criteria read GC cycles and GC CPU time from /metrics. The
// default Go collector exports neither; this pins that ours does.
func TestGoRuntimeMetricsExportGC(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(collectors.NewGoCollector())) // what the default registry starts with
	registerGoRuntimeMetrics(reg)

	families, err := reg.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, f := range families {
		names[f.GetName()] = true
	}
	for _, want := range []string{
		"go_gc_cycles_total_gc_cycles_total",
		"go_cpu_classes_gc_total_cpu_seconds_total",
		"go_memstats_heap_alloc_bytes", // memstats stay
	} {
		require.True(t, names[want], "missing %s", want)
	}
}
