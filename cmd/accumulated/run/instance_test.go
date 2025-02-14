// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestCoreValidatorConfig(t *testing.T) {
	c := &Config{
		Network: "MainNet",
		Configurations: []Configuration{
			&CoreValidatorConfiguration{
				Listen:        multiaddr.StringCast("/tcp/16591"),
				BVN:           "Apollo",
				EnableHealing: Ptr(true),
				StorageType:   Ptr(StorageTypeBadger),
			},
		},
	}

	// Apply configurations
	inst := new(Instance)
	for _, d := range c.Configurations {
		require.NoError(t, d.apply(inst, c))
	}

	c.Configurations = nil
	b, err := c.Marshal(MarshalTOML)
	require.NoError(t, err)

	fmt.Printf("%s", b)
}

func TestDevNetConfig(t *testing.T) {
	c := &Config{
		Network: "DevNet",
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey()},
		},
		Configurations: []Configuration{
			&DevnetConfiguration{
				Listen:     multiaddr.StringCast("/tcp/26656"),
				Bvns:       1,
				Validators: 1,
			},
		},
	}

	// Apply configurations
	inst := new(Instance)
	inst.rootDir = t.TempDir()
	// inst.rootDir = "../../../.nodes/devnet"
	inst.logger = slog.Default()

	c.file = inst.path("accumulate.toml")
	require.NoError(t, c.Save())

	for _, d := range c.Configurations {
		require.NoError(t, d.apply(inst, c))
	}

	c.Configurations = nil
	b, err := c.Marshal(MarshalTOML)
	require.NoError(t, err)

	fmt.Printf("%s", b)
}

func TestRun2(t *testing.T) {
	t.Skip("Manual")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := new(Config)
	require.NoError(t, c.LoadFrom("../../../.nodes/node-1/accumulate.toml"))

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	time.Sleep(time.Minute)

	inst.Stop()
}

func TestRun(t *testing.T) {
	t.Skip("Manual")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := &Config{
		Network: "DevNet",
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
			},
		},
		P2P: &P2P{
			// BootstrapPeers: accumulate.BootstrapServers,
			Key: &CometNodeKeyFile{Path: "node-1/dnn/config/node_key.json"},
		},
		Services: []Service{
			&ConsensusService{
				NodeDir: "node-1/dnn",
				Genesis: "node-1/dn-genesis.snap",
				App: &CoreConsensusApp{
					EnableHealing: Ptr(true),
					Partition: &protocol.PartitionInfo{
						ID:   protocol.Directory,
						Type: protocol.PartitionTypeDirectory,
					},
				},
			},
			&ConsensusService{
				NodeDir: "node-1/bvnn",
				Genesis: "node-1/bvn-genesis.snap",
				App: &CoreConsensusApp{
					EnableHealing: Ptr(true),
					Partition: &protocol.PartitionInfo{
						ID:   "BVN1",
						Type: protocol.PartitionTypeBlockValidator,
					},
				},
			},
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
			&HttpService{Router: ServiceReference[*RouterService]("directory")},

			&StorageService{
				Name: "BVN1",
				Storage: &BadgerStorage{
					Path: "node-1/bvnn/data/accumulate.db",
				},
			},
			&Querier{Partition: "BVN1"},
			&NetworkService{Partition: "BVN1"},
			&MetricsService{Partition: "BVN1"},
			&EventsService{Partition: "BVN1"},
		},
	}

	c.file = "../../../.nodes/test.toml"
	require.NoError(t, c.Save())

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	inst.Stop()
}

func TestMainNetHttp(t *testing.T) {
	t.Skip("Manual")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := &Config{
		Network: "MainNet",
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
			},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey(t.Name())},
		},
		Configurations: []Configuration{
			&GatewayConfiguration{},
		},
	}

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	time.Sleep(time.Hour)

	inst.Stop()
}

func TestEmbededStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := new(Config)
	require.NoError(t, c.Load([]byte(`
		[p2p]
			network = "DevNet"
			[p2p.key]
				type = "transient"

		[[services]]
			type = "querier"
			partition = "foo"
			[services.storage]
				type = "memory"
	`), toml.Unmarshal))

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	inst.Stop()
}

func TestReferenceStorage(t *testing.T) {
	c := new(Config)
	require.NoError(t, c.Load([]byte(`
		[p2p]
			network = "DevNet"
			[p2p.key]
				type = "transient"

		[[services]]
			type = "querier"
			partition = "foo"
			storage = "bar"

		[[services]]
			type = "storage"
			name = "bar"
			[services.storage]
				type = "memory"
	`), toml.Unmarshal))

	ctx := contextForTest(t)
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	inst.Stop()
}

func TestEmbededRouter(t *testing.T) {
	c := new(Config)
	require.NoError(t, c.Load([]byte(`
		[p2p]
			network = "DevNet"
			[p2p.key]
				type = "transient"

		[[services]]
			type = "http"
			[services.router]
	`), toml.Unmarshal))

	t.Skip("Hangs forever since the network doesn't have the required services")

	ctx := contextForTest(t)
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	inst.Stop()
}

func TestReferenceRouter(t *testing.T) {
	c := new(Config)
	require.NoError(t, c.Load([]byte(`
		[p2p]
			network = "DevNet"
			[p2p.key]
				type = "transient"

		[[services]]
			type = "http"
			router = "router"

		[[services]]
			type = "router"
			name = "router"
	`), toml.Unmarshal))

	t.Skip("Hangs forever since the network doesn't have the required services")

	ctx := contextForTest(t)
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	inst.Stop()
}

func contextForTest(t testing.TB) context.Context {
	ctx := context.Background()
	ctx = logging.With(ctx, "test", t.Name())
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	return ctx
}
