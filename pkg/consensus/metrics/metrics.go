// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package metrics provides Prometheus metrics for DAG-BFT consensus.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "accumulate"
	subsystem = "dagbft"
)

// Consensus metrics
var (
	// CurrentRound is the current consensus round.
	CurrentRound = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "current_round",
		Help:      "Current consensus round",
	})

	// LastCommitRound is the last committed leader round.
	LastCommitRound = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "last_commit_round",
		Help:      "Last committed leader round",
	})

	// CertificatesCreatedTotal is the total number of certificates created.
	CertificatesCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "certificates_created_total",
		Help:      "Total number of certificates created",
	})

	// CertificatesCommittedTotal is the total number of certificates committed.
	CertificatesCommittedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "certificates_committed_total",
		Help:      "Total number of certificates committed",
	})

	// LeaderCommitsTotal is the number of leader commits.
	LeaderCommitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "leader_commits_total",
		Help:      "Total number of leader commits",
	})

	// HeadersCreatedTotal is the total number of headers created.
	HeadersCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "headers_created_total",
		Help:      "Total number of headers created",
	})

	// VotesReceivedTotal is the total number of votes received.
	VotesReceivedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "votes_received_total",
		Help:      "Total number of votes received",
	})

	// VotesSentTotal is the total number of votes sent.
	VotesSentTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "votes_sent_total",
		Help:      "Total number of votes sent",
	})
)

// Worker metrics
var (
	// WorkerBatchesCreatedTotal is the total batches created per worker.
	WorkerBatchesCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "worker_batches_created_total",
		Help:      "Total number of batches created per worker",
	}, []string{"worker_id"})

	// WorkerTransactionsProcessedTotal is the total transactions processed per worker.
	WorkerTransactionsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "worker_transactions_processed_total",
		Help:      "Total number of transactions processed per worker",
	}, []string{"worker_id"})

	// WorkerTransactionsReceivedTotal is the total transactions received per worker.
	WorkerTransactionsReceivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "worker_transactions_received_total",
		Help:      "Total number of transactions received per worker",
	}, []string{"worker_id"})

	// WorkerPendingTransactions is the current pending transaction count per worker.
	WorkerPendingTransactions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "worker_pending_transactions",
		Help:      "Current pending transaction count per worker",
	}, []string{"worker_id"})

	// WorkerStoredBatches is the current stored batch count per worker.
	WorkerStoredBatches = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "worker_stored_batches",
		Help:      "Current stored batch count per worker",
	}, []string{"worker_id"})
)

// DAG metrics
var (
	// DAGRounds is the number of rounds in the DAG.
	DAGRounds = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "dag_rounds",
		Help:      "Number of rounds in the DAG",
	})

	// DAGCertificates is the total number of certificates in the DAG.
	DAGCertificates = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "dag_certificates",
		Help:      "Total number of certificates in the DAG",
	})

	// DAGGCRoundsRemovedTotal is the total rounds removed by garbage collection.
	DAGGCRoundsRemovedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "dag_gc_rounds_removed_total",
		Help:      "Total rounds removed by garbage collection",
	})
)

// Network metrics
var (
	// GossipMessagesSentTotal is the total gossip messages sent by type.
	GossipMessagesSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "gossip_messages_sent_total",
		Help:      "Total gossip messages sent",
	}, []string{"message_type"})

	// GossipMessagesReceivedTotal is the total gossip messages received by type.
	GossipMessagesReceivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "gossip_messages_received_total",
		Help:      "Total gossip messages received",
	}, []string{"message_type"})

	// PeersConnected is the current connected peer count.
	PeersConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "peers_connected",
		Help:      "Current number of connected peers",
	})
)

// Timing metrics (histograms)
var (
	// RoundDurationSeconds is the duration of consensus rounds.
	RoundDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "round_duration_seconds",
		Help:      "Duration of consensus rounds in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
	})

	// CertificateCreationSeconds is the time to create a certificate.
	CertificateCreationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "certificate_creation_seconds",
		Help:      "Time to create a certificate in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 14), // 0.1ms to ~800ms
	})

	// BlockProductionSeconds is the time to produce a block (batch).
	BlockProductionSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "block_production_seconds",
		Help:      "Time to produce a block (batch) in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 14), // 0.1ms to ~800ms
	})

	// TransactionLatencySeconds is the end-to-end transaction latency.
	TransactionLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "transaction_latency_seconds",
		Help:      "End-to-end transaction latency in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.001, 2, 16), // 1ms to ~32s
	})
)

