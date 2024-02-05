// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Badger database driver metrics
//
// TODO Make the namespace configurable
var (
	mDbOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "badger",
		Name:      "db_open",
		Help:      "Number of open databases",
		ConstLabels: prometheus.Labels{
			"driver": "v1",
		},
	})
	mGcRun = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "badger",
		Name:      "gc_run",
		Help:      "Number of times garbage collection has run",
		ConstLabels: prometheus.Labels{
			"driver": "v1",
		},
	})
	mGcDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "badger",
		Name:      "gc_duration",
		Help:      "Garbage collection duration in seconds",
		ConstLabels: prometheus.Labels{
			"driver": "v1",
		},
	})
	mTxnOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "badger",
		Name:      "txn_open",
		Help:      "Number of open transactions",
		ConstLabels: prometheus.Labels{
			"driver": "v1",
		},
	})
	mCommitDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "badger",
		Name:      "commit_duration",
		Help:      "Commit duration in seconds",
		ConstLabels: prometheus.Labels{
			"driver": "v1",
		},
	})
)
