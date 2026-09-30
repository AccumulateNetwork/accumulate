// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// `accumulated init node <peer>` must get the partition's genesis from a
// DAG-BFT node, which serves no CometBFT RPC (#3932). This runs a real netsim —
// the daemon's own start path, its HTTP API, its libp2p routing to its own
// consensus address — and calls init's own peer path against it, so nothing
// between init and the file on the node's disk is done by the test. Both
// partitions are asked at the node's base port: a DAG-BFT node has one API.
func TestInitNodeTakesGenesisFromTheNodesAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a netsim")
	}

	rootDir, basePort := startGenesisNetsim(t)

	flagMain.WorkDir = t.TempDir()
	flagInitNode.GenesisDoc = ""
	cmd := new(cobra.Command)
	cmd.SetContext(context.Background())

	for _, c := range []struct {
		name, file string
		typ        protocol.PartitionType
		port       int
	}{
		{"Directory", "dn-genesis.snap", protocol.PartitionTypeDirectory, basePort},
		{"BVN", "bvn1-genesis.snap", protocol.PartitionTypeBlockValidator, basePort},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, gen, err := initNodeFromPeer(cmd, []string{fmt.Sprintf("tcp://127.0.0.1:%d", c.port)}, c.typ)
			require.NoError(t, err)

			want, err := os.ReadFile(filepath.Join(rootDir, c.file))
			require.NoError(t, err)
			require.Equal(t, want, gen, "init did not receive the genesis the node started from")

			// The bootstrap peer is the node, by the libp2p identity its node
			// info reports, on its one P2P port.
			ni, err := jsonrpc.NewClient(fmt.Sprintf("http://127.0.0.1:%d/v3", basePort+int(config.PortOffsetAccumulateApi))).
				NodeInfo(context.Background(), v3.NodeInfoOptions{})
			require.NoError(t, err)
			peer := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", basePort+int(config.PortOffsetAccumulateP2P), ni.PeerID)
			var found bool
			for _, a := range cfg.Accumulate.P2P.BootstrapPeers {
				found = found || a.String() == peer
			}
			require.True(t, found, "%s is not among the bootstrap peers %v", peer, cfg.Accumulate.P2P.BootstrapPeers)
		})
	}
}

// startGenesisNetsim starts a one-BVN, one-validator network in this process
// and waits until its API answers node-info, then returns its root directory
// (where the netsim writes the genesis files) and base port.
func startGenesisNetsim(t *testing.T) (string, int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rootDir := t.TempDir()
	basePort := freeBase(t)

	cfg := &run.Config{
		Network: "InitGenesis",
		Logging: &run.Logging{
			Format: "plain",
			Rules:  []*run.LoggingRule{{Level: slog.LevelError}},
		},
		P2P: &run.P2P{Key: &run.PrivateKeySeed{Seed: record.NewKey("init-genesis")}},
		Configurations: []run.Configuration{
			&run.NetSimConfiguration{
				Listen:     multiaddr.StringCast(fmt.Sprintf("/tcp/%d", basePort)),
				Bvns:       1,
				Validators: 1,
				Globals: &network.GlobalValues{
					Globals: &protocol.NetworkGlobals{MajorBlockSchedule: "0 0 1 1 *"},
				},
			},
		},
	}
	cfg.SetFilePath(filepath.Join(rootDir, "accumulate.toml"))

	inst, err := run.New(ctx, cfg)
	require.NoError(t, err)
	require.NoError(t, inst.Start())
	t.Cleanup(inst.Stop)

	c := jsonrpc.NewClient(fmt.Sprintf("http://127.0.0.1:%d/v3", basePort+int(config.PortOffsetAccumulateApi)))
	for i := 0; i < 60; i++ {
		if _, err = c.NodeInfo(ctx, v3.NodeInfoOptions{}); err == nil {
			return rootDir, basePort
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("the netsim API never answered: %v", err)
	return "", 0
}

// freeBase returns a base port whose DN and BVN port blocks are free.
func freeBase(t *testing.T) int {
	t.Helper()
	for base := 41000 + (os.Getpid()%20)*1000; base < 62000; base += 1000 {
		free := true
		for part := 0; part <= 1 && free; part++ {
			for off := 0; off <= int(config.PortOffsetAccumulateApi) && free; off++ {
				l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", base+part*config.PortOffsetBlockValidator+off))
				if err != nil {
					free = false
					continue
				}
				_ = l.Close()
			}
		}
		if free {
			return base
		}
	}
	t.Fatal("no free port block")
	return 0
}
