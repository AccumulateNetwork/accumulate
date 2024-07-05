// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	tmconfig "github.com/cometbft/cometbft/config"
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
	Globals       network.GlobalValues
	Logging       run.Logging
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
	cmdRunDevnet.Flags().Var(cmdutil.JsonFlagOf(&flagRunDevnet.Globals), "globals", "Override the default global values")
	cmdRunDevnet.Flags().Var(cmdutil.JsonFlagOf(&flagRunDevnet.Logging), "logging", "Override the default logger configuration")

	setRunFlags(cmdRunDevnet)

	if os.Getenv("FORCE_COLOR") != "" {
		color.NoColor = false
	}
}

var colors = []*color.Color{
	color.New(color.FgRed),
	color.New(color.FgGreen),
	color.New(color.FgYellow),
	color.New(color.FgBlue),
	color.New(color.FgMagenta),
	color.New(color.FgCyan),
}

var fallbackColor = color.New(color.FgHiBlack)

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

var nodeIdLen int

var partitionColor = map[string]*color.Color{}

func newNodeWriter(w io.Writer, format, partition string, node int, color bool) io.Writer {
	switch format {
	case tmconfig.LogFormatPlain:
		id := fmt.Sprintf("%s.%d", partition, node)
		if nodeIdLen < len(id) {
			nodeIdLen = len(id)
		}
		s := fmt.Sprintf("[%s]", id) + strings.Repeat(" ", nodeIdLen-len(id)+1)
		if !color {
			return &plainNodeWriter{s, w}
		}

		c, ok := partitionColor[partition]
		if !ok {
			c = fallbackColor
			if len(colors) > 0 {
				c = colors[0]
				colors = colors[1:]
			}
			partitionColor[partition] = c
		}

		s = c.Sprint(s)
		return &plainNodeWriter{s, w}

	case tmconfig.LogFormatJSON:
		s := fmt.Sprintf(`"partition":"%s","node":%d`, partition, node)
		return &jsonNodeWriter{s, w}

	default:
		return w
	}
}

type plainNodeWriter struct {
	s string
	w io.Writer
}

func (w *plainNodeWriter) Write(b []byte) (int, error) {
	c := make([]byte, len(w.s)+len(b))
	n := copy(c, []byte(w.s))
	copy(c[n:], b)

	n, err := w.w.Write(c)
	if n >= len(w.s) {
		n -= len(w.s)
	}
	return n, err
}

type jsonNodeWriter struct {
	s string
	w io.Writer
}

func (w *jsonNodeWriter) Write(b []byte) (int, error) {
	if b[0] != '{' {
		return w.w.Write(b)
	}

	c := make([]byte, 0, len(w.s)+len(b)+1)
	c = append(c, '{')
	c = append(c, []byte(w.s)...)
	c = append(c, ',')
	c = append(c, b[1:]...)

	n, err := w.w.Write(c)
	if n >= len(w.s)+1 {
		n -= len(w.s) + 1
	}
	return n, err
}
