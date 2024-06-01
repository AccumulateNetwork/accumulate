// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TODO Make the namespace configurable
//
// getClient latency, wanted, and scanned should probably be summaries or
// histograms, but I don't know how to use those
var (
	mDispatchPanics = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "panics",
		Help:      "The number of times dispatch has panicked",
	})

	mDispatchCalls = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "calls",
		Help:      "The number of dispatch calls",
	})

	mDispatchEnvelopes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "envelopes",
		Help:      "The number of envelopes dispatched",
	})

	mDispatchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "errors",
		Help:      "The number of dispatch errors",
	})

	mGetClientsLatency = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "getClient_latency",
		Help:      "The latency of the peer scan",
	})

	mGetClientsScans = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "getClient_scans",
		Help:      "The number of getClient scans",
	})

	mGetClientsWanted = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "getClient_wanted",
		Help:      "The number of wanted clients",
	})

	mGetClientsPeers = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "getClient_peers",
		Help:      "The number of peers that were checked",
	})

	mGetClientsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "tendermintDispatch",
		Name:      "getClient_failed",
		Help:      "The number of peers that failed",
	})
)
