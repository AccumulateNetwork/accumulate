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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// `accumulated init node <peer>` must be able to write a BVN node (#4489).
// This drives the cobra command itself — the flags, the peer path and the
// writer, as an operator runs it — against a netsim in this process that runs
// the Directory and BVN1, and against a peer that runs only BVN1.
func TestInitNodeCommandChoosesThePartition(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a netsim")
	}

	rootDir, basePort := startGenesisNetsim(t)
	dual := fmt.Sprintf("tcp://127.0.0.1:%d", basePort)
	bvnOnly := startBVNOnlyPeer(t, basePort)
	dnWant, err := os.ReadFile(filepath.Join(rootDir, "dn-genesis.snap"))
	require.NoError(t, err)
	bvnWant, err := os.ReadFile(filepath.Join(rootDir, "bvn1-genesis.snap"))
	require.NoError(t, err)

	for _, c := range []struct {
		name  string
		peer  string
		flags []string
		mode  run.CoreValidatorMode
		dn    []byte
		bvn   []byte
	}{
		{"Default", dual, nil, run.CoreValidatorModeDual, dnWant, bvnWant},
		{"BVN", dual, []string{"--partition", "BVN1"}, run.CoreValidatorModeBVN, nil, bvnWant},
		{"BVNAnyCase", dual, []string{"--partition", "bvn1"}, run.CoreValidatorModeBVN, nil, bvnWant},
		{"Directory", dual, []string{"--partition", "Directory"}, run.CoreValidatorModeDN, dnWant, nil},
		{"BVNOnlyPeer", bvnOnly, nil, run.CoreValidatorModeBVN, nil, bvnWant},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			listen := fmt.Sprintf("tcp://127.0.0.1:%d", freeBase(t))
			require.NoError(t, runInitNode(t, append([]string{c.peer, "-w", dir, "--listen", listen, "--no-prometheus"}, c.flags...)...))

			cfg := new(run.Config)
			require.NoError(t, cfg.LoadFrom(filepath.Join(dir, "accumulate.toml")))
			require.Len(t, cfg.Configurations, 1)
			cvc, ok := cfg.Configurations[0].(*run.CoreValidatorConfiguration)
			require.True(t, ok, "init wrote %T", cfg.Configurations[0])
			require.Equal(t, c.mode, cvc.Mode)
			if c.dn == nil {
				require.Empty(t, cvc.DnGenesis)
			} else {
				b, err := os.ReadFile(filepath.Join(dir, cvc.DnGenesis))
				require.NoError(t, err)
				require.Equal(t, c.dn, b, "not the Directory genesis the peer started from")
			}
			if c.bvn == nil {
				require.Empty(t, cvc.BVN)
				require.Empty(t, cvc.BvnGenesis)
			} else {
				require.Equal(t, "BVN1", cvc.BVN)
				b, err := os.ReadFile(filepath.Join(dir, cvc.BvnGenesis))
				require.NoError(t, err)
				require.Equal(t, c.bvn, b, "not the BVN genesis the peer started from")
			}

			// The configuration and its keys load as the node loads them, and
			// the node starts on them: every partition it was written for
			// loads its genesis and opens its database.
			cfg.Logging = &run.Logging{Format: "plain", Rules: []*run.LoggingRule{{Level: slog.LevelError}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			inst, err := run.New(ctx, cfg)
			require.NoError(t, err)
			require.NoError(t, inst.Start())
			inst.Stop()
			for sub, want := range map[string]bool{"dnn": c.dn != nil, "bvnn": c.bvn != nil} {
				_, err := os.Stat(filepath.Join(dir, sub, "data"))
				require.Equal(t, want, err == nil, "%s started: %v", sub, err)
			}
		})
	}

	t.Run("NotRunByThePeer", func(t *testing.T) {
		err := runInitNode(t, bvnOnly, "-w", t.TempDir(), "--partition", "Directory")
		require.Error(t, err)
		require.Contains(t, err.Error(), "Directory")

		err = runInitNode(t, dual, "-w", t.TempDir(), "--partition", "BVN7")
		require.Error(t, err)
		require.Contains(t, err.Error(), "BVN7")
	})
}

// runInitNode runs `accumulated init node <args>` through the command tree and
// returns the error the command reported.
func runInitNode(t *testing.T, args ...string) error {
	t.Helper()
	initInitFlags()
	DidError = nil
	cmdMain.SetArgs(append([]string{"init", "node"}, args...))
	err := cmdMain.ExecuteContext(context.Background())
	if err != nil {
		return err
	}
	return DidError
}

// startBVNOnlyPeer serves API v3 on its own base port as a node that runs only
// BVN1 would: the netsim's answers, with the Directory's consensus service
// gone from its node info and the Directory's consensus and genesis refused.
func startBVNOnlyPeer(t *testing.T, netsimBase int) string {
	t.Helper()
	p := bvnOnlyPeer{jsonrpc.NewClient(fmt.Sprintf("http://127.0.0.1:%d/v3", netsimBase+int(config.PortOffsetAccumulateApi)))}
	h, err := jsonrpc.NewHandler(
		jsonrpc.NodeService{NodeService: p},
		jsonrpc.ConsensusService{ConsensusService: p},
		jsonrpc.GenesisService{GenesisService: p},
	)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("/v3", h)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	base := l.Addr().(*net.TCPAddr).Port - int(config.PortOffsetAccumulateApi)
	return fmt.Sprintf("tcp://127.0.0.1:%d", base)
}

type bvnOnlyPeer struct{ *jsonrpc.Client }

func (p bvnOnlyPeer) NodeInfo(ctx context.Context, opts v3.NodeInfoOptions) (*v3.NodeInfo, error) {
	ni, err := p.Client.NodeInfo(ctx, opts)
	if err != nil {
		return nil, err
	}
	var services []*v3.ServiceAddress
	for _, s := range ni.Services {
		if !strings.EqualFold(s.Argument, protocol.Directory) {
			services = append(services, s)
		}
	}
	ni.Services = services
	return ni, nil
}

func (p bvnOnlyPeer) ConsensusStatus(ctx context.Context, opts v3.ConsensusStatusOptions) (*v3.ConsensusStatus, error) {
	if opts.Partition == "" || strings.EqualFold(opts.Partition, protocol.Directory) {
		return nil, errors.NotFound.With("no consensus:Directory")
	}
	return p.Client.ConsensusStatus(ctx, opts)
}

func (p bvnOnlyPeer) Genesis(ctx context.Context, opts v3.GenesisOptions) ([]byte, error) {
	if opts.Partition == "" || strings.EqualFold(opts.Partition, protocol.Directory) {
		return nil, errors.NotFound.With("no consensus:Directory")
	}
	return p.Client.Genesis(ctx, opts)
}
