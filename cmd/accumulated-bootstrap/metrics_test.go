// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// Metric labels are bounded whatever peers announce or callers request: the
// partition label is the partition's role, and the HTTP endpoint label is the
// route (#4246). A label that repeated the announced name or the request
// path minted a series per value for the life of the process.
func TestMetricLabelsAreBounded(t *testing.T) {
	mc := &MetricsCollector{
		seenPeers:  map[peer.ID]struct{}{},
		partitions: map[string]map[peer.ID]struct{}{},
	}

	// A hundred announced partition names, one peer each
	for i := 0; i < 100; i++ {
		mc.SetPartitionPeers(fmt.Sprintf("announced-%d", i), []peer.ID{peer.ID(fmt.Sprintf("p%d", i))})
		mc.RecordDiscovery("probe", time.Millisecond, 1, fmt.Sprintf("announced-%d", i))
	}
	mc.SetPartitionPeers("Directory", []peer.ID{"d1", "d2"})
	mc.SetPartitionPeers(PartitionDN, []peer.ID{"d3"})
	mc.SetPartitionPeers(PartitionUnknown, []peer.ID{"u1"})

	require.LessOrEqual(t, testutil.CollectAndCount(partitionPeers), 3, "partition_peers has one series per role")
	require.Equal(t, float64(3), testutil.ToFloat64(partitionPeers.WithLabelValues(roleDirectory)))
	require.Equal(t, float64(100), testutil.ToFloat64(partitionPeers.WithLabelValues(roleBVN)))
	require.Equal(t, float64(1), testutil.ToFloat64(partitionPeers.WithLabelValues(roleUnknown)))
	require.LessOrEqual(t, testutil.CollectAndCount(discoveryPeersFound), 4, "discovery_peers_found_total: roles plus \"all\"")

	// A hundred requested partition paths
	for i := 0; i < 100; i++ {
		mc.RecordHTTPRequest("/peers/{partition}", http.StatusOK, time.Millisecond)
	}
	require.LessOrEqual(t, testutil.CollectAndCount(httpRequestsTotal), 2, "http_requests_total: one route, success and error")
}

func TestPartitionRole(t *testing.T) {
	require.Equal(t, roleDirectory, partitionRole("Directory"))
	require.Equal(t, roleDirectory, partitionRole("dn"))
	require.Equal(t, roleBVN, partitionRole("Apollo"))
	require.Equal(t, roleBVN, partitionRole(PartitionCyclops))
	require.Equal(t, roleUnknown, partitionRole(""))
	require.Equal(t, roleUnknown, partitionRole(PartitionUnknown))
}
