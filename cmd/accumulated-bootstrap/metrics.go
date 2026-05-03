// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Connection metrics
	peersConnected = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "bootstrap",
		Name:      "peers_connected",
		Help:      "Number of currently connected peers",
	}, []string{"direction"})

	peersTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "bootstrap",
		Name:      "peers_total",
		Help:      "Total number of peers seen since startup",
	})

	connectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "bootstrap",
		Name:      "connections_total",
		Help:      "Total number of connection events",
	}, []string{"direction", "status"})

	// DHT metrics
	dhtRoutingTableSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "bootstrap",
		Name:      "dht_routing_table_size",
		Help:      "Number of peers in the DHT routing table",
	})

	// Partition metrics
	partitionPeers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "bootstrap",
		Name:      "partition_peers",
		Help:      "Number of known peers per partition",
	}, []string{"partition"})

	// Discovery metrics
	discoveryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "bootstrap",
		Name:      "discovery_duration_seconds",
		Help:      "Time spent on peer discovery operations",
		Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
	}, []string{"operation"})

	discoveryPeersFound = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "bootstrap",
		Name:      "discovery_peers_found_total",
		Help:      "Total number of peers found during discovery",
	}, []string{"partition"})

	// Request metrics
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "bootstrap",
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests",
	}, []string{"endpoint", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "bootstrap",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds",
		Buckets:   prometheus.DefBuckets,
	}, []string{"endpoint"})

	// Uptime metric
	startTime = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "bootstrap",
		Name:      "start_time_seconds",
		Help:      "Unix timestamp of when the bootstrap server started",
	})
)

// MetricsCollector collects and updates metrics for the bootstrap server
type MetricsCollector struct {
	host       host.Host
	mu         sync.RWMutex
	seenPeers  map[peer.ID]struct{}
	partitions map[string]map[peer.ID]struct{}
	stopCh     chan struct{}
	updateTick *time.Ticker
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(h host.Host) *MetricsCollector {
	mc := &MetricsCollector{
		host:       h,
		seenPeers:  make(map[peer.ID]struct{}),
		partitions: make(map[string]map[peer.ID]struct{}),
		stopCh:     make(chan struct{}),
	}

	// Set start time
	startTime.SetToCurrentTime()

	// Subscribe to network events
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF:    mc.onConnected,
		DisconnectedF: mc.onDisconnected,
	})

	// Start periodic metrics update
	mc.updateTick = time.NewTicker(30 * time.Second)
	go mc.periodicUpdate()

	return mc
}

// Stop stops the metrics collector
func (mc *MetricsCollector) Stop() {
	close(mc.stopCh)
	if mc.updateTick != nil {
		mc.updateTick.Stop()
	}
}

// onConnected handles new connection events
func (mc *MetricsCollector) onConnected(n network.Network, conn network.Conn) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	peerID := conn.RemotePeer()
	direction := "inbound"
	if conn.Stat().Direction == network.DirOutbound {
		direction = "outbound"
	}

	// Track new peers
	if _, seen := mc.seenPeers[peerID]; !seen {
		mc.seenPeers[peerID] = struct{}{}
		peersTotal.Inc()
	}

	connectionsTotal.WithLabelValues(direction, "connected").Inc()
	mc.updateConnectionMetrics()
}

// onDisconnected handles disconnection events
func (mc *MetricsCollector) onDisconnected(n network.Network, conn network.Conn) {
	direction := "inbound"
	if conn.Stat().Direction == network.DirOutbound {
		direction = "outbound"
	}

	connectionsTotal.WithLabelValues(direction, "disconnected").Inc()
	mc.updateConnectionMetrics()
}

// updateConnectionMetrics updates the current connection gauges
func (mc *MetricsCollector) updateConnectionMetrics() {
	conns := mc.host.Network().Conns()
	inbound := 0
	outbound := 0
	for _, conn := range conns {
		if conn.Stat().Direction == network.DirInbound {
			inbound++
		} else {
			outbound++
		}
	}
	peersConnected.WithLabelValues("inbound").Set(float64(inbound))
	peersConnected.WithLabelValues("outbound").Set(float64(outbound))
}

// periodicUpdate updates metrics periodically
func (mc *MetricsCollector) periodicUpdate() {
	for {
		select {
		case <-mc.stopCh:
			return
		case <-mc.updateTick.C:
			mc.updateConnectionMetrics()
			mc.updateDHTMetrics()
			mc.updatePartitionMetrics()
		}
	}
}

// updateDHTMetrics updates DHT-related metrics
func (mc *MetricsCollector) updateDHTMetrics() {
	peers := mc.host.Network().Peers()
	dhtRoutingTableSize.Set(float64(len(peers)))
}

// updatePartitionMetrics updates partition peer counts
func (mc *MetricsCollector) updatePartitionMetrics() {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	for partition, peers := range mc.partitions {
		partitionPeers.WithLabelValues(partition).Set(float64(len(peers)))
	}
}

// RecordDiscovery records a discovery operation
func (mc *MetricsCollector) RecordDiscovery(operation string, duration time.Duration, peersFound int, partition string) {
	discoveryDuration.WithLabelValues(operation).Observe(duration.Seconds())
	if peersFound > 0 {
		discoveryPeersFound.WithLabelValues(partition).Add(float64(peersFound))
	}
}

// RecordHTTPRequest records an HTTP request
func (mc *MetricsCollector) RecordHTTPRequest(endpoint string, status int, duration time.Duration) {
	statusLabel := "success"
	if status >= 400 {
		statusLabel = "error"
	}
	httpRequestsTotal.WithLabelValues(endpoint, statusLabel).Inc()
	httpRequestDuration.WithLabelValues(endpoint).Observe(duration.Seconds())
}

// SetPartitionPeers updates the known peers for a partition
func (mc *MetricsCollector) SetPartitionPeers(partition string, peers []peer.ID) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	peerSet := make(map[peer.ID]struct{}, len(peers))
	for _, p := range peers {
		peerSet[p] = struct{}{}
	}
	mc.partitions[partition] = peerSet
	partitionPeers.WithLabelValues(partition).Set(float64(len(peers)))
}

// GetStats returns current metrics as a structured map
func (mc *MetricsCollector) GetStats() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	conns := mc.host.Network().Conns()
	inbound := 0
	outbound := 0
	for _, conn := range conns {
		if conn.Stat().Direction == network.DirInbound {
			inbound++
		} else {
			outbound++
		}
	}

	partitionStats := make(map[string]int)
	for partition, peers := range mc.partitions {
		partitionStats[partition] = len(peers)
	}

	return map[string]interface{}{
		"peers": map[string]interface{}{
			"connected_inbound":  inbound,
			"connected_outbound": outbound,
			"connected_total":    len(conns),
			"unique_seen":        len(mc.seenPeers),
		},
		"dht": map[string]interface{}{
			"routing_table_size": len(mc.host.Network().Peers()),
		},
		"partitions": partitionStats,
	}
}
