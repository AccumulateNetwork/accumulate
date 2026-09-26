// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TestDevnetBPTHistoryDepthReachesEveryPartition pins that a devnet's
// bpt-history-depth reaches the consensus app of every partition on every
// node. A devnet regenerates its nodes' configurations on every start, so a
// depth set in a node's own file is overwritten; the devnet's configuration is
// the only place it can be set and survive a restart.
func TestDevnetBPTHistoryDepthReachesEveryPartition(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "accumulate.toml")
	require.NoError(t, os.WriteFile(file, []byte(`
network = "TestDevNetHistory"

[[configurations]]
  type = "devnet"
  listen = "/tcp/56656"
  bvns = 2
  validators = 1
  followers = 1
  bpt-history-depth = 1000
`), 0600))

	cfg := new(Config)
	require.NoError(t, cfg.LoadFrom(file))
	cfg.P2P = &P2P{Key: &PrivateKeySeed{Seed: record.NewKey("test-devnet-history")}}

	ctx := logging.With(context.Background(), "test", t.Name())
	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = dir

	require.NoError(t, cfg.Configurations[0].(*DevnetConfiguration).apply(inst, cfg))

	var apps int
	for _, svc := range cfg.Services {
		sub, ok := svc.(*SubnodeService)
		if !ok {
			continue
		}
		for _, ss := range sub.Services {
			cs, ok := ss.(*ConsensusService)
			if !ok {
				continue
			}
			app := cs.App.(*CoreConsensusApp)
			require.NotNil(t, app.BPTHistoryDepth, "%s on %s has no history depth", app.Partition.ID, sub.Name)
			require.Equal(t, uint64(1000), *app.BPTHistoryDepth, "%s on %s", app.Partition.ID, sub.Name)
			apps++
		}

		// What the node runs from is the file written for it
		node := new(Config)
		require.NoError(t, node.LoadFrom(filepath.Join(dir, sub.Name, "accumulate.toml")))
		for _, ss := range node.Services {
			if cs, ok := ss.(*ConsensusService); ok {
				app := cs.App.(*CoreConsensusApp)
				require.NotNil(t, app.BPTHistoryDepth, "%s's file for %s has no history depth", sub.Name, app.Partition.ID)
				require.Equal(t, uint64(1000), *app.BPTHistoryDepth)
			}
		}
	}

	// Two BVNs of two nodes each, every node running the directory and its BVN
	require.Equal(t, 8, apps)
}