// CertSync metrics
var (
	// CertSyncRequestsSentTotal is the total cert sync requests sent.
	CertSyncRequestsSentTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cert_sync_requests_sent_total",
		Help:      "Total certificate sync requests sent",
	})

	// CertSyncResponsesReceivedTotal is the total cert sync responses received.
	CertSyncResponsesReceivedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cert_sync_responses_received_total",
		Help:      "Total certificate sync responses received",
	})

	// CertSyncCertificatesReceivedTotal is the total certificates received via sync.
	CertSyncCertificatesReceivedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cert_sync_certificates_received_total",
		Help:      "Total certificates received via sync",
	})
)

// Rate limiting metrics
var (
	// VotesRateLimitedTotal is the total number of votes rate limited.
	VotesRateLimitedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "votes_rate_limited_total",
		Help:      "Total number of votes rate limited",
	})

	// PeersBannedTotal is the total number of peers banned for rate limiting violations.
	PeersBannedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "peers_banned_total",
		Help:      "Total number of peers banned for rate limiting violations",
	})

	// PeersBannedCurrent is the current number of banned peers.
	PeersBannedCurrent = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "peers_banned_current",
		Help:      "Current number of banned peers",
	})
)

// Metrics provides an interface to update DAG-BFT metrics.
type Metrics struct {
	enabled bool
}

// NewMetrics creates a new Metrics instance.
// If enabled is false, all metric operations are no-ops.
func NewMetrics(enabled bool) *Metrics {
	return &Metrics{enabled: enabled}
}

// SetCurrentRound sets the current consensus round.
func (m *Metrics) SetCurrentRound(round uint64) {
	if m.enabled {
		CurrentRound.Set(float64(round))
	}
}

// SetLastCommitRound sets the last committed leader round.
func (m *Metrics) SetLastCommitRound(round uint64) {
	if m.enabled {
		LastCommitRound.Set(float64(round))
	}
}

// IncCertificatesCreated increments the certificates created counter.
func (m *Metrics) IncCertificatesCreated() {
	if m.enabled {
		CertificatesCreatedTotal.Inc()
	}
}

// IncCertificatesCommitted increments the certificates committed counter.
func (m *Metrics) IncCertificatesCommitted() {
	if m.enabled {
		CertificatesCommittedTotal.Inc()
	}
}

// IncLeaderCommits increments the leader commits counter.
func (m *Metrics) IncLeaderCommits() {
	if m.enabled {
		LeaderCommitsTotal.Inc()
	}
}

// IncHeadersCreated increments the headers created counter.
func (m *Metrics) IncHeadersCreated() {
	if m.enabled {
		HeadersCreatedTotal.Inc()
	}
}

// IncVotesReceived increments the votes received counter.
func (m *Metrics) IncVotesReceived() {
	if m.enabled {
		VotesReceivedTotal.Inc()
	}
}

// IncVotesSent increments the votes sent counter.
func (m *Metrics) IncVotesSent() {
	if m.enabled {
		VotesSentTotal.Inc()
	}
}

// IncWorkerBatchesCreated increments the worker batches created counter.
func (m *Metrics) IncWorkerBatchesCreated(workerID string) {
	if m.enabled {
		WorkerBatchesCreatedTotal.WithLabelValues(workerID).Inc()
	}
}

// IncWorkerTransactionsProcessed increments the worker transactions processed counter.
func (m *Metrics) IncWorkerTransactionsProcessed(workerID string, count uint64) {
	if m.enabled {
		WorkerTransactionsProcessedTotal.WithLabelValues(workerID).Add(float64(count))
	}
}

// IncWorkerTransactionsReceived increments the worker transactions received counter.
func (m *Metrics) IncWorkerTransactionsReceived(workerID string) {
	if m.enabled {
		WorkerTransactionsReceivedTotal.WithLabelValues(workerID).Inc()
	}
}

