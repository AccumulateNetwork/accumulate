// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

func TestRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := &Config{
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
			},
		},
		P2P: &P2P{
			Network: "DevNet",
			// BootstrapPeers: accumulate.BootstrapServers,
			Key: &CometNodeKeyFile{Path: "node-1/dnn/config/node_key.json"},
		},
		Apps: []Service{
			&ConsensusService{
				NodeDir: "node-1/dnn",
				App: &CoreConsensusApp{
					EnableHealing: true,
					Partition: &protocol.PartitionInfo{
						ID:   protocol.Directory,
						Type: protocol.PartitionTypeDirectory,
					},
				},
			},
		},
		Services: []Service{
			&StorageService{
				Name: "directory",
				Storage: &BadgerStorage{
					Path: "node-1/dnn/data/accumulate.db",
				},
			},
			&Querier{Partition: protocol.Directory},
			&NetworkService{Partition: protocol.Directory},
			&MetricsService{Partition: protocol.Directory},
			&EventsService{Partition: protocol.Directory},
		},
	}

	require.NoError(t, c.SaveTo("../../../.nodes/test.toml"))

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	require.NoError(t, inst.Stop())
}

func TestRun2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := new(Config)
	require.NoError(t, c.LoadFrom("../../../.nodes/test2.toml"))

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	require.NoError(t, inst.Stop())
}
