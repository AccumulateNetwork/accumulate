// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
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
			&HttpService{Router: ServiceReference[*RouterService]("directory")},
		},
	}

	c.file = "../../../.nodes/test.toml"
	require.NoError(t, c.Save())

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

func TestMainNetHttp(t *testing.T) {
	t.Skip("Manual")

	cu, err := user.Current()
	require.NoError(t, err)

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
			Network:        "MainNet",
			BootstrapPeers: accumulate.BootstrapServers,
			Key:            &PrivateKeySeed{Seed: record.NewKey(t.Name())},
			PeerDB:         filepath.Join(cu.HomeDir, ".accumulate", "cache", "peerdb.json"),
		},
		Apps: []Service{
			&HttpService{
				Router: ServiceValue(&RouterService{}),
				PeerMap: []*HttpPeerMapEntry{
					{
						ID:         mustParsePeer("12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj"),
						Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/apollo-mainnet.accumulate.defidevs.io")},
						Partitions: []string{"Apollo", "Directory"},
					},
					{
						ID:         mustParsePeer("12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7"),
						Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/yutu-mainnet.accumulate.defidevs.io")},
						Partitions: []string{"Yutu", "Directory"},
					},
					{
						ID:         mustParsePeer("12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL"),
						Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/chandrayaan-mainnet.accumulate.defidevs.io")},
						Partitions: []string{"Chandrayaan", "Directory"},
					},
				},
			},
		},
	}

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	time.Sleep(time.Hour)

	require.NoError(t, inst.Stop())
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

	require.NoError(t, inst.Stop())
}

func TestReferenceStorage(t *testing.T) {
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
			storage = "bar"

		[[services]]
			type = "storage"
			name = "bar"
			[services.storage]
				type = "memory"
	`), toml.Unmarshal))

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	require.NoError(t, inst.Stop())
}

func TestEmbededRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

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

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	require.NoError(t, inst.Stop())
}

func TestReferenceRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

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

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	require.NoError(t, inst.Stop())
}