// SetWorkerPendingTransactions sets the worker pending transactions gauge.
func (m *Metrics) SetWorkerPendingTransactions(workerID string, count int) {
	if m.enabled {
		WorkerPendingTransactions.WithLabelValues(workerID).Set(float64(count))
	}
}

// SetWorkerStoredBatches sets the worker stored batches gauge.
func (m *Metrics) SetWorkerStoredBatches(workerID string, count int) {
	if m.enabled {
		WorkerStoredBatches.WithLabelValues(workerID).Set(float64(count))
	}
}

// SetDAGRounds sets the DAG rounds gauge.
func (m *Metrics) SetDAGRounds(rounds int) {
	if m.enabled {
		DAGRounds.Set(float64(rounds))
	}
}

// SetDAGCertificates sets the DAG certificates gauge.
func (m *Metrics) SetDAGCertificates(count int) {
	if m.enabled {
		DAGCertificates.Set(float64(count))
	}
}

// IncDAGGCRoundsRemoved increments the DAG GC rounds removed counter.
func (m *Metrics) IncDAGGCRoundsRemoved(count int) {
	if m.enabled {
		DAGGCRoundsRemovedTotal.Add(float64(count))
	}
}

// IncGossipMessagesSent increments the gossip messages sent counter.
func (m *Metrics) IncGossipMessagesSent(messageType string) {
	if m.enabled {
		GossipMessagesSentTotal.WithLabelValues(messageType).Inc()
	}
}

// IncGossipMessagesReceived increments the gossip messages received counter.
func (m *Metrics) IncGossipMessagesReceived(messageType string) {
	if m.enabled {
		GossipMessagesReceivedTotal.WithLabelValues(messageType).Inc()
	}
}

// SetPeersConnected sets the connected peers gauge.
func (m *Metrics) SetPeersConnected(count int) {
	if m.enabled {
		PeersConnected.Set(float64(count))
	}
}

// ObserveRoundDuration observes a round duration.
func (m *Metrics) ObserveRoundDuration(seconds float64) {
	if m.enabled {
		RoundDurationSeconds.Observe(seconds)
	}
}

// ObserveCertificateCreation observes certificate creation time.
func (m *Metrics) ObserveCertificateCreation(seconds float64) {
	if m.enabled {
		CertificateCreationSeconds.Observe(seconds)
	}
}

// ObserveBlockProduction observes block production time.
func (m *Metrics) ObserveBlockProduction(seconds float64) {
	if m.enabled {
		BlockProductionSeconds.Observe(seconds)
	}
}

// ObserveTransactionLatency observes transaction latency.
func (m *Metrics) ObserveTransactionLatency(seconds float64) {
	if m.enabled {
		TransactionLatencySeconds.Observe(seconds)
	}
}

// IncCertSyncRequestsSent increments the cert sync requests sent counter.
func (m *Metrics) IncCertSyncRequestsSent() {
	if m.enabled {
		CertSyncRequestsSentTotal.Inc()
	}
}

// IncCertSyncResponsesReceived increments the cert sync responses received counter.
func (m *Metrics) IncCertSyncResponsesReceived() {
	if m.enabled {
		CertSyncResponsesReceivedTotal.Inc()
	}
}

// IncCertSyncCertificatesReceived increments the cert sync certificates received counter.
func (m *Metrics) IncCertSyncCertificatesReceived(count int) {
	if m.enabled {
		CertSyncCertificatesReceivedTotal.Add(float64(count))
	}
}

// IncVotesRateLimited increments the votes rate limited counter.
func (m *Metrics) IncVotesRateLimited() {
	if m.enabled {
		VotesRateLimitedTotal.Inc()
	}
}

// IncPeersBanned increments the peers banned counter.
func (m *Metrics) IncPeersBanned() {
	if m.enabled {
		PeersBannedTotal.Inc()
	}
}

// SetPeersBannedCurrent sets the current banned peers gauge.
func (m *Metrics) SetPeersBannedCurrent(count int) {
	if m.enabled {
		PeersBannedCurrent.Set(float64(count))
	}
}
