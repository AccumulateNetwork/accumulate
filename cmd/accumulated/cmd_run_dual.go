// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"net/http"
	"time"

	service2 "github.com/cometbft/cometbft/libs/service"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

var cmdRunDual = &cobra.Command{
	Use:   "run-dual <primary> <secondary>",
	Short: "Run a DN and BVN",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDualNode(cmd, args)
		printOutput(cmd, out, err)
	},
	Args: cobra.ExactArgs(2),
}

var flagRunDual = struct {
	Truncate         bool
	EnableTimingLogs bool

	CiStopAfter time.Duration
}{}

func init() {
	cmdMain.AddCommand(cmdRunDual)

	cmdRunDual.PersistentFlags().BoolVar(&flagRunDual.Truncate, "truncate", false, "Truncate Badger if necessary")
	cmdRunDual.PersistentFlags().BoolVar(&flagRunDual.EnableTimingLogs, "enable-timing-logs", false, "Enable core timing analysis logging")
	cmdRunDual.PersistentFlags().StringVar(&flagRun.PprofListen, "pprof", "", "Address to run net/http/pprof on")

	cmdRunDual.Flags().DurationVar(&flagRunDual.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
	cmdRunDual.Flag("ci-stop-after").Hidden = true

	cmdRunDual.PersistentPreRun = func(*cobra.Command, []string) {
		badger.TruncateBadger = flagRunDual.Truncate
	}
}

func runDualNode(cmd *cobra.Command, args []string) (string, error) {
	if flagRun.PprofListen != "" {
		s := new(http.Server)
		s.Addr = flagRun.PprofListen
		s.ReadHeaderTimeout = time.Minute
		go func() { check(s.ListenAndServe()) }() //nolint:gosec
	}

	prog := NewProgram(cmd, func(cmd *cobra.Command) (string, error) {
		return args[0], nil
	}, func(cmd *cobra.Command) (string, error) {
		return args[1], nil
	})

	flagRun.EnableTimingLogs = flagRunDual.EnableTimingLogs

	if flagRunDual.CiStopAfter != 0 {
		go watchDog(prog, flagRunDual.CiStopAfter)
	}

	err := prog.Run()
	if err != nil {
		//if it is already stopped, that is ok.
		if !errors.Is(err, service2.ErrAlreadyStopped) {
			slog.Error("Service failed", err)
			return "", err
		}
	}
	return "run complete", nil
}
