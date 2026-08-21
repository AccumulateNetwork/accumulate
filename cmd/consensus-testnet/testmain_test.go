// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestMain quiets consensus logging for the integration tests. The multi-node
// tests emit an INFO line per gossip vote — hundreds per second per node —
// which blew GitLab's 4 MB job-log cap barely halfway through the suite, so
// every CI failure past that point was invisible ("no more output will be
// collected"), and the log-write volume itself slowed starved runners enough
// to trip wall-clock assertions. Warnings and errors still print; set
// CONSENSUS_TEST_VERBOSE=1 to restore the firehose when debugging locally.
func TestMain(m *testing.M) {
	if os.Getenv("CONSENSUS_TEST_VERBOSE") == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		})))
	}
	os.Exit(m.Run())
}

// collectForCert gathers a committed certificate's batches via
// Node.CollectBatches — blocking until every batch is available — and adapts
// them to the map shape the test executors consume. Returns ok=false only on
// context cancellation. The tests' original loops read whatever batches the
// local workers happened to hold and SKIPPED the rest, which on slow CI
// runners meant every node executed a different subset of each certificate:
// 5,798 skips in one failed pipeline, stable divergent processed counts, and
// 1/7 matching state hashes (#4122) — the same bug 9630ea564 fixed in
// production, surviving in the tests' own consumers.
func collectForCert(ctx context.Context, node *consensus.Node, cert *types.Certificate) (map[types.BatchDigest]*types.Batch, []types.BatchDigest, bool) {
	collected, err := node.CollectBatches(ctx, cert)
	if err != nil {
		return nil, nil, false
	}
	batches := make(map[types.BatchDigest]*types.Batch, len(collected))
	digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
	for i, entry := range cert.Header.Payload {
		digests = append(digests, entry.Digest)
		batches[entry.Digest] = collected[i]
	}
	return batches, digests, true
}
