// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// cmdRun groups the in-process networks (`run devnet`). A node runs as
// `accumulated <accumulate.toml>`.
var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run an in-process network",
	Args:  cobra.NoArgs,
}

var flagRun = struct {
	Truncate    bool
	PprofListen string
	Debug       bool
}{}

func init() {
	cmdMain.AddCommand(cmdRun)

	initRunFlags(cmdRun)
}

func initRunFlags(cmd *cobra.Command) {
	cmd.ResetFlags()
	cmd.PersistentFlags().BoolVar(&flagRun.Truncate, "truncate", false, "Truncate Badger if necessary")
	cmd.PersistentFlags().StringVar(&flagRun.PprofListen, "pprof", "", "Address to run net/http/pprof on")
	cmd.PersistentFlags().BoolVar(&flagRun.Debug, "debug", false, "Enable debugging features")

	cmd.PersistentPreRun = func(*cobra.Command, []string) {
		badger.TruncateBadger = flagRun.Truncate

		if flagRun.PprofListen != "" {
			s := new(http.Server)
			s.Addr = flagRun.PprofListen
			s.ReadHeaderTimeout = time.Minute
			go func() { check(s.ListenAndServe()) }() //nolint:gosec
		}

		if flagRun.Debug {
			testing.EnableDebugFeatures()
		}
	}
}
