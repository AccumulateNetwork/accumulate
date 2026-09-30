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
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// `accumulated init node <peer>` and `init dual <peer>` must get the
// partitions' genesis from a DAG-BFT node, which serves no CometBFT RPC
// (#3932), and write the layout `init network` writes (#3910). This runs a
// real netsim — the daemon's own start path, its HTTP API, its libp2p routing
// to its own consensus address — and runs init's own peer path and writer
// against it, so nothing between init and the files on disk is done by the
// test. Every partition is asked at the node's base port: a DAG-BFT node has
// one API.
func TestInitNodeTakesGenesisFromTheNodesAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a netsim")
	}

	rootDir, basePort := startGenesisNetsim(t)
	peerURL := fmt.Sprintf("tcp://127.0.0.1:%d", basePort)
	dnWant, err := os.ReadFile(filepath.Join(rootDir, "dn-genesis.snap"))
	require.NoError(t, err)
	bvnWant, err := os.ReadFile(filepath.Join(rootDir, "bvn1-genesis.snap"))
	require.NoError(t, err)

	// The bootstrap peer is the node, by the libp2p identity its node info
	// reports, on its one P2P port.
	ni, err := jsonrpc.NewClient(fmt.Sprintf("http://127.0.0.1:%d/v3", basePort+int(config.PortOffsetAccumulateApi))).
		NodeInfo(context.Background(), v3.NodeInfoOptions{})
	require.NoError(t, err)
	peer := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", basePort+int(config.PortOffsetAccumulateP2P), ni.PeerID)

	for _, c := range []struct {
		name string
		typ  protocol.PartitionType
		dual bool
		mode run.CoreValidatorMode
		dn   []byte
		bvn  []byte
	}{
		{"Directory", protocol.PartitionTypeDirectory, false, run.CoreValidatorModeDN, dnWant, nil},
		{"BVN", protocol.PartitionTypeBlockValidator, false, run.CoreValidatorModeBVN, nil, bvnWant},
		{"Dual", 0, true, run.CoreValidatorModeDual, dnWant, bvnWant},
	} {
		t.Run(c.name, func(t *testing.T) {
			j, err := joinFromPeer(context.Background(), peerURL, c.typ, c.dual, "", "")
			require.NoError(t, err)
			require.Equal(t, c.dn, j.dnGenesis, "init did not receive the Directory genesis the node started from")
			require.Equal(t, c.bvn, j.bvnGenesis, "init did not receive the BVN genesis the node started from")

			dir := t.TempDir()
			require.NoError(t, writeNodeConfig(dir, j, nodeInitOptions{Listen: "tcp://127.0.0.1:1"}))

			// What init wrote is a node configuration of the current layout,
			// and it loads: keys inline, genesis beside it, the peer as the
			// bootstrap peer.
			cfg := new(run.Config)
			require.NoError(t, cfg.LoadFrom(filepath.Join(dir, "accumulate.toml")))
			require.Equal(t, ni.Network, cfg.Network)
			require.Len(t, cfg.P2P.BootstrapPeers, 1)
			require.Equal(t, peer, cfg.P2P.BootstrapPeers[0].String())
			require.IsType(t, (*run.RawPrivateKey)(nil), cfg.P2P.Key)
			require.Len(t, cfg.Configurations, 1)
			cvc, ok := cfg.Configurations[0].(*run.CoreValidatorConfiguration)
			require.True(t, ok, "init wrote %T", cfg.Configurations[0])
			require.Equal(t, c.mode, cvc.Mode)
			require.IsType(t, (*run.RawPrivateKey)(nil), cvc.ValidatorKey)
			require.True(t, cvc.JoinRunningNetwork != nil && *cvc.JoinRunningNetwork, "a node added to a running network must join it")
			if c.dn != nil {
				b, err := os.ReadFile(filepath.Join(dir, cvc.DnGenesis))
				require.NoError(t, err)
				require.Equal(t, c.dn, b)
			}
			if c.bvn != nil {
				require.Equal(t, "BVN1", cvc.BVN)
				b, err := os.ReadFile(filepath.Join(dir, cvc.BvnGenesis))
				require.NoError(t, err)
				require.Equal(t, c.bvn, b)
			}

			// The node's own keys load as a node would load them.
			_, err = run.New(context.Background(), cfg)
			require.NoError(t, err)

			// A second init does not overwrite the first.
			require.Error(t, writeNodeConfig(dir, j, nodeInitOptions{}))
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
