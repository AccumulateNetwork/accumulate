// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log/slog"
	"os"
	"testing"
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
