// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/prometheus/client_golang/prometheus"
)

// rcmgrMetricsOnce guards metric registration: a process may construct many
// Nodes (simulators, tests), but the rcmgr collectors register with the
// default Prometheus registerer exactly once.
var rcmgrMetricsOnce sync.Once

// newResourceManager builds the host's resource manager: libp2p's scaled
// defaults, with the per-service limits libp2p itself would apply, raised
// where a validator's workload exceeds a lightweight peer's — and with usage
// exported as Prometheus metrics (libp2p_rcmgr_*), because the 20260819 soak
// proved that an invisible budget is a budget discovered only by total
// failure (#4115).
//
// The transient scope is the critical one: it holds every stream between open
// and protocol-attach, so it fills first when peers answer negotiation slowly
// (an overloaded node, or a paused container whose kernel still completes TCP
// handshakes). Its default of 256 outbound is below one validator's measured
// steady-state under healing load.
func newResourceManager() (network.ResourceManager, error) {
	scaling := rcmgr.DefaultLimits
	libp2p.SetDefaultServiceLimits(&scaling)

	limits := rcmgr.PartialLimitConfig{
		System: rcmgr.ResourceLimits{
			Streams:         16384,
			StreamsInbound:  8192,
			StreamsOutbound: 8192,
		},
		Transient: rcmgr.ResourceLimits{
			Streams:         4096,
			StreamsInbound:  2048,
			StreamsOutbound: 2048,
			Conns:           1024,
			ConnsInbound:    512,
			ConnsOutbound:   512,
			Memory:          rcmgr.LimitVal64(256 << 20),
			FD:              1024,
		},
	}.Build(scaling.AutoScale())

	var opts []rcmgr.Option
	rcmgrMetricsOnce.Do(func() {
		rcmgr.MustRegisterWith(prometheus.DefaultRegisterer)
	})
	str, err := rcmgr.NewStatsTraceReporter()
	if err != nil {
		return nil, err
	}
	opts = append(opts, rcmgr.WithTraceReporter(str))

	return rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limits), opts...)
}
