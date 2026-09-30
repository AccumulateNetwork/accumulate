// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
)

// The Docker harness initializes its network with `accumulated init network
// test/docker/docker-network.yml`, and every node starts from the
// accumulate.toml that writes. With the CometBFT config and daemon gone
// (#3910), what it writes must be unchanged: every node's file loads, its keys
// load, and a validator still listens on its base port (26656 in that file),
// which the CometBFT P2P offset — zero — used to spell.
func TestInitNetworkWritesTheHarnessLayout(t *testing.T) {
	if testing.Short() {
		t.Skip("builds genesis for the harness network")
	}

	flagMain.WorkDir = t.TempDir()
	flagInit.FaucetSeed = "FAUCET"
	cmd := new(cobra.Command)
	cmd.SetContext(context.Background())
	initNetwork(cmd, []string{filepath.Join("..", "..", "test", "docker", "docker-network.yml")})

	ents, err := os.ReadDir(flagMain.WorkDir)
	require.NoError(t, err)
	var validators int
	for _, ent := range ents {
		file := filepath.Join(flagMain.WorkDir, ent.Name(), "accumulate.toml")
		if _, err := os.Stat(file); err != nil {
			continue
		}
		cfg := new(run.Config)
		require.NoError(t, cfg.LoadFrom(file), ent.Name())
		_, err := run.New(context.Background(), cfg)
		require.NoError(t, err, "%s: the node's keys do not load", ent.Name())

		for _, c := range cfg.Configurations {
			cvc, ok := c.(*run.CoreValidatorConfiguration)
			if !ok {
				continue
			}
			validators++
			require.Equal(t, "/ip4/0.0.0.0/tcp/26656", cvc.Listen.String(), ent.Name())
			require.IsType(t, (*run.RawPrivateKey)(nil), cvc.ValidatorKey, ent.Name())
		}
	}
	require.NotZero(t, validators, "init network wrote no validator")
}
