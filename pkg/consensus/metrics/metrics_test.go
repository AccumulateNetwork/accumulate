// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// A labelled family shows in a scrape only once a series is written, so the
// per-partition histograms are not listed; TestPipelineMetricsMoveWhenBlocksAreProduced
// covers that the pipeline writes.
func TestMetricRegistration(t *testing.T) {
	// The partition-labelled families show in a scrape once a series exists.
	for _, c := range []interface {
		WithLabelValues(...string) prometheus.Gauge
	}{
		CurrentRound, LastCommitRound, DAGRounds, DAGCertificates, PeersConnected,
	} {
		c.WithLabelValues("test")
	}
	for _, c := range []interface {
		WithLabelValues(...string) prometheus.Counter
	}{
		CertificatesCreatedTotal, CertificatesCommittedTotal, LeaderCommitsTotal,
		HeadersCreatedTotal, VotesReceivedTotal, VotesSentTotal,
	} {
		c.WithLabelValues("test")
	}

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
