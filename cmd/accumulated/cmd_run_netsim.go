// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var cmdRunDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Run all nodes of a local devnet",
	Run:   runDevNet,
	Args:  cobra.NoArgs,
}

var flagRunDevnet = struct {
	Except        []string
	Name          string
	Database      string
	NumBvns       int
	NumValidators int
	NumFollowers  int
	BasePort      int
	BlockInterval time.Duration
	Globals       network.GlobalValues
	Logging       run.Logging
	SoftReset     bool
}{
	Globals: network.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionLatest,
	},
	Logging: run.Logging{
		Format: "plain",
		Rules: []*run.LoggingRule{
			{Level: slog.LevelInfo},
			{Level: slog.LevelError, Modules: []string{"badger", "accumulate", "run", "anchoring"}},
			{Level: slog.LevelError, Modules: []string{"p2p", "abci-client", "pubsub", "txindex", "consensus", "mempool", "blocksync", "rpc-server", "evidence", "statesync", "pex", "proxy", "events", "state"}},
		},
	},
}

func init() {
	cmdRun.AddCommand(cmdRunDevnet)

	cmdRunDevnet.Flags().StringSliceVarP(&flagRunDevnet.Except, "except", "x", nil, "Names of nodes that should not be launched")
	cmdRunDevnet.Flags().StringVar(&flagRunDevnet.Name, "name", "DevNet", "Network name")
	cmdRunDevnet.Flags().IntVarP(&flagRunDevnet.NumBvns, "bvns", "b", 2, "Number of block validator networks to configure")
	cmdRunDevnet.Flags().IntVarP(&flagRunDevnet.NumValidators, "validators", "v", 2, "Number of validator nodes per partition to configure")
	cmdRunDevnet.Flags().IntVarP(&flagRunDevnet.NumFollowers, "followers", "f", 1, "Number of follower nodes per partition to configure")
	cmdRunDevnet.Flags().IntVar(&flagRunDevnet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdRunDevnet.Flags().StringVar(&flagRunDevnet.Database, "database", "", "The type of database to use")
	cmdRunDevnet.Flags().DurationVar(&flagRunDevnet.BlockInterval, "block-interval", 0, "The cadence the deployed network declares; recorded in genesis and paced from by every node (default: the protocol default)")
	cmdRunDevnet.Flags().Var(cmdutil.JsonFlagOf(&flagRunDevnet.Globals), "globals", "Override the default global values")
	cmdRunDevnet.Flags().Var(cmdutil.JsonFlagOf(&flagRunDevnet.Logging), "logging", "Override the default logger configuration")
	cmdRunDevnet.Flags().BoolVar(&flagRunDevnet.SoftReset, "soft-reset", false, "Reset only if necessary")

	setRunFlags(cmdRunDevnet)

	if os.Getenv("FORCE_COLOR") != "" {
		color.NoColor = false
	}
}

func runDevNet(cmd *cobra.Command, _ []string) {
	if flagRun.Debug {
		testing.EnableDebugFeatures()
	}

	cfg := initDevNet(cmd)
	runCfg(cfg, func(s run.Service) bool {
		sub, ok := s.(*run.SubnodeService)
		if !ok {
			return true
		}
		for _, name := range flagRunDevnet.Except {
			if strings.EqualFold(name, sub.Name) {
				return false
			}
		}
		return true
	})
}
