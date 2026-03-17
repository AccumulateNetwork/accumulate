// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_Enabled(t *testing.T) {
	m := NewMetrics(true)

	// Test round metrics
	m.SetCurrentRound(10)
	if got := testutil.ToFloat64(CurrentRound); got != 10 {
		t.Errorf("CurrentRound = %v, want 10", got)
	}

	m.SetLastCommitRound(8)
	if got := testutil.ToFloat64(LastCommitRound); got != 8 {
		t.Errorf("LastCommitRound = %v, want 8", got)
	}

	// Test counter increments
	m.IncCertificatesCreated()
	if got := testutil.ToFloat64(CertificatesCreatedTotal); got < 1 {
		t.Errorf("CertificatesCreatedTotal = %v, want >= 1", got)
	}

	m.IncCertificatesCommitted()
	if got := testutil.ToFloat64(CertificatesCommittedTotal); got < 1 {
		t.Errorf("CertificatesCommittedTotal = %v, want >= 1", got)
	}

	m.IncLeaderCommits()
	if got := testutil.ToFloat64(LeaderCommitsTotal); got < 1 {
		t.Errorf("LeaderCommitsTotal = %v, want >= 1", got)
	}

	// Test worker metrics
	m.IncWorkerBatchesCreated("0")
	if got := testutil.ToFloat64(WorkerBatchesCreatedTotal.WithLabelValues("0")); got < 1 {
		t.Errorf("WorkerBatchesCreatedTotal = %v, want >= 1", got)
	}

	m.SetWorkerPendingTransactions("0", 100)
	if got := testutil.ToFloat64(WorkerPendingTransactions.WithLabelValues("0")); got != 100 {
		t.Errorf("WorkerPendingTransactions = %v, want 100", got)
	}

	// Test DAG metrics
	m.SetDAGRounds(50)
	if got := testutil.ToFloat64(DAGRounds); got != 50 {
		t.Errorf("DAGRounds = %v, want 50", got)
	}

	m.SetDAGCertificates(200)
	if got := testutil.ToFloat64(DAGCertificates); got != 200 {
		t.Errorf("DAGCertificates = %v, want 200", got)
	}

	// Test gossip metrics
	m.IncGossipMessagesSent("batch")
	if got := testutil.ToFloat64(GossipMessagesSentTotal.WithLabelValues("batch")); got < 1 {
		t.Errorf("GossipMessagesSentTotal = %v, want >= 1", got)
	}

	m.IncGossipMessagesReceived("certificate")
	if got := testutil.ToFloat64(GossipMessagesReceivedTotal.WithLabelValues("certificate")); got < 1 {
		t.Errorf("GossipMessagesReceivedTotal = %v, want >= 1", got)
	}

	// Test histogram observations
	m.ObserveRoundDuration(0.1)
	m.ObserveCertificateCreation(0.01)
	m.ObserveBlockProduction(0.05)
	m.ObserveTransactionLatency(0.2)
}

func TestMetrics_Disabled(t *testing.T) {
	// Reset metrics for this test
	oldCurrentRound := testutil.ToFloat64(CurrentRound)

	m := NewMetrics(false)

	// Operations should be no-ops when disabled
	m.SetCurrentRound(999)

	// Value should not change
	if got := testutil.ToFloat64(CurrentRound); got != oldCurrentRound {
		t.Errorf("CurrentRound changed when disabled: %v -> %v", oldCurrentRound, got)
	}
}

func TestMetricRegistration(t *testing.T) {
	// Verify all metrics are registered
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedMetrics := []string{
		"accumulate_dagbft_current_round",
		"accumulate_dagbft_last_commit_round",
		"accumulate_dagbft_certificates_created_total",
		"accumulate_dagbft_certificates_committed_total",
		"accumulate_dagbft_leader_commits_total",
		"accumulate_dagbft_headers_created_total",
		"accumulate_dagbft_votes_received_total",
		"accumulate_dagbft_votes_sent_total",
		"accumulate_dagbft_dag_rounds",
		"accumulate_dagbft_dag_certificates",
		"accumulate_dagbft_dag_gc_rounds_removed_total",
		"accumulate_dagbft_peers_connected",
		"accumulate_dagbft_round_duration_seconds",
		"accumulate_dagbft_certificate_creation_seconds",
		"accumulate_dagbft_block_production_seconds",
		"accumulate_dagbft_transaction_latency_seconds",
	}

	registeredMetrics := make(map[string]bool)
	for _, mf := range mfs {
		registeredMetrics[*mf.Name] = true
	}

	for _, name := range expectedMetrics {
		if !registeredMetrics[name] {
			t.Errorf("Expected metric %s not registered", name)
		}
	}
}
